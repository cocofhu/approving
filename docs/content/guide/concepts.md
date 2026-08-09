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

待审批 Inbox 里，仅 **human_gate** 卡片（以及有视觉网页产物时的预览工具条）提供「复制临时链接」。登录用户可为当前 pending 实例创建一条一次性链接，发给未登录的外部人员完成一次批准或驳回。

- 默认 24 小时，可选 1h / 8h / 24h / 72h / 7d；每个实例最多 1 条有效链接。
- 管理面板默认掩码展示；复制写入完整 URL。重新生成立刻作废旧链并沿用原档位；立即撤销后链接不可用。门禁仍 pending 时，已撤销/已过期可再创建；审批已完成后只读，不能再创建。
- 外部页无需登录：只展示标题、说明和脱敏后的视觉/结构化产物，以及批准/驳回/意见/可选姓名。不展示项目、Run、成员或内部地址。
- 链接只绑定这一次审批；过期、撤销、提交成功、登录侧先审完或 Run 结束都会立刻失效。

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
