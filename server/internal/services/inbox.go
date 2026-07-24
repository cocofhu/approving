package services

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/cocofhu/approving/internal/models"
	"github.com/cocofhu/approving/internal/nodereg"
)

var terminalRunStatuses = []string{"completed", "failed", "cancelled"}

// reactAutoEnabled reports whether a react node should self-answer without
// waiting for a human (mirrors engine.autoReactEnabled).
func reactAutoEnabled(node *models.Node, vars map[string]any) bool {
	if node == nil {
		return false
	}
	autoVar := strings.TrimSpace(configStr(node.Config["auto_var"]))
	if autoVar == "" {
		return false
	}
	return varTruthy(vars[autoVar])
}

func configStr(v any) string {
	if v == nil {
		return ""
	}
	return fmt.Sprint(v)
}

func varTruthy(v any) bool {
	switch n := v.(type) {
	case bool:
		return n
	case float64:
		return n != 0
	case int:
		return n != 0
	case string:
		return n != "" && n != "false"
	case nil:
		return false
	default:
		return true
	}
}

func nodeLabel(g models.Graph, nodeID string) string {
	if label := GraphNodeLabel(g, nodeID); label != "" {
		return label
	}
	return nodeID
}

func latestMessageAt(msgs []models.ReactMessage, fallback time.Time) time.Time {
	var latest time.Time
	for _, m := range msgs {
		if t, err := time.Parse(time.RFC3339, m.At); err == nil && t.After(latest) {
			latest = t
		}
	}
	if latest.IsZero() {
		return fallback
	}
	return latest
}

// GateInboxItem is a pending gate in the unified inbox (GET /gates).
type GateInboxItem struct {
	Type              string              `json:"type"`
	RunID             string              `json:"runId"`
	NodeID            string              `json:"nodeId"`
	Iteration         int                 `json:"iteration"`
	WorkflowID        string              `json:"workflowId"`
	WorkflowName      string              `json:"workflowName"`
	RunTitle          string              `json:"runTitle,omitempty"`
	Title             string              `json:"title"`
	BodyMd            string              `json:"bodyMd"`
	Actions           []models.GateAction `json:"actions"`
	Form              []models.GateField  `json:"form"`
	UpstreamNodeID    string              `json:"upstreamNodeId,omitempty"`
	UpstreamIteration int                 `json:"upstreamIteration,omitempty"`
	RequestedAt       time.Time           `json:"requestedAt"`
}

func gateInboxItem(g models.Gate, runTitle string) GateInboxItem {
	return GateInboxItem{
		Type: "gate", RunID: g.RunID, NodeID: g.NodeID, Iteration: g.Iteration,
		WorkflowID: g.WorkflowID, WorkflowName: g.WorkflowName, RunTitle: runTitle,
		Title: g.Title, BodyMd: g.BodyMd, Actions: g.Actions, Form: g.Form,
		UpstreamNodeID: g.UpstreamNodeID, UpstreamIteration: g.UpstreamIteration,
		RequestedAt: g.RequestedAt,
	}
}

