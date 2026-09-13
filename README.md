# Grasp

**An FSM for coding agents — visual states humans can approve in parallel.**

Most multi-agent setups hide the path inside a conversation: one chat, one happy path, and a human who must re-prompt when something fails. Grasp makes the path a **finite state machine**. You design success, failure, and rollback on a canvas; agents run those states in Docker sandboxes; humans only enter at explicit gates — and they approve from a clarified spec and a `page.html` preview, not a transcript.

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

> Grasp is currently a public beta. It requires a Linux host with Docker Compose. Default startup only needs Grasp + Gateway; the single `universal-sandbox` image is pulled once.

## Why an FSM — not another agent chat

A single coding agent can finish one task. String several together and three things break:

- the **path** lives in prompts, so nobody can reuse, audit, or recover it;
- **failure** means “ask again”, not a designed rollback to a checkpoint;
- **humans** cannot keep up once many runs are waiting — unless each pending state is visual and structured.

Grasp’s bet: agents are fast; the workflow must still be a machine you designed.

```text
                    ┌──── fail / rollback (restore checkpoint) ────┐
                    ▼                                              │
One sentence → Grasp → Visual page.html → Human gate → Implement → Test → Review → PR
     ▲                    ▲                    │
     └──── revise ────────┴────────────────────┘
```

Nodes are states. Edges are transitions (`success` / `fail` / `rollback`) with optional `when` guards. Checkpoints snapshot variables so a retry is a state change, not a new chat.

## What is different

### 1. Design the path first

Build the machine on a Vue Flow canvas: Input, Grasp, role agents, Visual, Branch, Human gate, App preview, Output.

- **Success / fail / rollback** are first-class edges, not comments in a prompt.
- **`when` guards** and **Branch** (if / else-if / else) route on artifacts, JSON fields, and outputs.
- **Checkpoints** mark safe re-entry; rollback restores the variable snapshot and injects the error.
- A **state trace** records enter / exit / transition / rollback — the run is inspectable.

This is the opposite of a one-shot agent: the path exists before anyone types a goal.

### 2. Humans are states, not spectators

When a step needs a decision, the FSM **stops**. The gate appears in the inbox and on the run. Reviewers confirm from structured artifacts — a clarified requirement, a plan, a `page.html` preview — then the machine continues on the edge you drew.

They do not follow every tool call. Many runs can sit at different gates at once; people scan visuals and approve in parallel.

### 3. Visual clarification makes each state graspable

Home starts a **pre-dev Grasp** from one sentence (or a screenshot / doc). That node is a multi-turn ReAct with no prompt template: the user speaks first, the agent aligns requirements, and it only `ask_question` on a real decision.

Two deliverables finish the node:

1. `clarified_requirement.json` — structured WHAT
2. `plan.json` — a short, two-level plan

Optional: research, proposals, a live app preview, and a self-contained `page.html` grounded in the existing frontend. Gates render the page in an iframe, so the pending state is something you can *see*.

### 4. Artifacts fire the transitions

Each run has an isolated artifact MCP. Agents write with `write_artifact`, `set_*`, `node_complete`. The engine does not advance because the model “felt done” — required artifacts must exist (and `when` expressions can read them). Handoffs are contracts, not pasted chat.

### 5. Execution is sandboxed, backends are swappable

Agent states run in Docker through the vendored `sandbox-gateway`. One workflow can mix Cursor, Claude Code, CodeBuddy, Trae, and OpenCode. Credentials stay in Agent env, not in platform images.

## Core capabilities

| Capability | In the FSM |
|---|---|
| Visual canvas | Nodes + success / fail / rollback + `when` + checkpoints |
| Visual clarify | Grasp node → spec + plan + optional `page.html` |
| Human gates | Inbox, run detail, shareable temp links |
| Parallel runs | Many machines at once; humans approve from one inbox |
| Artifact MCP | Isolated per run; required outputs gate transitions |
| Git delivery | `gh` / `glab` / SSH inside the sandbox |
| Observability | Timeline, sandbox logs, artifacts, token usage |

The repository includes Clarify, Visual, Research, Proposal, Plan, Implement, Test, Preview, and Review role packs. Run `agents/pack.sh` and import them in Agent Studio.

## Typical workflow

Short pre-dev loop:

```text
One sentence → Grasp (clarify / plan / page.html) → Human gate → build
```

Fuller delivery machine:

```text
Clarify → Research → Proposal → Human gate
        → Plan → Implement → Test → Review
        → Human confirm → PR / MR
```

Draw the fail and rollback edges on the same canvas. The next failure should follow a path you already designed.

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

> The sandbox runtime is pulled on demand when you first create a sandbox (Inbox / run page show pull loading). Warm it with `./start.sh pull`.

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
2. Create an agent in **Agent Studio**, select `cursor`, `claude_code`, `codebuddy`, `trae`, or `opencode`, and configure the matching API key.
3. Open the canvas: connect a Grasp node after start, then Visual / gate / implement nodes. Draw success, fail, and rollback — mark checkpoints where a retry should re-enter.
4. Publish and start a run (or launch from **Home** in one sentence). Watch the state trace, `page.html` preview, and inbox items waiting at gates.

See [`server/README.md`](server/README.md) for backend authentication and Agent env configuration.

## Architecture

```text
┌──────────────────┐     ┌──────────────────┐     ┌──────────────────┐
│ Vue 3 + Vue Flow │────▶│ Go Backend       │────▶│ sandbox-gateway  │
│ FSM canvas       │◀────│ engine + API+MCP │◀────│ control plane    │
└──────────────────┘     └────────┬─────────┘     └────────┬─────────┘
                                  │                        │
                                  │                        ▼
                                  │               ┌──────────────────┐
                                  └──────────────▶│ Docker sandboxes │
                                    artifacts     │ ACP backends     │
                                                  └──────────────────┘
```

- `web/` — Vue 3 + Vue Flow canvas, Home clarify, run details, inbox, and Agent Studio.
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
- Grasp is still beta software. Perform your own security review, backups, and capacity validation before production use.
- **Reverse proxy Host:** temporary approval share links mint from this request's `Host` (never client `X-Forwarded-Host`). Preserve the browser Host (for example nginx `proxy_set_header Host $host`) and forward `X-Forwarded-Proto` when TLS terminates upstream. See [`SECURITY.md`](SECURITY.md).
- **DB ↔ attachment lifecycle:** release Compose separates SQLite (`./.localdata/db`) from app-data/blobs (`./.localdata/app-data`). Backup and clean them as a pair (and include a custom `GRASP_BLOBS_ROOT` if set); otherwise Run inputs can keep `blob:` refs while `GET /api/blobs/:id` returns 404. Historical orphans are shown as permanent UI placeholders only—this release does not ship an orphan scanner. See [Quick start · Database and attachments](docs/content/en/guide/quick-start.md#database-and-attachments-share-one-lifecycle-backup--cleanup).

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
