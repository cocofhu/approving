package channels

import (
	"testing"
	"time"

	"github.com/cocofhu/approving/internal/services"
)

func TestSetLiveLimitsHotUpdatesWindows(t *testing.T) {
	m := NewManager(nil, nil, nil)
	if m.transcriptLimit() != 20 || m.replyMaxTokens() != 2048 {
		t.Fatalf("defaults: transcript=%d tokens=%d", m.transcriptLimit(), m.replyMaxTokens())
	}
	m.SetLiveLimits(services.LiveLimits{
		TranscriptWindow: 40, LedgerLimit: 9, RecentTerminalHours: 6,
		MaxConcurrentWork: 7, ToolLoopLimit: 5, MaxTokens: 1024,
	})
	if m.transcriptLimit() != 40 || m.ledgerTaskLimit() != 9 {
		t.Fatalf("limits not applied: transcript=%d ledger=%d", m.transcriptLimit(), m.ledgerTaskLimit())
	}
	if m.recentTerminalLookback() != 6*time.Hour {
		t.Fatalf("lookback = %v", m.recentTerminalLookback())
	}
	if m.maxConcurrentTasks() != 7 || m.toolLoopLimit() != 5 || m.replyMaxTokens() != 1024 {
		t.Fatalf("work/tool/tokens = %d/%d/%d", m.maxConcurrentTasks(), m.toolLoopLimit(), m.replyMaxTokens())
	}
	// Zeros must not wipe a previously applied value.
	m.SetLiveLimits(services.LiveLimits{})
	if m.transcriptLimit() != 40 {
		t.Fatalf("zero apply wiped transcript to %d", m.transcriptLimit())
	}
}

func TestBridgeFollowsManagerTranscriptWindow(t *testing.T) {
	b := &ChannelBridge{}
	m := NewManager(b, nil, nil)
	if b.windowLimit() != 20 {
		t.Fatalf("bridge default window = %d", b.windowLimit())
	}
	m.SetLiveLimits(services.LiveLimits{TranscriptWindow: 32})
	if b.windowLimit() != 32 {
		t.Fatalf("bridge did not follow manager: %d", b.windowLimit())
	}
}
