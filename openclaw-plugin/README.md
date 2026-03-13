# LinkClaw OpenClaw Plugin

This plugin keeps OpenClaw on the safe side of the LinkClaw boundary:

- OpenClaw calls the local `linkclaw` core binary via `--json`
- the plugin never reads `state.db` or key files directly
- the publishing skill formats bundle artifacts and self-checks for agent or slash-command use

## Files

- `openclaw.plugin.json`: plugin manifest and config schema
- `index.ts`: OpenClaw entrypoint
- `src/bridge.ts`: binary discovery, command mapping, JSON parsing, error propagation
- `src/publish-skill.ts`: publish skill adapter and manifest fallback
- `skills/linkclaw-publish/SKILL.md`: user-invocable publishing skill

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

## Surfaces

- Tool: `linkclaw_core`
  - generic bridge for `init`, `publish`, `inspect`, `import`, and `known_*`
- Tool: `linkclaw_publish`
  - publishing skill adapter with manifest fallback
- Skill: `/linkclaw-publish`
  - dispatches raw args directly to `linkclaw_publish`

## Local Verification

```bash
cd openclaw-plugin
npm test
```
