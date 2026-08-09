---
title: Gateway
description: sandbox-gateway contract summary; full details in GATEWAY.md.
---

Approving schedules the generic sandbox image through the vendored **sandbox-gateway** control plane. The Web UI talks to the Approving API; agent / react nodes execute in containers via the gateway.

## Direct endpoints vs platform proxy

- **Direct (already authenticated)**: `session` (session password), `ide` (IDE password), `ssh` (`ROOT_PASSWORD` or `SSH_KEY`).
- **Not direct**: CDP `:9222` and noVNC `:6080` have no app-layer auth, are not published to the host/LB, and are not shown or copied on the sandbox detail page.
- **User path**: platform proxies only — `/sandbox-vnc/:sandboxId/ws` and `/preview-vnc/:runId/:nodeId/:port/ws` (Session required when platform Auth is enabled). “Open preview” opens sandbox console noVNC; it does not dial websockify.

## Full documentation

- [GATEWAY.md](https://github.com/cocofhu/approving/blob/main/GATEWAY.md)

## Health checks (default local stack)

- Gateway: http://localhost:8899/healthz
- Approving API: http://localhost:8080/api/health

## Source locations

- Control plane: `sandbox-gateway/gateway/`
- Sandbox image and scripts: `sandbox-gateway/sandbox/`, `sandbox-gateway/scripts/`

## Related

- [Configuration](../configuration/)
- [Core concepts](../../guide/concepts/)
