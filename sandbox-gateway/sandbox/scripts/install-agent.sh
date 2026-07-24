#!/usr/bin/env bash
# install-agent.sh — 按 AGENT_PROVIDER 只安装选中那一个 Agent CLI。
#
# 由 Dockerfile 在构建期调用（`--build-arg AGENT_PROVIDER=xxx`），配合"共享 base +
# 每 agent 薄镜像"策略：base 层跨 tag 复用缓存，本脚本产出的差异层只含所需 CLI。
#
# 约定：
#   $1 / $AGENT_PROVIDER  选择要安装的 provider（默认 cursor）。
#   $AGENT_INSTALL_CMD    可选：任意 provider 的安装命令覆盖；给出时完全接管安装
#                         （用于未内置安装方式的 provider，或临时改用私有源/镜像）。
#
# 退出码非 0 会让该 tag 的构建失败——这是刻意的：固定 agent 的镜像若缺了它自己的 CLI
# 属于致命错误，应在构建期暴露，而不是运行时才 LookPath 失败。
set -euo pipefail

provider="${1:-${AGENT_PROVIDER:-cursor}}"
custom_cmd="${AGENT_INSTALL_CMD:-}"

log() { echo "[install-agent] $*"; }

# retry <n> <cmd...> —— 带指数退避的重试，缓解构建期网络抖动。
retry() {
  local max="$1"; shift
  local i=1
  until "$@"; do
    if [ "$i" -ge "$max" ]; then return 1; fi
    log "第 $i 次失败，重试：$*"; sleep $((i * 5)); i=$((i + 1))
  done
}

npm_global() { retry 3 npm install -g "$@"; }

install_cursor() {
  # 官方 install.sh 无重试/无超时且会吞掉失败；改为解析版本→带重试下载解压→建软链→校验。
  local ok=0 script ver url dest
  for i in 1 2 3 4 5; do
    script="$(curl -fsSL --connect-timeout 20 --max-time 60 https://cursor.com/install || true)"
    ver="$(printf '%s' "$script" | grep -oE 'lab/[0-9]{4}\.[0-9]{2}\.[0-9]{2}-[a-f0-9]+/' | head -1 | sed -E 's#lab/(.+)/#\1#')"
    if [ -n "$ver" ]; then
      url="https://downloads.cursor.com/lab/${ver}/linux/x64/agent-cli-package.tar.gz"
      dest="/root/.local/share/cursor-agent/versions/${ver}"
      mkdir -p "$dest" /root/.local/bin
      if curl -fSL --retry 8 --retry-delay 5 --retry-all-errors --connect-timeout 30 --max-time 900 "$url" -o /tmp/agent.tgz \
         && tar --strip-components=1 -xzf /tmp/agent.tgz -C "$dest"; then
        rm -f /tmp/agent.tgz
        ln -sf "$dest/cursor-agent" /root/.local/bin/agent
        ln -sf "$dest/cursor-agent" /root/.local/bin/cursor-agent
        ok=1; break
      fi
      rm -f /tmp/agent.tgz
    fi
    log "cursor-agent 安装第 $i 次失败，重试..."; sleep 10
  done
  [ "$ok" = 1 ]
  cursor-agent --version
}

install_claude_native() {
  retry 3 bash -c 'curl -fsSL --connect-timeout 20 --max-time 120 https://claude.ai/install.sh | bash'
  claude --version
}

install_trae() {
  retry 3 bash -c 'curl -fsSL --connect-timeout 20 --max-time 120 "https://docs.trae.cn/cli/install.sh" | bash'
  command -v traecli >/dev/null 2>&1 && traecli --version || log "traecli 已安装（版本探测跳过）"
}

# 完全覆盖：给了 AGENT_INSTALL_CMD 就以它为准（适用于未内置安装方式的 provider）。
if [ -n "$custom_cmd" ]; then
  log "provider=$provider 使用 AGENT_INSTALL_CMD 覆盖安装"
  eval "$custom_cmd"
  exit 0
fi

log "安装 provider=$provider"
case "$provider" in
  cursor|cursor_acp)
    install_cursor ;;                                      # 同一 cursor-agent 二进制（stream-json / ACP 两用）
  claude_code|claude_stream_json)
    install_claude_native ;;                               # 原生 claude CLI（stream-json，默认）
  claude_code_acp)
    install_claude_native && npm_global @zed-industries/claude-code-acp ;;  # 原生 claude + ACP 适配器
  codebuddy|codebuddy_acp)
    npm_global @tencent-ai/codebuddy-code ;;               # 同一 codebuddy 二进制（stream-json / ACP 两用）
  trae)
    install_trae ;;
  codex)
    npm_global @openai/codex && codex --version ;;
  gemini)
    npm_global @google/gemini-cli && gemini --version ;;
  copilot)
    npm_global @github/copilot && copilot --version ;;
  opencode)
    npm_global opencode-ai && opencode --version ;;
  kiro|qoder|grok|kimi|hermes|deveco|openclaw|antigravity|pi)
    log "错误：provider=$provider 暂无内置安装方式。"
    log "请通过 --build-arg AGENT_INSTALL_CMD='<安装命令>' 提供其官方安装步骤后再构建该 tag。"
    exit 1 ;;
  *)
    log "错误：未知 provider=$provider（且未提供 AGENT_INSTALL_CMD）。"
    exit 1 ;;
esac

log "provider=$provider 安装完成"
