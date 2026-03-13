import test from "node:test";
import assert from "node:assert/strict";
import { access, mkdtemp } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";

import {
  buildLinkClawArgs,
  runLinkClaw,
} from "../src/bridge.ts";
import {
  buildLinkClawBinary,
  createResolverFixtureServer,
  pluginRoot,
} from "./helpers.ts";

let binaryPath = "";

test.before(async () => {
  binaryPath = await buildLinkClawBinary();
});

test("buildLinkClawArgs maps known trust and note flags", () => {
  const trustArgs = buildLinkClawArgs(
    "known_trust",
    {
      command: "known_trust",
      identifier: "contact-1",
      trustLevel: "trusted",
      riskFlags: ["manual-review", "fixture"],
      reason: "checked",
    },
    "/tmp/linkclaw-home",
  );
  assert.deepEqual(trustArgs, [
    "known",
    "trust",
    "--home",
    "/tmp/linkclaw-home",
    "--json",
    "--level",
    "trusted",
    "--risk",
    "manual-review,fixture",
    "--reason",
    "checked",
    "contact-1",
  ]);

  const noteArgs = buildLinkClawArgs(
    "known_note",
    {
      command: "known_note",
      identifier: "contact-1",
      noteBody: "hello",
    },
    "/tmp/linkclaw-home",
  );
  assert.deepEqual(noteArgs, [
    "known",
    "note",
    "--home",
    "/tmp/linkclaw-home",
    "--body",
    "hello",
    "--json",
    "contact-1",
  ]);
});

test("bridge runs init and publish against the real binary", async () => {
  const home = await mkdtemp(join(tmpdir(), "linkclaw-home-"));
  const output = join(home, "bundle");
  const config = { binaryPath, home };

  const initEnvelope = await runLinkClaw(
    config,
    {
      command: "init",
      canonicalId: "did:web:agent.example",
      displayName: "Agent Example",
    },
    pluginRoot,
  );
  assert.equal(initEnvelope.ok, true);
  assert.equal(initEnvelope.schema_version, "linkclaw.cli.v1");
  assert.equal(initEnvelope.subcommand, null);
  assert.deepEqual(initEnvelope.warnings, []);
  assert.equal(typeof initEnvelope.timestamp, "string");

  const publishEnvelope = await runLinkClaw(
    config,
    {
      command: "publish",
      origin: "https://agent.example",
      output,
      tier: "full",
    },
    pluginRoot,
  );
  assert.equal(publishEnvelope.ok, true);
  assert.equal(publishEnvelope.schema_version, "linkclaw.cli.v1");
  assert.equal(publishEnvelope.subcommand, null);
  const result = publishEnvelope.result as Record<string, unknown>;
  assert.equal(result.home_origin, "https://agent.example");
  assert.equal(result.tier, "full");
  const checks = result.checks as Array<Record<string, unknown>>;
  assert.ok(checks.length > 0);
  assert.ok(checks.every((check) => check.ok === true));
  await access(String(result.manifest_path));
});

test("bridge covers inspect, import, and known commands against the real binary", async () => {
  const fixture = await createResolverFixtureServer();
  const home = await mkdtemp(join(tmpdir(), "linkclaw-home-"));
  const config = { binaryPath, home };

  try {
    await runLinkClaw(
      config,
      {
        command: "init",
        canonicalId: "did:web:self.example",
        displayName: "Self Example",
      },
      pluginRoot,
    );

    const inspectEnvelope = await runLinkClaw(
      config,
      {
        command: "inspect",
        input: `${fixture.origin}/profile/`,
      },
      pluginRoot,
    );
    assert.equal(
      (inspectEnvelope.result as Record<string, unknown>).verification_state,
      "consistent",
    );
    assert.equal(inspectEnvelope.schema_version, "linkclaw.cli.v1");
    assert.equal(inspectEnvelope.subcommand, null);

    const importEnvelope = await runLinkClaw(
      config,
      {
        command: "import",
        input: `${fixture.origin}/profile/`,
      },
      pluginRoot,
    );
    const importResult = importEnvelope.result as Record<string, unknown>;
    assert.equal(importEnvelope.schema_version, "linkclaw.cli.v1");
    const contactID = String(importResult.contact_id);
    assert.ok(contactID !== "");

    const lsEnvelope = await runLinkClaw(
      config,
      {
        command: "known_ls",
      },
      pluginRoot,
    );
    assert.equal(lsEnvelope.subcommand, "ls");
    const contacts = (lsEnvelope.result as Record<string, unknown>).contacts as unknown[];
    assert.equal(contacts.length, 1);

    const showEnvelope = await runLinkClaw(
      config,
      {
        command: "known_show",
        identifier: contactID,
      },
      pluginRoot,
    );
    assert.equal(
      ((showEnvelope.result as Record<string, unknown>).contact as Record<string, unknown>).contact_id,
      contactID,
    );

    const trustEnvelope = await runLinkClaw(
      config,
      {
        command: "known_trust",
        identifier: contactID,
        trustLevel: "trusted",
        riskFlags: ["fixture"],
        reason: "reviewed from plugin test",
      },
      pluginRoot,
    );
    assert.equal(
      (((trustEnvelope.result as Record<string, unknown>).contact as Record<string, unknown>).trust as Record<string, unknown>).trust_level,
      "trusted",
    );

    const noteEnvelope = await runLinkClaw(
      config,
      {
        command: "known_note",
        identifier: contactID,
        noteBody: "hello from plugin test",
      },
      pluginRoot,
    );
    assert.equal(
      ((noteEnvelope.result as Record<string, unknown>).note as Record<string, unknown>).body,
      "hello from plugin test",
    );

    const refreshEnvelope = await runLinkClaw(
      config,
      {
        command: "known_refresh",
        identifier: contactID,
      },
      pluginRoot,
    );
    assert.equal(
      (((refreshEnvelope.result as Record<string, unknown>).inspection as Record<string, unknown>).status),
      "consistent",
    );

    const rmEnvelope = await runLinkClaw(
      config,
      {
        command: "known_rm",
        identifier: contactID,
      },
      pluginRoot,
    );
    assert.equal(
      ((rmEnvelope.result as Record<string, unknown>).removed as Record<string, unknown>).contacts,
      1,
    );
  } finally {
    await fixture.close();
  }
});
