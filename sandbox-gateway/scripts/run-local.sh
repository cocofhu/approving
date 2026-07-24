#!/usr/bin/env bash
# 本机无鉴权启动 gateway（不依赖 compose 镜像拉取；直连宿主机 Docker）
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT/gateway"
BIN="${ROOT}/bin/sandbox-gateway"
mkdir -p "$ROOT/bin" "$ROOT/.data"
if [[ ! -x "$BIN" ]] || [[ "$ROOT/gateway" -nt "$BIN" ]]; then
  echo "building $BIN ..."
  CGO_ENABLED=0 go build -ldflags="-s -w" -o "$BIN" ./cmd/server
fi
export SBGW_CONFIG="${SBGW_CONFIG:-$ROOT/deploy/config/config.local.yaml}"
export SBGW_DB_PATH="${SBGW_DB_PATH:-$ROOT/.data/gateway.db}"
echo "listen :8080  config=$SBGW_CONFIG  db=$SBGW_DB_PATH  (auth disabled)"
exec "$BIN" -config "$SBGW_CONFIG"
