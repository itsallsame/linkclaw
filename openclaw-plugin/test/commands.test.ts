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
  runContactsCommand,
  runDiscoverCommand,
  runFindCommand,
  runImportCommand,
  runInboxCommand,
  runInspectCommand,
  runMessageCommand,
  runOnboardingCommand,
  runReplyCommand,
  runSetupCommand,
  runShareCommand,
  runStatusCommand,
  runSyncCommand,
  runThreadCommand,
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

test("runInspectCommand verifies one identity surface and reports import readiness", async () => {
  const fixture = await createResolverFixtureServer();

  try {
    const result = await runInspectCommand(
      { binaryPath },
      `${fixture.origin}/.well-known/agent-card.json`,
      pluginRoot,
    );

    assert.equal(result.type, "message");
    assert.match(result.message, /LinkClaw inspect/);
    assert.match(result.message, /status: consistent/);
    assert.match(result.message, /importable: yes/);
    assert.match(result.message, /Next:/);
    assert.match(result.message, /\/linkclaw-import /);
  } finally {
    await fixture.close();
  }
});

test("runInspectCommand returns usage when no input is provided", async () => {
  const result = await runInspectCommand({ binaryPath }, "", pluginRoot);
  assert.equal(result.type, "message");
  assert.match(result.message, /Usage: \/linkclaw-inspect/);
});

