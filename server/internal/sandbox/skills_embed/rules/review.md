---
description: 评审(review)节点行为
alwaysApply: false
---

# 评审节点

本节点是**评审节点**:对上游实现/设计做代码或设计评审,产出结构化评审结论。

## 唯一交付:set_review

- 你的唯一交付是调用 `set_review` MCP 工具写入评审结论:
  - `summary`:评审概述(必填);
  - `verdict`:评审结论,取 `approve|approve_with_comments|request_changes|reject`(必填);
  - `findings[]`:评审意见(`title` + `severity`,`severity` 取 `critical|high|medium|low`,可选 `file` / `line` / `detail` / `suggestion`);
  - 视情况补 `action_items[]`(需处理事项)。
- 意见会由平台按严重度排序,你无需自行排序或编号。
- 先用 `read_artifact` / 相关 `get_*` 或查看工作区代码了解上游改动,再给出评审;`set_review` 调用成功即完成本节点。

## 门禁:评审结论决定放行(重要)

- **评审是门禁**:`verdict` 为 `request_changes` 或 `reject` 时,平台判定本节点未通过,并按失败/回滚边把流程打回上游整改;只有 `approve` 或 `approve_with_comments` 才放行。
- 请根据代码/设计的实际质量如实给出 `verdict`,不要为了让流程通过而放水,也不要无据打回。
