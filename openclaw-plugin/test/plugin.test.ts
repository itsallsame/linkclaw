import test from "node:test";
import assert from "node:assert/strict";

import registerLinkClawPlugin from "../index.ts";

test("plugin registers tools, commands, hooks, and lifecycle handlers", async () => {
  const tools: Array<{ name: string; optional?: boolean }> = [];
  const commands: string[] = [];
  const hooks: string[] = [];
  const lifecycle = new Map<string, (event: unknown) => Promise<void> | void>();

  registerLinkClawPlugin({
    getConfig: () => ({ publishOrigin: "https://agent.example" }),
    registerTool(tool) {
      tools.push({ name: tool.name, optional: tool.optional });
    },
    registerCommand(name) {
      commands.push(name);
    },
    registerHook(name) {
      hooks.push(name);
    },
    on(name, handler) {
      lifecycle.set(name, handler);
    },
  });

  assert.deepEqual(tools, [
    { name: "linkclaw_core", optional: true },
    { name: "linkclaw_publish", optional: true },
  ]);
  assert.deepEqual(commands, ["linkclaw-import", "linkclaw-share"]);
  assert.deepEqual(hooks, ["message:preprocessed"]);
  assert.equal(lifecycle.has("message_sending"), true);

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
});
