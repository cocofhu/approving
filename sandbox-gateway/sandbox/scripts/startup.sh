#!/bin/bash
set -e

# 统一通用沙箱启动脚本。整合两份来源：
#   - 运行环境（DinD + SSH + code-server）来自 ~/sandbox
#   - 多托管商 git 凭据路由 + 多仓库 PULL（GIT_REPOS）+ 多后端 backend（ACP 桥接）来自 code-flow
#
# Git 凭据路由（按每个仓库 URL 的 scheme 互斥选择，须在 clone 前完成）:
#   HTTPS — github.com→GITHUB_TOKEN; gitlab.com→GITLAB_TOKEN;
#           自建实例通过 GITHUB_URL / GITLAB_URL (scheme+host) 与 repo host 精确匹配。
#   SSH   — GIT_SSH_PRIVATE_KEY 写入 ~/.ssh/id_rsa (600); GIT_SSH_KNOWN_HOSTS 必填。
# 主要环境变量：WORKSPACE_DIR, ROOT_PASSWORD, CODE_SERVER_PORT, SSH_KEY, SKIP_INNER_DOCKER,
#   GIT_REPOS(多仓 name|url|branch) / GIT_CLONE_URL(单仓兼容),
#   GITHUB_TOKEN, GITHUB_URL, GITLAB_TOKEN, GITLAB_URL, GIT_SSH_PRIVATE_KEY, GIT_SSH_KNOWN_HOSTS,
#   ACP_BACKEND, ACP_BRIDGE_PORT, ACP_BRIDGE_PASSWORD, ACP_BRIDGE_MODEL

# PVC subPath 挂上来的 /tmp 目录默认不是 1777；补 sticky bit，避免 mktemp/多用户写入失败。
chmod 1777 /tmp 2>/dev/null || true

# DinD：在容器内启动 Docker 守护进程（宿主机需 --privileged；SKIP_INNER_DOCKER=1 可跳过）
if command -v dockerd >/dev/null 2>&1 && [ "${SKIP_INNER_DOCKER:-0}" != "1" ]; then
  if ! docker info >/dev/null 2>&1; then
    mkdir -p /var/run /var/lib/docker /etc/docker
    INNER_SD="${INNER_DOCKER_STORAGE_DRIVER:-vfs}"
    if [ -n "$DOCKER_INSECURE_REGISTRIES" ]; then
      regs_json=$(echo "$DOCKER_INSECURE_REGISTRIES" | tr ',' '\n' | sed 's/^[[:space:]]*//;s/[[:space:]]*$//' | grep -v '^$' | jq -R . | jq -s .)
    else
      regs_json='[]'
    fi
    jq -n \
      --arg sd "$INNER_SD" \
      --argjson regs "$regs_json" \
      '{ "storage-driver": $sd } + (if ($regs | length) > 0 then { "insecure-registries": $regs } else {} end)' \
      >/etc/docker/daemon.json
    dockerd --host=unix:///var/run/docker.sock >/var/log/dockerd.log 2>&1 &
    echo "等待 Docker 守护进程就绪..."
    for _ in $(seq 1 90); do
      docker info >/dev/null 2>&1 && break
      sleep 1
    done
    if ! docker info >/dev/null 2>&1; then
      echo "Docker 启动失败。请使用 --privileged 运行容器，或设置 SKIP_INNER_DOCKER=1 跳过内置 Docker。"
      tail -80 /var/log/dockerd.log 2>/dev/null || true
      exit 1
    fi
    echo "Docker 已就绪（docker / docker compose 可用）。"
  fi
fi

# 加载 Claude Code 环境
[ -f /etc/profile.d/claude.sh ] && . /etc/profile.d/claude.sh

# 默认密码是 toor，可以通过环境变量覆盖
PASSWORD=${ROOT_PASSWORD:-toor}
echo "root:${PASSWORD}" | chpasswd

# 默认工作目录 / code-server 端口
WORKSPACE_DIR=${WORKSPACE_DIR:-/root/workspace}
CODE_SERVER_PORT=${CODE_SERVER_PORT:-8744}

