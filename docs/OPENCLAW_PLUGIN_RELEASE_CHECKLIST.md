# OpenClaw Plugin Release Checklist

This checklist is for shipping the LinkClaw OpenClaw plugin as an installable package, not just a local development checkout.

## 1. Pre-release checks

From repository root:

```bash
go test ./...
```

From `openclaw-plugin`:

```bash
npm test
```

Expected result:

- core tests pass
- plugin tests pass
- no local runtime-only files are staged for release

## 2. Build a release tarball

From `openclaw-plugin`:

```bash
npm run pack:plugin:tgz
```

Expected result:

- a file like `linkclaw-0.1.0.tgz` appears in `openclaw-plugin/`

## 3. Verify install from `.tgz`

Use an isolated OpenClaw state directory when possible.

Example:

```bash
HOME="$(mktemp -d)" node /path/to/openclaw/openclaw.mjs plugins install /path/to/linkclaw-0.1.0.tgz
HOME="$(cat /tmp/your-temp-home 2>/dev/null || echo "$HOME")" node /path/to/openclaw/openclaw.mjs plugins list --json
```

Expected result:

- plugin id `linkclaw`
- plugin `status: loaded`
- no `linkclaw`-specific diagnostics

## 4. Verify minimum host config

Recommended minimum host config:

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

Assumptions:

- packaged installs include a bundled `linkclaw` runtime for the current platform
- no legacy HTTP fallback is required for the normal runtime-backed path

## 5. First-run acceptance

After host restart:

1. Run `/linkclaw-onboarding`
2. Run `/linkclaw-onboarding --display-name <name>`
3. Run `/linkclaw-share --card`
4. Import that card on a second host with `/linkclaw-connect`
5. Exchange one message in each direction

Expected result:

- onboarding reports binary health
- relay may be `not configured` unless you intentionally enable legacy HTTP fallback
- identity card includes `relay_url` only when that legacy fallback is explicitly configured
- messages queue or deliver successfully through the current runtime-backed path
- the second host can sync and read the message

## 6. Release notes / docs update

Before publishing or handing the package to another team:

- update `openclaw-plugin/README.md` if install steps changed
- update `docs/OPENCLAW_USER_MANUAL_ZH.md` if first-run flow changed
- confirm the package version in `openclaw-plugin/package.json`

## 7. Release strategy

Current supported release artifact:

- `.tgz` built from `openclaw-plugin/`

Current supported install commands:

```bash
openclaw plugins install -l /path/to/linkclaw/openclaw-plugin
openclaw plugins install /path/to/linkclaw-<version>.tgz
```

Planned end-user install command after npm publication:

```bash
openclaw plugins install <npm-package-spec>
```

Do not document `pnpm add ...` as the primary OpenClaw install flow. OpenClaw plugin installation should stay aligned with `openclaw plugins install ...`.

## 8. What not to ship

Do not include local runtime artifacts such as:

- `linkclaw-relay.db`
- local `~/.linkclaw` state
- temporary exported cards
- test relay databases or relay shim outputs
