package liveagent

import (
	"sync"
	"time"
)

// Stats is what the conversation layer has actually been doing.
//
// A test button proves a configuration can work; this proves whether it is
// being used. The two are not the same question, and only having the first is
// how a saved, working endpoint went unnoticed for a day while every message
// quietly went to a sandbox instead: from the outside an escalation looks like
// an ordinary answer, only slower. Calls=0 on a configured endpoint is the
// signal that was missing.
type Stats struct {
	Calls  int `json:"calls"`
	Failed int `json:"failed"`
	// AvgLatencyMS averages the successful calls. Failures are excluded because
	// a timeout would otherwise dominate the number and hide how fast the
	// endpoint is when it does answer.
	AvgLatencyMS  int64      `json:"avgLatencyMs"`
	LastLatencyMS int64      `json:"lastLatencyMs"`
	LastSuccessAt *time.Time `json:"lastSuccessAt,omitempty"`
	LastFailureAt *time.Time `json:"lastFailureAt,omitempty"`
	// LastFailure is the same operator-facing explanation the test button
	// gives, so a failure seen in production reads the same as one reproduced
	// by hand.
	LastFailure string `json:"lastFailure,omitempty"`
}

type stats struct {
	mu            sync.Mutex
	calls         int
	failed        int
	successes     int
	latencySumMS  int64
	lastLatencyMS int64
	lastSuccessAt time.Time
	lastFailureAt time.Time
	lastFailure   string
}

func (s *stats) record(latency time.Duration, at time.Time, failure string) {
	ms := latency.Milliseconds()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	s.lastLatencyMS = ms
	if failure != "" {
		s.failed++
		s.lastFailureAt = at
		s.lastFailure = failure
		return
	}
	s.successes++
	s.latencySumMS += ms
	s.lastSuccessAt = at
}

func (s *stats) snapshot() Stats {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := Stats{
		Calls: s.calls, Failed: s.failed,
		LastLatencyMS: s.lastLatencyMS, LastFailure: s.lastFailure,
	}
	if s.successes > 0 {
		out.AvgLatencyMS = s.latencySumMS / int64(s.successes)
	}
	if !s.lastSuccessAt.IsZero() {
		at := s.lastSuccessAt
		out.LastSuccessAt = &at
	}
	if !s.lastFailureAt.IsZero() {
		at := s.lastFailureAt
		out.LastFailureAt = &at
	}
	return out
}

// Stats reports the calls made since this process started. A probe does not
// appear here: it runs on a throwaway client so a manual test is never mistaken
// for traffic.
func (c *Client) Stats() Stats { return c.stats.snapshot() }
