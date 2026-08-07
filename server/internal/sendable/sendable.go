// Package sendable owns the explicit external-delivery contract and policy.
package sendable

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/cocofhu/approving/internal/models"

	"gorm.io/gorm"
)

type Channel string
type Priority string
type Kind string

const (
	ChannelInternal Channel = "internal"
	ChannelQQ       Channel = "qq"

	PriorityLow      Priority = "low"
	PriorityNormal   Priority = "normal"
	PriorityHigh     Priority = "high"
	PriorityCritical Priority = "critical"

	KindUnknown   Kind = "unknown"
	KindAgentRaw  Kind = "agent_raw"
	KindTool      Kind = "tool"
	KindReasoning Kind = "reasoning"
	// KindRunAcceptanceAck confirms that a real Run was accepted. It requires a
	// real RunID and is delivered at most once per run×conversation/user×channel.
	KindRunAcceptanceAck Kind = "run_acceptance_ack"
	// KindTurnProcessingAck is the per-turn "received, working on it" reply. It
	// is scoped to a turn, never to a Run, and must not be treated as a
	// run acceptance ACK.
	KindTurnProcessingAck  Kind = "turn_processing_ack"
	KindQueueAck           Kind = "queue_ack"
	KindProgress           Kind = "progress"
	KindBlocked            Kind = "blocked"
	KindActionRequired     Kind = "action_required"
	KindFinal              Kind = "final"
	KindCron               Kind = "cron"
	KindRunNotify          Kind = "run_notify"
	KindSafetyNotice       Kind = "safety_notice"
	KindCapabilityFallback Kind = "capability_fallback"
)

// requiresRunID marks kinds that are meaningless without a real Run. Other
// Run-related kinds may fall back to a TaskContext scope when no Run id is
// available, but must never invent one.
func (k Kind) requiresRunID() bool {
	return k == KindRunAcceptanceAck
}

// ProgressFields are the only fields that make a progress event substantive.
type ProgressFields struct {
	Stage          string
	Blocked        bool
	ActionRequired bool
	Conclusion     string
}

// DeliveryEnvelope must accompany every production outbound message.
// A zero envelope is internal-only.
//
// RunID is the real Run identifier and must never be synthesized from an
// inbound platform message id. Deliveries that belong to a conversation turn
// rather than a Run carry TaskContext instead, so per-Run merge/dedupe buckets
// stay free of turn traffic.
type DeliveryEnvelope struct {
	Channels       []Channel
	Priority       Priority
	RunID          string
	TaskContext    string
	ProjectID      string
	ConversationID string
	UserID         string
	// TraceID joins this delivery to the inbound turn's LiveDecisionSample.
	TraceID   string
	DedupeKey string
	Reason    string
	Kind      Kind
	Progress  ProgressFields
	Structured bool
}

// Internal returns an explicit non-deliverable envelope.
func Internal(kind Kind, reason string) DeliveryEnvelope {
	return DeliveryEnvelope{Channels: []Channel{ChannelInternal}, Kind: kind, Reason: reason}
}

// AppendSendable explicitly adds an external channel to an envelope.
func AppendSendable(e DeliveryEnvelope, channels ...Channel) DeliveryEnvelope {
	for _, channel := range channels {
		if channel == "" || channel == ChannelInternal || containsChannel(e.Channels, channel) {
			continue
		}
		e.Channels = append(e.Channels, channel)
	}
	return e
}

func (e DeliveryEnvelope) Allows(channel Channel) bool {
	return containsChannel(e.Channels, channel)
}

func containsChannel(channels []Channel, want Channel) bool {
	for _, channel := range channels {
		if channel == want {
			return true
		}
	}
	return false
}

type AuditEntry struct {
	ProjectID string
	RunID     string
	Channel   Channel
	TraceID   string
	DedupeKey string
	Reason    string
	Result    string
	Attempt   int
}

type AuditFunc func(AuditEntry)

type Decision struct {
	Send      bool
	Reason    string
	DedupeKey string
	Attempt   int
}

// Policy is a DB-backed delivery gate. It stores hashes and metadata only.
type Policy struct {
	db          *gorm.DB
	audit       AuditFunc
	now         func() time.Time
	rateWindow  time.Duration
	maxAttempts int
}

func NewPolicy(db *gorm.DB, audit AuditFunc) *Policy {
	return &Policy{
		db: db, audit: audit, now: time.Now,
		rateWindow: 60 * time.Second, maxAttempts: 3,
	}
}

func (p *Policy) SetClock(now func() time.Time) {
	if now != nil {
		p.now = now
	}
}

