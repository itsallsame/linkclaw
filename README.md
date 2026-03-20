# LinkClaw

LinkClaw is a local-first CLI for publishing, verifying, and importing agent identity surfaces built around `did.json`, `webfinger`, and a small profile page.

It is aimed at developers who want a working identity toolchain before they need a larger protocol stack:

- `linkclaw init` creates a local home and self identity.
- `linkclaw publish` builds the public bundle and Cloudflare-friendly headers.
- `linkclaw serve` hosts that bundle locally with the right MIME types.
- `linkclaw inspect` and `linkclaw import` verify other identity surfaces back into a local trust book.

It also includes a V0 local-direct-messaging stack for OpenClaw users:

- `linkclaw card export` and `linkclaw card import` exchange signed identity cards
- `linkclaw message send`, `sync`, `inbox`, and `outbox` cover one-to-one relay-backed messaging
- `linkclaw-relay` provides encrypted store-and-forward delivery

## 30-Second Flow

Install the CLI:

```bash
go install github.com/xiewanpeng/claw-identity/cmd/linkclaw@latest
linkclaw version
```

Create a local identity, publish a bundle, and serve it locally:

```bash
linkclaw init \
  --canonical-id did:web:localhost \
  --display-name "Agent Example" \
  --non-interactive

linkclaw publish \
  --origin http://127.0.0.1:8787 \
  --output ./publish \
  --tier full

linkclaw serve --dir ./publish
```

In a second terminal, verify the served identity:

```bash
linkclaw inspect http://127.0.0.1:8787/profile/
```

## Install

### Go install

```bash
go install github.com/xiewanpeng/claw-identity/cmd/linkclaw@latest
```

This is the recommended path for day-to-day development. `linkclaw version` will show release metadata when built from a tagged release, and sensible defaults for local source builds.

### Prebuilt binaries

Release archives are generated with GoReleaser for:

- Linux `amd64`, `arm64`
- macOS `amd64`, `arm64`
- Windows `amd64`, `arm64`

## Common Commands

```bash
linkclaw version
linkclaw init --canonical-id did:web:agent.example --display-name "Agent Example" --non-interactive
linkclaw publish --origin https://agent.example --tier full
linkclaw serve
linkclaw inspect https://agent.example/profile/
linkclaw import https://agent.example/profile/
linkclaw known ls
linkclaw card export
linkclaw message inbox
```

## Direct Messaging V0

LinkClaw V0 supports one-to-one asynchronous messaging for OpenClaw users without requiring a domain.

The loop is:

1. `linkclaw init` creates a local identity.
2. `linkclaw card export` produces a signed identity card.
3. Share that card with the other user over any existing channel.
4. The recipient runs `linkclaw card import <path-or-json>` to save you as a contact.
5. Messages are sent with `linkclaw message send <contact> --body "..."`
6. Offline messages sit in `linkclaw-relay` until the recipient runs `linkclaw message sync`

Minimal local example:

```bash
linkclaw init --canonical-id did:key:z6MkAlice --display-name Alice --non-interactive
LINKCLAW_RELAY_URL=http://127.0.0.1:8788 linkclaw card export --json
LINKCLAW_RELAY_URL=http://127.0.0.1:8788 linkclaw message inbox --json
```

## Cloudflare Pages

`linkclaw publish` always writes a `_headers` file so Pages serves `/.well-known/webfinger` as `application/json`.

To deploy the generated bundle to an existing Pages project:

```bash
linkclaw publish \
  --origin https://agent.example \
  --output ./publish \
  --tier full \
  --deploy cloudflare \
  --project agent-example
```

The CLI prefers a global `wrangler` binary and falls back to `npx wrangler@latest` when available.

## OpenClaw Plugin

The OpenClaw integration lives in [`openclaw-plugin`](openclaw-plugin/README.md).

Supported install paths follow normal OpenClaw plugin delivery:

- development checkout:

```bash
openclaw plugins install -l /path/to/linkclaw/openclaw-plugin
```

- packaged tarball:

```bash
cd /path/to/linkclaw/openclaw-plugin
npm run pack:plugin:tgz
openclaw plugins install ./linkclaw-0.1.0.tgz
```

The package README covers plugin registration, `binaryPath` / `home` / `publishOrigin` config, passive discovery, and the `/linkclaw-import` plus `/linkclaw-share` commands.

For a first-time end-user install and first-run flow, see [OpenClaw User Manual (ZH)](docs/OPENCLAW_USER_MANUAL_ZH.md).

## Docs

Current milestone validation for the OpenClaw natural-language messaging MVP is tracked in git as `milestone-v0-openclaw-dm`.

- [Quickstart](docs/quickstart.md)
- [Cloudflare Pages Deployment](docs/deploy-cloudflare.md)
- [OpenClaw User Manual (ZH)](docs/OPENCLAW_USER_MANUAL_ZH.md)
- [OpenClaw Minimal Acceptance (ZH)](docs/OPENCLAW_MINIMAL_ACCEPTANCE_ZH.md)
- [OpenClaw Minimal Plugin Config](docs/OPENCLAW_MINIMAL_PLUGIN_CONFIG.json)
- [OpenClaw Plugin Release Checklist](docs/OPENCLAW_PLUGIN_RELEASE_CHECKLIST.md)
- [V0 Messaging Plan](docs/v0-messaging-plan.md)
- [Agent-Social Runtime Design Index](docs/agent-social-runtime-index.md)
