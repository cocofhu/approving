package engine

import (
	"strings"

	"github.com/cocofhu/approving/internal/models"
	"github.com/cocofhu/approving/internal/services"

	"github.com/rs/zerolog/log"
)

// RunPausedEvent describes a Run that has stopped and will not move again until
// a person acts on it.
//
// It is a third channel on purpose. RunTerminalEvent settles a task and closes
// the conversation on it, which would be a lie about work that is still open.
// RunNotifyEvent is the project-level templated alert with a deep link,
// addressed to whoever watches the project. Neither one answers the person who
// asked for this particular piece of work, in the conversation where they asked
// for it — and that silence is what makes a paused run look like a stuck one.
type RunPausedEvent struct {
	ProjectID string
	RunID     string
	NodeID    string
	NodeLabel string
	Iteration int
	// Ask is what the run stopped to hear, carried over from the node output
	// that accompanied the pause. Empty when the node paused without saying.
	Ask string
}

// RunPausedObserver receives waiting-for-a-human transitions. Implementations
// must not block meaningfully; Engine invokes them in a goroutine.
type RunPausedObserver interface {
	OnRunPaused(ev RunPausedEvent)
}

// SetRunPausedObserver wires the paused-state hook (nil disables).
func (e *Engine) SetRunPausedObserver(o RunPausedObserver) {
	e.runPaused = o
}

// fireRunPaused reports a confirmed pause. It is called from the same branch as
// fireRunNotify — after finish(waiting_human) succeeded and the gate is still
// pending — so the two never disagree about whether the run really stopped.
func (e *Engine) fireRunPaused(c *execCtx, node *models.Node, ask string) {
	observer := e.runPaused
	if observer == nil || c == nil || c.run == nil || node == nil {
		return
	}
	iter := 0
	if c.iter != nil {
		iter = c.iter[node.ID]
	}
	if iter < 1 {
		return
	}
	projectID := services.ResolveProjectIDForRun(e.db, c.run.ID)
	if projectID == "" {
		return
	}
	ev := RunPausedEvent{
		ProjectID: projectID,
		RunID:     c.run.ID,
		NodeID:    node.ID,
		NodeLabel: node.Label,
		Iteration: iter,
		Ask:       strings.TrimSpace(ask),
	}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Error().Str("run_id", ev.RunID).Str("node_id", ev.NodeID).
					Interface("panic", r).Msg("run-paused: observer panic")
			}
		}()
		observer.OnRunPaused(ev)
	}()
}
