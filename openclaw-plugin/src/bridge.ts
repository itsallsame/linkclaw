import { access, constants, stat } from "node:fs/promises";
import { delimiter, dirname, join, resolve } from "node:path";
import { homedir } from "node:os";
import { execFile } from "node:child_process";
import { promisify } from "node:util";

const execFileAsync = promisify(execFile);

export const DEFAULT_RELAY_URL = "http://127.0.0.1:8788";

export type LinkClawPluginConfig = {
  binaryPath?: string;
  home?: string;
  relayUrl?: string;
  publishOrigin?: string;
  publishOutput?: string;
  publishTier?: "minimum" | "recommended" | "full";
  syncIntervalMs?: number;
};

export type LinkClawCommand =
  | "init"
  | "publish"
  | "inspect"
  | "import"
  | "card_export"
  | "card_import"
  | "message_send"
  | "message_inbox"
  | "message_thread"
  | "message_outbox"
  | "message_sync"
  | "message_status"
  | "known_ls"
  | "known_show"
  | "known_trust"
  | "known_note"
  | "known_refresh"
  | "known_rm";

export type LinkClawBridgeRequest = {
  command: string;
  home?: string;
  canonicalId?: string;
  displayName?: string;
  origin?: string;
  output?: string;
  tier?: string;
  input?: string;
  identifier?: string;
  body?: string;
  limit?: number;
  trustLevel?: string;
  riskFlags?: string[];
  clearRiskFlags?: boolean;
  reason?: string;
  noteBody?: string;
  allowDiscovered?: boolean;
  allowMismatch?: boolean;
};

export type LinkClawEnvelopeError = {
  code: string;
  message: string;
  retryable: boolean;
  details: Record<string, unknown>;
};

export type LinkClawEnvelope = {
  schema_version?: string;
  ok: boolean;
  command: string;
  subcommand?: string | null;
  timestamp?: string;
  warnings?: string[];
  result?: unknown;
  error?: LinkClawEnvelopeError;
};

type ExecFailure = Error & {
  code?: number | string;
  stdout?: string | Buffer;
  stderr?: string | Buffer;
};

export class LinkClawBinaryNotFoundError extends Error {
  readonly searchedPaths: string[];

  constructor(searchedPaths: string[]) {
    super(
      `unable to locate a linkclaw binary; searched ${searchedPaths.length} candidate path(s)`,
    );
    this.name = "LinkClawBinaryNotFoundError";
    this.searchedPaths = searchedPaths;
  }
}

export class LinkClawCommandError extends Error {
  readonly exitCode: number;
  readonly stderr: string;
  readonly stdout: string;
  readonly envelope?: LinkClawEnvelope;

  constructor(message: string, init: { exitCode: number; stderr: string; stdout: string; envelope?: LinkClawEnvelope }) {
    super(message);
    this.name = "LinkClawCommandError";
    this.exitCode = init.exitCode;
    this.stderr = init.stderr;
    this.stdout = init.stdout;
    this.envelope = init.envelope;
  }
}

type PreparedCommand = {
  binaryPath: string;
  args: string[];
  home: string;
  output?: string;
};

export async function runLinkClaw(
  config: LinkClawPluginConfig,
  request: LinkClawBridgeRequest,
  pluginRoot: string,
): Promise<LinkClawEnvelope> {
  const prepared = await prepareLinkClawCommand(config, request, pluginRoot);
  const relayUrl = resolveRelayUrl(config);
  try {
    const { stdout } = await execFileAsync(prepared.binaryPath, prepared.args, {
      cwd: dirname(pluginRoot),
      env: {
        ...process.env,
        LINKCLAW_HOME: prepared.home,
        ...(relayUrl ? { LINKCLAW_RELAY_URL: relayUrl } : {}),
      },
      encoding: "utf8",
      maxBuffer: 4 * 1024 * 1024,
    });
    const envelope = parseEnvelope(stdout);
    if (!envelope.ok) {
      throw new LinkClawCommandError(
        envelopeMessage(envelope, `${request.command} failed`),
        {
          exitCode: 1,
          stderr: "",
          stdout,
          envelope,
        },
      );
    }
    return envelope;
  } catch (error) {
    const failure = error as ExecFailure;
    const stdout = toString(failure.stdout);
    const stderr = toString(failure.stderr);
    const envelope = stdout === "" ? undefined : maybeParseEnvelope(stdout);
    if (error instanceof LinkClawCommandError) {
      throw error;
    }
    if (typeof failure.code === "number") {
      const message = envelopeMessage(
        envelope,
        stderr.trim() || `${request.command} failed`,
      );
      throw new LinkClawCommandError(
        message,
        {
          exitCode: failure.code,
          stderr,
          stdout,
          envelope,
        },
      );
    }
    throw error;
  }
}

