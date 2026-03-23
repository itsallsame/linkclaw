import { readFile } from "node:fs/promises";

import {
  LinkClawCommandError,
  type LinkClawPluginConfig,
  resolveLinkClawBinary,
  resolveLinkClawHome,
  resolveRelayUrl,
  runLinkClaw,
} from "./bridge.ts";
import {
  formatShareMessage,
  hasShareableArtifacts,
  toInspectResult,
} from "./discovery.ts";
import { tokenizeCommand } from "./publish-skill.ts";

type CommandResult = {
  type: "message";
  message: string;
};

type SetupOptions = {
  canonicalId?: string;
  displayName?: string;
  home?: string;
  checkOnly?: boolean;
};

type StatusOptions = {
  home?: string;
};

type ImportOptions = {
  input: string;
  allowDiscovered: boolean;
  allowMismatch: boolean;
  home?: string;
};

type ShareOptions = {
  origin?: string;
  home?: string;
  card?: boolean;
};

type ConnectOptions = {
  input: string;
  home?: string;
};

type FindOptions = {
  query: string;
  home?: string;
};

type ContactsOptions = {
  query?: string;
  home?: string;
};

type MessageOptions = {
  identifier: string;
  body: string;
  home?: string;
};

type ThreadOptions = {
  identifier: string;
  home?: string;
  limit?: number;
};

type InboxOptions = {
  home?: string;
  query?: string;
};

type KnownContact = {
  contactId?: string;
  canonicalId?: string;
  displayName?: string;
};

type InboxConversation = {
  canonicalId?: string;
  displayName?: string;
  status?: string;
  lastMessageAt?: string;
};

type SetupHealth = {
  binaryPath: string;
  relayStatus: string;
  publishStatus: string;
};

const replyContextByHome = new Map<string, string>();

type ContactResolutionContext =
  | { command: "message"; body?: string }
  | { command: "reply"; body?: string }
  | { command: "thread" };

export async function runImportCommand(
  config: LinkClawPluginConfig,
  rawArgs: string,
  pluginRoot: string,
): Promise<CommandResult> {
  let options: ImportOptions;
  try {
    options = parseImportCommand(rawArgs);
  } catch (error) {
    return {
      type: "message",
      message: `linkclaw import command failed: ${(error as Error).message}`,
    };
  }

  if (options.input === "") {
    return {
      type: "message",
      message:
        "Usage: /linkclaw-import [--allow-discovered] [--allow-mismatch] <did-or-agent-card-url>",
    };
  }

  try {
    const envelope = await runLinkClaw(
      config,
      {
        command: "import",
        home: options.home,
        input: options.input,
        allowDiscovered: options.allowDiscovered,
        allowMismatch: options.allowMismatch,
      },
      pluginRoot,
    );
    return {
      type: "message",
      message: formatImportMessage(envelope.result),
    };
  } catch (error) {
    return {
      type: "message",
      message: formatCommandError("linkclaw import", error),
    };
  }
}

export async function runSetupCommand(
  config: LinkClawPluginConfig,
  rawArgs: string,
  pluginRoot: string,
): Promise<CommandResult> {
  let options: SetupOptions;
  try {
    options = parseSetupCommand(rawArgs);
  } catch (error) {
    return {
      type: "message",
      message: `linkclaw setup command failed: ${(error as Error).message}`,
    };
  }

  const health = await collectSetupHealth(config, options.home, pluginRoot);

  try {
    const knownEnvelope = await runLinkClaw(
      config,
      {
        command: "known_ls",
        home: options.home,
      },
      pluginRoot,
    );
    const result = asObject(knownEnvelope.result);
    const contacts = Array.isArray(result?.contacts) ? result.contacts.length : 0;
    return {
      type: "message",
      message: [
        "LinkClaw 已就绪。",
        `home: ${resolveLinkClawHome(options.home, config)}`,
        `contacts: ${contacts}`,
        ...formatHealthSection(health),
        "下一步：",
        "- 运行 /linkclaw-share 分享你的身份卡",
        "- 运行 /linkclaw-connect <card-or-url> 添加联系人",
        "- 运行 /linkclaw-message <contact> <text> 发送消息",
      ].join("\n"),
    };
  } catch (error) {
    if (!isStateNotInitialized(error)) {
      return {
        type: "message",
        message: formatCommandError("linkclaw setup", error),
      };
    }
  }

  if (options.checkOnly) {
    return {
      type: "message",
      message: [
        "LinkClaw 检查完成。",
        `home: ${resolveLinkClawHome(options.home, config)}`,
        "状态：未初始化",
        ...formatHealthSection(health),
        "下一步：",
        "- 运行 /linkclaw-setup --display-name <name> 初始化当前 home",
      ].join("\n"),
    };
  }

  if (!options.canonicalId && !options.displayName) {
    return {
      type: "message",
      message: [
        "LinkClaw 当前 home 还没有初始化。",
        "用法：/linkclaw-setup [--check-only] [--display-name <name>] [--canonical-id <did:key|did:web>]",
        "示例：/linkclaw-setup --display-name Alice",
      ].join("\n"),
    };
  }

  try {
    const initEnvelope = await runLinkClaw(
      config,
      {
        command: "init",
        home: options.home,
        canonicalId: options.canonicalId,
        displayName: options.displayName,
      },
      pluginRoot,
    );
    const result = asObject(initEnvelope.result);
    return {
      type: "message",
      message: [
        "LinkClaw 初始化完成。",
        `home: ${String(result?.home ?? options.home ?? config.home ?? "")}`,
        `canonical id: ${String(result?.identity && asObject(result.identity)?.canonical_id ? asObject(result.identity)?.canonical_id : options.canonicalId)}`,
        ...(describeMessagingReadiness(result).length > 0 ? describeMessagingReadiness(result) : []),
        ...formatHealthSection(health),
        "下一步：",
        "- 运行 /linkclaw-share 发布或分享你的身份卡",
        "- 对方分享后，运行 /linkclaw-connect <card-or-url> 导入联系人",
      ].join("\n"),
    };
  } catch (error) {
    return {
      type: "message",
      message: formatCommandError("linkclaw setup", error),
    };
  }
}

export async function runOnboardingCommand(
  config: LinkClawPluginConfig,
  rawArgs: string,
  pluginRoot: string,
): Promise<CommandResult> {
  const trimmed = rawArgs.trim();
  const delegatedArgs = trimmed === "" ? "--check-only" : rawArgs;
  const result = await runSetupCommand(config, delegatedArgs, pluginRoot);
  return {
    type: "message",
    message: ["LinkClaw 首次引导", result.message].join("\n"),
  };
}

function describeMessagingReadiness(result: Record<string, unknown> | undefined): string[] {
  const messaging = asObject(result?.messaging);
  if (!messaging) {
    return [];
  }
  const lines: string[] = [];
  const transport = readString(messaging.transport);
  const recipientId = readString(messaging.recipient_id);
  const relayUrl = readString(messaging.relay_url);
  const ready = typeof messaging.ready === "boolean" ? messaging.ready : undefined;
  if (transport) {
    lines.push(`messaging: ${humanMessagingTransportLabel(transport)}${ready !== undefined ? ` | ready=${ready}` : ""}`);
  }
  if (recipientId) {
    lines.push(`recipient id: ${recipientId}`);
  }
  if (relayUrl) {
    lines.push(`offline recovery endpoint: ${relayUrl}`);
  }
  return lines;
}

function readString(value: unknown): string | undefined {
  return typeof value === "string" && value.trim() !== "" ? value.trim() : undefined;
}

function humanMessagingTransportLabel(_transport: string): string {
  return "runtime-managed";
}

