package channels

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/cocofhu/approving/internal/models"
	"github.com/cocofhu/approving/internal/sendable"
	"github.com/cocofhu/approving/internal/services"

	"github.com/rs/zerolog/log"
)

// ErrNoDeliveryChannel is returned by Deliver when no enabled channel for the
// project is configured as the cron delivery target.
var ErrNoDeliveryChannel = errors.New("项目未配置定时任务推送渠道")

// ErrNoRunNotifyTarget is returned by DeliverRunNotify when no enabled channel
// has a usable QQ target session for the project (CronDeliver flag not required).
var ErrNoRunNotifyTarget = errors.New("项目未配置可用的 QQ 推送目标")

// Reply/Work equivalent orchestration (physical dual sandbox NOT required):
//
//   - Manager (= Reply): the unique QQ egress — immediate ACK, queue ACK,
//     on-demand progress, terminal reports, and cron push coordination.
//   - ChannelBridge / PmTurnRunner / cron sandbox (= Work): execute turns and
//     emit internal progress/results; must not bypass Manager to Send on QQ.
//
// Speak priority: user ACK/final > on-demand progress > cron push > unchanged.

const (
	ackProcessingPrefix = "收到，正在处理："
	ackSummaryRunes     = 40
	queueAckPrefix      = "已收到，排队中"
	queueFullText       = "队列已满，请稍候"
	failReplyPrefix     = "处理失败："
	// convQueueDepth is the per-conversation pending FIFO capacity (in-flight
	// turn is not counted). The next inbound after 16 pending is rejected.
	convQueueDepth = 16
)

// queuedInbound is a message waiting behind an in-flight turn for the same
// conversation key (project|scene|conversationID).
type queuedInbound struct {
	ctx context.Context
	rc  *runningChannel
	in  InboundMessage
}

// convQueue serializes turns for one conversation: at most one in-flight
// handler plus a bounded FIFO of pending inbound messages.
type convQueue struct {
	mu      sync.Mutex
	pending []queuedInbound
	busy    bool
}

// Manager owns the lifecycle of channel adapters and is the Reply-side unique
// QQ egress. Configs are supplied by a loader (backed by the DB) and applied
// idempotently: adapters are started, stopped, or restarted based on a config
// fingerprint so admin edits hot-reload without a server restart.
type Manager struct {
	bridge    *ChannelBridge
	factories map[string]AdapterFactory
	decrypt   func(enc string) (string, error)
	loader    func() ([]models.ChannelConfig, error)

	applyMu sync.Mutex // serializes Apply/Reload
	mu      sync.Mutex
	running map[string]*runningChannel // keyed by config ID

	convMu     sync.Mutex
	convQueues map[string]*convQueue

	pushMu     sync.Mutex
	pushQueues map[string]*pushQueue

	baseCtx          context.Context
	policy           *sendable.Policy
	taskContext      *services.TaskContextService
	riskConfirmation *services.RiskConfirmationService
	// retryBackoff overrides the delivery backoff (tests set it to zero).
	retryBackoff func(attempt int) time.Duration

	ambiguityMu sync.Mutex
	ambiguity   map[string]ambiguityMemory

	riskExecutor RiskActionExecutor

	// Test hooks (production leaves these nil/zero):
	// handleFunc overrides bridge.Handle when set (no progress callback).
	handleFunc func(ctx context.Context, rc ResolvedChannel, in InboundMessage) (Reply, error)
	// handleFuncWithProgress overrides bridge.Handle when set.
	handleFuncWithProgress func(ctx context.Context, rc ResolvedChannel, in InboundMessage, onProgress func(ProgressEvent)) (Reply, error)
}

type runningChannel struct {
	cfg         models.ChannelConfig
	fingerprint string
	adapter     Adapter
	cancel      context.CancelFunc
}

// NewManager builds a manager. factories maps channel type → adapter factory;
// decrypt reverses the stored app secret.
func NewManager(bridge *ChannelBridge, factories map[string]AdapterFactory, decrypt func(string) (string, error)) *Manager {
	return &Manager{
		bridge:     bridge,
		factories:  factories,
		decrypt:    decrypt,
		running:    map[string]*runningChannel{},
		convQueues: map[string]*convQueue{},
		pushQueues: map[string]*pushQueue{},
		baseCtx:    context.Background(),
		policy:     sendable.NewPolicy(nil, nil),
	}
}

// SetSendablePolicy installs the single delivery gate used before Adapter.Send.
func (m *Manager) SetSendablePolicy(policy *sendable.Policy) {
	if policy != nil {
		m.policy = policy
	}
}

