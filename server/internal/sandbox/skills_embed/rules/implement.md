---
description: 实现(implement)节点行为
alwaysApply: false
---

# 实现节点

本节点是**实现节点**:消费上游计划并逐项落地实现。

## 先建工作分支

- **动手写代码前先新建一个工作分支**:`git checkout -b feature/<简短任务描述>`。分支名由你按任务自拟(用 `feature/` 前缀,禁止直接在 `main`/`master`/`release-*` 上提交)。
- 这个分支名会成为本节点的产出:平台在收尾时捕获各仓工作分支并写入全局变量 `branches`(仓名→分支),供下游节点按仓检出拿到你的代码。

## 读取计划(只读)

- 先调用 `get_plan` MCP 工具获取计划(计划不在工作区文件里,须经 MCP 读取)。
- 计划**只读**:你不能修改计划结构,`set_plan` 对本节点不可用。

## 按计划实施并标记进度

- 按大目标 → 小目标的顺序逐项实施。
- 开始一项前调用 `update_plan_status(id, "in_progress")`;完成该项后调用 `update_plan_status(id, "done")`。
  - `id` 为计划项标识,如大目标 `g1` 或小目标 `g1.2`。
- **必须把所有计划项都标记为 `done` 才算完成**;否则平台会自动要求你继续未完成的项。
- **软提示(计划贴合度)**:实现应按每个 plan 叶子交付,并在 `set_implementation_result` / 提交说明中留下便于测试阶段填写 `plan_coverage.evidence` 的可核对痕迹(改了哪些文件/行为)。implement 节点**不**因缺少 `plan_coverage` 而硬失败——贴合度硬门禁在 test 阶段。

## 收尾:提交推送 + set_implementation_result

- 所有计划项都 `done` 后,**先提交并推送代码**:
  - `git add -A && git commit -m "…"` 提交所有改动;
  - `git push -u origin <当前分支>` 推送工作分支到远端。
  - ⚠️ 下游节点(测试 / 评审等)在**全新克隆**里工作,**只有推送到远端的分支它们才能拿到你的代码**。若忘记推送,平台会在收尾时兜底自动提交并推送,但请以你自己规范的提交为准。
- 再调用 `set_implementation_result` MCP 工具写入结构化的实现结果说明:
  - `summary`:本次实现概述;可选 `change_type`、`changed_areas[]`(主要改动点)、`tests[]`、`breaking_changes[]`、`follow_ups[]`。
- 平台会把各仓工作分支名写入全局变量 `branches`(仓名→分支),下游节点据此按仓自动检出。

> ⚠️ 关键:平台**只**依据 `update_plan_status` 的状态判断完成度。即使你已经把代码写好、测试通过,只要没有把对应计划项标记为 `done`,就会被判为"未完成"并被反复催促,最终可能判定节点失败。请把"做完一项立刻标记 done"作为固定习惯,结束前再次确认没有遗漏的叶子项。
