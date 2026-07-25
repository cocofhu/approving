# Sandbox Gateway API

The gateway is a thin control plane. It creates, exposes, and destroys sandboxes
(instances of the Phase 1 universal image) and returns the client-reachable
address for each exposed port. It does **not** proxy or handle any data-plane
traffic: sessions (`8765`), code-server (`8744`), SSH (`22`), CDP (`9222`), and
noVNC (`6080`) are all direct client-to-sandbox connections.

- Base path: `/api/v1`
- Auth: `Authorization: Bearer <apiKey>` (when `auth.apiKeys` is configured)
- Data-plane contract for `session` (`8765`) is WSP/1, documented in
  [`../../sandbox/docs/PROTOCOL.md`](../../sandbox/docs/PROTOCOL.md).

A gateway instance runs one driver: `docker` (local testing) or `kubernetes`
(production, MetalLB LoadBalancer). The API surface is identical for both; only
the returned endpoint addresses differ (Docker host IP vs. LB IP).

## Health

```
GET /healthz  ->  200 {"status":"ok","driver":"docker"}
```

No auth. Useful for readiness/liveness checks.

## Create a sandbox

```
POST /api/v1/sandboxes
Content-Type: application/json
Authorization: Bearer <key>
```

Body (all fields optional):

```json
{
  "image": "universal-sandbox-cursor:local",
  "provider": "gemini",
  "env": {
    "ACP_BACKEND": "cursor",
    "GIT_REPOS": "app|https://github.com/acme/app|main",
    "ROOT_PASSWORD": "toor",
    "ACP_BRIDGE_PASSWORD": "s3cret"
  },
  "labels": {"owner": "team-a"},
  "workspaceDir": "/root/workspace",
  "ports": [3000, 5173],
  "mounts": ["/host/cache:/root/.cache:rw"],
  "resources": {
    "cpuCores": 2,
    "memoryMB": 4096,
    "diskGi": 160
  },
  "config": {
    "configRoot": "/root/.cursor",
    "hostPath": "/host/agent-config",
    "bundleUrl": "https://.../config.tar.gz",
    "headers": "Authorization: Bearer <token>"
  }
}
```

- `provider` selects the agent CLI (e.g. `cursor`, `gemini`, `codex`, `codebuddy`).
  The gateway resolves it to a per-agent image (`image.byProvider` / `image.template`
  in config, or `SBGW_IMAGE_TEMPLATE` / `SBGW_IMAGE_MAP` env) and injects
  `AGENT_PROVIDER`/`ACP_BACKEND` into the sandbox env when not already set.
  Ignored when `image` is given explicitly.
- `env` is the injection channel to the image (see the sandbox README for the
  full variable reference: `WORKSPACE_DIR`, `GIT_REPOS`, `ACP_BACKEND`,
  `VNC_PREVIEW`, `BROWSER_MCP`, `ROOT_PASSWORD`, `SSH_KEY`, etc.).
- `ports` adds application ports on top of the image defaults.
- `resources` sets per-sandbox limits (same knobs as remote-dev UI):
  - `cpuCores` — CPU limit in cores (maps to k8s `limits.cpu` / docker `--cpus`)
  - `memoryMB` — memory limit in MiB (k8s `limits.memory` / docker `--memory`)
  - `diskGi` — data PVC size in GiB (kubernetes only; docker ignores)
  - omit or `0` → gateway defaults from `kubernetes.*` in config
  - values above `maxCPUCores` / `maxMemoryMB` / `maxDataDiskGi` → `400`
- `config` seeds rules/skills/mcp before services start. Use `hostPath` for the
  Docker driver (same-host bind-mount) or `bundleUrl` (+ optional `headers`)
  which is translated to the image's `SANDBOX_INJECT` contract.
- `mounts` is docker-only.

Response `202 Accepted` as soon as the control-plane record is persisted.
Driver provisioning (k8s Namespace/Secret/PVC/Deployment/LB, or `docker run`)
and session readiness finalize run in the background — poll `GET /sandboxes/:id`
until `status` is `running` or `error`. Do not treat a slow PVC/LB as an HTTP
timeout on this POST.

`config.hostPath` is **docker-only** (same-host bind-mount). Against a remote
kubernetes gateway it is ignored; use `config.bundleUrl` (+ optional `headers`)
so the image pulls the seed archive via `SANDBOX_INJECT`.

Response body (initial `status` is usually `creating`):

```json
{
  "id": "a1b2c3d4e5f6",
  "name": "sbx-a1b2c3d4e5f6",
  "status": "creating",
  "image": "universal-sandbox-cursor:local",
  "resources": {"cpuCores": 2, "memoryMB": 4096, "diskGi": 160},
  "endpoints": {}
}
```

