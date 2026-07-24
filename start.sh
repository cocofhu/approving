#!/usr/bin/env bash
# 一键启动 Approving 本地开发环境(docker compose):gateway + server + web。
#
# 用法:
#   ./start.sh            前台启动(构建镜像 + 启动,Ctrl+C 停止)
#   ./start.sh -d          后台启动
#   ./start.sh logs        跟踪日志(后台模式配合使用)
#   ./start.sh down        停止并清理容器
#   ./start.sh build       仅(重新)构建前后端 + 网关镜像,不启动
#   ./start.sh gateway     只(重新)构建 sandbox-gateway 网关镜像
#   ./start.sh sandbox     构建通用沙箱镜像 universal-sandbox-cursor:local
#   ./start.sh gw-logs     只跟踪网关日志
#   ./start.sh restart     重启(down + up -d)
#
# 前置要求:本机 Docker(+ compose 插件)。后端与网关都用 network_mode: host
# (沙箱端口 publish 在宿主 127.0.0.1,同机直连),目前只在 Linux 宿主上验证过。
#
# 沙箱网关与沙箱镜像源码在本仓库 sandbox-gateway/(无需再 clone 平级仓库)。
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")"
export HOST_REPO_DIR="$(pwd)"

if ! command -v docker >/dev/null 2>&1; then
  echo "错误:未检测到 docker,请先安装 Docker。" >&2
  exit 1
fi

if docker compose version >/dev/null 2>&1; then
  COMPOSE=(docker compose)
elif command -v docker-compose >/dev/null 2>&1; then
  COMPOSE=(docker-compose)
else
  echo "错误:未检测到 docker compose / docker-compose。" >&2
  exit 1
fi

if [[ ! -f .env ]]; then
  cp .env.example .env
  echo "已从 .env.example 生成 .env(可按需改端口 / 沙箱镜像 / 网关 / GOPROXY,再重新运行)。"
fi

if [[ ! -f server/config.yaml ]]; then
  cp server/config.example.yaml server/config.yaml
  echo "已从 server/config.example.yaml 生成 server/config.yaml(默认账号 admin / demo1234)。"
fi

mkdir -p .devdata/db .devdata/sandbox-home

set -a
# shellcheck disable=SC1091
source .env
set +a

port="${APPROVING_PORT:-8080}"
gateway_port="${APPROVING_GATEWAY_PORT:-8899}"
gateway_dir="${SANDBOX_GATEWAY_DIR:-./sandbox-gateway}"
gateway_url="${APPROVING_SANDBOX_GATEWAY_URL:-http://127.0.0.1:${gateway_port}}"
gateway_image="sandbox-gateway:local"
sandbox_image="${APPROVING_GATEWAY_SANDBOX_IMAGE:-universal-sandbox-cursor:local}"
export APPROVING_SANDBOX_GATEWAY_URL="$gateway_url"

if [[ ! -f "${gateway_dir}/Dockerfile" || ! -f "${gateway_dir}/deploy/config/config.local.yaml" ]]; then
  echo "错误:未找到 sandbox-gateway 源码目录:${gateway_dir}" >&2
  echo "      单仓布局下应为 ./sandbox-gateway。" >&2
  exit 1
fi

ensure_sandbox_image() {
  if docker image inspect "$sandbox_image" >/dev/null 2>&1; then
    return 0
  fi
  echo "本地尚无沙箱镜像 ${sandbox_image},开始构建(首次较久)..."
  docker build \
    -t "$sandbox_image" \
    --build-arg AGENT_PROVIDER=cursor \
    -f "${gateway_dir}/sandbox/Dockerfile" \
    "${gateway_dir}/sandbox"
}

build_gateway_image() {
  echo "构建沙箱网关镜像 ${gateway_image}(${gateway_dir}/Dockerfile)..."
  "${COMPOSE[@]}" build gateway
}

wait_for_gateway() {
  local url="http://127.0.0.1:${gateway_port}/healthz"
  local -a probe=()
  if command -v curl >/dev/null 2>&1; then
    probe=(curl -fsS -o /dev/null "$url")
  elif command -v wget >/dev/null 2>&1; then
    probe=(wget -q -O /dev/null "$url")
  else
    echo "网关地址 ${gateway_url}(未装 curl/wget,跳过健康探测)。"
    return 0
  fi
  echo -n "等待网关就绪 ${url} "
  for _ in $(seq 1 30); do
    if "${probe[@]}" >/dev/null 2>&1; then
      echo "✓"
      return 0
    fi
    echo -n "."
    sleep 1
  done
  echo ""
  echo "警告:网关 ${url} 30s 内未就绪,请查看日志:./start.sh gw-logs" >&2
  return 0
}

print_endpoints() {
  echo "—— 后端 http://localhost:${port}/api/health  前端 http://localhost:5173"
  echo "—— 默认登录 admin / demo1234(可改 server/config.yaml 的 auth.users)"
  echo "—— 沙箱网关 ${gateway_url}/healthz"
  echo "—— 沙箱镜像 ${sandbox_image}"
}

cmd="${1:-up}"
case "$cmd" in
  up)
    ensure_sandbox_image
    echo "启动中(前台)—— 后端 http://localhost:${port}  前端 http://localhost:5173  网关 ${gateway_url}"
    "${COMPOSE[@]}" up --build
    ;;
  -d|up-d|detach)
    ensure_sandbox_image
    "${COMPOSE[@]}" up --build -d
    wait_for_gateway
    echo "已后台启动"
    print_endpoints
    echo "查看日志: ./start.sh logs   停止: ./start.sh down"
    ;;
  logs)
    "${COMPOSE[@]}" logs -f
    ;;
  gw-logs)
    "${COMPOSE[@]}" logs -f gateway
    ;;
  down)
    "${COMPOSE[@]}" down
    ;;
  restart)
    "${COMPOSE[@]}" down
    ensure_sandbox_image
    "${COMPOSE[@]}" up --build -d
    wait_for_gateway
    echo "已重启"
    print_endpoints
    ;;
  build)
    ensure_sandbox_image
    build_gateway_image
    "${COMPOSE[@]}" build
    ;;
  gateway)
    build_gateway_image
    ;;
  sandbox)
    docker build \
      -t "$sandbox_image" \
      --build-arg AGENT_PROVIDER=cursor \
      -f "${gateway_dir}/sandbox/Dockerfile" \
      "${gateway_dir}/sandbox"
    echo "已构建 ${sandbox_image}"
    ;;
  *)
    echo "未知参数: $cmd" >&2
    echo "用法: ./start.sh [up|-d|logs|gw-logs|down|restart|build|gateway|sandbox]" >&2
    exit 1
    ;;
esac
