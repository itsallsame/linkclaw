import assert from "node:assert/strict";
import { mkdtemp, writeFile } from "node:fs/promises";
import { createServer } from "node:http";
import test from "node:test";
import { tmpdir } from "node:os";
import { join } from "node:path";

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
    runRegistryConnectCommand,
    runRegistryPublishCommand,
    runRegistrySearchCommand,
    runReplyCommand,
    runSetupCommand,
    runShareCommand,
  runStatusCommand,
  runSyncCommand,
  runThreadCommand,
} from "../src/commands.ts";
import {
  buildLinkClawBinary,
  createResolverFixtureServer,
  pluginRoot,
} from "./helpers.ts";

let binaryPath = "";

test.before(async () => {
  binaryPath = await buildLinkClawBinary();
});

async function createRegistryFixture(handler: (req: import("node:http").IncomingMessage, body: string) => { status?: number; payload: unknown }) {
  const server = createServer(async (req, res) => {
    const chunks: Buffer[] = [];
    for await (const chunk of req) {
      chunks.push(Buffer.isBuffer(chunk) ? chunk : Buffer.from(chunk));
    }
    const result = handler(req, Buffer.concat(chunks).toString("utf8"));
    res.statusCode = result.status ?? 200;
    res.setHeader("Content-Type", "application/json");
    res.end(JSON.stringify(result.payload));
  });
  await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", () => resolve()));
  const address = server.address();
  if (!address || typeof address === "string") {
    throw new Error("failed to bind registry fixture");
  }
  return {
    url: `http://127.0.0.1:${address.port}`,
    close: () => new Promise<void>((resolve, reject) => server.close((error) => error ? reject(error) : resolve())),
  };
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

test("runShareCommand requires an origin when publishOrigin is not configured", async () => {
  const result = await runShareCommand({ binaryPath }, "", pluginRoot);
  assert.equal(result.type, "message");
  assert.match(result.message, /publishOrigin/);
  assert.match(result.message, /\/linkclaw-share --card/);
});

test("runRegistryPublishCommand exports the local card and publishes it to the registry", async () => {
  const home = await mkdtemp(join(tmpdir(), "linkclaw-registry-publish-home-"));
  await runLinkClaw(
    { binaryPath, home },
    {
      command: "init",
      canonicalId: "did:key:z6MkRegistryPublisher",
      displayName: "Registry Publisher",
    },
    pluginRoot,
  );

  const registry = await createRegistryFixture((req, body) => {
    assert.equal(req.url, "/api/agents/publish");
    const payload = JSON.parse(body) as { identity_card?: { id?: string; display_name?: string } };
    return {
      payload: {
        agent_id: "agent_test123",
        canonical_id: payload.identity_card?.id,
        display_name: payload.identity_card?.display_name,
        profile_url: "http://registry.test/api/agents/agent_test123",
        card_url: "http://registry.test/api/agents/agent_test123/card",
      },
    };
  });

  try {
    const result = await runRegistryPublishCommand(
      { binaryPath, home, registryUrl: registry.url },
      "--summary 'security coordination agent' --capabilities ops,coordination --tags security,ops",
      pluginRoot,
    );
    assert.equal(result.type, "message");
    assert.match(result.message, /LinkClaw registry publish/);
    assert.match(result.message, /agent id: agent_test123/);
    assert.match(result.message, /name: Registry Publisher/);
  } finally {
    await registry.close();
  }
});

test("runRegistrySearchCommand lists registry hits and next-step guidance", async () => {
  const registry = await createRegistryFixture((req) => {
    assert.match(req.url ?? "", /\/api\/agents\/search/);
    return {
      payload: {
        records: [
          {
            agent_id: "agent_alpha",
            display_name: "Alpha Agent",
            canonical_id: "did:key:z6MkAlpha",
            summary: "translation specialist",
            capabilities: ["translation", "zh-en"],
          },
        ],
      },
    };
  });

  try {
    const result = await runRegistrySearchCommand(
      { binaryPath, registryUrl: registry.url },
      "translation",
      pluginRoot,
    );
    assert.equal(result.type, "message");
    assert.match(result.message, /LinkClaw agent search/);
    assert.match(result.message, /agent_alpha \| Alpha Agent \| translation specialist/);
    assert.match(result.message, /\/linkclaw-connect-agent <agent-id>/);
  } finally {
    await registry.close();
  }
});

test("runRegistryConnectCommand imports the identity card from the registry record", async () => {
  const aliceHome = await mkdtemp(join(tmpdir(), "linkclaw-registry-alice-home-"));
  const bobHome = await mkdtemp(join(tmpdir(), "linkclaw-registry-bob-home-"));
  await runLinkClaw(
    { binaryPath, home: aliceHome },
    {
      command: "init",
      canonicalId: "did:key:z6MkRegistryAlice",
      displayName: "Registry Alice",
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
      canonicalId: "did:key:z6MkRegistryBob",
      displayName: "Registry Bob",
    },
    pluginRoot,
  );

  const registry = await createRegistryFixture((req) => {
    assert.equal(req.url, "/api/agents/agent_registry_alice");
    return {
      payload: {
        agent_id: "agent_registry_alice",
        identity_card: (exported.result as { card: unknown }).card,
      },
    };
  });

  try {
    const result = await runRegistryConnectCommand(
      { binaryPath, home: bobHome, registryUrl: registry.url },
      "agent_registry_alice",
      pluginRoot,
    );
    assert.equal(result.type, "message");
    assert.match(result.message, /LinkClaw contact saved/);
    assert.match(result.message, /name: Registry Alice/);
  } finally {
    await registry.close();
  }
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
  assert.match(result.message, /运行 \/linkclaw-setup --display-name <name>/);
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
  const home = await mkdtemp(join(tmpdir(), "linkclaw-status-home-"));

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
    { binaryPath, home },
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
  assert.match(result.message, /runtime mode: host-managed/);
});

test("runSyncCommand uses recovery wording instead of relay internals", async () => {
  const home = await mkdtemp(join(tmpdir(), "linkclaw-sync-home-"));

  await runLinkClaw(
    { binaryPath, home },
    {
      command: "init",
      canonicalId: "did:key:z6MkSyncCopy",
      displayName: "Sync Copy",
    },
    pluginRoot,
  );

  const result = await runSyncCommand(
    { binaryPath, home },
    "",
    pluginRoot,
  );

  assert.equal(result.type, "message");
  assert.match(result.message, /LinkClaw sync completed/);
  assert.match(result.message, /recovery checks: 0/);
  assert.doesNotMatch(result.message.toLowerCase(), /relay calls/);
  assert.doesNotMatch(result.message.toLowerCase(), /nostr/);
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
    const promotion = connectResult.promotion as Record<string, unknown>;
    assert.equal(typeof promotion.contact_id, "string");
    assert.ok(String(promotion.contact_id ?? "").length > 0);
    assert.equal(promotion.contact_created, true);
    assert.equal(promotion.trust_linked, true);
    assert.equal(promotion.note_written, false);
    assert.equal(promotion.pin_written, false);
  } finally {
    await fixture.close();
  }
});
