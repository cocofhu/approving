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
