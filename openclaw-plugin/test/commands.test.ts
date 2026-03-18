import assert from "node:assert/strict";
import { execFile } from "node:child_process";
import { mkdtemp, writeFile } from "node:fs/promises";
import { createServer } from "node:net";
import test from "node:test";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";

import { runLinkClaw } from "../src/bridge.ts";
import {
  runConnectCommand,
  runImportCommand,
  runInboxCommand,
  runMessageCommand,
  runShareCommand,
  runSyncCommand,
} from "../src/commands.ts";
import {
  buildLinkClawBinary,
  buildLinkClawRelayBinary,
  createResolverFixtureServer,
  pluginRoot,
} from "./helpers.ts";

let binaryPath = "";
let relayBinaryPath = "";

test.before(async () => {
  binaryPath = await buildLinkClawBinary();
  relayBinaryPath = await buildLinkClawRelayBinary();
});

async function reservePort(): Promise<number> {
  const server = createServer();
  await new Promise<void>((resolvePromise, rejectPromise) => {
    server.once("error", rejectPromise);
    server.listen(0, "127.0.0.1", () => resolvePromise());
  });
  const address = server.address();
  if (!address || typeof address === "string") {
    server.close();
    throw new Error("unable to allocate test port");
  }
  const { port } = address;
  await new Promise<void>((resolvePromise, rejectPromise) => {
    server.close((error) => {
      if (error) {
        rejectPromise(error);
        return;
      }
      resolvePromise();
    });
  });
  return port;
}

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
  assert.match(result.message, /Identity imported/);
  assert.match(result.message, /status: consistent/);
  assert.match(result.message, /contact:/);
  assert.match(result.message, /Next:/);
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

test("runConnectCommand imports an identity card into contacts", async () => {
  const aliceHome = await mkdtemp(join(tmpdir(), "linkclaw-alice-home-"));
  const bobHome = await mkdtemp(join(tmpdir(), "linkclaw-bob-home-"));

  await runLinkClaw(
    { binaryPath, home: aliceHome },
    {
      command: "init",
      canonicalId: "did:key:z6MkAlice",
      displayName: "Alice",
    },
    pluginRoot,
  );
  const exported = await runLinkClaw(
    { binaryPath, home: aliceHome },
    { command: "card_export" },
    pluginRoot,
  );
  const cardPath = join(bobHome, "alice.card.json");
  await writeFile(
    cardPath,
    JSON.stringify((exported.result as { card: unknown }).card, null, 2),
    "utf8",
  );

  await runLinkClaw(
    { binaryPath, home: bobHome },
    {
      command: "init",
      canonicalId: "did:key:z6MkBob",
      displayName: "Bob",
    },
    pluginRoot,
  );

  const result = await runConnectCommand(
    { binaryPath, home: bobHome },
    cardPath,
    pluginRoot,
  );

  assert.equal(result.type, "message");
  assert.match(result.message, /Contact added/);
  assert.match(result.message, /name: Alice/);
  assert.match(result.message, /\/linkclaw-message Alice <text>/);
});

test("runMessageCommand and inbox/sync summarize messaging workflows", async () => {
  const relayPort = await reservePort();
  const relayDir = await mkdtemp(join(tmpdir(), "linkclaw-relay-"));
  const relayDb = join(relayDir, "relay.db");
  const relayProc = execFile(relayBinaryPath, ["--db", relayDb, "--listen", `127.0.0.1:${relayPort}`], {
    cwd: resolve(pluginRoot, ".."),
    env: { ...process.env },
  });

  const aliceHome = await mkdtemp(join(tmpdir(), "linkclaw-alice-home-"));
  const bobHome = await mkdtemp(join(tmpdir(), "linkclaw-bob-home-"));

  try {
    await new Promise((resolvePromise) => setTimeout(resolvePromise, 1000));
    const relayConfig = { binaryPath, relayUrl: `http://127.0.0.1:${relayPort}` };

    await runLinkClaw(
      { ...relayConfig, home: aliceHome },
      {
        command: "init",
        canonicalId: "did:key:z6MkAlice",
        displayName: "Alice",
      },
      pluginRoot,
    );
    const aliceCard = await runLinkClaw(
      { ...relayConfig, home: aliceHome },
      { command: "card_export" },
      pluginRoot,
    );
    const aliceCardPath = join(bobHome, "alice.card.json");
    await writeFile(
      aliceCardPath,
      JSON.stringify((aliceCard.result as { card: unknown }).card, null, 2),
      "utf8",
    );

    await runLinkClaw(
      { ...relayConfig, home: bobHome },
      {
        command: "init",
        canonicalId: "did:key:z6MkBob",
        displayName: "Bob",
      },
      pluginRoot,
    );
    const bobCard = await runLinkClaw(
      { ...relayConfig, home: bobHome },
      { command: "card_export" },
      pluginRoot,
    );
    const bobCardPath = join(aliceHome, "bob.card.json");
    await writeFile(
      bobCardPath,
      JSON.stringify((bobCard.result as { card: unknown }).card, null, 2),
      "utf8",
    );

    const bobImportsAlice = await runConnectCommand(
      { ...relayConfig, home: bobHome },
      aliceCardPath,
      pluginRoot,
    );
    assert.match(bobImportsAlice.message, /contact:/);
    const contactMatch = bobImportsAlice.message.match(/contact: ([^\n]+)/);
    assert.ok(contactMatch);

    await runConnectCommand(
      { ...relayConfig, home: aliceHome },
      bobCardPath,
      pluginRoot,
    );

    const sendResult = await runMessageCommand(
      { ...relayConfig, home: bobHome },
      `${contactMatch[1]} hello from bob`,
      pluginRoot,
    );
    assert.equal(sendResult.type, "message");
    assert.match(sendResult.message, /Message queued for relay delivery/);

    const syncResult = await runSyncCommand(
      { ...relayConfig, home: aliceHome },
      "",
      pluginRoot,
    );
    assert.equal(syncResult.type, "message");
    assert.match(syncResult.message, /LinkClaw sync completed/);
    assert.match(syncResult.message, /\/linkclaw-inbox/);

    const inboxResult = await runInboxCommand(
      { ...relayConfig, home: aliceHome },
      "",
      pluginRoot,
    );
    assert.equal(inboxResult.type, "message");
    assert.match(inboxResult.message, /LinkClaw inbox/);
    assert.match(inboxResult.message, /did:key:z6MkBob/);
    assert.match(inboxResult.message, /\/linkclaw-message <contact> <text>/);
  } finally {
    relayProc.kill();
  }
});

