# Contributing to Approving

Before contributing, read [`CODE_OF_CONDUCT.md`](CODE_OF_CONDUCT.md) and
[`SECURITY.md`](SECURITY.md).

## Development setup

- Backend: `cd server && go test ./...`
- Web: `cd web && npm ci && npm test && npx vue-tsc --noEmit`
- Configuration doc: `cd server && go run ./cmd/gen-configdoc -check`
- Full local development stack: `./start.sh dev -d` (source/HMR; gateway
  sources live in `sandbox-gateway/`). Default `./start.sh -d` pulls published
  GHCR images via `compose.release.yaml`.

Never commit credentials, local configuration, generated databases, or
organization-only URLs to public examples.

## Repository layout

```
approving/
├── server/                 Go backend (FSM + sandbox client + artifact MCP)
├── web/                    Vue3 + Vue Flow UI
├── sandbox-gateway/        Vendored gateway + universal sandbox image
├── docker-compose.yml      Dev/source stack (./start.sh dev)
├── start.sh                Default: pull GHCR + up; dev/source via ./start.sh dev
├── compose.release.yaml    Published-image stack (./start.sh default)
├── release-smoke.sh        Clean-Linux release smoke
├── GATEWAY.md              Gateway contract
└── .github/                Issues, PRs, Actions
```

## Release images and smoke

Pushing a `v*` tag runs:

- `publish-image` → `ghcr.io/cocofhu/approving`
- `publish-gateway` → `ghcr.io/cocofhu/sandbox-gateway`
- `publish-sandbox` → `ghcr.io/cocofhu/universal-sandbox-{cursor,claude_code,codebuddy,trae}`

Sandbox builds are large (often 30–90+ minutes). Packages may start private;
set them Public under GitHub → Packages if anonymous pulls are required.

Default tags used by `./start.sh` (overridable in `.env`):

- `ghcr.io/cocofhu/approving:0.0.1-beta`
- `ghcr.io/cocofhu/sandbox-gateway:0.0.1-beta`
- `ghcr.io/cocofhu/universal-sandbox-cursor:0.0.1-beta`

After digest-pinned images exist, run a clean-Linux smoke:

```bash
export APPROVING_IMAGE='ghcr.io/cocofhu/approving@sha256:...'
export SANDBOX_GATEWAY_IMAGE='ghcr.io/cocofhu/sandbox-gateway@sha256:...'
export SANDBOX_IMAGE='ghcr.io/cocofhu/universal-sandbox-cursor@sha256:...'
./release-smoke.sh
```

Dev-only local sandbox image:

```bash
./start.sh sandbox       # build universal-sandbox-cursor:local
```

## Coverage badges

README shows `coverage-web` / `coverage-sandbox` Lines% via shields.io Endpoint
Badges. Endpoint JSON lives on the orphan `coverage-badges` branch and is
updated only when the corresponding workflow succeeds on the default branch
(`ci-web` / `ci-sandbox`). Failed or skipped coverage runs do not overwrite the
last successful value. Color bands: ≥85% green, 70–84% yellow, below 70% orange;
cold start is `n/a` / lightgrey.

Because those workflows are path-filtered, a module badge stays at its last
successful percent until that module’s paths change again — expected lag, not a
badge outage. There is no `coverage-server` badge and no coverage SaaS.

## Changes and pull requests

1. Create a focused branch from the current default branch.
2. Add tests for behavior changes and update `README.md` /
   `server/README.md` / `GATEWAY.md` / `server/CONFIGURATION.md` when public
   behavior, commands, configuration, or links change.
3. Run the relevant checks above. Generated configuration docs must be current
   (`go run ./cmd/gen-configdoc`).
4. Use a concise commit subject describing why the change is needed.
5. Open a pull request with the problem, solution, risk, and test evidence.

Keep pull requests reviewable. Avoid unrelated formatting or refactors.
Maintainers may ask for a smaller follow-up when a proposal crosses security,
compatibility, or release-contract boundaries.

## Reporting problems

Use the issue templates for reproducible defects and feature proposals. Do not
include secrets or vulnerability details. Security reports follow
[`SECURITY.md`](SECURITY.md).
