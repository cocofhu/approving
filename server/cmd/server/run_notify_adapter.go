package main

import (
	"github.com/cocofhu/approving/internal/engine"
	"github.com/cocofhu/approving/internal/services"
)

// runNotifyEngineAdapter bridges engine.RunNotifier → RunNotifyService.
type runNotifyEngineAdapter struct {
	svc *services.RunNotifyService
}

func (a runNotifyEngineAdapter) NotifyRunLifecycle(ev engine.RunNotifyEvent) {
	if a.svc == nil {
		return
	}
	a.svc.AttemptDeliver(services.RunNotifyEvent{
		ProjectID:    ev.ProjectID,
		RunID:        ev.RunID,
		WorkflowID:   ev.WorkflowID,
		WorkflowName: ev.WorkflowName,
		ProjectName:  ev.ProjectName,
		NodeID:       ev.NodeID,
		NodeLabel:    ev.NodeLabel,
		Iteration:    ev.Iteration,
		Kind:         ev.Kind,
	})
}
