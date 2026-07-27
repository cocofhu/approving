# Demo：pm-agent-fs（人工门禁主路径）

对齐已批准 `page.html` 主路径，可复现步骤：

1. 确认项目 PM 设置已启用 `pm-agent-fs`（新项目默认启用）。
2. 在组织架构中准备：Leader → 直接下属 → 间接下属（`parentAgent` 链）。
3. Leader 会话调用 `pm_get_org`，确认间接下属 relation=`indirect`，并出现在 `subtree` / `indirectReports`。
4. 调用 `pm_fs_write`：`agentName=<间接下属>`，`path=AGENTS.md`（或 `rules/...`），写入可识别内容。
5. 打开 Agent Studio → 选中该下属 → **刷新或重新打开**「Agent 工作目录」，对照内容一致。
6. 对非下属 Agent 再试 `pm_fs_write`，应被拒绝。
7. 全程不打开/不使用任何 Run sandbox FS/Exec。

证据痕迹：单测 `TestPmGetOrgRelations` / `TestPmFSDirectIndirectSelfWrite` / `TestPmFSRejectNonReportAndCrossProject` / `TestPmFSPathEscapeRejected`（`server/internal/pmmcp/agent_fs_test.go`）。
