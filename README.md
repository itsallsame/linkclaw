# LinkClaw

LinkClaw is a local-first CLI for publishing, verifying, and importing agent identity surfaces built around `did.json`, `webfinger`, and a small profile page.

It is aimed at developers who want a working identity toolchain before they need a larger protocol stack:

- `linkclaw init` creates a local home and self identity.
- `linkclaw publish` builds the public bundle and Cloudflare-friendly headers.
- `linkclaw serve` hosts that bundle locally with the right MIME types.
- `linkclaw inspect` and `linkclaw import` verify other identity surfaces back into a local trust book.

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

From an OpenClaw workspace:

```bash
pnpm add linkclaw-openclaw-plugin
```

The package README covers plugin registration, `binaryPath` / `home` / `publishOrigin` config, passive discovery, and the `/linkclaw-import` plus `/linkclaw-share` commands.

## Docs

- [Quickstart](docs/quickstart.md)
- [Cloudflare Pages Deployment](docs/deploy-cloudflare.md)