test("runDiscoverCommand lists discovery records from runtime", async () => {
  const home = await mkdtemp(join(tmpdir(), "linkclaw-discovery-home-"));
  await runLinkClaw(
    { binaryPath, home },
    {
      command: "init",
      canonicalId: "did:key:z6MkDiscoverSelf",
      displayName: "Discovery Self",
    },
    pluginRoot,
  );

  const result = await runDiscoverCommand(
    { binaryPath, home },
    "--fresh-only --limit 5",
    pluginRoot,
  );

  assert.equal(result.type, "message");
  assert.match(result.message, /LinkClaw discovery/);
  assert.match(result.message, /records: \d+/);
  assert.match(result.message, /fresh only: yes/);
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

test("runShareCommand can emit a signed identity card directly", async () => {
  const home = await mkdtemp(join(tmpdir(), "linkclaw-share-card-home-"));
  await runLinkClaw(
    { binaryPath, home },
    {
      command: "init",
      canonicalId: "did:key:z6MkShareCard",
      displayName: "Share Card",
    },
    pluginRoot,
  );

  const result = await runShareCommand(
    { binaryPath, home },
    "--card",
    pluginRoot,
  );

  assert.equal(result.type, "message");
  assert.match(result.message, /LinkClaw identity card ready to share/);
  assert.match(result.message, /card compact:/);
  assert.match(result.message, /--- card-compact-begin ---/);
  assert.match(result.message, /--- card-compact-end ---/);
  assert.match(result.message, /"schema_version": "linkclaw\.identity_card\.v1"/);
  assert.match(result.message, /did:key:z6MkShareCard/);
  assert.match(result.message, /\/linkclaw-connect <card-json>/);
  assert.match(result.message, /--- connect-command-begin ---/);
  assert.match(result.message, /--- connect-command-end ---/);
  assert.match(result.message, /\/linkclaw-connect '/);
});

test("runShareCommand falls back to LINKCLAW_RELAY_URL when relayUrl config is omitted", async () => {
  const home = await mkdtemp(join(tmpdir(), "linkclaw-share-card-relay-home-"));
  await runLinkClaw(
    { binaryPath, home },
    {
      command: "init",
      canonicalId: "did:key:z6MkShareRelay",
      displayName: "Share Relay",
    },
    pluginRoot,
  );

  const previousRelay = process.env.LINKCLAW_RELAY_URL;
  process.env.LINKCLAW_RELAY_URL = "http://127.0.0.1:8788";
  try {
    const result = await runShareCommand(
      { binaryPath, home },
      "--card",
      pluginRoot,
    );

    assert.equal(result.type, "message");
    assert.match(result.message, /"relay_url":"http:\/\/127\.0\.0\.1:8788"/);
  } finally {
    if (previousRelay === undefined) {
      delete process.env.LINKCLAW_RELAY_URL;
    } else {
      process.env.LINKCLAW_RELAY_URL = previousRelay;
    }
  }
});

test("runShareCommand requires an origin when publishOrigin is not configured", async () => {
  const result = await runShareCommand({ binaryPath }, "", pluginRoot);
  assert.equal(result.type, "message");
  assert.match(result.message, /publishOrigin/);
  assert.match(result.message, /\/linkclaw-share --card/);
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
  assert.match(result.message, /LinkClaw contact saved/);
  assert.match(result.message, /name: Alice/);
  assert.match(result.message, /--- contact-summary-begin ---/);
  assert.match(result.message, /--- contact-summary-end ---/);
  assert.match(result.message, /\/linkclaw-message did:key:z6MkAlice <text>/);
  assert.match(result.message, /\/linkclaw-thread did:key:z6MkAlice/);
  assert.match(result.message, /\/linkclaw-share --card/);
});

test("runConnectCommand accepts card export JSON envelope files directly", async () => {
  const aliceHome = await mkdtemp(join(tmpdir(), "linkclaw-alice-home-"));
  const bobHome = await mkdtemp(join(tmpdir(), "linkclaw-bob-home-"));

  await runLinkClaw(
    { binaryPath, home: aliceHome },
    {
      command: "init",
      canonicalId: "did:key:z6MkAliceEnvelope",
      displayName: "Alice Envelope",
    },
    pluginRoot,
  );
  const exported = await runLinkClaw(
    { binaryPath, home: aliceHome },
    { command: "card_export" },
    pluginRoot,
  );

  await runLinkClaw(
    { binaryPath, home: bobHome },
    {
      command: "init",
      canonicalId: "did:key:z6MkBobEnvelope",
      displayName: "Bob Envelope",
    },
    pluginRoot,
  );

  const envelopePath = join(bobHome, "alice.export-envelope.json");
  await writeFile(envelopePath, JSON.stringify(exported, null, 2), "utf8");

  const result = await runConnectCommand(
    { binaryPath, home: bobHome },
    envelopePath,
    pluginRoot,
  );

  assert.equal(result.type, "message");
  assert.match(result.message, /LinkClaw contact saved/);
  assert.match(result.message, /Alice Envelope/);
});

test("runConnectCommand accepts identity cards pasted as fenced json blocks", async () => {
  const aliceHome = await mkdtemp(join(tmpdir(), "linkclaw-alice-fenced-home-"));
  const bobHome = await mkdtemp(join(tmpdir(), "linkclaw-bob-fenced-home-"));

  await runLinkClaw(
    { binaryPath, home: aliceHome },
    {
      command: "init",
      canonicalId: "did:key:z6MkAliceFenced",
      displayName: "Alice Fenced",
    },
    pluginRoot,
  );
  const exported = await runLinkClaw(
    { binaryPath, home: aliceHome },
    { command: "card_export" },
    pluginRoot,
  );

  await runLinkClaw(
    { binaryPath, home: bobHome },
    {
      command: "init",
      canonicalId: "did:key:z6MkBobFenced",
      displayName: "Bob Fenced",
    },
    pluginRoot,
  );

  const rawCard = JSON.stringify((exported.result as { card: unknown }).card, null, 2);
  const fencedCard = `\`\`\`json\n${rawCard}\n\`\`\``;
  const result = await runConnectCommand(
    { binaryPath, home: bobHome },
    fencedCard,
    pluginRoot,
  );

  assert.equal(result.type, "message");
  assert.match(result.message, /LinkClaw contact saved/);
  assert.match(result.message, /Alice Fenced/);
});

test("runContactsCommand lists imported contacts", async () => {
  const aliceHome = await mkdtemp(join(tmpdir(), "linkclaw-alice-home-"));
  const bobHome = await mkdtemp(join(tmpdir(), "linkclaw-bob-home-"));

  await runLinkClaw(
    { binaryPath, home: aliceHome },
    {
      command: "init",
      canonicalId: "did:key:z6MkAliceContacts",
      displayName: "Alice Contacts",
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
      canonicalId: "did:key:z6MkBobContacts",
      displayName: "Bob Contacts",
    },
    pluginRoot,
  );
  await runConnectCommand(
    { binaryPath, home: bobHome },
    cardPath,
    pluginRoot,
  );

  const result = await runContactsCommand(
    { binaryPath, home: bobHome },
    "",
    pluginRoot,
  );

  assert.equal(result.type, "message");
  assert.match(result.message, /LinkClaw contacts/);
  assert.match(result.message, /Alice Contacts/);
  assert.match(result.message, /did:key:z6MkAliceContacts/);
  assert.match(result.message, /trust=unknown/);
  assert.match(result.message, /--- contacts-list-begin ---/);
  assert.match(result.message, /--- contacts-list-end ---/);
});

test("runContactsCommand filters saved contacts by query", async () => {
  const aliceHome = await mkdtemp(join(tmpdir(), "linkclaw-alice-contacts-filter-home-"));
  const bobHome = await mkdtemp(join(tmpdir(), "linkclaw-bob-contacts-filter-home-"));
  const carolHome = await mkdtemp(join(tmpdir(), "linkclaw-carol-contacts-filter-home-"));

  await runLinkClaw(
    { binaryPath, home: aliceHome },
    {
      command: "init",
      canonicalId: "did:key:z6MkAliceContactsFilter",
      displayName: "Alice Contact",
    },
    pluginRoot,
  );
  await runLinkClaw(
    { binaryPath, home: carolHome },
    {
      command: "init",
      canonicalId: "did:key:z6MkCarolContactsFilter",
      displayName: "Carol Contact",
    },
    pluginRoot,
  );
  await runLinkClaw(
    { binaryPath, home: bobHome },
    {
      command: "init",
      canonicalId: "did:key:z6MkBobContactsFilter",
      displayName: "Bob Contact",
    },
    pluginRoot,
  );

  const aliceCard = await runLinkClaw(
    { binaryPath, home: aliceHome },
    { command: "card_export" },
    pluginRoot,
  );
  const carolCard = await runLinkClaw(
    { binaryPath, home: carolHome },
    { command: "card_export" },
    pluginRoot,
  );
  const aliceCardPath = join(bobHome, "alice-contacts-filter.card.json");
  const carolCardPath = join(bobHome, "carol-contacts-filter.card.json");
  await writeFile(
    aliceCardPath,
    JSON.stringify((aliceCard.result as { card: unknown }).card, null, 2),
    "utf8",
  );
  await writeFile(
    carolCardPath,
    JSON.stringify((carolCard.result as { card: unknown }).card, null, 2),
    "utf8",
  );

  await runConnectCommand(
    { binaryPath, home: bobHome },
    aliceCardPath,
    pluginRoot,
  );
  await runConnectCommand(
    { binaryPath, home: bobHome },
    carolCardPath,
    pluginRoot,
  );

  const result = await runContactsCommand(
    { binaryPath, home: bobHome },
    "alice",
    pluginRoot,
  );

  assert.equal(result.type, "message");
  assert.match(result.message, /LinkClaw contacts/);
  assert.match(result.message, /query: alice/);
  assert.match(result.message, /Alice Contact/);
  assert.doesNotMatch(result.message, /Carol Contact/);
});

test("runFindCommand filters saved contacts and prints ready-to-run commands", async () => {
  const aliceHome = await mkdtemp(join(tmpdir(), "linkclaw-alice-find-home-"));
  const bobHome = await mkdtemp(join(tmpdir(), "linkclaw-bob-find-home-"));

  await runLinkClaw(
    { binaryPath, home: aliceHome },
    {
      command: "init",
      canonicalId: "did:key:z6MkAliceFinder",
      displayName: "Alice Finder",
    },
    pluginRoot,
  );
  const exported = await runLinkClaw(
    { binaryPath, home: aliceHome },
    { command: "card_export" },
    pluginRoot,
  );
  const cardPath = join(bobHome, "alice-find.card.json");
  await writeFile(
    cardPath,
    JSON.stringify((exported.result as { card: unknown }).card, null, 2),
    "utf8",
  );

  await runLinkClaw(
    { binaryPath, home: bobHome },
    {
      command: "init",
      canonicalId: "did:key:z6MkBobFinder",
      displayName: "Bob Finder",
    },
    pluginRoot,
  );
  await runConnectCommand(
    { binaryPath, home: bobHome },
    cardPath,
    pluginRoot,
  );

  const result = await runFindCommand(
    { binaryPath, home: bobHome },
    "finder",
    pluginRoot,
  );

  assert.equal(result.type, "message");
  assert.match(result.message, /LinkClaw contact search/);
  assert.match(result.message, /matches: 1/);
  assert.match(result.message, /Alice Finder/);
  assert.match(result.message, /\/linkclaw-message did:key:z6MkAliceFinder <text>/);
  assert.match(result.message, /\/linkclaw-thread did:key:z6MkAliceFinder/);
});

test("runSetupCommand initializes an uninitialized home", async () => {
  const home = await mkdtemp(join(tmpdir(), "linkclaw-setup-home-"));

  const result = await runSetupCommand(
    { binaryPath, home },
    "--display-name Setup",
    pluginRoot,
  );

  assert.equal(result.type, "message");
  assert.match(result.message, /LinkClaw 初始化完成/);
  assert.match(result.message, /canonical id: did:key:z/);
  assert.match(result.message, /检查项：/);
  assert.match(result.message, /--- health-checks-begin ---/);
  assert.match(result.message, /--- health-checks-end ---/);
  assert.match(result.message, /binary: 正常/);
  assert.match(result.message, /offline recovery: not configured/);
  assert.match(result.message, /publish origin: not configured/);
});

test("runOnboardingCommand defaults to a readiness check when no args are provided", async () => {
  const home = await mkdtemp(join(tmpdir(), "linkclaw-onboarding-home-"));

  const result = await runOnboardingCommand(
    { binaryPath, home },
    "",
    pluginRoot,
  );

  assert.equal(result.type, "message");
  assert.match(result.message, /LinkClaw 首次引导/);
  assert.match(result.message, /LinkClaw 检查完成/);
  assert.match(result.message, /状态：未初始化/);
  assert.match(result.message, /offline recovery: not configured/);
});

test("runSetupCommand can locate the binary through LINKCLAW_BINARY", async () => {
  const home = await mkdtemp(join(tmpdir(), "linkclaw-setup-env-binary-home-"));
  const previousBinary = process.env.LINKCLAW_BINARY;
  process.env.LINKCLAW_BINARY = binaryPath;
  try {
    const result = await runSetupCommand(
      { home },
      "--display-name EnvBinary",
      pluginRoot,
    );

    assert.equal(result.type, "message");
    assert.match(result.message, /LinkClaw 初始化完成/);
    assert.match(result.message, /binary: 正常/);
  } finally {
    if (previousBinary === undefined) {
      delete process.env.LINKCLAW_BINARY;
    } else {
      process.env.LINKCLAW_BINARY = previousBinary;
    }
  }
});

test("runSetupCommand supports check-only mode before initialization", async () => {
  const home = await mkdtemp(join(tmpdir(), "linkclaw-setup-check-only-home-"));

  const result = await runSetupCommand(
    { binaryPath, home },
    "--check-only",
    pluginRoot,
  );

  assert.equal(result.type, "message");
  assert.match(result.message, /LinkClaw 检查完成/);
  assert.match(result.message, /状态：未初始化/);
  assert.match(result.message, /--- health-checks-begin ---/);
  assert.match(result.message, /--- health-checks-end ---/);
  assert.match(result.message, /binary: 正常/);
  assert.match(result.message, /offline recovery: not configured/);
  assert.match(result.message, /运行 \/linkclaw-setup --display-name <name>/);
});

test("runSetupCommand reports relay reachability when configured", async () => {
  const relayPort = await reservePort();
  const relayDir = await mkdtemp(join(tmpdir(), "linkclaw-relay-"));
  const relayDb = join(relayDir, "relay.db");
  const relayProc = execFile(relayBinaryPath, ["--db", relayDb, "--listen", `127.0.0.1:${relayPort}`], {
    cwd: resolve(pluginRoot, ".."),
    env: { ...process.env },
  });
  const home = await mkdtemp(join(tmpdir(), "linkclaw-setup-relay-home-"));

  try {
    await new Promise((resolvePromise) => setTimeout(resolvePromise, 1000));
    const result = await runSetupCommand(
      { binaryPath, home, relayUrl: `http://127.0.0.1:${relayPort}` },
      "--display-name SetupRelay",
      pluginRoot,
    );

    assert.equal(result.type, "message");
    assert.match(result.message, /LinkClaw 初始化完成/);
    assert.match(result.message, /offline recovery: ok \(404\) http:\/\/127\.0\.0\.1:/);
    assert.match(result.message, /publish origin: not configured/);
  } finally {
    relayProc.kill();
  }
});

test("runSetupCommand reports publish origin readiness when configured", async () => {
  const fixture = await createResolverFixtureServer();
  const home = await mkdtemp(join(tmpdir(), "linkclaw-setup-publish-home-"));

  try {
    const result = await runSetupCommand(
      { binaryPath, home, publishOrigin: fixture.origin },
      "--display-name SetupPublish",
      pluginRoot,
    );

    assert.equal(result.type, "message");
    assert.match(result.message, /LinkClaw 初始化完成/);
    assert.match(result.message, new RegExp(`publish origin: ok ${fixture.origin.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")}`));
  } finally {
    await fixture.close();
  }
});

test("runSetupCommand reports ready state in check-only mode after initialization", async () => {
  const home = await mkdtemp(join(tmpdir(), "linkclaw-setup-check-ready-home-"));

  await runLinkClaw(
    { binaryPath, home },
    {
      command: "init",
      canonicalId: "did:key:z6MkSetupCheckReady",
      displayName: "SetupCheckReady",
    },
    pluginRoot,
  );

  const result = await runSetupCommand(
    { binaryPath, home },
    "--check-only",
    pluginRoot,
  );

  assert.equal(result.type, "message");
  assert.match(result.message, /LinkClaw 已就绪/);
  assert.match(result.message, /contacts: 0/);
});

test("runStatusCommand summarizes health and local state after initialization", async () => {
  const relayPort = await reservePort();
  const relayDir = await mkdtemp(join(tmpdir(), "linkclaw-relay-"));
  const relayDb = join(relayDir, "relay.db");
  const relayProc = execFile(relayBinaryPath, ["--db", relayDb, "--listen", `127.0.0.1:${relayPort}`], {
    cwd: resolve(pluginRoot, ".."),
    env: { ...process.env },
  });
  const home = await mkdtemp(join(tmpdir(), "linkclaw-status-home-"));

  try {
    await new Promise((resolvePromise) => setTimeout(resolvePromise, 1000));
    await runLinkClaw(
      { binaryPath, home },
      {
        command: "init",
        canonicalId: "did:key:z6MkStatus",
        displayName: "Status",
      },
      pluginRoot,
    );

    const result = await runStatusCommand(
      { binaryPath, home, relayUrl: `http://127.0.0.1:${relayPort}` },
      "",
      pluginRoot,
    );

    assert.equal(result.type, "message");
    assert.match(result.message, /LinkClaw status/);
    assert.match(result.message, /state: ready/);
    assert.match(result.message, /--- status-summary-begin ---/);
    assert.match(result.message, /--- status-summary-end ---/);
    assert.match(result.message, /--- health-checks-begin ---/);
    assert.match(result.message, /--- health-checks-end ---/);
    assert.match(result.message, /contacts: 0/);
    assert.match(result.message, /conversations: 0/);
    assert.match(result.message, /unread: 0/);
    assert.match(result.message, /messaging: ready/);
    assert.match(result.message, /identity ready: yes/);
    assert.match(result.message, /transport ready: yes/);
    assert.match(result.message, /discovery ready: no/);
    assert.match(result.message, /queued outgoing: 0/);
    assert.match(result.message, /message status: direct=0 deferred=0 recovered=0/);
    assert.match(result.message, /offline recovery: not configured/);
    assert.match(result.message, /runtime mode: host-managed/);
    assert.match(result.message, /offline recovery: ok \(404\) http:\/\/127\.0\.0\.1:/);
  } finally {
    relayProc.kill();
  }
});

test("runStatusCommand reports not initialized state", async () => {
  const home = await mkdtemp(join(tmpdir(), "linkclaw-status-empty-home-"));

  const result = await runStatusCommand(
    { binaryPath, home },
    "",
    pluginRoot,
  );

  assert.equal(result.type, "message");
  assert.match(result.message, /LinkClaw 状态/);
  assert.match(result.message, /状态：未初始化/);
  assert.match(result.message, /--- health-checks-begin ---/);
  assert.match(result.message, /--- health-checks-end ---/);
  assert.match(result.message, /binary: 正常/);
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
    assert.match(sendResult.message, /LinkClaw message queued/);
    assert.match(sendResult.message, /transport status: deferred/);

    const syncResult = await runSyncCommand(
      { ...relayConfig, home: aliceHome },
      "",
      pluginRoot,
    );
    assert.equal(syncResult.type, "message");
    assert.match(syncResult.message, /LinkClaw sync completed/);
    assert.match(syncResult.message, /recovery checks: \d+/);
    assert.doesNotMatch(syncResult.message, /relay calls/i);
    assert.match(syncResult.message, /\/linkclaw-inbox/);

    const inboxResult = await runInboxCommand(
      { ...relayConfig, home: aliceHome },
      "",
      pluginRoot,
    );
    assert.equal(inboxResult.type, "message");
    assert.match(inboxResult.message, /LinkClaw inbox/);
    assert.match(inboxResult.message, /did:key:z6MkBob/);
    assert.match(inboxResult.message, /--- inbox-conversations-begin ---/);
    assert.match(inboxResult.message, /--- inbox-conversations-end ---/);
    assert.match(inboxResult.message, /\/linkclaw-thread <contact>/);
    assert.match(inboxResult.message, /\/linkclaw-reply <contact> <text>/);

    const threadResult = await runThreadCommand(
      { ...relayConfig, home: aliceHome },
      "did:key:z6MkBob",
      pluginRoot,
    );
    assert.equal(threadResult.type, "message");
    assert.match(threadResult.message, /LinkClaw thread/);
    assert.match(threadResult.message, /hello from bob/);
    assert.match(threadResult.message, /--- thread-messages-begin ---/);
    assert.match(threadResult.message, /--- thread-messages-end ---/);
    assert.match(threadResult.message, /\/linkclaw-reply did:key:z6MkBob <text>/);

    const inboxAfterThread = await runInboxCommand(
      { ...relayConfig, home: aliceHome },
      "",
      pluginRoot,
    );
    assert.match(inboxAfterThread.message, /unread=0/);
  } finally {
    relayProc.kill();
  }
});

test("runStatusCommand reflects offline recovery state after sync", async () => {
  const relayPort = await reservePort();
  const relayDir = await mkdtemp(join(tmpdir(), "linkclaw-relay-"));
  const relayDb = join(relayDir, "relay.db");
  const relayProc = execFile(relayBinaryPath, ["--db", relayDb, "--listen", `127.0.0.1:${relayPort}`], {
    cwd: resolve(pluginRoot, ".."),
    env: { ...process.env },
  });

  const aliceHome = await mkdtemp(join(tmpdir(), "linkclaw-alice-status-home-"));
  const bobHome = await mkdtemp(join(tmpdir(), "linkclaw-bob-status-home-"));

  try {
    await new Promise((resolvePromise) => setTimeout(resolvePromise, 1000));
    const relayConfig = { binaryPath, relayUrl: `http://127.0.0.1:${relayPort}` };

    await runLinkClaw(
      { ...relayConfig, home: aliceHome },
      { command: "init", canonicalId: "did:key:z6MkAliceStatusFlow", displayName: "Alice Status Flow" },
      pluginRoot,
    );
    await runLinkClaw(
      { ...relayConfig, home: bobHome },
      { command: "init", canonicalId: "did:key:z6MkBobStatusFlow", displayName: "Bob Status Flow" },
      pluginRoot,
    );

    const aliceCard = await runLinkClaw(
      { ...relayConfig, home: aliceHome },
      { command: "card_export" },
      pluginRoot,
    );
    const bobCard = await runLinkClaw(
      { ...relayConfig, home: bobHome },
      { command: "card_export" },
      pluginRoot,
    );
    const aliceCardPath = join(bobHome, "alice-status.card.json");
    const bobCardPath = join(aliceHome, "bob-status.card.json");
    await writeFile(aliceCardPath, JSON.stringify((aliceCard.result as { card: unknown }).card, null, 2), "utf8");
    await writeFile(bobCardPath, JSON.stringify((bobCard.result as { card: unknown }).card, null, 2), "utf8");

    const bobImportsAlice = await runConnectCommand(
      { ...relayConfig, home: bobHome },
      aliceCardPath,
      pluginRoot,
    );
    const contactMatch = bobImportsAlice.message.match(/contact: ([^\n]+)/);
    assert.ok(contactMatch);
    await runConnectCommand(
      { ...relayConfig, home: aliceHome },
      bobCardPath,
      pluginRoot,
    );

    await runMessageCommand(
      { ...relayConfig, home: bobHome },
      `${contactMatch[1]} hello for status`,
      pluginRoot,
    );
    await runSyncCommand(
      { ...relayConfig, home: aliceHome },
      "",
      pluginRoot,
    );

    const statusResult = await runStatusCommand(
      { ...relayConfig, home: aliceHome },
      "",
      pluginRoot,
    );
    assert.equal(statusResult.type, "message");
    assert.match(statusResult.message, /LinkClaw status/);
    assert.match(statusResult.message, /conversations: 1/);
    assert.match(statusResult.message, /unread: 1/);
    assert.match(statusResult.message, /message status: direct=0 deferred=0 recovered=1/);
    assert.match(statusResult.message, /offline recovery: ready \(1 path\)/);
    assert.match(statusResult.message, /discovery ready: no/);
    assert.match(statusResult.message, /runtime mode: host-managed/);
    assert.match(statusResult.message, /last recovery: .*recovered=1/);
    assert.match(statusResult.message, /recent route outcomes:/);
    assert.doesNotMatch(statusResult.message, /store_forward/i);
  } finally {
    relayProc.kill();
  }
});

test("runReplyCommand mirrors message send for an existing contact", async () => {
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
        canonicalId: "did:key:z6MkAliceReply",
        displayName: "Alice Reply",
      },
      pluginRoot,
    );
    await runLinkClaw(
      { ...relayConfig, home: bobHome },
      {
        command: "init",
        canonicalId: "did:key:z6MkBobReply",
        displayName: "Bob Reply",
      },
      pluginRoot,
    );

    const aliceCard = await runLinkClaw(
      { ...relayConfig, home: aliceHome },
      { command: "card_export" },
      pluginRoot,
    );
    const bobCard = await runLinkClaw(
      { ...relayConfig, home: bobHome },
      { command: "card_export" },
      pluginRoot,
    );
    const aliceCardPath = join(bobHome, "alice-reply.card.json");
    const bobCardPath = join(aliceHome, "bob-reply.card.json");
    await writeFile(
      aliceCardPath,
      JSON.stringify((aliceCard.result as { card: unknown }).card, null, 2),
      "utf8",
    );
    await writeFile(
      bobCardPath,
      JSON.stringify((bobCard.result as { card: unknown }).card, null, 2),
      "utf8",
    );

    await runConnectCommand(
      { ...relayConfig, home: bobHome },
      aliceCardPath,
      pluginRoot,
    );
    await runConnectCommand(
      { ...relayConfig, home: aliceHome },
      bobCardPath,
      pluginRoot,
    );

    const replyResult = await runReplyCommand(
      { ...relayConfig, home: aliceHome },
      "\"Bob Reply\" hello back",
      pluginRoot,
    );
    assert.equal(replyResult.type, "message");
    assert.match(replyResult.message, /LinkClaw message queued/);
  } finally {
    relayProc.kill();
  }
});