export async function runStatusCommand(
  config: LinkClawPluginConfig,
  rawArgs: string,
  pluginRoot: string,
): Promise<CommandResult> {
  let options: StatusOptions;
  try {
    options = parseStatusCommand(rawArgs);
  } catch (error) {
    return {
      type: "message",
      message: `linkclaw status command failed: ${(error as Error).message}`,
    };
  }

  try {
    const health = await collectSetupHealth(config, options.home, pluginRoot);
    const statusEnvelope = await runLinkClaw(
      config,
      {
        command: "message_status",
        home: options.home,
      },
      pluginRoot,
    );
    return {
      type: "message",
      message: formatStatus(
        options.home,
        config,
        health,
        statusEnvelope.result,
      ),
    };
  } catch (error) {
    if (isStateNotInitialized(error)) {
      const health = await collectSetupHealth(config, options.home, pluginRoot);
      return {
        type: "message",
        message: [
        "LinkClaw 状态",
        `home: ${resolveLinkClawHome(options.home, config)}`,
        "状态：未初始化",
        ...formatHealthSection(health),
        "下一步：",
        "- 运行 /linkclaw-setup --display-name <name>",
      ].join("\n"),
      };
    }
    return {
      type: "message",
      message: formatCommandError("linkclaw status", error),
    };
  }
}

export async function runShareCommand(
  config: LinkClawPluginConfig,
  rawArgs: string,
  pluginRoot: string,
): Promise<CommandResult> {
  let options: ShareOptions;
  try {
    options = parseShareCommand(rawArgs);
  } catch (error) {
    return {
      type: "message",
      message: `linkclaw share command failed: ${(error as Error).message}`,
    };
  }

  if (options.card) {
    try {
      const envelope = await runLinkClaw(
        config,
        {
          command: "card_export",
          home: options.home,
        },
        pluginRoot,
      );
      return {
        type: "message",
        message: formatShareCardMessage(envelope.result),
      };
    } catch (error) {
      return {
        type: "message",
        message: formatCommandError("linkclaw share", error),
      };
    }
  }

  const origin = options.origin ?? config.publishOrigin;
  if (!origin) {
    return {
      type: "message",
      message:
        "Set plugins.entries.linkclaw.config.publishOrigin, pass /linkclaw-share --origin https://agent.example, or use /linkclaw-share --card.",
    };
  }

  try {
    const envelope = await runLinkClaw(
      config,
      {
        command: "inspect",
        input: origin,
      },
      pluginRoot,
    );
    const inspection = toInspectResult(envelope.result);
    if (!hasShareableArtifacts(inspection)) {
      return {
        type: "message",
        message:
          "No shareable LinkClaw bundle was found. Publish the identity with at least recommended tier first.",
      };
    }
    return {
      type: "message",
      message: formatShareMessage(inspection, origin),
    };
  } catch (error) {
    return {
      type: "message",
      message: formatCommandError("linkclaw share", error),
    };
  }
}

export async function runConnectCommand(
  config: LinkClawPluginConfig,
  rawArgs: string,
  pluginRoot: string,
): Promise<CommandResult> {
  let options: ConnectOptions;
  try {
    options = parseConnectCommand(rawArgs);
  } catch (error) {
    return {
      type: "message",
      message: `linkclaw connect command failed: ${(error as Error).message}`,
    };
  }

  if (options.input === "") {
    return {
      type: "message",
      message: "Usage: /linkclaw-connect [--home /path/to/home] <identity-card-json>",
    };
  }

  try {
    const normalizedInput = await normalizeConnectInput(options.input);
    const envelope = await runLinkClaw(
      config,
      {
        command: "card_import",
        home: options.home,
        input: normalizedInput,
      },
      pluginRoot,
    );
    return {
      type: "message",
      message: formatConnectMessage(envelope.result),
    };
  } catch (error) {
    return {
      type: "message",
      message: formatCommandError("linkclaw connect", error),
    };
  }
}

export async function runFindCommand(
  config: LinkClawPluginConfig,
  rawArgs: string,
  pluginRoot: string,
): Promise<CommandResult> {
  let options: FindOptions;
  try {
    options = parseFindCommand(rawArgs);
  } catch (error) {
    return {
      type: "message",
      message: `linkclaw find command failed: ${(error as Error).message}`,
    };
  }

  if (options.query === "") {
    return {
      type: "message",
      message: "Usage: /linkclaw-find [--home /path/to/home] <query>",
    };
  }

  try {
    const contacts = await loadKnownContacts(config, options.home, pluginRoot);
    return {
      type: "message",
      message: formatFindResults(options.query, filterContactsByQuery(contacts, options.query)),
    };
  } catch (error) {
    return {
      type: "message",
      message: formatCommandError("linkclaw find", error),
    };
  }
}

export async function runContactsCommand(
  config: LinkClawPluginConfig,
  rawArgs: string,
  pluginRoot: string,
): Promise<CommandResult> {
  let options: ContactsOptions;
  try {
    options = parseContactsCommand(rawArgs);
  } catch (error) {
    return {
      type: "message",
      message: `linkclaw contacts command failed: ${(error as Error).message}`,
    };
  }

  try {
    const envelope = await runLinkClaw(
      config,
      {
        command: "known_ls",
        home: options.home,
      },
      pluginRoot,
    );
    return {
      type: "message",
      message: formatContacts(envelope.result, options.query),
    };
  } catch (error) {
    return {
      type: "message",
      message: formatCommandError("linkclaw contacts", error),
    };
  }
}

export async function runThreadCommand(
  config: LinkClawPluginConfig,
  rawArgs: string,
  pluginRoot: string,
): Promise<CommandResult> {
  let options: ThreadOptions;
  try {
    options = parseThreadCommand(rawArgs);
  } catch (error) {
    return {
      type: "message",
      message: `linkclaw thread command failed: ${(error as Error).message}`,
    };
  }

  if (options.identifier === "") {
    return {
      type: "message",
      message: "Usage: /linkclaw-thread [--home /path/to/home] [--limit 20] <contact>",
    };
  }

  try {
    const resolvedIdentifier = await resolveContactIdentifier(
      config,
      options.home,
      options.identifier,
      pluginRoot,
      { command: "thread" },
    );
    rememberReplyContext(config, options.home, resolvedIdentifier);
    const envelope = await runLinkClaw(
      config,
      {
        command: "message_thread",
        home: options.home,
        identifier: resolvedIdentifier,
        limit: options.limit,
      },
      pluginRoot,
    );
    return {
      type: "message",
      message: formatThread(envelope.result),
    };
  } catch (error) {
    return {
      type: "message",
      message: formatCommandError("linkclaw thread", error),
    };
  }
}

export async function runMessageCommand(
  config: LinkClawPluginConfig,
  rawArgs: string,
  pluginRoot: string,
): Promise<CommandResult> {
  let options: MessageOptions;
  try {
    options = parseMessageCommand(rawArgs);
  } catch (error) {
    return {
      type: "message",
      message: `linkclaw message command failed: ${(error as Error).message}`,
    };
  }

  if (options.identifier === "" || options.body === "") {
    return {
      type: "message",
      message: 'Usage: /linkclaw-message [--home /path/to/home] <contact> <message>\nTip: quote contact names with spaces, for example /linkclaw-message "Alice Example" hello',
    };
  }

  return sendMessageCommand(config, options, pluginRoot, "linkclaw message");
}

export async function runReplyCommand(
  config: LinkClawPluginConfig,
  rawArgs: string,
  pluginRoot: string,
): Promise<CommandResult> {
  let options: MessageOptions;
  try {
    options = parseReplyCommand(rawArgs);
  } catch (error) {
    return {
      type: "message",
      message: `linkclaw reply command failed: ${(error as Error).message}`,
    };
  }

  if (options.body === "") {
    return {
      type: "message",
      message: 'Usage: /linkclaw-reply [--home /path/to/home] [<contact>] <message>\nTip: quote contact names with spaces, for example /linkclaw-reply "Alice Example" hello\nTip: omit <contact> to reply to your most recent known conversation.',
    };
  }

  try {
    const resolvedOptions = await resolveReplyOptions(config, options, pluginRoot);
    return await sendMessageCommand(config, resolvedOptions, pluginRoot, "linkclaw reply");
  } catch (error) {
    return {
      type: "message",
      message: formatCommandError("linkclaw reply", error),
    };
  }
}

