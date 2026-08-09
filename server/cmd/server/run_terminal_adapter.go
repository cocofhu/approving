package main

import (
	"context"

	"github.com/cocofhu/approving/internal/channels"
	"github.com/cocofhu/approving/internal/engine"

	"github.com/rs/zerolog/log"
)

// runTerminalAdapter brings a finished Run's outcome back to the conversation
// that started it. It is separate from runNotifyEngineAdapter on purpose: that
// one is the project-level "a run needs attention" push with its own policy and
// templates, this one is a reply in an ongoing conversation.
type runTerminalAdapter struct {
	mgr *channels.Manager
}

func (a runTerminalAdapter) OnRunTerminal(ev engine.RunTerminalEvent) {
	if a.mgr == nil {
		return
	}
	err := a.mgr.ReflowTaskOutcome(context.Background(), channels.TaskOutcome{
		ProjectID:     ev.ProjectID,
		RunID:         ev.RunID,
		Status:        ev.Status,
		FailureReason: ev.FailureSummary,
		ResultSummary: ev.ResultSummary,
	})
	if err != nil {
		log.Warn().Err(err).Str("run", ev.RunID).Str("status", ev.Status).
			Msg("task outcome did not reach its origin conversation")
	}
}

// runPausedAdapter tells the conversation that dispatched a task that the task
// is now waiting on them. Same reasoning as above, one step earlier in the
// run's life: a pause is news the person who asked needs, and it is not what
// the project-level notify template is for.
type runPausedAdapter struct {
	mgr *channels.Manager
}

func (a runPausedAdapter) OnRunPaused(ev engine.RunPausedEvent) {
	if a.mgr == nil {
		return
	}
	err := a.mgr.ReflowTaskPaused(context.Background(), channels.TaskPause{
		ProjectID: ev.ProjectID,
		RunID:     ev.RunID,
		NodeID:    ev.NodeID,
		Iteration: ev.Iteration,
		Ask:       ev.Ask,
	})
	if err != nil {
		log.Warn().Err(err).Str("run", ev.RunID).Str("node", ev.NodeID).
			Msg("task pause did not reach its origin conversation")
	}
}

// runHeartbeatAdapter carries the "still going" tick to the conversation layer,
// which decides whether anyone is waiting and whether they were told recently
// enough. The engine deliberately does not know either of those things.
type runHeartbeatAdapter struct {
	mgr *channels.Manager
}

func (a runHeartbeatAdapter) OnRunHeartbeat(ev engine.RunHeartbeatEvent) {
	if a.mgr == nil {
		return
	}
	err := a.mgr.ReportRunHeartbeat(context.Background(), channels.RunHeartbeat{
		ProjectID:  ev.ProjectID,
		RunID:      ev.RunID,
		NodeLabel:  ev.NodeLabel,
		RunningFor: ev.RunningFor,
	})
	if err != nil {
		log.Warn().Err(err).Str("run", ev.RunID).
			Msg("long-running update did not reach its origin conversation")
	}
}
