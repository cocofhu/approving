# Contributing to Approving

Short, hard Agent/contribution rules (path→commands, gates, pitfalls, do-not-touch):
see [`AGENTS.md`](AGENTS.md).

Before contributing, read [`CODE_OF_CONDUCT.md`](CODE_OF_CONDUCT.md) and
[`SECURITY.md`](SECURITY.md).

## Development setup

- Backend: `cd server && go test ./...`
- Web: `cd web && npm ci && npm test && npx vue-tsc --noEmit`
- Web critical e2e (same subset as CI `web-e2e`): see [Critical-path Playwright e2e](#critical-path-playwright-e2e)
- Configuration doc: `cd server && go run ./cmd/gen-configdoc -check`
- Full local development stack: `./start.sh dev -d` (source/HMR; gateway
  sources live in `sandbox-gateway/`). Default `./start.sh -d` pulls published
  GHCR images via `compose.release.yaml`.

### Static checks (lint)

CI appends these gates alongside existing vet/tests/`vue-tsc` (nothing is
replaced). Use the same commands locally before opening a PR.

**Go** — shared root config [`.golangci.yml`](.golangci.yml) (golangci-lint v2,
`staticcheck` SA*/S1*, `_test.go` excluded for the first pass). Install a v2.x
binary (`go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.0`
or the [official releases](https://golangci-lint.run/docs/welcome/install/)),
then from each module directory:

```bash
# from repo root
ROOT="$PWD"
(cd server && golangci-lint run --config "$ROOT/.golangci.yml" ./...)
(cd sandbox-gateway/gateway && golangci-lint run --config "$ROOT/.golangci.yml" ./...)
(cd sandbox-gateway/sandbox && golangci-lint run --config "$ROOT/.golangci.yml" ./...)
```

Matching workflows: `ci-server`, `ci-gateway`, `ci-sandbox` (sandbox-go job).

**Web** — ESLint (flat config in `web/`) coexists with `vue-tsc` (types stay on
`vue-tsc`; ESLint is not type-aware in this first pass). CI fails on ESLint
**errors** only; warnings are allowed:

```bash
cd web && npm ci && npm run lint && npx vue-tsc --noEmit
```

Matching workflow: `ci-web` (`web` job).

### Critical-path Playwright e2e

PR `ci-web` also runs a parallel `web-e2e` job for a fixed critical-path subset
(local vite.e2e mock harness; no real backend). Specs:

- `gate-mobile-fill.spec.ts` — gate mobile approve / reject
- `clarify-inbox-product.spec.ts` — clarify inbox product surface
- `delete-run-list.spec.ts` — run list delete / cancel / placeholders
- `cancel-run.spec.ts` — cancel run across statuses

Reproduce locally (same entry as CI):

```bash
cd web && npm ci && npx playwright install chromium && npm run test:e2e:ci
```

Full suite remains `npm run test:e2e` (not run in PR CI). On `web-e2e`
failure, Actions uploads `playwright-report/` and `test-results/` artifacts.

**Follow-up:** add the `web-e2e` check to main branch protection required
checks so a red job hard-blocks merge. Until then, failure is a visible red
signal only.

Later tightening (more Go linters, stricter ESLint rules, optional type-aware
ESLint) can ratchet without changing this layout; not required for this pass.

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
