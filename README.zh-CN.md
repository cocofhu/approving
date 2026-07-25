# Approving

**把 coding agent 编成可信任的工作流。**

开源 Dev Agent 编排平台。
构建可回滚、可人工门禁、可观测的工作流 — Agent 在真实 Docker 沙箱中执行。

[![CI Server](https://github.com/cocofhu/approving/actions/workflows/ci-server.yml/badge.svg)](https://github.com/cocofhu/approving/actions/workflows/ci-server.yml)
[![CI Web](https://github.com/cocofhu/approving/actions/workflows/ci-web.yml/badge.svg)](https://github.com/cocofhu/approving/actions/workflows/ci-web.yml)
[![CI Sandbox](https://github.com/cocofhu/approving/actions/workflows/ci-sandbox.yml/badge.svg)](https://github.com/cocofhu/approving/actions/workflows/ci-sandbox.yml)
[![CI Gateway](https://github.com/cocofhu/approving/actions/workflows/ci-gateway.yml/badge.svg)](https://github.com/cocofhu/approving/actions/workflows/ci-gateway.yml)

[![coverage-web](https://img.shields.io/endpoint?url=https%3A%2F%2Fraw.githubusercontent.com%2Fcocofhu%2Fapproving%2Fcoverage-badges%2Fcoverage-web.json)](https://github.com/cocofhu/approving/actions/workflows/ci-web.yml)
[![coverage-sandbox](https://img.shields.io/endpoint?url=https%3A%2F%2Fraw.githubusercontent.com%2Fcocofhu%2Fapproving%2Fcoverage-badges%2Fcoverage-sandbox.json)](https://github.com/cocofhu/approving/actions/workflows/ci-sandbox.yml)

[项目站](https://cocofhu.github.io/approving-pages/) · [贡献指南](CONTRIBUTING.md) · [安全](SECURITY.md) · [支持](SUPPORT.md) · [配置](server/CONFIGURATION.md) · [网关](GATEWAY.md)

**[English](README.md) | 简体中文**

## Approving 是什么？

Approving 把 coding agent 变成工作流里的真实步骤。你在有限状态机上编排 Agent — 节点即状态、边即转移 — 支持成功、失败与回滚路径。当某一步需要人做决定时，运行会停在门禁上，直到有人批准。

Agent 不是在你笔记本上跑的黑盒 prompt。它们通过仓库内嵌的 [sandbox-gateway](sandbox-gateway/) 在 Docker 沙箱中执行，调用按 run 隔离的产物 MCP，并留下可检查的轨迹。支持 **Cursor**、**Claude Code**、**CodeBuddy**、**Trae**。

告别无法撤销的一次性 Agent 跑法。设计路径、把关风险步骤、让每份产物都落在契约之下。

## 为什么叫 "Approving"？

因为关键路径应当需要人点头批准（approve）。

自治 Agent 很强 — 也往往不透明。Approving 把「批准」做成一等公民：检查点、人工门禁、回滚边都是图的一部分，而不是事后补丁。失败时，工作流可以走失败路径或回滚，而不是留下半成品。

名字即产品赌注：Agent 负责加速；人仍拥有真正重要的决策。

## 特性

Approving 覆盖完整 Agent 工作流：从图设计到沙箱执行，再到产物交接。

- **FSM 编排** — 节点即状态、边即转移。成功 / 失败 / 回滚路径、`when` 守卫、检查点与人工门禁。
- **真实 Docker 沙箱** — agent / react 节点经 ACP、通过内嵌 `sandbox-gateway` 在容器中运行；Web UI 走 API。
- **多 Agent 后端** — 同一平台支持 Cursor、Claude Code、CodeBuddy、Trae。按 Agent 选择 `acpBackend`；密钥放在 Agent 元信息 env。
- **产物契约 + 按 run 隔离的 MCP** — Agent 调用 `write_artifact` / `set_*` / `node_complete`；每个 run 用 token 隔离。
- **沙箱内 Git 凭据** — 在 Agent 元信息 env 配置 `GITHUB_*`、`GITLAB_*` 或 SSH（值可引用 `${vars.<name>}`）。GitLab 仍可用平台 `glab` 自动开 MR；GitHub PR 由 Agent 侧 `gh` 处理。
- **单仓自托管** — `sandbox-gateway` 与沙箱镜像源码在本仓库内，一次 clone 即可跑通。

---

## 快速开始

需要 Linux 宿主与 Docker Compose。默认拉取已发布的 GHCR 镜像（无需本地构建）：

```bash
./start.sh -d
```

然后打开：

- UI / API：http://localhost:8080
- API 健康检查：http://localhost:8080/api/health
- 网关健康检查：http://localhost:8899/healthz
- 默认登录：`admin` / `demo1234`（local-demo）

`./start.sh` 还会从 GHCR 拉取 **沙箱运行时镜像**（数 GB）。未拉完前，沙箱对话会停在「正在启动沙箱…」。

常用命令：

```bash
./start.sh logs
./start.sh down
./start.sh pull          # 刷新 GHCR 镜像
./start.sh dev -d        # 源码栈：go run + Vite HMR
```

镜像 tag / digest 可在 `.env` 覆盖 — 见 [`.env.example`](.env.example)。发布钉死与冒烟测试见 [贡献指南](CONTRIBUTING.md#release-images-and-smoke)。

---

## 上手

### 1. 启动栈

```bash
./start.sh -d
```

打开 http://localhost:8080，使用 `admin` / `demo1234` 登录。全新安装默认是空项目，没有样例流水线。

### 2. 配置 Agent

在 **Agent Studio** 创建或编辑 Agent。设置 `acpBackend`（`cursor` / `claude_code` / `codebuddy` / `trae`），并把对应 API Key 写在 Agent env。详见 [`server/README.md`](server/README.md)。

### 3. 新建流水线

通过「新建工作流」创建流水线，再搭建 FSM 图：agent / react 节点、成功与失败边、可选回滚路径，以及需要人工批准才能继续的门禁。

### 4. 运行并观察

如需发布流水线则先发布，再启动一次 run。观察沙箱执行、经 MCP 写入的产物，以及等待批准的门禁。失败步骤可走失败或回滚边，而不是静默中断。

---

## 架构

```
┌──────────────┐     ┌──────────────────┐     ┌──────────────────┐
│  Vue3 +      │────>│  Go Backend      │────>│ sandbox-gateway  │
│  Vue Flow    │<────│  (FSM + MCP)     │<────│  (Docker)        │
└──────────────┘     └────────┬─────────┘     └────────┬─────────┘
                              │                        │
                              │                        ▼
                              │               ┌──────────────────┐
                              └──────────────>│ universal sandbox│
                                产物 MCP      │ (ACP backends)   │
                                              └──────────────────┘
```

| 层 | 技术栈 |
|----|--------|
| 前端 | Vue 3 + Vue Flow |
| 后端 | Go（`approving-server`，环境变量前缀 `APPROVING_`） |
| 执行 | 内嵌 `sandbox-gateway` + universal 沙箱镜像 |
| Agent 后端 | Cursor、Claude Code、CodeBuddy、Trae（ACP） |

## 开发

参与 Approving 代码贡献，请看 [贡献指南](CONTRIBUTING.md)。关键路径 Playwright e2e（CI `web-e2e`）见 [贡献指南 — Critical-path Playwright e2e](CONTRIBUTING.md#critical-path-playwright-e2e)。

**前置要求：** Go、Node.js、Docker Compose（沙箱需 Linux 宿主）

```bash
./start.sh dev -d
```

源码布局：`server/`（Go FSM + 沙箱客户端 + 产物 MCP）、`web/`（Vue UI）、`sandbox-gateway/`（网关 + 沙箱镜像源码）。

配置见 [`server/CONFIGURATION.md`](server/CONFIGURATION.md) 与 `server/config.example.yaml`。优先级：显式环境变量 > 挂载配置文件 > 代码默认值。沙箱 Git 凭据放在 Agent 元信息 env，不进平台配置。

## 许可证

[MIT](LICENSE) © 2026 cocofhu
