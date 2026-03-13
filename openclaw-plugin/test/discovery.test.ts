import test from "node:test";
import assert from "node:assert/strict";
import { mkdtemp } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";

import { runLinkClaw } from "../src/bridge.ts";
import {
  appendDIDLinkToText,
  extractIdentityArtifactURLs,
  handlePassiveDiscovery,
} from "../src/discovery.ts";
import {
  buildLinkClawBinary,
  createResolverFixtureServer,
  pluginRoot,
} from "./helpers.ts";

let binaryPath = "";

test.before(async () => {
  binaryPath = await buildLinkClawBinary();
});

test("extractIdentityArtifactURLs keeps explicit did/card urls and trims punctuation", () => {
  const urls = extractIdentityArtifactURLs(
    "see https://agent.example/.well-known/did.json, and (https://agent.example/.well-known/agent-card.json).",
  );

  assert.deepEqual(urls, [
    "https://agent.example/.well-known/did.json",
    "https://agent.example/.well-known/agent-card.json",
  ]);
});

test("appendDIDLinkToText appends the did url when agent-card is present", () => {
  const content = appendDIDLinkToText(
    "share https://agent.example/.well-known/agent-card.json",
    "https://agent.example",
  );

  assert.match(content, /did\.json: https:\/\/agent\.example\/\.well-known\/did\.json/);
});

test("passive discovery hook prompts for unknown identities", async () => {
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

    const event = {
      context: {
        content: `check ${fixture.origin}/.well-known/agent-card.json`,
      },
      messages: [] as Array<{ role: "assistant" | "system" | "user"; content: string }>,
    };

    await handlePassiveDiscovery({ binaryPath, home }, event, pluginRoot);

    assert.equal(event.messages.length, 1);
    assert.match(event.messages[0].content, /LinkClaw identity link detected/);
    assert.match(event.messages[0].content, /did:web:fixture\.example/);
    assert.match(event.messages[0].content, /\/linkclaw-import /);
  } finally {
    await fixture.close();
  }
});

test("passive discovery hook skips contacts already present in known", async () => {
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
    await runLinkClaw(
      { binaryPath, home },
      {
        command: "import",
        input: `${fixture.origin}/.well-known/agent-card.json`,
      },
      pluginRoot,
    );

    const event = {
      context: {
        content: `known ${fixture.origin}/.well-known/agent-card.json`,
      },
      messages: [] as Array<{ role: "assistant" | "system" | "user"; content: string }>,
    };

    await handlePassiveDiscovery({ binaryPath, home }, event, pluginRoot);

    assert.equal(event.messages.length, 0);
  } finally {
    await fixture.close();
  }
});
