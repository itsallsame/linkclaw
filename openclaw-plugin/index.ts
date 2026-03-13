import { fileURLToPath } from "node:url";

import {
  type LinkClawBridgeRequest,
  runLinkClaw,
  type LinkClawPluginConfig,
} from "./src/bridge.ts";
import { runPublishSkill } from "./src/publish-skill.ts";

type ToolContent = {
  type: "text";
  text: string;
};

type ToolResult = {
  content: ToolContent[];
};

type ToolRegistration = {
  name: string;
  description: string;
  optional?: boolean;
  parameters: Record<string, unknown>;
  execute: (params: Record<string, unknown>) => Promise<ToolResult>;
};

export type PluginAPI = {
  config?: LinkClawPluginConfig;
  getConfig?: () => LinkClawPluginConfig | undefined;
  registerTool: (tool: ToolRegistration) => void;
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

export default function registerLinkClawPlugin(api: PluginAPI): void {
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
}
