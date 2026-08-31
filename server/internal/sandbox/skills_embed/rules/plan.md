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
- **设计区(可选,写入则须完整)**:`architecture` / `data_design` / `interfaces` / `components` / `interaction` / `test_design`。一旦写入设计区,六节应齐全;无实质内容显式写「不涉及」,禁止静默省略。纯 goals-only 仍合法。
- **图按需、非强制**:`architecture`/`data_design`/`interaction` 可挂 `diagrams[]`(及兼容单数 `diagram`);`interfaces`/`components` 项亦可选同结构。图对象含 `kind`/`title`/`scope`/`format?`/`source`/`fallback_artifact?`/`caption?`(有对象则 source 必填;format 缺省 mermaid)。一等 kind:`activity` / `flowchart` / `sequence` / `er`。**禁止「必须同时提交四种图否则失败」**。涉及活动/业务流/时序/数据时尽量都提供以便审批;未涉及的种类不必出;多子模块按需补图并写 `scope`。前端同节多图用**节内小 Tab**(不是左目录+右画布);0 张无图位,1 张无 Tab。
- **实质 data_design 硬门禁**:当 `data_design.summary`(去空白)不是「不涉及」/「N/A」时,必须提供至少一张 ER(`diagrams[]` 中 `kind=er`,或兼容单数 `diagram`)、至少 1 个实体,且每个实体至少 1 个结构化 `fields[]`(`name`+`type` 必填;可选 `pk`/`nullable`/`fk`/`description`);仅 legacy `attributes` 不足以通过。缺 activity/flowchart/sequence **不**失败。流程:set_plan → 解析与硬门禁 → 入库 → PlanView 展示。
- 若上游有产物,先用 `list_artifacts` / `read_artifact` MCP 工具读取它们再规划(产物不在工作区,须经 MCP 读取)。
- `set_plan` 调用成功即视为本节点完成;**不要**用 `write_artifact` 写其它文件,也不要动仓库。
