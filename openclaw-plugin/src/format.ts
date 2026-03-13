type Artifact = {
  type?: string;
  path?: string;
  url?: string;
  sha256?: string;
};

type Check = {
  name?: string;
  ok?: boolean;
  details?: string;
};

type PublishResultLike = {
  tier?: string;
  home_origin?: string;
  output_dir?: string;
  manifest_path?: string;
  generated_at?: string;
  artifacts?: Artifact[];
  checks?: Check[];
};

export type PublishManifest = {
  generated_at?: string;
  tier?: string;
  home_origin?: string;
  artifacts?: Artifact[];
  checks?: Check[];
};

export function formatPublishSummary(input: {
  result?: unknown;
  manifest?: PublishManifest;
  error?: string;
  outputDir?: string;
  manifestPath?: string;
}): string {
  const result = toPublishResult(input.result);
  const manifest = input.manifest ?? {};
  const artifacts = result.artifacts ?? manifest.artifacts ?? [];
  const checks = result.checks ?? manifest.checks ?? [];
  const lines: string[] = [];

  if (input.error && checks.length > 0) {
    lines.push("linkclaw publish failed after bundle generation");
  } else if (input.error) {
    lines.push("linkclaw publish failed");
  } else {
    lines.push("linkclaw publish completed");
  }

  if (input.error) {
    lines.push(`error: ${input.error}`);
  }
  if (result.home_origin ?? manifest.home_origin) {
    lines.push(`origin: ${result.home_origin ?? manifest.home_origin}`);
  }
  if (result.tier ?? manifest.tier) {
    lines.push(`tier: ${result.tier ?? manifest.tier}`);
  }
  if (result.output_dir ?? input.outputDir) {
    lines.push(`output: ${result.output_dir ?? input.outputDir}`);
  }
  if (result.manifest_path ?? input.manifestPath) {
    lines.push(`manifest: ${result.manifest_path ?? input.manifestPath}`);
  }
  if (result.generated_at ?? manifest.generated_at) {
    lines.push(`generated: ${result.generated_at ?? manifest.generated_at}`);
  }

  lines.push(`artifacts: ${artifacts.length}`);
  for (const artifact of artifacts) {
    const label = artifact.type ?? "artifact";
    const path = artifact.path ?? "(unknown path)";
    const url = artifact.url ? ` -> ${artifact.url}` : "";
    lines.push(`- ${label}: ${path}${url}`);
  }

  lines.push(`checks: ${checks.length}`);
  for (const check of checks) {
    const status = check.ok ? "PASS" : "FAIL";
    const name = check.name ?? "unnamed-check";
    const details = check.details ? ` (${check.details})` : "";
    lines.push(`- ${status} ${name}${details}`);
  }

  return lines.join("\n");
}

export function toPublishResult(value: unknown): PublishResultLike {
  if (typeof value !== "object" || value === null) {
    return {};
  }
  const record = value as Record<string, unknown>;
  return {
    tier: asOptionalString(record.tier),
    home_origin: asOptionalString(record.home_origin),
    output_dir: asOptionalString(record.output_dir),
    manifest_path: asOptionalString(record.manifest_path),
    generated_at: asOptionalString(record.generated_at),
    artifacts: asArtifacts(record.artifacts),
    checks: asChecks(record.checks),
  };
}

function asOptionalString(value: unknown): string | undefined {
  return typeof value === "string" && value.trim() !== "" ? value : undefined;
}

function asArtifacts(value: unknown): Artifact[] | undefined {
  if (!Array.isArray(value)) {
    return undefined;
  }
  return value
    .filter((item): item is Record<string, unknown> => typeof item === "object" && item !== null)
    .map((item) => ({
      type: asOptionalString(item.type),
      path: asOptionalString(item.path),
      url: asOptionalString(item.url),
      sha256: asOptionalString(item.sha256),
    }));
}

function asChecks(value: unknown): Check[] | undefined {
  if (!Array.isArray(value)) {
    return undefined;
  }
  return value
    .filter((item): item is Record<string, unknown> => typeof item === "object" && item !== null)
    .map((item) => ({
      name: asOptionalString(item.name),
      ok: typeof item.ok === "boolean" ? item.ok : undefined,
      details: asOptionalString(item.details),
    }));
}
