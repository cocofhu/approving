---
title: 核心概念
description: FSM 编排、人审门禁、沙箱执行与产物契约。
---

## FSM 编排

Approving 把 coding agent 变成工作流里的步骤。你在有限状态机上编排：

- **节点**即状态（agent / react / gate / …）
- **边**即转移，可配置成功、失败、回滚路径
- 可用 `when` 守卫、检查点，把风险步骤显式化

不再是无法撤销的一次性 Agent 跑法：先设计路径，再门禁关键步骤。

## 人审门禁（gate）

当某一步需要人做决定时，运行会停在 **gate**，直到有人批准或拒绝。

批准时刻是一等公民，而不是事后补丁。产品名 Approving 的赌注是：Agent 可以很快，但关键决策仍由人拥有。

### 临时审批链接（human_gate）

待审批 Inbox 里，仅 **human_gate** 卡片（以及有视觉网页产物时的预览工具条）提供「复制临时链接」。登录用户可为当前 pending 实例创建一条一次性链接，发给未登录的外部人员打开**与审批工作台同样的三区界面**（左产物台 | 右多轮 ReAct | 底栏通栏上游上下文 + 确认并流转 / 驳回）。

- 默认 24 小时，可选 1h / 8h / 24h / 72h / 7d；每个实例最多 1 条有效链接。
- 管理面板默认掩码展示；复制写入完整 URL。同一浏览器标签刷新后仍可再复制同一有效链。重新生成立刻作废旧链并沿用原档位；立即撤销后链接不可用。门禁仍 pending 时，已撤销/已过期可再创建；审批已完成后只读，不能再创建。`proposal_select` 没有此入口（澄清与应用预览走下方「待复审临时链接」同策略）。
- 外部页无需登录。顶栏仅种类标签「外部一次决策」+ 剩余时间（冷态可附「会话已结束」）；不展示 Approving 品牌、应用侧栏、Run# 或「打开运行详情」。
- 热会话可在右侧发送 / 取消，视觉网页可取点标注；冷态只读历史，底栏仍可最终处置。确认与驳回均须填写姓名与意见。确认并流转不触发 Agent。
- 一次性令牌仅在确认或驳回成功后消耗；中间 ReAct 发送 / 取消不烧链。过期、撤销、登录侧先审完或 Run 结束都会立刻失效。不可用态保持深色工作台空态，不再使用紫色一次确认镜框。

### 待复审临时链接（Inbox kind=review / app_preview / clarify）

Inbox **待复审**、**应用预览**与 **待澄清**卡片使用同一套管理面板与令牌规则（`ShareLinkKindReview`），认证 API 走 `/api/runs/:id/reviews/:nodeId/share-link*`，不复用 `/gates/...`，也不伪造 Gate 行。站内入口：卡片「复制临时链接」、移动端详情顶栏同名按钮、应用预览工作台工具栏「分享审批」。公开页：待复审标识为「外部复审」；待澄清标识为「待澄清 / 外部澄清」。热态可多轮 ReAct（发送 / 流式轮询 / 取消）；`productKind=app_preview` 时产物区为只读占位说明，**不提供 noVNC / 取点**（登录态同一项仍可远程桌面与取点）。底栏仅「确认并流转」（无驳回、无姓名意见）。澄清确认走 Agent 收尾写入结构化需求，取消仅清当前轮并保留队列。运行详情澄清/复审 Tab / 登录侧复审面板 / 产物预览工具条不提供临时链接入口；`proposal_select` 没有此入口。

## 真实 Docker 沙箱

Agent 不是在笔记本上跑的黑盒 prompt。它们通过仓库内嵌的 [sandbox-gateway](https://github.com/cocofhu/approving/tree/main/sandbox-gateway) 在 Docker 容器中执行，经 ACP 通信。

支持 **Cursor**、**Claude Code**、**CodeBuddy**、**Trae**。按 Agent 配置 `acpBackend`；密钥放在 Agent meta env。

## 产物契约与 MCP

每个 run 有隔离的 artifact MCP。Agent 调用例如：

- `write_artifact`
- `set_*`
- `node_complete`

按 run token 隔离，留下可检查的纸面轨迹。

## PM：`pm-agent-fs`（组织架构 + Agent 工作目录）

项目绑定的 PM Leader 可启用独立 MCP `pm-agent-fs`（新项目默认启用；旧项目若已显式保存过 EnabledMcps 需在 PM 设置中手动勾选）：

- `pm_get_org`：读取组织架构，并标注相对 Leader 的 self / direct / indirect / other
- `pm_fs_*`：对自身与汇报闭包内下属的 **host 侧** `workspace/` 做 list/read/write/delete/mkdir/rename（**不是** Run 沙箱 FS）

写入结果与 Agent Studio「Agent 工作目录」同一磁盘树；**刷新或重新打开该 Agent 后可见**（不做热更新）。若 Studio 仍打开同一 Agent 且存在未保存草稿，随后 Save 可能覆盖 MCP 已写入内容——使用/Demo 时请刷新并避免并行脏写。

## 单仓自托管

`sandbox-gateway` 与通用沙箱镜像源码在本仓库。一次克隆即可自托管。
