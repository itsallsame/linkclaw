import test from "node:test";
import assert from "node:assert/strict";

import linkClawPlugin from "../index.ts";

test("plugin registers tools, commands, hooks, and lifecycle handlers", async () => {
  const tools: Array<{ name: string; optional?: boolean }> = [];
  const commands: Array<{
    name: string;
    handler?: (ctx: { args?: string }) => Promise<{ text: string }> | { text: string };
  }> = [];
  const hooks: string[] = [];
  const services: string[] = [];
  const lifecycle = new Map<string, (event: unknown) => Promise<void> | void>();

  linkClawPlugin.register({
    getConfig: () => ({ publishOrigin: "https://agent.example" }),
    registerTool(tool) {
      tools.push({ name: tool.name, optional: tool.optional });
    },
    registerCommand(command) {
      commands.push(command);
    },
    registerHook(name) {
      hooks.push(name);
    },
    registerService(service) {
      services.push(service.id);
    },
    on(name, handler) {
      lifecycle.set(name, handler);
    },
  });

  assert.deepEqual(tools, [
    { name: "linkclaw_core", optional: true },
    { name: "linkclaw_publish", optional: true },
    { name: "linkclaw_setup", optional: true },
    { name: "linkclaw_status", optional: true },
    { name: "linkclaw_share_card", optional: true },
    { name: "linkclaw_connect_card", optional: true },
    { name: "linkclaw_send_message", optional: true },
    { name: "linkclaw_sync_inbox", optional: true },
  ]);
  assert.deepEqual(
    commands.map((command) => command.name),
    [
      "linkclaw-setup",
      "linkclaw-status",
      "linkclaw-import",
      "linkclaw-share",
      "linkclaw-connect",
      "linkclaw-contacts",
      "linkclaw-find",
      "linkclaw-message",
      "linkclaw-reply",
      "linkclaw-thread",
      "linkclaw-inbox",
      "linkclaw-sync",
    ],
  );
  assert.deepEqual(hooks, ["message:preprocessed"]);
  assert.deepEqual(services, ["linkclaw-background-sync"]);
  assert.equal(lifecycle.has("message_sending"), true);
  assert.equal(lifecycle.has("session_start"), true);
  assert.equal(lifecycle.has("session_started"), false);
  assert.equal(lifecycle.has("message_received"), true);

  const messageSending = lifecycle.get("message_sending");
  assert.ok(messageSending);
  const event = {
    content: "share https://agent.example/.well-known/agent-card.json",
  };
  await messageSending?.(event);
  assert.match(
    event.content,
    /https:\/\/agent\.example\/\.well-known\/did\.json/,
  );

  const statusCommand = commands.find((command) => command.name === "linkclaw-status");
  assert.ok(statusCommand?.handler);
  const statusReply = await statusCommand.handler?.({ args: "" });
  assert.equal(typeof statusReply?.text, "string");
});
