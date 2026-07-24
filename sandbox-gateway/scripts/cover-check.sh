#!/usr/bin/env bash
# Enforce Go coverage ≥ MIN% on a filtered cover profile (excludes cmd / fake helpers).
# Usage: cover-check.sh <module-dir> <min-percent> [exclude-regex]
set -euo pipefail
DIR="${1:?module dir}"
MIN="${2:?min percent e.g. 90}"
EXCLUDE="${3:-/cmd/|/driver/fake/}"
cd "$DIR"
COVER="$(mktemp)"
GATE="$(mktemp)"
trap 'rm -f "$COVER" "$GATE"' EXIT

go test ./... -count=1 -coverprofile="$COVER"
{ head -1 "$COVER"; grep -vE "$EXCLUDE" "$COVER" | grep -v '^mode:' || true; } >"$GATE"
TOTAL="$(go tool cover -func="$GATE" | awk '/^total:/{gsub(/%/,"",$NF); print $NF}')"
echo "coverage=${TOTAL}% (gate min=${MIN}%, exclude=${EXCLUDE})"
awk -v t="$TOTAL" -v m="$MIN" 'BEGIN{ if ((t+0) < (m+0)) { print "FAIL: coverage below threshold"; exit 1 } print "OK: coverage meets threshold" }'
