# Approving

**Agent 工作流与人类协同推进，开启多 Agent 协作新范式。**

Approving 是一个开源、可自托管的多 Agent 工作流平台。它把 coding agent 编排成可视化、可审查、可回滚的交付流程：Agent 在真实 Docker 沙箱中执行，关键节点由人 **Approve** 后再继续。

[项目站](https://www.approving-ai.com/) · [快速开始](https://www.approving-ai.com/guide/quick-start/) · [贡献指南](CONTRIBUTING.md) · [配置](server/CONFIGURATION.md)

**[English](README.md) | 简体中文**

[![CI Server](https://github.com/cocofhu/approving/actions/workflows/ci-server.yml/badge.svg)](https://github.com/cocofhu/approving/actions/workflows/ci-server.yml)
[![CI Web](https://github.com/cocofhu/approving/actions/workflows/ci-web.yml/badge.svg)](https://github.com/cocofhu/approving/actions/workflows/ci-web.yml)
[![CI Sandbox](https://github.com/cocofhu/approving/actions/workflows/ci-sandbox.yml/badge.svg)](https://github.com/cocofhu/approving/actions/workflows/ci-sandbox.yml)
[![CI Gateway](https://github.com/cocofhu/approving/actions/workflows/ci-gateway.yml/badge.svg)](https://github.com/cocofhu/approving/actions/workflows/ci-gateway.yml)

[![coverage-web](https://img.shields.io/endpoint?url=https%3A%2F%2Fraw.githubusercontent.com%2Fcocofhu%2Fapproving%2Fcoverage-badges%2Fcoverage-web.json)](https://github.com/cocofhu/approving/actions/workflows/ci-web.yml)
[![coverage-sandbox](https://img.shields.io/endpoint?url=https%3A%2F%2Fraw.githubusercontent.com%2Fcocofhu%2Fapproving%2Fcoverage-badges%2Fcoverage-sandbox.json)](https://github.com/cocofhu/approving/actions/workflows/ci-sandbox.yml)
[![coverage-server](https://img.shields.io/endpoint?url=https%3A%2F%2Fraw.githubusercontent.com%2Fcocofhu%2Fapproving%2Fcoverage-badges%2Fcoverage-server.json)](https://github.com/cocofhu/approving/actions/workflows/ci-server.yml)
[![coverage-gateway](https://img.shields.io/endpoint?url=https%3A%2F%2Fraw.githubusercontent.com%2Fcocofhu%2Fapproving%2Fcoverage-badges%2Fcoverage-gateway.json)](https://github.com/cocofhu/approving/actions/workflows/ci-gateway.yml)

> 当前版本为公开 Beta。需要 Linux 宿主和 Docker Compose；首次启动会拉取体积较大的沙箱运行时镜像。

## 为什么需要 Approving？

单 Agent 的 Vibe Coding 擅长完成一次任务，但当需求扩展到调研、方案、实现、测试和评审时，新的瓶颈会出现：

- 协作过程藏在对话中，难以复用和审计；
- 多个 Agent 并行后，人的理解与审核速度限制整体吞吐；
- Agent 之间缺少稳定、可验证的产物交接；
- 高风险操作缺少明确的人类决策点；
- 失败通常依赖人工重试，缺少显式的恢复与回滚路径。

Approving 在 Agent 之上提供一层 Harness：用 FSM 设计路径，用沙箱隔离执行，用 MCP 交接产物，并把人工审批变成工作流中的一等节点。

## 核心能力

### 可视化 FSM 编排

通过 Vue Flow 画布编排 agent、react 和 gate 节点。边可以表达成功、失败与回滚路径，并可结合 `when` 守卫和 checkpoint 控制流程。

### Human-in-the-loop 门禁

工作流可以在方案选择、视觉验收、发布确认等节点暂停。审批者查看结构化产物后，可批准、拒绝或退回修改，而不必持续跟踪 Agent 的每一步执行。

### 真实 Docker 沙箱

agent / react 节点通过仓库内置的 `sandbox-gateway` 在独立容器中运行。平台统一管理沙箱生命周期，ACP bridge 负责连接不同 Agent 后端。

### 多 Agent 后端

同一套工作流可使用：

- Cursor
- Claude Code
- CodeBuddy
- Trae

每个 Agent 独立选择 `acpBackend`，密钥放在 Agent 元信息 env 中，不写入平台镜像。

### Run 级产物契约

每次 run 拥有隔离的 artifact MCP 和 token。Agent 使用 `write_artifact`、`read_artifact`、`set_*`、`node_complete` 等工具完成结构化交接；平台在节点结束前检查必要产物是否存在。

### Git 与交付

GitHub、GitLab 或 SSH 凭据可按 Agent 注入沙箱，值支持 `${vars.<name>}` 引用。GitLab 可通过 `glab` 创建 MR；GitHub PR 由 Agent 在沙箱内使用 `gh` 创建。

### 运行观测

通过运行详情、执行时间线、沙箱日志、产物、待审批收件箱和 Token 统计查看任务状态与资源消耗。

## 典型工作流

一个完整的软件交付流程可以拆分为：

```text
需求澄清 → 技术调研 → 方案设计 → 人工审批
        → 执行计划 → 代码实现 → 测试验证 → 代码评审
        → 人工确认 → PR / MR
```

仓库内提供 Clarify、Research、Proposal、Plan、Implement、Test、Review 等角色 Agent 包，可通过 `agents/pack.sh` 打包后导入 Agent Studio。

## 快速开始

### 前置条件

- Linux 主机
- Git
- Docker 与 Docker Compose

### 启动

默认路径直接拉取已发布的 GHCR 镜像，无需本地构建：

```bash
git clone https://github.com/cocofhu/approving.git
cd approving
./start.sh -d
```

启动后访问：

- UI / API：<http://localhost:8080>
- API 健康检查：<http://localhost:8080/api/health>
- Gateway 健康检查：<http://localhost:8899/healthz>
- 本地演示账号：`admin` / `demo1234`

> `./start.sh` 还会拉取数 GB 的沙箱运行时镜像。完成前，沙箱对话可能停留在“正在启动沙箱…”。

常用命令：

```bash
./start.sh logs          # 查看日志
./start.sh down          # 停止服务
./start.sh pull          # 刷新 GHCR 镜像
./start.sh dev -d        # 源码开发栈：Go + Vite HMR
```

镜像 tag / digest 可在 `.env` 中覆盖，参见 [`.env.example`](.env.example)。

## 四步创建第一个工作流

1. 使用本地演示账号登录。全新安装默认是空项目，不会自动创建样例流水线。
2. 在 **Agent Studio** 创建 Agent，选择 `cursor`、`claude_code`、`codebuddy` 或 `trae`，并配置对应 API Key。
3. 新建工作流，在画布中连接 Agent 节点、成功/失败边、回滚路径与人工 gate。
4. 发布并启动 run，观察沙箱执行、MCP 产物和待审批节点。

后端鉴权和 Agent env 配置详见 [`server/README.md`](server/README.md)。

## 系统架构

- `web/`：Vue 3 + Vue Flow，负责画布、运行详情、审批与 Agent Studio。
- `server/`：Go 后端，负责 FSM 引擎、API、SQLite、artifact MCP、调度与审计。
- `sandbox-gateway/gateway/`：沙箱生命周期控制面。
- `sandbox-gateway/sandbox/`：通用沙箱镜像与 ACP bridge。
- `agents/`：可导入的角色 Agent 工作区。
- `docs/`：项目站和中英文帮助文档。

配置优先级为：显式环境变量 > 挂载配置文件 > 代码默认值。完整配置见 [`server/CONFIGURATION.md`](server/CONFIGURATION.md)，网关契约见 [`GATEWAY.md`](GATEWAY.md)。

## 开发与质量

**开发前置：** Go、Node.js、Docker Compose；运行沙箱需要 Linux 宿主。

```bash
./start.sh dev -d
```

各模块的 lint、测试、覆盖率和 E2E 命令见 [`AGENTS.md`](AGENTS.md) 与 [`CONTRIBUTING.md`](CONTRIBUTING.md)。安全工作流在 push / PR 时运行 CodeQL、Web `npm audit` 和 gitleaks。

## 部署与安全提示

- 默认账号仅用于本地演示；共享或生产环境必须配置自己的鉴权用户。
- ACP API Key 与 Git 凭据应配置在项目或 Agent env，不应提交到仓库。
- 发布环境建议使用 digest 固定镜像，参考 [Release images and smoke](CONTRIBUTING.md#release-images-and-smoke)。
- 项目仍处于 Beta 阶段，请在实际环境中完成安全评估、备份和容量验证。

## 文档

- [核心概念](docs/content/guide/concepts.md)
- [快速开始](docs/content/guide/quick-start.md)
- [完整配置](server/CONFIGURATION.md)
- [Gateway 契约](GATEWAY.md)
- [贡献指南](CONTRIBUTING.md)
- [安全策略](SECURITY.md)
- [支持渠道](SUPPORT.md)

## 参与贡献

欢迎提交 Issue 和 Pull Request。请先阅读 [`CONTRIBUTING.md`](CONTRIBUTING.md)、[`AGENTS.md`](AGENTS.md) 与 [`CODE_OF_CONDUCT.md`](CODE_OF_CONDUCT.md)。

## 许可证

[MIT](LICENSE) © 2026 cocofhu
