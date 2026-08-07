package channels

import (
	"strings"
	"time"

	"github.com/google/uuid"
)

// ensureTraceID mints a per-inbound-turn id used to join Live routing, sandbox
// work, MCP calls, synthesis, and outbound delivery. It is stable for the life
// of one user message — including queueing and fallthrough — and is never a
// Run id or the dedupe TurnID.
func ensureTraceID(in *InboundMessage) {
	if in == nil || strings.TrimSpace(in.TraceID) != "" {
		return
	}
	in.TraceID = "tr-" + uuid.NewString()[:12]
}

// TraceSpan is one step on a turn's call chain. Spans are stored on the
// LiveDecisionSample for that turn so a single query reconstructs what happened.
type TraceSpan struct {
	Name       string    `json:"name"`
	Status     string    `json:"status,omitempty"`
	Detail     string    `json:"detail,omitempty"`
	StartedAt  time.Time `json:"startedAt"`
	EndedAt    time.Time `json:"endedAt,omitempty"`
	DurationMs int64     `json:"durationMs,omitempty"`
}

func finishSpan(name, status, detail string, started time.Time) TraceSpan {
	ended := time.Now()
	if started.IsZero() {
		started = ended
	}
	return TraceSpan{
		Name: name, Status: status, Detail: truncateRunes(detail, 500),
		StartedAt: started, EndedAt: ended,
		DurationMs: ended.Sub(started).Milliseconds(),
	}
}
