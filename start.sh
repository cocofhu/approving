#!/usr/bin/env bash
# Start Approving from published GHCR images (default), or the local source
# stack for development.
#
# Usage:
#   ./start.sh            foreground (pull GHCR + up)
#   ./start.sh -d          detached
#   ./start.sh logs        follow logs
#   ./start.sh down        stop and remove containers
#   ./start.sh restart     down + up -d
#   ./start.sh pull        docker compose pull
#   ./start.sh dev         local source stack (build server/web/gateway)
#   ./start.sh dev -d      local source stack, detached
#   ./start.sh sandbox     build universal-sandbox-cursor:local (dev only)
#   ./start.sh gateway     rebuild local sandbox-gateway image (dev only)
#
# Requires Docker Compose on a Linux host (services use network_mode: host).
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")"
export HOST_REPO_DIR="$(pwd)"

if ! command -v docker >/dev/null 2>&1; then
  echo "error: docker not found" >&2
  exit 1
fi

if docker compose version >/dev/null 2>&1; then
  COMPOSE=(docker compose)
elif command -v docker-compose >/dev/null 2>&1; then
  COMPOSE=(docker-compose)
else
  echo "error: docker compose / docker-compose not found" >&2
  exit 1
fi

if [[ ! -f .env ]]; then
  cp .env.example .env
  echo "created .env from .env.example"
fi

set -a
# shellcheck disable=SC1091
source .env
set +a

# Defaults so a bare clone can start without editing .env.
: "${APPROVING_PORT:=8080}"
: "${APPROVING_GATEWAY_PORT:=8899}"
: "${APPROVING_SANDBOX_GATEWAY_URL:=http://127.0.0.1:${APPROVING_GATEWAY_PORT}}"
: "${APPROVING_DEPLOYMENT_MODE:=local-demo}"
: "${APPROVING_IMAGE:=ghcr.io/cocofhu/approving:0.3.2-beta}"
: "${SANDBOX_GATEWAY_IMAGE:=ghcr.io/cocofhu/sandbox-gateway:0.3.2-beta}"
: "${SANDBOX_GATEWAY_API_KEY:=approving-local-demo}"

# Optional global force: capture user-set SANDBOX_IMAGE BEFORE applying the
# cursor fallback default, so a bare default does not re-force all backends.
_user_sandbox_image="${SANDBOX_IMAGE-}"
: "${SANDBOX_IMAGE:=ghcr.io/cocofhu/universal-sandbox-cursor:0.3.2-beta}"
: "${APPROVING_SANDBOX_IMAGE_CURSOR:=ghcr.io/cocofhu/universal-sandbox-cursor:0.3.2-beta}"
: "${APPROVING_SANDBOX_IMAGE_CLAUDE_CODE:=ghcr.io/cocofhu/universal-sandbox-claude_code:0.3.2-beta}"
: "${APPROVING_SANDBOX_IMAGE_CODEBUDDY:=ghcr.io/cocofhu/universal-sandbox-codebuddy:0.3.2-beta}"
: "${APPROVING_SANDBOX_IMAGE_TRAE:=ghcr.io/cocofhu/universal-sandbox-trae:0.3.2-beta}"
: "${SBGW_IMAGE_TEMPLATE:=ghcr.io/cocofhu/universal-sandbox-{provider}:0.3.2-beta}"
# Explicit SANDBOX_IMAGE (or APPROVING_SANDBOX_IMAGE) → global force; default path leaves it empty.
if [[ -z "${APPROVING_SANDBOX_IMAGE:-}" && -n "${_user_sandbox_image}" ]]; then
  APPROVING_SANDBOX_IMAGE="${_user_sandbox_image}"
fi

# Demo account (admin / demo1234). Set outside the .env file so `$` in the
# bcrypt hash is not eaten by shell/compose env parsing.
if [[ -z "${APPROVING_AUTH_USERS:-}" ]]; then
  APPROVING_AUTH_USERS='[{"username":"admin","password_hash":"$2a$10$EY.SdHq0p6drMz6U9JVrz.Kq0jNkg7TWmsVUFLtB1dL1yIelDkITi","is_admin":true}]'
