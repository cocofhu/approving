# Approving

English | [中文](README.zh-CN.md)

**Dev Agent orchestration platform** — compose coding agents into rollback-capable,
human-gated, observable workflows. Agents run in real Docker sandboxes through the
vendored [sandbox-gateway](sandbox-gateway/).

> Backend binary `approving-server`, environment prefix `APPROVING_`.
>
> License: [MIT](LICENSE), Copyright (c) 2026 cocofhu.
> Repository: [github.com/cocofhu/approving](https://github.com/cocofhu/approving).
>
> Community: [`CONTRIBUTING.md`](CONTRIBUTING.md), [`SECURITY.md`](SECURITY.md),
> [`SUPPORT.md`](SUPPORT.md).

## Features

- **FSM orchestration**: nodes = states, edges = transitions, with success /
  failure / rollback paths, `when` guards, checkpoints, and human gates.
- **Real execution**: agent/react nodes run in sandbox containers via ACP;
  the web UI is API-driven.
- **Artifact contract + run-scoped MCP**: agents call `write_artifact` /
  `set_*` / `node_complete`; runs are isolated by token.
- **Git host credentials in the sandbox**: configure `GITHUB_*`, `GITLAB_*`,
  or SSH on Agent meta env (values may reference `${vars.<name>}`). Platform
  auto-MR via `glab` remains available for GitLab; GitHub PRs use Agent-side `gh`.
- **Single-repo runtime**: `sandbox-gateway` and sandbox image sources live in
  this repository — no sibling checkout required.

## Layout

```
approving/
├── server/                 Go backend (FSM + sandbox client + artifact MCP)
├── web/                    Vue3 + Vue Flow UI
├── sandbox-gateway/        Vendored gateway + universal sandbox image
├── docker-compose.yml      Local stack: gateway + server + web
├── start.sh                One-shot local entrypoint
├── compose.release.yaml    Digest-pinned public release stack
├── release-smoke.sh        Clean-Linux release smoke
├── GATEWAY.md              Gateway contract
└── .github/                Issues, PRs, Actions
```

## Quick start (Docker Compose)

Linux host with Docker Compose:

```bash
./start.sh -d
# equivalent: docker compose up --build -d
```

- Web UI: http://localhost:5173  
- API health: http://localhost:8080/api/health  
- Gateway health: http://localhost:8899/healthz  
- Default login: `admin` / `demo1234` (from `server/config.yaml`)

First run copies `.env.example` → `.env` and may build
`universal-sandbox-cursor:local` from `sandbox-gateway/sandbox` (can take a while).

Useful commands:

```bash
./start.sh logs
./start.sh down
./start.sh sandbox    # rebuild sandbox image only
./start.sh gateway    # rebuild gateway image only
```

## Published images (GHCR)

Pushing a `v*` tag runs:

- `publish-image` → `ghcr.io/cocofhu/approving`
- `publish-gateway` → `ghcr.io/cocofhu/sandbox-gateway`
- `publish-sandbox` → `ghcr.io/cocofhu/universal-sandbox-{cursor,claude_code,codebuddy,trae}`

Sandbox builds are large (often 30–90+ minutes). Packages may start private;
set them Public under GitHub → Packages if anonymous pulls are required.

## Public release smoke

After digest-pinned images exist:

```bash
export APPROVING_IMAGE='ghcr.io/cocofhu/approving@sha256:...'
export SANDBOX_GATEWAY_IMAGE='ghcr.io/cocofhu/sandbox-gateway@sha256:...'
export SANDBOX_IMAGE='ghcr.io/cocofhu/universal-sandbox-cursor@sha256:...'
./release-smoke.sh
```

## Configuration

See [`server/CONFIGURATION.md`](server/CONFIGURATION.md) and
`server/config.example.yaml`. Precedence: explicit env > mounted file > defaults.
Git credentials for sandboxes stay on Agent meta env, not platform config.

## License

[MIT](LICENSE) © 2026 cocofhu