// SetRetryBackoff overrides the outbound retry backoff schedule.
func (m *Manager) SetRetryBackoff(backoff func(attempt int) time.Duration) {
	m.retryBackoff = backoff
}

// SetTaskContextService exposes DB-backed task identity/focus to orchestration.
func (m *Manager) SetTaskContextService(service *services.TaskContextService) {
	m.taskContext = service
}

// TaskContextService returns the configured task context service.
func (m *Manager) TaskContextService() *services.TaskContextService { return m.taskContext }

// SetRiskConfirmationService exposes one-shot high-risk confirmation tickets.
func (m *Manager) SetRiskConfirmationService(service *services.RiskConfirmationService) {
	m.riskConfirmation = service
}

// RiskConfirmationService returns the configured ticket service.
func (m *Manager) RiskConfirmationService() *services.RiskConfirmationService {
	return m.riskConfirmation
}

// SetLoader registers the DB-backed config source used by Reload/ApplyOnBoot.
func (m *Manager) SetLoader(loader func() ([]models.ChannelConfig, error)) {
	m.loader = loader
}

// ApplyOnBoot loads persisted configs and starts enabled adapters.
func (m *Manager) ApplyOnBoot() {
	m.Reload()
}

// Reload re-reads configs from the loader and applies them.
func (m *Manager) Reload() {
	if m.loader == nil {
		return
	}
	cfgs, err := m.loader()
	if err != nil {
		log.Warn().Err(err).Msg("channel manager: load configs failed")
		return
	}
	m.Apply(cfgs)
}

// Apply reconciles running adapters against the desired config set. Adapters
// are started asynchronously so a slow connect never blocks the caller
// (admin write / boot).
func (m *Manager) Apply(cfgs []models.ChannelConfig) {
	m.applyMu.Lock()
	defer m.applyMu.Unlock()

	desired := map[string]models.ChannelConfig{}
	for _, c := range cfgs {
		if !c.Enabled {
			continue
		}
		desired[c.ID] = c
	}

	// Stop removed or changed adapters.
	m.mu.Lock()
	var toStop []*runningChannel
	for id, rc := range m.running {
		want, ok := desired[id]
		if !ok || fingerprint(want) != rc.fingerprint {
			toStop = append(toStop, rc)
			delete(m.running, id)
		}
	}
	m.mu.Unlock()
	for _, rc := range toStop {
		m.stopChannel(rc)
	}

	// Start new / changed adapters.
	for id, cfg := range desired {
		m.mu.Lock()
		_, already := m.running[id]
		m.mu.Unlock()
		if already {
			continue
		}
		m.startChannel(cfg)
	}
}

// StopAll tears down every running adapter (shutdown path).
func (m *Manager) StopAll() {
	m.mu.Lock()
	all := make([]*runningChannel, 0, len(m.running))
	for _, rc := range m.running {
		all = append(all, rc)
	}
	m.running = map[string]*runningChannel{}
	m.mu.Unlock()
	for _, rc := range all {
		m.stopChannel(rc)
	}
}

func (m *Manager) startChannel(cfg models.ChannelConfig) {
	factory, ok := m.factories[cfg.Type]
	if !ok {
		log.Warn().Str("type", cfg.Type).Str("id", cfg.ID).Msg("channel manager: no adapter factory")
		return
	}
	secret := ""
	if strings.TrimSpace(cfg.AppSecretEnc) != "" {
		dec, err := m.decrypt(cfg.AppSecretEnc)
		if err != nil {
			log.Warn().Err(err).Str("id", cfg.ID).Msg("channel manager: decrypt secret failed; skipping")
			return
		}
		secret = dec
	}
	adapter, err := factory(AdapterConfig{
		ID: cfg.ID, Type: cfg.Type, Name: cfg.Name, ProjectID: cfg.ProjectID,
		AppID: cfg.AppID, AppSecret: secret, Config: cfg.Config,
	})
	if err != nil {
		log.Warn().Err(err).Str("id", cfg.ID).Msg("channel manager: build adapter failed")
		return
	}
	ctx, cancel := context.WithCancel(m.baseCtx)
	rc := &runningChannel{cfg: cfg, fingerprint: fingerprint(cfg), adapter: adapter, cancel: cancel}
	m.mu.Lock()
	m.running[cfg.ID] = rc
	m.mu.Unlock()

	go func() {
		handler := func(ctx context.Context, in InboundMessage) {
			m.dispatch(ctx, rc, in)
		}
		if err := adapter.Start(ctx, handler); err != nil {
			log.Warn().Err(err).Str("id", cfg.ID).Str("type", cfg.Type).Msg("channel adapter start failed")
			cancel()
			m.mu.Lock()
			if cur := m.running[cfg.ID]; cur == rc {
				delete(m.running, cfg.ID)
			}
			m.mu.Unlock()
			return
		}
		log.Info().Str("id", cfg.ID).Str("type", cfg.Type).Str("project", cfg.ProjectID).Msg("channel adapter started")
	}()
}

