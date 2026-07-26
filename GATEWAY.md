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
./start.sh -d          # default: published GHCR images (compose.release.yaml)
./start.sh dev -d      # source/HMR stack (docker-compose.yml)
```

Two paths, different UI ports:

| Mode | Command | Gateway | API | UI |
| --- | --- | --- | --- | --- |
| Release (default) | `./start.sh -d` | `:8899` | `:8080` | `:8080` (served with the API) |
| Dev / source | `./start.sh dev -d` | `:8899` | `:8080` | `:5173` (Vite) |

Release mode pulls GHCR images (including the large sandbox runtime). Dev mode may
build `universal-sandbox-cursor:local` from `sandbox-gateway/sandbox` on first run
(slow).

## Minimum compatible API

| Capability | Contract |
| --- | --- |
| Health | `GET /healthz` returns 2xx |
| Create | `POST /api/v1/sandboxes` accepts image, env, labels, ports, resources, config.bundleUrl; response `202`, status often `creating` |
| Get | `GET /api/v1/sandboxes/{id}` returns status and endpoints (session/ide/ssh/cdp/novnc) |
| List | `GET /api/v1/sandboxes?label=key:value` (AND) |
| Delete | `DELETE /api/v1/sandboxes/{id}` returns 2xx |
| Logs | `GET /api/v1/sandboxes/{id}/logs?tail=` returns `{content}` (PID1 stdout/stderr, non-follow). Docker (`docker logs --tail`) and kubernetes (pod `sandbox` container via client-go GetLogs) both supported. Cluster RBAC must allow `get` on `pods/log` in the sandbox namespace; the incremental Role+RoleBinding is shipped in `sandbox-gateway/deploy/k8s/` (apply alongside existing Roles — do not replace a full production Role). Drivers that still omit Logs → `501` |
| Ready | status `running` with a `session` endpoint |
| Images | per Agent `acpBackend`: `universal-sandbox-{cursor\|claude_code\|codebuddy\|trae}` |
| Data plane | SSH / session / cdp / novnc connect to endpoints directly |
| Auth | Bearer token: gateway `SBGW_API_KEYS` / client `APPROVING_SANDBOX_GATEWAY_API_KEY` (compose default `approving-local-demo` via `SANDBOX_GATEWAY_API_KEY`) |

`approving doctor --run-demo` verifies health, create, ready, and cleanup after failure.

## Published images

Release tags may also publish digest-pinned images to `ghcr.io/cocofhu/...`.
`compose.release.yaml` still requires explicit immutable references for the
clean-Linux smoke path.
