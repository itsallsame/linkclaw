import { mkdir, rm } from "node:fs/promises";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { execFile } from "node:child_process";
import { promisify } from "node:util";

const execFileAsync = promisify(execFile);

const scriptDir = dirname(fileURLToPath(import.meta.url));
const pluginRoot = resolve(scriptDir, "..");
const repoRoot = resolve(pluginRoot, "..");
const bundledRuntimeDir = resolve(pluginRoot, "bundled-runtime");
const binaryName = process.platform === "win32" ? "linkclaw.exe" : "linkclaw";
const binaryPath = join(bundledRuntimeDir, binaryName);

await rm(bundledRuntimeDir, { recursive: true, force: true });
await mkdir(bundledRuntimeDir, { recursive: true });

await execFileAsync(
  "go",
  ["build", "-o", binaryPath, "./cmd/linkclaw"],
  {
    cwd: repoRoot,
    encoding: "utf8",
    maxBuffer: 8 * 1024 * 1024,
  },
);

process.stdout.write(`staged bundled LinkClaw runtime: ${binaryPath}\n`);
