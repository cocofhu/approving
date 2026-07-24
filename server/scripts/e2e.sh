#!/usr/bin/env bash
# Local runner for the real-sandbox end-to-end tests.
#
# These are the APPROVING_LIVE-gated tests in internal/runtime/: they launch a
# real approving-sandbox container via Docker, drive cursor-agent over ACP, and
# assert the declared `produces` file is harvested back. The default `go test`
# run stays credential-free on the mock provider; this script opts into the
# live path. It mirrors the CI `server:e2e` job (see .github/workflows/ci-server.yml) but talks
# to your local Docker daemon instead of a dind sidecar.
#
# Usage:
#   APPROVING_CURSOR_API_KEY=crsr_xxx server/scripts/e2e.sh
#   APPROVING_CURSOR_API_KEY=crsr_xxx APPROVING_SANDBOX_IMAGE=approving-sandbox:dev \
#     server/scripts/e2e.sh -run TestCursorLiveRunAgent
#
# Any extra args are passed straight through to `go test`.
set -euo pipefail

# Resolve the server module dir (this script lives in server/scripts/).
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SERVER_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"

fail() { echo "error: $*" >&2; exit 1; }

command -v go >/dev/null 2>&1 || fail "go is not installed / not on PATH"
command -v docker >/dev/null 2>&1 || fail "docker is not installed / not on PATH"
docker info >/dev/null 2>&1 || fail "docker daemon is not reachable (is Docker running?)"

[ -n "${APPROVING_CURSOR_API_KEY:-}" ] || fail "APPROVING_CURSOR_API_KEY is required (a crsr_... Cursor API key)"

# Default to the locally built dev sandbox image (see ./start.sh sandbox). Set
# APPROVING_SANDBOX_IMAGE to override (e.g. the published :stable image).
export APPROVING_SANDBOX_IMAGE="${APPROVING_SANDBOX_IMAGE:-approving-sandbox:dev}"
export APPROVING_LIVE=1

if ! docker image inspect "$APPROVING_SANDBOX_IMAGE" >/dev/null 2>&1; then
  echo "sandbox image '$APPROVING_SANDBOX_IMAGE' not present locally; attempting docker pull..."
  docker pull "$APPROVING_SANDBOX_IMAGE" || fail "could not find or pull sandbox image '$APPROVING_SANDBOX_IMAGE' (build it with: ./start.sh sandbox)"
fi

# Default test selection: the sandbox run-agent e2e. Callers can override the
# -run filter (and add -v, -timeout, etc.) via extra args.
if [ "$#" -eq 0 ]; then
  set -- -run TestCursorLiveRunAgent -v -count=1 -timeout 30m
fi

echo "running live e2e (image=$APPROVING_SANDBOX_IMAGE) ..."
cd "$SERVER_DIR"
exec go test ./internal/runtime/ "$@"