func (p *Policy) Evaluate(ctx context.Context, e DeliveryEnvelope, channel Channel, content string) (Decision, error) {
	now := p.now()
	content = strings.TrimSpace(content)
	if reason := validate(e, channel, content); reason != "" {
		p.record(e, channel, e.DedupeKey, reason, "suppressed", 0)
		return Decision{Reason: reason}, nil
	}

	contentHash := digest(content)
	key := strings.TrimSpace(e.DedupeKey)
	if key == "" {
		key = stableKey(e, channel, contentHash)
	}
	if p.db == nil {
		return Decision{Send: true, DedupeKey: key, Attempt: 1}, nil
	}

	var receipt models.SendableDeliveryReceipt
	err := p.db.WithContext(ctx).Where("dedupe_key = ?", key).First(&receipt).Error
	if err == nil {
		switch receipt.Status {
		case "sent":
			p.record(e, channel, key, "already_sent", "suppressed", receipt.Attempts)
			return Decision{Reason: "already_sent", DedupeKey: key, Attempt: receipt.Attempts}, nil
		case "pending":
			if now.Sub(receipt.LastAttemptAt) < retryDelay(receipt.Attempts) {
				p.record(e, channel, key, "pending", "suppressed", receipt.Attempts)
				return Decision{Reason: "pending", DedupeKey: key, Attempt: receipt.Attempts}, nil
			}
		case "failed":
			if receipt.Attempts >= p.maxAttempts {
				p.record(e, channel, key, "retry_exhausted", "suppressed", receipt.Attempts)
				return Decision{Reason: "retry_exhausted", DedupeKey: key, Attempt: receipt.Attempts}, nil
			}
			if now.Before(receipt.NextAttemptAt) {
				p.record(e, channel, key, "retry_backoff", "suppressed", receipt.Attempts)
				return Decision{Reason: "retry_backoff", DedupeKey: key, Attempt: receipt.Attempts}, nil
			}
		}
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return Decision{}, err
	}

	if reason := p.bucketSuppression(ctx, e, channel, key, contentHash, now); reason != "" {
		p.record(e, channel, key, reason, "suppressed", 0)
		return Decision{Reason: reason, DedupeKey: key}, nil
	}

	if receipt.ID == 0 {
		receipt = models.SendableDeliveryReceipt{
			DedupeKey: key, ProjectID: e.ProjectID, RunID: e.RunID,
			TaskContext:    e.TaskContext,
			ConversationID: e.ConversationID, Channel: string(channel),
			Kind: string(e.Kind), ContentHash: contentHash,
			ProgressDigest: progressDigest(e), CreatedAt: now,
		}
	}
	receipt.Status = "pending"
	receipt.Result = "pending"
	receipt.ContentHash = contentHash
	receipt.ProgressDigest = progressDigest(e)
	receipt.Attempts++
	receipt.LastAttemptAt = now
	receipt.UpdatedAt = now
	if receipt.ID == 0 {
		if err := p.db.WithContext(ctx).Create(&receipt).Error; err != nil {
			// A concurrent claimant wins; this caller suppresses.
			if isUniqueConflict(err) {
				return Decision{Reason: "concurrent_duplicate", DedupeKey: key}, nil
			}
			return Decision{}, err
		}
	} else if err := p.db.WithContext(ctx).Save(&receipt).Error; err != nil {
		return Decision{}, err
	}
	return Decision{Send: true, DedupeKey: key, Attempt: receipt.Attempts}, nil
}

// GateReason performs the transport-level fail-closed envelope check.
func GateReason(e DeliveryEnvelope, channel Channel, content string) string {
	return validate(e, channel, strings.TrimSpace(content))
}

func validate(e DeliveryEnvelope, channel Channel, content string) string {
	if content == "" {
		return "empty"
	}
	if !e.Allows(channel) {
		return "internal_or_channel_not_allowed"
	}
	if strings.TrimSpace(e.RunID) == "" && strings.TrimSpace(e.TaskContext) == "" {
		return "missing_delivery_scope"
	}
	if e.Kind.requiresRunID() && strings.TrimSpace(e.RunID) == "" {
		return "missing_run_id"
	}
	switch e.Kind {
	case KindUnknown, KindAgentRaw, KindTool, KindReasoning, "":
		return "unsafe_kind"
	case KindFinal:
		if !e.Structured {
			return "raw_assistant_final"
		}
	case KindProgress:
		if strings.TrimSpace(e.Progress.Stage) == "" &&
			!e.Progress.Blocked && !e.Progress.ActionRequired &&
			strings.TrimSpace(e.Progress.Conclusion) == "" {
			return "non_substantive_progress"
		}
	}
	return ""
}