async function sendMessageCommand(
  config: LinkClawPluginConfig,
  options: MessageOptions,
  pluginRoot: string,
  errorPrefix: string,
): Promise<CommandResult> {
  try {
    const resolvedIdentifier = await resolveContactIdentifier(
      config,
      options.home,
      options.identifier,
      pluginRoot,
      {
        command: errorPrefix === "linkclaw reply" ? "reply" : "message",
        body: options.body,
      },
    );
    rememberReplyContext(config, options.home, resolvedIdentifier);
    const envelope = await runLinkClaw(
      config,
      {
        command: "message_send",
        home: options.home,
        identifier: resolvedIdentifier,
        body: options.body,
      },
      pluginRoot,
    );
    return {
      type: "message",
      message: formatMessageSend(envelope.result),
    };
  } catch (error) {
    return {
      type: "message",
      message: formatCommandError(errorPrefix, error),
    };
  }
}

export async function runInboxCommand(
  config: LinkClawPluginConfig,
  rawArgs: string,
  pluginRoot: string,
): Promise<CommandResult> {
  let options: InboxOptions;
  try {
    options = parseInboxCommand(rawArgs);
  } catch (error) {
    return {
      type: "message",
      message: `linkclaw inbox command failed: ${(error as Error).message}`,
    };
  }
  try {
    const envelope = await runLinkClaw(
      config,
      {
        command: "message_inbox",
        home: options.home,
      },
      pluginRoot,
    );
    return {
      type: "message",
      message: formatInbox(envelope.result, options.query),
    };
  } catch (error) {
    return {
      type: "message",
      message: formatCommandError("linkclaw inbox", error),
    };
  }
}

export async function runSyncCommand(
  config: LinkClawPluginConfig,
  rawArgs: string,
  pluginRoot: string,
): Promise<CommandResult> {
  let home: string | undefined;
  try {
    home = parseOptionalHome(rawArgs, "sync");
  } catch (error) {
    return {
      type: "message",
      message: `linkclaw sync command failed: ${(error as Error).message}`,
    };
  }
  try {
    const envelope = await runLinkClaw(
      config,
      {
        command: "message_sync",
        home,
      },
      pluginRoot,
    );
    return {
      type: "message",
      message: formatSync(envelope.result),
    };
  } catch (error) {
    return {
      type: "message",
      message: formatCommandError("linkclaw sync", error),
    };
  }
}

export function parseSetupCommand(rawArgs: string): SetupOptions {
  const tokens = tokenizeCommand(rawArgs);
  const options: SetupOptions = {};

  for (let index = 0; index < tokens.length; index += 1) {
    const token = tokens[index];
    if (token === "--home") {
      if (index + 1 >= tokens.length) {
        throw new Error("missing value for --home");
      }
      options.home = tokens[index + 1];
      index += 1;
      continue;
    }
    if (token === "--canonical-id") {
      if (index + 1 >= tokens.length) {
        throw new Error("missing value for --canonical-id");
      }
      options.canonicalId = tokens[index + 1];
      index += 1;
      continue;
    }
    if (token === "--display-name") {
      if (index + 1 >= tokens.length) {
        throw new Error("missing value for --display-name");
      }
      options.displayName = tokens[index + 1];
      index += 1;
      continue;
    }
    if (token === "--check-only") {
      options.checkOnly = true;
      continue;
    }
    throw new Error(`unsupported setup argument: ${token}`);
  }

  return options;
}

export function parseStatusCommand(rawArgs: string): StatusOptions {
  return {
    home: parseOptionalHome(rawArgs, "status"),
  };
}

export function parseImportCommand(rawArgs: string): ImportOptions {
  const tokens = tokenizeCommand(rawArgs);
  let input = "";
  let allowDiscovered = false;
  let allowMismatch = false;
  let home: string | undefined;

  for (let index = 0; index < tokens.length; index += 1) {
    const token = tokens[index];
    if (token === "--allow-discovered") {
      allowDiscovered = true;
      continue;
    }
    if (token === "--allow-mismatch") {
      allowMismatch = true;
      continue;
    }
    if (token === "--home") {
      if (index + 1 >= tokens.length) {
        throw new Error("missing value for --home");
      }
      home = tokens[index + 1];
      index += 1;
      continue;
    }
    if (token.startsWith("--")) {
      throw new Error(`unsupported import argument: ${token}`);
    }
    if (input !== "") {
      throw new Error("linkclaw-import accepts exactly one input URL");
    }
    input = token;
  }

  return {
    input,
    allowDiscovered,
    allowMismatch,
    home,
  };
}

export function parseShareCommand(rawArgs: string): ShareOptions {
  const tokens = tokenizeCommand(rawArgs);
  const options: ShareOptions = {};

  for (let index = 0; index < tokens.length; index += 1) {
    const token = tokens[index];
    if (token === "--home") {
      if (index + 1 >= tokens.length) {
        throw new Error("missing value for --home");
      }
      options.home = tokens[index + 1];
      index += 1;
      continue;
    }
    if (token === "--card") {
      options.card = true;
      continue;
    }
    if (token === "--origin") {
      if (index + 1 >= tokens.length) {
        throw new Error("missing value for --origin");
      }
      options.origin = tokens[index + 1];
      index += 1;
      continue;
    }
    throw new Error(`unsupported share argument: ${token}`);
  }

  return options;
}

export function parseConnectCommand(rawArgs: string): ConnectOptions {
  const directPayload = parseDirectConnectPayload(rawArgs);
  if (directPayload) {
    return directPayload;
  }

  const tokens = tokenizeCommand(rawArgs);
  let input = "";
  let home: string | undefined;

  for (let index = 0; index < tokens.length; index += 1) {
    const token = tokens[index];
    if (token === "--home") {
      if (index + 1 >= tokens.length) {
        throw new Error("missing value for --home");
      }
      home = tokens[index + 1];
      index += 1;
      continue;
    }
    if (token.startsWith("--")) {
      throw new Error(`unsupported connect argument: ${token}`);
    }
    if (input !== "") {
      throw new Error("linkclaw-connect accepts exactly one identity card input");
    }
    input = token;
  }

  return { input, home };
}

function parseDirectConnectPayload(rawArgs: string): ConnectOptions | undefined {
  const trimmed = rawArgs.trim();
  if (trimmed === "") {
    return undefined;
  }

  const homeMatch = trimmed.match(/^--home\s+(\S+)\s+([\s\S]+)$/);
  if (homeMatch) {
    const payload = homeMatch[2]?.trim() ?? "";
    if (looksLikeInlineCardPayload(payload)) {
      return {
        home: homeMatch[1],
        input: payload,
      };
    }
  }

  if (looksLikeInlineCardPayload(trimmed)) {
    return { input: trimmed };
  }

  return undefined;
}

function looksLikeInlineCardPayload(value: string): boolean {
  return (
    (/^```(?:json)?[\s\S]*```$/i.test(value) ||
      (value.startsWith("{") && value.endsWith("}")))
  );
}

export function parseFindCommand(rawArgs: string): FindOptions {
  const tokens = tokenizeCommand(rawArgs);
  let home: string | undefined;
  const positional: string[] = [];

  for (let index = 0; index < tokens.length; index += 1) {
    const token = tokens[index];
    if (token === "--home") {
      if (index + 1 >= tokens.length) {
        throw new Error("missing value for --home");
      }
      home = tokens[index + 1];
      index += 1;
      continue;
    }
    if (token.startsWith("--")) {
      throw new Error(`unsupported find argument: ${token}`);
    }
    positional.push(token);
  }

  return {
    home,
    query: positional.join(" ").trim(),
  };
}

export function parseContactsCommand(rawArgs: string): ContactsOptions {
  const tokens = tokenizeCommand(rawArgs);
  let home: string | undefined;
  const positional: string[] = [];

  for (let index = 0; index < tokens.length; index += 1) {
    const token = tokens[index];
    if (token === "--home") {
      if (index + 1 >= tokens.length) {
        throw new Error("missing value for --home");
      }
      home = tokens[index + 1];
      index += 1;
      continue;
    }
    if (token.startsWith("--")) {
      throw new Error(`unsupported contacts argument: ${token}`);
    }
    positional.push(token);
  }

  return {
    home,
    query: positional.join(" ").trim() || undefined,
  };
}