test("runReplyCommand can target the most recent known conversation when contact is omitted", async () => {
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
        canonicalId: "did:key:z6MkAliceRecent",
        displayName: "Alice Recent",
      },
      pluginRoot,
    );
    await runLinkClaw(
      { ...relayConfig, home: bobHome },
      {
        command: "init",
        canonicalId: "did:key:z6MkBobRecent",
        displayName: "Bob Recent",
      },
      pluginRoot,
    );

    const aliceCard = await runLinkClaw(
      { ...relayConfig, home: aliceHome },
      { command: "card_export" },
      pluginRoot,
    );
    const bobCard = await runLinkClaw(
      { ...relayConfig, home: bobHome },
      { command: "card_export" },
      pluginRoot,
    );
    const aliceCardPath = join(bobHome, "alice-recent.card.json");
    const bobCardPath = join(aliceHome, "bob-recent.card.json");
    await writeFile(
      aliceCardPath,
      JSON.stringify((aliceCard.result as { card: unknown }).card, null, 2),
      "utf8",
    );
    await writeFile(
      bobCardPath,
      JSON.stringify((bobCard.result as { card: unknown }).card, null, 2),
      "utf8",
    );

    await runConnectCommand(
      { ...relayConfig, home: bobHome },
      aliceCardPath,
      pluginRoot,
    );
    await runConnectCommand(
      { ...relayConfig, home: aliceHome },
      bobCardPath,
      pluginRoot,
    );

    await runMessageCommand(
      { ...relayConfig, home: bobHome },
      "\"Alice Recent\" hello first",
      pluginRoot,
    );
    await runSyncCommand(
      { ...relayConfig, home: aliceHome },
      "",
      pluginRoot,
    );

    const replyResult = await runReplyCommand(
      { ...relayConfig, home: aliceHome },
      "hello latest thread",
      pluginRoot,
    );
    assert.equal(replyResult.type, "message");
    assert.match(replyResult.message, /LinkClaw message queued/);
  } finally {
    relayProc.kill();
  }
});