func (p *Policy) bucketSuppression(ctx context.Context, e DeliveryEnvelope, channel Channel, key, hash string, now time.Time) string {
	// The merge bucket is run × task context × conversation × channel, so turn
	// traffic (no RunID) can never merge with, or suppress, Run events.
	base := func() *gorm.DB {
		return p.db.WithContext(ctx).Model(&models.SendableDeliveryReceipt{}).
			Where("run_id = ? AND task_context = ? AND conversation_id = ? AND channel = ? AND status IN ? AND dedupe_key <> ?",
				e.RunID, e.TaskContext, e.ConversationID, string(channel),
				[]string{"pending", "sent"}, key)
	}
	var count int64
	if err := base().Where("content_hash = ?", hash).Count(&count).Error; err == nil && count > 0 {
		return "duplicate_content"
	}
	// Only the Run acceptance ACK is once-per-run×conversation/user×channel.
	// Per-turn processing/queue ACKs are a different class and stay per turn.
	if e.Kind == KindRunAcceptanceAck {
		acked := p.db.WithContext(ctx).Model(&models.SendableDeliveryReceipt{}).
			Where("run_id = ? AND conversation_id = ? AND channel = ? AND kind = ? AND status IN ? AND dedupe_key <> ?",
				e.RunID, e.ConversationID, string(channel), string(KindRunAcceptanceAck),
				[]string{"pending", "sent"}, key)
		if err := acked.Count(&count).Error; err == nil && count > 0 {
			return "run_acceptance_ack_already_sent"
		}
	}
	if e.Kind == KindProgress {
		var last models.SendableDeliveryReceipt
		err := base().Where("kind = ?", string(KindProgress)).Order("last_attempt_at desc").First(&last).Error
		if err == nil && now.Sub(last.LastAttemptAt) < p.rateWindow {
			// Every ordinary progress update shares the same 60-second bucket,
			// including a changed stage/conclusion. Producers keep reporting
			// snapshots, so the first snapshot evaluated after the window is the
			// latest one. Urgent blocked/action_required/final updates use their
			// dedicated kinds and therefore bypass this progress-only limit.
			return "progress_rate_limited_merged"
		}
		// The per-run bucket above says nothing about how many runs are talking.
		// Several tasks running in parallel each stay within their own limit and
		// still bury the conversation, so the conversation has a budget of its
		// own. Blocked / action_required / final are separate kinds and are not
		// counted here — a decision the user has to make must never be dropped
		// because other tasks were chatty.
		if reason := p.conversationQuota(ctx, e, channel, key, now); reason != "" {
			return reason
		}
	}
	return ""
}

// conversationProgressQuota is how many ordinary progress messages one
// conversation may receive per rate window, across all of its Runs.
const conversationProgressQuota = 3

func (p *Policy) conversationQuota(ctx context.Context, e DeliveryEnvelope, channel Channel, key string, now time.Time) string {
	conv := strings.TrimSpace(e.ConversationID)
	if conv == "" || e.Priority == PriorityCritical {
		return ""
	}
	var count int64
	err := p.db.WithContext(ctx).Model(&models.SendableDeliveryReceipt{}).
		Where("conversation_id = ? AND channel = ? AND kind = ? AND status IN ? AND dedupe_key <> ? AND last_attempt_at > ?",
			conv, string(channel), string(KindProgress),
			[]string{"pending", "sent"}, key, now.Add(-p.rateWindow)).
		Count(&count).Error
	if err == nil && count >= conversationProgressQuota {
		return "conversation_rate_limited"
	}
	return ""
}

func progressDigest(e DeliveryEnvelope) string {
	if e.Kind != KindProgress {
		return ""
	}
	raw := strings.Join([]string{
		strings.TrimSpace(e.Progress.Stage),
		fmtBool(e.Progress.Blocked),
		fmtBool(e.Progress.ActionRequired),
		strings.TrimSpace(e.Progress.Conclusion),
	}, "\x1f")
	if strings.TrimSpace(strings.ReplaceAll(raw, "\x1f", "")) == "" {
		return ""
	}
	return digest(raw)
}

func fmtBool(v bool) string {
	if v {
		return "1"
	}
	return "0"
}