fi

if [[ -z "${APPROVING_DOCTOR_TOKEN:-}" ]]; then
  APPROVING_DOCTOR_TOKEN="$(od -An -N32 -tx1 /dev/urandom | tr -d ' \n')"
fi

export APPROVING_PORT APPROVING_GATEWAY_PORT APPROVING_SANDBOX_GATEWAY_URL
export APPROVING_DEPLOYMENT_MODE APPROVING_IMAGE SANDBOX_GATEWAY_IMAGE
export SANDBOX_IMAGE SANDBOX_GATEWAY_API_KEY APPROVING_AUTH_USERS APPROVING_DOCTOR_TOKEN
export APPROVING_SANDBOX_IMAGE_CURSOR APPROVING_SANDBOX_IMAGE_CLAUDE_CODE
export APPROVING_SANDBOX_IMAGE_CODEBUDDY APPROVING_SANDBOX_IMAGE_TRAE
export SBGW_IMAGE_TEMPLATE
# May be empty (no global force). Export so compose substitutes ${APPROVING_SANDBOX_IMAGE:-}.
export APPROVING_SANDBOX_IMAGE="${APPROVING_SANDBOX_IMAGE:-}"
# Approving client uses the dedicated env name; keep it in sync with the gateway.
export APPROVING_SANDBOX_GATEWAY_API_KEY="${APPROVING_SANDBOX_GATEWAY_API_KEY:-$SANDBOX_GATEWAY_API_KEY}"
export SBGW_API_KEYS="${SBGW_API_KEYS:-$SANDBOX_GATEWAY_API_KEY}"

# Optional stamp for the source stack (`go run` + Dockerfile.dev ldflags).
# Unset / failed rev-parse → empty; overview badge stays hidden (allowed).
if [[ -z "${GIT_COMMIT:-}" ]] && command -v git >/dev/null 2>&1; then
  GIT_COMMIT="$(git -C "$HOST_REPO_DIR" rev-parse HEAD 2>/dev/null || true)"
fi
export GIT_COMMIT="${GIT_COMMIT:-}"

RELEASE_COMPOSE_FILE="${COMPOSE_FILE:-compose.release.yaml}"
DEV_COMPOSE_FILE="docker-compose.yml"

wait_for_url() {
  local url="$1"
  local label="$2"
  local -a probe=()
  if command -v curl >/dev/null 2>&1; then
    probe=(curl -fsS -o /dev/null "$url")
  elif command -v wget >/dev/null 2>&1; then
    probe=(wget -q -O /dev/null "$url")
  else
    echo "${label}: ${url} (no curl/wget; skip probe)"
    return 0
  fi
  echo -n "waiting ${label} ${url} "
  for _ in $(seq 1 60); do
    if "${probe[@]}" >/dev/null 2>&1; then
      echo "ok"
      return 0
    fi
    echo -n "."
    sleep 1
  done
  echo ""
  echo "warning: ${label} not ready in 60s; check: ./start.sh logs" >&2
  return 0
}

print_release_endpoints() {
  echo "—— UI/API  http://localhost:${APPROVING_PORT}"
  echo "—— health  http://localhost:${APPROVING_PORT}/api/health"
  echo "—— gateway http://127.0.0.1:${APPROVING_GATEWAY_PORT}/healthz"
  echo "—— login   admin / demo1234  (local-demo)"
  echo "—— gateway token  ${SANDBOX_GATEWAY_API_KEY}"
  echo "—— images  ${APPROVING_IMAGE}"
  echo "           ${SANDBOX_GATEWAY_IMAGE}"
  echo "—— sandbox (per backend)"
  echo "           cursor     ${APPROVING_SANDBOX_IMAGE_CURSOR}"
  echo "           claude_code ${APPROVING_SANDBOX_IMAGE_CLAUDE_CODE}"
  echo "           codebuddy  ${APPROVING_SANDBOX_IMAGE_CODEBUDDY}"
  echo "           trae       ${APPROVING_SANDBOX_IMAGE_TRAE}"
  if [[ -n "${APPROVING_SANDBOX_IMAGE:-}" ]]; then
    echo "—— sandbox GLOBAL FORCE  ${APPROVING_SANDBOX_IMAGE}"
  fi
  echo "—— gateway template  ${SBGW_IMAGE_TEMPLATE}"
  echo "—— gateway fallback  ${SANDBOX_IMAGE}"
}

