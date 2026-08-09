---
name: git-mr
description: Implement 节点分支/提交/push/MR 约定与检查项（不含任何凭据）
---

# Git / MR 操作检查清单

本 skill 补充 ImplementAgent 在多仓工作区中的版本管理约定。凭据由运行时环境或 Agent Studio 注入，**禁止**写入仓库或本 Agent 目录。

## 工作区布局

- 工作区根通常不是 git 仓库；每个仓在 `/root/workspace/<name>/`
- 对每个有改动的仓分别 `cd` 进其目录再执行 git 操作
- 多个仓可以使用同一工作分支名，但必须各自 push

## 推荐流程

1. 在目标仓创建或切换到工作分支（如 `feature/<简短描述>`），避免直接推 `main` / 受保护分支
2. 实现并完成本地验证后：`git add` 相关文件 → `git commit`（语义化说明 why）→ `git push -u origin HEAD`
3. 如任务要求创建 GitLab MR，使用环境已配置的 `glab`（或其他平台等价工具）；GitHub 等按任务说明处理
4. 在 `set_implementation_result` 中写明各仓工作分支名

## 检查项

- [ ] 每个有改动的仓都已 commit 且 push 到远端工作分支
- [ ] 提交信息不包含密钥、Token、私钥或 `.env` 实值
- [ ] 未将 ACP/Git 凭据、Token、私钥写入 `agents/`、业务源码或任何明文文件；配置仅允许 `${...}` 占位模板
- [ ] 未擅自修改 `.github/workflows` / `.gitlab-ci.yml` / `Dockerfile`（除非计划明确要求）
- [ ] 推送失败时先暴露错误原因，不静默重试掩盖问题

## 禁止事项

- **禁止密钥入库**：不得将 `GITLAB_TOKEN`、`GITHUB_TOKEN`、SSH 私钥、ACP Key 等写入仓库
- 不要 force push 到 main/master；不要跳过 hooks，除非任务明确要求
- 不要提交 `dist/`、构建产物、本地密钥文件或与计划无关的大文件
