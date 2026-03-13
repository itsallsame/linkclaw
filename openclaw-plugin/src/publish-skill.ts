import { readFile } from "node:fs/promises";
import { join } from "node:path";

import {
  type LinkClawPluginConfig,
  LinkClawCommandError,
  prepareLinkClawCommand,
  resolvePublishTier,
  runLinkClaw,
} from "./bridge.ts";
import { formatPublishSummary, type PublishManifest } from "./format.ts";

type PublishSkillParams = Record<string, unknown>;

type PublishOptions = {
  home?: string;
  origin?: string;
  output?: string;
  tier?: string;
};

export async function runPublishSkill(
  config: LinkClawPluginConfig,
  params: PublishSkillParams,
  pluginRoot: string,
): Promise<string> {
  const options = resolvePublishSkillOptions(config, params);
  const request = {
    command: "publish",
    home: options.home,
    origin: options.origin,
    output: options.output,
    tier: options.tier,
  } as const;
  const prepared = await prepareLinkClawCommand(config, request, pluginRoot);

  try {
    const envelope = await runLinkClaw(config, request, pluginRoot);
    return formatPublishSummary({ result: envelope.result });
  } catch (error) {
    if (!(error instanceof LinkClawCommandError)) {
      throw error;
    }
    const manifest = await loadManifestFallback(prepared.output);
    return formatPublishSummary({
      result: error.envelope?.result,
      manifest,
      error: error.message,
      outputDir: prepared.output,
      manifestPath: prepared.output ? join(prepared.output, "manifest.json") : undefined,
    });
  }
}

export function resolvePublishSkillOptions(
  config: LinkClawPluginConfig,
  params: PublishSkillParams,
): PublishOptions {
  const rawCommand = asOptionalString(params.command);
  const fromCommand = rawCommand ? parsePublishCommand(rawCommand) : {};

  return {
    home: fromCommand.home ?? asOptionalString(params.home),
    origin:
      fromCommand.origin ??
      asOptionalString(params.origin) ??
      config.publishOrigin,
    output: fromCommand.output ?? asOptionalString(params.output),
    tier:
      fromCommand.tier ??
      asOptionalString(params.tier) ??
      resolvePublishTier(undefined, config),
  };
}

async function loadManifestFallback(outputDir: string | undefined): Promise<PublishManifest | undefined> {
  if (!outputDir) {
    return undefined;
  }
  try {
    const content = await readFile(join(outputDir, "manifest.json"), "utf8");
    return JSON.parse(content) as PublishManifest;
  } catch {
    return undefined;
  }
}

export function parsePublishCommand(raw: string): PublishOptions {
  const tokens = tokenizeCommand(raw);
  const options: PublishOptions = {};

  for (let index = 0; index < tokens.length; index += 1) {
    const token = tokens[index];
    if (token === "--home") {
      options.home = requireNext(tokens, ++index, "--home");
      continue;
    }
    if (token === "--origin") {
      options.origin = requireNext(tokens, ++index, "--origin");
      continue;
    }
    if (token === "--output") {
      options.output = requireNext(tokens, ++index, "--output");
      continue;
    }
    if (token === "--tier") {
      options.tier = requireNext(tokens, ++index, "--tier");
      continue;
    }
    throw new Error(`unsupported publish skill argument: ${token}`);
  }

  return options;
}

export function tokenizeCommand(raw: string): string[] {
  const tokens: string[] = [];
  let current = "";
  let quote: "'" | "\"" | undefined;

  for (let index = 0; index < raw.length; index += 1) {
    const char = raw[index];
    if (quote) {
      if (char === quote) {
        quote = undefined;
        continue;
      }
      if (char === "\\" && quote === "\"" && index + 1 < raw.length) {
        current += raw[index + 1];
        index += 1;
        continue;
      }
      current += char;
      continue;
    }

    if (char === "'" || char === "\"") {
      quote = char;
      continue;
    }
    if (/\s/.test(char)) {
      if (current !== "") {
        tokens.push(current);
        current = "";
      }
      continue;
    }
    if (char === "\\" && index + 1 < raw.length) {
      current += raw[index + 1];
      index += 1;
      continue;
    }
    current += char;
  }

  if (quote) {
    throw new Error("unterminated quote in publish skill command");
  }
  if (current !== "") {
    tokens.push(current);
  }
  return tokens;
}

function requireNext(tokens: string[], index: number, flag: string): string {
  if (index >= tokens.length) {
    throw new Error(`missing value for ${flag}`);
  }
  return tokens[index];
}

function asOptionalString(value: unknown): string | undefined {
  return typeof value === "string" && value.trim() !== "" ? value : undefined;
}