export function parseMessageCommand(rawArgs: string): MessageOptions {
  const tokens = tokenizeCommand(rawArgs);
  let home: string | undefined;
  const positional: string[] = [];

  for (let index = 0; index < tokens.length; index += 1) {
    const token = tokens[index];
    if (token === "--home") {
      if (index + 1 >= tokens.length) {
        throw new Error("missing value for --home");
      }
      home = tokens[index + 1];
      index += 1;
      continue;
    }
    if (token.startsWith("--")) {
      throw new Error(`unsupported message argument: ${token}`);
    }
    positional.push(token);
  }

  return {
    home,
    identifier: positional[0] ?? "",
    body: positional.slice(1).join(" ").trim(),
  };
}

export function parseReplyCommand(rawArgs: string): MessageOptions {
  const parsed = parseMessageCommand(rawArgs);
  if (parsed.identifier !== "" && parsed.body !== "") {
    return parsed;
  }

  const tokens = tokenizeCommand(rawArgs);
  let home: string | undefined;
  const positional: string[] = [];

  for (let index = 0; index < tokens.length; index += 1) {
    const token = tokens[index];
    if (token === "--home") {
      if (index + 1 >= tokens.length) {
        throw new Error("missing value for --home");
      }
      home = tokens[index + 1];
      index += 1;
      continue;
    }
    if (token.startsWith("--")) {
      throw new Error(`unsupported reply argument: ${token}`);
    }
    positional.push(token);
  }

  if (positional.length === 0) {
    return { home, identifier: "", body: "" };
  }
  if (positional.length === 1) {
    return { home, identifier: "", body: positional[0] };
  }

  return {
    home,
    identifier: positional[0] ?? "",
    body: positional.slice(1).join(" ").trim(),
  };
}

export function parseThreadCommand(rawArgs: string): ThreadOptions {
  const tokens = tokenizeCommand(rawArgs);
  let home: string | undefined;
  let limit: number | undefined;
  const positional: string[] = [];

  for (let index = 0; index < tokens.length; index += 1) {
    const token = tokens[index];
    if (token === "--home") {
      if (index + 1 >= tokens.length) {
        throw new Error("missing value for --home");
      }
      home = tokens[index + 1];
      index += 1;
      continue;
    }
    if (token === "--limit") {
      if (index + 1 >= tokens.length) {
        throw new Error("missing value for --limit");
      }
      const parsed = Number.parseInt(tokens[index + 1] ?? "", 10);
      if (!Number.isFinite(parsed) || parsed <= 0) {
        throw new Error("invalid value for --limit");
      }
      limit = parsed;
      index += 1;
      continue;
    }
    if (token.startsWith("--")) {
      throw new Error(`unsupported thread argument: ${token}`);
    }
    positional.push(token);
  }

  return {
    home,
    limit,
    identifier: positional[0] ?? "",
  };
}

export function parseInboxCommand(rawArgs: string): InboxOptions {
  const tokens = tokenizeCommand(rawArgs);
  let home: string | undefined;
  const positional: string[] = [];

  for (let index = 0; index < tokens.length; index += 1) {
    const token = tokens[index];
    if (token === "--home") {
      if (index + 1 >= tokens.length) {
        throw new Error("missing value for --home");
      }
      home = tokens[index + 1];
      index += 1;
      continue;
    }
    if (token.startsWith("--")) {
      throw new Error(`unsupported inbox argument: ${token}`);
    }
    positional.push(token);
  }

  return {
    home,
    query: positional.join(" ").trim() || undefined,
  };
}

function formatImportMessage(value: unknown): string {
  if (typeof value !== "object" || value === null) {
    return "Identity imported.\nNext:\n- run /linkclaw-contacts or /linkclaw-inbox to review it.";
  }

  const record = value as Record<string, unknown>;
  const inspection = toInspectResult(record.inspection);
  const lines = ["Identity imported into your local trust book."];

  if (typeof record.contact_id === "string" && record.contact_id.trim() !== "") {
    lines.push(`contact: ${record.contact_id}`);
  }
  if (inspection.display_name) {
    lines.push(`name: ${inspection.display_name}`);
  }
  if (inspection.canonical_id) {
    lines.push(`canonical id: ${inspection.canonical_id}`);
  }
  if (inspection.verification_state) {
    lines.push(`status: ${inspection.verification_state}`);
  }
  if (typeof record.created === "boolean") {
    lines.push(`created: ${record.created ? "yes" : "no"}`);
  }
  if (typeof record.handle_count === "number") {
    lines.push(`handles: ${record.handle_count}`);
  }
  if (typeof record.snapshot_count === "number") {
    lines.push(`artifacts: ${record.snapshot_count}`);
  }
  if (typeof record.proof_count === "number") {
    lines.push(`proofs: ${record.proof_count}`);
  }
  lines.push("Next:");
  if (typeof record.contact_id === "string" && record.contact_id.trim() !== "") {
    lines.push(`- review the imported contact: ${record.contact_id}`);
  }
  lines.push("- if you want to message them, exchange identity cards and use /linkclaw-message <contact> <text>");

  return lines.join("\n");
}

function formatConnectMessage(value: unknown): string {
  const record = asObject(value);
  const card = asObject(record?.card);
  const displayName =
    typeof card?.display_name === "string" && card.display_name.trim() !== ""
      ? card.display_name
      : undefined;
  const canonicalId =
    typeof card?.id === "string" && card.id.trim() !== "" ? card.id : undefined;
  const lines = ["LinkClaw contact saved."];
  if (typeof record?.contact_id === "string" && record.contact_id.trim() !== "") {
    lines.push(`contact: ${record.contact_id}`);
  }
  if (displayName) {
    lines.push(`name: ${displayName}`);
  }
  if (canonicalId) {
    lines.push(`canonical id: ${canonicalId}`);
  }
  if (typeof record?.created === "boolean") {
    lines.push(`created: ${record.created ? "yes" : "no"}`);
  }
  lines.push(
    ...formatMarkedSection("contact-summary", [
      ...(typeof record?.contact_id === "string" && record.contact_id.trim() !== ""
        ? [`contact: ${record.contact_id}`]
        : []),
      ...(displayName ? [`name: ${displayName}`] : []),
      ...(canonicalId ? [`canonical id: ${canonicalId}`] : []),
      ...(typeof record?.created === "boolean" ? [`created: ${record.created ? "yes" : "no"}`] : []),
    ]),
  );
  lines.push("Next:");
  const target = canonicalId ?? displayName ?? "<contact>";
  lines.push(`- send a message with /linkclaw-message ${target} <text>`);
  lines.push(`- open the conversation with /linkclaw-thread ${target}`);
  lines.push("- if they do not have your card yet, share back with /linkclaw-share --card");
  lines.push("- or run /linkclaw-inbox to review existing conversations");
  return lines.join("\n");
}

function formatMessageSend(value: unknown): string {
  const record = asObject(value);
  const message = asObject(record?.message);
  const conversation = asObject(record?.conversation);
  const status = typeof message?.status === "string" ? message.status : "";
  const transportStatus = typeof message?.transport_status === "string" ? message.transport_status : "";
  const headline =
    status === "failed"
      ? "LinkClaw message failed."
      : transportStatus === "direct" || status === "delivered"
        ? "LinkClaw message delivered."
        : "LinkClaw message queued.";
  const lines = [headline];
  if (typeof conversation?.conversation_id === "string") {
    lines.push(`conversation: ${conversation.conversation_id}`);
  }
  if (typeof message?.message_id === "string") {
    lines.push(`message: ${message.message_id}`);
  }
  if (status) {
    lines.push(`status: ${status}`);
    if (status === "failed") {
      lines.push("delivery: failed before runtime handoff");
    }
  }
  if (transportStatus) {
    lines.push(`transport status: ${transportStatus}`);
  }
  if (typeof message?.preview === "string") {
    lines.push(`preview: ${message.preview}`);
  }
  lines.push("Next:");
  if (status === "failed") {
    lines.push("- run /linkclaw-setup to verify your local identity and messaging setup");
    lines.push("- confirm the contact was imported from a full identity card, not a partial profile");
  } else if (status === "delivered") {
    lines.push("- the recipient can open /linkclaw-thread <contact> to refresh the conversation");
    lines.push("- if the UI has not refreshed yet, they can run /linkclaw-sync once");
  } else {
    lines.push("- the recipient needs to run /linkclaw-sync to receive it");
  }
  lines.push("- use /linkclaw-thread <contact> to review this conversation");
  return lines.join("\n");
}

