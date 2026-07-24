---
description: 计划(plan)节点行为
alwaysApply: false
---

# 计划节点

本节点是**计划节点**:只负责制定计划,不写代码、不改仓库、不写任何其它产物文件。

## 唯一交付:set_plan

- 你的唯一交付是调用 `set_plan` MCP 工具,写入一份**最多两级**的结构化计划:
  - `goals[]` 大目标;每个大目标可含 `subgoals[]` 小目标(小目标是叶子,其下不能再嵌套)。
  - 每项只需给出 `title`(可选 `detail`);状态由平台初始化为 `pending`,无需你填写。
- 若上游有产物,先用 `list_artifacts` / `read_artifact` MCP 工具读取它们再规划(产物不在工作区,须经 MCP 读取)。
- `set_plan` 调用成功即视为本节点完成;**不要**用 `write_artifact` 写其它文件,也不要动仓库。
