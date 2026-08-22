#!/usr/bin/env bash
# Install or refresh the sandbox artifact-upload CLI from this repo checkout.
# Use when the control plane has not yet re-seeded /usr/local/bin (old embedded
# copy still calling write_artifact kind=image). Idempotent.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SRC="$ROOT/scripts/artifact-upload"
DEST="/usr/local/bin/artifact-upload"
if [[ ! -f "$SRC" ]]; then
  echo "install-artifact-upload: missing $SRC" >&2
  exit 1
fi
install -m 755 "$SRC" "$DEST"
if grep -q 'upload_image_artifact' "$DEST"; then
  echo "install-artifact-upload: installed $DEST (upload_image_artifact channel)"
else
  echo "install-artifact-upload: $DEST missing upload_image_artifact — check $SRC" >&2
  exit 1
fi