# 提前解析 agent provider 与配置根：契约注入（默认落到 CONFIG_ROOT）与 backend 都要用。
# 选型优先级：AGENT_PROVIDER > 旧变量 ACP_BACKEND > 构建期固定值（镜像已按 AGENT_PROVIDER 装 CLI）。
AGENT_PROVIDER=${AGENT_PROVIDER:-${ACP_BACKEND:-cursor}}
# 兼容旧变量：backend 的 FromEnv 会同时识别 AGENT_PROVIDER 与 ACP_BACKEND。
ACP_BACKEND=${ACP_BACKEND:-$AGENT_PROVIDER}
case "$AGENT_PROVIDER" in
  cursor|cursor_acp)                              CONFIG_ROOT=${CONFIG_ROOT:-/root/.cursor} ;;
  claude_code|claude_code_acp|claude_stream_json) CONFIG_ROOT=${CONFIG_ROOT:-/root/.claude} ;;
  codebuddy|codebuddy_acp)                        CONFIG_ROOT=${CONFIG_ROOT:-/root/.codebuddy} ;;
  opencode)                                       CONFIG_ROOT=${CONFIG_ROOT:-/root/.config/opencode} ;;
  *)                                              CONFIG_ROOT=${CONFIG_ROOT:-/root/.$AGENT_PROVIDER} ;;
esac
export AGENT_PROVIDER ACP_BACKEND CONFIG_ROOT

# --- Git credential helpers -------------------------------------------------

repo_scheme() {
    local url="$1"
    case "$url" in
        http://*|https://*) echo "https" ;;
        ssh://*)            echo "ssh" ;;
        *@*:*)              echo "ssh" ;;
        *)                  echo "unknown" ;;
    esac
}

repo_host() {
    local url="$1"
    local host=""
    case "$url" in
        http://*|https://*)
            host="${url#*://}"; host="${host%%/*}"; host="${host%%:*}" ;;
        ssh://*)
            host="${url#ssh://}"; host="${host#*@}"; host="${host%%/*}"; host="${host%%:*}" ;;
        *@*:*)
            host="${url#*@}"; host="${host%%:*}" ;;
    esac
    echo "$host"
}

url_host() {
    local u="$1"
    u="${u#http://}"; u="${u#https://}"; u="${u#ssh://}"; u="${u%%/*}"; u="${u%%:*}"
    echo "$u"
}

prepare_git_credentials_file() {
    mkdir -p /root
    if [ -d "/root/.git-credentials" ]; then
        rm -rf /root/.git-credentials
    fi
    git config --global credential.helper store
    if [ "${_GIT_CRED_RESET:-0}" != "1" ]; then
        rm -f /root/.git-credentials
        _GIT_CRED_RESET=1
    fi
    if ! { [ -f /root/.git-credentials ] && grep -qxF "$1" /root/.git-credentials; }; then
        echo "$1" >> /root/.git-credentials
    fi
    chmod 600 /root/.git-credentials
}

glab_auth_login() {
    local hostname="$1"
    local token="$2"
    command -v glab >/dev/null 2>&1 || return 0
    if glab auth login --hostname "${hostname}" --token "${token}" --api-protocol https --git-protocol https; then
        echo "glab: 已使用 GITLAB_TOKEN 登录 ${hostname}"
    else
        echo "glab: 自动登录失败，请检查 GITLAB_TOKEN 或稍后手动登录 ${hostname}" >&2
    fi
}

gh_auth_login() {
    local hostname="$1"
    local token="$2"
    command -v gh >/dev/null 2>&1 || return 0
    if printf '%s\n' "${token}" | gh auth login --hostname "${hostname}" --with-token; then
        echo "gh: 已使用 GITHUB_TOKEN 登录 ${hostname}"
    else
        echo "gh: 自动登录失败，请检查 GITHUB_TOKEN 或稍后手动登录 ${hostname}" >&2
    fi
}

