import test from "node:test";
import assert from "node:assert/strict";
import { mkdtemp } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";

import {
  parsePublishCommand,
  runPublishSkill,
  tokenizeCommand,
} from "../src/publish-skill.ts";
import {
  pluginRoot,
  writeFailingPublishBinary,
} from "./helpers.ts";

test("tokenizeCommand handles quotes and escapes", () => {
  assert.deepEqual(tokenizeCommand(`--origin "https://agent.example" --output /tmp/x\\ y`), [
    "--origin",
    "https://agent.example",
    "--output",
    "/tmp/x y",
  ]);
});

test("parsePublishCommand rejects unsupported flags", () => {
  assert.throws(() => parsePublishCommand("--bad-flag value"), /unsupported publish skill argument/);
});

test("runPublishSkill falls back to manifest when publish exits with an error", async () => {
  const home = await mkdtemp(join(tmpdir(), "linkclaw-home-"));
  const binaryDir = await mkdtemp(join(tmpdir(), "linkclaw-fake-bin-"));
  const binaryPath = await writeFailingPublishBinary(binaryDir);

  const text = await runPublishSkill(
    {
      binaryPath,
      home,
    },
    {
      command: "--origin https://failed.example",
    },
    pluginRoot,
  );

  assert.match(text, /linkclaw publish failed after bundle generation/);
  assert.match(text, /manifest: .*manifest\.json/);
  assert.match(text, /FAIL did-canonical-id/);
  assert.match(text, /https:\/\/failed\.example/);
});
