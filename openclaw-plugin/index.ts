import { fileURLToPath } from "node:url";

import {
  type LinkClawBridgeRequest,
  runLinkClaw,
  type LinkClawPluginConfig,
} from "./src/bridge.ts";
import {
  runConnectCommand,
  runContactsCommand,
  runFindCommand,
  runImportCommand,
  runInboxCommand,
  runMessageCommand,
  runReplyCommand,
  runSetupCommand,
  runShareCommand,
  runStatusCommand,
  runSyncCommand,
  runThreadCommand,
} from "./src/commands.ts";
import {
  attachDIDLinkToOutgoingEvent,
  type HookEvent,
  handlePassiveDiscovery,
} from "./src/discovery.ts";
import {
  appendSyncMessage,
  createBackgroundSyncService,
  triggerBackgroundSync,
} from "./src/messaging.ts";
import { runPublishSkill } from "./src/publish-skill.ts";

type ToolContent = {
  type: "text";
  text: string;
};

type ToolResult = {
  content: ToolContent[];
};

type CommandResult = {
  type: "message";
  message: string;
};

type ToolRegistration = {
  name: string;
  description: string;
  optional?: boolean;
  parameters: Record<string, unknown>;
  execute: (params: Record<string, unknown>) => Promise<ToolResult>;
};

type CommandHandler = (args: string) => Promise<CommandResult> | CommandResult;

type CommandRegistration = {
  name: string;
  description: string;
  acceptsArgs?: boolean;
  handler: (ctx: { args?: string }) => Promise<CommandResult> | CommandResult;
};

type PluginAPI = {
  config?: LinkClawPluginConfig;
  getConfig?: () => LinkClawPluginConfig | undefined;
  registerTool: (tool: ToolRegistration) => void;
  registerCommand?: ((command: CommandRegistration) => void) | ((name: string, description: string, handler: CommandHandler) => void);
  registerHook?: (
    events: string | string[],
    handler: (event: HookEvent) => Promise<void> | void,
    opts?: {
      name?: string;
      description?: string;
      register?: boolean;
    },
  ) => void;
  registerService?: (service: {
    id: string;
    start: () => void | Promise<void>;
    stop?: () => void | Promise<void>;
  }) => void;
  on?: (name: string, handler: (event: unknown) => Promise<void> | void) => void;
  logger?: {
    info?: (message: string) => void;
    warn?: (message: string) => void;
  };
};

const pluginRoot = fileURLToPath(new URL(".", import.meta.url));

function loadConfig(api: PluginAPI): LinkClawPluginConfig {
  return api.getConfig?.() ?? api.config ?? {};
}

function asBridgeRequest(params: Record<string, unknown>): LinkClawBridgeRequest {
  return {
    command: String(params.command ?? ""),
    home: asOptionalString(params.home),
    canonicalId: asOptionalString(params.canonicalId),
    displayName: asOptionalString(params.displayName),
    origin: asOptionalString(params.origin),
    output: asOptionalString(params.output),
    tier: asOptionalString(params.tier),
    input: asOptionalString(params.input),
    identifier: asOptionalString(params.identifier),
    body: asOptionalString(params.body),
    limit: asOptionalNumber(params.limit),
    trustLevel: asOptionalString(params.trustLevel),
    reason: asOptionalString(params.reason),
    noteBody: asOptionalString(params.noteBody),
    allowDiscovered: asOptionalBoolean(params.allowDiscovered),
    allowMismatch: asOptionalBoolean(params.allowMismatch),
    riskFlags: asOptionalStringArray(params.riskFlags),
    clearRiskFlags: asOptionalBoolean(params.clearRiskFlags),
  };
}

function asOptionalString(value: unknown): string | undefined {
  if (typeof value !== "string") {
    return undefined;
  }
  const trimmed = value.trim();
  return trimmed === "" ? undefined : trimmed;
}

function asOptionalBoolean(value: unknown): boolean | undefined {
  return typeof value === "boolean" ? value : undefined;
}