setup_https_credentials() {
    local clone_url="$1"
    local host
    host=$(repo_host "$clone_url")
    if [ -z "$host" ]; then
        echo "startup.sh: 无法解析仓库主机: ${clone_url}" >&2
        return 1
    fi

    local cred_line="" provider="" glab_host="" gh_cli_host=""

    if [ "$host" = "github.com" ] && [ -n "$GITHUB_TOKEN" ]; then
        cred_line="https://x-access-token:${GITHUB_TOKEN}@github.com"; provider="GitHub"; gh_cli_host="github.com"
    elif [ "$host" = "gitlab.com" ] && [ -n "$GITLAB_TOKEN" ]; then
        cred_line="https://oauth2:${GITLAB_TOKEN}@gitlab.com"; provider="GitLab"; glab_host="gitlab.com"
    elif [ -n "$GITHUB_URL" ] && [ -n "$GITHUB_TOKEN" ]; then
        local ghe_host; ghe_host=$(url_host "$GITHUB_URL")
        if [ "$host" = "$ghe_host" ]; then
            cred_line="https://x-access-token:${GITHUB_TOKEN}@${host}"; provider="GitHub (self-hosted)"; gh_cli_host="$ghe_host"
        fi
    elif [ -n "$GITLAB_TOKEN" ]; then
        local gl_url="${GITLAB_URL:-https://${host}}"
        local gl_host; gl_host=$(url_host "$gl_url")
        if [ "$host" = "$gl_host" ]; then
            cred_line="https://oauth2:${GITLAB_TOKEN}@${host}"; provider="GitLab"; glab_host="$gl_host"
        fi
    fi

    if [ -n "$cred_line" ]; then
        prepare_git_credentials_file "$cred_line"
        echo "Git ${provider} HTTPS token configured for ${host}"
        [ -n "$gh_cli_host" ] && gh_auth_login "$gh_cli_host" "$GITHUB_TOKEN"
        [ -n "$glab_host" ] && glab_auth_login "$glab_host" "$GITLAB_TOKEN"
        return 0
    fi

    echo "startup.sh: HTTPS clone 未找到匹配凭据 (host=${host})" >&2
    echo "  支持: GitHub (GITHUB_TOKEN) / GitLab (GITLAB_TOKEN + GITLAB_URL); 自建实例配 GITHUB_URL / GITLAB_URL" >&2
    echo "  其它托管商请改用 SSH: GIT_SSH_PRIVATE_KEY + GIT_SSH_KNOWN_HOSTS" >&2
    return 1
}

setup_ssh_credentials() {
    local clone_url="$1"
    local host; host=$(repo_host "$clone_url")

    if [ -z "$GIT_SSH_PRIVATE_KEY" ]; then
        echo "startup.sh: SSH repo_url 需要 GIT_SSH_PRIVATE_KEY (host=${host:-unknown})" >&2
        return 1
    fi
    if [ -z "$GIT_SSH_KNOWN_HOSTS" ]; then
        echo "startup.sh: SSH repo_url 需要 GIT_SSH_KNOWN_HOSTS (host=${host:-unknown})" >&2
        echo "  获取指纹示例: ssh-keyscan -t ed25519,rsa ${host}" >&2
        return 1
    fi

    mkdir -p /root/.ssh
    chmod 700 /root/.ssh
    cat > /root/.ssh/id_rsa <<EOF
${GIT_SSH_PRIVATE_KEY}
EOF
    chmod 600 /root/.ssh/id_rsa
    cat > /root/.ssh/known_hosts <<EOF
${GIT_SSH_KNOWN_HOSTS}
EOF
    chmod 644 /root/.ssh/known_hosts
    echo "Git SSH credentials configured for ${host:-ssh remote}"
    return 0
}

# 仅设置了 GITLAB_TOKEN（无 GIT_REPOS / GIT_CLONE_URL）时，按 GITLAB_URL 注入凭据。
# 通用镜像不预设任何具体 GitLab 域名：未提供 GITLAB_URL 则跳过。
setup_bare_gitlab_credentials() {
    if [ -z "$GITLAB_URL" ]; then
        echo "startup.sh: 仅设置了 GITLAB_TOKEN 但未提供 GITLAB_URL，跳过凭据注入" >&2
        return 0
    fi
    local gitlab_host; gitlab_host=$(url_host "$GITLAB_URL")
    prepare_git_credentials_file "https://oauth2:${GITLAB_TOKEN}@${gitlab_host}"
    echo "GitLab token configured successfully for ${GITLAB_URL}"
    glab_auth_login "$gitlab_host" "$GITLAB_TOKEN"
}