function formatContacts(value: unknown, query?: string): string {
  const record = asObject(value);
  const rawContacts = Array.isArray(record?.contacts) ? record.contacts : [];
  const contacts = filterRawContactsByQuery(rawContacts, query);
  const lines = ["LinkClaw contacts", `contacts: ${contacts.length}`];
  if (query && query.trim() !== "") {
    lines.push(`query: ${query}`);
  }
  for (const contact of contacts) {
    const item = asObject(contact);
    const name =
      typeof item?.display_name === "string" && item.display_name.trim() !== ""
        ? item.display_name
        : "(unknown)";
    const canonicalID =
      typeof item?.canonical_id === "string" ? item.canonical_id : "";
    const trust = asObject(item?.trust);
    const trustLevel =
      typeof trust?.trust_level === "string" && trust.trust_level.trim() !== ""
        ? trust.trust_level
        : "unknown";
    const verification =
      typeof trust?.verification_state === "string" ? trust.verification_state : "";
    const notes =
      typeof item?.note_count === "number" ? item.note_count : 0;

    const summary = [name];
    if (canonicalID !== "") {
      summary.push(canonicalID);
    }
    summary.push(`trust=${trustLevel}`);
    if (verification !== "") {
      summary.push(`verification=${verification}`);
    }
    summary.push(`notes=${notes}`);
    lines.push(`- ${summary.join(" | ")}`);
  }
  lines.push(
    ...formatMarkedSection(
      "contacts-list",
      contacts.map((contact) => {
        const item = asObject(contact);
        const name =
          typeof item?.display_name === "string" && item.display_name.trim() !== ""
            ? item.display_name
            : "(unknown)";
        const canonicalID =
          typeof item?.canonical_id === "string" && item.canonical_id.trim() !== ""
            ? item.canonical_id
            : "";
        const trust = asObject(item?.trust);
        const trustLevel =
          typeof trust?.trust_level === "string" && trust.trust_level.trim() !== ""
            ? trust.trust_level
            : "unknown";
        const verification =
          typeof trust?.verification_state === "string" && trust.verification_state.trim() !== ""
            ? trust.verification_state
            : "";
        const notes = typeof item?.note_count === "number" ? item.note_count : 0;
        return [
          `name: ${name}`,
          ...(canonicalID !== "" ? [`canonical id: ${canonicalID}`] : []),
          `trust: ${trustLevel}`,
          ...(verification !== "" ? [`verification: ${verification}`] : []),
          `notes: ${notes}`,
        ].join(" | ");
      }),
    ),
  );
  if (contacts.length > 0) {
    lines.push("Next:");
    lines.push("- message someone with /linkclaw-message <contact> <text>");
    lines.push("- inspect new activity with /linkclaw-inbox");
  }
  return lines.join("\n");
}

function formatFindResults(query: string, contacts: KnownContact[]): string {
  const lines = ["LinkClaw contact search", `query: ${query}`, `matches: ${contacts.length}`];
  for (const contact of contacts) {
    const summary = [
      contact.displayName ?? "(unknown)",
      contact.canonicalId ?? contact.contactId ?? "",
    ].filter((item) => item !== "");
    lines.push(`- ${summary.join(" | ")}`);
    if (contact.canonicalId ?? contact.contactId) {
      const identifier = contact.canonicalId ?? contact.contactId ?? "";
      lines.push(`  message: /linkclaw-message ${identifier} <text>`);
      lines.push(`  thread: /linkclaw-thread ${identifier}`);
    }
  }
  if (contacts.length === 0) {
    lines.push("No saved contacts matched that query.");
  }
  return lines.join("\n");
}

function formatShareCardMessage(value: unknown): string {
  const record = asObject(value);
  const card = asObject(record?.card);
  const displayName =
    typeof card?.display_name === "string" && card.display_name.trim() !== ""
      ? card.display_name
      : "(unknown)";
  const canonicalId =
    typeof card?.id === "string" && card.id.trim() !== "" ? card.id : "";
  const compactCardJSON = JSON.stringify(card ?? value);
  const lines = ["LinkClaw identity card ready to share."];
  lines.push(`name: ${displayName}`);
  if (canonicalId !== "") {
    lines.push(`canonical id: ${canonicalId}`);
  }
  lines.push("card compact:");
  lines.push("--- card-compact-begin ---");
  lines.push(compactCardJSON);
  lines.push("--- card-compact-end ---");
  lines.push("card:");
  lines.push(JSON.stringify(card ?? value, null, 2));
  lines.push("Next:");
  lines.push("- send this JSON directly to the other side");
  lines.push("- if they use this plugin, they can run /linkclaw-connect <card-json>");
  lines.push("--- connect-command-begin ---");
  lines.push(`/linkclaw-connect '${escapeSingleQuotes(compactCardJSON)}'`);
  lines.push("--- connect-command-end ---");
  return lines.join("\n");
}

function formatInbox(value: unknown, query?: string): string {
  const record = asObject(value);
  const rawConversations = Array.isArray(record?.conversations) ? record.conversations : [];
  const conversations = filterInboxConversations(rawConversations, query);
  const lines = ["LinkClaw inbox", `conversations: ${conversations.length}`];
  if (query && query.trim() !== "") {
    lines.push(`query: ${query}`);
  }
  let hasUnknownSender = false;
  for (const conversation of conversations) {
    const item = asObject(conversation);
    const name = typeof item?.contact_display_name === "string" ? item.contact_display_name : "(unknown)";
    const canonicalID = typeof item?.contact_canonical_id === "string" ? item.contact_canonical_id : "";
    const status = typeof item?.contact_status === "string" ? item.contact_status : "";
    const unread = typeof item?.unread_count === "number" ? item.unread_count : 0;
    const preview = typeof item?.last_message_preview === "string" ? item.last_message_preview : "";
    const senderLabel = status === "discovered" ? "new sender" : status || "known";
    if (status === "discovered") {
      hasUnknownSender = true;
    }
    const summary = [name];
    if (canonicalID !== "") {
      summary.push(canonicalID);
    }
    summary.push(senderLabel);
    summary.push(`unread=${unread}`);
    if (preview !== "") {
      summary.push(`last=${preview}`);
    }
    lines.push(`- ${summary.join(" | ")}`);
  }
  lines.push(
    ...formatMarkedSection(
      "inbox-conversations",
      conversations.map((conversation) => {
        const item = asObject(conversation);
        const name =
          typeof item?.contact_display_name === "string" && item.contact_display_name.trim() !== ""
            ? item.contact_display_name
            : "(unknown)";
        const canonicalID =
          typeof item?.contact_canonical_id === "string" && item.contact_canonical_id.trim() !== ""
            ? item.contact_canonical_id
            : "";
        const status =
          typeof item?.contact_status === "string" && item.contact_status.trim() !== ""
            ? item.contact_status
            : "known";
        const unread = typeof item?.unread_count === "number" ? item.unread_count : 0;
        const preview =
          typeof item?.last_message_preview === "string" && item.last_message_preview.trim() !== ""
            ? item.last_message_preview
            : "";
        return [
          `name: ${name}`,
          ...(canonicalID !== "" ? [`canonical id: ${canonicalID}`] : []),
          `status: ${status}`,
          `unread: ${unread}`,
          ...(preview !== "" ? [`last: ${preview}`] : []),
        ].join(" | ");
      }),
    ),
  );
  if (hasUnknownSender) {
    lines.push("Next:");
    lines.push("- ask the sender for an identity card");
    lines.push("- then run /linkclaw-connect <card> if you want to keep them");
  } else if (conversations.length > 0) {
    lines.push("Next:");
    lines.push("- open one conversation with /linkclaw-thread <contact>");
    lines.push("- reply with /linkclaw-reply <contact> <text>");
  }
  return lines.join("\n");
}

