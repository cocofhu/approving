# 计划贴合度门禁（plan_coverage）回归说明

面向 wf-07079fa7「自我迭代」：test 阶段强制对照 plan 叶子给出覆盖与非空证据。

## 机检矩阵（单元）

`go test ./internal/mcp/ -run 'TestPlanCoverageOK|TestPlanLeafIDs'` 与
`go test ./internal/engine/ -run 'TestTestGateUnit'`：

| 场景 | 期望 |
|------|------|
| 有叶子且缺 `plan_coverage` / 漏叶子 | fail（reason 含缺少/未覆盖） |
| 未知或重复 `plan_id` | fail |
| `passed=false` 或 evidence 空白 | fail |
| 全覆盖且全 `passed`+非空 evidence | pass |
| 无 plan / 空叶子 | fail-open（不因缺 coverage 失败） |
| cases 有 failed | 仍先被 cases 门禁拦住（与贴合度 AND） |

图测 `TestStructuredGatePlanCoverage*` 覆盖 FailGoto / PassGoto / 无 plan fail-open（需可用的 sqlite 测试环境）。

## Demo 主路径（逻辑）

### A. 故意漏做 → 回修

1. plan 含叶子 `g1.1`、`g1.2`
2. test 写入 cases 全 passed，但 `plan_coverage` 只覆盖 `g1.1`（或某项 `passed=false` / evidence 空白）
3. `PlanCoverageOK` / `testGate` 失败 → `finalizeStructuredGate` action=`fail`
4. 若图配置 `exits.fail.goto=implement`，回到 implement，**不到达 submit_mr**

### B. 完整实现 → 可开 PR

1. 全部叶子均有 `plan_coverage` 且 `passed=true`、evidence 非空
2. cases 无 failed
3. test 门禁通过 → `exits.pass` → submit_mr，可开 PR/MR

### C. 无计划叶子

无 `plan.json` 或叶子为空时，不强制 `plan_coverage`；仅按既有 cases 门禁判定。

## wf-07079fa7 拓扑核对清单

引擎侧贴合度门禁已全局生效；**本改动不修改工作流图 JSON**（复用既有 `exits.fail` / `exits.pass`）。图侧核对结论：

| 检查项 | 结论 |
|--------|------|
| 本 run 为自我迭代（feature 指向 wf-07079fa7） | 是：节点序列 research→clarify→visual→plan→implement→… |
| test 失败走 `finalizeStructuredGate` → action=`fail` → `exits.fail.goto` | 引擎语义已具备；贴合度失败与 cases 失败共用该出口 |
| submit_mr 仅挂 pass 路径 | 未改 submit_mr；未新增 fail→submit_mr 边 |
| 其它工作流 | 本轮未改 |

> 本轮无图结构 diff，无需为贴合度单独改边。若运行环境发布版曾缺失 `exits.fail→implement`，需在管理端打开 wf-07079fa7 补边后 Publish（沙箱无平台登录凭据时由运维完成）。

## 不变式（本需求未改）

- `ensurePlanComplete`（implement 叶子 status=done）仍在
- submit_mr 不做计划贴合度业务校验
- implement 仅软提示，无 `plan_coverage` 硬门禁
