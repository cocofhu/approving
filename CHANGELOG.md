# Changelog

All notable public-release changes are documented here.

## Unreleased

## 0.2.0-beta — 2026-08-13

- Public beta follow-up on [`v0.2.0-beta`](https://github.com/cocofhu/approving/releases/tag/v0.2.0-beta)
  (relative to `v0.1.2-beta`: ~385 commits, PRs through #309). Full notes on the GitHub Release.
- Pin `./start.sh`, `.env.example`, and `compose.release.yaml` defaults to GHCR
  `*:0.2.0-beta` (tag publish does not rewrite these files).
- Highlights: QQ/企微/飞书/钉钉等多渠道与 live 接话；公开审批与分享链路；需求草稿/PRD；
  通知中心与顶栏指标；应用预览/noVNC；工作流收藏与团队模板；大量 Web Loading/布局打磨。

## 0.1.2-beta — 2026-07-31

- Public beta follow-up on [`v0.1.2-beta`](https://github.com/cocofhu/approving/releases/tag/v0.1.2-beta)
  (relative to `v0.1.1-beta`: PRs #143–#151). Full notes on the GitHub Release.
- Pin `./start.sh`, `.env.example`, and `compose.release.yaml` defaults to GHCR
  `*:0.1.2-beta` (tag publish does not rewrite these files).
- Highlights: Gates Inbox 纳入 app_preview 待审批；Inbox 布局/预览预算修复；inspect 可取消；
  revise 失败不再显示 Done；PreviewIssues 冷会话退回；sandbox `/tmp` PVC；Token 实心饼图；
  ArtifactPreview 图片 padding；输出节点 goto 邻接选项。

## 0.1.1-beta — 2026-07-29

- Public beta follow-up on [`v0.1.1-beta`](https://github.com/cocofhu/approving/releases/tag/v0.1.1-beta)
  (relative to `v0.1.0-beta`: PRs #136–#141). Full notes on the GitHub Release.
- Pin `./start.sh`, `.env.example`, and `compose.release.yaml` defaults to GHCR
  `*:0.1.1-beta` (tag publish does not rewrite these files).
- Highlights: Token 按模型统计；审计分页升级；HTML 主产物放大；沙箱环境变量单行启用开关；
  human_gate 冷会话静默；Run 筛选触发文案左对齐。

## 0.1.0-beta — 2026-07-29

- Public beta on [`v0.1.0-beta`](https://github.com/cocofhu/approving/releases/tag/v0.1.0-beta)
  (relative to `v0.0.4-beta`: PRs #113–#134). Full notes on the GitHub Release.
- Pin `./start.sh`, `.env.example`, and `compose.release.yaml` defaults to GHCR
  `*:0.1.0-beta` (tag publish does not rewrite these files).
- Highlights: 评审统一「确认并流转」；澄清/预览流式与刷新续传；桌面 HTML 预览 fillParent；
  项目管理文案；app_preview 纯 ReAct；同项目 skill_profile 约束。

## 0.0.4-beta — 2026-07-28

- Public beta follow-up on [`v0.0.4-beta`](https://github.com/cocofhu/approving/releases/tag/v0.0.4-beta)
  (relative to `v0.0.3-beta`: onboarding bootstrap #109 + pin). Full notes on the GitHub Release.
- Pin `./start.sh`, `.env.example`, and `compose.release.yaml` defaults to GHCR
  `*:0.0.4-beta` (tag publish does not rewrite these files).
- Highlights: 空项目首次上手引导（五步向导 + bootstrap 五 Agent +「快速上手·轻量」；默认 well-known Heroku git）。

## 0.0.3-beta — 2026-07-28

- Public beta follow-up on [`v0.0.3-beta`](https://github.com/cocofhu/approving/releases/tag/v0.0.3-beta)
  (relative to `v0.0.2-beta`: ~43 commits, PRs #62–#106). Full notes on the GitHub Release.
- Pin `./start.sh`, `.env.example`, and `compose.release.yaml` defaults to GHCR
  `*:0.0.3-beta` (tag publish does not rewrite these files).
- Highlights: PM MCP artifact/react/fs；QQ 通知与模板；审计轨迹；TagFilter；澄清/审批流式续传；
  release 持久化 `/app/data` + 按 acpBackend 多镜像；安全/CI（CodeQL、x/net、gateway Dockerfile）。

## 0.0.2-beta — 2026-07-26

- Public beta follow-up on [`v0.0.2-beta`](https://github.com/cocofhu/approving/releases/tag/v0.0.2-beta)
  (relative to `v0.0.1-beta`: ~161 commits, PRs #1–#61). Full notes on the GitHub Release.
- Pin `./start.sh`, `.env.example`, and `compose.release.yaml` defaults to GHCR
  `*:0.0.2-beta` (tag publish does not rewrite these files).
- Highlights: Run delete/cancel/sort + Token KPIs; Board/project Token stats;
  Agent 5-step wizard; PM IM QQ egress + cron UTC fix; Docker/K8s sandbox logs;
  docs `/en` + marketing site; CI coverage gates, e2e, CodeQL/security cleanup.

## 0.0.1-beta — 2026-07-25

- Initial public beta of Approving (MIT, Copyright 2026 cocofhu).
- Strip private registry, Apollo, and k3s preview hosts; default sandbox images
  to `universal-sandbox-*:local` (keep public `github.com/cocofhu` / `ghcr.io/cocofhu`).
- Adopt `APPROVING_*` environment variables, `.approving/` workspace paths, and
  matching binary/image names.
- Remove repository GitLab CI, VitePress docs site, showcases, deploy previews,
  and root selftest scripts. Keep sandbox Git host authorization for GitLab and
  GitHub (`GITLAB_*` / `GITHUB_*` / SSH on Agent meta env).
- Vendor `sandbox-gateway` (control plane + universal sandbox image) into
  `sandbox-gateway/` so a single clone runs via `./start.sh` / `docker compose up`.
- Add GitHub Actions CI, release-smoke, and GHCR publish workflows; root
  community policy files; `GATEWAY.md` and generated `server/CONFIGURATION.md`.
- Chinese README (`README.zh-CN.md`).