function formatThread(value: unknown): string {
  const record = asObject(value);
  const conversation = asObject(record?.conversation);
  const name =
    typeof conversation?.contact_display_name === "string" && conversation.contact_display_name.trim() !== ""
      ? conversation.contact_display_name
      : "(unknown)";
  const canonicalID =
    typeof conversation?.contact_canonical_id === "string" ? conversation.contact_canonical_id : "";
  const messages = Array.isArray(conversation?.messages) ? conversation.messages : [];
  const lines = ["LinkClaw thread", `contact: ${name}`];
  if (canonicalID !== "") {
    lines.push(`canonical id: ${canonicalID}`);
  }
  lines.push(`messages: ${messages.length}`);
  for (const message of messages) {
    const item = asObject(message);
    const direction = typeof item?.direction === "string" ? item.direction : "message";
    const body = typeof item?.body === "string" ? item.body : "";
    const createdAt = typeof item?.created_at === "string" ? item.created_at : "";
    const status = typeof item?.status === "string" ? item.status : "";
    const transportStatus = typeof item?.transport_status === "string" ? item.transport_status : "";
    const summary = [`[${direction}]`];
    if (createdAt !== "") {
      summary.push(createdAt);
    }
    if (status !== "") {
      summary.push(status);
    }
    if (transportStatus !== "") {
      summary.push(`transport=${transportStatus}`);
    }
    lines.push(`- ${summary.join(" | ")} | ${body}`);
  }
  lines.push(
    ...formatMarkedSection(
      "thread-messages",
      messages.map((message) => {
        const item = asObject(message);
        const direction = typeof item?.direction === "string" ? item.direction : "message";
        const body = typeof item?.body === "string" ? item.body : "";
        const createdAt = typeof item?.created_at === "string" ? item.created_at : "";
        const status = typeof item?.status === "string" ? item.status : "";
        const transportStatus = typeof item?.transport_status === "string" ? item.transport_status : "";
        return [
          `direction: ${direction}`,
          ...(createdAt !== "" ? [`created_at: ${createdAt}`] : []),
          ...(status !== "" ? [`status: ${status}`] : []),
          ...(transportStatus !== "" ? [`transport_status: ${transportStatus}`] : []),
          `body: ${body}`,
        ].join(" | ");
      }),
    ),
  );
  if (messages.length === 0) {
    lines.push("No messages in this thread yet.");
  }
  const replyTarget = canonicalID !== "" ? canonicalID : name;
  if (replyTarget !== "(unknown)") {
    lines.push("Next:");
    lines.push(`- reply with /linkclaw-reply ${replyTarget} <text>`);
  }
  return lines.join("\n");
}

function formatSync(value: unknown): string {
  const record = asObject(value);
  const synced = typeof record?.synced === "number" ? record.synced : 0;
  const recoveryChecks = typeof record?.relay_calls === "number" ? record.relay_calls : 0;
  const lines = ["LinkClaw sync completed", `synced: ${synced}`, `recovery checks: ${recoveryChecks}`];
  lines.push("Next:");
  if (synced > 0) {
    lines.push("- run /linkclaw-inbox to read new messages");
  } else {
    lines.push("- no new messages yet; run /linkclaw-message <contact> <text> to start a conversation");
  }
  return lines.join("\n");
}

function formatStatus(
  home: string | undefined,
  config: LinkClawPluginConfig,
  health: SetupHealth,
  statusValue: unknown,
): string {
  const record = asObject(statusValue);
  const identity = readString(record?.display_name) ?? "(not initialized)";
  const selfId = readString(record?.self_id);
  const peerId = readString(record?.peer_id);
  const contacts = typeof record?.contacts === "number" ? record.contacts : 0;
  const conversations = typeof record?.conversations === "number" ? record.conversations : 0;
  const unread = typeof record?.unread === "number" ? record.unread : 0;
  const pendingOutbox = typeof record?.pending_outbox === "number" ? record.pending_outbox : 0;
  const identityReady = typeof record?.identity_ready === "boolean" ? record.identity_ready : false;
  const transportReady = typeof record?.transport_ready === "boolean" ? record.transport_ready : false;
  const discoveryReady = typeof record?.discovery_ready === "boolean" ? record.discovery_ready : false;
  const messageDirect = typeof record?.message_status_direct === "number" ? record.message_status_direct : 0;
  const messageDeferred = typeof record?.message_status_deferred === "number" ? record.message_status_deferred : 0;
  const messageRecovered = typeof record?.message_status_recovered === "number" ? record.message_status_recovered : 0;
  const presenceEntries = typeof record?.presence_entries === "number" ? record.presence_entries : 0;
  const reachablePresence = typeof record?.reachable_presence === "number" ? record.reachable_presence : 0;
  const storeForwardRoutes = typeof record?.store_forward_routes === "number" ? record.store_forward_routes : 0;
  const directEnabled = typeof record?.direct_enabled === "boolean" ? record.direct_enabled : false;
  const backgroundRuntime = typeof record?.background_runtime === "boolean" ? record.background_runtime : false;
  const runtimeMode = readString(record?.runtime_mode) ?? "host-managed";
  const lastStoreForwardSyncAt = readString(record?.last_store_forward_sync_at);
  const lastStoreForwardResult = readString(record?.last_store_forward_result);
  const lastRecoveredCount = typeof record?.last_recovered_count === "number" ? record.last_recovered_count : 0;
  const lastAnnounceAt = readString(record?.last_announce_at);
  const recentRouteOutcomes = Array.isArray(record?.recent_route_outcomes) ? record.recent_route_outcomes : [];
  const messagingReady = selfId !== undefined && selfId !== "";
  const recoveryStatus =
    storeForwardRoutes > 0 ? `ready (${storeForwardRoutes} path${storeForwardRoutes === 1 ? "" : "s"})` : "not configured";
  const directStatus = directEnabled ? "experimental on" : "experimental off";
  const routeOutcomeLines = recentRouteOutcomes
    .map((value) => asObject(value))
    .filter((value): value is Record<string, unknown> => value !== undefined)
    .map((item) => {
      const routeType = readString(item.route_type) ?? "unknown";
      const outcome = readString(item.outcome) ?? "unknown";
      const attemptedAt = readString(item.attempted_at);
      const cursor = readString(item.cursor);
      const error = readString(item.error);
      const parts = [humanRouteOutcomeLabel(routeType), outcome];
      if (attemptedAt) {
        parts.push(attemptedAt);
      }
      if (cursor) {
        parts.push(`cursor=${cursor}`);
      }
      if (error) {
        parts.push(`error=${error}`);
      }
      return parts.join(" | ");
    });
  const summaryLines = [
    `identity: ${identity}`,
    ...(selfId ? [`self id: ${selfId}`] : []),
    ...(peerId ? [`peer id: ${peerId}`] : []),
    `identity ready: ${identityReady ? "yes" : "no"}`,
    `transport ready: ${transportReady ? "yes" : "no"}`,
    `discovery ready: ${discoveryReady ? "yes" : "no"}`,
    `messaging: ${messagingReady ? "ready" : "not ready"}`,
    `contacts: ${contacts}`,
    `conversations: ${conversations}`,
    `unread: ${unread}`,
    `queued outgoing: ${pendingOutbox}`,
    `message status: direct=${messageDirect} deferred=${messageDeferred} recovered=${messageRecovered}`,
    `offline recovery: ${recoveryStatus}`,
    `presence cache: ${presenceEntries} (${reachablePresence} reachable)`,
    `runtime mode: ${runtimeMode}${backgroundRuntime ? " (experimental)" : ""}`,
    `direct transport: ${directStatus}`,
    ...(lastAnnounceAt ? [`last announce: ${lastAnnounceAt}`] : []),
    ...(lastStoreForwardSyncAt
      ? [
          `last recovery: ${lastStoreForwardSyncAt}${lastStoreForwardResult ? ` | result=${lastStoreForwardResult}` : ""} | recovered=${lastRecoveredCount}`,
        ]
      : []),
    ...(routeOutcomeLines.length > 0 ? ["recent route outcomes:"] : []),
    ...routeOutcomeLines.map((line) => `- ${line}`),
  ];

  const lines = [
    "LinkClaw status",
    `home: ${resolveLinkClawHome(home, config)}`,
    "state: ready",
    ...summaryLines,
    "--- status-summary-begin ---",
    ...summaryLines,
    "--- status-summary-end ---",
    ...formatHealthSection(health),
    "Next:",
    "- run /linkclaw-contacts or /linkclaw-find <query> to inspect contacts",
    "- run /linkclaw-inbox to inspect recent conversations",
    "- run /linkclaw-sync to recover new messages",
  ];

  return lines.join("\n");
}