func (m *Manager) stopChannel(rc *runningChannel) {
	if rc == nil {
		return
	}
	rc.cancel()
	if err := rc.adapter.Stop(); err != nil {
		log.Warn().Err(err).Str("id", rc.cfg.ID).Msg("channel adapter stop error")
	}
	log.Info().Str("id", rc.cfg.ID).Str("type", rc.cfg.Type).Msg("channel adapter stopped")
}

// convKey builds the per-conversation queue key.
func convKey(projectID string, scene Scene, conversationID string) string {
	return projectID + "|" + string(scene) + "|" + conversationID
}

// IsConversationBusy reports whether a user turn is in-flight or the user FIFO
// is non-empty for the conversation. Shared by cron push coordination.
func (m *Manager) IsConversationBusy(projectID string, scene Scene, conversationID string) bool {
	q := m.convQueueFor(convKey(projectID, scene, conversationID))
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.busy || len(q.pending) > 0
}

// dispatch serializes messages per conversation via a bounded in-process FIFO.
// Idle + empty queue: immediate processing ACK then Work. Busy: enqueue with a
// per-message queue ACK (ahead count); dequeue sends another processing ACK.
// Full queue: reject with a visible reply (never silently drop).
func (m *Manager) dispatch(ctx context.Context, rc *runningChannel, in InboundMessage) {
	// A notice-only inbound (e.g. every attachment rejected) has no turn to run;
	// it still goes out through the single policy/dedupe/audit egress.
	if in.Safety != nil && in.Safety.Only {
		m.sendSafetyNotice(ctx, rc, in, *in.Safety)
		return
	}
	key := convKey(rc.cfg.ProjectID, in.Scene, in.ConversationID)
	q := m.convQueueFor(key)

	q.mu.Lock()
	if q.busy {
		if len(q.pending) >= convQueueDepth {
			q.mu.Unlock()
			m.sendOutbound(ctx, rc, OutboundMessage{
				Scene: in.Scene, ConversationID: in.ConversationID,
				ReplyToMessageID: in.MessageID, Text: queueFullText,
				Envelope: turnEnvelope(rc, in, sendable.KindSafetyNotice, "queue_full", sendable.PriorityHigh),
			})
			return
		}
		// Ahead = in-flight turn + already-queued messages.
		ahead := 1 + len(q.pending)
		q.pending = append(q.pending, queuedInbound{ctx: ctx, rc: rc, in: in})
		q.mu.Unlock()
		m.sendOutbound(ctx, rc, OutboundMessage{
			Scene: in.Scene, ConversationID: in.ConversationID,
			ReplyToMessageID: in.MessageID, Text: queueAckTextFor(ahead, in.Text),
			Envelope: turnEnvelope(rc, in, sendable.KindQueueAck, "turn_queued", sendable.PriorityHigh),
		})
		return
	}
	q.busy = true
	q.mu.Unlock()

	// This goroutine owns the busy cycle: run the idle-first turn, then drain.
	m.runTurn(ctx, rc, in, true /* withProcessingAck */)
	m.drainConvQueue(q, key)
}

// sendSafetyNotice delivers an adapter-detected notice through the Manager, so a
// replayed inbound message cannot produce a second notice and the outcome is
// audited as sent or suppressed like any other outbound.
func (m *Manager) sendSafetyNotice(ctx context.Context, rc *runningChannel, in InboundMessage, notice SafetyNotice) DeliveryResult {
	text := strings.TrimSpace(notice.Text)
	if text == "" {
		return DeliveryResult{Decision: sendable.Decision{Reason: "empty"}}
	}
	reason := strings.TrimSpace(notice.Reason)
	if reason == "" {
		reason = "safety_notice"
	}
	envelope := turnEnvelope(rc, in, sendable.KindSafetyNotice, reason, sendable.PriorityHigh)
	if key := strings.TrimSpace(notice.DedupeKey); key != "" {
		envelope.DedupeKey = key
	}
	return m.sendOutboundResult(ctx, rc, OutboundMessage{
		Scene: in.Scene, ConversationID: in.ConversationID,
		ReplyToMessageID: in.MessageID, Text: text, Envelope: envelope,
	})
}

