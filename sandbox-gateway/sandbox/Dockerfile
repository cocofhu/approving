# 统一通用沙箱镜像：多语言 + 容器内 DinD（docker / docker compose）+ SSH + code-server + backend 网关
#
# 镜像策略：共享 base + 每 agent 一层薄镜像。除 backend 网关与全套工具链外，构建期仅安装
#   `--build-arg AGENT_PROVIDER=<provider>` 选中的那一个 Agent CLI（默认 cursor）。base 层跨
#   不同 agent tag 复用同一缓存，差异层只是"装哪个 CLI"。
# 构建示例：
#   docker build --build-arg AGENT_PROVIDER=gemini -t sandbox:gemini .
#   docker build --build-arg AGENT_PROVIDER=codex  -t sandbox:codex  .
#   # 未内置安装方式的 provider：自带安装命令
#   docker build --build-arg AGENT_PROVIDER=grok --build-arg AGENT_INSTALL_CMD='npm i -g <pkg>' -t sandbox:grok .
# 运行示例：docker run --privileged -e CODE_SERVER_PORT=8744 -p 8744:8744 -p 22:22 -p 8765:8765 <image>
# backend 默认监听 0.0.0.0:8765（容器内需 -p 映射方可从宿主机访问），可用 -e ACP_BRIDGE_PORT=<端口> 覆盖；
#   可选 -e ACP_BRIDGE_PASSWORD=<口令> 启用登录页；cwd 为 WORKSPACE_DIR。镜像已按 AGENT_PROVIDER 固定 agent，
#   运行时该值与构建期一致（如需换 agent 请换对应 tag 的镜像）。
# 不需要内置 Docker：docker run -e SKIP_INNER_DOCKER=1 ...

# 22.04：glibc ≥ 2.34，满足新版 code-server 内置 argon2 原生模块要求（20.04 会报 GLIBC_2.34 not found）
# 默认走国内 Docker Hub 镜像；海外可 --build-arg BASE_IMAGE=ubuntu:22.04
#
# 多阶段：cli-tools = 系统工具 + glab/gh（CI 可用 --target cli-tools 冒烟，避免拉全量
# Playwright / Go / agent CLI）；最终阶段从此继续装 Node/后端/agent。
ARG BASE_IMAGE=docker.m.daocloud.io/library/ubuntu:22.04
FROM ${BASE_IMAGE} AS cli-tools

ENV DEBIAN_FRONTEND=noninteractive
# code-server 默认端口刻意避开常用的 8080（留给用户自己的应用）
ENV CODE_SERVER_PORT=8744

# 使用清华源 (HTTP)
RUN sed -i 's@http://.*archive.ubuntu.com@http://mirrors.tuna.tsinghua.edu.cn@g' /etc/apt/sources.list && \
    sed -i 's@http://.*security.ubuntu.com@http://mirrors.tuna.tsinghua.edu.cn@g' /etc/apt/sources.list

