import {
  LinkClawCommandError,
  type LinkClawPluginConfig,
  runLinkClaw,
} from "./bridge.ts";

type InspectArtifact = {
  type?: string;
  url?: string;
  ok?: boolean;
  http_status?: number;
  summary?: string;
  error?: string;
};

export type InspectResult = {
  input?: string;
  normalized_origin?: string;
  verification_state?: string;
  can_import?: boolean;
  canonical_id?: string;
  display_name?: string;
  profile_url?: string;
  artifacts?: InspectArtifact[];
  warnings?: string[];
  mismatches?: string[];
  resolved_at?: string;
};

export type HookMessage = {
  role: "assistant" | "system" | "user";
  content: string;
};

export type HookEvent = Record<string, unknown> & {
  context?: unknown;
  messages?: HookMessage[];
};

const identityArtifactPaths = new Set([
  "/.well-known/agent-card.json",
  "/.well-known/did.json",
]);

const trailingURLPunctuation = /[),.;!?]+$/;

export function extractIdentityArtifactURLs(content: string): string[] {
  const matches = content.match(/https?:\/\/[^\s<>"'`]+/g) ?? [];
  const urls: string[] = [];
  const seen = new Set<string>();

  for (const match of matches) {
    const candidate = trimTrailingURLPunctuation(match);
    let parsed: URL;
    try {
      parsed = new URL(candidate);
    } catch {
      continue;
    }
    if (!identityArtifactPaths.has(parsed.pathname)) {
      continue;
    }
    parsed.hash = "";
    const normalized = parsed.toString();
    if (seen.has(normalized)) {
      continue;
    }
    seen.add(normalized);
    urls.push(normalized);
  }

  return urls;
}

export function toInspectResult(value: unknown): InspectResult {
  if (typeof value !== "object" || value === null) {
    return {};
  }
  const record = value as Record<string, unknown>;
  return {
    input: asOptionalString(record.input),
    normalized_origin: asOptionalString(record.normalized_origin),
    verification_state:
      asOptionalString(record.verification_state) ?? asOptionalString(record.status),
    can_import: typeof record.can_import === "boolean" ? record.can_import : undefined,
    canonical_id: asOptionalString(record.canonical_id),
    display_name: asOptionalString(record.display_name),
    profile_url: asOptionalString(record.profile_url),
    artifacts: asArtifacts(record.artifacts),
    warnings: asStringArray(record.warnings),
    mismatches: asStringArray(record.mismatches),
    resolved_at: asOptionalString(record.resolved_at),
  };
}

export function ensureHookMessages(event: HookEvent): HookMessage[] {
  if (Array.isArray(event.messages)) {
    return event.messages;
  }
  const messages: HookMessage[] = [];
  event.messages = messages;
  return messages;
}

export function buildShareLinks(origin: string): {
  origin: string;
  didURL: string;
  agentCardURL: string;
  profileURL: string;
} {
  const parsed = new URL(origin);
  parsed.pathname = "";
  parsed.search = "";
  parsed.hash = "";
  const normalizedOrigin = parsed.toString().replace(/\/$/, "");
  return {
    origin: normalizedOrigin,
    didURL: `${normalizedOrigin}/.well-known/did.json`,
    agentCardURL: `${normalizedOrigin}/.well-known/agent-card.json`,
    profileURL: `${normalizedOrigin}/profile/`,
  };
}

export function appendDIDLinkToText(content: string, origin: string): string {
  const links = buildShareLinks(origin);
  if (!content.includes(links.agentCardURL) || content.includes(links.didURL)) {
    return content;
  }
  return `${content}\n\ndid.json: ${links.didURL}`;
}

export function attachDIDLinkToOutgoingEvent(event: unknown, origin: string): boolean {
  if (typeof event !== "object" || event === null) {
    return false;
  }

  let mutated = false;
  const visit = (record: Record<string, unknown>, key: string) => {
    if (typeof record[key] !== "string") {
      return;
    }
    const next = appendDIDLinkToText(record[key], origin);
    if (next === record[key]) {
      return;
    }
    record[key] = next;
    mutated = true;
  };

  const root = event as Record<string, unknown>;
  visit(root, "content");

  for (const key of ["context", "message", "payload"]) {
    const nested = root[key];
    if (typeof nested !== "object" || nested === null) {
      continue;
    }
    visit(nested as Record<string, unknown>, "content");
  }

  return mutated;
}

export async function handlePassiveDiscovery(
  config: LinkClawPluginConfig,
  event: HookEvent,
  pluginRoot: string,
  importCommandName = "linkclaw-import",
): Promise<void> {
  const content = extractEventContent(event);
  if (content === "") {
    return;
  }

  const sources = extractIdentityArtifactURLs(content);
  if (sources.length === 0) {
    return;
  }

  const messages = ensureHookMessages(event);
  const prompted = new Set<string>();

  for (const source of sources.slice(0, 3)) {
    let inspection: InspectResult;
    try {
      const envelope = await runLinkClaw(
        config,
        {
          command: "inspect",
          input: source,
        },
        pluginRoot,
      );
      inspection = toInspectResult(envelope.result);
    } catch {
      continue;
    }

    const dedupeKey =
      inspection.canonical_id ??
      inspection.profile_url ??
      inspection.normalized_origin ??
      source;
    if (prompted.has(dedupeKey)) {
      continue;
    }
    prompted.add(dedupeKey);

    if (await isKnownIdentity(config, inspection, source, pluginRoot)) {
      continue;
    }

    messages.push({
      role: "assistant",
      content: formatInspectionPrompt(inspection, source, importCommandName),
    });
  }
}

export function formatInspectionPrompt(
  inspection: InspectResult,
  source: string,
  importCommandName = "linkclaw-import",
): string {
  const lines = ["LinkClaw identity link detected", `source: ${source}`];

  if (inspection.display_name) {
    lines.push(`name: ${inspection.display_name}`);
  }
  if (inspection.canonical_id) {
    lines.push(`canonical id: ${inspection.canonical_id}`);
  }
  if (inspection.normalized_origin) {
    lines.push(`origin: ${inspection.normalized_origin}`);
  }
  if (inspection.profile_url) {
    lines.push(`profile: ${inspection.profile_url}`);
  }
  if (inspection.verification_state) {
    lines.push(`status: ${inspection.verification_state}`);
  }

  const artifactSummary = summarizeArtifacts(inspection.artifacts);
  if (artifactSummary !== "") {
    lines.push(`artifacts: ${artifactSummary}`);
  }

  if ((inspection.warnings ?? []).length > 0) {
    lines.push(`warnings: ${(inspection.warnings ?? []).join("; ")}`);
  }
  if ((inspection.mismatches ?? []).length > 0) {
    lines.push(`mismatches: ${(inspection.mismatches ?? []).join("; ")}`);
  }

  if (inspection.can_import) {
    lines.push(`Import it with: /${importCommandName} ${source}`);
  } else {
    lines.push("Import is blocked until the identity resolves cleanly.");
  }

  return lines.join("\n");
}

export function formatShareMessage(inspection: InspectResult, origin: string): string {
  const links = buildShareLinks(origin);
  const didURL = artifactURL(inspection.artifacts, "did") ?? links.didURL;
  const agentCardURL =
    artifactURL(inspection.artifacts, "agent-card") ?? links.agentCardURL;
  const profileURL = inspection.profile_url ?? artifactURL(inspection.artifacts, "profile");

  const lines = ["LinkClaw identity bundle ready to share", `origin: ${links.origin}`];

  if (inspection.canonical_id) {
    lines.push(`canonical id: ${inspection.canonical_id}`);
  }
  lines.push(`agent card: ${agentCardURL}`);
  lines.push(`did.json: ${didURL}`);
  if (profileURL) {
    lines.push(`profile: ${profileURL}`);
  }
  lines.push("Share the agent card and did.json links together.");
  return lines.join("\n");
}

export function hasShareableArtifacts(inspection: InspectResult): boolean {
  return (
    artifactOK(inspection.artifacts, "did") &&
    artifactOK(inspection.artifacts, "agent-card")
  );
}

async function isKnownIdentity(
  config: LinkClawPluginConfig,
  inspection: InspectResult,
  source: string,
  pluginRoot: string,
): Promise<boolean> {
  for (const identifier of knownLookupIdentifiers(inspection, source)) {
    try {
      await runLinkClaw(
        config,
        {
          command: "known_show",
          identifier,
        },
        pluginRoot,
      );
      return true;
    } catch (error) {
      if (isKnownMiss(error) || isMissingHome(error)) {
        continue;
      }
    }
  }
  return false;
}

function knownLookupIdentifiers(inspection: InspectResult, source: string): string[] {
  const candidates = [
    inspection.canonical_id,
    inspection.profile_url,
    inspection.normalized_origin,
    source,
  ];
  const seen = new Set<string>();
  return candidates.filter((candidate): candidate is string => {
    if (typeof candidate !== "string") {
      return false;
    }
    const trimmed = candidate.trim();
    if (trimmed === "" || seen.has(trimmed)) {
      return false;
    }
    seen.add(trimmed);
    return true;
  });
}

function extractEventContent(event: HookEvent): string {
  const parts = [
    contentFromUnknown(event.context),
    contentFromUnknown(event.content),
    contentFromUnknown((event as Record<string, unknown>).message),
  ];
  return parts.filter((part) => part !== "").join("\n");
}

function contentFromUnknown(value: unknown): string {
  if (typeof value === "string") {
    return value.trim();
  }
  if (Array.isArray(value)) {
    return value.map((entry) => contentFromUnknown(entry)).filter(Boolean).join("\n");
  }
  if (typeof value !== "object" || value === null) {
    return "";
  }
  const record = value as Record<string, unknown>;
  if (typeof record.content === "string") {
    return record.content.trim();
  }
  if (typeof record.text === "string") {
    return record.text.trim();
  }
  return "";
}

function summarizeArtifacts(artifacts: InspectArtifact[] | undefined): string {
  if (!Array.isArray(artifacts) || artifacts.length === 0) {
    return "";
  }
  return artifacts
    .map((artifact) => {
      const label = artifact.type ?? "artifact";
      return `${label}:${artifact.ok ? "ok" : "missing"}`;
    })
    .join(", ");
}

function artifactOK(artifacts: InspectArtifact[] | undefined, type: string): boolean {
  return artifacts?.some((artifact) => artifact.type === type && artifact.ok === true) ?? false;
}

function artifactURL(
  artifacts: InspectArtifact[] | undefined,
  type: string,
): string | undefined {
  return artifacts?.find((artifact) => artifact.type === type && artifact.ok === true)?.url;
}

function trimTrailingURLPunctuation(value: string): string {
  let result = value;
  while (trailingURLPunctuation.test(result)) {
    result = result.replace(trailingURLPunctuation, "");
  }
  return result;
}

function asOptionalString(value: unknown): string | undefined {
  return typeof value === "string" && value.trim() !== "" ? value : undefined;
}

function asStringArray(value: unknown): string[] | undefined {
  if (!Array.isArray(value)) {
    return undefined;
  }
  const items = value
    .filter((item): item is string => typeof item === "string")
    .map((item) => item.trim())
    .filter((item) => item !== "");
  return items.length > 0 ? items : [];
}

function asArtifacts(value: unknown): InspectArtifact[] | undefined {
  if (!Array.isArray(value)) {
    return undefined;
  }
  return value
    .filter((item): item is Record<string, unknown> => typeof item === "object" && item !== null)
    .map((item) => ({
      type: asOptionalString(item.type),
      url: asOptionalString(item.url),
      ok: typeof item.ok === "boolean" ? item.ok : undefined,
      http_status: typeof item.http_status === "number" ? item.http_status : undefined,
      summary: asOptionalString(item.summary),
      error: asOptionalString(item.error),
    }));
}

function isKnownMiss(error: unknown): boolean {
  return (
    error instanceof LinkClawCommandError &&
    /known contact .* not found/i.test(error.message)
  );
}

function isMissingHome(error: unknown): boolean {
  return (
    error instanceof LinkClawCommandError &&
    /state db not found/i.test(error.message)
  );
}
