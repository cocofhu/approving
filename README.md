# Approving

**Agent workflows that advance with humans — a new paradigm for multi-agent collaboration.**

Approving turns coding agents into workflows you can trust: visual orchestration, sandboxed execution, and human Approve at critical nodes before continuing. Open-source and self-hostable, with rollback and human gates. See the [Site](https://www.approving-ai.com/) — This project was built with this workflow.

[![CI Server](https://github.com/cocofhu/approving/actions/workflows/ci-server.yml/badge.svg)](https://github.com/cocofhu/approving/actions/workflows/ci-server.yml)
[![CI Web](https://github.com/cocofhu/approving/actions/workflows/ci-web.yml/badge.svg)](https://github.com/cocofhu/approving/actions/workflows/ci-web.yml)
[![CI Sandbox](https://github.com/cocofhu/approving/actions/workflows/ci-sandbox.yml/badge.svg)](https://github.com/cocofhu/approving/actions/workflows/ci-sandbox.yml)
[![CI Gateway](https://github.com/cocofhu/approving/actions/workflows/ci-gateway.yml/badge.svg)](https://github.com/cocofhu/approving/actions/workflows/ci-gateway.yml)

[![coverage-web](https://img.shields.io/endpoint?url=https%3A%2F%2Fraw.githubusercontent.com%2Fcocofhu%2Fapproving%2Fcoverage-badges%2Fcoverage-web.json)](https://github.com/cocofhu/approving/actions/workflows/ci-web.yml)
[![coverage-sandbox](https://img.shields.io/endpoint?url=https%3A%2F%2Fraw.githubusercontent.com%2Fcocofhu%2Fapproving%2Fcoverage-badges%2Fcoverage-sandbox.json)](https://github.com/cocofhu/approving/actions/workflows/ci-sandbox.yml)
[![coverage-server](https://img.shields.io/endpoint?url=https%3A%2F%2Fraw.githubusercontent.com%2Fcocofhu%2Fapproving%2Fcoverage-badges%2Fcoverage-server.json)](https://github.com/cocofhu/approving/actions/workflows/ci-server.yml)
[![coverage-gateway](https://img.shields.io/endpoint?url=https%3A%2F%2Fraw.githubusercontent.com%2Fcocofhu%2Fapproving%2Fcoverage-badges%2Fcoverage-gateway.json)](https://github.com/cocofhu/approving/actions/workflows/ci-gateway.yml)

Security scans (CodeQL, web npm audit, gitleaks) run via the [security](https://github.com/cocofhu/approving/actions/workflows/security.yml) workflow on push/PR. View CodeQL under Security → Code scanning (or PR Checks); npm audit and gitleaks results are in the corresponding Actions job logs.

[Site](https://www.approving-ai.com/) · [Contributing](CONTRIBUTING.md) · [Security](SECURITY.md) · [Support](SUPPORT.md) · [Configuration](server/CONFIGURATION.md) · [Gateway](GATEWAY.md)

**English | [简体中文](README.zh-CN.md)**

## What is Approving?

Approving helps agent workflows move forward with humans: visually orchestrate multi-agent collaboration, run in sandboxes, and pause for human Approve at critical nodes. Backends include Cursor, Claude Code, CodeBuddy, and Trae.

## Why Approving?

Named for Approve: in human–agent collaboration, critical nodes must be approved by a person before the workflow continues — trust is built into the path.

## Features

Approving manages the full agent workflow: from graph design to sandboxed execution to artifact handoff.

- **Visual orchestration** — canvas FSM with when guards, checkpoints, and gates; rollback on failure
- **Real Docker sandboxes** — agent and react nodes run in containers via ACP through the vendored `sandbox-gateway`. The web UI is API-driven.
- **Multi-agent backends** — one platform for Cursor, Claude Code, CodeBuddy, and Trae. Pick `acpBackend` per agent; keys live on Agent meta env.
- **Artifact contract + run-scoped MCP** — agents call `write_artifact` / `set_*` / `node_complete`. Each run is isolated by token.
- **Git credentials in the sandbox** — configure `GITHUB_*`, `GITLAB_*`, or SSH on Agent meta env (values may reference `${vars.<name>}`). GitLab auto-MR via `glab` remains available; GitHub PRs use Agent-side `gh`.
- **Single-repo self-host** — `sandbox-gateway` and sandbox image sources live in this repository. One clone is enough.

---

## Quick Start

Linux host with Docker Compose. The default path pulls published GHCR images (no local image build):

```bash
./start.sh -d
```

Then open:

- UI / API: http://localhost:8080
- API health: http://localhost:8080/api/health
- Gateway health: http://localhost:8899/healthz
- Default login: `admin` / `demo1234` (local-demo)

`./start.sh` also pulls the **sandbox runtime** image from GHCR (several GB). Until that finishes, sandbox chat stays on “starting sandbox…”.

Useful commands:

```bash
./start.sh logs
./start.sh down
./start.sh pull          # refresh GHCR images
./start.sh dev -d        # source stack: go run + Vite HMR
```

Image tags and digests are overridable in `.env` — see [`.env.example`](.env.example). For release pinning and smoke tests, see [Contributing](CONTRIBUTING.md#release-images-and-smoke).

---

## Getting Started

### 1. Start the stack

```bash
./start.sh -d
```

Open http://localhost:8080 and sign in with `admin` / `demo1234`. Fresh installs start with an empty project — there is no sample pipeline.

### 2. Configure an agent

In **Agent Studio**, create or edit an agent. Set `acpBackend` (`cursor`, `claude_code`, `codebuddy`, or `trae`) and put the matching API key on Agent env. Details: [`server/README.md`](server/README.md).

### 3. Create a workflow

Use **New workflow** to create a pipeline, then build an FSM graph: agent / react nodes, success and failure edges, optional rollback paths, and human gates where someone must approve before the run continues.

### 4. Run and observe

Publish the pipeline if needed, then start a run. Watch sandbox execution, artifacts written via MCP, and any gate that waits for approval. Failed steps can follow failure or rollback edges instead of dying silently.

---

## Architecture

```
┌──────────────┐     ┌──────────────────┐     ┌──────────────────┐
│  Vue3 +      │────>│  Go Backend      │────>│ sandbox-gateway  │
│  Vue Flow    │<────│  (FSM + MCP)     │<────│  (Docker)        │
└──────────────┘     └────────┬─────────┘     └────────┬─────────┘
                              │                        │
                              │                        ▼
                              │               ┌──────────────────┐
                              └──────────────>│ universal sandbox│
                                artifacts     │ (ACP backends)   │
                                              └──────────────────┘
```

| Layer | Stack |
|-------|-------|
| Frontend | Vue 3 + Vue Flow |
| Backend | Go (`approving-server`, env prefix `APPROVING_`) |
| Execution | Vendored `sandbox-gateway` + universal sandbox images |
| Agent backends | Cursor, Claude Code, CodeBuddy, Trae (ACP) |

## Development

For contributors working on the Approving codebase, see the [Contributing Guide](CONTRIBUTING.md). Critical-path Playwright e2e (CI `web-e2e`): see [CONTRIBUTING — Critical-path Playwright e2e](CONTRIBUTING.md#critical-path-playwright-e2e).

**Prerequisites:** Go, Node.js, Docker Compose (Linux host for sandboxes)

```bash
./start.sh dev -d
```

Source layout: `server/` (Go FSM + sandbox client + artifact MCP), `web/` (Vue UI), `sandbox-gateway/` (gateway + sandbox image sources).

Configuration: [`server/CONFIGURATION.md`](server/CONFIGURATION.md) and `server/config.example.yaml`. Precedence: explicit env > mounted file > defaults. Git credentials for sandboxes stay on Agent meta env, not platform config.

## License

[MIT](LICENSE) © 2026 cocofhu
