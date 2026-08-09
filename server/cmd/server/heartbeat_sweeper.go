package main

import (
	"context"
	"time"

	"github.com/cocofhu/approving/internal/channels"
	"github.com/cocofhu/approving/internal/engine"
)

// heartbeatSweepInterval is how often the engine looks for long-running runs.
//
// It is not the reporting interval — that one is configurable and lives with
// the task ledger, which is the only layer that knows who was told what and
// when. This is just how finely the sweep can land on it, so a five-minute tick
// means a thirty-minute setting is honoured to within five minutes. Anything
// finer would be a query on a schedule nobody asked for.
const heartbeatSweepInterval = 5 * time.Minute

// minHeartbeatAge is how long a run must have been going before it is a
// candidate at all. It bounds the scan cheaply, and short runs are covered by
// the events that already exist — a task that finishes in two minutes does not
// need reassuring anyone that it is still alive.
const minHeartbeatAge = 10 * time.Minute

func runHeartbeatSweeper(ctx context.Context, eng *engine.Engine, mgr *channels.Manager) {
	if eng == nil || mgr == nil {
		return
	}
	ticker := time.NewTicker(heartbeatSweepInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			eng.SweepRunHeartbeats(minHeartbeatAge)
		}
	}
}
