package channels

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/cocofhu/approving/internal/models"
	"github.com/cocofhu/approving/internal/services"
	"gorm.io/gorm"
)

type DeliveryClass string

const (
	DeliveryInternal DeliveryClass = "internal"
	DeliverySendable DeliveryClass = "sendable"
)

type DeliveryPriority string

const (
	PriorityOrdinary  DeliveryPriority = "ordinary"
	PriorityImmediate DeliveryPriority = "immediate"
)

type DeliveryReason string

const (
	ReasonTurnProcessingACK DeliveryReason = "turn_processing_ack"
	ReasonRunAcceptanceACK  DeliveryReason = "run_acceptance_ack"
	ReasonProgress          DeliveryReason = "progress"
	ReasonBlocked           DeliveryReason = "blocked"
	ReasonActionRequired    DeliveryReason = "action_required"
	ReasonFinal             DeliveryReason = "final"
	ReasonFailure           DeliveryReason = "failure"
	ReasonQueue             DeliveryReason = "queue"
	ReasonCron              DeliveryReason = "cron"
	ReasonRunNotify         DeliveryReason = "run_notify"
)

const (
	DeliveryTypeStage             = "stage"
	DeliveryTypeBlocked           = "blocked"
	DeliveryTypeActionRequired    = "action_required"
	DeliveryTypeStructuredSummary = "structured_summary"
)

// RunTaskContext carries only routing identity. It must never contain prompt,
// tool output, or chain-of-thought.
type RunTaskContext struct {
	RunID      string `json:"runId,omitempty"`
	TaskID     string `json:"taskId,omitempty"`
	ShortTitle string `json:"shortTitle,omitempty"`
	ProjectID  string `json:"projectId,omitempty"`
	UserID     string `json:"userId,omitempty"`
}

// Envelope is the first-class Work→Reply delivery decision. Its zero value is
// internal. Only AppendSendable can attach the unexported authorization bit.
type Envelope struct {
	Delivery  DeliveryClass    `json:"delivery"`
	Channels  []string         `json:"channels,omitempty"`
	Priority  DeliveryPriority `json:"priority,omitempty"`
	Context   RunTaskContext   `json:"context,omitempty"`
	DedupeKey string           `json:"dedupeKey,omitempty"`
	Reason    DeliveryReason   `json:"reason,omitempty"`
	Type      string           `json:"type,omitempty"`

	authorized bool
}

// InternalEnvelope is the safe default for unknown/raw agent, tool, and
// reasoning events.
func InternalEnvelope(typ string) Envelope {
	return Envelope{Delivery: DeliveryInternal, Type: strings.TrimSpace(typ)}
}

// deliveryProgressState tracks the rate-limit window per run+conversation.
// lastSent only advances after the adapter accepted the message; reserved
// holds the candidate window start while a send is in flight.
type deliveryProgressState struct {
	lastSent time.Time
	reserved time.Time
	inflight bool
	latest   string
}

type pendingProgressDelivery struct {
	rc    *runningChannel
	out   OutboundMessage
	env   Envelope
	timer *time.Timer
}

type deliveryPolicy struct {
	mu       sync.Mutex
	runACK   map[string]string
	progress map[string]deliveryProgressState
	now      func() time.Time
}

func newDeliveryPolicy() *deliveryPolicy {
	return &deliveryPolicy{
		runACK: map[string]string{}, progress: map[string]deliveryProgressState{}, now: time.Now,
	}
}

func (p *deliveryPolicy) allow(env Envelope, out OutboundMessage) (bool, string) {
	if env.Delivery != DeliverySendable || !env.authorized {
		return false, "not_explicitly_sendable"
	}
	if len(env.Channels) == 0 {
		return false, "no_channel"
	}
	if strings.TrimSpace(out.ConversationID) == "" {
		return false, "no_conversation"
	}
	switch env.Reason {
	case ReasonTurnProcessingACK, ReasonQueue, ReasonFailure, ReasonCron, ReasonRunNotify:
		return true, "allowed"
	case ReasonRunAcceptanceACK:
		if env.Priority != PriorityImmediate {
			return false, "run_ack_not_immediate"
		}
		if strings.TrimSpace(env.Context.RunID) == "" {
			return false, "run_ack_missing_run"
		}
		key := runACKKey(env, out)
		p.mu.Lock()
		defer p.mu.Unlock()
		if _, exists := p.runACK[key]; exists {
			return false, "run_ack_duplicate"
		}
		p.runACK[key] = models.DeliveryReceiptPending
		return true, "allowed"
	case ReasonProgress:
		if env.Type != DeliveryTypeStage {
			return false, "progress_not_substantive_stage"
		}
		return p.allowProgress(env, out)
	case ReasonBlocked:
		if strings.TrimSpace(env.Context.RunID) == "" {
			return false, "progress_missing_run"
		}
		if env.Priority != PriorityImmediate || env.Type != DeliveryTypeBlocked {
			return false, "blocked_not_immediate"
		}
		return true, "allowed"
	case ReasonActionRequired:
		if strings.TrimSpace(env.Context.RunID) == "" {
			return false, "progress_missing_run"
		}
		if env.Priority != PriorityImmediate || env.Type != DeliveryTypeActionRequired {
			return false, "action_required_not_immediate"
		}
		return true, "allowed"
	case ReasonFinal:
		if strings.TrimSpace(env.Context.RunID) == "" || env.Type != DeliveryTypeStructuredSummary || env.Priority != PriorityImmediate {
			return false, "final_not_structured"
		}
		return true, "allowed"
	default:
		return false, "unknown_reason"
	}
}