# 仅设置了 GITHUB_TOKEN（无 GIT_REPOS / GIT_CLONE_URL）时注入凭据并 gh auth login。
# 未提供 GITHUB_URL 时默认 github.com（与公开 GitHub 场景对齐）。
setup_bare_github_credentials() {
    local github_host="github.com"
    if [ -n "$GITHUB_URL" ]; then
        github_host=$(url_host "$GITHUB_URL")
    fi
    prepare_git_credentials_file "https://x-access-token:${GITHUB_TOKEN}@${github_host}"
    echo "GitHub token configured successfully for ${github_host}"
    gh_auth_login "$github_host" "$GITHUB_TOKEN"
}

setup_repo_credentials() {
    local url="$1"
    case "$(repo_scheme "$url")" in
        https) setup_https_credentials "$url" || true ;;
        ssh)   setup_ssh_credentials "$url" || true ;;
        *)     echo "startup.sh: 无法识别 clone url scheme: ${url}" >&2 ;;
    esac
}

repo_name_from_url() {
    local u="${1%/}"
    u="${u##*[:/]}"
    printf '%s' "${u%.git}"
}

# --- 凭据配置（clone 前）----------------------------------------------------
if [ -n "$GIT_REPOS" ]; then
    IFS=',' read -ra _entries <<< "$GIT_REPOS"
    for _entry in "${_entries[@]}"; do
        [ -n "$_entry" ] || continue
        IFS='|' read -r _name _url _branch <<< "$_entry"
        [ -n "$_url" ] && setup_repo_credentials "$_url"
    done
    unset _entries _entry _name _url _branch
elif [ -n "$GIT_CLONE_URL" ]; then
    setup_repo_credentials "$GIT_CLONE_URL"
else
    [ -n "$GITLAB_TOKEN" ] && setup_bare_gitlab_credentials
    [ -n "$GITHUB_TOKEN" ] && setup_bare_github_credentials
fi

# 配置 Git 用户信息（通用中性默认，可由环境变量覆盖）
GIT_USER_EMAIL=${GIT_USER_EMAIL:-sandbox@localhost}
GIT_USER_NAME=${GIT_USER_NAME:-sandbox}
git config --global user.email "${GIT_USER_EMAIL}"
git config --global user.name "${GIT_USER_NAME}"
echo "Git user configured: ${GIT_USER_NAME} <${GIT_USER_EMAIL}>"

