# Approving

**Agent workflows that advance with humans — a new paradigm for multi-agent collaboration.**

Approving is an open-source, self-hostable platform for turning coding agents into visual, reviewable, and recoverable delivery workflows. Agents run in real Docker sandboxes, exchange structured artifacts, and pause for human **Approve** at critical nodes.

[Website](https://www.approving-ai.com/) · [Quick start](https://www.approving-ai.com/en/guide/quick-start/) · [Contributing](CONTRIBUTING.md) · [Configuration](server/CONFIGURATION.md) · [Gateway](GATEWAY.md)

**English | [简体中文](README.zh-CN.md)**

[![CI Server](https://github.com/cocofhu/approving/actions/workflows/ci-server.yml/badge.svg)](https://github.com/cocofhu/approving/actions/workflows/ci-server.yml)
[![CI Web](https://github.com/cocofhu/approving/actions/workflows/ci-web.yml/badge.svg)](https://github.com/cocofhu/approving/actions/workflows/ci-web.yml)
[![CI Sandbox](https://github.com/cocofhu/approving/actions/workflows/ci-sandbox.yml/badge.svg)](https://github.com/cocofhu/approving/actions/workflows/ci-sandbox.yml)
[![CI Gateway](https://github.com/cocofhu/approving/actions/workflows/ci-gateway.yml/badge.svg)](https://github.com/cocofhu/approving/actions/workflows/ci-gateway.yml)

[![coverage-web](https://img.shields.io/endpoint?url=https%3A%2F%2Fraw.githubusercontent.com%2Fcocofhu%2Fapproving%2Fcoverage-badges%2Fcoverage-web.json)](https://github.com/cocofhu/approving/actions/workflows/ci-web.yml)
[![coverage-sandbox](https://img.shields.io/endpoint?url=https%3A%2F%2Fraw.githubusercontent.com%2Fcocofhu%2Fapproving%2Fcoverage-badges%2Fcoverage-sandbox.json)](https://github.com/cocofhu/approving/actions/workflows/ci-sandbox.yml)
[![coverage-server](https://img.shields.io/endpoint?url=https%3A%2F%2Fraw.githubusercontent.com%2Fcocofhu%2Fapproving%2Fcoverage-badges%2Fcoverage-server.json)](https://github.com/cocofhu/approving/actions/workflows/ci-server.yml)
[![coverage-gateway](https://img.shields.io/endpoint?url=https%3A%2F%2Fraw.githubusercontent.com%2Fcocofhu%2Fapproving%2Fcoverage-badges%2Fcoverage-gateway.json)](https://github.com/cocofhu/approving/actions/workflows/ci-gateway.yml)

> Approving is currently a public beta. It requires a Linux host with Docker Compose, and the first startup pulls a large sandbox runtime image.

## Why Approving?

A single coding agent is effective at completing one task. As work expands across research, design, implementation, testing, and review, new bottlenecks appear:

- collaboration is hidden inside conversations and is hard to reuse or audit;
- once agents run in parallel, human understanding and review speed limit throughput;
- agents lack stable, verifiable contracts for handing work to one another;
- high-risk actions do not have explicit human decision points;
- failures often require manual prompting instead of following designed recovery paths.

Approving adds a harness above coding agents: FSMs define the path, sandboxes isolate execution, MCP carries artifacts, and human approval becomes a first-class workflow node.

```text
Requirement
    → Clarify Agent
    → Research Agent
    → Proposal Agent
    → Human Gate
    → Implement Agent
    → Test Agent
    → Review Agent
    → PR / MR
```

## Core capabilities

### Visual FSM orchestration

Build workflows on a Vue Flow canvas with agent, react, and gate nodes. Edges describe success, failure, and rollback paths; `when` guards and checkpoints control transitions.

### Human-in-the-loop gates

Pause a run for proposal selection, visual acceptance, release confirmation, or another high-value decision. Reviewers can approve, reject, or request revision from structured artifacts instead of following every agent action.

### Real Docker sandboxes

Agent and react nodes execute in isolated containers through the vendored `sandbox-gateway`. The platform manages sandbox lifecycle, while the ACP bridge connects supported agent backends.

### Multiple agent backends

Use different backends in one workflow:

- Cursor
- Claude Code
- CodeBuddy
- Trae

Each agent chooses its own `acpBackend`. Credentials live in Agent meta env and are not baked into platform images.

### Run-scoped artifact contracts

Every run receives an isolated artifact MCP and token. Agents use tools such as `write_artifact`, `read_artifact`, `set_*`, and `node_complete` to exchange structured results. Before advancing, the platform checks that required artifacts exist.

### Git delivery

Inject GitHub, GitLab, or SSH credentials per agent, including values referenced through `${vars.<name>}`. GitLab MRs can be created with `glab`; GitHub PRs are created by the agent with `gh` inside the sandbox.

### Execution visibility

Inspect run details, execution timelines, sandbox logs, artifacts, pending approvals, and token usage to understand status and resource consumption.

## Typical workflow

A software delivery workflow can separate responsibilities explicitly:

```text
Clarify → Research → Proposal → Human approval
        → Plan → Implement → Test → Review
        → Human confirmation → PR / MR
```

The repository includes Clarify, Visual, Research, Proposal, Plan, Implement, Test, Preview, and Review role packs. Run `agents/pack.sh` to package them for import through Agent Studio.

## Quick start

### Requirements

- Linux host
- Git
- Docker and Docker Compose

### Start

The default path pulls published GHCR images and does not build them locally:

```bash
git clone https://github.com/cocofhu/approving.git
cd approving
./start.sh -d
```

Open:

- UI / API: <http://localhost:8080>
- API health: <http://localhost:8080/api/health>
- Gateway health: <http://localhost:8899/healthz>
- Local demo login: `admin` / `demo1234`

> `./start.sh` also pulls the multi-gigabyte sandbox runtime image. Sandbox chat may remain on “starting sandbox…” until the pull completes.

Useful commands:

```bash
./start.sh logs          # follow logs
./start.sh down          # stop the stack
./start.sh pull          # refresh GHCR images
./start.sh dev -d        # source stack: Go + Vite HMR
```

Override image tags or digests in `.env`; see [`.env.example`](.env.example).

## Build your first workflow

1. Sign in with the local demo account. A fresh installation starts with an empty project and does not create a sample pipeline.
2. Create an agent in **Agent Studio**, select `cursor`, `claude_code`, `codebuddy`, or `trae`, and configure the matching API key.
3. Create a workflow and connect agent nodes, success/failure edges, rollback paths, and human gates.
4. Publish and start a run. Observe sandbox execution, MCP artifacts, and nodes waiting for approval.

See [`server/README.md`](server/README.md) for backend authentication and Agent env configuration.

## Architecture

```text
┌──────────────────┐     ┌──────────────────┐     ┌──────────────────┐
│ Vue 3 + Vue Flow │────▶│ Go Backend       │────▶│ sandbox-gateway  │
│ orchestration    │◀────│ FSM + API + MCP  │◀────│ control plane    │
└──────────────────┘     └────────┬─────────┘     └────────┬─────────┘
                                  │                        │
                                  │                        ▼
                                  │               ┌──────────────────┐
                                  └──────────────▶│ Docker sandboxes │
                                    artifacts     │ ACP backends     │
                                                  └──────────────────┘
```

- `web/` — Vue 3 + Vue Flow canvas, run details, approvals, and Agent Studio.
- `server/` — Go FSM engine, API, SQLite, artifact MCP, scheduling, and audit.
- `sandbox-gateway/gateway/` — sandbox lifecycle control plane.
- `sandbox-gateway/sandbox/` — universal sandbox image and ACP bridge.
- `agents/` — importable role-agent workspaces.
- `docs/` — project site and bilingual help content.

Configuration precedence is explicit environment variables > mounted config file > defaults. See [`server/CONFIGURATION.md`](server/CONFIGURATION.md) for all options and [`GATEWAY.md`](GATEWAY.md) for the gateway contract.

## Development and quality

**Development requirements:** Go, Node.js, and Docker Compose; sandbox execution requires Linux.

```bash
./start.sh dev -d
```

Module-specific lint, test, coverage, and E2E commands are documented in [`AGENTS.md`](AGENTS.md) and [`CONTRIBUTING.md`](CONTRIBUTING.md). The security workflow runs CodeQL, web `npm audit`, and gitleaks on pushes and pull requests.

## Deployment and security notes

- The default account is for local demos only. Configure your own authentication users before any shared or production deployment.
- Keep ACP API keys and Git credentials in project or Agent env; never commit them.
- Pin production images by digest; see [Release images and smoke](CONTRIBUTING.md#release-images-and-smoke).
- Approving is still beta software. Perform your own security review, backups, and capacity validation before production use.
- **DB ↔ attachment lifecycle:** release Compose separates SQLite (`./.localdata/db`) from app-data/blobs (`./.localdata/app-data`). Backup and clean them as a pair (and include a custom `APPROVING_BLOBS_ROOT` if set); otherwise Run inputs can keep `blob:` refs while `GET /api/blobs/:id` returns 404. Historical orphans are shown as permanent UI placeholders only—this release does not ship an orphan scanner. See [Quick start · Database and attachments](docs/content/en/guide/quick-start.md#database-and-attachments-share-one-lifecycle-backup--cleanup).

## Documentation

- [Core concepts](docs/content/en/guide/concepts.md)
- [Quick start](docs/content/en/guide/quick-start.md)
- [Full configuration](server/CONFIGURATION.md)
- [Gateway contract](GATEWAY.md)
- [Contributing guide](CONTRIBUTING.md)
- [Security policy](SECURITY.md)
- [Support](SUPPORT.md)

## Contributing

Issues and pull requests are welcome. Read [`CONTRIBUTING.md`](CONTRIBUTING.md), [`AGENTS.md`](AGENTS.md), and [`CODE_OF_CONDUCT.md`](CODE_OF_CONDUCT.md) before contributing.

## License

[MIT](LICENSE) © 2026 cocofhu