func runACKKey(env Envelope, out OutboundMessage) string {
	return strings.Join([]string{
		env.Context.RunID, env.Context.UserID, firstString(env.Channels),
		string(out.Scene), out.ConversationID,
	}, "|")
}

// finishSend releases the reservations taken by allow once the adapter cycle
// for this envelope has terminated. A failed cycle must leave no state that
// could permanently suppress a later attempt.
func (p *deliveryPolicy) finishSend(env Envelope, out OutboundMessage, sent bool) {
	p.finishRunACK(env, out, sent)
	p.finishProgress(env, out, sent)
}

func (p *deliveryPolicy) finishRunACK(env Envelope, out OutboundMessage, sent bool) {
	if env.Reason != ReasonRunAcceptanceACK {
		return
	}
	key := runACKKey(env, out)
	p.mu.Lock()
	defer p.mu.Unlock()
	if sent {
		p.runACK[key] = models.DeliveryReceiptSent
	} else {
		delete(p.runACK, key)
	}
}

func (p *deliveryPolicy) allowProgress(env Envelope, out OutboundMessage) (bool, string) {
	runID := strings.TrimSpace(env.Context.RunID)
	if runID == "" {
		return false, "progress_missing_run"
	}
	now := p.now()
	key := progressDeliveryKey(env, out)
	p.mu.Lock()
	defer p.mu.Unlock()
	state := p.progress[key]
	state.latest = out.Text
	if state.inflight {
		p.progress[key] = state
		return false, "progress_rate_limited_latest_merged"
	}
	if !state.lastSent.IsZero() && now.Sub(state.lastSent) < 60*time.Second {
		p.progress[key] = state
		return false, "progress_rate_limited_latest_merged"
	}
	state.reserved, state.inflight = now, true
	p.progress[key] = state
	return true, "allowed"
}

// finishProgress commits the reserved rate-limit window only on a successful
// adapter send; a failed cycle rolls it back so the merged latest text can be
// delivered immediately by the next attempt.
func (p *deliveryPolicy) finishProgress(env Envelope, out OutboundMessage, sent bool) {
	if env.Reason != ReasonProgress {
		return
	}
	key := progressDeliveryKey(env, out)
	p.mu.Lock()
	defer p.mu.Unlock()
	state, ok := p.progress[key]
	if !ok || !state.inflight {
		return
	}
	if sent {
		state.lastSent = state.reserved
	}
	state.reserved, state.inflight = time.Time{}, false
	p.progress[key] = state
}

// mergedLatest returns the newest progress text observed for this run while a
// send was rate-limited or failing.
func (p *deliveryPolicy) mergedLatest(env Envelope, out OutboundMessage) string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.progress[progressDeliveryKey(env, out)].latest
}

func progressDeliveryKey(env Envelope, out OutboundMessage) string {
	return strings.Join([]string{env.Context.RunID, firstString(env.Channels), string(out.Scene), out.ConversationID}, "|")
}

func (p *deliveryPolicy) progressDelay(env Envelope, out OutboundMessage) time.Duration {
	p.mu.Lock()
	defer p.mu.Unlock()
	state := p.progress[progressDeliveryKey(env, out)]
	delay := state.lastSent.Add(60 * time.Second).Sub(p.now())
	if delay < 0 {
		return 0
	}
	return delay
}

var ErrDeliverySuppressed = errors.New("delivery suppressed by policy")

