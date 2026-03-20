import assert from "node:assert/strict";
import test from "node:test";

import {
  createBackgroundSyncService,
  extractUnknownSenders,
  formatBackgroundSyncNotice,
} from "../src/messaging.ts";

test("extractUnknownSenders returns discovered conversations with unread messages", () => {
  const unknown = extractUnknownSenders({
    conversations: [
      {
        contact_display_name: "Alice",
        contact_status: "known",
        unread_count: 3,
      },
      {
        contact_display_name: "Bob",
        contact_canonical_id: "did:key:z6MkBob",
        contact_status: "discovered",
        unread_count: 1,
      },
      {
        contact_display_name: "Carol",
        contact_status: "discovered",
        unread_count: 0,
      },
    ],
  });

  assert.equal(unknown.length, 1);
  assert.equal(unknown[0]?.contact_display_name, "Bob");
});

test("formatBackgroundSyncNotice adds explicit guidance for unknown senders", () => {
  const message = formatBackgroundSyncNotice("linkclaw sync completed\nsynced: 1\nrelay calls: 1", {
    conversations: [
      {
        contact_display_name: "Bob",
        contact_canonical_id: "did:key:z6MkBob",
        contact_status: "discovered",
        unread_count: 1,
        last_message_preview: "hello there",
      },
    ],
  });

  assert.match(message, /unknown senders/i);
  assert.match(message, /did:key:z6MkBob/);
  assert.match(message, /\/linkclaw-inbox/);
  assert.match(message, /\/linkclaw-connect <card>/);
});

test("createBackgroundSyncService no-ops when relay sync is explicitly disabled", async () => {
  const service = createBackgroundSyncService({
    config: { relayUrl: "" },
    pluginRoot: "/tmp/plugin-root",
    logger: {
      info() {
        throw new Error("unexpected info log");
      },
      warn() {
        throw new Error("unexpected warn log");
      },
    },
  });

  await service.start();
  await service.stop?.();

  assert.equal(service.id, "linkclaw-background-sync");
});