// drainConvQueue runs pending messages in arrival order until the queue is
// empty, then flushes any silent cron push queue for this conversation.
func (m *Manager) drainConvQueue(q *convQueue, key string) {
	for {
		q.mu.Lock()
		if len(q.pending) == 0 {
			q.busy = false
			q.mu.Unlock()
			m.flushPushQueue(key)
			return
		}
		next := q.pending[0]
		q.pending = q.pending[1:]
		q.mu.Unlock()
		// Dequeue: another processing ACK before Work.
		m.runTurn(next.ctx, next.rc, next.in, true /* withProcessingAck */)
	}
}

// runTurn executes one PM turn and sends the final or failure reply.
// withProcessingAck emits the required ≤1s ACK before dispatching Work.
func (m *Manager) runTurn(ctx context.Context, rc *runningChannel, in InboundMessage, withProcessingAck bool) {
	if withProcessingAck {
		m.sendOutbound(ctx, rc, OutboundMessage{
			Scene: in.Scene, ConversationID: in.ConversationID,
			ReplyToMessageID: in.MessageID, Text: processingAckText(in.Text),
			Envelope: turnEnvelope(rc, in, sendable.KindTurnProcessingAck, "turn_processing", sendable.PriorityHigh),
		})
	}

	if m.handleInboundOrchestration(ctx, rc, &in) {
		return
	}

	resolved := ResolvedChannel{
		ID: rc.cfg.ID, Type: rc.cfg.Type, ProjectID: rc.cfg.ProjectID,
		TurnTimeout: time.Duration(rc.cfg.TurnTimeoutSeconds) * time.Second,
		Caps:        SessionCapsFromConfig(rc.cfg.Config),
	}
	onProgress := func(ev ProgressEvent) {
		text := FormatProgressText(ev)
		if text == "" {
			return
		}
		// Classification-only events stay internal; sendOutbound still runs so
		// the suppression is audited with a reason instead of vanishing.
		m.sendOutbound(ctx, rc, OutboundMessage{
			Scene: in.Scene, ConversationID: in.ConversationID,
			ReplyToMessageID: in.MessageID, Text: text,
			Envelope: progressEnvelope(rc, in, ev),
		})
	}
	reply, err := m.handleTurn(ctx, resolved, in, onProgress)
	if err != nil {
		log.Warn().Err(err).Str("channel", rc.cfg.ID).Msg("channel turn failed")
		m.sendOutbound(ctx, rc, OutboundMessage{
			Scene: in.Scene, ConversationID: in.ConversationID,
			ReplyToMessageID: in.MessageID, Text: failReplyPrefix + friendlyErr(err),
			Envelope: turnEnvelope(rc, in, sendable.KindBlocked, "turn_failed", sendable.PriorityCritical),
		})
		return
	}
	summary, reason := deliverableFinalText(reply)
	images := reply.ImageURLs
	if summary == safeFinalNotice {
		images = nil // no structured summary → no derived attachments either
	}
	m.sendOutbound(ctx, rc, OutboundMessage{
		Scene: in.Scene, ConversationID: in.ConversationID, ReplyToMessageID: in.MessageID,
		Text: summary, ImageURLs: images,
		Envelope: func() sendable.DeliveryEnvelope {
			e := turnEnvelope(rc, in, sendable.KindFinal, reason, sendable.PriorityCritical)
			e.Structured = true
			return e
		}(),
	})
}

// safeFinalNotice is sent when Work produced no structured summary. Raw
// assistant text is never used as a fallback.
const safeFinalNotice = "本回合已结束，请在 Approving 查看完整结果。"

// deliverableFinalText returns the only text allowed out of a finished turn:
// the structured FinalSummary, or a fixed safe notice. Reply.Text stays
// internal regardless of its content.
func deliverableFinalText(reply Reply) (text, reason string) {
	if summary := strings.TrimSpace(reply.FinalSummary); summary != "" {
		return truncateRunes(summary, 240), "structured_turn_final"
	}
	return safeFinalNotice, "final_summary_missing_safe_notice"
}

func (m *Manager) handleTurn(ctx context.Context, rc ResolvedChannel, in InboundMessage, onProgress func(ProgressEvent)) (Reply, error) {
	if m.handleFuncWithProgress != nil {
		return m.handleFuncWithProgress(ctx, rc, in, onProgress)
	}
	if m.handleFunc != nil {
		return m.handleFunc(ctx, rc, in)
	}
	return m.bridge.Handle(ctx, rc, in, onProgress)
}

// turnScope identifies one conversation turn. A turn is NOT a Run: the inbound
// platform MessageID may only key turn-level dedupe, never a run_id.
func turnScope(rc *runningChannel, in InboundMessage) string {
	if id := strings.TrimSpace(in.MessageID); id != "" {
		return "turn:" + rc.cfg.ProjectID + "|" + id
	}
	return "turn:" + convKey(rc.cfg.ProjectID, in.Scene, in.ConversationID)
}

