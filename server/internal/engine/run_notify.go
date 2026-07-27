package engine

import (
	"strings"

	"github.com/cocofhu/approving/internal/models"

	"github.com/rs/zerolog/log"
)

// RunNotifyEvent carries confirmed waiting_human / failed context for an
// optional async IM side-effect. Engine never blocks on handlers.
type RunNotifyEvent struct {
	ProjectID    string
	RunID        string
	WorkflowID   string
	WorkflowName string
	ProjectName  string
	NodeID       string
	NodeLabel    string
	Iteration    int
	Kind         string // waiting_human | failed
}

// RunNotifier is an optional async observer for Run lifecycle transitions.
// Implementations must not block meaningfully; Engine invokes them in a goroutine.
type RunNotifier interface {
	NotifyRunLifecycle(ev RunNotifyEvent)
}

// SetRunNotifier wires the Run→IM side-effect hook (nil disables).
func (e *Engine) SetRunNotifier(n RunNotifier) {
	e.runNotify = n
}

// fireRunNotify schedules a non-blocking notify after a confirmed lifecycle
// transition with node context. No-op when unset or kind/node invalid.
func (e *Engine) fireRunNotify(c *execCtx, node *models.Node, kind string) {
	if e.runNotify == nil || c == nil || node == nil {
		return
	}
	kind = strings.TrimSpace(kind)
	if kind != models.NotifyKindWaitingHuman && kind != models.NotifyKindFailed {
		return
	}
	iter := 0
	if c.iter != nil {
		iter = c.iter[node.ID]
	}
	if iter < 1 {
		return
	}
	var wf models.WorkflowDef
	if err := e.db.Select("project_id", "name").Where("id = ?", c.run.WorkflowID).First(&wf).Error; err != nil {
		log.Warn().Err(err).Str("run_id", c.run.ID).Str("workflow", c.run.WorkflowID).
			Msg("run-notify: resolve workflow failed")
		return
	}
	projectID := strings.TrimSpace(wf.ProjectID)
	if projectID == "" {
		log.Warn().Str("run_id", c.run.ID).Str("workflow", c.run.WorkflowID).
			Msg("run-notify: workflow has empty project_id")
		return
	}
	projectName := ""
	var project models.Project
	if err := e.db.Select("name").Where("id = ?", projectID).First(&project).Error; err == nil {
		projectName = project.Name
	}
	wfName := wf.Name
	if wfName == "" {
		wfName = c.run.WorkflowName
	}
	ev := RunNotifyEvent{
		ProjectID:    projectID,
		RunID:        c.run.ID,
		WorkflowID:   c.run.WorkflowID,
		WorkflowName: wfName,
		ProjectName:  projectName,
		NodeID:       node.ID,
		NodeLabel:    node.Label,
		Iteration:    iter,
		Kind:         kind,
	}
	n := e.runNotify
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Error().Str("run_id", ev.RunID).Str("node_id", ev.NodeID).
					Interface("panic", r).Msg("run-notify: notify panic")
			}
		}()
		n.NotifyRunLifecycle(ev)
	}()
}
