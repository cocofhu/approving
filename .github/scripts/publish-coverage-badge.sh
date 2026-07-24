#!/usr/bin/env bash
# Publish one shields endpoint JSON file to the coverage-badges branch.
# Only call on default-branch success paths. Failure/skip should not invoke this
# script, so the previous successful endpoint snapshot is retained.
#
# Usage: publish-coverage-badge.sh <filename> <label> <percent> <color>
# Example: publish-coverage-badge.sh coverage-web.json coverage-web 82 green
set -euo pipefail

filename="${1:?filename}"
label="${2:?label}"
percent="${3:?percent}"
color="${4:?color}"

: "${GITHUB_TOKEN:?GITHUB_TOKEN is required}"
: "${GITHUB_REPOSITORY:?GITHUB_REPOSITORY is required}"

# Normalize message to integer percent with % suffix (e.g. 82%).
message="$(awk -v p="$percent" 'BEGIN{printf "%.0f%%", p+0}')"

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

# Clone only the endpoint branch (orphan; no product source).
git clone --depth 1 --branch coverage-badges \
  "https://x-access-token:${GITHUB_TOKEN}@github.com/${GITHUB_REPOSITORY}.git" \
  "$work"

python3 - "$work/$filename" "$label" "$message" "$color" <<'PY'
import json, pathlib, sys
path, label, message, color = sys.argv[1:5]
payload = {
    "schemaVersion": 1,
    "label": label,
    "message": message,
    "color": color,
}
pathlib.Path(path).write_text(json.dumps(payload, indent=2) + "\n", encoding="utf-8")
PY

cd "$work"
git config user.name "github-actions[bot]"
git config user.email "41898282+github-actions[bot]@users.noreply.github.com"
git add "$filename"
if git diff --cached --quiet; then
  echo "coverage badge unchanged: ${filename} (${message})"
  exit 0
fi
git commit -m "chore: update ${label} badge to ${message}"
git push origin HEAD:coverage-badges
echo "published ${filename}: ${label}=${message} color=${color}"