export async function prepareLinkClawCommand(
  config: LinkClawPluginConfig,
  request: LinkClawBridgeRequest,
  pluginRoot: string,
): Promise<PreparedCommand> {
  const command = normalizeCommand(request.command);
  const home = resolveLinkClawHome(request.home, config);
  const output =
    command === "publish" ? resolvePublishOutput(request.output, home, config) : undefined;
  return {
    binaryPath: await resolveLinkClawBinary(config, pluginRoot),
    args: buildLinkClawArgs(command, request, home, output),
    home,
    output,
  };
}

export function resolveLinkClawHome(
  overrideHome: string | undefined,
  config: LinkClawPluginConfig,
): string {
  const raw = overrideHome ?? config.home ?? process.env.LINKCLAW_HOME ?? join(homedir(), ".linkclaw");
  return resolve(raw);
}

export function resolveRelayUrl(config: LinkClawPluginConfig): string | undefined {
  const raw = config.relayUrl ?? process.env.LINKCLAW_RELAY_URL ?? DEFAULT_RELAY_URL;
  if (typeof raw !== "string") {
    return undefined;
  }
  const trimmed = raw.trim();
  return trimmed === "" ? undefined : trimmed;
}

export function resolvePublishOutput(
  overrideOutput: string | undefined,
  home: string,
  config: LinkClawPluginConfig,
): string {
  return resolve(overrideOutput ?? config.publishOutput ?? join(home, "publish"));
}

export function resolvePublishTier(
  overrideTier: string | undefined,
  config: LinkClawPluginConfig,
): string {
  return overrideTier ?? config.publishTier ?? "recommended";
}

export async function resolveLinkClawBinary(
  config: LinkClawPluginConfig,
  pluginRoot: string,
): Promise<string> {
  const candidates = candidateBinaryPaths(config, pluginRoot);
  for (const candidate of candidates) {
    if (await isExecutableFile(candidate)) {
      return candidate;
    }
  }
  throw new LinkClawBinaryNotFoundError(candidates);
}

export function buildLinkClawArgs(
  command: LinkClawCommand,
  request: LinkClawBridgeRequest,
  home: string,
  publishOutput?: string,
): string[] {
  const args: string[] = [];
	switch (command) {
	case "init":
		args.push("init", "--home", home, "--non-interactive", "--json");
		if (request.canonicalId) {
			args.push("--canonical-id", request.canonicalId);
		}
		if (request.displayName) {
			args.push("--display-name", request.displayName);
		}
      return args;
    case "publish":
      args.push("publish", "--home", home, "--json");
      if (request.origin) {
        args.push("--origin", request.origin);
      }
      if (publishOutput) {
        args.push("--output", publishOutput);
      }
      args.push("--tier", request.tier ?? "recommended");
      return args;
    case "inspect":
      requireField(request.input, "input");
      return ["inspect", "--json", request.input];
    case "import":
      requireField(request.input, "input");
      args.push("import", "--home", home, "--json");
      if (request.allowDiscovered) {
        args.push("--allow-discovered");
      }
      if (request.allowMismatch) {
        args.push("--allow-mismatch");
      }
      args.push(request.input);
      return args;
    case "card_export":
      return ["card", "export", "--home", home, "--json"];
    case "card_import":
      requireField(request.input, "input");
      return ["card", "import", "--home", home, "--json", request.input];
    case "message_send":
      requireField(request.identifier, "identifier");
      requireField(request.body, "body");
      return [
        "message",
        "send",
        "--home",
        home,
        "--body",
        request.body,
        "--json",
        request.identifier,
      ];
    case "message_inbox":
      return ["message", "inbox", "--home", home, "--json"];
    case "message_thread":
      requireField(request.identifier, "identifier");
      args.push("message", "thread", "--home", home);
      if (typeof request.limit === "number" && Number.isFinite(request.limit)) {
        args.push("--limit", String(Math.floor(request.limit)));
      }
      args.push("--json", request.identifier);
      return args;
    case "message_outbox":
      return ["message", "outbox", "--home", home, "--json"];
    case "message_sync":
      return ["message", "sync", "--home", home, "--json"];
    case "message_status":
      return ["message", "status", "--home", home, "--json"];
    case "known_ls":
      return ["known", "ls", "--home", home, "--json"];
    case "known_show":
      requireField(request.identifier, "identifier");
      return ["known", "show", "--home", home, "--json", request.identifier];
    case "known_trust":
      requireField(request.identifier, "identifier");
      args.push("known", "trust", "--home", home, "--json");
      if (request.trustLevel) {
        args.push("--level", request.trustLevel);
      }
      if (request.riskFlags && request.riskFlags.length > 0) {
        args.push("--risk", request.riskFlags.join(","));
      } else if (request.clearRiskFlags) {
        args.push("--risk", "");
      }
      if (request.reason) {
        args.push("--reason", request.reason);
      }
      args.push(request.identifier);
      return args;
    case "known_note":
      requireField(request.identifier, "identifier");
      requireField(request.noteBody, "noteBody");
      return [
        "known",
        "note",
        "--home",
        home,
        "--body",
        request.noteBody,
        "--json",
        request.identifier,
      ];
    case "known_refresh":
      requireField(request.identifier, "identifier");
      return ["known", "refresh", "--home", home, "--json", request.identifier];
    case "known_rm":
      requireField(request.identifier, "identifier");
      return ["known", "rm", "--home", home, "--json", request.identifier];
    default:
      return assertNever(command);
  }
}

