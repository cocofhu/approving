package mcp

import (
	"encoding/json"
	"fmt"
	"strings"
)

// PlanCoverageOK checks test_result.json plan_coverage against plan.json leaves.
//
// Rules (aligned with clarified plan-fit gate):
//   - no plan / no leaves → pass (fail-open)
//   - with leaves: require plan_coverage covering every leaf exactly once,
//     each with passed=true and non-empty/non-whitespace evidence
//   - unknown or duplicate plan_id → fail
//
// Distinguishable reason strings help agents repair on exits.fail → implement.
func PlanCoverageOK(testJSON, planJSON string) (bool, string) {
	leaves := PlanLeafIDs(planJSON)
	if len(leaves) == 0 {
		return true, ""
	}

	var doc struct {
		PlanCoverage []struct {
			PlanID   string `json:"plan_id"`
			Passed   bool   `json:"passed"`
			Evidence string `json:"evidence"`
		} `json:"plan_coverage"`
	}
	if json.Unmarshal([]byte(testJSON), &doc) != nil {
		return false, "计划贴合度校验失败:无法解析 test_result.json"
	}
	if len(doc.PlanCoverage) == 0 {
		return false, "计划贴合度校验失败:缺少 plan_coverage(有计划叶子时必填)"
	}

	leafSet := make(map[string]struct{}, len(leaves))
	for _, id := range leaves {
		leafSet[id] = struct{}{}
	}
	seen := make(map[string]struct{}, len(doc.PlanCoverage))
	for _, item := range doc.PlanCoverage {
		id := strings.TrimSpace(item.PlanID)
		if id == "" {
			return false, "计划贴合度校验失败:plan_coverage 存在空 plan_id"
		}
		if _, ok := leafSet[id]; !ok {
			return false, fmt.Sprintf("计划贴合度校验失败:未知 plan_id %s", id)
		}
		if _, dup := seen[id]; dup {
			return false, fmt.Sprintf("计划贴合度校验失败:重复 plan_id %s", id)
		}
		seen[id] = struct{}{}
		if !item.Passed {
			return false, fmt.Sprintf("计划贴合度校验失败:%s 未通过(passed≠true)", id)
		}
		if strings.TrimSpace(item.Evidence) == "" {
			return false, fmt.Sprintf("计划贴合度校验失败:%s 的 evidence 为空", id)
		}
	}

	var missing []string
	for _, id := range leaves {
		if _, ok := seen[id]; !ok {
			missing = append(missing, id)
		}
	}
	if len(missing) > 0 {
		return false, fmt.Sprintf("计划贴合度校验失败:未覆盖计划叶子 %s", strings.Join(missing, ", "))
	}
	return true, ""
}