// Retry claims the next bounded attempt for a delivery that already failed.
// The caller is responsible for waiting RetryDelay first, so this deliberately
// bypasses the backoff gate that Evaluate applies to fresh callers. Send=false
// means the delivery is finished: already sent by someone else, or exhausted.
func (p *Policy) Retry(ctx context.Context, d Decision, e DeliveryEnvelope, channel Channel) (Decision, error) {
	if strings.TrimSpace(d.DedupeKey) == "" {
		return Decision{Reason: "missing_dedupe_key"}, nil
	}
	now := p.now()
	if p.db == nil {
		if d.Attempt >= p.maxAttempts {
			p.record(e, channel, d.DedupeKey, "retry_exhausted", "suppressed", d.Attempt)
			return Decision{Reason: "retry_exhausted", DedupeKey: d.DedupeKey, Attempt: d.Attempt}, nil
		}
		return Decision{Send: true, DedupeKey: d.DedupeKey, Attempt: d.Attempt + 1}, nil
	}
	var receipt models.SendableDeliveryReceipt
	if err := p.db.WithContext(ctx).Where("dedupe_key = ?", d.DedupeKey).First(&receipt).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Decision{Reason: "receipt_missing", DedupeKey: d.DedupeKey}, nil
		}
		return Decision{}, err
	}
	if receipt.Status == "sent" {
		p.record(e, channel, d.DedupeKey, "already_sent", "suppressed", receipt.Attempts)
		return Decision{Reason: "already_sent", DedupeKey: d.DedupeKey, Attempt: receipt.Attempts}, nil
	}
	if receipt.Attempts >= p.maxAttempts {
		p.record(e, channel, d.DedupeKey, "retry_exhausted", "suppressed", receipt.Attempts)
		return Decision{Reason: "retry_exhausted", DedupeKey: d.DedupeKey, Attempt: receipt.Attempts}, nil
	}
	receipt.Status = "pending"
	receipt.Result = "pending"
	receipt.Attempts++
	receipt.LastAttemptAt = now
	receipt.UpdatedAt = now
	if err := p.db.WithContext(ctx).Save(&receipt).Error; err != nil {
		return Decision{}, err
	}
	return Decision{Send: true, DedupeKey: d.DedupeKey, Attempt: receipt.Attempts}, nil
}

// MaxAttempts is the bounded delivery attempt ceiling per dedupe key.
func (p *Policy) MaxAttempts() int { return p.maxAttempts }

func (p *Policy) MarkSent(ctx context.Context, d Decision, e DeliveryEnvelope, channel Channel) error {
	if !d.Send {
		return nil
	}
	now := p.now()
	if p.db != nil {
		if err := p.db.WithContext(ctx).Model(&models.SendableDeliveryReceipt{}).
			Where("dedupe_key = ?", d.DedupeKey).
			Updates(map[string]any{"status": "sent", "result": "sent", "updated_at": now}).Error; err != nil {
			return err
		}
	}
	p.record(e, channel, d.DedupeKey, e.Reason, "sent", d.Attempt)
	return nil
}

func (p *Policy) MarkFailed(ctx context.Context, d Decision, e DeliveryEnvelope, channel Channel, sendErr error) error {
	if !d.Send {
		return nil
	}
	now := p.now()
	if p.db != nil {
		if err := p.db.WithContext(ctx).Model(&models.SendableDeliveryReceipt{}).
			Where("dedupe_key = ?", d.DedupeKey).
			Updates(map[string]any{
				"status": "failed", "result": "failed",
				"next_attempt_at": now.Add(retryDelay(d.Attempt)), "updated_at": now,
			}).Error; err != nil {
			return err
		}
	}
	reason := e.Reason
	if sendErr != nil {
		reason = "transport_failed"
	}
	p.record(e, channel, d.DedupeKey, reason, "failed", d.Attempt)
	return nil
}

func (p *Policy) record(e DeliveryEnvelope, channel Channel, key, reason, result string, attempt int) {
	if p.audit != nil {
		p.audit(AuditEntry{
			ProjectID: e.ProjectID, RunID: e.RunID, Channel: channel,
			TraceID: e.TraceID, DedupeKey: key, Reason: reason, Result: result, Attempt: attempt,
		})
	}
}

func retryDelay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	d := time.Duration(attempt*attempt) * time.Second
	if d > 30*time.Second {
		return 30 * time.Second
	}
	return d
}

// RetryDelay returns the bounded backoff used by delivery callers.
func RetryDelay(attempt int) time.Duration { return retryDelay(attempt) }

func stableKey(e DeliveryEnvelope, channel Channel, contentHash string) string {
	raw := strings.Join([]string{
		e.RunID, e.TaskContext, e.ConversationID, e.UserID,
		string(channel), string(e.Kind), contentHash,
	}, "\x1f")
	return "snd-" + digest(raw)[:32]
}

func digest(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func isUniqueConflict(err error) bool {
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "unique") || strings.Contains(s, "duplicate")
}