# --- 克隆代码仓库 -----------------------------------------------------------
if [ -n "$GIT_REPOS" ]; then
    # 多仓平级布局：每个仓库（即使只有一个）clone 到 $WORKSPACE_DIR/<name>/，
    # 并写 /root/.sandbox/repos.json 记录已克隆仓库清单（name+path，供业务方/工具发现）。
    mkdir -p "$WORKSPACE_DIR" /root/.sandbox
    _manifest=""
    IFS=',' read -ra _entries <<< "$GIT_REPOS"
    for _entry in "${_entries[@]}"; do
        [ -n "$_entry" ] || continue
        IFS='|' read -r _name _url _branch <<< "$_entry"
        [ -n "$_url" ] || continue
        [ -n "$_name" ] || _name=$(repo_name_from_url "$_url")
        [ -n "$_name" ] || continue
        # 防御：仓名用作平级子目录名,拒绝路径分隔符与 . / .. 以防逃逸 WORKSPACE_DIR
        case "$_name" in
            */*|*\\*|.|..) echo "startup.sh: 跳过不安全的仓名: ${_name}" >&2; continue ;;
        esac
        _dest="$WORKSPACE_DIR/$_name"
        if [ -d "$_dest/.git" ]; then
            echo "Repo ${_name} already exists in ${_dest}, skipping clone"
        else
            echo "Cloning ${_name} from ${_url}..."
            rm -rf "$_dest"; mkdir -p "$_dest"
            if ! git clone "$_url" "$_dest"; then
                echo "Failed to clone ${_name} from ${_url}" >&2
                rm -rf "$_dest"; continue
            fi
        fi
        if [ -n "$_branch" ] && git -C "$_dest" rev-parse --git-dir >/dev/null 2>&1; then
            git -C "$_dest" fetch origin "$_branch" 2>/dev/null || true
            if git -C "$_dest" checkout "$_branch" 2>/dev/null; then
                echo "  ${_name} on branch ${_branch}"
            else
                echo "  ${_name}: branch ${_branch} not found on remote; staying on default"
            fi
        fi
        if git -C "$_dest" rev-parse --git-dir >/dev/null 2>&1; then
            _entry_json="{\"name\":\"$_name\",\"path\":\"$_dest\"}"
            if [ -n "$_manifest" ]; then _manifest="$_manifest,$_entry_json"; else _manifest="$_entry_json"; fi
        fi
    done
    mkdir -p /root/.sandbox
    printf '[%s]' "$_manifest" > /root/.sandbox/repos.json
    echo "Recorded repo manifest: $(cat /root/.sandbox/repos.json)"
    unset _entries _entry _manifest _name _url _branch _dest _entry_json
    # Control plane may seed an outdated artifact-upload that still calls write_artifact.
    # After clone, prefer workspace server/scripts when the installed CLI is stale.
    if [ -f /usr/local/bin/artifact-upload ] && ! grep -q upload_image_artifact /usr/local/bin/artifact-upload 2>/dev/null; then
        _healed=0
        for _inst in "$WORKSPACE_DIR"/*/server/scripts/install-artifact-upload.sh; do
            if [ -x "$_inst" ]; then
                echo "startup.sh: refreshing artifact-upload via $_inst"
                "$_inst" && _healed=1 && break
            fi
        done
        if [ "$_healed" = 0 ]; then
            for _src in "$WORKSPACE_DIR"/*/server/scripts/artifact-upload; do
                if [ -f "$_src" ] && grep -q upload_image_artifact "$_src" 2>/dev/null; then
                    echo "startup.sh: installing artifact-upload from $_src"
                    install -m 755 "$_src" /usr/local/bin/artifact-upload && _healed=1 && break
                fi
            done
        fi
        unset _healed _inst _src
    fi
elif [ -n "$GIT_CLONE_URL" ]; then
    # 单仓兼容：clone 到 $WORKSPACE_DIR 根（兼容 K8S PVC 空挂载点）
    if [ -d "$WORKSPACE_DIR/.git" ]; then
        echo "Repository already exists in ${WORKSPACE_DIR}, skipping clone"
    elif [ -d "$WORKSPACE_DIR" ] && [ -n "$(ls -A "$WORKSPACE_DIR" 2>/dev/null)" ]; then
        echo "Workspace directory ${WORKSPACE_DIR} is not empty, skipping clone"
    else
        echo "Cloning repository from ${GIT_CLONE_URL}..."
        rm -rf "$WORKSPACE_DIR"/{..?*,.[!.]*,*} 2>/dev/null || true
        mkdir -p "$WORKSPACE_DIR"
        if git clone "$GIT_CLONE_URL" "$WORKSPACE_DIR"; then
            echo "Repository cloned successfully to ${WORKSPACE_DIR}"
        else
            echo "Failed to clone repository, using default workspace directory" >&2
        fi
    fi
else
    mkdir -p "$WORKSPACE_DIR"
    echo "Using workspace directory: ${WORKSPACE_DIR}"
fi

# --- 契约注入（务必早于 backend / code-server / SSH / noVNC 等服务启动）--------
# 业务方在服务启动前把配置/文件注入沙箱。两种方式：
#   1) 声明式 SANDBOX_INJECT="src[|dest],src2[|dest2],..."
#      - src：容器内已存在的文件/目录/归档，或 http(s):// URL；
#      - 归档（.tar/.tar.gz/.tgz/.tar.bz2/.tar.xz/.zip）解压到 dest，其余直接复制到 dest；
#      - dest 省略时默认 $CONFIG_ROOT（agent 配置根，含 mcp.json / rules/ / skills/）。
#   2) 钩子式 /root/.sandbox/init.d/*.sh：按文件名排序、在服务启动前依次 source 执行。
#
# 鉴权：本地文件/目录/归档不需要——由「谁能拉起沙箱/挂载文件」隐式授权。
# 仅 http(s):// 远端可能需要凭据：
#   - 首选预签名 URL（凭据在 query，天然带鉴权）；
#   - 或设 SANDBOX_INJECT_HEADERS（每行一个 HTTP 头，如 Authorization: Bearer xxx），
#     经 curl -K 配置文件下发（不出现在 ps 进程参数里）；日志隐去 URL 的 query。
inject_one() {
    local spec="$1" src dest tmp="" _curl_cfg="" _h
    IFS='|' read -r src dest <<< "$spec"
    [ -n "$src" ] || return 0
    dest="${dest:-$CONFIG_ROOT}"
    case "$src" in
        http://*|https://*)
            tmp="$(mktemp)"
            echo "inject: 下载 ${src%%\?*}"   # 隐去 query，避免泄露预签名 token
            if [ -n "$SANDBOX_INJECT_HEADERS" ]; then
                _curl_cfg="$(mktemp)"; chmod 600 "$_curl_cfg"
                while IFS= read -r _h; do
                    [ -n "$_h" ] || continue
                    printf 'header = "%s"\n' "$_h" >> "$_curl_cfg"
                done <<< "$SANDBOX_INJECT_HEADERS"
            fi
            if ! curl -fsSL ${_curl_cfg:+-K "$_curl_cfg"} "$src" -o "$tmp"; then
                echo "inject: 下载失败，跳过 ${src%%\?*}" >&2
                [ -n "$_curl_cfg" ] && rm -f "$_curl_cfg"
                rm -f "$tmp"; return 0
            fi
            [ -n "$_curl_cfg" ] && rm -f "$_curl_cfg"
            # 按 URL 扩展名补后缀，便于下面按归档类型判定
            case "${src%%\?*}" in
                *.tar.gz|*.tgz) mv "$tmp" "${tmp}.tgz";  src="${tmp}.tgz";  tmp="${tmp}.tgz" ;;
                *.tar.bz2)      mv "$tmp" "${tmp}.tbz2"; src="${tmp}.tbz2"; tmp="${tmp}.tbz2" ;;
                *.tar.xz)       mv "$tmp" "${tmp}.txz";  src="${tmp}.txz";  tmp="${tmp}.txz" ;;
                *.tar)          mv "$tmp" "${tmp}.tar";  src="${tmp}.tar";  tmp="${tmp}.tar" ;;
                *.zip)          mv "$tmp" "${tmp}.zip";  src="${tmp}.zip";  tmp="${tmp}.zip" ;;
                *)              src="$tmp" ;;
            esac
            ;;
    esac
    if [ ! -e "$src" ]; then
        echo "inject: 源不存在，跳过 ${src}" >&2
        [ -n "$tmp" ] && rm -f "$tmp"
        return 0
    fi
    mkdir -p "$dest"
    case "$src" in
        *.tar.gz|*.tgz)   echo "inject: 解压 ${src} → ${dest}"; tar -xzf "$src" -C "$dest" || echo "inject: 解压失败 ${src}" >&2 ;;
        *.tar.bz2|*.tbz2) echo "inject: 解压 ${src} → ${dest}"; tar -xjf "$src" -C "$dest" || echo "inject: 解压失败 ${src}" >&2 ;;
        *.tar.xz|*.txz)   echo "inject: 解压 ${src} → ${dest}"; tar -xJf "$src" -C "$dest" || echo "inject: 解压失败 ${src}" >&2 ;;
        *.tar)            echo "inject: 解压 ${src} → ${dest}"; tar -xf  "$src" -C "$dest" || echo "inject: 解压失败 ${src}" >&2 ;;
        *.zip)            echo "inject: 解压 ${src} → ${dest}"; unzip -oq "$src" -d "$dest" || echo "inject: 解压失败（需 unzip）${src}" >&2 ;;
        *)
            if [ -d "$src" ]; then
                echo "inject: 复制目录 ${src}/. → ${dest}/"; cp -a "$src/." "$dest/" || echo "inject: 复制失败 ${src}" >&2
            else
                echo "inject: 复制文件 ${src} → ${dest}/"; cp -a "$src" "$dest/" || echo "inject: 复制失败 ${src}" >&2
            fi
            ;;
    esac
    [ -n "$tmp" ] && rm -f "$tmp"
    return 0
}

if [ -n "$SANDBOX_INJECT" ]; then
    echo "契约注入：处理 SANDBOX_INJECT（默认目标 CONFIG_ROOT=${CONFIG_ROOT}）…"
    IFS=',' read -ra _inject_specs <<< "$SANDBOX_INJECT"
    for _spec in "${_inject_specs[@]}"; do
        [ -n "$_spec" ] || continue
        inject_one "$_spec"
    done
    unset _inject_specs _spec
fi

if [ -d /root/.sandbox/init.d ]; then
    for _hook in $(ls -1 /root/.sandbox/init.d/*.sh 2>/dev/null | sort); do
        [ -r "$_hook" ] || continue
        echo "契约注入：执行启动钩子 ${_hook}"
        # shellcheck disable=SC1090
        . "$_hook" || echo "inject: 钩子非零退出（忽略）${_hook}" >&2
    done
    unset _hook
fi

# --- 浏览器 MCP（可选，BROWSER_MCP=1）----------------------------------------
# 把沙箱内 headed Chromium 经 CDP 暴露成 MCP 工具给 agent：注册 chrome-devtools-mcp 到
# $CONFIG_ROOT/mcp.json（jq 非破坏合并，保留业务已注入的其它 MCP），并强制拉起预览栈
# （MCP 经 --browser-url attach 到该 Chromium；容器内自起 Chrome 有 sandbox 权限问题）。
# 在 backend 启动前完成，agent connect 时即可见该 MCP。
if [ "${BROWSER_MCP:-}" = "1" ] || [ "${BROWSER_MCP:-}" = "true" ]; then
    export VNC_PREVIEW=1   # MCP 需要沙箱内 Chromium/CDP，复用预览栈把它拉起
    _cdp_port="${CDP_PORT:-9222}"
    _mcp_file="$CONFIG_ROOT/mcp.json"
    mkdir -p "$CONFIG_ROOT"
    [ -s "$_mcp_file" ] || echo '{}' > "$_mcp_file"
    if command -v jq >/dev/null 2>&1; then
        _tmp_mcp="$(mktemp)"
        if jq --arg url "http://127.0.0.1:${_cdp_port}" \
              '.mcpServers //= {} | .mcpServers["chrome-devtools"] = {command:"chrome-devtools-mcp", args:["--browser-url=\($url)"]}' \
              "$_mcp_file" > "$_tmp_mcp" 2>/dev/null; then
            mv "$_tmp_mcp" "$_mcp_file"
            echo "浏览器 MCP：已注册 chrome-devtools → CDP http://127.0.0.1:${_cdp_port}（${_mcp_file}）"
        else
            rm -f "$_tmp_mcp"
            echo "浏览器 MCP：写入 ${_mcp_file} 失败，跳过" >&2
        fi
    else
        echo "浏览器 MCP：未找到 jq，无法安全合并 mcp.json，跳过" >&2
    fi
    unset _cdp_port _mcp_file _tmp_mcp
fi

# 配置 code-server：密码登录（默认 ROOT_PASSWORD）
mkdir -p /root/.config/code-server
cat > /root/.config/code-server/config.yaml <<EOF
bind-addr: 0.0.0.0:${CODE_SERVER_PORT}
auth: password
password: ${PASSWORD}
cert: false
EOF

# 降低 code-server 文件监视对 inotify 的消耗
mkdir -p /root/.local/share/code-server/User
cat > /root/.local/share/code-server/User/settings.json <<'EOF'
{
  "files.watcherExclude": {
    "**/.git/objects/**": true,
    "**/.git/subtree-cache/**": true,
    "**/node_modules/**": true,
    "**/dist/**": true,
    "**/build/**": true,
    "**/out/**": true,
    "**/target/**": true,
    "**/.gradle/**": true,
    "**/vendor/**": true,
    "**/.venv/**": true,
    "**/venv/**": true,
    "**/__pycache__/**": true,
    "**/.cache/**": true,
    "/ms-playwright/**": true,
    "/root/go/pkg/**": true
  }
}
EOF

# 加载开发工具 PATH（Go / Java / MongoDB 等），确保 code-server 及其子进程也可见
[ -f /etc/profile.d/devtools-path.sh ] && . /etc/profile.d/devtools-path.sh

# 配置 SSH 免密登录（如果提供了 SSH_KEY），并启动 SSH 服务
if [ -n "$SSH_KEY" ]; then
    echo "Configuring SSH key for passwordless login..."
    mkdir -p /root/.ssh
    chmod 700 /root/.ssh
    echo "$SSH_KEY" >> /root/.ssh/authorized_keys
    chmod 600 /root/.ssh/authorized_keys
    chown -R root:root /root/.ssh
    echo "SSH key configured successfully"
fi
service ssh start

# noVNC 预览栈（headed Chromium + CDP:9222 + websockify:6080）：仅当显式开启时启动，
# 避免普通场景吃 headed Chromium 资源。
_vnc_flag="${VNC_PREVIEW:-${ENABLE_VNC_PREVIEW:-}}"
if [ "$_vnc_flag" = "1" ] || [ "$_vnc_flag" = "true" ]; then
  if [ -x /usr/local/bin/vnc-preview.sh ]; then
    echo "启动沙箱内 noVNC 预览栈（VNC_PREVIEW=1）…"
    /usr/local/bin/vnc-preview.sh &
  else
    echo "未找到 vnc-preview.sh，跳过 noVNC 预览" >&2
  fi
fi

# 直连预览 HTML 注入：应用仍听 PREVIEW_PORT；入站经 iptables REDIRECT 到 17980。
# 与 noVNC 互斥由 Approving 保证（direct_preview 不设 VNC_PREVIEW）。
if [ "${PREVIEW_DIRECT:-}" = "1" ] || [ "${PREVIEW_DIRECT:-}" = "true" ]; then
  if [ -x /usr/local/bin/preview-inject.sh ]; then
    echo "启动直连预览 HTML 注入（PREVIEW_DIRECT=1）…"
    /usr/local/bin/preview-inject.sh &
  else
    echo "未找到 preview-inject.sh，跳过直连预览注入" >&2
  fi
fi

# backend（网关）：按 AGENT_PROVIDER 单活启动对应 provider，监听 8765（AGENT_PROVIDER / CONFIG_ROOT 已在前面解析）。
# ACP_BRIDGE_* 为主环境变量；CURSOR_ACP_* 为 deprecated 兼容别名
ACP_BRIDGE_PORT=${ACP_BRIDGE_PORT:-${CURSOR_ACP_PORT:-8765}}
if [ -x /usr/local/bin/backend ] && [ -d /usr/local/share/backend/web ]; then
  echo "启动 backend (AGENT_PROVIDER=${AGENT_PROVIDER})，监听 0.0.0.0:${ACP_BRIDGE_PORT}，configRoot ${CONFIG_ROOT}，工作目录 ${WORKSPACE_DIR}"
  (
    cd "$WORKSPACE_DIR"
    _acp_args=(
      -listen "0.0.0.0:${ACP_BRIDGE_PORT}"
      -web /usr/local/share/backend/web
      -gin-mode release
    )
    _acp_password="${ACP_BRIDGE_PASSWORD:-${CURSOR_ACP_PASSWORD:-}}"
    [ -n "$_acp_password" ] && _acp_args+=( -password "$_acp_password" )
    export ACP_BRIDGE_MODEL="${ACP_BRIDGE_MODEL:-${CURSOR_ACP_MODEL:-}}"
    exec /usr/local/bin/backend "${_acp_args[@]}"
  ) &
else
  echo "未找到 backend 可执行文件或 web 资源，跳过 ACP 服务"
fi

# 启动 code-server（root 用户运行，前台），指定工作目录
cd "$WORKSPACE_DIR"
code-server "$WORKSPACE_DIR"
