#!/usr/bin/env bash
# Map Lines coverage percent → shields.io endpoint color (discrete bands).
# Bands (aligned with Demo high/mid readability):
#   >=85  → green  (high)
#   70–84 → yellow (mid)
#   <70   → orange (low)
set -euo pipefail
pct="${1:?usage: coverage-badge-color.sh <percent>}"
# Accept integers or decimals; compare numerically.
if awk -v p="$pct" 'BEGIN{exit !(p+0 >= 85)}'; then
  echo green
elif awk -v p="$pct" 'BEGIN{exit !(p+0 >= 70)}'; then
  echo yellow
else
  echo orange
fi
