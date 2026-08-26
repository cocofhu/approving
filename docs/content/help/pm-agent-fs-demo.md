# Demo：pm-agent-fs（人工门禁主路径）

对齐已批准 `page.html` 主路径，可复现步骤：

1. 确认项目 PM 设置已启用 `pm-agent-fs`（新项目默认启用）。
2. 在组织架构中准备：同项目多个 Agent（绑定相同 `projectId`），可挂虚拟组。
3. Leader 会话调用 `pm_get_org`，确认同项目成员 relation=`direct`，并出现在 `directReports` / 扁平 `subtree`。
4. 调用 `pm_fs_write`：`agentName=<同项目另一 Agent>`，`path=AGENTS.md`（或 `rules/...`），写入可识别内容。
5. 打开 Agent Studio → 选中该 Agent → **刷新或重新打开**「Agent 工作目录」，对照内容一致。
6. 对跨项目 Agent 再试 `pm_fs_write`，应被拒绝。
7. 全程不打开/不使用任何 Run sandbox FS/Exec。

证据痕迹：单测 `TestPmGetOrgRelations` / `TestPmFSDirectIndirectSelfWrite` / `TestPmFSRejectNonReportAndCrossProject` / `TestPmFSPathEscapeRejected`（`server/internal/pmmcp/agent_fs_test.go`）。
