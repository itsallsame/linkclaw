# Cloudflare Pages Deployment

This guide covers deploying a LinkClaw publish bundle to Cloudflare Pages.

## Prerequisites

- A Cloudflare account.
- Node.js and npm available locally.
- Either:
  - a global `wrangler` install, or
  - `npx` available so LinkClaw can run `npx wrangler@latest`.
- A Pages project name.

Authenticate with Cloudflare before the first deploy. Either log in with Wrangler:

```bash
npx wrangler@latest login
```

Or provide the usual Cloudflare credentials in the environment, such as `CLOUDFLARE_API_TOKEN` and `CLOUDFLARE_ACCOUNT_ID`.

## 1. Create the Pages project once

If the project does not already exist, create it:

```bash
npx wrangler@latest pages project create agent-example --production-branch main
```

You only need to do this once per project.

## 2. Publish and deploy

Generate the bundle and push it to Pages in one command:

```bash
linkclaw publish \
  --origin https://agent.example \
  --output ./publish \
  --tier full \
  --deploy cloudflare \
  --project agent-example
```

What this does:

- builds the LinkClaw bundle into `./publish`
- writes `_headers` so Pages serves `/.well-known/webfinger` as `application/json`
- invokes `wrangler pages deploy ./publish --project-name agent-example`

## 3. Verify the deployed surface

After deployment finishes:

```bash
linkclaw inspect https://agent.example/profile/
```

If your custom domain is attached to the Pages project, use that domain in both `--origin` and the final `inspect` command.

## Notes

- `--project` is required with `--deploy cloudflare`.
- LinkClaw currently targets existing Pages projects rather than provisioning every Cloudflare resource itself.
- If a deploy fails, the local bundle is still left on disk in the output directory for manual inspection or retry.