function candidateBinaryPaths(config: LinkClawPluginConfig, pluginRoot: string): string[] {
  const binaryName = process.platform === "win32" ? "linkclaw.exe" : "linkclaw";
  const explicit = [
    config.binaryPath,
    process.env.LINKCLAW_BINARY,
  ]
    .filter((value): value is string => typeof value === "string" && value.trim() !== "")
    .map((value) => resolve(value));

  const local = [
    resolve(pluginRoot, "..", "bin", binaryName),
    resolve(pluginRoot, "..", binaryName),
    resolve(pluginRoot, "bin", binaryName),
    resolve(process.cwd(), "bin", binaryName),
    resolve(process.cwd(), binaryName),
  ];

  const pathEntries = (process.env.PATH ?? "")
    .split(delimiter)
    .filter((entry) => entry.trim() !== "")
    .map((entry) => resolve(entry, binaryName));

  return dedupePaths([...explicit, ...local, ...pathEntries]);
}

function dedupePaths(paths: string[]): string[] {
  return [...new Set(paths)];
}

async function isExecutableFile(pathname: string): Promise<boolean> {
  try {
    const info = await stat(pathname);
    if (!info.isFile()) {
      return false;
    }
    await access(
      pathname,
      process.platform === "win32" ? constants.F_OK : constants.X_OK,
    );
    return true;
  } catch {
    return false;
  }
}

function parseEnvelope(stdout: string): LinkClawEnvelope {
  const parsed = maybeParseEnvelope(stdout);
  if (!parsed) {
    throw new Error(`linkclaw stdout was not valid JSON: ${stdout}`);
  }
  return parsed;
}

function maybeParseEnvelope(stdout: string): LinkClawEnvelope | undefined {
  try {
    const value = JSON.parse(stdout);
    if (isEnvelope(value)) {
      return value;
    }
  } catch {
    return undefined;
  }
  return undefined;
}

function envelopeMessage(envelope: LinkClawEnvelope | undefined, fallback: string): string {
  const message = envelope?.error?.message;
  if (typeof message === "string" && message.trim() !== "") {
    return message;
  }
  return fallback;
}

function isEnvelope(value: unknown): value is LinkClawEnvelope {
  if (typeof value !== "object" || value === null) {
    return false;
  }
  const candidate = value as Record<string, unknown>;
  return typeof candidate.ok === "boolean" && typeof candidate.command === "string";
}

function requireField<T>(value: T | undefined, name: string): asserts value is T {
  if (value === undefined || value === null || value === "") {
    throw new Error(`${name} is required`);
  }
}

function normalizeCommand(raw: string): LinkClawCommand {
  switch (raw) {
    case "init":
    case "publish":
    case "inspect":
    case "import":
    case "card_export":
    case "card_import":
    case "message_send":
    case "message_inbox":
    case "message_thread":
    case "message_outbox":
    case "message_sync":
    case "message_status":
    case "known_ls":
    case "known_show":
    case "known_trust":
    case "known_note":
    case "known_refresh":
    case "known_rm":
      return raw;
    default:
      throw new Error(`unsupported linkclaw command: ${raw}`);
  }
}

function toString(value: string | Buffer | undefined): string {
  if (typeof value === "string") {
    return value;
  }
  if (value) {
    return value.toString("utf8");
  }
  return "";
}

function assertNever(value: never): never {
  throw new Error(`unreachable command: ${value}`);
}
