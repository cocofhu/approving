---
title: Gateway
description: sandbox-gateway contract summary; full details in GATEWAY.md.
---

Approving schedules the generic sandbox image through the vendored **sandbox-gateway** control plane. The Web UI talks to the Approving API; agent / react nodes execute in containers via the gateway.

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