const (
	maxDeliveryAttempts = 3
	// deliveryPendingLease bounds how long a claimed (pending) receipt blocks
	// other attempts. A process that crashes mid-send leaves a pending row; the
	// lease lets the next attempt reclaim it instead of suppressing forever.
	deliveryPendingLease = 2 * time.Minute
)

// SetDeliveryPersistence enables persistent receipts and content-free delivery
// audits. It is optional for compatibility with embedded/test managers.
func (m *Manager) SetDeliveryPersistence(db *gorm.DB, audit *services.ProjectAuditService) {
	m.deliveryDB, m.deliveryAudit = db, audit
}

// AppendInternal records an internal event without allowing transport egress.
func (m *Manager) AppendInternal(env Envelope) {
	env.Delivery = DeliveryInternal
	m.auditDelivery(env, "suppressed", "internal", 0)
}

// AppendSendable is the only application authorization path to Adapter.Send.
// Classification/markers alone cannot set the private authorization bit.
func (m *Manager) AppendSendable(ctx context.Context, rc *runningChannel, out OutboundMessage, env Envelope) error {
	env.Delivery = DeliverySendable
	env.authorized = true
	if len(env.Channels) == 0 && rc != nil {
		env.Channels = []string{rc.cfg.Type}
	}
	return m.sendEnvelope(ctx, rc, out, env)
}

func (m *Manager) sendEnvelope(ctx context.Context, rc *runningChannel, out OutboundMessage, env Envelope) error {
	if rc == nil || rc.adapter == nil {
		return fmt.Errorf("channel adapter unavailable")
	}
	allowed, result := m.deliveryPolicy.allow(env, out)
	if !allowed {
		m.auditDelivery(env, "suppressed", result, 0)
		if result == "progress_rate_limited_latest_merged" {
			m.scheduleLatestProgress(rc, out, env)
		}
		return ErrDeliverySuppressed
	}
	if env.Reason == ReasonProgress {
		m.cancelPendingProgress(env, out)
	}
	m.reopenExhaustedReceipt(env)
	localAttempt := 0
	for {
		localAttempt++
		attempt, claimed, result, err := m.claimDelivery(env)
		if attempt < localAttempt {
			attempt = localAttempt
		}
		if err != nil {
			m.auditDelivery(env, "suppressed", "receipt_error", attempt)
			m.deliveryPolicy.finishSend(env, out, false)
			return err
		}
		if !claimed {
			m.auditDelivery(env, "suppressed", result, attempt)
			m.deliveryPolicy.finishSend(env, out, result == "already_sent")
			return ErrDeliverySuppressed
		}
		err = rc.adapter.Send(ctx, out)
		if err == nil {
			m.finishDelivery(env, attempt, nil)
			m.auditDelivery(env, "sent", "sent", attempt)
			m.bindOutboundMessage(env, out)
			m.deliveryPolicy.finishSend(env, out, true)
			return nil
		}
		m.finishDelivery(env, attempt, err)
		m.auditDelivery(env, "sent", "failed", attempt)
		if attempt >= maxDeliveryAttempts {
			m.deliveryPolicy.finishSend(env, out, false)
			m.rescheduleMergedProgress(rc, out, env)
			return err
		}
		// The persisted failed state is reclaimed for the next bounded attempt.
		// No content is retained in the receipt or audit.
	}
}

// reopenExhaustedReceipt clears the bounded attempt counter so a later call can
// start a fresh cycle. Exhausted pending rows are only reopened once their
// lease has expired, so a live concurrent sender is never displaced.
func (m *Manager) reopenExhaustedReceipt(env Envelope) {
	if m.deliveryDB == nil || strings.TrimSpace(env.DedupeKey) == "" {
		return
	}
	staleBefore := time.Now().Add(-deliveryPendingLease)
	_ = m.deliveryDB.Model(&models.DeliveryReceipt{}).
		Where("dedupe_key = ? AND attempts >= ?", env.DedupeKey, maxDeliveryAttempts).
		Where("status = ? OR (status = ? AND (last_tried_at IS NULL OR last_tried_at <= ?))",
			models.DeliveryReceiptFailed, models.DeliveryReceiptPending, staleBefore).
		Updates(map[string]any{"attempts": 0, "last_tried_at": nil}).Error
}

// rescheduleMergedProgress re-arms the merged latest progress after a failed
// bounded cycle so the newest state is not lost with the rolled-back window.
func (m *Manager) rescheduleMergedProgress(rc *runningChannel, out OutboundMessage, env Envelope) {
	if env.Reason != ReasonProgress {
		return
	}
	latest := m.deliveryPolicy.mergedLatest(env, out)
	if strings.TrimSpace(latest) == "" || latest == out.Text {
		return
	}
	merged := out
	merged.Text = latest
	m.scheduleLatestProgress(rc, merged, env)
}

