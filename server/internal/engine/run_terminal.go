package engine

import (
	"strings"

	"github.com/cocofhu/approving/internal/services"

	"github.com/rs/zerolog/log"
)

// RunTerminalEvent describes a Run that has reached a state it will not leave.
//
// It is deliberately separate from RunNotifyEvent. RunNotify is a project-level
// template push ("a run needs attention, here's a link"); this is the
// conversational counterpart — the answer to a question a specific user asked in
// a specific conversation, and it must read as such. Sharing the path would mean
// sharing the templates, which is the behaviour this replaces.
type RunTerminalEvent struct {
	ProjectID    string
	RunID        string
	WorkflowID   string
	WorkflowName string
	RunTitle     string
	// Status is one of completed / failed / cancelled.
	Status string
	// FailureSummary carries the aggregated reason for a failed run so the
	// observer can explain the outcome without reading raw logs.
	FailureSummary string
	// ResultSummary is a short digest of what the run produced (artifact
	// summary / findings). Empty when nothing readable was left — callers must
	// not invent details, but also must not pretend they are waiting elsewhere.
	ResultSummary string
}

// RunTerminalObserver receives terminal Run transitions. Implementations must
// not block meaningfully; Engine invokes them in a goroutine.
type RunTerminalObserver interface {
	OnRunTerminal(ev RunTerminalEvent)
}

// SetRunTerminalObserver wires the terminal-state hook (nil disables).
func (e *Engine) SetRunTerminalObserver(o RunTerminalObserver) {
	e.runTerminal = o
}

// fireRunTerminal reports a confirmed terminal transition. finish() is the sole
// writer of completed / failed / cancelled, which is why the hook lives there:
// every path that ends a Run passes through it exactly once.
func (e *Engine) fireRunTerminal(runID, status string) {
	observer := e.runTerminal
	if observer == nil {
		return
	}
	status = strings.TrimSpace(status)
	switch status {
	case "completed", "failed", "cancelled":
	default:
		return
	}
	projectID := services.ResolveProjectIDForRun(e.db, runID)
	if projectID == "" {
		return
	}
	ev := RunTerminalEvent{ProjectID: projectID, RunID: runID, Status: status}
	var run modelsRun
	if err := e.db.Table("runs").
		Select("workflow_id", "workflow_name", "title").
		Where("id = ?", runID).Take(&run).Error; err == nil {
		ev.WorkflowID, ev.WorkflowName, ev.RunTitle = run.WorkflowID, run.WorkflowName, run.Title
	}
	if status == "failed" {
		// Reason only. Node ids and log tails are diagnostics for the platform,
		// not something to read out in a chat.
		info := services.NewRunService(e.db).AggregateRunFailure(runID)
		ev.FailureSummary = strings.TrimSpace(info.Reason)
	}
	if status == "completed" {
		// Pull whatever the work layer left as a readable conclusion so IM
		// reflow can say what finished — not only that it finished. Include
		// mr_url when submit_mr left one, so「PR是什么」can be answered later.
		ev.ResultSummary = services.NewArtifactService(e.db).DigestedRunOutcome(runID, 800)
		mrURL := services.NewRunService(e.db).RunVarString(runID, "mr_url")
		ev.ResultSummary = services.AppendRunDeliveryURL(ev.ResultSummary, mrURL)
	}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Error().Str("run_id", ev.RunID).Interface("panic", r).
					Msg("run-terminal: observer panic")
			}
		}()
		observer.OnRunTerminal(ev)
	}()
}

// modelsRun is a narrow projection of the columns fireRunTerminal reads.
type modelsRun struct {
	WorkflowID   string
	WorkflowName string
	Title        string
}