function humanRouteOutcomeLabel(routeType: string): string {
  const normalized = routeType.trim().toLowerCase();
  if (normalized.includes("store_forward") || normalized.includes("store-forward")) {
    return "offline recovery";
  }
  if (normalized.includes("direct")) {
    return "direct transport";
  }
  return "delivery path";
}

function parseOptionalHome(rawArgs: string, commandName: string): string | undefined {
  const tokens = tokenizeCommand(rawArgs);
  let home: string | undefined;
  for (let index = 0; index < tokens.length; index += 1) {
    const token = tokens[index];
    if (token === "--home") {
      if (index + 1 >= tokens.length) {
        throw new Error(`missing value for --home`);
      }
      home = tokens[index + 1];
      index += 1;
      continue;
    }
    throw new Error(`unsupported ${commandName} argument: ${token}`);
  }
  return home;
}

function asObject(value: unknown): Record<string, unknown> | undefined {
  return typeof value === "object" && value !== null ? (value as Record<string, unknown>) : undefined;
}

async function resolveContactIdentifier(
  config: LinkClawPluginConfig,
  home: string | undefined,
  identifier: string,
  pluginRoot: string,
  context: ContactResolutionContext,
): Promise<string> {
  const trimmed = identifier.trim();
  if (trimmed === "" || looksLikeStableContactReference(trimmed)) {
    return trimmed;
  }

  const contacts = await loadKnownContacts(config, home, pluginRoot);
  if (contacts.length === 0) {
    return trimmed;
  }

  const exactMatches = collectContactMatches(contacts, trimmed, true);
  if (exactMatches.length === 1) {
    return exactMatches[0].canonicalId ?? exactMatches[0].contactId ?? trimmed;
  }
  if (exactMatches.length > 1) {
    throw new Error(formatAmbiguousContactMessage(trimmed, exactMatches, context));
  }

  const fuzzyMatches = collectContactMatches(contacts, trimmed, false);
  if (fuzzyMatches.length === 1) {
    return fuzzyMatches[0].canonicalId ?? fuzzyMatches[0].contactId ?? trimmed;
  }
  if (fuzzyMatches.length > 1) {
    throw new Error(formatAmbiguousContactMessage(trimmed, fuzzyMatches, context));
  }

  return trimmed;
}

async function resolveReplyOptions(
  config: LinkClawPluginConfig,
  options: MessageOptions,
  pluginRoot: string,
): Promise<MessageOptions> {
  const rememberedIdentifier = readReplyContext(config, options.home);
  if (rememberedIdentifier) {
    return {
      ...options,
      identifier: rememberedIdentifier,
    };
  }

  let nextOptions = options;
  if (options.identifier.trim() !== "") {
    if (looksLikeStableContactReference(options.identifier)) {
      return options;
    }
    const contacts = await loadKnownContacts(config, options.home, pluginRoot);
    const exactMatches = collectContactMatches(contacts, options.identifier, true);
    const fuzzyMatches =
      exactMatches.length === 0
        ? collectContactMatches(contacts, options.identifier, false)
        : exactMatches;
    if (fuzzyMatches.length > 0) {
      return options;
    }

    nextOptions = {
      ...options,
      identifier: "",
      body: [options.identifier, options.body].filter((item) => item.trim() !== "").join(" ").trim(),
    };
  }

  const conversation = await loadMostRecentReplyableConversation(config, nextOptions.home, pluginRoot);
  const identifier = conversation.canonicalId ?? conversation.displayName ?? "";
  if (identifier === "") {
    throw new Error("no recent known conversation is available to reply to");
  }

  return {
    ...nextOptions,
    identifier,
  };
}

async function loadKnownContacts(
  config: LinkClawPluginConfig,
  home: string | undefined,
  pluginRoot: string,
): Promise<KnownContact[]> {
  const envelope = await runLinkClaw(
    config,
    {
      command: "known_ls",
      home,
    },
    pluginRoot,
  );
  const result = asObject(envelope.result);
  const items = Array.isArray(result?.contacts) ? result.contacts : [];
  return items
    .map((item) => {
      const record = asObject(item);
      return {
        contactId:
          typeof record?.contact_id === "string" && record.contact_id.trim() !== ""
            ? record.contact_id
            : undefined,
        canonicalId:
          typeof record?.canonical_id === "string" && record.canonical_id.trim() !== ""
            ? record.canonical_id
            : undefined,
        displayName:
          typeof record?.display_name === "string" && record.display_name.trim() !== ""
            ? record.display_name
            : undefined,
      } satisfies KnownContact;
    })
    .filter((item) => item.contactId || item.canonicalId || item.displayName);
}

function filterContactsByQuery(contacts: KnownContact[], query: string): KnownContact[] {
  const needle = normalizeIdentifier(query);
  return contacts.filter((contact) => {
    const fields = [contact.contactId, contact.canonicalId, contact.displayName].filter(
      (value): value is string => typeof value === "string" && value.trim() !== "",
    );
    return fields.some((field) => normalizeIdentifier(field).includes(needle));
  });
}

function filterRawContactsByQuery(contacts: unknown[], query: string | undefined): unknown[] {
  const trimmed = query?.trim() ?? "";
  if (trimmed === "") {
    return contacts;
  }

  const needle = normalizeIdentifier(trimmed);
  return contacts.filter((contact) => {
    const item = asObject(contact);
    const trust = asObject(item?.trust);
    const fields = [
      typeof item?.display_name === "string" ? item.display_name : "",
      typeof item?.canonical_id === "string" ? item.canonical_id : "",
      typeof trust?.trust_level === "string" ? trust.trust_level : "",
      typeof trust?.verification_state === "string" ? trust.verification_state : "",
    ];
    return fields.some((field) => normalizeIdentifier(field).includes(needle));
  });
}

function filterInboxConversations(conversations: unknown[], query: string | undefined): unknown[] {
  const trimmed = query?.trim() ?? "";
  if (trimmed === "") {
    return conversations;
  }

  const needle = normalizeIdentifier(trimmed);
  return conversations.filter((conversation) => {
    const item = asObject(conversation);
    const fields = [
      typeof item?.contact_display_name === "string" ? item.contact_display_name : "",
      typeof item?.contact_canonical_id === "string" ? item.contact_canonical_id : "",
      typeof item?.last_message_preview === "string" ? item.last_message_preview : "",
    ];
    return fields.some((field) => normalizeIdentifier(field).includes(needle));
  });
}

async function loadMostRecentReplyableConversation(
  config: LinkClawPluginConfig,
  home: string | undefined,
  pluginRoot: string,
): Promise<InboxConversation> {
  const envelope = await runLinkClaw(
    config,
    {
      command: "message_inbox",
      home,
    },
    pluginRoot,
  );
  const result = asObject(envelope.result);
  const items = Array.isArray(result?.conversations) ? result.conversations : [];
  const conversations = items
    .map((item) => {
      const record = asObject(item);
      return {
        canonicalId:
          typeof record?.contact_canonical_id === "string" && record.contact_canonical_id.trim() !== ""
            ? record.contact_canonical_id
            : undefined,
        displayName:
          typeof record?.contact_display_name === "string" && record.contact_display_name.trim() !== ""
            ? record.contact_display_name
            : undefined,
        status:
          typeof record?.contact_status === "string" && record.contact_status.trim() !== ""
            ? record.contact_status
            : undefined,
        lastMessageAt:
          typeof record?.last_message_at === "string" && record.last_message_at.trim() !== ""
            ? record.last_message_at
            : undefined,
      } satisfies InboxConversation;
    })
    .filter((item) => item.status !== "discovered");

  if (conversations.length === 0) {
    throw new Error("no recent known conversation is available; run /linkclaw-inbox or specify a contact");
  }

  conversations.sort((left, right) =>
    (right.lastMessageAt ?? "").localeCompare(left.lastMessageAt ?? ""),
  );
  return conversations[0];
}