func (m *Manager) bindOutboundMessage(env Envelope, out OutboundMessage) {
	if m.taskContext == nil || strings.TrimSpace(out.MessageID) == "" ||
		strings.TrimSpace(env.Context.RunID) == "" || strings.TrimSpace(env.Context.ProjectID) == "" {
		return
	}
	_ = m.taskContext.BindExternalMessage(models.MessageBinding{
		ProjectID: env.Context.ProjectID, Channel: firstString(env.Channels),
		ConversationID: out.ConversationID, MessageID: out.MessageID,
		UserID: env.Context.UserID, RunID: env.Context.RunID,
		Action: string(env.Reason), Direction: "outbound",
	})
}

func (m *Manager) cancelPendingProgress(env Envelope, out OutboundMessage) {
	key := progressDeliveryKey(env, out)
	m.progressMu.Lock()
	if pending := m.pendingProgress[key]; pending != nil {
		if pending.timer != nil {
			pending.timer.Stop()
		}
		delete(m.pendingProgress, key)
	}
	m.progressMu.Unlock()
}

func (m *Manager) scheduleLatestProgress(rc *runningChannel, out OutboundMessage, env Envelope) {
	key := progressDeliveryKey(env, out)
	m.progressMu.Lock()
	if current := m.pendingProgress[key]; current != nil {
		current.rc, current.out, current.env = rc, out, env
		m.progressMu.Unlock()
		return
	}
	pending := &pendingProgressDelivery{rc: rc, out: out, env: env}
	m.pendingProgress[key] = pending
	delay := m.deliveryPolicy.progressDelay(env, out)
	pending.timer = time.AfterFunc(delay, func() {
		m.progressMu.Lock()
		latest := m.pendingProgress[key]
		delete(m.pendingProgress, key)
		m.progressMu.Unlock()
		if latest == nil {
			return
		}
		ctx, cancel := context.WithTimeout(m.baseCtx, 60*time.Second)
		defer cancel()
		_ = m.AppendSendable(ctx, latest.rc, latest.out, latest.env)
	})
	m.progressMu.Unlock()
}

func (m *Manager) claimDelivery(env Envelope) (attempt int, claimed bool, result string, err error) {
	key := strings.TrimSpace(env.DedupeKey)
	if m.deliveryDB == nil || key == "" {
		return 1, true, "no_receipt", nil
	}
	now := time.Now()
	row := models.DeliveryReceipt{DedupeKey: key, Status: models.DeliveryReceiptPending, CreatedAt: now, UpdatedAt: now}
	if createErr := m.deliveryDB.Create(&row).Error; createErr != nil && !isDeliveryUnique(createErr) {
		return 0, false, "", createErr
	}
	if row.ID == 0 {
		if err = m.deliveryDB.Where("dedupe_key = ?", key).First(&row).Error; err != nil {
			return 0, false, "", err
		}
	}
	if row.Status == models.DeliveryReceiptSent {
		return row.Attempts, false, "already_sent", nil
	}
	if row.Attempts >= maxDeliveryAttempts {
		return row.Attempts, false, "retry_exhausted", nil
	}
	// A pending row is a lease held by whoever claimed it. Only an expired
	// lease (typically a crashed sender) may be reclaimed.
	staleBefore := now.Add(-deliveryPendingLease)
	if row.Status == models.DeliveryReceiptPending && row.LastTriedAt != nil && row.LastTriedAt.After(staleBefore) {
		return row.Attempts, false, "pending_lease_active", nil
	}
	attempt = row.Attempts + 1
	res := m.deliveryDB.Model(&models.DeliveryReceipt{}).
		Where("id = ? AND status <> ? AND attempts = ?", row.ID, models.DeliveryReceiptSent, row.Attempts).
		Where("status <> ? OR last_tried_at IS NULL OR last_tried_at <= ?",
			models.DeliveryReceiptPending, staleBefore).
		Updates(map[string]any{"status": models.DeliveryReceiptPending, "attempts": attempt, "last_tried_at": now, "last_error": ""})
	if res.Error != nil {
		return attempt, false, "", res.Error
	}
	if res.RowsAffected != 1 {
		return attempt, false, "concurrent_claim", nil
	}
	return attempt, true, "claimed", nil
}

