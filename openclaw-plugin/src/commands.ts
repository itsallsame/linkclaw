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

function formatImportMessage(value: unknown): string {
  if (typeof value !== "object" || value === null) {
    return "linkclaw import completed";
  }

  const record = value as Record<string, unknown>;
  const inspection = toInspectResult(record.inspection);
  const lines = ["linkclaw import completed"];

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

  return lines.join("\n");
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
