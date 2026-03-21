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
- `src/commands.ts`: slash commands for import/share/message flows
- `src/messaging.ts`: background sync and unknown-sender prompts
- `src/publish-skill.ts`: publish skill adapter and manifest fallback
- `skills/linkclaw-publish/SKILL.md`: user-invocable publishing skill

## Install

If you want the shortest user-facing path first, start with [OpenClaw User Manual (ZH)](../docs/OPENCLAW_USER_MANUAL_ZH.md).

For the shortest go/no-go validation on a clean host, use [OpenClaw Minimal Acceptance (ZH)](../docs/OPENCLAW_MINIMAL_ACCEPTANCE_ZH.md) and [OpenClaw Minimal Plugin Config](../docs/OPENCLAW_MINIMAL_PLUGIN_CONFIG.json).

### Zero-config baseline

If the OpenClaw host can already find `linkclaw` on `PATH`, the plugin now works with a much smaller config:

```json
{
  "plugins": {
    "allow": ["linkclaw"],
    "entries": {
      "linkclaw": {
        "enabled": true
      }
    }
  }
}
```

Defaults:

- `binaryPath`: optional when `linkclaw` is already discoverable via `LINKCLAW_BINARY`, repo-local candidates, or `PATH`
- `home`: defaults to `~/.linkclaw`
- `relayUrl`: optional compatibility-only legacy HTTP fallback; use it only if you still want the old store-and-forward path

For most local installs, the fastest path is:

1. install `linkclaw` somewhere on `PATH`
2. install the plugin
3. run `/linkclaw-onboarding`
4. only configure `relayUrl` if you explicitly want legacy HTTP fallback compatibility

### Development checkout

1. Build the LinkClaw core binary.

```bash
cd /path/to/linkclaw
go build -o /home/ubuntu/bin/linkclaw ./cmd/linkclaw
```

2. Link this plugin into an OpenClaw checkout.

```bash
cd /path/to/openclaw
openclaw plugins install -l /path/to/linkclaw/openclaw-plugin
openclaw plugins enable linkclaw
```

3. Register it in your OpenClaw config.

```json
{
  "plugins": {
    "allow": ["linkclaw"],
    "entries": {
      "linkclaw": {
        "enabled": true,
        "config": {
          "binaryPath": "/absolute/path/to/linkclaw",
          "home": "/absolute/path/to/.linkclaw",
          "publishOrigin": "https://agent.example",
          "publishOutput": "/absolute/path/to/publish",
          "publishTier": "recommended",
          "syncIntervalMs": 30000
        }
      }
    }
  }
}
```

4. Validate the plugin before first use.

```text
/linkclaw-setup --check-only
```

Expected signals:

- `binary: ok (...)`
- `relay: ...`
- `publish origin: ...`

If the host is not initialized yet, `state: not initialized` is expected at this point.

### Packaged plugin (`.tgz`)

1. Install or build the LinkClaw core binary.

```bash
go install github.com/xiewanpeng/claw-identity/cmd/linkclaw@latest
linkclaw version
```

2. Build an installable plugin tarball.

```bash
cd openclaw-plugin
npm run pack:plugin:tgz
```

3. Add the plugin to your OpenClaw workspace.

```bash
openclaw plugins install ./linkclaw-<version>.tgz
```

4. Register it in your OpenClaw config.

```json
{
  "plugins": {
    "allow": ["linkclaw"],
    "entries": {
      "linkclaw": {
        "package": "linkclaw",
        "config": {
          "binaryPath": "/absolute/path/to/linkclaw",
          "home": "/absolute/path/to/.linkclaw",
          "publishOrigin": "https://agent.example",
          "publishOutput": "/absolute/path/to/publish",
          "publishTier": "recommended",
          "syncIntervalMs": 30000
        }
      }
    }
  }
}
```

5. Validate the plugin before first use.

```text
/linkclaw-setup --check-only
```