function asOptionalNumber(value: unknown): number | undefined {
  return typeof value === "number" && Number.isFinite(value) ? value : undefined;
}

function asOptionalStringArray(value: unknown): string[] | undefined {
  if (!Array.isArray(value)) {
    return undefined;
  }
  const items = value
    .filter((item): item is string => typeof item === "string")
    .map((item) => item.trim())
    .filter((item) => item !== "");
  return items.length > 0 ? items : [];
}

function asJSONText(value: unknown): string {
  return JSON.stringify(value, null, 2);
}

function registerPluginCommand(
  api: PluginAPI,
  name: string,
  description: string,
  handler: CommandHandler,
): void {
  const registrar = api.registerCommand;
  if (!registrar) {
    return;
  }
  if (registrar.length <= 1) {
    (registrar as (command: CommandRegistration) => void)({
      name,
      description,
      acceptsArgs: true,
      handler: async (ctx) => await handler(ctx.args ?? ""),
    });
    return;
  }
  (registrar as (legacyName: string, legacyDescription: string, legacyHandler: CommandHandler) => void)(
    name,
    description,
    handler,
  );
}

function registerLifecycleHook(
  api: PluginAPI,
  names: string[],
  handler: (event: unknown) => Promise<void> | void,
): void {
  if (!api.on) {
    return;
  }
  for (const name of names) {
    api.on(name, handler);
  }
}

