# Approving

[English](README.md) | 中文

**Dev Agent 编排平台** — 把多个 coding agent 组成可回滚、可人工门禁、可观测的工作流。
Agent 通过本仓库内嵌的 [sandbox-gateway](sandbox-gateway/) 在真实 Docker 沙箱中执行。

> 后端二进制 `approving-server`，环境变量前缀 `APPROVING_`。
>
> 许可证：[MIT](LICENSE)，Copyright (c) 2026 cocofhu。
> 仓库：[github.com/cocofhu/approving](https://github.com/cocofhu/approving)。
>
> 社区文档：[`CONTRIBUTING.md`](CONTRIBUTING.md)、[`SECURITY.md`](SECURITY.md)、
> [`SUPPORT.md`](SUPPORT.md)。

## 特性

- **FSM 编排**：节点即状态、边即转移；支持成功 / 失败 / 回滚路径、`when` 守卫、检查点与人工门禁。
- **真实执行**：agent / react 节点经 ACP 在沙箱容器中运行；Web UI 走 API。
- **产物契约 + 按 run 隔离的 MCP**：agent 调用 `write_artifact` / `set_*` / `node_complete`；按 token 隔离。
- **沙箱内 Git 凭据**：在 Agent 元信息 env 配置 `GITHUB_*`、`GITLAB_*` 或 SSH（值可引用 `${vars.<name>}`）。
  GitLab 仍可用平台 `glab` 自动开 MR；GitHub PR 由 Agent 侧 `gh` 处理。
- **单仓可跑**：`sandbox-gateway` 与沙箱镜像源码在本仓库内，无需再 clone 平级仓库。

## 目录结构

```
approving/
├── server/                 Go 后端（FSM + 沙箱客户端 + 产物 MCP）
├── web/                    Vue3 + Vue Flow UI
├── sandbox-gateway/        内嵌网关 + 通用沙箱镜像
├── docker-compose.yml      本地栈：gateway + server + web
├── start.sh                一键本地入口
├── compose.release.yaml    digest 钉死的公开发布栈
├── release-smoke.sh        干净 Linux 发布冒烟
├── GATEWAY.md              网关契约
└── .github/                Issues、PR、Actions
```

## 快速开始（Docker Compose）

需要 Linux 宿主与 Docker Compose：

```bash
./start.sh -d
# 等价：docker compose up --build -d
```

- Web UI：http://localhost:5173
- API 健康检查：http://localhost:8080/api/health
- 网关健康检查：http://localhost:8899/healthz
- 默认登录：`admin` / `demo1234`（见 `server/config.yaml`）

首次运行会从 `.env.example` 生成 `.env`，并可能从 `sandbox-gateway/sandbox`
构建 `universal-sandbox-cursor:local`（耗时较长）。

常用命令：

```bash
./start.sh logs
./start.sh down
./start.sh sandbox    # 仅重建沙箱镜像
./start.sh gateway    # 仅重建网关镜像
```

## 发布镜像（GHCR）

推送 `v*` 标签会触发：

- `publish-image` → `ghcr.io/cocofhu/approving`
- `publish-gateway` → `ghcr.io/cocofhu/sandbox-gateway`
- `publish-sandbox` → `ghcr.io/cocofhu/universal-sandbox-{cursor,claude_code,codebuddy,trae}`

沙箱镜像体积大（常需 30–90+ 分钟）。Package 默认可能是 Private；若需匿名拉取，
在 GitHub → Packages 设为 Public。

## 公开发布冒烟

在已有 digest 钉死镜像之后：

```bash
export APPROVING_IMAGE='ghcr.io/cocofhu/approving@sha256:...'
export SANDBOX_GATEWAY_IMAGE='ghcr.io/cocofhu/sandbox-gateway@sha256:...'
export SANDBOX_IMAGE='ghcr.io/cocofhu/universal-sandbox-cursor@sha256:...'
./release-smoke.sh
```

## 配置

见 [`server/CONFIGURATION.md`](server/CONFIGURATION.md) 与
`server/config.example.yaml`。优先级：显式环境变量 > 挂载配置文件 > 代码默认值。
沙箱 Git 凭据放在 Agent 元信息 env，不进平台配置。

## 许可证

[MIT](LICENSE) © 2026 cocofhu
