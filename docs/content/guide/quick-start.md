---
title: 快速开始
description: 在 Linux 主机上用 Docker Compose 拉起 Approving。
---

## 前提

- Linux 主机
- 已安装 Git、Docker 与 Docker Compose

## 克隆仓库

```bash
git clone https://github.com/cocofhu/approving.git
cd approving
```

## 启动

默认路径从 GHCR 拉取已发布镜像（无需本地构建镜像）：

```bash
./start.sh -d
```

然后打开：

- UI / API：http://localhost:8080
- API health：http://localhost:8080/api/health
- Gateway health：http://localhost:8899/healthz
- 默认登录：`admin` / `demo1234`（local-demo）

`./start.sh` 还会从 GHCR 拉取 **四个 sandbox runtime** 镜像（按 acpBackend：cursor / claude_code / codebuddy / trae，体积较大）。完成前，沙箱对话可能停留在 “starting sandbox…”。

Agent / workspace / platform-rules 与 SQLite 持久在仓库根 `.localdata` 宿主机目录（bind mount：`gateway` / `db` / `app-data`）。`./start.sh restart` 与 `./start.sh down` 会保留该目录。清空数据：`./start.sh down && rm -rf .localdata`。

### 数据库与附件同生命周期（备份 / 清理）

正式栈（`compose.release.yaml`）把 **SQLite** 挂在 `./.localdata/db`，把 **应用数据（含默认附件 blobs，相对 `WORKDIR` 的 `data/blobs`）** 挂在 `./.localdata/app-data`。复合变量附图在库/Run 输出里只保留 `blob:{id}` 引用，字节落在 blobs 目录；若只备份或只清理其中一侧，会出现「库里还有引用、GET `/api/blobs/:id` 却 404」的孤儿引用，Run 详情里附图会显示为「无法显示 / 附件不可用」。

运维约束：

- **成对备份**：同一备份集须同时包含 `./.localdata/db` 与 `./.localdata/app-data`（或整棵 `.localdata`）。
- **成对清理 / 迁移 / 升级**：不要只搬 SQLite 或只删附件目录；自定义 `APPROVING_BLOBS_ROOT` 时，须把该路径与数据库一并纳入同一生命周期。
- **历史孤儿**：已损坏的引用不保证可从附件存储找回；界面仅展示永久失败占位。本次交付**不做**孤儿巡检台、批量扫描页或启动/健康检查告警。

## 常用命令

```bash
./start.sh logs
./start.sh down
./start.sh pull          # 刷新 GHCR 镜像（含四个 sandbox runtime）
./start.sh restart       # down + up -d（保留 .localdata）
./start.sh dev -d        # 源码栈：go run + Vite HMR
```

镜像 tag / digest 可在 `.env` 覆盖 — 见仓库根目录 [`.env.example`](https://github.com/cocofhu/approving/blob/main/.env.example)。默认按 backend 分流沙箱镜像；仅当显式设置 `SANDBOX_IMAGE` / `APPROVING_SANDBOX_IMAGE` 时才全局强制。发布与 smoke 见 [Contributing](https://github.com/cocofhu/approving/blob/main/CONTRIBUTING.md)。

## 下一步

- 登录后进入空项目时，可按「首次上手引导」配置后端与 API Key，一键生成 5 个 Agent 与「快速上手·轻量」工作流（默认 clone Heroku 官方 nodejs-getting-started 公开仓）。引导使用固定 Agent 名（Clarify/Visual/Implement/Test/Preview），同一 Approving 实例内仅一个项目可完成该引导。
- [核心概念](../concepts/) — FSM、gate、sandbox、artifact
- [配置摘要](../../help/configuration/) — 指向完整 `CONFIGURATION.md`
- [网关摘要](../../help/gateway/) — 指向 `GATEWAY.md`
