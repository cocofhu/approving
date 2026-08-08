package engine

import (
	"time"

	"github.com/cocofhu/approving/internal/models"
	"github.com/cocofhu/approving/internal/services"

	"github.com/rs/zerolog/log"
)

// RunHeartbeatEvent is a Run that has been working for a while without anything
// happening that was worth interrupting anyone about.
//
// It exists because the other three events are all edges — accepted, paused,
// finished — and a task that runs for an hour between two of them looks
// identical to a task that hung. The obvious fix is to have the worker report
// in, and that is the wrong shape: the worker cannot know how long the user has
// been waiting, whether anyone was already told, or whether this conversation
// wants updates at all. It ends up either spamming or silent, and which one
// depends on how the agent felt about its own progress.
//
// So the platform asks instead of being told. Nothing about the worker changes;
// it does not opt in and cannot opt out. This event only carries what the
// engine can see for itself — which node, for how long — and the conversation
// layer adds what it knows about the task before deciding whether to speak.
type RunHeartbeatEvent struct {
	ProjectID string
	RunID     string
	NodeID    string
	NodeLabel string
	// RunningFor is how long the run has been going. The conversation layer
	// turns this into "还在跑，一个多小时了" rather than reporting a timestamp.
	RunningFor time.Duration
}

// RunHeartbeatObserver receives long-running Run ticks. Implementations must
// not block meaningfully; Engine invokes them in a goroutine.
type RunHeartbeatObserver interface {
	OnRunHeartbeat(ev RunHeartbeatEvent)
}

// SetRunHeartbeatObserver wires the long-running hook (nil disables).
func (e *Engine) SetRunHeartbeatObserver(o RunHeartbeatObserver) {
	e.runHeartbeat = o
}

// SweepRunHeartbeats reports every run that has been going longer than minAge.
//
// It reports on every sweep rather than tracking who was told when: the engine
// has no idea who is listening, and per-conversation throttling belongs with
// the task ledger that knows. Deciding here would mean the engine holding a
// second, quietly divergent copy of that state.
func (e *Engine) SweepRunHeartbeats(minAge time.Duration) {
	observer := e.runHeartbeat
	if observer == nil || e.db == nil || minAge <= 0 {
		return
	}
	cutoff := time.Now().Add(-minAge)
	var runs []models.Run
	if err := e.db.Where("status = ? AND started_at < ?", "running", cutoff).
		Find(&runs).Error; err != nil {
		log.Debug().Err(err).Msg("run-heartbeat: long-running scan failed")
		return
	}
	for _, run := range runs {
		projectID := services.ResolveProjectIDForRun(e.db, run.ID)
		if projectID == "" {
			continue
		}
		ev := RunHeartbeatEvent{
			ProjectID: projectID, RunID: run.ID,
			RunningFor: time.Since(run.StartedAt),
		}
		var state models.StateRun
		if err := e.db.Where("run_id = ? AND status = ?", run.ID, "running").
			Order("id desc").First(&state).Error; err == nil {
			ev.NodeID = state.NodeID
			ev.NodeLabel = services.GraphNodeLabel(run.Graph, state.NodeID)
		}
		go func(ev RunHeartbeatEvent) {
			defer func() {
				if r := recover(); r != nil {
					log.Error().Str("run_id", ev.RunID).Interface("panic", r).
						Msg("run-heartbeat: observer panic")
				}
			}()
			observer.OnRunHeartbeat(ev)
		}(ev)
	}
}
