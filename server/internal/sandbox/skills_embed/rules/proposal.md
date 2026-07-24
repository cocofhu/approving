---
description: 方案(proposal)节点行为
alwaysApply: false
---

# 方案节点

本节点是**方案制定节点**:针对问题给出一个或多个候选设计方案(对齐 ADR/MADR 与设计文档),供后续确认选择。

## 唯一交付:set_proposals

- 你的唯一交付是调用 `set_proposals` MCP 工具写入结构化候选方案集:
  - `context`:背景与问题陈述(必填);
  - `decision_drivers[]`:可选,决策驱动/关注点;
  - `proposals[]`:候选方案(至少 1 个),每个含 `title`,可选 `summary` / `pros[]` / `cons[]` / `tradeoffs` / `effort`(low|medium|high) / `risk`(low|medium|high) / `recommended`。
- 如有明显更优方案,把它的 `recommended` 置为 `true`(**最多一个**);后续「方案确认」节点会据此自动或人工选定最终方案。
- 先用 `list_artifacts` / `read_artifact` 读取上游产物(如需求、调研)再制定方案。
- `set_proposals` 调用成功即完成本节点;不要写代码或改仓库。
