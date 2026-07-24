package mcp

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// --- structured plan (set_plan / get_plan / update_plan_status) ------------

// PlanArtifactName is the reserved artifact holding the run's global, two-level
// plan. It is written by set_plan (plan node), read by get_plan, and its item
// statuses are advanced by update_plan_status (implement node).
const PlanArtifactName = "plan.json"

const (
	planStatusPending    = "pending"
	planStatusInProgress = "in_progress"
	planStatusDone       = "done"
)

type planSub struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Detail string `json:"detail,omitempty"`
	Status string `json:"status"`
}

type planGoal struct {
	ID       string    `json:"id"`
	Title    string    `json:"title"`
	Detail   string    `json:"detail,omitempty"`
	Status   string    `json:"status"`
	Subgoals []planSub `json:"subgoals,omitempty"`
}

type planDoc struct {
	Title string     `json:"title,omitempty"`
	Goals []planGoal `json:"goals"`
}

func validPlanStatus(s string) bool {
	switch s {
	case planStatusPending, planStatusInProgress, planStatusDone:
		return true
	}
	return false
}

func normPlanStatus(s string) string {
	if validPlanStatus(s) {
		return s
	}
	return planStatusPending
}

// parsePlan coerces the loosely-typed set_plan arguments into a normalized
// planDoc: it enforces the two-level limit (a subgoal may not carry its own
// subgoals), requires non-empty titles, and assigns stable ids (g1, g1.2) plus
// an initial pending status to every item.
func parsePlan(args map[string]any) (planDoc, error) {
	raw, _ := json.Marshal(args)
	var in struct {
		Title string `json:"title"`
		Goals []struct {
			Title    string `json:"title"`
			Detail   string `json:"detail"`
			Status   string `json:"status"`
			Subgoals []struct {
				Title    string          `json:"title"`
				Detail   string          `json:"detail"`
				Status   string          `json:"status"`
				Subgoals json.RawMessage `json:"subgoals"`
			} `json:"subgoals"`
		} `json:"goals"`
	}
	if err := json.Unmarshal(raw, &in); err != nil {
		return planDoc{}, fmt.Errorf("解析计划失败: %w", err)
	}
	if len(in.Goals) == 0 {
		return planDoc{}, errors.New("goals 不能为空")
	}
	doc := planDoc{Title: strings.TrimSpace(in.Title)}
	for gi, g := range in.Goals {
		if strings.TrimSpace(g.Title) == "" {
			return planDoc{}, fmt.Errorf("第 %d 个大目标缺少 title", gi+1)
		}
		goal := planGoal{ID: fmt.Sprintf("g%d", gi+1), Title: strings.TrimSpace(g.Title),
			Detail: strings.TrimSpace(g.Detail), Status: normPlanStatus(g.Status)}
		for si, s := range g.Subgoals {
			if len(s.Subgoals) > 0 {
				return planDoc{}, errors.New("计划最多支持两级(大目标→小目标),小目标下不能再有子目标")
			}
			if strings.TrimSpace(s.Title) == "" {
				return planDoc{}, fmt.Errorf("大目标 g%d 的第 %d 个小目标缺少 title", gi+1, si+1)
			}
			goal.Subgoals = append(goal.Subgoals, planSub{ID: fmt.Sprintf("g%d.%d", gi+1, si+1),
				Title: strings.TrimSpace(s.Title), Detail: strings.TrimSpace(s.Detail), Status: normPlanStatus(s.Status)})
		}
		doc.Goals = append(doc.Goals, goal)
	}
	return doc, nil
}

// applyPlanStatus sets the status of the item identified by id and reports
// whether an item matched. id may be a leaf (a subgoal, or a big goal with no
// subgoals) or a big goal that has subgoals. For a parent goal:
//   - status "done" cascades to mark all its subgoals done (so the plan-
//     completion contract, which only inspects leaf items, is satisfied);
//   - status "in_progress"/"pending" sets the parent's own displayed status
//     without touching its subgoals (so a just-started goal shows progress and
//     already-done subgoals are never regressed).
//
// Every other parent goal's status is rolled up from its subgoals.
func applyPlanStatus(doc *planDoc, id, status string) bool {
	found := false
	for gi := range doc.Goals {
		g := &doc.Goals[gi]
		if len(g.Subgoals) == 0 {
			if g.ID == id {
				g.Status = status
				found = true
			}
			continue
		}
		parentHit := g.ID == id
		if parentHit && status == planStatusDone {
			for si := range g.Subgoals {
				g.Subgoals[si].Status = planStatusDone
			}
		}
		for si := range g.Subgoals {
			if g.Subgoals[si].ID == id {
				g.Subgoals[si].Status = status
				found = true
			}
		}
		if parentHit && status != planStatusDone {
			// Keep the manually-set parent status; skip the rollup that would
			// otherwise derive it from (unchanged) subgoals.
			g.Status = status
			found = true
			continue
		}
		if parentHit {
			found = true
		}
		g.Status = rollupStatus(g.Subgoals)
	}
	return found
}

// rollupStatus derives a parent goal's status from its subgoals: done when all
// are done, pending when all are pending, otherwise in_progress.
func rollupStatus(subs []planSub) string {
	if len(subs) == 0 {
		return planStatusPending
	}
	allDone, allPending := true, true
	for _, s := range subs {
		if s.Status != planStatusDone {
			allDone = false
		}
		if s.Status != planStatusPending {
			allPending = false
		}
	}
	switch {
	case allDone:
		return planStatusDone
	case allPending:
		return planStatusPending
	default:
		return planStatusInProgress
	}
}

// planIncomplete returns human-readable descriptions of every leaf item not yet
// done (subgoals, plus big goals that have no subgoals). Empty ⇒ plan complete.
func planIncomplete(doc planDoc) []string {
	var out []string
	for _, g := range doc.Goals {
		if len(g.Subgoals) == 0 {
			if g.Status != planStatusDone {
				out = append(out, fmt.Sprintf("%s %s(%s)", g.ID, g.Title, g.Status))
			}
			continue
		}
		for _, s := range g.Subgoals {
			if s.Status != planStatusDone {
				out = append(out, fmt.Sprintf("%s %s(%s)", s.ID, s.Title, s.Status))
			}
		}
	}
	return out
}

// RenderPlanMarkdown turns a plan.json content string into a human-readable
// GitHub-flavored task list (checkbox per item, with status chips), so the plan
// can be surfaced verbatim in a human_gate body or any markdown consumer. On any
// parse error it returns the raw content unchanged.
func RenderPlanMarkdown(content string) string {
	var doc planDoc
	if err := json.Unmarshal([]byte(content), &doc); err != nil || len(doc.Goals) == 0 {
		return content
	}
	var b strings.Builder
	if doc.Title != "" {
		b.WriteString("### " + doc.Title + "\n\n")
	}
	for _, g := range doc.Goals {
		b.WriteString(planMarkdownLine("", g.ID, g.Title, g.Detail, g.Status))
		for _, s := range g.Subgoals {
			b.WriteString(planMarkdownLine("  ", s.ID, s.Title, s.Detail, s.Status))
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func planMarkdownLine(indent, id, title, detail, status string) string {
	box := " "
	if status == planStatusDone {
		box = "x"
	}
	line := fmt.Sprintf("%s- [%s] `%s` %s", indent, box, id, title)
	switch status {
	case planStatusInProgress:
		line += " _(进行中)_"
	case planStatusPending:
		line += " _(待办)_"
	}
	if detail != "" {
		line += " — " + detail
	}
	return line + "\n"
}
