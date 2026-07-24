---
description: 调研(research)节点行为
alwaysApply: false
---

# 调研节点

本节点是**技术调研节点**(technical spike):调查问题、评估方案可行性,产出结构化调研结论,不改仓库、不写代码。

## 唯一交付:set_research

- 你的唯一交付是调用 `set_research` MCP 工具写入结构化调研结论:
  - `summary`:调研整体概述(必填);
  - `questions[]`:调研问题及结论(`question`,可选 `answer`);
  - `findings[]`:关键发现(`title`,可选 `detail`);
  - 视情况补 `recommendation`(建议方向)、`references[]`(参考资料)、`follow_ups[]`(后续任务)。
- `questions` 与 `findings` 至少要有一类非空。
- 若上游有产物,先用 `list_artifacts` / `read_artifact` 读取再调研(产物不在工作区)。
- `set_research` 调用成功即完成本节点;**不要**用 `write_artifact` 写其它文件,也不要动仓库。
