import test from "node:test";
import assert from "node:assert/strict";
import { mkdtemp } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";

import { runLinkClaw } from "../src/bridge.ts";
import { runImportCommand, runShareCommand } from "../src/commands.ts";
import {
  buildLinkClawBinary,
  createResolverFixtureServer,
  pluginRoot,
} from "./helpers.ts";

let binaryPath = "";

test.before(async () => {
  binaryPath = await buildLinkClawBinary();
});

test("runImportCommand imports a contact and summarizes the result", async () => {
  const fixture = await createResolverFixtureServer();
  const home = await mkdtemp(join(tmpdir(), "linkclaw-home-"));

  try {
    await runLinkClaw(
      { binaryPath, home },
      {
        command: "init",
        canonicalId: "did:web:self.example",
        displayName: "Self Example",
      },
      pluginRoot,
    );

    const result = await runImportCommand(
      { binaryPath, home },
      `${fixture.origin}/.well-known/agent-card.json`,
      pluginRoot,
    );

    assert.equal(result.type, "message");
    assert.match(result.message, /linkclaw import completed/);
    assert.match(result.message, /status: consistent/);
    assert.match(result.message, /contact:/);
  } finally {
    await fixture.close();
  }
});

test("runImportCommand returns usage when no input is provided", async () => {
  const result = await runImportCommand({ binaryPath }, "", pluginRoot);
  assert.equal(result.type, "message");
  assert.match(result.message, /Usage: \/linkclaw-import/);
});

test("runShareCommand reports published share links for the configured origin", async () => {
  const fixture = await createResolverFixtureServer();

  try {
    const result = await runShareCommand(
      {
        binaryPath,
        publishOrigin: fixture.origin,
      },
      "",
      pluginRoot,
    );

    assert.equal(result.type, "message");
    assert.match(result.message, /LinkClaw identity bundle ready to share/);
    assert.match(result.message, /agent card:/);
    assert.match(result.message, /did\.json:/);
    assert.match(result.message, new RegExp(fixture.origin.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")));
  } finally {
    await fixture.close();
  }
});

test("runShareCommand requires an origin when publishOrigin is not configured", async () => {
  const result = await runShareCommand({ binaryPath }, "", pluginRoot);
  assert.equal(result.type, "message");
  assert.match(result.message, /publishOrigin/);
});
