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

`./start.sh` also pulls the **sandbox runtime** image from GHCR (large). Until that finishes, sandbox chats may stay on “starting sandbox…”.

## Common commands

```bash
./start.sh logs
./start.sh down
./start.sh pull          # refresh GHCR images
./start.sh dev -d        # source stack: go run + Vite HMR
```

Image tags / digests can be overridden in `.env` — see [`.env.example`](https://github.com/cocofhu/approving/blob/main/.env.example) at the repo root. Publish and smoke checks are covered in [Contributing](https://github.com/cocofhu/approving/blob/main/CONTRIBUTING.md).

## Next steps

- [Core concepts](../concepts/) — FSM, gates, sandbox, artifacts
- [Configuration summary](../../help/configuration/) — points to full `CONFIGURATION.md`
- [Gateway summary](../../help/gateway/) — points to `GATEWAY.md`
