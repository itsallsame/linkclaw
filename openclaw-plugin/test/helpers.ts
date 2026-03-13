import { createServer } from "node:http";
import { chmod, mkdtemp, readFile, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { dirname, join, resolve } from "node:path";
import { execFile } from "node:child_process";
import { promisify } from "node:util";
import { fileURLToPath } from "node:url";

const execFileAsync = promisify(execFile);

export const pluginRoot = fileURLToPath(new URL("..", import.meta.url));
export const repoRoot = fileURLToPath(new URL("../..", import.meta.url));

export async function buildLinkClawBinary(): Promise<string> {
  const outputDir = await mkdtemp(join(tmpdir(), "linkclaw-plugin-bin-"));
  const binaryPath = join(outputDir, process.platform === "win32" ? "linkclaw.exe" : "linkclaw");
  await execFileAsync("go", ["build", "-o", binaryPath, "./cmd/linkclaw"], {
    cwd: repoRoot,
    encoding: "utf8",
    maxBuffer: 4 * 1024 * 1024,
  });
  return binaryPath;
}

export async function createResolverFixtureServer(): Promise<{
  close: () => Promise<void>;
  origin: string;
}> {
  const fixtureRoot = resolve(repoRoot, "internal", "resolver", "testdata", "consistent");
  const server = createServer(async (req, res) => {
    const requestPath = req.url ?? "/";
    const cleaned = requestPath.endsWith("/")
      ? join(requestPath, "index.html")
      : requestPath;
    const filePath = join(fixtureRoot, cleaned.replace(/^\//, ""));
    try {
      const content = await readFile(filePath, "utf8");
      const origin = `http://127.0.0.1:${addressPort(server)}`;
      const replaced = content
        .replaceAll("{{ORIGIN}}", origin)
        .replaceAll("{{RESOURCE}}", `${origin}/`);
      const ext = filePath.endsWith(".html") ? "text/html; charset=utf-8" : "application/json";
      res.writeHead(200, { "content-type": ext });
      res.end(replaced);
    } catch {
      res.writeHead(404);
      res.end("not found");
    }
  });

  await new Promise<void>((resolvePromise) => {
    server.listen(0, "127.0.0.1", () => resolvePromise());
  });

  return {
    origin: `http://127.0.0.1:${addressPort(server)}`,
    close: async () => {
      await new Promise<void>((resolvePromise, rejectPromise) => {
        server.close((error) => {
          if (error) {
            rejectPromise(error);
            return;
          }
          resolvePromise();
        });
      });
    },
  };
}

export async function writeFailingPublishBinary(targetDir: string): Promise<string> {
  const binaryPath = join(targetDir, "fake-linkclaw.mjs");
  const content = `#!/usr/bin/env node
import { mkdirSync, writeFileSync } from "node:fs";
import { join } from "node:path";

const args = process.argv.slice(2);
const outputIndex = args.indexOf("--output");
const outputDir = outputIndex >= 0 ? args[outputIndex + 1] : process.cwd();
mkdirSync(join(outputDir, ".well-known"), { recursive: true });
writeFileSync(
  join(outputDir, "manifest.json"),
  JSON.stringify(
    {
      generated_at: "2026-03-13T12:00:00Z",
      tier: "recommended",
      home_origin: "https://failed.example",
      artifacts: [
        {
          type: "did",
          path: ".well-known/did.json",
          url: "https://failed.example/.well-known/did.json"
        }
      ],
      checks: [
        {
          name: "did-canonical-id",
          ok: false,
          details: "fixture failure"
        }
      ]
    },
    null,
    2
  )
);
process.stdout.write(JSON.stringify({
  schema_version: "linkclaw.cli.v1",
  ok: false,
  command: "publish",
  subcommand: null,
  timestamp: "2026-03-13T12:00:00Z",
  warnings: [],
  error: {
    code: "command_failed",
    message: "bundle consistency checks failed: did-canonical-id",
    retryable: false,
    details: {
      kind: "command"
    }
  }
}));
process.exit(1);
`;
  await writeFile(binaryPath, content, "utf8");
  await chmod(binaryPath, 0o755);
  return binaryPath;
}

function addressPort(server: ReturnType<typeof createServer>): number {
  const address = server.address();
  if (!address || typeof address === "string") {
    throw new Error("fixture server address is unavailable");
  }
  return address.port;
}