// turnEnvelope builds a turn-scoped envelope. RunID stays empty on purpose;
// Run-scoped delivery must come from ProgressEvent.RunID or an explicit
// DeliverSendable / SendRunAcceptanceAck call that carries a real Run id.
func turnEnvelope(rc *runningChannel, in InboundMessage, kind sendable.Kind, reason string, priority sendable.Priority) sendable.DeliveryEnvelope {
	scope := turnScope(rc, in)
	e := sendable.DeliveryEnvelope{
		Priority: priority, TaskContext: scope, ProjectID: rc.cfg.ProjectID,
		ConversationID: in.ConversationID, UserID: in.UserID,
		DedupeKey: scope + ":" + string(kind),
		Reason:    reason, Kind: kind,
	}
	return sendable.AppendSendable(e, sendable.ChannelQQ)
}

// progressEnvelope maps a progress event onto a delivery envelope. Events that
// are not explicitly deliverable stay internal so a prompt-shaped marker in
// model output can never reach a channel.
func progressEnvelope(rc *runningChannel, in InboundMessage, ev ProgressEvent) sendable.DeliveryEnvelope {
	if !ev.Deliverable() {
		return sendable.Internal(sendable.KindAgentRaw, "classified_progress_not_sendable")
	}
	kind := sendable.KindProgress
	priority := sendable.PriorityNormal
	switch {
	case ev.Blocked || ev.Kind == ProgressBlocker:
		kind, priority = sendable.KindBlocked, sendable.PriorityCritical
	case ev.ActionRequired || ev.Kind == ProgressConfirm:
		kind, priority = sendable.KindActionRequired, sendable.PriorityCritical
	}
	e := turnEnvelope(rc, in, kind, strings.TrimSpace(ev.Reason), priority)
	if runID := strings.TrimSpace(ev.RunID); runID != "" {
		e.RunID = runID
		e.TaskContext = ""
	}
	if key := strings.TrimSpace(ev.DedupeKey); key != "" {
		e.DedupeKey = key
	} else {
		e.DedupeKey = ""
	}
	e.Progress = sendable.ProgressFields{
		Stage: ev.Stage, Blocked: ev.Blocked || ev.Kind == ProgressBlocker,
		ActionRequired: ev.ActionRequired || ev.Kind == ProgressConfirm,
		Conclusion:     ev.Conclusion,
	}
	if kind == sendable.KindProgress && strings.TrimSpace(e.Progress.Stage) == "" {
		e.Progress.Stage = strings.TrimSpace(ev.Summary)
	}
	return e
}

func (m *Manager) sendOutbound(ctx context.Context, rc *runningChannel, out OutboundMessage) {
	_ = m.sendOutboundResult(ctx, rc, out)
}

func (m *Manager) sendOutboundResult(ctx context.Context, rc *runningChannel, out OutboundMessage) DeliveryResult {
	if rc == nil || rc.adapter == nil {
		return DeliveryResult{Decision: sendable.Decision{Reason: "no_adapter"}}
	}
	contentFingerprint := out.Text + "\n" + strings.Join(out.ImageURLs, "\n")
	decision, err := m.policy.Evaluate(ctx, out.Envelope, sendable.ChannelQQ, contentFingerprint)
	if err != nil {
		log.Warn().Err(err).Str("channel", rc.cfg.ID).Msg("channel outbound policy failed closed")
		return DeliveryResult{Decision: sendable.Decision{Reason: "policy_error"}}
	}
	for decision.Send {
		sent, sendErr := rc.adapter.Send(ctx, out)
		if sendErr == nil {
			_ = m.policy.MarkSent(ctx, decision, out.Envelope, sendable.ChannelQQ)
			m.bindOutboundMessage(rc, out, sent.MessageID)
			return DeliveryResult{Decision: decision, Sent: true, ExternalMessageID: sent.MessageID}
		}
		_ = m.policy.MarkFailed(ctx, decision, out.Envelope, sendable.ChannelQQ, sendErr)
		log.Warn().Err(sendErr).Str("channel", rc.cfg.ID).Int("attempt", decision.Attempt).
			Msg("channel outbound send failed")
		if !m.waitBackoff(ctx, decision.Attempt) {
			decision.Reason = "transport_failed"
			return DeliveryResult{Decision: decision}
		}
		// Retry claims the next bounded attempt for this dedupe key; it returns
		// Send=false once the receipt is sent elsewhere or attempts are spent.
		next, err := m.policy.Retry(ctx, decision, out.Envelope, sendable.ChannelQQ)
		if err != nil {
			log.Warn().Err(err).Str("channel", rc.cfg.ID).Msg("channel outbound retry claim failed")
			decision.Reason = "retry_claim_failed"
			return DeliveryResult{Decision: decision}
		}
		decision = next
	}
	if decision.Reason == "" {
		decision.Reason = "suppressed"
	}
	return DeliveryResult{Decision: decision}
}