// ClarifyInboxItem is a pending react clarification or product review in the
// unified inbox. Type is the inbox channel (always "clarify" for this path);
// Kind is the list-badge semantic: "clarify" (react) or "review" (ReviewCapable).
type ClarifyInboxItem struct {
	Type         string    `json:"type"`
	Kind         string    `json:"kind"` // clarify | review
	RunID        string    `json:"runId"`
	NodeID       string    `json:"nodeId"`
	Iteration    int       `json:"iteration"`
	WorkflowID   string    `json:"workflowId"`
	WorkflowName string    `json:"workflowName"`
	RunTitle     string    `json:"runTitle,omitempty"`
	Label        string    `json:"label"`
	Done         bool      `json:"done"`
	RequestedAt  time.Time `json:"requestedAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

// clarifyInboxKind returns badge semantic for a waiting_human conversation.
// react → clarify; ReviewCapable product nodes → review; default clarify.
func clarifyInboxKind(node *models.Node) string {
	if node != nil && node.Type != "react" && nodereg.ReviewCapable(node.Type) {
		return "review"
	}
	return "clarify"
}

type inboxEntry struct {
	sortAt time.Time
	item   any
}

// AllPendingInboxItems merges unresolved gates and pending (non-auto) react
// clarifications on alive runs, sorted newest-first by sortAt.
func (s *RunService) AllPendingInboxItems() []any {
	items, _ := s.PendingInboxItems("", "", 0, 0)
	return items
}

// PendingInboxItems returns merged inbox items sorted newest-first. When limit > 0
// the result is sliced to [offset:offset+limit]; total is always the full count.
func (s *RunService) PendingInboxItems(wf, projectID string, offset, limit int) ([]any, int) {
	entries := s.pendingInboxEntries(wf, projectID)
	total := len(entries)
	if limit > 0 {
		end := offset + limit
		if offset > total {
			offset = total
		}
		if end > total {
			end = total
		}
		entries = entries[offset:end]
	}
	out := make([]any, len(entries))
	for i, e := range entries {
		out[i] = e.item
	}
	return out, total
}

func (s *RunService) pendingInboxEntries(wf, projectID string) []inboxEntry {
	gates := s.pendingGatesFiltered(wf, projectID)
	clarifies := s.pendingClarificationsFiltered(wf, projectID)

	runIDs := make([]string, 0, len(gates)+len(clarifies))
	seen := map[string]bool{}
	for _, g := range gates {
		if !seen[g.RunID] {
			seen[g.RunID] = true
			runIDs = append(runIDs, g.RunID)
		}
	}
	for _, c := range clarifies {
		if !seen[c.RunID] {
			seen[c.RunID] = true
			runIDs = append(runIDs, c.RunID)
		}
	}
	titles := s.runTitlesByIDs(runIDs)

	entries := make([]inboxEntry, 0, len(gates)+len(clarifies))
	for _, g := range gates {
		entries = append(entries, inboxEntry{sortAt: g.RequestedAt, item: gateInboxItem(g, titles[g.RunID])})
	}
	for _, c := range clarifies {
		if t := titles[c.RunID]; t != "" {
			c.RunTitle = t
		}
		entries = append(entries, inboxEntry{sortAt: c.UpdatedAt, item: c})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].sortAt.After(entries[j].sortAt)
	})
	return entries
}

// runTitlesByIDs returns Run.Title keyed by run id (empty titles omitted).
func (s *RunService) runTitlesByIDs(runIDs []string) map[string]string {
	out := make(map[string]string, len(runIDs))
	if len(runIDs) == 0 {
		return out
	}
	var rows []struct {
		ID    string
		Title string
	}
	s.db.Model(&models.Run{}).Select("id, title").Where("id IN ?", runIDs).Find(&rows)
	for _, r := range rows {
		if t := strings.TrimSpace(r.Title); t != "" {
			out[r.ID] = t
		}
	}
	return out
}

func (s *RunService) pendingGatesFiltered(wf, projectID string) []models.Gate {
	q := s.db.Joins("JOIN runs ON runs.id = gates.run_id").
		Where("gates.resolved = ? AND runs.status NOT IN ?", false, terminalRunStatuses)
	if wf != "" {
		q = q.Where("runs.workflow_id = ?", wf)
	} else if projectID != "" {
		q = q.Where("runs.workflow_id IN (?)", s.db.Model(&models.WorkflowDef{}).Select("id").Where("project_id = ?", projectID))
	}
	var gates []models.Gate
	q.Order("gates.requested_at desc").Find(&gates)
	return gates
}

func (s *RunService) pendingClarificationsFiltered(wf, projectID string) []ClarifyInboxItem {
	clarifies := s.pendingClarifications()
	if wf == "" && projectID == "" {
		return clarifies
	}
	var projectWFs map[string]struct{}
	if wf == "" && projectID != "" {
		var ids []string
		s.db.Model(&models.WorkflowDef{}).Where("project_id = ?", projectID).Pluck("id", &ids)
		projectWFs = make(map[string]struct{}, len(ids))
		for _, id := range ids {
			projectWFs[id] = struct{}{}
		}
	}
	out := make([]ClarifyInboxItem, 0, len(clarifies))
	for _, c := range clarifies {
		if wf != "" {
			if c.WorkflowID == wf {
				out = append(out, c)
			}
			continue
		}
		if _, ok := projectWFs[c.WorkflowID]; ok {
			out = append(out, c)
		}
	}
	return out
}

func (s *RunService) pendingClarifications() []ClarifyInboxItem {
	var convs []models.ReactConversation
	s.db.Joins("JOIN runs ON runs.id = react_conversations.run_id").
		Where("react_conversations.done = ? AND runs.status NOT IN ?", false, terminalRunStatuses).
		Order("react_conversations.id asc").
		Find(&convs)
	if len(convs) == 0 {
		return nil
	}

	runIDs := make([]string, 0, len(convs))
	seenRun := map[string]bool{}
	for _, c := range convs {
		if !seenRun[c.RunID] {
			seenRun[c.RunID] = true
			runIDs = append(runIDs, c.RunID)
		}
	}

	var runs []models.Run
	s.db.Where("id IN ?", runIDs).Find(&runs)
	runByID := make(map[string]models.Run, len(runs))
	for _, r := range runs {
		runByID[r.ID] = r
	}

	varsByRun := s.varsByRun(runIDs)

	seen := map[string]bool{}
	out := make([]ClarifyInboxItem, 0, len(convs))
	for _, conv := range convs {
		key := fmt.Sprintf("%s:%s:%d", conv.RunID, conv.NodeID, conv.Iteration)
		if seen[key] {
			continue
		}
		seen[key] = true

		run, ok := runByID[conv.RunID]
		if !ok {
			continue
		}
		node := run.Graph.FindNode(conv.NodeID)
		if reactAutoEnabled(node, varsByRun[conv.RunID]) {
			continue
		}
		// Only surface clarifications where the node is genuinely waiting for
		// human input — exclude failed sandbox-setup paths (even if a stale
		// conversation row exists from before the fix).
		var sr models.StateRun
		if err := s.db.Where("run_id = ? AND node_id = ? AND iteration = ?", conv.RunID, conv.NodeID, conv.Iteration).
			Order("id desc").First(&sr).Error; err != nil || sr.Status != "waiting_human" {
			continue
		}

		sortAt := latestMessageAt(conv.Messages, run.StartedAt)
		out = append(out, ClarifyInboxItem{
			Type: "clarify", Kind: clarifyInboxKind(node),
			RunID: conv.RunID, NodeID: conv.NodeID, Iteration: conv.Iteration,
			WorkflowID: run.WorkflowID, WorkflowName: run.WorkflowName,
			RunTitle: strings.TrimSpace(run.Title),
			Label:    nodeLabel(run.Graph, conv.NodeID), Done: conv.Done,
			RequestedAt: run.StartedAt, UpdatedAt: sortAt,
		})
	}
	return out
}

func (s *RunService) varsByRun(runIDs []string) map[string]map[string]any {
	out := make(map[string]map[string]any, len(runIDs))
	if len(runIDs) == 0 {
		return out
	}
	var vars []models.RunVariable
	s.db.Where("run_id IN ?", runIDs).Find(&vars)
	for _, v := range vars {
		if out[v.RunID] == nil {
			out[v.RunID] = map[string]any{}
		}
		out[v.RunID][v.Name] = v.Value
	}
	return out
}