test("runReplyCommand reports when no recent known conversation exists", async () => {
  const home = await mkdtemp(join(tmpdir(), "linkclaw-reply-empty-home-"));

  await runLinkClaw(
    { binaryPath, home },
    {
      command: "init",
      canonicalId: "did:key:z6MkReplyEmpty",
      displayName: "Reply Empty",
    },
    pluginRoot,
  );

  const result = await runReplyCommand(
    { binaryPath, home },
    "hello no thread",
    pluginRoot,
  );
  assert.equal(result.type, "message");
  assert.match(result.message, /no recent known conversation is available/);
});

test("runReplyCommand prefers the last opened thread over the most recent conversation", async () => {
  const relayPort = await reservePort();
  const relayDir = await mkdtemp(join(tmpdir(), "linkclaw-relay-"));
  const relayDb = join(relayDir, "relay.db");
  const relayProc = execFile(relayBinaryPath, ["--db", relayDb, "--listen", `127.0.0.1:${relayPort}`], {
    cwd: resolve(pluginRoot, ".."),
    env: { ...process.env },
  });

  const aliceHome = await mkdtemp(join(tmpdir(), "linkclaw-alice-home-"));
  const bobOneHome = await mkdtemp(join(tmpdir(), "linkclaw-bob-one-home-"));
  const bobTwoHome = await mkdtemp(join(tmpdir(), "linkclaw-bob-two-home-"));

  try {
    await new Promise((resolvePromise) => setTimeout(resolvePromise, 1000));
    const relayConfig = { binaryPath, relayUrl: `http://127.0.0.1:${relayPort}` };

    await runLinkClaw(
      { ...relayConfig, home: aliceHome },
      {
        command: "init",
        canonicalId: "did:key:z6MkAliceCtx",
        displayName: "Alice Ctx",
      },
      pluginRoot,
    );
    await runLinkClaw(
      { ...relayConfig, home: bobOneHome },
      {
        command: "init",
        canonicalId: "did:key:z6MkBobCtxOne",
        displayName: "Bob One",
      },
      pluginRoot,
    );
    await runLinkClaw(
      { ...relayConfig, home: bobTwoHome },
      {
        command: "init",
        canonicalId: "did:key:z6MkBobCtxTwo",
        displayName: "Bob Two",
      },
      pluginRoot,
    );

    const bobOneCard = await runLinkClaw(
      { ...relayConfig, home: bobOneHome },
      { command: "card_export" },
      pluginRoot,
    );
    const bobTwoCard = await runLinkClaw(
      { ...relayConfig, home: bobTwoHome },
      { command: "card_export" },
      pluginRoot,
    );
    const aliceCard = await runLinkClaw(
      { ...relayConfig, home: aliceHome },
      { command: "card_export" },
      pluginRoot,
    );

    const bobOneCardPath = join(aliceHome, "bob-one.card.json");
    const bobTwoCardPath = join(aliceHome, "bob-two.card.json");
    const aliceCardForBobOnePath = join(bobOneHome, "alice.card.json");
    const aliceCardForBobTwoPath = join(bobTwoHome, "alice.card.json");
    await writeFile(
      bobOneCardPath,
      JSON.stringify((bobOneCard.result as { card: unknown }).card, null, 2),
      "utf8",
    );
    await writeFile(
      bobTwoCardPath,
      JSON.stringify((bobTwoCard.result as { card: unknown }).card, null, 2),
      "utf8",
    );
    await writeFile(
      aliceCardForBobOnePath,
      JSON.stringify((aliceCard.result as { card: unknown }).card, null, 2),
      "utf8",
    );
    await writeFile(
      aliceCardForBobTwoPath,
      JSON.stringify((aliceCard.result as { card: unknown }).card, null, 2),
      "utf8",
    );

    await runConnectCommand(
      { ...relayConfig, home: aliceHome },
      bobOneCardPath,
      pluginRoot,
    );
    await runConnectCommand(
      { ...relayConfig, home: aliceHome },
      bobTwoCardPath,
      pluginRoot,
    );
    await runConnectCommand(
      { ...relayConfig, home: bobOneHome },
      aliceCardForBobOnePath,
      pluginRoot,
    );
    await runConnectCommand(
      { ...relayConfig, home: bobTwoHome },
      aliceCardForBobTwoPath,
      pluginRoot,
    );

    await runMessageCommand(
      { ...relayConfig, home: bobOneHome },
      "\"Alice Ctx\" hello from one",
      pluginRoot,
    );
    await runSyncCommand(
      { ...relayConfig, home: aliceHome },
      "",
      pluginRoot,
    );
    await new Promise((resolvePromise) => setTimeout(resolvePromise, 25));
    await runMessageCommand(
      { ...relayConfig, home: bobTwoHome },
      "\"Alice Ctx\" hello from two",
      pluginRoot,
    );
    await runSyncCommand(
      { ...relayConfig, home: aliceHome },
      "",
      pluginRoot,
    );

    await runThreadCommand(
      { ...relayConfig, home: aliceHome },
      "did:key:z6MkBobCtxOne",
      pluginRoot,
    );

    const replyResult = await runReplyCommand(
      { ...relayConfig, home: aliceHome },
      "hello from context",
      pluginRoot,
    );
    assert.equal(replyResult.type, "message");
    assert.match(replyResult.message, /LinkClaw message queued/);

    const bobOneSync = await runSyncCommand(
      { ...relayConfig, home: bobOneHome },
      "",
      pluginRoot,
    );
    const bobTwoSync = await runSyncCommand(
      { ...relayConfig, home: bobTwoHome },
      "",
      pluginRoot,
    );
    assert.match(bobOneSync.message, /synced: 1/);
    assert.match(bobTwoSync.message, /synced: 0/);
  } finally {
    relayProc.kill();
  }
});