The gateway backfills `endpoints` and flips `status` to `running` once the
sandbox is reachable (LB IP assigned where applicable, session port accepting
connections). Poll `GET /sandboxes/:id` until `status == "running"`.

## List sandboxes

```
GET /api/v1/sandboxes
GET /api/v1/sandboxes?label=owner:team-a
GET /api/v1/sandboxes?label=owner:team-a&label=env:prod
```

Returns `{"sandboxes":[ {sandbox}, ... ]}` newest first.

- Optional repeated `label=key:value` filters; all must match (AND) against the
  sandbox `labels` map written at create time.
- Value may contain `:` (only the first `:` separates key from value).
- Invalid `label` (missing `:` or empty key) → `400`.

## Get a sandbox

```
GET /api/v1/sandboxes/:id
```

```json
{
  "id": "a1b2c3d4e5f6",
  "name": "sbx-a1b2c3d4e5f6",
  "status": "running",
  "image": "universal-sandbox-cursor:local",
  "resources": {"cpuCores": 2, "memoryMB": 4096, "diskGi": 160},
  "endpoints": {
    "session": "10.0.0.21:8765",
    "ide": "10.0.0.21:8744",
    "ssh": "10.0.0.21:22",
    "cdp": "10.0.0.21:9222",
    "novnc": "10.0.0.21:6080",
    "8765": "10.0.0.21:8765",
    "8744": "10.0.0.21:8744",
    "22": "10.0.0.21:22"
  }
}
```

`endpoints` carries both friendly names (`session`/`ide`/`ssh`/`cdp`/`novnc`)
and raw port keys. Clients connect directly to these addresses.

## Lifecycle

```
POST   /api/v1/sandboxes/:id/start      # docker start / k8s scale=1
POST   /api/v1/sandboxes/:id/stop       # docker stop  / k8s scale=0 (retained)
POST   /api/v1/sandboxes/:id/reinstall  # rebuild Pod; optional PVC wipe
DELETE /api/v1/sandboxes/:id            # remove sandbox + record
GET    /api/v1/sandboxes/:id/status     # {"status":"running|stopped|not_found|..."}
```

### Reinstall

```
POST /api/v1/sandboxes/:id/reinstall
Content-Type: application/json
Authorization: Bearer <key>

{"preserveData": true}
```

Aligned with remote-dev「重装环境」:

| `preserveData` | 行为 |
|----------------|------|
| `true` | 不删 PVC（k8s）/ 匿名卷（docker），只重建容器；工作区与缓存保留 |
| `false` / 省略 | 删除数据卷后重建（工作区、docker 层等清空） |

Host 挂载的共享配置（如 `config.hostPath` → `.cursor`）不会被本接口删除。
响应 `202 Accepted`，随后异步探测就绪（同 create），轮询 `GET /sandboxes/:id` 直至 `running`。

## Single-port lookup

```
GET /api/v1/sandboxes/:id/hosts/:port  ->  {"port":8744,"address":"10.0.0.21:8744"}
```

## Status values

| status     | meaning                                                    |
|------------|------------------------------------------------------------|
| `creating` | record persisted, resource provisioning / not yet ready    |
| `running`  | ready; `endpoints` populated                               |
| `stopped`  | stopped but retained (restartable via `start`)             |
| `error`    | provisioning or reconcile failed (see `error` field)       |

## Container logs (read-only)

```
GET /api/v1/sandboxes/:id/logs
GET /api/v1/sandboxes/:id/logs?tail=5000
```

Returns the sandbox PID1 combined stdout/stderr as a synchronous JSON body
(non-follow). Used for infrastructure / boot troubleshooting — not a substitute
for the agent execution event log.

```json
{"content": "[boot] sandbox container started\n…"}
```

| Query | Default | Notes |
|-------|---------|-------|
| `tail` | `5000` | Lines from the end of the log stream (`docker logs --tail`) |

- **Docker driver**: implemented via `docker logs --tail` (stdout+stderr).
- **Kubernetes driver**: returns `501` (`sandbox logs not supported by this driver`).
- Missing sandbox → `404`. Driver / docker CLI failure → `500` with `error`.

## What the gateway does NOT do

- No `exec` / file / terminal endpoints. Run commands and move files by
  connecting directly to the sandbox **SSH (`22`)** or the WSP/1 session.
- No reverse proxy for code-server, session, or preview. Clients connect to the
  returned endpoint addresses directly.
- No streaming / follow logs (`?follow=1` / SSE). Clients re-fetch on demand.
- No kubernetes log retrieval in this release (explicit `501`).
- code-flow's former `/api/changes` is gone from the image; compute "changes"
  by running `git` over a direct SSH/exec connection.
