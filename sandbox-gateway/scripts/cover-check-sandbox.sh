#!/usr/bin/env bash
# Sandbox coverage gate: unit-testable packages (auth / backend* / config / correl).
# Excludes acp/qqbot/service/handler/router (WS/CLI integration surfaces covered via e2e).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
MIN="${1:-90}"
cd "$ROOT/sandbox"
COVER="$(mktemp)"; trap 'rm -f "$COVER"' EXIT
PKGS=(./internal/auth ./internal/backend/... ./internal/config ./internal/correl)
LIST="$(go list "${PKGS[@]}" | paste -sd, -)"
go test "${PKGS[@]}" -count=1 -coverpkg="$LIST" -coverprofile="$COVER"
TOTAL="$(go tool cover -func="$COVER" | awk '/^total:/{gsub(/%/,"",$NF); print $NF}')"
echo "sandbox-core coverage=${TOTAL}% (min=${MIN}%)"
awk -v t="$TOTAL" -v m="$MIN" 'BEGIN{ if ((t+0)<(m+0)) { print "FAIL"; exit 1 } print "OK" }'