test("runMessageCommand resolves a unique saved display name before sending", async () => {
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
        canonicalId: "did:key:z6MkAliceResolve",
        displayName: "Alice",
      },
      pluginRoot,
    );
    await runLinkClaw(
      { ...relayConfig, home: bobHome },
      {
        command: "init",
        canonicalId: "did:key:z6MkBobResolve",
        displayName: "Bob",
      },
      pluginRoot,
    );

    const aliceCard = await runLinkClaw(
      { ...relayConfig, home: aliceHome },
      { command: "card_export" },
      pluginRoot,
    );
    const aliceCardPath = join(bobHome, "alice-resolve.card.json");
    await writeFile(
      aliceCardPath,
      JSON.stringify((aliceCard.result as { card: unknown }).card, null, 2),
      "utf8",
    );

    await runConnectCommand(
      { ...relayConfig, home: bobHome },
      aliceCardPath,
      pluginRoot,
    );

    const sendResult = await runMessageCommand(
      { ...relayConfig, home: bobHome },
      "alice hello by name",
      pluginRoot,
    );
    assert.equal(sendResult.type, "message");
    assert.match(sendResult.message, /LinkClaw message queued/);
  } finally {
    relayProc.kill();
  }
});

test("runMessageCommand reports ambiguous saved contact names with candidates", async () => {
  const bobHome = await mkdtemp(join(tmpdir(), "linkclaw-bob-home-"));
  const aliceOneHome = await mkdtemp(join(tmpdir(), "linkclaw-alice-one-home-"));
  const aliceTwoHome = await mkdtemp(join(tmpdir(), "linkclaw-alice-two-home-"));

  await runLinkClaw(
    { binaryPath, home: bobHome },
    {
      command: "init",
      canonicalId: "did:key:z6MkBobAmbiguous",
      displayName: "Bob",
    },
    pluginRoot,
  );
  await runLinkClaw(
    { binaryPath, home: aliceOneHome },
    {
      command: "init",
      canonicalId: "did:key:z6MkAliceOne",
      displayName: "Alice",
    },
    pluginRoot,
  );
  await runLinkClaw(
    { binaryPath, home: aliceTwoHome },
    {
      command: "init",
      canonicalId: "did:key:z6MkAliceTwo",
      displayName: "Alice",
    },
    pluginRoot,
  );

  const aliceOneCard = await runLinkClaw(
    { binaryPath, home: aliceOneHome },
    { command: "card_export" },
    pluginRoot,
  );
  const aliceTwoCard = await runLinkClaw(
    { binaryPath, home: aliceTwoHome },
    { command: "card_export" },
    pluginRoot,
  );
  const aliceOneCardPath = join(bobHome, "alice-one.card.json");
  const aliceTwoCardPath = join(bobHome, "alice-two.card.json");
  await writeFile(
    aliceOneCardPath,
    JSON.stringify((aliceOneCard.result as { card: unknown }).card, null, 2),
    "utf8",
  );
  await writeFile(
    aliceTwoCardPath,
    JSON.stringify((aliceTwoCard.result as { card: unknown }).card, null, 2),
    "utf8",
  );

  await runConnectCommand(
    { binaryPath, home: bobHome },
    aliceOneCardPath,
    pluginRoot,
  );
  await runConnectCommand(
    { binaryPath, home: bobHome },
    aliceTwoCardPath,
    pluginRoot,
  );

  const sendResult = await runMessageCommand(
    { binaryPath, home: bobHome },
    "Alice hello ambiguous",
    pluginRoot,
  );
  assert.equal(sendResult.type, "message");
  assert.match(sendResult.message, /matched multiple saved contacts/);
  assert.match(sendResult.message, /did:key:z6MkAliceOne/);
  assert.match(sendResult.message, /did:key:z6MkAliceTwo/);
  assert.match(sendResult.message, /\/linkclaw-message did:key:z6MkAliceOne hello ambiguous/);
  assert.match(sendResult.message, /\/linkclaw-message did:key:z6MkAliceTwo hello ambiguous/);
});

