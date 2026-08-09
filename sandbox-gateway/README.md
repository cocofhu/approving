# sandbox-gateway (vendored)

Control plane for Approving sandboxes. This tree lives inside the Approving
repository so a single clone can run the full stack with Docker Compose.

- Gateway API: `gateway/`
- Universal sandbox image: `sandbox/`
- Local config: `deploy/config/config.local.yaml`

Public data-plane ports: session / ide / ssh / app. CDP `:9222` and noVNC `:6080`
stay on the container or ClusterIP network (no host/LB publish). Users reach
noVNC only through Approving VNC WebSockets. See `SECURITY.md` and `GATEWAY.md`
in the Approving repo root.

## Build

```bash
# Gateway control plane
docker build -t sandbox-gateway:local -f Dockerfile .

# Default sandbox image used by local compose (Cursor ACP)
docker build -t universal-sandbox-cursor:local \
  --build-arg AGENT_PROVIDER=cursor \
  -f sandbox/Dockerfile sandbox
```

## Standalone (optional)

```bash
docker compose up --build -d   # from this directory; listens on :8080 by default
curl -s localhost:8080/healthz
```

From the Approving repo root, prefer `./start.sh` or root `docker compose up --build`,
which wires gateway on `:8899` next to the Approving server.
