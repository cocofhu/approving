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
	})
	if err != nil {
		log.Warn().Err(err).Str("run", ev.RunID).Str("status", ev.Status).
			Msg("task outcome did not reach its origin conversation")
	}
}