test("runThreadCommand reports ambiguous saved contact names with suggested commands", async () => {
  const bobHome = await mkdtemp(join(tmpdir(), "linkclaw-bob-thread-home-"));
  const aliceOneHome = await mkdtemp(join(tmpdir(), "linkclaw-alice-thread-one-home-"));
  const aliceTwoHome = await mkdtemp(join(tmpdir(), "linkclaw-alice-thread-two-home-"));

  await runLinkClaw(
    { binaryPath, home: bobHome },
    {
      command: "init",
      canonicalId: "did:key:z6MkBobThreadAmbiguous",
      displayName: "Bob",
    },
    pluginRoot,
  );
  await runLinkClaw(
    { binaryPath, home: aliceOneHome },
    {
      command: "init",
      canonicalId: "did:key:z6MkAliceThreadOne",
      displayName: "Alice",
    },
    pluginRoot,
  );
  await runLinkClaw(
    { binaryPath, home: aliceTwoHome },
    {
      command: "init",
      canonicalId: "did:key:z6MkAliceThreadTwo",
      displayName: "Alice",
    },
    pluginRoot,
  );

  const aliceOneCard = await runLinkClaw(
    { binaryPath, home: aliceOneHome },
    { command: "card_export" },
    pluginRoot,
  );
  const aliceTwoCard = await runLinkClaw(
    { binaryPath, home: aliceTwoHome },
    { command: "card_export" },
    pluginRoot,
  );
  const aliceOneCardPath = join(bobHome, "alice-thread-one.card.json");
  const aliceTwoCardPath = join(bobHome, "alice-thread-two.card.json");
  await writeFile(
    aliceOneCardPath,
    JSON.stringify((aliceOneCard.result as { card: unknown }).card, null, 2),
    "utf8",
  );
  await writeFile(
    aliceTwoCardPath,
    JSON.stringify((aliceTwoCard.result as { card: unknown }).card, null, 2),
    "utf8",
  );

  await runConnectCommand(
    { binaryPath, home: bobHome },
    aliceOneCardPath,
    pluginRoot,
  );
  await runConnectCommand(
    { binaryPath, home: bobHome },
    aliceTwoCardPath,
    pluginRoot,
  );

  const threadResult = await runThreadCommand(
    { binaryPath, home: bobHome },
    "Alice",
    pluginRoot,
  );
  assert.equal(threadResult.type, "message");
  assert.match(threadResult.message, /matched multiple saved contacts/);
  assert.match(threadResult.message, /\/linkclaw-thread did:key:z6MkAliceThreadOne/);
  assert.match(threadResult.message, /\/linkclaw-thread did:key:z6MkAliceThreadTwo/);
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
    assert.match(sendResult.message, /LinkClaw message queued/);

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

test("runInboxCommand filters conversations by contact query", async () => {
  const relayPort = await reservePort();
  const relayDir = await mkdtemp(join(tmpdir(), "linkclaw-relay-"));
  const relayDb = join(relayDir, "relay.db");
  const relayProc = execFile(relayBinaryPath, ["--db", relayDb, "--listen", `127.0.0.1:${relayPort}`], {
    cwd: resolve(pluginRoot, ".."),
    env: { ...process.env },
  });

  const aliceHome = await mkdtemp(join(tmpdir(), "linkclaw-alice-home-"));
  const bobHome = await mkdtemp(join(tmpdir(), "linkclaw-bob-home-"));
  const carolHome = await mkdtemp(join(tmpdir(), "linkclaw-carol-home-"));

  try {
    await new Promise((resolvePromise) => setTimeout(resolvePromise, 1000));
    const relayConfig = { binaryPath, relayUrl: `http://127.0.0.1:${relayPort}` };

    await runLinkClaw(
      { ...relayConfig, home: aliceHome },
      { command: "init", canonicalId: "did:key:z6MkAliceInbox", displayName: "Alice Inbox" },
      pluginRoot,
    );
    await runLinkClaw(
      { ...relayConfig, home: bobHome },
      { command: "init", canonicalId: "did:key:z6MkBobInbox", displayName: "Bob Inbox" },
      pluginRoot,
    );
    await runLinkClaw(
      { ...relayConfig, home: carolHome },
      { command: "init", canonicalId: "did:key:z6MkCarolInbox", displayName: "Carol Inbox" },
      pluginRoot,
    );

    const aliceCard = await runLinkClaw(
      { ...relayConfig, home: aliceHome },
      { command: "card_export" },
      pluginRoot,
    );
    const bobCard = await runLinkClaw(
      { ...relayConfig, home: bobHome },
      { command: "card_export" },
      pluginRoot,
    );
    const carolCard = await runLinkClaw(
      { ...relayConfig, home: carolHome },
      { command: "card_export" },
      pluginRoot,
    );

    const bobCardPath = join(aliceHome, "bob-inbox.card.json");
    const carolCardPath = join(aliceHome, "carol-inbox.card.json");
    const aliceForBobPath = join(bobHome, "alice-inbox.card.json");
    const aliceForCarolPath = join(carolHome, "alice-inbox.card.json");
    await writeFile(bobCardPath, JSON.stringify((bobCard.result as { card: unknown }).card, null, 2), "utf8");
    await writeFile(carolCardPath, JSON.stringify((carolCard.result as { card: unknown }).card, null, 2), "utf8");
    await writeFile(aliceForBobPath, JSON.stringify((aliceCard.result as { card: unknown }).card, null, 2), "utf8");
    await writeFile(aliceForCarolPath, JSON.stringify((aliceCard.result as { card: unknown }).card, null, 2), "utf8");

    await runConnectCommand({ ...relayConfig, home: aliceHome }, bobCardPath, pluginRoot);
    await runConnectCommand({ ...relayConfig, home: aliceHome }, carolCardPath, pluginRoot);
    await runConnectCommand({ ...relayConfig, home: bobHome }, aliceForBobPath, pluginRoot);
    await runConnectCommand({ ...relayConfig, home: carolHome }, aliceForCarolPath, pluginRoot);

    await runMessageCommand({ ...relayConfig, home: bobHome }, "\"Alice Inbox\" hello from bob inbox", pluginRoot);
    await runMessageCommand({ ...relayConfig, home: carolHome }, "\"Alice Inbox\" hello from carol inbox", pluginRoot);
    await runSyncCommand({ ...relayConfig, home: aliceHome }, "", pluginRoot);

    const inboxResult = await runInboxCommand(
      { ...relayConfig, home: aliceHome },
      "bob",
      pluginRoot,
    );
    assert.equal(inboxResult.type, "message");
    assert.match(inboxResult.message, /query: bob/);
    assert.match(inboxResult.message, /Bob Inbox/);
    assert.doesNotMatch(inboxResult.message, /Carol Inbox/);
  } finally {
    relayProc.kill();
  }
});

test("runInboxCommand filters conversations by preview query", async () => {
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
      { command: "init", canonicalId: "did:key:z6MkAliceInboxPreview", displayName: "Alice Preview" },
      pluginRoot,
    );
    await runLinkClaw(
      { ...relayConfig, home: bobHome },
      { command: "init", canonicalId: "did:key:z6MkBobInboxPreview", displayName: "Bob Preview" },
      pluginRoot,
    );

    const aliceCard = await runLinkClaw(
      { ...relayConfig, home: aliceHome },
      { command: "card_export" },
      pluginRoot,
    );
    const bobCard = await runLinkClaw(
      { ...relayConfig, home: bobHome },
      { command: "card_export" },
      pluginRoot,
    );

    const bobCardPath = join(aliceHome, "bob-preview.card.json");
    const aliceCardPath = join(bobHome, "alice-preview.card.json");
    await writeFile(bobCardPath, JSON.stringify((bobCard.result as { card: unknown }).card, null, 2), "utf8");
    await writeFile(aliceCardPath, JSON.stringify((aliceCard.result as { card: unknown }).card, null, 2), "utf8");

    await runConnectCommand({ ...relayConfig, home: aliceHome }, bobCardPath, pluginRoot);
    await runConnectCommand({ ...relayConfig, home: bobHome }, aliceCardPath, pluginRoot);
    await runMessageCommand({ ...relayConfig, home: bobHome }, "\"Alice Preview\" shipment status green", pluginRoot);
    await runSyncCommand({ ...relayConfig, home: aliceHome }, "", pluginRoot);

    const inboxResult = await runInboxCommand(
      { ...relayConfig, home: aliceHome },
      "shipment",
      pluginRoot,
    );
    assert.equal(inboxResult.type, "message");
    assert.match(inboxResult.message, /query: shipment/);
    assert.match(inboxResult.message, /last=shipment status green/);
  } finally {
    relayProc.kill();
  }
});