// bindOutboundMessage records "this channel message belongs to that Run" so a
// later reply reference resolves to the same task without any guessing. It is
// best-effort and deliberately narrow: a delivery without a real Run id, without
// a channel-reported message id, without a sender, or whose Run has no task
// identity in this user's scope is simply not bound. No id is ever synthesized.
func (m *Manager) bindOutboundMessage(rc *runningChannel, out OutboundMessage, messageID string) {
	if m.taskContext == nil || rc == nil {
		return
	}
	runID := strings.TrimSpace(out.Envelope.RunID)
	messageID = strings.TrimSpace(messageID)
	qqUserID := strings.TrimSpace(out.Envelope.UserID)
	if runID == "" || messageID == "" || qqUserID == "" {
		return
	}
	identity, err := m.taskContext.IdentityForRun(runID, rc.cfg.ProjectID)
	if err != nil {
		log.Warn().Err(err).Str("run", runID).Msg("outbound message binding: load task identity failed")
		return
	}
	if identity == nil {
		return
	}
	scope := services.TaskScope{
		ProjectID: rc.cfg.ProjectID, UserID: services.SyntheticQQUserID(qqUserID),
		Channel: rc.cfg.Type, ConversationID: out.ConversationID,
	}
	// BindMessage re-checks project/user ownership, so another user's task can
	// never be bound to this conversation's message.
	if err := m.taskContext.BindMessage(scope, messageID, identity); err != nil {
		log.Warn().Err(err).Str("run", runID).Str("message", messageID).
			Msg("outbound message binding skipped")
	}
}

