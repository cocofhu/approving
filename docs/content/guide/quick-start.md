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

Agent / workspace / platform-rules 持久在命名卷（`/app/data`）；`./start.sh restart` 与 `./start.sh down`（无 `-v`）会保留。**勿用 `docker compose down -v`**，否则会清空 Agent 配置与 SQLite。

## 常用命令

```bash
./start.sh logs
./start.sh down
./start.sh pull          # 刷新 GHCR 镜像（含四个 sandbox runtime）
./start.sh restart       # down + up -d（保留命名卷）
./start.sh dev -d        # 源码栈：go run + Vite HMR
```

镜像 tag / digest 可在 `.env` 覆盖 — 见仓库根目录 [`.env.example`](https://github.com/cocofhu/approving/blob/main/.env.example)。默认按 backend 分流沙箱镜像；仅当显式设置 `SANDBOX_IMAGE` / `APPROVING_SANDBOX_IMAGE` 时才全局强制。发布与 smoke 见 [Contributing](https://github.com/cocofhu/approving/blob/main/CONTRIBUTING.md)。

## 下一步

- [核心概念](../concepts/) — FSM、gate、sandbox、artifact
- [配置摘要](../../help/configuration/) — 指向完整 `CONFIGURATION.md`
- [网关摘要](../../help/gateway/) — 指向 `GATEWAY.md`
