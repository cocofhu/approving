package channels

import (
	"sync/atomic"
	"time"

	"github.com/cocofhu/approving/internal/services"
)

// Defaults match the values that used to be compiled constants. They are the
// floor the manager boots with before settings.Apply pushes DB/config overrides,
// and the fallback if a zero somehow lands in an atomic.
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

func (m *Manager) initLiveLimits() {
	if m == nil {
		return
	}
	m.liveTranscriptWindow.Store(defaultTranscriptWindow)
	m.liveLedgerLimit.Store(defaultLedgerLimit)
	m.liveRecentTerminalHours.Store(defaultRecentTerminalHours)
	m.liveMaxConcurrentWork.Store(defaultMaxConcurrentWork)
	m.liveToolLoopLimit.Store(defaultToolLoopLimit)
	m.liveMaxTokens.Store(defaultLiveMaxTokens)
	if m.bridge != nil {
		m.bridge.transcriptLimit = m.transcriptLimit
	}
}

// SetLiveLimits applies settings-page overrides. Zero / negative values are
// ignored so a partial apply cannot wipe a window back to an unusable size.
func (m *Manager) SetLiveLimits(lim services.LiveLimits) {
	if m == nil {
		return
	}
	setPositive := func(dst *atomic.Int64, v, fallback int) {
		if v > 0 {
			dst.Store(int64(v))
			return
		}
		if dst.Load() == 0 {
			dst.Store(int64(fallback))
		}
	}
	setPositive(&m.liveTranscriptWindow, lim.TranscriptWindow, defaultTranscriptWindow)
	setPositive(&m.liveLedgerLimit, lim.LedgerLimit, defaultLedgerLimit)
	setPositive(&m.liveRecentTerminalHours, lim.RecentTerminalHours, defaultRecentTerminalHours)
	setPositive(&m.liveMaxConcurrentWork, lim.MaxConcurrentWork, defaultMaxConcurrentWork)
	setPositive(&m.liveToolLoopLimit, lim.ToolLoopLimit, defaultToolLoopLimit)
	setPositive(&m.liveMaxTokens, lim.MaxTokens, defaultLiveMaxTokens)
	// Not setPositive: zero is a meaningful value here, not an unset one. A
	// project that does not want unprompted updates sets this to 0, and
	// treating that as "fall back to the default" would ignore them.
	m.SetHeartbeatInterval(lim.RunHeartbeat)
}

func (m *Manager) transcriptLimit() int {
	return liveLimitOr(&m.liveTranscriptWindow, defaultTranscriptWindow)
}

func (m *Manager) ledgerTaskLimit() int {
	return liveLimitOr(&m.liveLedgerLimit, defaultLedgerLimit)
}

func (m *Manager) recentTerminalLookback() time.Duration {
	hours := liveLimitOr(&m.liveRecentTerminalHours, defaultRecentTerminalHours)
	return time.Duration(hours) * time.Hour
}

func (m *Manager) maxConcurrentTasks() int {
	return liveLimitOr(&m.liveMaxConcurrentWork, defaultMaxConcurrentWork)
}

func (m *Manager) toolLoopLimit() int {
	return liveLimitOr(&m.liveToolLoopLimit, defaultToolLoopLimit)
}

func (m *Manager) replyMaxTokens() int {
	return liveLimitOr(&m.liveMaxTokens, defaultLiveMaxTokens)
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