function looksLikeStableContactReference(identifier: string): boolean {
  return identifier.startsWith("did:") || identifier.startsWith("contact_");
}

function collectContactMatches(
  contacts: KnownContact[],
  identifier: string,
  exactOnly: boolean,
): KnownContact[] {
  const needle = normalizeIdentifier(identifier);
  const seen = new Set<string>();
  const matches: KnownContact[] = [];

  for (const contact of contacts) {
    const fields = [contact.contactId, contact.canonicalId, contact.displayName].filter(
      (value): value is string => typeof value === "string" && value.trim() !== "",
    );
    const matched = fields.some((field) => {
      const candidate = normalizeIdentifier(field);
      if (exactOnly) {
        return candidate === needle;
      }
      return candidate.includes(needle);
    });
    if (!matched) {
      continue;
    }

    const dedupeKey = contact.canonicalId ?? contact.contactId ?? contact.displayName ?? "";
    if (dedupeKey === "" || seen.has(dedupeKey)) {
      continue;
    }
    seen.add(dedupeKey);
    matches.push(contact);
  }

  return matches;
}

function normalizeIdentifier(value: string): string {
  return value.trim().toLowerCase();
}

function formatAmbiguousContactMessage(
  identifier: string,
  matches: KnownContact[],
  context: ContactResolutionContext,
): string {
  const lines = [`contact "${identifier}" matched multiple saved contacts:`];
  for (const match of matches) {
    const summary = [
      match.displayName ?? "(unknown)",
      match.canonicalId ?? match.contactId ?? "",
    ].filter((item) => item !== "");
    lines.push(`- ${summary.join(" | ")}`);
  }
  lines.push("Try one of these:");
  for (const match of matches) {
    const target = match.canonicalId ?? match.contactId;
    if (!target) {
      continue;
    }
    lines.push(`- ${buildSuggestedCommand(context, target)}`);
  }
  return lines.join("\n");
}

function buildSuggestedCommand(context: ContactResolutionContext, identifier: string): string {
  switch (context.command) {
    case "message":
      return `/linkclaw-message ${identifier} ${context.body?.trim() || "<text>"}`;
    case "reply":
      return `/linkclaw-reply ${identifier} ${context.body?.trim() || "<text>"}`;
    case "thread":
      return `/linkclaw-thread ${identifier}`;
    default:
      return identifier;
  }
}

function rememberReplyContext(
  config: LinkClawPluginConfig,
  home: string | undefined,
  identifier: string,
): void {
  const trimmed = identifier.trim();
  if (trimmed === "") {
    return;
  }
  replyContextByHome.set(resolveLinkClawHome(home, config), trimmed);
}

function readReplyContext(
  config: LinkClawPluginConfig,
  home: string | undefined,
): string | undefined {
  return replyContextByHome.get(resolveLinkClawHome(home, config));
}

async function collectSetupHealth(
  config: LinkClawPluginConfig,
  home: string | undefined,
  pluginRoot: string,
): Promise<SetupHealth> {
  const binaryPath = await resolveLinkClawBinary(config, pluginRoot);
  const relayStatus = await checkRelayStatus(resolveRelayUrl(config));
  const publishStatus = await checkPublishOriginStatus(config, pluginRoot);
  return {
    binaryPath,
    relayStatus,
    publishStatus,
  };
}

async function checkRelayStatus(relayUrl: string | undefined): Promise<string> {
  const trimmed = relayUrl?.trim() ?? "";
  if (trimmed === "") {
    return "not configured";
  }

  try {
    const controller = new AbortController();
    const timeout = setTimeout(() => controller.abort(), 2000);
    try {
      const response = await fetch(trimmed, {
        method: "GET",
        redirect: "follow",
        signal: controller.signal,
      });
      return `ok (${response.status}) ${trimmed}`;
    } finally {
      clearTimeout(timeout);
    }
  } catch (error) {
    return `unreachable (${formatRelayError(error)}) ${trimmed}`;
  }
}

function formatRelayError(error: unknown): string {
  if (error instanceof Error && error.message.trim() !== "") {
    return error.message.trim();
  }
  return "request failed";
}

function formatMarkedSection(label: string, lines: string[]): string[] {
  return [
    `--- ${label}-begin ---`,
    ...(lines.length > 0 ? lines : ["(empty)"]),
    `--- ${label}-end ---`,
  ];
}

function formatHealthSection(health: SetupHealth): string[] {
  return [
    "检查项：",
    "--- health-checks-begin ---",
    `- binary: 正常 (${health.binaryPath})`,
    `- relay: ${health.relayStatus}`,
    `- publish origin: ${health.publishStatus}`,
    "--- health-checks-end ---",
  ];
}

async function checkPublishOriginStatus(
  config: LinkClawPluginConfig,
  pluginRoot: string,
): Promise<string> {
  const origin = config.publishOrigin?.trim() ?? "";
  if (origin === "") {
    return "not configured";
  }

  try {
    const envelope = await runLinkClaw(
      config,
      {
        command: "inspect",
        input: origin,
      },
      pluginRoot,
    );
    const inspection = toInspectResult(envelope.result);
    if (hasShareableArtifacts(inspection)) {
      return `ok ${origin}`;
    }
    return `configured but bundle missing ${origin}`;
  } catch (error) {
    return `unreachable (${formatRelayError(error)}) ${origin}`;
  }
}

function escapeSingleQuotes(value: string): string {
  return value.replace(/'/g, "'\"'\"'");
}

async function normalizeConnectInput(input: string): Promise<string> {
  const trimmed = input.trim();
  if (trimmed === "") {
    return trimmed;
  }

  const parsedInline = parseIdentityCardEnvelopeCandidate(trimmed);
  if (parsedInline) {
    return JSON.stringify(parsedInline);
  }

  if (/^https?:\/\//.test(trimmed)) {
    return trimmed;
  }

  try {
    const content = await readFile(trimmed, "utf8");
    const parsedFile = parseIdentityCardEnvelopeCandidate(content);
    if (parsedFile) {
      return JSON.stringify(parsedFile);
    }
  } catch {
    return trimmed;
  }

  return trimmed;
}

function parseIdentityCardEnvelopeCandidate(raw: string): unknown | undefined {
  for (const candidate of extractJSONCandidates(raw)) {
    const parsed = parseIdentityCardEnvelope(candidate);
    if (parsed) {
      return parsed;
    }
  }
  return undefined;
}

function parseIdentityCardEnvelope(raw: string): unknown | undefined {
  try {
    const parsed = JSON.parse(raw);
    const record = asObject(parsed);
    const result = asObject(record?.result);
    const card = result && "card" in result ? result.card : undefined;
    const cardRecord = asObject(card);
    if (cardRecord && typeof cardRecord.signature === "string" && cardRecord.signature.trim() !== "") {
      return card;
    }
    if (record && typeof record.signature === "string" && record.signature.trim() !== "") {
      return parsed;
    }
  } catch {
    return undefined;
  }
  return undefined;
}

function extractJSONCandidates(raw: string): string[] {
  const candidates = new Set<string>();
  const trimmed = raw.trim();
  if (trimmed !== "") {
    candidates.add(trimmed);
  }

  const fencedMatch = trimmed.match(/^```(?:json)?\s*([\s\S]*?)\s*```$/i);
  if (fencedMatch?.[1]) {
    candidates.add(fencedMatch[1].trim());
  }

  const firstBrace = trimmed.indexOf("{");
  const lastBrace = trimmed.lastIndexOf("}");
  if (firstBrace >= 0 && lastBrace > firstBrace) {
    candidates.add(trimmed.slice(firstBrace, lastBrace + 1).trim());
  }

  return [...candidates];
}

function isStateNotInitialized(error: unknown): boolean {
  return (
    error instanceof LinkClawCommandError &&
    (error.envelope?.error?.code === "state_not_initialized" ||
      error.message.includes("state db not found"))
  );
}

function formatCommandError(action: string, error: unknown): string {
  if (error instanceof LinkClawCommandError) {
    return `${action} failed: ${error.message}`;
  }
  if (error instanceof Error) {
    return `${action} failed: ${error.message}`;
  }
  return `${action} failed`;
}