`plugins.allow` is recommended on real hosts. Without it, OpenClaw can still load the plugin, but it will warn that non-bundled plugins may auto-load.

### Future npm package

This repository is now aligned for directory installs and `.tgz` installs.

The intended final end-user install command, once the plugin is published to npm, is:

```bash
openclaw plugins install <npm-package-spec>
```

Until that publish path is in place, treat `.tgz` as the release artifact for non-developer installs.

## Config

Configure under `plugins.entries.linkclaw.config`:

```json
{
  "binaryPath": "/absolute/path/to/linkclaw",
  "home": "/absolute/path/to/.linkclaw",
  "publishOrigin": "https://agent.example",
  "publishOutput": "/absolute/path/to/publish",
  "publishTier": "recommended",
  "syncIntervalMs": 30000
}
```

`binaryPath` is the safest option. If omitted, the plugin falls back to `LINKCLAW_BINARY`, common repo-local candidates, and finally `PATH`.

Config notes:

- `binaryPath`: preferred; use it when the OpenClaw host is not running inside this repository.
- `home`: default `LINKCLAW_HOME` for `init`, `publish`, `import`, and `known_*`.
- `relayUrl`: optional compatibility-only legacy HTTP fallback. If omitted, the plugin does not inject a relay URL at all. `LINKCLAW_RELAY_URL` still works as an environment override when you intentionally want that fallback path.
- `publishOrigin`: default public origin for `/linkclaw-share` and `/linkclaw-publish`.
- `publishOutput`: default publish directory; falls back to `<home>/publish`.
- `publishTier`: default publish tier for the publish skill.
- `syncIntervalMs`: optional background polling interval in milliseconds; defaults to `30000` and is clamped to at least `5000`.

Top-level host config recommendation:

- `plugins.allow`: set to `["linkclaw"]` on hosts where you want explicit plugin trust instead of non-bundled auto-load warnings.

## Host Checklist

Before trying A/B messaging, verify these host-level conditions:

- OpenClaw can execute the configured `binaryPath`
- `home` points to a writable directory
- if you still use legacy HTTP fallback, `relayUrl` is reachable from this machine
- if you want public share links, `publishOrigin` resolves to a published LinkClaw bundle
- if you still use legacy HTTP fallback, that relay is reachable from both peers, not only from localhost on one side
- `plugins.allow` includes `linkclaw` if you want a clean trusted-host config

## Surfaces

- Tool: `linkclaw_core`
  - generic bridge for `init`, `publish`, `inspect`, `import`, and `known_*`
- Tool: `linkclaw_publish`
  - publishing skill adapter with manifest fallback
- Tool: `linkclaw_onboarding`
  - first-run readiness check and identity bootstrap for normal OpenClaw users
- Skill: `/linkclaw-publish`
  - dispatches raw args directly to `linkclaw_publish`
- Command: `/linkclaw-onboarding`
  - first-run entrypoint that defaults to a readiness check, then upgrades to full setup when you provide a display name
- Command: `/linkclaw-import`
  - imports a discovered `did.json` or `agent-card.json` link into the local known contacts book
- Command: `/linkclaw-status`
  - shows readiness, health checks, contact count, and inbox summary for the current home
- Command: `/linkclaw-share`
  - returns the published agent-card and did.json links for the configured origin, or a raw signed identity card with `--card`
- Command: `/linkclaw-connect`
  - imports a local identity card into contacts
- Command: `/linkclaw-contacts`
  - lists saved contacts with trust and verification summary, and can filter by query
- Command: `/linkclaw-find`
  - searches saved contacts and prints copy-ready message/thread commands
- Command: `/linkclaw-message`
  - sends a direct message to an imported contact
- Command: `/linkclaw-reply`
  - replies to one saved contact with the same send flow but clearer chat-oriented wording
- Command: `/linkclaw-thread`
  - shows recent messages for one contact without leaving OpenClaw
- Command: `/linkclaw-inbox`
  - lists local conversations, flags unknown senders, and can filter by contact or preview text
