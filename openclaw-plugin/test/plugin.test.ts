import test from "node:test";
import assert from "node:assert/strict";

import registerLinkClawPlugin from "../index.ts";

test("plugin registers the generic bridge and publish skill tools", () => {
  const tools: Array<{ name: string; optional?: boolean }> = [];

  registerLinkClawPlugin({
    getConfig: () => ({}),
    registerTool(tool) {
      tools.push({ name: tool.name, optional: tool.optional });
    },
  });

  assert.deepEqual(tools, [
    { name: "linkclaw_core", optional: true },
    { name: "linkclaw_publish", optional: true },
  ]);
});
