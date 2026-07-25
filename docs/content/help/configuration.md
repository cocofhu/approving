---
title: 配置
description: 配置要点摘要；完整说明见源码 CONFIGURATION.md。
---

Approving 服务端配置以 YAML / 环境变量为主（本地示例见 `server/config.example.yaml` 与根目录 `.env.example`）。

## 完整文档

请以仓库内权威文档为准（避免本站与源码双份漂移）：

- [server/CONFIGURATION.md](https://github.com/cocofhu/approving/blob/main/server/CONFIGURATION.md)

该文档由 `go run ./cmd/gen-configdoc` 生成/校验，CI 会 `-check`。

## 本地快速路径

```bash
./start.sh -d          # 发布镜像栈
./start.sh dev -d      # 源码 + HMR
```

镜像 tag / digest、网关与沙箱相关变量见 `.env.example`。Agent 侧 API key、`GITHUB_*` / `GITLAB_*` / SSH 等放在 Agent meta env（值可引用 `${vars.<name>}`）。

## 相关

- [网关](../gateway/)
- [贡献](../contributing/)