func (m *Manager) finishDelivery(env Envelope, attempt int, sendErr error) {
	if m.deliveryDB == nil || strings.TrimSpace(env.DedupeKey) == "" {
		return
	}
	now := time.Now()
	updates := map[string]any{"updated_at": now}
	if sendErr == nil {
		updates["status"], updates["sent_at"] = models.DeliveryReceiptSent, now
	} else {
		updates["status"], updates["last_error"] = models.DeliveryReceiptFailed, truncateRunes(sendErr.Error(), 200)
	}
	_ = m.deliveryDB.Model(&models.DeliveryReceipt{}).
		Where("dedupe_key = ? AND attempts = ?", env.DedupeKey, attempt).Updates(updates).Error
}

func isDeliveryUnique(err error) bool {
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	lower := strings.ToLower(fmt.Sprint(err))
	return strings.Contains(lower, "unique") || strings.Contains(lower, "duplicate")
}

func (m *Manager) auditDelivery(env Envelope, action, result string, attempt int) {
	if m.deliveryAudit == nil || strings.TrimSpace(env.Context.ProjectID) == "" {
		return
	}
	outcome := models.AuditOutcomeOK
	if result == "failed" || result == "receipt_error" {
		outcome = models.AuditOutcomeFail
	}
	m.deliveryAudit.Record(services.AuditRecord{
		ProjectID: env.Context.ProjectID, Actor: services.SystemActor(), CallerKind: models.CallerKindSystem,
		Action: "delivery." + action, ResourceType: "delivery", ResourceID: env.DedupeKey,
		RunID: env.Context.RunID, Outcome: outcome, Summary: string(env.Reason),
		Payload: map[string]any{
			"reason": string(env.Reason), "runId": env.Context.RunID,
			"channel": firstString(env.Channels), "dedupe": env.DedupeKey,
			"result": result, "attempt": attempt,
		},
	})
}

func firstString(v []string) string {
	if len(v) == 0 {
		return ""
	}
	return v[0]
}

func deliveryKey(v string) string {
	sum := sha256.Sum256([]byte(v))
	return fmt.Sprintf("%x", sum[:8])
}

// DetectIMLanguage uses the current message first, then recent conversation,
// and defaults to zh-CN.
func DetectIMLanguage(current, recent string) string {
	for _, text := range []string{current, recent} {
		if strings.TrimSpace(text) == "" {
			continue
		}
		for _, r := range text {
			if unicode.Is(unicode.Han, r) {
				return "zh-CN"
			}
		}
		for _, r := range text {
			if unicode.IsLetter(r) && r <= unicode.MaxASCII {
				return "en"
			}
		}
	}
	return "zh-CN"
}

func TaskMessagePrefix(shortTitle, typ string) string {
	title, kind := strings.TrimSpace(shortTitle), strings.TrimSpace(typ)
	if title == "" {
		title = "任务"
	}
	if kind == "" {
		kind = "状态"
	}
	return "【" + title + "｜" + kind + "】"
}

func IMTypeLabel(kind, language string) string {
	en := language == "en"
	switch strings.TrimSpace(kind) {
	case string(ReasonRunAcceptanceACK), "accepted":
		if en {
			return "accepted"
		}
		return "已接收"
	case string(ReasonProgress), DeliveryTypeStage:
		if en {
			return "progress"
		}
		return "进度"
	case string(ReasonBlocked), "failed":
		if en {
			return "blocked"
		}
		return "阻塞"
	case string(ReasonActionRequired), "waiting_human":
		if en {
			return "action required"
		}
		return "需确认"
	case string(ReasonFinal), DeliveryTypeStructuredSummary:
		if en {
			return "final"
		}
		return "结论"
	default:
		return strings.TrimSpace(kind)
	}
}

func FormatTaskMessage(shortTitle, typ, zh, en, language string) string {
	body := strings.TrimSpace(zh)
	if language == "en" {
		body = strings.TrimSpace(en)
	}
	if body == "" {
		body = strings.TrimSpace(zh)
	}
	return TaskMessagePrefix(shortTitle, typ) + body
}

// ExtractStructuredFinalSummary accepts only an explicit final-summary marker.
// Unmarked terminal assistant text remains internal.
func ExtractStructuredFinalSummary(text string) (string, bool) {
	trimmed := strings.TrimSpace(text)
	for _, marker := range []string{"【总结】", "[总结]", "【final】", "[final]"} {
		if strings.HasPrefix(strings.ToLower(trimmed), strings.ToLower(marker)) {
			summary := strings.TrimSpace(trimmed[len(marker):])
			if summary != "" {
				return truncateRunes(summary, 500), true
			}
		}
	}
	return "", false
}
