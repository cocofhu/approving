#!/usr/bin/env bash
# Mirror GitLab CI server:test for local/agent runs.
# Cold CGO (mattn/go-sqlite3) can burn several minutes before any test starts;
# engine alone may need ~5–6m on slow hosts. Default go test -timeout 3m will FAIL.
set -euo pipefail
cd "$(dirname "$0")/.."
export CGO_ENABLED="${CGO_ENABLED:-1}"
export GOPROXY="${GOPROXY:-https://goproxy.cn,direct}"
export GOSUMDB="${GOSUMDB:-sum.golang.google.cn}"

echo "==> warm CGO sqlite (database package)"
go test -c -o /tmp/approving-database.test ./internal/database/

TIMEOUT="${TEST_TIMEOUT:-20m}"
echo "==> go test ./... -count=1 -timeout ${TIMEOUT}"
go test ./... -count=1 -timeout "${TIMEOUT}" "$@"
