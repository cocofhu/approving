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
	"github.com/cocofhu/approving/internal/services"

	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
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

	progressMu      sync.Mutex
	pendingProgress map[string]*pendingProgressDelivery

	baseCtx context.Context

	deliveryPolicy *deliveryPolicy
	deliveryDB     *gorm.DB
	deliveryAudit  *services.ProjectAuditService
	taskContext    *services.TaskContextService

	// Test hooks (production leaves these nil/zero):
	// handleFunc overrides bridge.Handle when set (no progress callback).
	handleFunc func(ctx context.Context, rc ResolvedChannel, in InboundMessage) (Reply, error)
	// handleFuncWithProgress overrides bridge.Handle when set.
	handleFuncWithProgress func(ctx context.Context, rc ResolvedChannel, in InboundMessage, onProgress func(ProgressEvent)) (Reply, error)
}

func (m *Manager) SetTaskContext(tasks *services.TaskContextService) {
	m.taskContext = tasks
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
	m := &Manager{
		bridge:          bridge,
		factories:       factories,
		decrypt:         decrypt,
		running:         map[string]*runningChannel{},
		convQueues:      map[string]*convQueue{},
		pushQueues:      map[string]*pushQueue{},
		pendingProgress: map[string]*pendingProgressDelivery{},
		baseCtx:         context.Background(),
		deliveryPolicy:  newDeliveryPolicy(),
	}
	if bridge != nil {
		bridge.SetRunAcceptanceHook(m.onRunAccepted)
	}
	return m
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
	m.progressMu.Lock()
	for key, pending := range m.pendingProgress {
		if pending.timer != nil {
			pending.timer.Stop()
		}
		delete(m.pendingProgress, key)
	}
	m.progressMu.Unlock()
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
	if text, ok := in.Raw["system_rejection"].(string); ok && strings.TrimSpace(text) != "" {
		env := m.turnEnvelope(rc, in, ReasonFailure, "safe_status", "system-rejection")
		env.Priority = PriorityImmediate
		_ = m.AppendSendable(ctx, rc, OutboundMessage{
			Scene: in.Scene, ConversationID: in.ConversationID,
			ReplyToMessageID: in.MessageID, Text: text,
		}, env)
		return
	}
	key := convKey(rc.cfg.ProjectID, in.Scene, in.ConversationID)
	q := m.convQueueFor(key)

	q.mu.Lock()
	if q.busy {
		if len(q.pending) >= convQueueDepth {
			q.mu.Unlock()
			_ = m.AppendSendable(ctx, rc, OutboundMessage{
				Scene: in.Scene, ConversationID: in.ConversationID,
				ReplyToMessageID: in.MessageID, Text: queueFullText,
			}, m.turnEnvelope(rc, in, ReasonQueue, "queue_full", "queue-full"))
			return
		}
		// Ahead = in-flight turn + already-queued messages.
		ahead := 1 + len(q.pending)
		q.pending = append(q.pending, queuedInbound{ctx: ctx, rc: rc, in: in})
		q.mu.Unlock()
		_ = m.AppendSendable(ctx, rc, OutboundMessage{
			Scene: in.Scene, ConversationID: in.ConversationID,
			ReplyToMessageID: in.MessageID, Text: queueAckTextFor(ahead, in.Text),
		}, m.turnEnvelope(rc, in, ReasonQueue, "queue_ack", fmt.Sprintf("queue-%d", ahead)))
		return
	}
	q.busy = true
	q.mu.Unlock()

	// This goroutine owns the busy cycle: run the idle-first turn, then drain.
	m.runTurn(ctx, rc, in, true /* withProcessingAck */)
	m.drainConvQueue(q, key)
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
		_ = m.AppendSendable(ctx, rc, OutboundMessage{
			Scene: in.Scene, ConversationID: in.ConversationID,
			ReplyToMessageID: in.MessageID, Text: processingAckText(in.Text),
		}, m.turnEnvelope(rc, in, ReasonTurnProcessingACK, "turn_ack", "processing"))
	}

	resolved := ResolvedChannel{
		ID: rc.cfg.ID, Type: rc.cfg.Type, ProjectID: rc.cfg.ProjectID,
		TurnTimeout: time.Duration(rc.cfg.TurnTimeoutSeconds) * time.Second,
		Caps:        SessionCapsFromConfig(rc.cfg.Config),
	}
	if capable, ok := rc.adapter.(CapabilityAdapter); ok {
		resolved.ReplyMetadata = capable.Capabilities().ReplyMetadata
	}
	reply, err := m.handleTurn(ctx, resolved, in, m.turnProgressHandler(ctx, rc, in))
	if err != nil {
		log.Warn().Err(err).Str("channel", rc.cfg.ID).Msg("channel turn failed")
		env := m.turnEnvelope(rc, in, ReasonFailure, "safe_status", "failure")
		env.Priority = PriorityImmediate
		_ = m.AppendSendable(ctx, rc, OutboundMessage{
			Scene: in.Scene, ConversationID: in.ConversationID,
			ReplyToMessageID: in.MessageID, Text: failReplyPrefix + friendlyErr(err),
		}, env)
		return
	}
	runID := strings.TrimSpace(reply.RunID)
	if runID == "" {
		runID = turnContextRunID(in)
	}
	summary := "处理完成，请在项目中查看结果。"
	// Attachments ride on the structured final report only; raw assistant text
	// never authorizes an outbound image.
	var images []string
	if reply.Final != nil {
		images = reply.Final.ImageURLs
	}
	if reply.Final != nil && strings.TrimSpace(reply.Final.Summary) != "" {
		summary = strings.TrimSpace(reply.Final.Summary)
	} else if (m.handleFunc != nil || m.handleFuncWithProgress != nil) && strings.TrimSpace(reply.Text) != "" {
		// Test hooks are trusted application producers, never live agent output.
		summary = strings.TrimSpace(reply.Text)
	}
	if reply.ShortTitle != "" && !resolved.ReplyMetadata && !strings.HasPrefix(summary, "【") {
		summary = TaskMessagePrefix(reply.ShortTitle, IMTypeLabel(string(ReasonFinal), DetectIMLanguage(summary, ""))) + summary
	}
	env := m.turnEnvelope(rc, in, ReasonFinal, DeliveryTypeStructuredSummary, "final")
	env.Context.RunID, env.Context.ShortTitle, env.Priority = runID, reply.ShortTitle, PriorityImmediate
	_ = m.AppendSendable(ctx, rc, OutboundMessage{
		Scene: in.Scene, ConversationID: in.ConversationID, ReplyToMessageID: in.MessageID,
		Text: summary, ImageURLs: images,
	}, env)
}