// waitBackoff sleeps the delivery backoff and reports whether the caller may
// continue (false when the context ended).
func (m *Manager) waitBackoff(ctx context.Context, attempt int) bool {
	delay := sendable.RetryDelay(attempt)
	if m.retryBackoff != nil {
		delay = m.retryBackoff(attempt)
	}
	if delay <= 0 {
		return ctx.Err() == nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

// Deliver pushes cron/proactive text to a project's configured delivery channel.
// Legacy entry: treats body as "changed" and routes through coordinated egress.
// Implements services.ChannelDeliverer.
func (m *Manager) Deliver(projectID, text string) error {
	return m.DeliverCron(services.CronDelivery{
		ProjectID: projectID,
		Category:  "cron",
		Kind:      string(CronResultChanged),
		Text:      text,
	})
}

// DeliverCron is the coordinated cron→QQ egress: busy conversations silent-enqueue
// (no enqueue side-chat); idle sends immediately. Implements services.ChannelDeliverer.
func (m *Manager) DeliverCron(d services.CronDelivery) error {
	target, scene, conv, err := m.lookupDeliveryTarget(d.ProjectID)
	if err != nil {
		return err
	}
	_ = target
	kind := CronResultKind(strings.TrimSpace(d.Kind))
	switch kind {
	case CronResultChanged, CronResultUnchanged, CronResultFailed:
	default:
		kind = ClassifyCronResult(d.Text)
	}
	text := FormatCronPush(d.Category, kind, d.Text)
	key := convKey(d.ProjectID, scene, conv)
	item := CronPushItem{
		ProjectID: d.ProjectID,
		Scene:     scene,
		Conv:      conv,
		Category:  d.Category,
		Kind:      kind,
		// Keep raw formatted text (incl. image URLs); flushPushQueue splits on send.
		Text: text,
		Envelope: sendable.AppendSendable(sendable.DeliveryEnvelope{
			Priority: func() sendable.Priority {
				if kind == CronResultFailed {
					return sendable.PriorityCritical
				}
				return sendable.PriorityLow
			}(),
			// Cron jobs are not Runs: they carry a real Run id only when the
			// scheduler supplied one, otherwise a cron task scope.
			RunID:       strings.TrimSpace(d.RunID),
			TaskContext: cronTaskContext(d),
			ProjectID:   d.ProjectID, ConversationID: conv,
			DedupeKey: d.DedupeKey, Reason: "cron_delivery", Kind: sendable.KindCron,
			Structured: true,
		}, sendable.ChannelQQ),
		Enqueued: time.Now(),
	}

	// Always enqueue then flush: idle path still goes through flushPushQueue so
	// a concurrent user ACK between IsConversationBusy and Send cannot insert
	// cron text ahead of the user turn (TOCTOU). Busy path stays silent — no
	// "已入队"旁白. flushPushQueue re-checks busy before each Send.
	m.enqueuePush(key, item)
	m.flushPushQueue(key)
	return nil
}

// DeliverRunNotify pushes a Run lifecycle notification to the project's bound
// QQ target session. Unlike DeliverCron it does NOT require CronDeliver=true —
// any Enabled channel with a non-empty CronDeliverTarget (session address) works.
// Text is sent as-is (no FormatCronPush). Implements services.RunNotifyDeliverer.
// Missing target returns services.ErrRunNotifyNoTarget (caller treats as no-op).
func (m *Manager) DeliverRunNotify(projectID, text string) error {
	_, scene, conv, err := m.lookupRunNotifyTarget(projectID)
	if err != nil {
		if errors.Is(err, ErrNoRunNotifyTarget) || errors.Is(err, ErrNoDeliveryChannel) {
			return services.ErrRunNotifyNoTarget
		}
		return err
	}
	key := convKey(projectID, scene, conv)
	item := CronPushItem{
		ProjectID: projectID,
		Scene:     scene,
		Conv:      conv,
		Category:  "run_notify",
		Kind:      CronResultChanged, // priority: treat actionable Run alerts like "changed"
		Text:      text,
		Envelope: sendable.AppendSendable(sendable.DeliveryEnvelope{
			Priority: sendable.PriorityCritical,
			// Only a Run id actually present in the notification is used; an
			// unlinked notification falls back to a conversation task scope.
			RunID:       runNotifyRunID(text),
			TaskContext: runNotifyTaskContext(projectID, text),
			ProjectID:   projectID, ConversationID: conv,
			Reason: "run_notification", Kind: sendable.KindRunNotify, Structured: true,
		}, sendable.ChannelQQ),
		Enqueued: time.Now(),
	}
	m.enqueuePush(key, item)
	m.flushPushQueue(key)
	return nil
}

// HasRunNotifyTarget reports whether an Enabled channel exposes a usable QQ
// session address for the project (independent of CronDeliver).
func (m *Manager) HasRunNotifyTarget(projectID string) bool {
	_, _, _, err := m.lookupRunNotifyTarget(projectID)
	return err == nil
}

func (m *Manager) flushPushQueue(key string) {
	items := m.takePushQueue(key)
	if len(items) == 0 {
		return
	}
	for i, item := range items {
		// Re-check busy: a new user message may have arrived.
		// Re-queue current AND all remaining — never drop the tail.
		if m.IsConversationBusy(item.ProjectID, item.Scene, item.Conv) {
			m.requeuePushAll(key, items[i:])
			return
		}
		var target *runningChannel
		var err error
		if item.Category == "run_notify" {
			target, _, _, err = m.lookupRunNotifyTarget(item.ProjectID)
		} else {
			target, _, _, err = m.lookupDeliveryTarget(item.ProjectID)
		}
		if err != nil {
			log.Warn().Err(err).Str("project", item.ProjectID).Str("category", item.Category).
				Msg("push flush: no delivery channel")
			continue
		}
		stripped, urls := splitImageURLs(item.Text)
		ctx, cancel := context.WithTimeout(m.baseCtx, 60*time.Second)
		m.sendOutbound(ctx, target, OutboundMessage{
			Scene: item.Scene, ConversationID: item.Conv, Text: stripped,
			ImageURLs: urls, Envelope: item.Envelope,
		})
		cancel()
	}
}

// runNotifyRunID extracts the real Run id from a notification link, or "" when
// the notification is not linked to a Run. It never fabricates an id.
func runNotifyRunID(text string) string {
	const marker = "/runs/"
	idx := strings.Index(text, marker)
	if idx < 0 {
		return ""
	}
	value := text[idx+len(marker):]
	if end := strings.IndexAny(value, " \t\r\n?#"); end >= 0 {
		value = value[:end]
	}
	return strings.TrimSpace(value)
}

// runNotifyTaskContext scopes an unlinked notification so it still has a
// delivery bucket without pretending to be a Run.
func runNotifyTaskContext(projectID, text string) string {
	if runNotifyRunID(text) != "" {
		return ""
	}
	return "run-notify:" + projectID
}

// cronTaskContext scopes a cron push that carries no Run id.
func cronTaskContext(d services.CronDelivery) string {
	if strings.TrimSpace(d.RunID) != "" {
		return ""
	}
	category := strings.TrimSpace(d.Category)
	if category == "" {
		category = "cron"
	}
	return "cron:" + d.ProjectID + "|" + category
}

// requeuePushAll puts remaining flush items back ahead of anything that arrived
// while the queue was taken, re-applying merge/depth via enqueuePush so pending
// never silently exceeds pushQueueDepth.
func (m *Manager) requeuePushAll(key string, items []CronPushItem) {
	if len(items) == 0 {
		return
	}
	arrived := m.drainPushQueueRaw(key)
	combined := append(append([]CronPushItem(nil), items...), arrived...)
	for _, item := range combined {
		m.enqueuePush(key, item)
	}
}

// drainPushQueueRaw clears pending without priority reordering (used by requeue).
func (m *Manager) drainPushQueueRaw(key string) []CronPushItem {
	q := m.pushQueueFor(key)
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.pending) == 0 {
		return nil
	}
	out := append([]CronPushItem(nil), q.pending...)
	q.pending = nil
	return out
}

func (m *Manager) lookupDeliveryTarget(projectID string) (*runningChannel, Scene, string, error) {
	m.mu.Lock()
	var target *runningChannel
	for _, rc := range m.running {
		if rc.cfg.ProjectID == projectID && rc.cfg.CronDeliver && strings.TrimSpace(rc.cfg.CronDeliverTarget) != "" {
			if target == nil || rc.cfg.UpdatedAt.After(target.cfg.UpdatedAt) {
				target = rc
			}
		}
	}
	m.mu.Unlock()
	if target == nil {
		return nil, "", "", ErrNoDeliveryChannel
	}
	scene, conv := parseTarget(target.cfg.CronDeliverTarget)
	if conv == "" {
		return nil, "", "", ErrNoDeliveryChannel
	}
	return target, scene, conv, nil
}

// lookupRunNotifyTarget finds an Enabled channel with a usable session address
// (CronDeliverTarget), without requiring CronDeliver=true.
func (m *Manager) lookupRunNotifyTarget(projectID string) (*runningChannel, Scene, string, error) {
	m.mu.Lock()
	var target *runningChannel
	for _, rc := range m.running {
		if !rc.cfg.Enabled {
			continue
		}
		if rc.cfg.ProjectID == projectID && strings.TrimSpace(rc.cfg.CronDeliverTarget) != "" {
			if target == nil || rc.cfg.UpdatedAt.After(target.cfg.UpdatedAt) {
				target = rc
			}
		}
	}
	m.mu.Unlock()
	if target == nil {
		return nil, "", "", ErrNoRunNotifyTarget
	}
	scene, conv := parseTarget(target.cfg.CronDeliverTarget)
	if conv == "" {
		return nil, "", "", ErrNoRunNotifyTarget
	}
	return target, scene, conv, nil
}

func (m *Manager) convQueueFor(key string) *convQueue {
	m.convMu.Lock()
	defer m.convMu.Unlock()
	q, ok := m.convQueues[key]
	if !ok {
		q = &convQueue{}
		m.convQueues[key] = q
	}
	return q
}

func processingAckText(userText string) string {
	return ackProcessingPrefix + truncateRunes(userText, ackSummaryRunes)
}

func queueAckTextFor(ahead int, userText string) string {
	summary := truncateRunes(userText, ackSummaryRunes)
	if ahead > 0 {
		return fmt.Sprintf("%s（前方 %d 条）：%s", queueAckPrefix, ahead, summary)
	}
	return fmt.Sprintf("%s：%s", queueAckPrefix, summary)
}

func parseTarget(target string) (Scene, string) {
	parts := strings.SplitN(strings.TrimSpace(target), ":", 2)
	if len(parts) != 2 {
		return "", ""
	}
	return Scene(parts[0]), parts[1]
}

func fingerprint(c models.ChannelConfig) string {
	b, _ := json.Marshal(struct {
		Type, ProjectID, AppID, AppSecretEnc, CronDeliverTarget string
		Enabled, CronDeliver                                    bool
		TurnTimeoutSeconds                                      int
		Config                                                  map[string]any
	}{
		Type: c.Type, ProjectID: c.ProjectID, AppID: c.AppID, AppSecretEnc: c.AppSecretEnc,
		CronDeliverTarget: c.CronDeliverTarget, Enabled: c.Enabled, CronDeliver: c.CronDeliver,
		TurnTimeoutSeconds: c.TurnTimeoutSeconds, Config: c.Config,
	})
	return string(b)
}

func friendlyErr(err error) string {
	msg := err.Error()
	// Truncate on runes so a multi-byte (e.g. Chinese) message is never cut
	// mid-character into invalid UTF-8.
	if r := []rune(msg); len(r) > 200 {
		msg = string(r[:200]) + "…"
	}
	return msg
}
