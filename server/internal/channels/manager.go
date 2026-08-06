package channels

import (
	"context"
	"encoding/json"
	"errors"
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

// The conversation layer and the work layer are separate on purpose, and only
// the conversation layer talks to the user:
//
//   - Manager is the sole QQ egress. It answers, relays background events into
//     the conversation that asked for them, and coordinates cron pushes.
//   - ChannelBridge / PmTurnRunner / cron sandbox execute turns and emit
//     internal progress and results. They must never Send on QQ directly.
//
// A foreground turn either answers or delegates; it never waits for a Run, so
// the user can keep talking while work happens. Speak priority when several
// things want the channel at once: the user's own answer, then background
// progress, then cron pushes.

const (
	// busyHintText is the only thing a user hears about queueing, and only when
	// the backlog is genuinely full. Live conversations do not narrate their own
	// plumbing: there is no per-message "received, working on it" and no queue
	// position, because those crowd out the answer without informing anyone.
	busyHintText = "我这边还在处理前面几条，稍等一下。"
	// busyHintCooldown rate-limits busyHintText per conversation so a burst
	// produces one hint instead of one per rejected message.
	busyHintCooldown = 2 * time.Minute
	// convQueueDepth is the per-conversation pending FIFO capacity (in-flight
	// turn is not counted). The next inbound after 16 pending is rejected.
	convQueueDepth = 16
	// foregroundTurnTimeout caps the agent's own work in a Live turn. The
	// foreground contract is "answer briefly or delegate", so exceeding this
	// means the agent tried to do the work inline.
	foregroundTurnTimeout = 25 * time.Second
	// sandboxOpenBudget is allowed on top, for getting a sandbox ready. It is
	// separate because the two waits are not comparable: a warm conversation
	// spends none of it, while the first message of a conversation waits for a
	// container. Folding it into the answer budget is how a plain question
	// times out before the agent has read it.
	sandboxOpenBudget = 45 * time.Second
	// stillWorkingDelay is how long a turn may leave the user with nothing at
	// all before saying so. The streaming opener (firstSentenceDelay) covers
	// any turn that has produced text; this only fires when there is genuinely
	// nothing yet, which in practice means a cold start. It is deliberately
	// later than the opener so a real sentence always wins the race.
	stillWorkingDelay = 8 * time.Second
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

	busyHintMu   sync.Mutex
	busyHintSent map[string]time.Time

	repliedMu sync.Mutex
	replied   map[string]bool

	// synthesize rewrites background events for the conversation they belong
	// to. nil means outcomes go out as structured fallbacks.
	synthesize SynthesisFunc

	captureMu sync.Mutex
	captured  map[string]*string

	pushMu     sync.Mutex
	pushQueues map[string]*pushQueue

	baseCtx          context.Context
	policy           *sendable.Policy
	taskContext      *services.TaskContextService
	riskConfirmation *services.RiskConfirmationService
	// retryBackoff overrides the delivery backoff (tests set it to zero).
	retryBackoff func(attempt int) time.Duration
	// openBudget overrides sandboxOpenBudget; a negative value means none.
	openBudget time.Duration
	// stillWorkingAfter overrides stillWorkingDelay (tests shorten it).
	stillWorkingAfter time.Duration

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
		bridge:       bridge,
		factories:    factories,
		decrypt:      decrypt,
		running:      map[string]*runningChannel{},
		convQueues:   map[string]*convQueue{},
		pushQueues:   map[string]*pushQueue{},
		busyHintSent: map[string]time.Time{},
		replied:      map[string]bool{},
		baseCtx:      context.Background(),
		policy:       sendable.NewPolicy(nil, nil),
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

// dispatch is the Live inbound pipeline.
//
// Order matters and is the heart of the Live model: messages that can be
// answered from stored task state are served before the queue is ever
// consulted, so asking "how's that going?" stays instant no matter how many
// Runs are executing or how long the agent has been thinking. Only messages
// that genuinely need the agent contend for the single per-conversation turn.
func (m *Manager) dispatch(ctx context.Context, rc *runningChannel, in InboundMessage) {
	// A notice-only inbound (e.g. every attachment rejected) has no turn to run;
	// it still goes out through the single policy/dedupe/audit egress.
	if in.Safety != nil && in.Safety.Only {
		m.sendSafetyNotice(ctx, rc, in, *in.Safety)
		return
	}

	// Fast path: answered from the database, never queued, never sandboxed.
	if m.handleFastPath(ctx, rc, &in) {
		return
	}

	key := convKey(rc.cfg.ProjectID, in.Scene, in.ConversationID)
	q := m.convQueueFor(key)

	q.mu.Lock()
	if q.busy {
		if len(q.pending) >= convQueueDepth {
			q.mu.Unlock()
			// Full queue must never drop silently, but it also must not emit one
			// notice per rejected message — that is how a burst turns into spam.
			m.sendBusyHint(ctx, rc, in, key)
			return
		}
		q.pending = append(q.pending, queuedInbound{ctx: ctx, rc: rc, in: in})
		q.mu.Unlock()
		return
	}
	q.busy = true
	q.mu.Unlock()

	// This goroutine owns the busy cycle: run the idle-first turn, then drain.
	m.runTurn(ctx, rc, in)
	m.drainConvQueue(q, key)
}

// handleInbound is the complete Live pipeline for one message, used by callers
// that are not going through the per-conversation queue.
func (m *Manager) handleInbound(ctx context.Context, rc *runningChannel, in InboundMessage) {
	if m.handleFastPath(ctx, rc, &in) {
		return
	}
	m.runTurn(ctx, rc, in)
}

// sendBusyHint emits at most one backlog notice per conversation per cooldown.
func (m *Manager) sendBusyHint(ctx context.Context, rc *runningChannel, in InboundMessage, key string) {
	now := time.Now()
	m.busyHintMu.Lock()
	last, seen := m.busyHintSent[key]
	if seen && now.Sub(last) < busyHintCooldown {
		m.busyHintMu.Unlock()
		log.Warn().Str("conversation", in.ConversationID).Str("project", rc.cfg.ProjectID).
			Msg("live: inbound dropped, queue full within busy-hint cooldown")
		return
	}
	m.busyHintSent[key] = now
	m.busyHintMu.Unlock()

	log.Warn().Str("conversation", in.ConversationID).Str("project", rc.cfg.ProjectID).
		Msg("live: inbound dropped, queue full")
	m.sendOutbound(ctx, rc, OutboundMessage{
		Scene: in.Scene, ConversationID: in.ConversationID,
		ReplyToMessageID: in.MessageID, Text: busyHintText,
		Envelope: turnEnvelope(rc, in, sendable.KindSafetyNotice, "queue_full", sendable.PriorityHigh),
	})
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
		m.runTurn(next.ctx, next.rc, next.in)
	}
}

// runTurn executes one foreground Live turn.
//
// The turn is expected to either answer briefly or delegate, and it is bounded
// so a conversation is never held open by work that belongs in a Run. No
// acknowledgement precedes it — the reply is the acknowledgement.
func (m *Manager) runTurn(ctx context.Context, rc *runningChannel, in InboundMessage) {
	// Keyed by conversation, not by inbound message id: the MCP host marks its
	// own replies and only knows which conversation it is serving.
	scope := conversationTurnScope(rc.cfg.ProjectID, in.Scene, in.ConversationID)
	m.clearReplied(scope)
	defer m.clearReplied(scope)

	pendingID := m.beginPendingTurn(rc, in)
	defer m.endPendingTurn(pendingID)

	timeout := foregroundTurnTimeout
	if configured := time.Duration(rc.cfg.TurnTimeoutSeconds) * time.Second; configured > 0 {
		timeout = configured
	}
	open := m.sandboxOpenBudget()
	// The outer deadline is a backstop covering both phases; the bridge bounds
	// each one separately so cold start cannot consume the answer budget.
	turnCtx, cancel := context.WithTimeout(ctx, timeout+open)
	defer cancel()

	resolved := ResolvedChannel{
		ID: rc.cfg.ID, Type: rc.cfg.Type, ProjectID: rc.cfg.ProjectID,
		OpenTimeout: open,
		TurnTimeout: timeout,
		Caps:        SessionCapsFromConfig(rc.cfg.Config),
	}
	stopWaiting := m.sayStillWorking(turnCtx, rc, in, scope)
	defer stopWaiting()
	onProgress := func(ev ProgressEvent) {
		text := FormatProgressText(ev)
		if text == "" {
			return
		}
		// Classification-only events stay internal; sendOutbound still runs so
		// the suppression is audited with a reason instead of vanishing.
		result := m.sendOutboundResult(ctx, rc, OutboundMessage{
			Scene: in.Scene, ConversationID: in.ConversationID,
			ReplyToMessageID: in.MessageID, Text: text,
			Envelope: progressEnvelope(rc, in, ev),
		})
		// A delivered milestone is the turn's first meaningful response; the
		// final summary must not repeat it verbatim.
		if result.Sent {
			m.markReplied(scope)
		}
	}
	reply, err := m.handleTurn(turnCtx, resolved, in, onProgress)
	stopWaiting()
	if err != nil {
		// Running out of time is not a failure, but it is not a delegation
		// either: nothing was started and nothing is still running. Saying "I'll
		// keep at it in the background" here would be a promise the platform
		// cannot keep, so the user is offered the delegation instead — their
		// next message turns it into a real Run.
		if turnCtx.Err() != nil && ctx.Err() == nil {
			log.Info().Str("channel", rc.cfg.ID).Dur("timeout", timeout).
				Msg("live: foreground turn exceeded its budget without an answer")
			if !m.hasReplied(scope) {
				m.sendOutbound(ctx, rc, OutboundMessage{
					Scene: in.Scene, ConversationID: in.ConversationID, ReplyToMessageID: in.MessageID,
					Text:     turnTooSlowText(services.DetectLanguage(in.Text, "")),
					Envelope: turnEnvelope(rc, in, sendable.KindFinal, "turn_budget_exceeded", sendable.PriorityHigh),
				})
			}
			return
		}
		m.sendTurnFailure(ctx, rc, in, scope, err)
		return
	}

	summary, reason, ok := deliverableFinalText(reply)
	if !ok {
		// The agent finished without submitting anything the user can read. If
		// something substantive already went out this turn, staying quiet is the
		// honest outcome; otherwise say so in terms the user can act on.
		if m.hasReplied(scope) {
			return
		}
		m.sendOutbound(ctx, rc, OutboundMessage{
			Scene: in.Scene, ConversationID: in.ConversationID, ReplyToMessageID: in.MessageID,
			Text:     m.noAnswerFallback(rc, in),
			Envelope: turnEnvelope(rc, in, sendable.KindFinal, "final_missing_fallback", sendable.PriorityCritical),
		})
		return
	}
	m.sendOutbound(ctx, rc, OutboundMessage{
		Scene: in.Scene, ConversationID: in.ConversationID, ReplyToMessageID: in.MessageID,
		Text: summary, ImageURLs: reply.ImageURLs,
		Envelope: func() sendable.DeliveryEnvelope {
			e := turnEnvelope(rc, in, sendable.KindFinal, reason, sendable.PriorityCritical)
			e.Structured = true
			return e
		}(),
	})
}

// sandboxOpenBudget is the cold-start allowance for this Manager.
func (m *Manager) sandboxOpenBudget() time.Duration {
	if m.openBudget < 0 {
		return 0
	}
	if m.openBudget > 0 {
		return m.openBudget
	}
	return sandboxOpenBudget
}

// sayStillWorking breaks a long silence once, and returns a function that
// cancels it.
//
// It exists for the one case the streaming opener cannot cover: a cold start,
// where there is no sandbox yet and therefore no text to release early. Leaving
// the user staring at nothing for the length of a container boot is its own
// kind of bad. This is not the mechanical acknowledgement that was removed —
// that one preceded every message including instant ones; this fires only after
// a genuinely long wait, at most once, and never when anything has been said.
func (m *Manager) sayStillWorking(ctx context.Context, rc *runningChannel, in InboundMessage, scope string) (stop func()) {
	done := make(chan struct{})
	var once sync.Once
	stop = func() { once.Do(func() { close(done) }) }
	delay := stillWorkingDelay
	if m.stillWorkingAfter > 0 {
		delay = m.stillWorkingAfter
	}
	go func() {
		timer := time.NewTimer(delay)
		defer timer.Stop()
		select {
		case <-done:
			return
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		if m.hasReplied(scope) {
			return
		}
		envelope := turnEnvelope(rc, in, sendable.KindProgress, "live_still_working", sendable.PriorityNormal)
		envelope.Progress = sendable.ProgressFields{Stage: "live_still_working"}
		result := m.sendOutboundResult(ctx, rc, OutboundMessage{
			Scene: in.Scene, ConversationID: in.ConversationID, ReplyToMessageID: in.MessageID,
			Text:     stillWorkingText(services.DetectLanguage(in.Text, "")),
			Envelope: envelope,
		})
		if result.Sent {
			m.markReplied(scope)
		}
	}()
	return stop
}

// stillWorkingText admits the wait without pretending to report progress.
func stillWorkingText(language string) string {
	if services.NormalizeLanguage(language) == "en" {
		return "Give me a moment on this one."
	}
	return "稍等，我看一下。"
}

// sendTurnFailure reports a failed turn in the user's terms. Internal error
// strings ("assistant produced no reply", sandbox/ACP plumbing) never leave the
// platform; they are logged and mapped to a cause plus a next step.
func (m *Manager) sendTurnFailure(ctx context.Context, rc *runningChannel, in InboundMessage, scope string, err error) {
	log.Warn().Err(err).Str("channel", rc.cfg.ID).Msg("channel turn failed")
	if m.hasReplied(scope) {
		return
	}
	m.sendOutbound(ctx, rc, OutboundMessage{
		Scene: in.Scene, ConversationID: in.ConversationID,
		ReplyToMessageID: in.MessageID, Text: turnFailureText(err),
		Envelope: turnEnvelope(rc, in, sendable.KindBlocked, "turn_failed", sendable.PriorityCritical),
	})
}

// deprecatedSafeFinalNotice is the #157 fake-completion string: a turn that
// produced nothing used to report success and send the user elsewhere. It is
// kept only so tests can assert it never comes back.
const deprecatedSafeFinalNotice = "本回合已结束，请在 Approving 查看完整结果。"

// deliverableFinalText returns the text allowed out of a finished turn. Only an
// explicitly submitted summary qualifies; Reply.Text stays internal regardless
// of its content. ok=false means the turn produced nothing sendable and the
// caller must fall back rather than emit a placeholder.
func deliverableFinalText(reply Reply) (text, reason string, ok bool) {
	if summary := strings.TrimSpace(reply.FinalSummary); summary != "" {
		return truncateRunes(summary, 240), "structured_turn_final", true
	}
	return "", "final_summary_missing", false
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
	return conversationTurnScope(rc.cfg.ProjectID, in.Scene, in.ConversationID)
}

// conversationTurnScope keys the in-flight turn by conversation. The MCP host
// knows which conversation it is serving but not which inbound message id
// started the turn, so the "already replied" marker is tracked per conversation
// and both spellings resolve to the same live turn.
func conversationTurnScope(projectID string, scene Scene, conversationID string) string {
	return "turn:" + convKey(projectID, scene, conversationID)
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
		// Turn-level text is composed by the platform (or extracted through an
		// explicit structured path), never copied from raw model output, which
		// is what the transport's Structured gate is checking for.
		Structured: true,
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
	// Last gate before the transport. Every outbound path converges here, so
	// scrubbing once here is what makes "internals never leak" a property of the
	// system rather than a rule each call site has to remember.
	if scrubbed := ScrubInternalTerms(out.Text); scrubbed != out.Text {
		log.Debug().Str("channel", rc.cfg.ID).Str("reason", out.Envelope.Reason).
			Msg("outbound text scrubbed of internal terms")
		out.Text = scrubbed
	}
	if strings.TrimSpace(out.Text) == "" && len(out.ImageURLs) == 0 {
		return DeliveryResult{Decision: sendable.Decision{Reason: "empty_after_scrub"}}
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

// markReplied records that something substantive already reached the user in
// this turn. pm_reply runs in the MCP host and the final summary runs here, so
// without a shared marker a turn can answer twice.
func (m *Manager) markReplied(scope string) {
	if strings.TrimSpace(scope) == "" {
		return
	}
	m.repliedMu.Lock()
	m.replied[scope] = true
	m.repliedMu.Unlock()
}

func (m *Manager) hasReplied(scope string) bool {
	m.repliedMu.Lock()
	defer m.repliedMu.Unlock()
	return m.replied[scope]
}

func (m *Manager) clearReplied(scope string) {
	m.repliedMu.Lock()
	delete(m.replied, scope)
	m.repliedMu.Unlock()
}

// MarkConversationReplied lets the MCP host record an explicit agent reply for
// the conversation's current turn.
func (m *Manager) MarkConversationReplied(projectID string, scene Scene, conversationID string) {
	m.markReplied(conversationTurnScope(projectID, scene, conversationID))
}

// noAnswerFallback is what the user hears when a turn ends without the agent
// submitting an answer. It names the task in flight and the next step, because
// "go look somewhere else" is not an answer.
func (m *Manager) noAnswerFallback(rc *runningChannel, in InboundMessage) string {
	language := services.DetectLanguage(in.Text, "")
	if task := m.activeTaskFor(rc, in); task != nil {
		if language == "en" {
			return "I'm still on \"" + task.ShortTitle + "\" and don't have a usable answer yet. I'll come back as soon as there's something concrete."
		}
		return "「" + task.ShortTitle + "」我还在弄，暂时没有可用的结论。有实质进展我就回来说。"
	}
	if language == "en" {
		return "I couldn't put together an answer for that one. Could you rephrase it, or tell me which part matters most?"
	}
	return "这条我没能给出结论。你可以换个说法，或者告诉我最关心哪部分。"
}

// activeTaskFor returns the conversation's focused task, if any.
func (m *Manager) activeTaskFor(rc *runningChannel, in InboundMessage) *models.TaskIdentity {
	if m.taskContext == nil || rc == nil {
		return nil
	}
	res, err := m.taskContext.ResolveTask(services.ResolveTaskInput{
		Scope: services.TaskScope{
			ProjectID: rc.cfg.ProjectID, UserID: services.SyntheticQQUserID(in.UserID),
			Channel: rc.cfg.Type, ConversationID: in.ConversationID,
		},
	})
	if err != nil {
		return nil
	}
	return res.Identity
}

// turnHandoffText is what the user hears when a foreground turn outlives its
// budget: the work continues, the conversation does not wait for it.
// turnTooSlowText handles a turn that ran out of time without answering.
//
// It offers to delegate rather than claiming to have done so. Nothing is
// running at this point, and a message that says otherwise leaves the user
// waiting for a result that will never arrive — worse than the mechanical
// acknowledgement it would have replaced. Answering yes makes it a real Run on
// the next turn.
func turnTooSlowText(language string) string {
	if services.NormalizeLanguage(language) == "en" {
		return "This is more than I can work out while we're talking. Want me to take it on as a background task and come back with the result?"
	}
	return "这个我一时半会儿聊不完。要不要我当成一个后台任务来做，做完了告诉你？"
}

// turnFailureText maps an internal turn error onto a cause the user can act on.
func turnFailureText(err error) string {
	if err == nil {
		return "这条我没处理成功，你再发一次试试。"
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "context deadline exceeded"), strings.Contains(msg, "超时"),
		strings.Contains(msg, "timeout"):
		return "这条想得有点久，我先放到后台继续，有结果告诉你。"
	case strings.Contains(msg, "沙箱"), strings.Contains(msg, "sandbox"):
		return "我的执行环境暂时起不来，稍后再试一次；一直不行就需要管理员看一下。"
	case strings.Contains(msg, "no reply"), strings.Contains(msg, "empty"):
		return "这条我没能给出结论。换个说法我再试试。"
	case strings.Contains(msg, "未启用"), strings.Contains(msg, "disabled"):
		return "这个项目还没开启对话能力，需要管理员在后台启用。"
	default:
		return "这条我没处理成功。你可以再说一次，或者换个问法。"
	}
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