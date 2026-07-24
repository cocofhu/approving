---
name: git
description: 沙箱内 git clone/push 与合并请求（GitLab glab / GitHub gh；HTTPS 与 SSH 通用凭据）
---

# Git 操作指引

沙箱 startup.sh 按 `repo_url` scheme 注入凭据:

| 场景 | 环境变量 | 说明 |
|------|----------|------|
| GitHub HTTPS | `GITHUB_TOKEN` | `https://github.com/...` |
| 自建 GitHub/GHE | `GITHUB_TOKEN` + `GITHUB_URL` | URL 的 scheme+host 须与 repo 主机一致 |
| GitLab / 自建 GitLab | `GITLAB_TOKEN` + `GITLAB_URL` | 例: `GITLAB_URL=https://git.example.com`; 未配时从 repo_url 推导 |
| 任意 SSH 托管商 | `GIT_SSH_PRIVATE_KEY` + `GIT_SSH_KNOWN_HOSTS` | 必填 known_hosts,禁止 accept-new |

Gitea/Bitbucket 等 HTTPS 不支持 Token 注入,请改用 SSH。

## 新功能分支 + push

```bash
cd /root/workspace
git checkout -b feature/<简短描述>-$(date +%s)
# ... 实现改动,跑通测试 ...
git add -A
git commit -m "<语义化提交信息>"
git push -u origin HEAD
```

## 合并请求（MR/PR）— 按托管商分流

`submit_mr` 步骤顺序与 DefaultMRContract 一致:**解决冲突 → 推送源分支 → 按远端主机选型创建/复用合并请求 → `node_complete(outputs.mr_url)`**。

选型规则（按远端主机与环境变量,不按 Token 有无或 CLI 轮询）:

- **GitLab**（gitlab.com,或与 `GITLAB_URL` 主机一致）→ `glab`
- **GitHub**（github.com,或与 `GITHUB_URL` 主机一致,含 GHE）→ `gh pr`
- **匹配不上**（如 Gitea）→ 不支持自动建单:仍须完成冲突解决与 `git push`,再 `node_complete(status=failed)`;摘要须含「冲突已解决」「源分支已推送」及建单/CLI/托管商原因

```bash
# GitLab
glab mr list --source-branch "$(git rev-parse --abbrev-ref HEAD)" -F json
glab mr create --fill --yes --source-branch "$(git rev-parse --abbrev-ref HEAD)" --target-branch <target>

# GitHub
gh pr list --head "$(git rev-parse --abbrev-ref HEAD)"
gh pr create --fill --base <target> --head "$(git rev-parse --abbrev-ref HEAD)"
```

成功时合并请求 Web URL（含 GitHub PR）写入 `outputs.mr_url`（字段名不变）。需预装对应 CLI（`glab` / `gh`）及凭据（`GITLAB_*` / `GITHUB_*`）。

> 平台可选路径 `detect_push` + `create_mr` 本迭代仍仅 GitLab；GitHub 依赖 Agent 侧 `gh`，平台不代建 PR。

## 注意

- 只 push 到 `feature/*`,不要碰 `main` / 受保护分支。
- 不要修改 `.github/workflows` / `.gitlab-ci.yml` / `Dockerfile`,除非任务明确要求。
- 命令失败先 surface 错误原因,不要静默重试。