test("discover inspect connect message recover flow stays healthy end-to-end", async () => {
  const fixture = await createResolverFixtureServer();
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
      { command: "init", canonicalId: "did:key:z6MkAliceE2EChain", displayName: "Alice E2E Chain" },
      pluginRoot,
    );
    await runLinkClaw(
      { ...relayConfig, home: bobHome },
      { command: "init", canonicalId: "did:key:z6MkBobE2EChain", displayName: "Bob E2E Chain" },
      pluginRoot,
    );

    const inspectResult = await runInspectCommand(
      { binaryPath },
      `${fixture.origin}/.well-known/agent-card.json`,
      pluginRoot,
    );
    assert.equal(inspectResult.type, "message");
    assert.match(inspectResult.message, /LinkClaw inspect/);
    assert.match(inspectResult.message, /status: consistent/);
    assert.match(inspectResult.message, /importable: yes/);

    const importResult = await runImportCommand(
      { binaryPath, home: bobHome },
      `${fixture.origin}/.well-known/agent-card.json`,
      pluginRoot,
    );
    assert.equal(importResult.type, "message");
    assert.match(importResult.message, /Identity imported/);

    const discoverResult = await runDiscoverCommand(
      { binaryPath, home: bobHome },
      "--limit 10",
      pluginRoot,
    );
    assert.equal(discoverResult.type, "message");
    assert.match(discoverResult.message, /LinkClaw discovery/);
    assert.match(discoverResult.message, /did:web:fixture\.example/);

    const aliceCard = await runLinkClaw(
      { ...relayConfig, home: aliceHome },
      { command: "card_export" },
      pluginRoot,
    );
    const bobCard = await runLinkClaw(
      { ...relayConfig, home: bobHome },
      { command: "card_export" },
      pluginRoot,
    );

    const aliceCardPath = join(bobHome, "alice-e2e-chain.card.json");
    const bobCardPath = join(aliceHome, "bob-e2e-chain.card.json");
    await writeFile(aliceCardPath, JSON.stringify((aliceCard.result as { card: unknown }).card, null, 2), "utf8");
    await writeFile(bobCardPath, JSON.stringify((bobCard.result as { card: unknown }).card, null, 2), "utf8");

    const bobConnectsAlice = await runConnectCommand(
      { ...relayConfig, home: bobHome },
      aliceCardPath,
      pluginRoot,
    );
    const contactMatch = bobConnectsAlice.message.match(/contact: ([^\n]+)/);
    assert.ok(contactMatch);
    await runConnectCommand(
      { ...relayConfig, home: aliceHome },
      bobCardPath,
      pluginRoot,
    );

    const sendResult = await runMessageCommand(
      { ...relayConfig, home: bobHome },
      `${contactMatch[1]} hello e2e chain`,
      pluginRoot,
    );
    assert.equal(sendResult.type, "message");
    assert.match(sendResult.message, /LinkClaw message (queued|delivered)/);

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
    assert.match(syncResult.message, /recovery checks: \d+/);

    const threadResult = await runThreadCommand(
      { ...relayConfig, home: aliceHome },
      "did:key:z6MkBobE2EChain",
      pluginRoot,
    );
    assert.equal(threadResult.type, "message");
    assert.match(threadResult.message, /LinkClaw thread/);
    assert.match(threadResult.message, /hello e2e chain/);
  } finally {
    relayProc.kill();
    await fixture.close();
  }
});

