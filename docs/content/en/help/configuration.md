---
title: Configuration
description: Configuration highlights; full details live in source CONFIGURATION.md.
---

Approving server configuration is primarily YAML / environment variables (local examples: `server/config.example.yaml` and the root `.env.example`).

## Full documentation

Treat the in-repo docs as authoritative (avoids drift between this site and source):

- [server/CONFIGURATION.md](https://github.com/cocofhu/approving/blob/main/server/CONFIGURATION.md)

That document is generated/checked by `go run ./cmd/gen-configdoc`; CI runs `-check`.

## Local quick path

```bash
./start.sh -d          # published image stack
./start.sh dev -d      # source + HMR
```

Image tags / digests, gateway, and sandbox-related variables are in `.env.example`. Agent API keys, `GITHUB_*` / `GITLAB_*` / SSH, and similar belong in agent meta env (values may reference `${vars.<name>}`).

## Related

- [Gateway](../gateway/)
- [Contributing](../contributing/)
