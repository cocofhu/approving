package models

import "time"

// Notify event kinds. waiting_human and failed are on by default; completed is
// opt-in (DefaultEvents stays [waiting_human, failed] so existing projects do
// not start notifying on success).
const (
	NotifyKindWaitingHuman = "waiting_human"
	NotifyKindFailed       = "failed"
	NotifyKindCompleted    = "completed"
)

// Completed notify receipt / deep-link fallbacks when a run has no output node.
const (
	NotifyCompletedSentinelNodeID = "_run"
	NotifyCompletedFallbackLabel  = "输出"
)

// Workflow notify override modes.
const (
	NotifyModeOff     = "off"
	NotifyModeInherit = "inherit"
	NotifyModeCustom  = "custom"
)

// ProjectNotifyPolicy is the project-level Run→IM notification default.
// Enabled=nil means default ON (hard-close only when explicitly false).
// DefaultEvents=nil means the product default [waiting_human, failed]; an
// explicit empty slice means "no default events".
// ChannelIDs is the explicit 0~N fan-out target list (may include primary).
// Empty / nil ChannelIDs means do not deliver project notify to any channel
// (independent of per-channel cron deliver targets).
// WaitingHumanTemplate / FailedTemplate / CompletedTemplate are optional full
// message bodies; trim-empty means fall back to FormatRunNotifyMessage.
type ProjectNotifyPolicy struct {
	Enabled              *bool    `json:"enabled"`
	DefaultEvents        []string `json:"defaultEvents"`
	ChannelIDs           []string `json:"channelIds,omitempty"`
	WaitingHumanTemplate string   `json:"waitingHumanTemplate,omitempty"`
	FailedTemplate       string   `json:"failedTemplate,omitempty"`
	CompletedTemplate    string   `json:"completedTemplate,omitempty"`
}

// WorkflowNotifyPolicy is the per-workflow override. Empty Mode ≡ inherit.
type WorkflowNotifyPolicy struct {
	Mode   string   `json:"mode"`
	Events []string `json:"events,omitempty"`
}

// DefaultProjectNotifyPolicy returns the product default used for new projects
// and for legacy rows whose NotifyPolicy JSON is zero/unset.
func DefaultProjectNotifyPolicy() ProjectNotifyPolicy {
	on := true
	return ProjectNotifyPolicy{
		Enabled:       &on,
		DefaultEvents: []string{NotifyKindWaitingHuman, NotifyKindFailed},
	}
}

// IsEnabled reports whether the project kill-switch allows delivery.
// Missing Enabled (nil) defaults to true so upgrades stay opt-out, not silent.
func (p ProjectNotifyPolicy) IsEnabled() bool {
	if p.Enabled == nil {
		return true
	}
	return *p.Enabled
}

// EffectiveDefaultEvents returns the project default event set.
// nil DefaultEvents → product default; non-nil (incl. empty) is respected.
func (p ProjectNotifyPolicy) EffectiveDefaultEvents() []string {
	if p.DefaultEvents == nil {
		return []string{NotifyKindWaitingHuman, NotifyKindFailed}
	}
	return append([]string(nil), p.DefaultEvents...)
}

// EffectiveMode normalizes workflow mode; empty → inherit.
func (w WorkflowNotifyPolicy) EffectiveMode() string {
	switch w.Mode {
	case NotifyModeOff, NotifyModeInherit, NotifyModeCustom:
		return w.Mode
	default:
		return NotifyModeInherit
	}
}

// NotifyDeliveryReceipt is the at-most-once claim key for Run IM pushes.
// Insert success means the (run, node, iteration, kind) tuple is consumed —
// including no-op (no channel) and send failure (P0 does not retry).
type NotifyDeliveryReceipt struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	RunID     string    `gorm:"size:64;uniqueIndex:idx_notify_receipt" json:"runId"`
	NodeID    string    `gorm:"size:128;uniqueIndex:idx_notify_receipt" json:"nodeId"`
	Iteration int       `gorm:"uniqueIndex:idx_notify_receipt" json:"iteration"`
	Kind      string    `gorm:"size:32;uniqueIndex:idx_notify_receipt" json:"kind"`
	CreatedAt time.Time `json:"createdAt"`
}
