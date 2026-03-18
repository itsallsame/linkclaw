import {
  LinkClawCommandError,
  type LinkClawPluginConfig,
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

type ImportOptions = {
  input: string;
  allowDiscovered: boolean;
  allowMismatch: boolean;
  home?: string;
};

type ShareOptions = {
  origin?: string;
};

type ConnectOptions = {
  input: string;
  home?: string;
};

type MessageOptions = {
  identifier: string;
  body: string;
  home?: string;
};

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

  const origin = options.origin ?? config.publishOrigin;
  if (!origin) {
    return {
      type: "message",
      message:
        "Set plugins.entries.linkclaw.config.publishOrigin or pass /linkclaw-share --origin https://agent.example.",
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
    const envelope = await runLinkClaw(
      config,
      {
        command: "card_import",
        home: options.home,
        input: options.input,
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
      message: "Usage: /linkclaw-message [--home /path/to/home] <contact> <message>",
    };
  }

  try {
    const envelope = await runLinkClaw(
      config,
      {
        command: "message_send",
        home: options.home,
        identifier: options.identifier,
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
      message: formatCommandError("linkclaw message", error),
    };
  }
}

export async function runInboxCommand(
  config: LinkClawPluginConfig,
  rawArgs: string,
  pluginRoot: string,
): Promise<CommandResult> {
  let home: string | undefined;
  try {
    home = parseOptionalHome(rawArgs, "inbox");
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
        home,
      },
      pluginRoot,
    );
    return {
      type: "message",
      message: formatInbox(envelope.result),
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
  const lines = ["Contact added to your LinkClaw contacts."];
  if (typeof record?.contact_id === "string" && record.contact_id.trim() !== "") {
    lines.push(`contact: ${record.contact_id}`);
  }
  if (typeof card?.display_name === "string" && card.display_name.trim() !== "") {
    lines.push(`name: ${card.display_name}`);
  }
  if (typeof card?.id === "string" && card.id.trim() !== "") {
    lines.push(`canonical id: ${card.id}`);
  }
  if (typeof record?.created === "boolean") {
    lines.push(`created: ${record.created ? "yes" : "no"}`);
  }
  lines.push("Next:");
  if (typeof card?.display_name === "string" && card.display_name.trim() !== "") {
    lines.push(`- send a message with /linkclaw-message ${card.display_name} <text>`);
  } else {
    lines.push("- send a message with /linkclaw-message <contact> <text>");
  }
  lines.push("- or run /linkclaw-inbox to review existing conversations");
  return lines.join("\n");
}

function formatMessageSend(value: unknown): string {
  const record = asObject(value);
  const message = asObject(record?.message);
  const lines = ["Message queued for relay delivery."];
  if (typeof message?.message_id === "string") {
    lines.push(`message: ${message.message_id}`);
  }
  if (typeof message?.status === "string") {
    lines.push(`status: ${message.status}`);
  }
  if (typeof message?.preview === "string") {
    lines.push(`preview: ${message.preview}`);
  }
  lines.push("Next:");
  lines.push("- the recipient needs to run /linkclaw-sync to receive it");
  lines.push("- use /linkclaw-inbox to review local conversation state");
  return lines.join("\n");
}

function formatInbox(value: unknown): string {
  const record = asObject(value);
  const conversations = Array.isArray(record?.conversations) ? record.conversations : [];
  const lines = ["LinkClaw inbox", `conversations: ${conversations.length}`];
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
    lines.push(`- ${name} | ${canonicalID} | ${senderLabel} | unread=${unread} | last=${preview}`);
  }
  if (hasUnknownSender) {
    lines.push("Next:");
    lines.push("- ask the sender for an identity card");
    lines.push("- then run /linkclaw-connect <card> if you want to keep them");
  } else if (conversations.length > 0) {
    lines.push("Next:");
    lines.push("- reply with /linkclaw-message <contact> <text>");
  }
  return lines.join("\n");
}

function formatSync(value: unknown): string {
  const record = asObject(value);
  const synced = typeof record?.synced === "number" ? record.synced : 0;
  const relayCalls = typeof record?.relay_calls === "number" ? record.relay_calls : 0;
  const lines = ["LinkClaw sync completed", `synced: ${synced}`, `relay calls: ${relayCalls}`];
  lines.push("Next:");
  if (synced > 0) {
    lines.push("- run /linkclaw-inbox to read new messages");
  } else {
    lines.push("- no new messages yet; run /linkclaw-message <contact> <text> to start a conversation");
  }
  return lines.join("\n");
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

function formatCommandError(action: string, error: unknown): string {
  if (error instanceof LinkClawCommandError) {
    return `${action} failed: ${error.message}`;
  }
  if (error instanceof Error) {
    return `${action} failed: ${error.message}`;
  }
  return `${action} failed`;
}
