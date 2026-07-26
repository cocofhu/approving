# Changelog

All notable public-release changes are documented here.

## Unreleased

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
