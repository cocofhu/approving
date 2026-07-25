---
title: 网关
description: sandbox-gateway 契约摘要；完整说明见 GATEWAY.md。
---

Approving 通过 vendored 的 **sandbox-gateway** 控制平面调度通用沙箱镜像。Web UI 走 Approving API；Agent / react 节点经 gateway 在容器中执行。

## 完整文档

- [GATEWAY.md](https://github.com/cocofhu/approving/blob/main/GATEWAY.md)

## 健康检查（默认本地栈）

- Gateway：http://localhost:8899/healthz
- Approving API：http://localhost:8080/api/health

## 源码位置

- 控制平面：`sandbox-gateway/gateway/`
- 沙箱镜像与脚本：`sandbox-gateway/sandbox/`、`sandbox-gateway/scripts/`

## 相关

- [配置](../configuration/)
- [核心概念](../../guide/concepts/)