# Sandbox runtime images are NOT compose services — compose pull never fetches them.
# Pull all four GHCR runtimes so per-backend Agents can create sandboxes.
ensure_sandbox_runtime_image() {
  local images=(
    "${APPROVING_SANDBOX_IMAGE_CURSOR}"
    "${APPROVING_SANDBOX_IMAGE_CLAUDE_CODE}"
    "${APPROVING_SANDBOX_IMAGE_CODEBUDDY}"
    "${APPROVING_SANDBOX_IMAGE_TRAE}"
  )
  # Deduplicate while preserving order (global force may equal one backend).
  if [[ -n "${APPROVING_SANDBOX_IMAGE:-}" ]]; then
    images+=("${APPROVING_SANDBOX_IMAGE}")
  fi
  local -A seen=()
  local img
  for img in "${images[@]}"; do
    [[ -n "$img" ]] || continue
    [[ -n "${seen[$img]:-}" ]] && continue
    seen[$img]=1
    echo "pulling sandbox runtime image ${img} (GHCR, several GB)..."
    docker pull "$img"
  done
}

up_release() {
  local detach="${1:-}"
  mkdir -p .localdata/gateway .localdata/db .localdata/app-data
  echo "pulling GHCR images (approving + gateway + sandbox)..."
  "${COMPOSE[@]}" -f "$RELEASE_COMPOSE_FILE" pull
  ensure_sandbox_runtime_image
  if [[ "$detach" == "1" ]]; then
    "${COMPOSE[@]}" -f "$RELEASE_COMPOSE_FILE" up -d
    wait_for_url "http://127.0.0.1:${APPROVING_GATEWAY_PORT}/healthz" "gateway"
    wait_for_url "http://127.0.0.1:${APPROVING_PORT}/api/health" "api"
    echo "started (GHCR)"
    print_release_endpoints
    echo "data: .localdata/{gateway,db,app-data} (bind mounts)"
    echo "wipe: ./start.sh down && rm -rf .localdata"
    echo "logs: ./start.sh logs   stop: ./start.sh down"
  else
    echo "starting (foreground) — UI http://localhost:${APPROVING_PORT}"
    "${COMPOSE[@]}" -f "$RELEASE_COMPOSE_FILE" up
  fi
}

ensure_dev_sandbox_image() {
  local sandbox_image="${APPROVING_GATEWAY_SANDBOX_IMAGE:-universal-sandbox-cursor:local}"
  local gateway_dir="${SANDBOX_GATEWAY_DIR:-./sandbox-gateway}"
  if docker image inspect "$sandbox_image" >/dev/null 2>&1; then
    return 0
  fi
  if [[ ! -f "${gateway_dir}/sandbox/Dockerfile" ]]; then
    echo "error: missing ${gateway_dir}/sandbox/Dockerfile (needed to build ${sandbox_image})" >&2
    exit 1
  fi
  echo "building local sandbox image ${sandbox_image} (first run is slow)..."
  docker build --network=host \
    -t "$sandbox_image" \
    --build-arg AGENT_PROVIDER=cursor \
    -f "${gateway_dir}/sandbox/Dockerfile" \
    "${gateway_dir}/sandbox"
}

