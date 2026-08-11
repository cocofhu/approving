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

Agent / workspace / platform-rules and SQLite data live under `.localdata` at the repo root (bind mounts: `gateway` / `db` / `app-data`). `./start.sh restart` and `./start.sh down` keep that directory. To wipe: `./start.sh down && rm -rf .localdata`.

### Database and attachments share one lifecycle (backup / cleanup)

In the release stack (`compose.release.yaml`), **SQLite** is mounted at `./.localdata/db` and **app data (including default attachment blobs under `data/blobs` relative to `WORKDIR`)** at `./.localdata/app-data`. Composite variable images keep only `blob:{id}` refs in the DB / Run outputs; bytes live under the blobs directory. Backing up or cleaning only one side creates orphan refs (“ref still present, GET `/api/blobs/:id` → 404”), and Run detail shows **Cannot display / Attachment unavailable**.

Ops rules:

- **Paired backup**: every backup set must include both `./.localdata/db` and `./.localdata/app-data` (or the whole `.localdata` tree).
- **Paired cleanup / migration / upgrade**: never move only SQLite or delete only the blobs directory; if you override `APPROVING_BLOBS_ROOT`, include that path in the same lifecycle as the database.
- **Historical orphans**: broken refs are not guaranteed recoverable; the UI only shows a permanent-failure placeholder. This delivery does **not** add an orphan inspection console, bulk scan page, or startup/health-check alerts.

## Common commands

```bash
./start.sh logs
./start.sh down
./start.sh pull          # refresh GHCR images (including four sandbox runtimes)
./start.sh restart       # down + up -d (keeps .localdata)
./start.sh dev -d        # source stack: go run + Vite HMR
```

Image tags / digests can be overridden in `.env` — see [`.env.example`](https://github.com/cocofhu/approving/blob/main/.env.example) at the repo root. By default sandbox images follow acpBackend; set `SANDBOX_IMAGE` / `APPROVING_SANDBOX_IMAGE` only for an optional global force. Publish and smoke checks are covered in [Contributing](https://github.com/cocofhu/approving/blob/main/CONTRIBUTING.md).

## Next steps

- After login, an empty project can use **Project quick start** to configure backend + API Key and generate 5 Agents plus the published light workflow (default git: public Heroku nodejs-getting-started).
- [Core concepts](../concepts/) — FSM, gates, sandbox, artifacts
- [Configuration summary](../../help/configuration/) — points to full `CONFIGURATION.md`
- [Gateway summary](../../help/gateway/) — points to `GATEWAY.md`
