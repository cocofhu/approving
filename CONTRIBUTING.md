# Contributing to Approving

Before contributing, read [`CODE_OF_CONDUCT.md`](CODE_OF_CONDUCT.md) and
[`SECURITY.md`](SECURITY.md).

## Development setup

- Backend: `cd server && go test ./...`
- Web: `cd web && npm ci && npm test && npx vue-tsc --noEmit`
- Configuration doc: `cd server && go run ./cmd/gen-configdoc -check`
- Full local development stack: `./start.sh` or `docker compose up --build`
  (gateway sources live in `sandbox-gateway/`). The digest-pinned release stack
  remains `compose.release.yaml` + `./release-smoke.sh`.

Never commit credentials, local configuration, generated databases, or
organization-only URLs to public examples.

## Changes and pull requests

1. Create a focused branch from the current default branch.
2. Add tests for behavior changes and update `README.md` /
   `server/README.md` / `GATEWAY.md` / `server/CONFIGURATION.md` when public
   behavior, commands, configuration, or links change.
3. Run the relevant checks above. Generated configuration docs must be current
   (`go run ./cmd/gen-configdoc`).
4. Use a concise commit subject describing why the change is needed.
5. Open a pull request with the problem, solution, risk, and test evidence.

Keep pull requests reviewable. Avoid unrelated formatting or refactors.
Maintainers may ask for a smaller follow-up when a proposal crosses security,
compatibility, or release-contract boundaries.

## Reporting problems

Use the issue templates for reproducible defects and feature proposals. Do not
include secrets or vulnerability details. Security reports follow
[`SECURITY.md`](SECURITY.md).
