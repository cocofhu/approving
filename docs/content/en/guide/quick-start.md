---
title: Quick start
description: Bring up Approving on a Linux host with Docker Compose.
---

## Prerequisites

- A Linux host
- Git, Docker, and Docker Compose installed

## Clone the repository

```bash
git clone https://github.com/cocofhu/approving.git
cd approving
```

## Start

By default this pulls published images from GHCR (no local image build required):

```bash
./start.sh -d
```

Then open:

- UI / API: http://localhost:8080
- API health: http://localhost:8080/api/health
- Gateway health: http://localhost:8899/healthz
- Default login: `admin` / `demo1234` (local-demo)

`./start.sh` also pulls **four sandbox runtime** images from GHCR (one per acpBackend: cursor / claude_code / codebuddy / trae; large). Until that finishes, sandbox chats may stay on “starting sandbox…”.

Agent / workspace / platform-rules data lives on a named volume (`/app/data`); `./start.sh restart` and `./start.sh down` (without `-v`) keep it. **Do not run `docker compose down -v`** — that wipes Agent config and SQLite.

## Common commands

```bash
./start.sh logs
./start.sh down
./start.sh pull          # refresh GHCR images (including four sandbox runtimes)
./start.sh restart       # down + up -d (keeps named volumes)
./start.sh dev -d        # source stack: go run + Vite HMR
```

Image tags / digests can be overridden in `.env` — see [`.env.example`](https://github.com/cocofhu/approving/blob/main/.env.example) at the repo root. By default sandbox images follow acpBackend; set `SANDBOX_IMAGE` / `APPROVING_SANDBOX_IMAGE` only for an optional global force. Publish and smoke checks are covered in [Contributing](https://github.com/cocofhu/approving/blob/main/CONTRIBUTING.md).

## Next steps

- [Core concepts](../concepts/) — FSM, gates, sandbox, artifacts
- [Configuration summary](../../help/configuration/) — points to full `CONFIGURATION.md`
- [Gateway summary](../../help/gateway/) — points to `GATEWAY.md`