const plugin = {
  id: "linkclaw",
  name: "LinkClaw",
  description:
    "OpenClaw bridge for LinkClaw identity publishing, contact exchange, and relay-backed direct messaging.",
  register(api: PluginAPI): void {
    api.registerTool({
      name: "linkclaw_core",
      description:
        "Run the local linkclaw core binary via the stable CLI JSON contract for init, publish, inspect, import, and known subcommands.",
      optional: true,
      parameters: {
        type: "object",
        additionalProperties: false,
        required: ["command"],
        properties: {
          command: {
            type: "string",
            enum: [
              "init",
              "publish",
              "inspect",
              "import",
              "card_export",
              "card_import",
              "message_send",
              "message_inbox",
              "message_thread",
              "message_outbox",
              "message_sync",
              "known_ls",
              "known_show",
              "known_trust",
              "known_note",
              "known_refresh",
              "known_rm",
            ],
          },
          home: { type: "string" },
          canonicalId: { type: "string" },
          displayName: { type: "string" },
          origin: { type: "string" },
          output: { type: "string" },
          tier: { type: "string", enum: ["minimum", "recommended", "full"] },
          input: { type: "string" },
          identifier: { type: "string" },
          body: { type: "string" },
          limit: { type: "number" },
          trustLevel: {
            type: "string",
            enum: ["unknown", "seen", "verified", "trusted", "pinned"],
          },
          riskFlags: {
            type: "array",
            items: { type: "string" },
          },
          clearRiskFlags: { type: "boolean" },
          reason: { type: "string" },
          noteBody: { type: "string" },
          allowDiscovered: { type: "boolean" },
          allowMismatch: { type: "boolean" },
        },
      },
      async execute(params) {
        const result = await runLinkClaw(loadConfig(api), asBridgeRequest(params), pluginRoot);
        return {
          content: [
            {
              type: "text",
              text: asJSONText(result),
            },
          ],
        };
      },
    });

    api.registerTool({
      name: "linkclaw_publish",
      description:
        "Run linkclaw publish and format artifacts, manifest, and checks for the OpenClaw publishing skill, including manifest fallback on publish failures.",
      optional: true,
      parameters: {
        type: "object",
        additionalProperties: false,
        properties: {
          command: {
            type: "string",
            description:
              "Raw slash-command args. Example: --origin https://agent.example --tier full",
          },
          home: { type: "string" },
          origin: { type: "string" },
          output: { type: "string" },
          tier: { type: "string", enum: ["minimum", "recommended", "full"] },
          commandName: { type: "string" },
          skillName: { type: "string" },
        },
      },
      async execute(params) {
        const text = await runPublishSkill(loadConfig(api), params, pluginRoot);
        return {
          content: [{ type: "text", text }],
        };
      },
    });

    registerPluginCommand(
      api,
      "linkclaw-setup",
      "Initialize LinkClaw in the configured home and verify the local binary/identity setup.",
      async (args) => runSetupCommand(loadConfig(api), args, pluginRoot),
    );

    registerPluginCommand(
      api,
      "linkclaw-status",
      "Show LinkClaw readiness, health checks, contacts, and inbox summary for the configured home.",
      async (args) => runStatusCommand(loadConfig(api), args, pluginRoot),
    );

    registerPluginCommand(
      api,
      "linkclaw-import",
      "Import a LinkClaw did.json or agent-card.json link into the local known contacts store.",
      async (args) => runImportCommand(loadConfig(api), args, pluginRoot),
    );

    registerPluginCommand(
      api,
      "linkclaw-share",
      "Share the published LinkClaw agent-card and did.json links for the configured publish origin.",
      async (args) => runShareCommand(loadConfig(api), args, pluginRoot),
    );

    registerPluginCommand(
      api,
      "linkclaw-connect",
      "Import a LinkClaw identity card into the local contacts book.",
      async (args) => runConnectCommand(loadConfig(api), args, pluginRoot),
    );

    registerPluginCommand(
      api,
      "linkclaw-contacts",
      "List saved LinkClaw contacts with trust and verification state.",
      async (args) => runContactsCommand(loadConfig(api), args, pluginRoot),
    );

    registerPluginCommand(
      api,
      "linkclaw-find",
      "Search saved LinkClaw contacts by display name, contact id, or canonical id.",
      async (args) => runFindCommand(loadConfig(api), args, pluginRoot),
    );

    registerPluginCommand(
      api,
      "linkclaw-message",
      "Send a direct LinkClaw message to an imported contact.",
      async (args) => runMessageCommand(loadConfig(api), args, pluginRoot),
    );

    registerPluginCommand(
      api,
      "linkclaw-reply",
      "Reply to one LinkClaw contact from the current conversation flow.",
      async (args) => runReplyCommand(loadConfig(api), args, pluginRoot),
    );

    registerPluginCommand(
      api,
      "linkclaw-thread",
      "Show recent messages for one LinkClaw contact.",
      async (args) => runThreadCommand(loadConfig(api), args, pluginRoot),
    );

    registerPluginCommand(
      api,
      "linkclaw-inbox",
      "Show LinkClaw conversations from the local inbox.",
      async (args) => runInboxCommand(loadConfig(api), args, pluginRoot),
    );

    registerPluginCommand(
      api,
      "linkclaw-sync",
      "Sync LinkClaw messages from the configured relay.",
      async (args) => runSyncCommand(loadConfig(api), args, pluginRoot),
    );

    api.registerHook?.(
      "message:preprocessed",
      async (event) => {
        await handlePassiveDiscovery(loadConfig(api), event, pluginRoot);
      },
      {
        name: "linkclaw-passive-discovery",
        description:
          "Inspect inbound LinkClaw artifact links and suggest importing identities that are not already known.",
      },
    );

    api.registerService?.(
      createBackgroundSyncService({
        config: loadConfig(api),
        pluginRoot,
        logger: api.logger,
      }),
    );

    registerLifecycleHook(api, ["message_sending"], async (event) => {
      const config = loadConfig(api);
      if (!config.publishOrigin) {
        return;
      }
      attachDIDLinkToOutgoingEvent(event, config.publishOrigin);
    });

    const syncHandler = async (event: unknown) => {
      const message = await triggerBackgroundSync(loadConfig(api), pluginRoot);
      if (message) {
        appendSyncMessage(event, message);
      }
    };

    registerLifecycleHook(api, ["session_start"], syncHandler);
    registerLifecycleHook(api, ["message_received"], syncHandler);
  },
};

export default plugin;
