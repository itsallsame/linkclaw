# Quickstart

This guide walks from a fresh local home to a locally verifiable identity surface in a few minutes.

## 1. Install

```bash
go install github.com/xiewanpeng/claw-identity/cmd/linkclaw@latest
linkclaw version
```

## 2. Create a local identity

```bash
linkclaw init \
  --canonical-id did:web:localhost \
  --display-name "Agent Example" \
  --non-interactive
```

This creates the local LinkClaw home, state database, and the first self key.

## 3. Publish a local bundle

For the first closed-loop test, publish against a local origin that matches the dev server address:

```bash
linkclaw publish \
  --origin http://127.0.0.1:8787 \
  --output ./publish \
  --tier full
```

The output directory now contains:

- `.well-known/did.json`
- `.well-known/webfinger`
- `.well-known/agent-card.json`
- `profile/index.html`
- `_headers`
- `manifest.json`

## 4. Serve the bundle locally

```bash
linkclaw serve --dir ./publish
```

The dev server is designed for LinkClaw publish bundles:

- `/.well-known/webfinger` is served as `application/json` even without a file extension.
- `/.well-known/did.json` is served as `application/did+json`.
- `/profile` and `/profile/` both resolve to `profile/index.html`.

## 5. Inspect your published surface

In a second terminal:

```bash
linkclaw inspect http://127.0.0.1:8787/profile/
```

You should see a successful inspection with the expected artifacts and proofs.

## 6. Re-publish for your public domain

Once the local loop works, publish again with your real origin:

```bash
linkclaw publish \
  --origin https://agent.example \
  --output ./publish \
  --tier full
```

At that point you can serve the directory elsewhere or deploy it to Cloudflare Pages. See [deploy-cloudflare.md](deploy-cloudflare.md).