- Command: `/linkclaw-sync`
  - syncs runtime-backed messages into the local inbox, and can still use legacy HTTP fallback when configured
- Service: background sync
  - polls the runtime message path in the background; legacy HTTP fallback sync only activates when `relayUrl` is configured
- Hook: `message:preprocessed`
  - watches inbound messages for explicit `did.json` or `agent-card.json` URLs, runs `linkclaw inspect`, and prompts for import unless the identity is already known

If your OpenClaw runtime exposes lifecycle events, the plugin also runs background sync on session start and inbound message receive. It also registers a lightweight polling service for periodic relay sync. When new relay messages arrive from an unknown sender, the plugin adds a prompt telling the user to open `/linkclaw-inbox` and request an identity card before saving the sender as a contact.

## First Use

Shortest path:

1. Make sure `linkclaw` is either on `PATH` or configured via `binaryPath`.
2. Run `/linkclaw-onboarding`.
3. Run `/linkclaw-onboarding --display-name <name>`.
4. Run `/linkclaw-share --card`.
5. Only configure `relayUrl` or `LINKCLAW_RELAY_URL` if you intentionally want legacy HTTP fallback compatibility.

Extended path:

1. Make sure `linkclaw` is either on `PATH` or configured via `binaryPath`.
2. Run `/linkclaw-setup --display-name <name>`.
   It now reports lightweight setup checks for the resolved binary path, relay reachability, and publish-origin readiness.
   If you only want to validate the environment first, run `/linkclaw-setup --check-only`.
   You can also run `/linkclaw-status` at any time to review readiness, contacts, and inbox state in one place.
3. Run `/linkclaw-share` after you have published a bundle, or `/linkclaw-share --card` if you want to exchange a raw signed identity card directly.
4. On the other side, run `/linkclaw-connect <card-or-url>`.
   A successful import now prints copy-ready follow-up commands for `/linkclaw-message`, `/linkclaw-thread`, and `/linkclaw-share --card`.
5. Review saved contacts with `/linkclaw-contacts` or filter with `/linkclaw-contacts <query>`.
6. Search saved contacts quickly with `/linkclaw-find <query>`.
7. Start messaging with `/linkclaw-message <contact> <text>`.
8. Review one conversation with `/linkclaw-thread <contact>`.
9. Reply in-place with `/linkclaw-reply <contact> <text>`.
10. Only configure `relayUrl` if you explicitly need legacy HTTP fallback.

## Verification Checklist

After installation, this is the fastest way to verify the plugin is wired correctly:

1. Run `/linkclaw-setup --check-only`.
2. Run `/linkclaw-status`.
3. Confirm `state:` is either `ready` or `not initialized`, depending on the home.
4. Confirm `binary: ok (...)` appears.
5. Confirm `relay:` is either `ok (...)` or intentionally `not configured`.
6. If you use public share links, confirm `publish origin:` is `ok ...`.
7. Initialize the home with `/linkclaw-setup --canonical-id ... --display-name ...`.
8. Run `/linkclaw-share --card` and verify the plugin prints `card compact:`.
9. Import that card on another host with `/linkclaw-connect '<card-json>'`.
10. Send a test message with `/linkclaw-message ...`.

## Minimal Acceptance

For a host-level go/no-go check, this is the shortest acceptance sequence:

1. Install the plugin with either `openclaw plugins install -l /path/to/linkclaw/openclaw-plugin` or `openclaw plugins install ./linkclaw-<version>.tgz`.
2. Ensure `plugins.allow` contains `linkclaw`.
3. Run `/linkclaw-setup --check-only`.
4. Run `/linkclaw-status`.
5. Confirm:
   - `state:` is reported
   - `binary: ok (...)` appears
   - `relay:` is either `ok (...)` or intentionally `not configured`
   - no `linkclaw`-specific plugin load warnings appear in `openclaw plugins list --json`
6. Run `/linkclaw-share --card`.
7. Import that card from another host with `/linkclaw-connect '<card-json>'`.
8. Exchange one message in each direction.

