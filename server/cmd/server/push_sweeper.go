package main

import (
	"context"
	"time"

	"github.com/cocofhu/approving/internal/channels"
)

// pushQueueSweepInterval is deliberately slow. The queue's normal drain paths
// (a cron push, a run notification, the end of a user turn) still do the work
// promptly; this only exists so a message never waits on an event that may
// never come. Slower would leave a user staring at nothing for minutes, faster
// would mean re-checking busy conversations for no reason.
const pushQueueSweepInterval = 30 * time.Second

func runPushQueueSweeper(ctx context.Context, mgr *channels.Manager) {
	if mgr == nil {
		return
	}
	ticker := time.NewTicker(pushQueueSweepInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			mgr.SweepPushQueues()
		}
	}
}
