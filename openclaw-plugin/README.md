# LinkClaw OpenClaw Plugin

This plugin keeps OpenClaw on the safe side of the LinkClaw boundary:

- OpenClaw calls the local `linkclaw` core binary via `--json`
- the plugin never reads `state.db` or key files directly
- the publishing skill formats bundle artifacts and self-checks for agent or slash-command use
- inbound `did.json` and `agent-card.json` links can be inspected automatically and surfaced as import prompts

## Files

- `openclaw.plugin.json`: plugin manifest and config schema
- `index.ts`: OpenClaw entrypoint
- `src/bridge.ts`: binary discovery, command mapping, JSON parsing, error propagation
- `src/discovery.ts`: passive discovery, known-contact dedupe, and share helpers
- `src/commands.ts`: slash commands for import/share flows
- `src/publish-skill.ts`: publish skill adapter and manifest fallback
- `skills/linkclaw-publish/SKILL.md`: user-invocable publishing skill

## Install

1. Install or build the LinkClaw core binary.

```bash
go install github.com/xiewanpeng/claw-identity/cmd/linkclaw@latest
linkclaw version
```

2. Add the plugin to your OpenClaw workspace.

```bash
pnpm add linkclaw-openclaw-plugin
```

3. Register it in your OpenClaw config.

```json
{
  "plugins": {
    "entries": {
      "linkclaw": {
        "package": "linkclaw-openclaw-plugin",
        "config": {
          "binaryPath": "/absolute/path/to/linkclaw",
          "home": "/absolute/path/to/.linkclaw",
          "publishOrigin": "https://agent.example",
          "publishOutput": "/absolute/path/to/publish",
          "publishTier": "recommended"
        }
      }
    }
  }
}
```

## Config

Configure under `plugins.entries.linkclaw.config`:

```json
{
  "binaryPath": "/absolute/path/to/linkclaw",
  "home": "/absolute/path/to/.linkclaw",
  "publishOrigin": "https://agent.example",
  "publishOutput": "/absolute/path/to/publish",
  "publishTier": "recommended"
}
```

`binaryPath` is the safest option. If omitted, the plugin falls back to `LINKCLAW_BINARY`, common repo-local candidates, and finally `PATH`.

Config notes:

- `binaryPath`: preferred; use it when the OpenClaw host is not running inside this repository.
- `home`: default `LINKCLAW_HOME` for `init`, `publish`, `import`, and `known_*`.
- `publishOrigin`: default public origin for `/linkclaw-share` and `/linkclaw-publish`.
- `publishOutput`: default publish directory; falls back to `<home>/publish`.
- `publishTier`: default publish tier for the publish skill.

## Surfaces

- Tool: `linkclaw_core`
  - generic bridge for `init`, `publish`, `inspect`, `import`, and `known_*`
- Tool: `linkclaw_publish`
  - publishing skill adapter with manifest fallback
- Skill: `/linkclaw-publish`
  - dispatches raw args directly to `linkclaw_publish`
- Command: `/linkclaw-import`
  - imports a discovered `did.json` or `agent-card.json` link into the local known contacts book
- Command: `/linkclaw-share`
  - returns the published agent-card and did.json links for the configured origin
- Hook: `message:preprocessed`
  - watches inbound messages for explicit `did.json` or `agent-card.json` URLs, runs `linkclaw inspect`, and prompts for import unless the identity is already known

## Passive Discovery

When an inbound message contains a URL ending in `/.well-known/did.json` or `/.well-known/agent-card.json`, the plugin:

1. Runs `linkclaw inspect --json <url>`.
2. Summarizes verification status, canonical id, origin, artifacts, warnings, and mismatches.
3. Checks whether the identity is already present in `known`.
4. If the contact is new, suggests `/linkclaw-import <url>`.

By default the import command keeps LinkClaw's core safety checks: discovered or mismatched identities are not imported unless the user explicitly adds `--allow-discovered` or `--allow-mismatch`.

## Share Flow

`/linkclaw-share` confirms the configured `publishOrigin` resolves to a published LinkClaw bundle, then emits both:

- `/.well-known/agent-card.json`
- `/.well-known/did.json`

If your OpenClaw runtime exposes the lifecycle hook API, outbound messages that already contain the configured agent-card URL will have the matching `did.json` URL appended automatically.

## FAQ

Q: The plugin cannot find `linkclaw`.
A: Set `plugins.entries.linkclaw.config.binaryPath` explicitly. The fallback search only checks `LINKCLAW_BINARY`, a few repo-local candidates, and `PATH`.

Q: `/linkclaw-import` says the state DB is missing.
A: Run `linkclaw init --home /absolute/path/to/.linkclaw --canonical-id ...` first, or point the plugin `home` config at an existing initialized LinkClaw home.

Q: Passive discovery did not trigger.
A: The hook only reacts to explicit URLs ending in `/.well-known/did.json` or `/.well-known/agent-card.json`. Plain domains and profile URLs are intentionally ignored to keep false positives down.

Q: `/linkclaw-share` says no shareable bundle was found.
A: Publish the identity first, ideally with `recommended` or `full` tier so both `did.json` and `agent-card.json` exist at the configured origin.

## Local Verification

```bash
cd openclaw-plugin
npm test
```
