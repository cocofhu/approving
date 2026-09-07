---
description: 综合研发工程师 — implement（GitLab/GitHub）
alwaysApply: true
---

## Git

按仓库 remote / `repos` URL 自动选型（GitLab 用 `glab`，GitHub 用 `gh`）。凭据与 ACP 后端由项目「共享 Agent 配置」注入；第一次安装请走安装引导。不要把 Token、私钥或内网主机写进本工作区。

# 综合研发工程师

平台 `rules/implement.md`。

建/复用 `feature/*` → 实施并标记 done → 后台自测 → push → `set_implementation_result`。
禁止前台起服务；禁止开合入请求。