up_dev() {
  local detach="${1:-}"
  if [[ ! -f server/config.yaml ]]; then
    cp server/config.example.yaml server/config.yaml
    echo "created server/config.yaml from config.example.yaml"
  fi
  mkdir -p .devdata/db .devdata/sandbox-home
  export APPROVING_SANDBOX_GATEWAY_URL="http://127.0.0.1:${APPROVING_GATEWAY_PORT}"
  ensure_dev_sandbox_image
  if [[ "$detach" == "1" ]]; then
    "${COMPOSE[@]}" -f "$DEV_COMPOSE_FILE" up --build -d
    wait_for_url "http://127.0.0.1:${APPROVING_GATEWAY_PORT}/healthz" "gateway"
    echo "started (dev/source)"
    echo "—— API http://localhost:${APPROVING_PORT}/api/health  UI http://localhost:5173"
    echo "—— gateway http://127.0.0.1:${APPROVING_GATEWAY_PORT}/healthz"
  else
    echo "starting dev stack (foreground) — UI http://localhost:5173"
    "${COMPOSE[@]}" -f "$DEV_COMPOSE_FILE" up --build
  fi
}

cmd="${1:-up}"
shift || true
case "$cmd" in
  up)
    up_release 0
    ;;
  -d|up-d|detach)
    up_release 1
    ;;
  pull)
    "${COMPOSE[@]}" -f "$RELEASE_COMPOSE_FILE" pull
    ensure_sandbox_runtime_image
    ;;
  logs)
    if "${COMPOSE[@]}" -f "$RELEASE_COMPOSE_FILE" ps -q 2>/dev/null | grep -q .; then
      "${COMPOSE[@]}" -f "$RELEASE_COMPOSE_FILE" logs -f "$@"
    else
      "${COMPOSE[@]}" -f "$DEV_COMPOSE_FILE" logs -f "$@"
    fi
    ;;
  gw-logs)
    if "${COMPOSE[@]}" -f "$RELEASE_COMPOSE_FILE" ps -q 2>/dev/null | grep -q .; then
      "${COMPOSE[@]}" -f "$RELEASE_COMPOSE_FILE" logs -f gateway
    else
      "${COMPOSE[@]}" -f "$DEV_COMPOSE_FILE" logs -f gateway
    fi
    ;;
  down)
    "${COMPOSE[@]}" -f "$RELEASE_COMPOSE_FILE" down --remove-orphans || true
    "${COMPOSE[@]}" -f "$DEV_COMPOSE_FILE" down --remove-orphans || true
    ;;
  restart)
    "${COMPOSE[@]}" -f "$RELEASE_COMPOSE_FILE" down --remove-orphans || true
    up_release 1
    ;;
  dev)
    sub="${1:-up}"
    case "$sub" in
      -d|up-d|detach) up_dev 1 ;;
      up|"") up_dev 0 ;;
      *)
        echo "usage: ./start.sh dev [-d]" >&2
        exit 1
        ;;
    esac
    ;;
  sandbox)
    gateway_dir="${SANDBOX_GATEWAY_DIR:-./sandbox-gateway}"
    sandbox_image="${APPROVING_GATEWAY_SANDBOX_IMAGE:-universal-sandbox-cursor:local}"
    docker build --network=host \
      -t "$sandbox_image" \
      --build-arg AGENT_PROVIDER=cursor \
      -f "${gateway_dir}/sandbox/Dockerfile" \
      "${gateway_dir}/sandbox"
    echo "built ${sandbox_image}"
    ;;
  gateway)
    "${COMPOSE[@]}" -f "$DEV_COMPOSE_FILE" build gateway
    ;;
  build)
    echo "default start uses GHCR images (no local build)." >&2
    echo "for source builds: ./start.sh dev   or   ./start.sh sandbox" >&2
    exit 1
    ;;
  *)
    echo "unknown: $cmd" >&2
    echo "usage: ./start.sh [up|-d|pull|logs|gw-logs|down|restart|dev|sandbox|gateway]" >&2
    exit 1
    ;;
esac