// turnProgressHandler is the production Work→Reply progress sink. Only events
// produced by NewSendableProgressEvent (structured server state) are eligible
// for egress; classified assistant text stays internal.
func (m *Manager) turnProgressHandler(ctx context.Context, rc *runningChannel, in InboundMessage) func(ProgressEvent) {
	return func(ev ProgressEvent) {
		text := FormatProgressText(ev)
		if text == "" {
			return
		}
		if !ev.authorized && m.handleFuncWithProgress == nil {
			m.AppendInternal(Envelope{
				Delivery: DeliveryInternal, Reason: ReasonProgress, Type: "classified_agent_text",
				Context: RunTaskContext{RunID: ev.RunID, ProjectID: rc.cfg.ProjectID, UserID: in.UserID},
			})
			return
		}
		runID := strings.TrimSpace(ev.RunID)
		if runID == "" {
			runID = turnContextRunID(in)
		}
		reason := ReasonProgress
		priority := PriorityOrdinary
		eventType := DeliveryTypeStage
		if ev.Kind == ProgressBlocker {
			reason, priority = ReasonBlocked, PriorityImmediate
			eventType = DeliveryTypeBlocked
		} else if ev.Kind == ProgressConfirm {
			reason, priority = ReasonActionRequired, PriorityImmediate
			eventType = DeliveryTypeActionRequired
		}
		env := m.turnEnvelope(rc, in, reason, eventType, "progress-"+string(ev.Kind)+"-"+deliveryKey(text))
		env.Context.RunID, env.Priority = runID, priority
		_ = m.AppendSendable(ctx, rc, OutboundMessage{
			Scene: in.Scene, ConversationID: in.ConversationID,
			ReplyToMessageID: in.MessageID, Text: text,
		}, env)
	}
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

func (m *Manager) turnEnvelope(rc *runningChannel, in InboundMessage, reason DeliveryReason, typ, suffix string) Envelope {
	projectID, channel := "", ""
	if rc != nil {
		projectID, channel = rc.cfg.ProjectID, rc.cfg.Type
	}
	return Envelope{
		Channels: []string{channel}, Priority: PriorityOrdinary, Reason: reason, Type: typ,
		Context:   RunTaskContext{RunID: turnContextRunID(in), ProjectID: projectID, UserID: in.UserID},
		DedupeKey: strings.Join([]string{"turn", projectID, channel, string(in.Scene), in.ConversationID, in.MessageID, suffix}, ":"),
	}
}

func turnContextRunID(in InboundMessage) string {
	if strings.TrimSpace(in.MessageID) != "" {
		return "turn:" + in.MessageID
	}
	return "turn:" + deliveryKey(string(in.Scene)+"|"+in.ConversationID+"|"+in.Text)
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
		Text:     text,
		Enqueued: time.Now(),
		Envelope: Envelope{
			Channels: []string{target.cfg.Type}, Reason: ReasonCron, Type: "structured_cron",
			Context:   RunTaskContext{ProjectID: d.ProjectID},
			DedupeKey: "cron:" + d.ProjectID + ":" + deliveryKey(d.Category+"|"+string(kind)+"|"+text),
		},
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
	return m.deliverRunNotifyContext(projectID, "", "", "", text)
}

// DeliverRunNotifyContext carries Run identity into the first-class envelope
// and applies the QQ natural-language metadata fallback.
func (m *Manager) DeliverRunNotifyContext(projectID, runID, shortTitle, kind, text string) error {
	if strings.TrimSpace(shortTitle) != "" {
		text = TaskMessagePrefix(shortTitle, IMTypeLabel(kind, DetectIMLanguage(text, ""))) + text
	}
	return m.deliverRunNotifyContext(projectID, runID, shortTitle, kind, text)
}

func (m *Manager) deliverRunNotifyContext(projectID, runID, shortTitle, kind, text string) error {
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
		Enqueued:  time.Now(),
		Envelope: Envelope{
			Channels: []string{"qq"}, Priority: PriorityImmediate, Reason: ReasonRunNotify,
			Type: "structured_run_notify",
			Context: RunTaskContext{
				ProjectID: projectID, RunID: runID, ShortTitle: shortTitle,
			},
			DedupeKey: "run-notify:" + projectID + ":" + deliveryKey(text),
		},
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

// DeliverRunAcceptance emits the once-per-run acceptance ACK. It is distinct
// from the per-turn processing ACK used by dispatch.
func (m *Manager) DeliverRunAcceptance(projectID, runID, userID, shortTitle, text string) error {
	target, scene, conv, err := m.lookupRunNotifyTarget(projectID)
	if err != nil {
		return err
	}
	return m.deliverRunAcceptance(target, scene, conv, runID, userID, shortTitle, text)
}

// onRunAccepted is the production lifecycle hook: the inbound orchestration
// calls it the first time a conversation is associated with a Run. Identity is
// stable (run + channel + scene/conversation + external user) so both the
// in-memory policy and the persistent receipt dedupe reconnects.
func (m *Manager) onRunAccepted(a RunAcceptance) {
	m.mu.Lock()
	var target *runningChannel
	for _, rc := range m.running {
		if rc.cfg.ProjectID == a.ProjectID && rc.cfg.Type == a.Channel {
			if target == nil || rc.cfg.UpdatedAt.After(target.cfg.UpdatedAt) {
				target = rc
			}
		}
	}
	m.mu.Unlock()
	if target == nil {
		return
	}
	text := "任务已接收，稍后同步进展。"
	if a.Language == "en" {
		text = "Task accepted; updates will follow."
	}
	err := m.deliverRunAcceptance(target, a.Scene, a.ConversationID, a.RunID, a.UserID, a.ShortTitle, text)
	if err != nil && !errors.Is(err, ErrDeliverySuppressed) {
		log.Warn().Err(err).Str("run", a.RunID).Msg("run acceptance ack failed")
	}
}

func (m *Manager) deliverRunAcceptance(target *runningChannel, scene Scene, conv, runID, userID, shortTitle, text string) error {
	if strings.TrimSpace(text) == "" {
		text = "任务已接收。"
	}
	text = TaskMessagePrefix(shortTitle, IMTypeLabel(string(ReasonRunAcceptanceACK), DetectIMLanguage(text, ""))) + text
	env := Envelope{
		Channels: []string{target.cfg.Type}, Priority: PriorityImmediate,
		Reason: ReasonRunAcceptanceACK, Type: "run_acceptance",
		Context: RunTaskContext{
			RunID: runID, ShortTitle: shortTitle, ProjectID: target.cfg.ProjectID, UserID: userID,
		},
		DedupeKey: strings.Join([]string{
			"run-ack", target.cfg.ProjectID, runID, userID, target.cfg.Type, string(scene), conv,
		}, ":"),
	}
	ctx, cancel := context.WithTimeout(m.baseCtx, 60*time.Second)
	defer cancel()
	return m.AppendSendable(ctx, target, OutboundMessage{Scene: scene, ConversationID: conv, Text: text}, env)
}

// AppendRunUpdate is the explicit application path for structured Run
// progress, blocked/action-required, and final updates.
func (m *Manager) AppendRunUpdate(projectID, channelID, sceneText, conversationID, userID, runID, shortTitle string, reason DeliveryReason, summary string) error {
	m.mu.Lock()
	target := m.running[channelID]
	m.mu.Unlock()
	if target == nil || target.cfg.ProjectID != projectID {
		return ErrNoRunNotifyTarget
	}
	scene := Scene(sceneText)
	typ := DeliveryTypeStage
	priority := PriorityOrdinary
	if reason == ReasonFinal {
		typ, priority = DeliveryTypeStructuredSummary, PriorityImmediate
	} else if reason == ReasonBlocked {
		typ, priority = DeliveryTypeBlocked, PriorityImmediate
	} else if reason == ReasonActionRequired {
		typ, priority = DeliveryTypeActionRequired, PriorityImmediate
	} else if reason != ReasonProgress {
		return ErrDeliverySuppressed
	}
	env := Envelope{
		Channels: []string{target.cfg.Type}, Priority: priority, Reason: reason, Type: typ,
		Context:   RunTaskContext{RunID: runID, ShortTitle: shortTitle, ProjectID: projectID, UserID: userID},
		DedupeKey: strings.Join([]string{"run-update", projectID, runID, string(reason), deliveryKey(summary)}, ":"),
	}
	summary = TaskMessagePrefix(shortTitle, IMTypeLabel(string(reason), DetectIMLanguage(summary, ""))) + strings.TrimSpace(summary)
	ctx, cancel := context.WithTimeout(m.baseCtx, 60*time.Second)
	defer cancel()
	return m.AppendSendable(ctx, target, OutboundMessage{Scene: scene, ConversationID: conversationID, Text: summary}, env)
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
		if err := m.AppendSendable(ctx, target, OutboundMessage{
			Scene: item.Scene, ConversationID: item.Conv, Text: stripped, ImageURLs: urls,
		}, item.Envelope); err != nil && !errors.Is(err, ErrDeliverySuppressed) {
			log.Warn().Err(err).Str("project", item.ProjectID).Msg("cron push flush send failed")
		}
		cancel()
	}
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