If all eight steps pass, the plugin is ready for normal use.

## Stable Output Markers

Some commands now emit explicit boundary markers so an OpenClaw host can extract machine-useful sections without parsing the whole message body heuristically.

- `/linkclaw-share --card`
  - `--- card-compact-begin ---` / `--- card-compact-end ---`
  - `--- connect-command-begin ---` / `--- connect-command-end ---`
- `/linkclaw-setup`
  - `--- health-checks-begin ---` / `--- health-checks-end ---`
- `/linkclaw-status`
  - `--- status-summary-begin ---` / `--- status-summary-end ---`
  - `--- health-checks-begin ---` / `--- health-checks-end ---`
- `/linkclaw-connect`
  - `--- contact-summary-begin ---` / `--- contact-summary-end ---`
- `/linkclaw-contacts`
  - `--- contacts-list-begin ---` / `--- contacts-list-end ---`
- `/linkclaw-inbox`
  - `--- inbox-conversations-begin ---` / `--- inbox-conversations-end ---`
- `/linkclaw-thread`
  - `--- thread-messages-begin ---` / `--- thread-messages-end ---`

When a contact display name contains spaces, quote it:

```text
/linkclaw-message "Alice Example" hello
/linkclaw-reply "Alice Example" got it
```

For saved contacts, `/linkclaw-message`, `/linkclaw-reply`, and `/linkclaw-thread` first try to resolve the contact against your local contacts list. A unique display name match is accepted automatically. If multiple contacts share the same name, the plugin shows the matching candidates and prints copy-ready commands using each canonical id.

`/linkclaw-reply` also accepts a bare message with no contact. In that case the plugin targets your most recent known conversation:

```text
/linkclaw-reply got it
```

If you opened a thread with `/linkclaw-thread <contact>` first, bare `/linkclaw-reply` prefers that thread target. Otherwise it falls back to your most recent known conversation. If there is no recent known conversation yet, the plugin tells you to open `/linkclaw-inbox` or specify a contact explicitly.

`/linkclaw-connect` accepts:

- a raw identity-card JSON string
- a file path containing the raw card JSON
- a file produced by `linkclaw card export --json`

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

If you do not have a published origin yet, `/linkclaw-share --card` exports a signed identity card directly from the local LinkClaw home. The output now includes both a compact one-line JSON blob and a copy-ready `/linkclaw-connect '...json...'` command for the other side. It also wraps those two machine-useful segments in stable markers:

- `--- card-compact-begin ---` / `--- card-compact-end ---`
- `--- connect-command-begin ---` / `--- connect-command-end ---`

That makes it easier for an OpenClaw host to add copy buttons or extract those segments directly. `/linkclaw-connect` also accepts raw card JSON, `linkclaw card export --json` envelopes, and fenced ```json``` blocks pasted from chat.

## Direct Messaging Flow

1. Install the plugin and configure `binaryPath` and `home` if auto-discovery is not enough.
2. Initialize LinkClaw once with `linkclaw init`.
3. Share a signed identity card with `/linkclaw-share` or `linkclaw card export`.
4. Import the other side with `/linkclaw-connect <card>`.
5. Send a message with `/linkclaw-message <contact> <text>`.
6. Use `/linkclaw-sync` or let the lifecycle hook sync automatically.
7. Review conversations with `/linkclaw-inbox` or filter with `/linkclaw-inbox <query>`.
8. Open one conversation with `/linkclaw-thread <contact>` and answer with `/linkclaw-reply <contact> <text>`.
9. Only configure `relayUrl` when you intentionally want legacy HTTP fallback compatibility.

## A/B Walkthrough

This is the shortest end-to-end path for two OpenClaw users who want to exchange identities and send direct messages through the current runtime-backed path.

### Assumptions

- A and B both installed the plugin
- both sides can resolve `linkclaw` either from config or `PATH`
- both sides use the default runtime-backed path unless they intentionally enable legacy HTTP fallback

