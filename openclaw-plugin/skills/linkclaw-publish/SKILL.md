---
name: linkclaw-publish
description: Publish the current LinkClaw identity bundle through the local core binary and show artifacts plus self-check results.
user-invocable: true
command-dispatch: tool
command-tool: linkclaw_publish
command-arg-mode: raw
---

Use this skill when you need to publish the current LinkClaw identity bundle through the local `linkclaw` core binary without touching `state.db` or key files directly.

Supported args:
- `--origin https://agent.example`
- `--tier minimum|recommended|full`
- `--output /absolute/path/to/publish-dir`
- `--home /absolute/path/to/linkclaw-home`

Behavior:
- Executes `linkclaw publish --json` through the plugin bridge.
- Shows manifest path, published artifacts, and check status on success.
- If publish fails after writing `manifest.json`, the adapter falls back to that manifest so failed checks are still surfaced.
