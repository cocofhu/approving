# sandbox-gateway contract

Approving vendors [sandbox-gateway](sandbox-gateway/) in this repository. The
control plane creates and destroys sandboxes; the data plane uses SSH endpoints
returned by the gateway.

Source layout:

- `sandbox-gateway/gateway/` — Go control plane
- `sandbox-gateway/sandbox/` — universal sandbox image
- `sandbox-gateway/deploy/config/config.local.yaml` — local compose config

## Local stack

From the repo root (Linux host with Docker Compose):

```bash
./start.sh -d
# or: docker compose up --build -d
```

This starts gateway (`:8899`), Approving server (`:8080`), and the web UI
(`:5173`). The first run may build `universal-sandbox-cursor:local` from
`sandbox-gateway/sandbox` (slow).

## Minimum compatible API

| Capability | Contract |
| --- | --- |
| Health | `GET /healthz` returns 2xx |
| Create | `POST /api/v1/sandboxes` accepts image, env, labels, ports, resources, config.bundleUrl; response `202`, status often `creating` |
| Get | `GET /api/v1/sandboxes/{id}` returns status and endpoints (session/ide/ssh/cdp/novnc) |
| List | `GET /api/v1/sandboxes?label=key:value` (AND) |
| Delete | `DELETE /api/v1/sandboxes/{id}` returns 2xx |
| Ready | status `running` with a `session` endpoint |
| Images | per Agent `acpBackend`: `universal-sandbox-{cursor\|claude_code\|codebuddy\|trae}` |
| Data plane | SSH / session / cdp / novnc connect to endpoints directly |
| Auth | optional Bearer token via `APPROVING_SANDBOX_GATEWAY_API_KEY` |

`approving doctor --run-demo` verifies health, create, ready, and cleanup after failure.

## Published images

Release tags may also publish digest-pinned images to `ghcr.io/cocofhu/...`.
`compose.release.yaml` still requires explicit immutable references for the
clean-Linux smoke path.