### A side

1. Initialize LinkClaw:

```text
/linkclaw-setup --canonical-id did:key:z6MkAlice --display-name Alice
```

2. Export a signed card directly for chat handoff:

```text
/linkclaw-share --card
```

3. Copy either:

- the `card compact:` one-line JSON
- or the ready-to-run `example: /linkclaw-connect '...json...'` line and send it to B

### B side

1. Initialize LinkClaw:

```text
/linkclaw-setup --canonical-id did:key:z6MkBob --display-name Bob
```

2. Import A's card:

```text
/linkclaw-connect '<alice-card-json>'
```

3. Confirm the contact exists:

```text
/linkclaw-contacts alice
```

4. Send the first message:

```text
/linkclaw-message Alice hello from Bob
```

### A side again

1. Sync and review the inbox:

```text
/linkclaw-inbox
```

2. Open the conversation:

```text
/linkclaw-thread Bob
```

3. Reply in-place:

```text
/linkclaw-reply hello from Alice
```

Because `/linkclaw-thread Bob` sets reply context, the last command can omit the contact and still target Bob.

### Troubleshooting

- If `/linkclaw-setup` shows `relay: not configured`, that is normal unless you intentionally enabled legacy HTTP fallback.
- If `/linkclaw-setup` shows `relay: unreachable (...)`, fix network reachability before testing messaging.
- If `/linkclaw-setup` shows `publish origin: configured but bundle missing ...`, publish the bundle first or use `/linkclaw-share --card`.
- If `/linkclaw-connect` fails on pasted chat content, try the compact one-line JSON from `/linkclaw-share --card`.
- If `/linkclaw-message Alice ...` is ambiguous, the plugin will print copy-ready commands using canonical ids.
- If OpenClaw warns that non-bundled plugins may auto-load, add `"plugins": { "allow": ["linkclaw"] }` to the host config.

## Updating

### Development checkout

When the plugin source changes:

1. Update the linked checkout.
2. Restart or reload the OpenClaw host if it caches plugin modules.
3. Run `/linkclaw-setup --check-only` again.
4. Run through the A/B walkthrough if you changed messaging or identity flows.

### Packaged release update

When a new `.tgz` package is released:

1. Install the new archive with `openclaw plugins install ./linkclaw-<version>.tgz`
2. Restart or reload the OpenClaw host
3. Run `/linkclaw-onboarding`
4. Run `/linkclaw-status`

### Future npm package update

After the plugin is published to npm, update instructions should use your final package spec:

```bash
openclaw plugins install <npm-package-spec>
```

## FAQ

Q: The plugin cannot find `linkclaw`.
A: Set `plugins.entries.linkclaw.config.binaryPath` explicitly. The fallback search only checks `LINKCLAW_BINARY`, a few repo-local candidates, and `PATH`.

Q: `/linkclaw-import` says the state DB is missing.
A: Run `/linkclaw-setup --canonical-id ... --display-name ...` first, or point the plugin `home` config at an existing initialized LinkClaw home.

Q: `/linkclaw-connect` fails on a file created with `linkclaw card export --json`.
A: Current plugin builds unwrap the CLI envelope automatically. If it still fails, confirm the file came from the same LinkClaw version and contains `result.card.signature`.

Q: Passive discovery did not trigger.
A: The hook only reacts to explicit URLs ending in `/.well-known/did.json` or `/.well-known/agent-card.json`. Plain domains and profile URLs are intentionally ignored to keep false positives down.

Q: `/linkclaw-share` says no shareable bundle was found.
A: Publish the identity first, ideally with `recommended` or `full` tier so both `did.json` and `agent-card.json` exist at the configured origin.

Q: OpenClaw warns that non-bundled plugins may auto-load.
A: Add `linkclaw` to `plugins.allow`. This is a host trust setting, not a LinkClaw plugin failure.

## Local Verification

```bash
cd openclaw-plugin
npm test
```