# 最小依赖（Docker 安装前置条件），尽早安装以便快速发现源问题
RUN set -eu; \
    ok=0; \
    for i in 1 2 3 4 5; do \
      if apt-get update && apt-get install -y \
        ca-certificates \
        curl \
        gnupg \
        lsb-release \
        iptables \
        pigz; then ok=1; break; fi; \
      echo "apt install attempt $i failed; waiting for mirror re-sync..."; sleep 20; \
    done; \
    [ "$ok" = 1 ]; \
    rm -rf /var/lib/apt/lists/*

# Docker Engine + CLI + Buildx + Compose 插件（清华 apt 源，提前安装以便尽早发现源问题）
RUN set -eu; \
    curl -fsSL https://mirrors.tuna.tsinghua.edu.cn/docker-ce/linux/ubuntu/gpg | gpg --dearmor -o /usr/share/keyrings/docker-archive-keyring.gpg; \
    echo "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/docker-archive-keyring.gpg] https://mirrors.tuna.tsinghua.edu.cn/docker-ce/linux/ubuntu $(lsb_release -cs) stable" > /etc/apt/sources.list.d/docker.list; \
    ok=0; \
    for i in 1 2 3 4 5; do \
      if apt-get update && apt-get install -y \
        docker-ce \
        docker-ce-cli \
        containerd.io \
        docker-buildx-plugin \
        docker-compose-plugin; then ok=1; break; fi; \
      echo "apt install attempt $i failed; waiting for mirror re-sync..."; sleep 20; \
    done; \
    [ "$ok" = 1 ]; \
    rm -rf /var/lib/apt/lists/*

# 通用工具 + C++/Python/Java 等其余组件
# 清华镜像偶发"半同步"（如 openssh-server 依赖旧版 openssh-client 而索引已更新），
# 导致 apt 解析失败；重试 + 重新 apt-get update 可自愈，构建必须真正装上才通过。
RUN set -eu; \
    ok=0; \
    for i in 1 2 3 4 5; do \
      if apt-get update && apt-get install -y \
        git \
        openssh-server \
        sudo \
        vim \
        jq \
        unzip \
        mysql-client \
        redis-tools \
        postgresql-client \
        build-essential \
        gcc \
        g++ \
        cmake \
        make \
        gdb \
        valgrind \
        python3 \
        python3-pip \
        python3-dev \
        python3-venv \
        openjdk-11-jdk \
        maven \
        gradle \
        xz-utils \
        iproute2 \
        net-tools \
        iputils-ping \
        dnsutils; then ok=1; break; fi; \
      echo "apt install attempt $i failed; waiting for mirror re-sync..."; sleep 20; \
    done; \
    [ "$ok" = 1 ]; \
    rm -rf /var/lib/apt/lists/*

# MongoDB 客户端（必须装上，不允许 best-effort 跳过）
#   - mongosh / mongodb-database-tools：清华 MongoDB apt 镜像（稳定可达，规避官方 CDN 的 SSL 中断）
#       [trusted=yes] 免去从 pgp.mongodb.com 拉 GPG key（该域名在国内同样易 SSL 失败）。
#   - mongocli：apt 源里没有，走 fastdl 强重试下载（github 在国内会挂，fastdl 可达）。
ENV MONGOCLI_VERSION=1.30.0
RUN set -eu; \
    echo "deb [trusted=yes] https://mirrors.tuna.tsinghua.edu.cn/mongodb/apt/ubuntu jammy/mongodb-org/8.0 multiverse" \
      > /etc/apt/sources.list.d/mongodb-org.list; \
    ok=0; \
    for i in 1 2 3 4 5; do \
      if apt-get update && apt-get install -y mongodb-mongosh mongodb-database-tools; then ok=1; break; fi; \
      echo "mongodb apt attempt $i failed; waiting for mirror re-sync..."; sleep 20; \
    done; \
    [ "$ok" = 1 ]; \
    rm -rf /var/lib/apt/lists/*; \
    command -v mongosh; command -v mongodump; \
    curl -fsSL --retry 8 --retry-delay 5 --retry-all-errors --connect-timeout 30 --max-time 300 \
      "https://fastdl.mongodb.org/mongocli/mongocli_${MONGOCLI_VERSION}_linux_x86_64.tar.gz" -o /tmp/mongocli.tgz; \
    tar -C /usr/local -xzf /tmp/mongocli.tgz; \
    mv "/usr/local/mongocli_${MONGOCLI_VERSION}_linux_x86_64" /usr/local/mongocli; \
    rm -f /tmp/mongocli.tgz; \
    /usr/local/mongocli/bin/mongocli --version
ENV PATH=$PATH:/usr/local/mongocli/bin

# Agent CLI 运行路径（cursor-agent 等下载型 CLI 装到 /root/.local/bin）。PATH 提前声明，
# 便于 Cursor Server / SSH 等非 entrypoint 进程也能找到命令。具体 CLI 由末尾的 agent 安装层
# 按 AGENT_PROVIDER 装入（见 scripts/install-agent.sh）。
RUN mkdir -p /root/.local/bin && \
    echo 'export PATH="/root/.local/bin:$PATH"' > /etc/profile.d/agent-path.sh
ENV PATH=/root/.local/bin:$PATH

# GitLab CLI（与 startup.sh 中 GITLAB_TOKEN 自动执行 glab auth login 配合）
ENV GLAB_VERSION=1.91.0
RUN curl -fsSL --http1.1 --retry 8 --retry-delay 5 --retry-all-errors --connect-timeout 30 --max-time 300 \
      "https://gitlab.com/gitlab-org/cli/-/releases/v${GLAB_VERSION}/downloads/glab_${GLAB_VERSION}_linux_amd64.tar.gz" -o /tmp/glab.tgz && \
    tar -xzf /tmp/glab.tgz -C /tmp && \
    install -m 0755 /tmp/bin/glab /usr/local/bin/glab && \
    rm -rf /tmp/glab.tgz /tmp/bin /tmp/CHANGELOG.md /tmp/LICENSE /tmp/README.md && \
    glab --version

# GitHub CLI（与 startup.sh 中 GITHUB_TOKEN 自动执行 gh auth login 配合；Agent 侧 gh pr 建单依赖）
# github.com 在国内构建易 SSL/超时，官方 release + 镜像回退。
ENV GH_VERSION=2.96.0
RUN set -eu; \
    file="gh_${GH_VERSION}_linux_amd64.tar.gz"; \
    ok=0; \
    for url in \
      "https://github.com/cli/cli/releases/download/v${GH_VERSION}/${file}" \
      "https://ghfast.top/https://github.com/cli/cli/releases/download/v${GH_VERSION}/${file}"; do \
      if curl -fsSL --http1.1 --retry 5 --retry-delay 5 --retry-all-errors \
          --connect-timeout 30 --max-time 300 "$url" -o /tmp/gh.tgz; then \
        ok=1; \
        break; \
      fi; \
      rm -f /tmp/gh.tgz; \
    done; \
    test "$ok" = "1"; \
    tar -xzf /tmp/gh.tgz -C /tmp; \
    install -m 0755 "/tmp/gh_${GH_VERSION}_linux_amd64/bin/gh" /usr/local/bin/gh; \
    rm -rf /tmp/gh.tgz "/tmp/gh_${GH_VERSION}_linux_amd64"; \
    gh --version

# 最终镜像：在 cli-tools 之上继续装语言运行时 / IDE / backend / agent
FROM cli-tools

# Node.js LTS（nodejs.org 官方 CDN 国内常 SSL 中断；改用 npmmirror 镜像 + 强重试）
ENV NODE_VERSION=24.13.0
ENV NODE_ARCH=x64
RUN curl -fsSL --retry 8 --retry-delay 5 --retry-all-errors --connect-timeout 30 --max-time 300 \
      "https://cdn.npmmirror.com/binaries/node/v${NODE_VERSION}/node-v${NODE_VERSION}-linux-${NODE_ARCH}.tar.xz" -o /tmp/node.tar.xz && \
    tar -C /usr/local --strip-components=1 -xf /tmp/node.tar.xz && \
    rm /tmp/node.tar.xz && \
    node --version && npm --version

RUN npm config set registry https://registry.npmmirror.com
RUN npm install -g yarn pnpm typescript ts-node

# Playwright 浏览器 E2E + noVNC 预览栈：预装 Chromium + Playwright 系统依赖，落到共享目录
# /ms-playwright；另装 CJK 字体（无头渲染/截图不出方块）与 xvfb/x11vnc/websockify/socat
# （vnc-preview.sh 起 headed Chromium + CDP + websockify）。
ENV PLAYWRIGHT_BROWSERS_PATH=/ms-playwright
ENV PLAYWRIGHT_VERSION=1.61.1
RUN apt-get update && apt-get install -y --no-install-recommends \
      fonts-noto-cjk \
      fonts-liberation \
      fonts-noto-color-emoji \
      fontconfig \
      xvfb \
      x11vnc \
      websockify \
      socat \
    && npx --yes playwright@${PLAYWRIGHT_VERSION} install --with-deps chromium \
    && fc-cache -f \
    && rm -rf /var/lib/apt/lists/* /root/.npm

# Go (阿里云优先；golang.google.cn 会回源 dl.google.com，国内 CI 易超时)
ENV GO_VERSION=1.25.0
ENV GO_ARCH=amd64
RUN set -eux; \
    file="go${GO_VERSION}.linux-${GO_ARCH}.tar.gz"; \
    ok=0; \
    for base in \
      "https://mirrors.aliyun.com/golang" \
      "https://golang.google.cn/dl"; do \
      if curl -fsSL --retry 5 --retry-delay 5 --retry-all-errors \
          --connect-timeout 30 --max-time 600 \
          "${base}/${file}" -o /tmp/go.tar.gz; then \
        ok=1; \
        break; \
      fi; \
      rm -f /tmp/go.tar.gz; \
    done; \
    test "$ok" = "1"; \
    tar -C /usr/local -xzf /tmp/go.tar.gz; \
    rm /tmp/go.tar.gz; \
    /usr/local/go/bin/go version

ENV PATH=$PATH:/usr/local/go/bin
ENV GOPATH=/root/go
ENV GOPROXY=https://goproxy.cn,direct
# 预装 gopls：钉 v0.21.1（go 1.25）对齐镜像 GO_VERSION；GOTOOLCHAIN=local 禁止 @latest 自动拉新 toolchain
ENV GOTOOLCHAIN=local
RUN mkdir -p /root/go/bin && go install golang.org/x/tools/gopls@v0.21.1
ENV PATH=$PATH:/root/go/bin

# Java
ENV JAVA_HOME=/usr/lib/jvm/java-11-openjdk-amd64
ENV PATH=$PATH:$JAVA_HOME/bin

# 将所有自定义工具路径写入 shell 初始化文件，确保 Cursor Server / SSH 等非 entrypoint 进程也能找到命令
RUN { \
      echo 'export JAVA_HOME=/usr/lib/jvm/java-11-openjdk-amd64'; \
      echo 'export GOPATH=/root/go'; \
      echo 'export GOPROXY=https://goproxy.cn,direct'; \
      echo 'for p in /usr/local/go/bin /root/go/bin /usr/local/mongocli/bin $JAVA_HOME/bin; do'; \
      echo '  case ":$PATH:" in *:"$p":*) ;; *) export PATH="$PATH:$p" ;; esac'; \
      echo 'done'; \
    } > /etc/profile.d/devtools-path.sh && \
    echo '[ -f /etc/profile.d/devtools-path.sh ] && . /etc/profile.d/devtools-path.sh' >> /root/.bashrc

# Python pip 清华源
RUN pip3 config set global.index-url https://pypi.tuna.tsinghua.edu.cn/simple
RUN pip3 install --upgrade pip setuptools wheel

# 安装 code-server（带重试，防止网络抖动）
RUN for i in 1 2 3; do curl -fsSL --connect-timeout 20 --max-time 120 https://code-server.dev/install.sh | sh && break || echo "code-server 安装失败，第 $i 次重试..." && sleep $((i * 5)); done

# 刻意不预装任何 code-server 扩展：保持镜像轻量，扩展由用户按需在 IDE 内自行安装。

# Claude CLI 的环境初始化（仅设置环境变量；claude 本体由 agent 安装层按需装入）。
COPY scripts/claude-env.sh /etc/profile.d/claude.sh
RUN echo '[ -f /etc/profile.d/claude.sh ] && . /etc/profile.d/claude.sh' >> /root/.bashrc

# SSH
RUN mkdir /var/run/sshd && \
    sed -i 's/#PermitRootLogin prohibit-password/PermitRootLogin yes/' /etc/ssh/sshd_config && \
    sed -i 's@session\s*required\s*pam_loginuid.so@session optional pam_loginuid.so@g' /etc/pam.d/sshd

# 8744 code-server / 22 SSH / 8765 backend(ACP 桥接) / 9222 Chromium CDP / 6080 noVNC(websockify)
EXPOSE 8744 22 8765 9222 6080

# 浏览器 MCP（可选，BROWSER_MCP=1 启用）：chrome-devtools-mcp 经 CDP attach 到沙箱内
# headed Chromium（vnc-preview 起的那个），把导航/点击/截图/读 DOM 暴露为 MCP 工具。
# 预装以便离线用全局二进制（mcp.json 里直接 command: chrome-devtools-mcp，不走 npx 拉取）。
RUN npm install -g chrome-devtools-mcp

# backend（ACP 桥接服务）：编译多后端二进制并安装前端静态资源；运行时监听 0.0.0.0:8765
COPY go.mod go.sum /tmp/backend-src/
COPY cmd /tmp/backend-src/cmd
COPY internal /tmp/backend-src/internal
COPY web /tmp/backend-src/web
RUN cd /tmp/backend-src && \
    go build -ldflags="-s -w" -o /usr/local/bin/backend ./cmd/backend && \
    ln -sf /usr/local/bin/backend /usr/local/bin/acp-bridge && \
    ln -sf /usr/local/bin/backend /usr/local/bin/acp-gateway && \
    ln -sf /usr/local/bin/backend /usr/local/bin/cursor-acp && \
    mkdir -p /usr/local/share/backend && \
    cp -a web /usr/local/share/backend/web && \
    ln -sf /usr/local/share/backend /usr/local/share/acp-bridge && \
    ln -sf /usr/local/share/backend /usr/local/share/acp-gateway && \
    ln -sf /usr/local/share/backend /usr/local/share/cursor-acp && \
    rm -rf /tmp/backend-src

ENV ACP_BRIDGE_PORT=8765

# noVNC 预览栈脚本（headed Chromium on Xvfb + x11vnc + websockify）；VNC_PREVIEW=1 时启动
COPY scripts/vnc-preview.sh /usr/local/bin/vnc-preview.sh
RUN chmod +x /usr/local/bin/vnc-preview.sh

COPY scripts/startup.sh /startup.sh
RUN chmod +x /startup.sh

# ── Agent 安装层（每 agent 薄镜像的唯一差异层）───────────────────────────────
# ARG 刻意放在最后：其上所有层（工具链 / code-server / backend 网关 / 脚本）在不同
# AGENT_PROVIDER 之间完全一致，可跨 tag 复用同一缓存；仅此层随所选 agent 变化。
# ENV AGENT_PROVIDER 使运行时默认选中构建期安装的那个 agent（与镜像内 CLI 一致）。
ARG AGENT_PROVIDER=cursor
ARG AGENT_INSTALL_CMD=
ENV AGENT_PROVIDER=${AGENT_PROVIDER}
COPY scripts/install-agent.sh /usr/local/bin/install-agent.sh
RUN chmod +x /usr/local/bin/install-agent.sh && \
    AGENT_INSTALL_CMD="${AGENT_INSTALL_CMD}" /usr/local/bin/install-agent.sh "${AGENT_PROVIDER}"

ENTRYPOINT ["/startup.sh"]
