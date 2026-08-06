// Package liveagent is the platform's only direct LLM client. It speaks the
// OpenAI chat/completions shape over plain net/http so any compatible endpoint
// can be pointed at from the settings page.
//
// It exists to answer one narrow question per inbound message — can this be
// answered now, or does it need the project itself — and to return that answer
// as a structured decision rather than prose. Everything that needs real
// project capability still goes to a sandbox agent, which is where MCP,
// multi-turn tool loops and long-running work live.
package liveagent

import (
	"strings"
	"sync/atomic"
	"time"
)

// Endpoint is one immutable snapshot of where and how to call the model.
// Snapshots are swapped wholesale so a settings-page edit can never be observed
// half-applied (e.g. a new base URL with the old key).
type Endpoint struct {
	BaseURL string
	APIKey  string
	Model   string
	Timeout time.Duration
}

// Configured reports whether the snapshot is complete enough to call.
func (e Endpoint) Configured() bool {
	return e.BaseURL != "" && e.APIKey != "" && e.Model != ""
}

// defaultTimeout bounds a call when none is configured. The conversation layer
// is only worth having while it is fast; past this the sandbox is the better
// answer even though it is slower, because it can actually do the work.
const defaultTimeout = 8 * time.Second

// current holds the live Endpoint. A pointer swap keeps reads lock-free on the
// hot path, which runs once per inbound message.
type current struct {
	v atomic.Pointer[Endpoint]
}

func (c *current) load() Endpoint {
	if p := c.v.Load(); p != nil {
		return *p
	}
	return Endpoint{}
}

func (c *current) store(e Endpoint) {
	e.BaseURL = strings.TrimRight(strings.TrimSpace(e.BaseURL), "/")
	e.APIKey = strings.TrimSpace(e.APIKey)
	e.Model = strings.TrimSpace(e.Model)
	if e.Timeout <= 0 {
		e.Timeout = defaultTimeout
	}
	c.v.Store(&e)
}