test("message connect-peer reports blocked readiness when no transport route is usable", async () => {
  const fixture = await createResolverFixtureServer();
  const home = await mkdtemp(join(tmpdir(), "linkclaw-connect-peer-blocked-home-"));

  try {
    await runLinkClaw(
      { binaryPath, home },
      { command: "init", canonicalId: "did:key:z6MkBlockedReady", displayName: "Blocked Ready" },
      pluginRoot,
    );

    const importEnvelope = await runLinkClaw(
      { binaryPath, home },
      {
        command: "import",
        input: `${fixture.origin}/.well-known/agent-card.json`,
      },
      pluginRoot,
    );
    const imported = importEnvelope.result as { contact_id?: string };
    assert.equal(typeof imported.contact_id, "string");
    assert.ok((imported.contact_id ?? "").length > 0);

    const removeEnvelope = await runLinkClaw(
      { binaryPath, home },
      {
        command: "known_rm",
        identifier: imported.contact_id ?? "",
      },
      pluginRoot,
    );
    assert.equal(removeEnvelope.ok, true);

    const connectEnvelope = await runLinkClaw(
      { binaryPath, home },
      {
        command: "message_connect_peer",
        identifier: "did:web:fixture.example",
      },
      pluginRoot,
    );
    assert.equal(connectEnvelope.ok, true);
    const connectResult = connectEnvelope.result as Record<string, unknown>;
    assert.equal(connectResult.connected, false);
    assert.match(String(connectResult.reason ?? ""), /no usable transport route/i);
  } finally {
    await fixture.close();
  }
});
