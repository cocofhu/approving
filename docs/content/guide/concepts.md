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

待审批 Inbox 里，仅 **human_gate** 卡片（以及有视觉网页产物时的预览工具条）提供「复制临时链接」。登录用户可为当前 pending 实例创建一条一次性链接，发给未登录的外部人员打开**独立一次确认页**（不是内部审批工作台）。

- 默认 24 小时，可选 1h / 8h / 24h / 72h / 7d；每个实例最多 1 条有效链接。
- 管理面板默认掩码展示；复制写入完整 URL。同一浏览器标签刷新后仍可再复制同一有效链。重新生成立刻作废旧链并沿用原档位；立即撤销后链接不可用。门禁仍 pending 时，已撤销/已过期可再创建；审批已完成后只读，不能再创建。`proposal_select` / 澄清 / 应用预览没有此入口。
- 外部页无需登录，采用浅色独立镜框：顶栏徽章「外部一次决策」，主标题固定为「请确认本次交付」；门禁名、一次性、无需登录、预览已脱敏仅作元信息。预览区标签为「待确认的内容」，全幅只读展示脱敏视觉产物与结构化摘要，不可取点、就地改或编辑保存。
- 主出口为「确认」（意见可空）；次要出口为「驳回并说明原因」（意见必填；门禁未配置退回/不通过动作时不展示）。「你的姓名」可选，仅用于审计。不展示项目、Run、成员或内部地址，也不复用 Inbox / 复审三区壳。
- 链接只绑定这一次决策；过期、撤销、提交成功、登录侧先审完或 Run 结束都会立刻失效。已用/过期/撤销/无效状态页沿用同一外部一次确认镜框，不再展示决策按钮。

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
