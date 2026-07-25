# Changelog

All notable public-release changes are documented here.

## Unreleased

- IM Reply/Work equivalent orchestration on QQ channels: `channels.Manager` is
  the unique egress (immediate ACK with ~40-rune summary, per-message queue ACK
  with ahead count + summary, dequeue ACK, on-demand progress, terminal report);
  Work (`ChannelBridge` / cron sandbox) emits internal progress/results only.
  Progress coalesces ACP streaming chunks before classify; push flush re-queues
  all remaining items if the conversation becomes busy mid-flush; full push
  queue retains newest failed/changed. Cron `deliverToChannel` uses coordinated
  push (busy → silent queue depth 8, idle flush; unchanged templates like
  `PR：无变化`).
- Preinstall GitHub CLI (`gh`) in the universal sandbox image and auto
  `gh auth login` from `GITHUB_TOKEN` (mirrors existing `glab` + `GITLAB_TOKEN`),
  so Agent `submit_mr` / `gh pr` works out of the box on GitHub remotes.
- Add path-filtered `ci-sandbox` workflow (startup script smokes, sandbox Go
  coverage, Dockerfile `--target cli-tools` verifying `glab`/`gh`).
- Make `./start.sh` default to published GHCR images (`compose.release.yaml`) with
  tag defaults so a clone can `./start.sh -d` without a local image build; keep
  `./start.sh dev` for the source/HMR stack.
- Add GHCR publish workflows for `sandbox-gateway` and
  `universal-sandbox-{cursor,claude_code,codebuddy,trae}` on `v*` tags.

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
