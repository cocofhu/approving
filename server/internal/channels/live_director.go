package channels

import (
	"strings"
	"sync/atomic"
	"time"

	"github.com/cocofhu/approving/internal/services"
)

// Defaults match the values that used to be compiled constants. They are the
// floor the director boots with before settings.Apply pushes DB/config
// overrides, and the fallback if a zero somehow lands in an atomic.
const (
	defaultTranscriptWindow    = 20
	defaultLedgerLimit         = 5
	defaultRecentTerminalHours = 24
	defaultMaxConcurrentWork   = 3
	defaultToolLoopLimit       = 3
	defaultLiveMaxTokens       = 2048
)

// transcriptWindow is kept as an alias so call sites that are not on a Manager
// (bridge attachment lookup before limits are wired) still have a sensible
// default. Prefer Manager.transcriptLimit() when a Manager is available.
const transcriptWindow = defaultTranscriptWindow

// liveToolLoopLimit / liveMaxTokens stay as package defaults for documentation
// and for tests that never call SetLiveLimits.
const (
	liveToolLoopLimit = defaultToolLoopLimit
	liveMaxTokens     = defaultLiveMaxTokens
)

// liveDirector is the fast model and the shape of what it is allowed to see:
// the handle itself, how much history and ledger go into a call, how many tool
// loops it may take, and which prompt body it speaks from.
//
// These travel together for one reason — an operator changing the fast model's
// behaviour from the settings page changes some mix of them, and every one is
// read while a turn is in flight. Atomics rather than a lock because they are
// written once per settings apply and read on every message.
//
// The zero value is usable: an unset limit reads as its default and an unset
// body as the built-in prompt, so a director that never saw a settings apply
// behaves like one that was applied with nothing in it.
type liveDirector struct {
	model LiveModel

	transcriptWindow    atomic.Int64
	ledgerLimit         atomic.Int64
	recentTerminalHours atomic.Int64
	maxConcurrentWork   atomic.Int64
	toolLoops           atomic.Int64
	maxTokens           atomic.Int64

	// systemBody is the operator's replacement for the routing prompt's body.
	// Empty means the built-in one. The persona is never in here — it is
	// re-attached on every read, so no setting can talk the platform out of
	// how it speaks.
	systemBody atomic.Pointer[string]
}

// setLimits applies settings-page overrides. Zero / negative values are ignored
// so a partial apply cannot wipe a window back to an unusable size.
func (d *liveDirector) setLimits(lim services.LiveLimits) {
	setPositive := func(dst *atomic.Int64, v, fallback int) {
		if v > 0 {
			dst.Store(int64(v))
			return
		}
		if dst.Load() == 0 {
			dst.Store(int64(fallback))
		}
	}
	setPositive(&d.transcriptWindow, lim.TranscriptWindow, defaultTranscriptWindow)
	setPositive(&d.ledgerLimit, lim.LedgerLimit, defaultLedgerLimit)
	setPositive(&d.recentTerminalHours, lim.RecentTerminalHours, defaultRecentTerminalHours)
	setPositive(&d.maxConcurrentWork, lim.MaxConcurrentWork, defaultMaxConcurrentWork)
	setPositive(&d.toolLoops, lim.ToolLoopLimit, defaultToolLoopLimit)
	setPositive(&d.maxTokens, lim.MaxTokens, defaultLiveMaxTokens)
}

// setPromptBody records the operator's routing-prompt body. Only the body: the
// synthesis body belongs to the component that phrases outcomes and temperature
// belongs to the model client, so the composition root fans one settings
// snapshot out to all three rather than making each read the whole thing.
func (d *liveDirector) setPromptBody(body string) {
	trimmed := strings.TrimSpace(body)
	d.systemBody.Store(&trimmed)
}

// systemPrompt is the routing prompt as it will actually be sent: the fixed
// persona, then whichever body is in effect.
func (d *liveDirector) systemPrompt() string {
	if body := d.systemBody.Load(); body != nil && strings.TrimSpace(*body) != "" {
		return ComposeVoicePrompt(*body)
	}
	return liveSystemPrompt
}

func liveLimitOr(v *atomic.Int64, fallback int) int {
	if v == nil {
		return fallback
	}
	if n := int(v.Load()); n > 0 {
		return n
	}
	return fallback
}

// Manager facade. The conversation layer asks the manager for these because
// that is where a turn already is; the answers all come from the director.

// SetLiveModel installs the fast model. nil means every message goes to the
// agent.
func (m *Manager) SetLiveModel(model LiveModel) { m.director.model = model }

// liveModel is the fast model handle, or nil when none is configured.
func (m *Manager) liveModel() LiveModel {
	if m == nil {
		return nil
	}
	return m.director.model
}

// SetLiveLimits applies settings-page overrides for the conversation layer's
// context windows.
func (m *Manager) SetLiveLimits(lim services.LiveLimits) {
	if m == nil {
		return
	}
	m.director.setLimits(lim)
	// Not part of the director's limits: zero is a meaningful value here, not
	// an unset one. A project that does not want unprompted updates sets this
	// to 0, and treating that as "fall back to the default" would ignore them.
	m.SetHeartbeatInterval(lim.RunHeartbeat)
}

// SetLivePrompts applies the settings-page prompt overrides.
func (m *Manager) SetLivePrompts(p services.LivePrompts) {
	if m == nil {
		return
	}
	m.director.setPromptBody(p.SystemBody)
}

func (m *Manager) systemPrompt() string {
	if m == nil {
		return liveSystemPrompt
	}
	return m.director.systemPrompt()
}

func (m *Manager) transcriptLimit() int {
	return liveLimitOr(&m.director.transcriptWindow, defaultTranscriptWindow)
}

func (m *Manager) ledgerTaskLimit() int {
	return liveLimitOr(&m.director.ledgerLimit, defaultLedgerLimit)
}

func (m *Manager) recentTerminalLookback() time.Duration {
	hours := liveLimitOr(&m.director.recentTerminalHours, defaultRecentTerminalHours)
	return time.Duration(hours) * time.Hour
}

func (m *Manager) maxConcurrentTasks() int {
	return liveLimitOr(&m.director.maxConcurrentWork, defaultMaxConcurrentWork)
}

func (m *Manager) toolLoopLimit() int {
	return liveLimitOr(&m.director.toolLoops, defaultToolLoopLimit)
}

func (m *Manager) replyMaxTokens() int {
	return liveLimitOr(&m.director.maxTokens, defaultLiveMaxTokens)
}
