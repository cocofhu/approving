# AGENTS.md — repo contribution rules

**This file is the short, hard contribution / change-code rules for this
repository.** It is for humans and coding agents working on Approving.

It is **not** `agents/*/workspace/AGENTS.md` (platform role-pack mission and
delivery contracts). Do not mix them; nested role-pack files must not replace
these repo rules.

This file does **not** replace [`CONTRIBUTING.md`](CONTRIBUTING.md),
[`README.md`](README.md), or other encyclopedic docs. Prefer this page for
path→commands, gates, pitfalls, and do-not-touch; use CONTRIBUTING for full
setup and layout.

---

## Directory roles

| Path | Role |
|------|------|
| `server/` | Go backend (FSM, sandbox client, artifact MCP, APIs) |
| `web/` | Vue 3 + Vue Flow UI |
| `sandbox-gateway/gateway/` | Gateway control plane |
| `sandbox-gateway/sandbox/` + `sandbox-gateway/scripts/` | Universal sandbox image, startup/scripts, cover helpers |
| `docs/` | Project site (static HTML) + help (Markdown → HTML); CI publishes to `cocofhu/approving-pages` |

Always-on branch-protection job: `.github/workflows/ci.yml` (`gate` only —
does **not** run lint/test). Module suites are path-filtered.

---

## Change path → local commands

Copy from CI. Cross-tree changes: run each matching suite. `ROOT` = repo root.

### `server/**` or root `.golangci.yml` → `ci-server`

Working directory: `server/` (golangci-lint v2.12, config `$ROOT/.golangci.yml`).

```bash
ROOT="$PWD"   # from repo root
cd server
golangci-lint run --config "$ROOT/.golangci.yml" ./...
go vet ./...
go run ./cmd/gen-configdoc -out CONFIGURATION.md -check
go test ./...
./scripts/cover-check-server.sh 90
go test ./internal/runtime/ -count=1 -run 'TestRunAgent|TestReact'
```

### `web/**` → `ci-web`

Working directory: `web/` (Node 24).

```bash
cd web
npm ci --no-audit --no-fund
npm run lint
npx vue-tsc --noEmit
npm test -- --coverage
npm run build
```

### `sandbox-gateway/gateway/**` → `ci-gateway`

```bash
ROOT="$PWD"
cd sandbox-gateway/gateway
golangci-lint run --config "$ROOT/.golangci.yml" ./...
go vet ./...
cd .. && ./scripts/cover-check.sh gateway 90
```

### `docs/**` → `ci-docs`

Working directory: `docs/` (Node 24). Homepage is static HTML under `site/`;
help pages are Markdown under `content/` (built to `public/`).

```bash
cd docs
npm ci --no-audit --no-fund
npm run build
# optional local preview (root-relative assets):
# BASE_PATH=/ npm run server
```

On push to `main`, `ci-docs` also runs `.github/scripts/publish-pages.sh` when
Secret `PAGES_DEPLOY_KEY` is set (SSH deploy key with write access on
`cocofhu/approving-pages`).

### `sandbox-gateway/sandbox/**` or `sandbox-gateway/scripts/**` → `ci-sandbox`

From `sandbox-gateway/`:

```bash
# Shell syntax + smoke (cwd: sandbox-gateway)
bash -n sandbox/scripts/startup.sh
bash -n sandbox/scripts/install-agent.sh
bash -n sandbox/scripts/claude-env.sh
bash -n sandbox/scripts/vnc-preview.sh
bash -n scripts/test-inject.sh
bash -n scripts/test-git-auth.sh
bash -n scripts/cover-check-sandbox.sh
./scripts/test-inject.sh
./scripts/test-git-auth.sh

# Sandbox Go (golangci from sandbox/; cover from sandbox-gateway/)
ROOT="$PWD/.."   # if cwd is sandbox-gateway; else set to repo root
(cd sandbox && golangci-lint run --config "$ROOT/.golangci.yml" ./...)
./scripts/cover-check-sandbox.sh 90

# Docker cli-tools stage (glab/gh) — as in ci-sandbox
# docker build --target cli-tools -t universal-sandbox-cli-tools:ci \
#   -f sandbox/Dockerfile sandbox/
```

---

## Quality gates (hard numbers)

| Gate | Threshold / rule | Source |
|------|------------------|--------|
| Server Go coverage | ≥ **90** via `./scripts/cover-check-server.sh 90` | **Core unit-testable package subset**, not full `go test ./...` coverpkg |
| Gateway Go coverage | ≥ **90** via `./scripts/cover-check.sh gateway 90` | `ci-gateway` |
| Sandbox Go coverage | ≥ **90** via `./scripts/cover-check-sandbox.sh 90` | `ci-sandbox` |
| Web Lines coverage | ≥ **85** | `web/vite.config.ts` → `thresholds.lines` |
| golangci-lint | v2.12, root `.golangci.yml` | ci-server / ci-gateway / ci-sandbox |
| ESLint | `npm run lint` — **errors** fail; warnings allowed | `ci-web` |
| vue-tsc | `npx vue-tsc --noEmit` | `ci-web` |
| gen-configdoc | `go run ./cmd/gen-configdoc -out CONFIGURATION.md -check` | `ci-server` |

Do not invent new thresholds. Badge color bands on orphan `coverage-badges` are
not a substitute for these gates.

---

## Before opening a PR

- [ ] Run the local gates for every tree you touched (cross-tree → run each suite).
- [ ] Do not commit secrets, local config, generated artifacts, or org-private URLs.
- [ ] Land via PR into `main`; do not push protected `main` directly.
- [ ] Commands and thresholds were not invented — verified against `ci-*.yml`,
      cover scripts, and `web/vite.config.ts`.

---

## Known pitfalls (short)

1. **Pending gates UI after approve** — Optimistic remove + invalidate generation +
   drop stale peek, or ghost rows / badge bounce. (PR #20 · `usePendingGates` /
   `GatesInboxView`)
2. **cron `NextScheduleTime` → UTC** — After IANA wall-clock math, persist with
   `.UTC()`; SQLite compares `next_run_at` as UTC text. (PR #19 · `schedule.go`)
3. **`submit_mr` list-first idempotency** — List open → create → parse
   already-exists / no-commits-between; non-zero create must not mean hard fail;
   idempotent success may leave empty `mr_url`. (PR #13 · git SKILL)
4. **Path-filtered CI blind spot** — Docs-only / root-only / cross-module PRs may
   skip module jobs; the always-on `ci` gate is not module proof. Still run local
   suites for trees you actually touched.

---

## Do not touch

1. Secrets, local config, generated artifacts, org-private URLs.
2. `.github/workflows` and publish/image contracts unless the task explicitly requires it.
3. Orphan `coverage-badges` branch payload.
4. Direct push to protected `main`.
5. Treat or rewrite `agents/*/workspace/AGENTS.md` as repo contribution rules.