test("runInboxCommand marks discovered senders as new sender", async () => {
  const relayPort = await reservePort();
  const relayDir = await mkdtemp(join(tmpdir(), "linkclaw-relay-"));
  const relayDb = join(relayDir, "relay.db");
  const relayProc = execFile(relayBinaryPath, ["--db", relayDb, "--listen", `127.0.0.1:${relayPort}`], {
    cwd: resolve(pluginRoot, ".."),
    env: { ...process.env },
  });

  const aliceHome = await mkdtemp(join(tmpdir(), "linkclaw-alice-home-"));
  const bobHome = await mkdtemp(join(tmpdir(), "linkclaw-bob-home-"));

  try {
    await new Promise((resolvePromise) => setTimeout(resolvePromise, 1000));
    const relayConfig = { binaryPath, relayUrl: `http://127.0.0.1:${relayPort}` };

    await runLinkClaw(
      { ...relayConfig, home: aliceHome },
      {
        command: "init",
        canonicalId: "did:key:z6MkAlice",
        displayName: "Alice",
      },
      pluginRoot,
    );
    const aliceCard = await runLinkClaw(
      { ...relayConfig, home: aliceHome },
      { command: "card_export" },
      pluginRoot,
    );
    const aliceCardPath = join(bobHome, "alice.card.json");
    await writeFile(
      aliceCardPath,
      JSON.stringify((aliceCard.result as { card: Record<string, unknown> }).card, null, 2),
      "utf8",
    );

    await runLinkClaw(
      { ...relayConfig, home: bobHome },
      {
        command: "init",
        canonicalId: "did:key:z6MkBob",
        displayName: "Bob",
      },
      pluginRoot,
    );
    await runLinkClaw(
      { ...relayConfig, home: bobHome },
      { command: "card_export" },
      pluginRoot,
    );
    const connectResult = await runConnectCommand(
      { ...relayConfig, home: bobHome },
      aliceCardPath,
      pluginRoot,
    );
    assert.match(connectResult.message, /name: Alice/);
    const sendResult = await runMessageCommand(
      { ...relayConfig, home: bobHome },
      "Alice hello stranger",
      pluginRoot,
    );
    assert.match(sendResult.message, /Message queued for relay delivery/);

    let syncResult = await runSyncCommand(
      { ...relayConfig, home: aliceHome },
      "",
      pluginRoot,
    );
    for (let attempt = 0; attempt < 3 && !/synced: 1/.test(syncResult.message); attempt += 1) {
      await new Promise((resolvePromise) => setTimeout(resolvePromise, 250));
      syncResult = await runSyncCommand(
        { ...relayConfig, home: aliceHome },
        "",
        pluginRoot,
      );
    }
    assert.match(syncResult.message, /synced: 1/);

    const inboxResult = await runInboxCommand(
      { ...relayConfig, home: aliceHome },
      "",
      pluginRoot,
    );
    assert.match(inboxResult.message, /new sender/);
    assert.match(inboxResult.message, /did:key:z6MkBob/);
    assert.match(inboxResult.message, /\/linkclaw-connect <card>/);
  } finally {
    relayProc.kill();
  }
});
