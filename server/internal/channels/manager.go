package channels

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/cocofhu/approving/internal/models"

	"github.com/rs/zerolog/log"
)

// ErrNoDeliveryChannel is returned by Deliver when no enabled channel for the
// project is configured as the cron delivery target.
var ErrNoDeliveryChannel = errors.New("项目未配置定时任务推送渠道")

const (
	delayedAckText  = "收到，正在确认…"
	defaultAckDelay = 3 * time.Second
	failReplyPrefix = "处理失败："
	queueAckText    = "已收到，排队中"
	queueFullText   = "队列已满，请稍候"
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
	ackSent bool // queue ACK already sent in this busy cycle
}

// Manager owns the lifecycle of channel adapters. Configs are supplied by a
// loader (backed by the DB) and applied idempotently: adapters are started,
// stopped, or restarted based on a config fingerprint so admin edits hot-reload
// without a server restart.
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

	baseCtx context.Context

	// Test hooks (production leaves these nil/zero):
	// handleFunc overrides bridge.Handle when set.
	handleFunc func(ctx context.Context, rc ResolvedChannel, in InboundMessage) (Reply, error)
	// ackDelay overrides the 3s delayed-ack threshold when > 0.
	ackDelay time.Duration
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
		baseCtx:    context.Background(),
	}
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

// dispatch serializes messages per conversation via a bounded in-process FIFO.
// Idle + empty queue: run immediately (with delayedAck). Busy: enqueue up to
// convQueueDepth; first enqueue in a busy cycle gets a throttled queue ACK.
// Full queue: reject with a visible reply (never silently drop).
func (m *Manager) dispatch(ctx context.Context, rc *runningChannel, in InboundMessage) {
	key := rc.cfg.ProjectID + "|" + string(in.Scene) + "|" + in.ConversationID
	q := m.convQueueFor(key)

	q.mu.Lock()
	if q.busy {
		if len(q.pending) >= convQueueDepth {
			q.mu.Unlock()
			if err := rc.adapter.Send(ctx, OutboundMessage{
				Scene: in.Scene, ConversationID: in.ConversationID,
				ReplyToMessageID: in.MessageID, Text: queueFullText,
			}); err != nil {
				log.Warn().Err(err).Str("channel", rc.cfg.ID).Msg("channel queue-full reply send failed")
			}
			return
		}
		q.pending = append(q.pending, queuedInbound{ctx: ctx, rc: rc, in: in})
		sendAck := !q.ackSent
		if sendAck {
			q.ackSent = true
		}
		q.mu.Unlock()
		if sendAck {
			if err := rc.adapter.Send(ctx, OutboundMessage{
				Scene: in.Scene, ConversationID: in.ConversationID,
				ReplyToMessageID: in.MessageID, Text: queueAckText,
			}); err != nil {
				log.Warn().Err(err).Str("channel", rc.cfg.ID).Msg("channel queue ack send failed")
			}
		}
		return
	}
	q.busy = true
	q.mu.Unlock()

	// This goroutine owns the busy cycle: run the idle-first turn, then drain.
	m.runTurn(ctx, rc, in, true /* withDelayedAck */)
	m.drainConvQueue(q)
}

// drainConvQueue runs pending messages in arrival order until the queue is
// empty, then clears the busy-cycle ACK flag. Queued turns never get
// delayedAck or a fresh queue ACK.
func (m *Manager) drainConvQueue(q *convQueue) {
	for {
		q.mu.Lock()
		if len(q.pending) == 0 {
			q.busy = false
			q.ackSent = false
			q.mu.Unlock()
			return
		}
		next := q.pending[0]
		q.pending = q.pending[1:]
		q.mu.Unlock()
		m.runTurn(next.ctx, next.rc, next.in, false /* withDelayedAck */)
	}
}

// runTurn executes one PM turn and sends the final or failure reply.
// withDelayedAck is true only for the idle-first path (not dequeue continuations).
func (m *Manager) runTurn(ctx context.Context, rc *runningChannel, in InboundMessage, withDelayedAck bool) {
	var cancelAck func()
	if withDelayedAck {
		cancelAck = startDelayedAck(m.ackDelayOrDefault(), func() {
			if err := rc.adapter.Send(ctx, OutboundMessage{
				Scene: in.Scene, ConversationID: in.ConversationID,
				ReplyToMessageID: in.MessageID, Text: delayedAckText,
			}); err != nil {
				log.Warn().Err(err).Str("channel", rc.cfg.ID).Msg("channel delayed ack send failed")
			}
		})
		defer cancelAck()
	}

	resolved := ResolvedChannel{
		ID: rc.cfg.ID, Type: rc.cfg.Type, ProjectID: rc.cfg.ProjectID,
		TurnTimeout: time.Duration(rc.cfg.TurnTimeoutSeconds) * time.Second,
		Caps:        SessionCapsFromConfig(rc.cfg.Config),
	}
	reply, err := m.handleTurn(ctx, resolved, in)
	if withDelayedAck {
		cancelAck()
	}
	if err != nil {
		log.Warn().Err(err).Str("channel", rc.cfg.ID).Msg("channel turn failed")
		if sendErr := rc.adapter.Send(ctx, OutboundMessage{
			Scene: in.Scene, ConversationID: in.ConversationID,
			ReplyToMessageID: in.MessageID, Text: failReplyPrefix + friendlyErr(err),
		}); sendErr != nil {
			log.Warn().Err(sendErr).Str("channel", rc.cfg.ID).Msg("channel error reply send failed")
		}
		return
	}
	if err := rc.adapter.Send(ctx, OutboundMessage{
		Scene: in.Scene, ConversationID: in.ConversationID, ReplyToMessageID: in.MessageID,
		Text: reply.Text, ImageURLs: reply.ImageURLs,
	}); err != nil {
		log.Warn().Err(err).Str("channel", rc.cfg.ID).Msg("channel reply send failed")
	}
}

func (m *Manager) handleTurn(ctx context.Context, rc ResolvedChannel, in InboundMessage) (Reply, error) {
	if m.handleFunc != nil {
		return m.handleFunc(ctx, rc, in)
	}
	return m.bridge.Handle(ctx, rc, in)
}

func (m *Manager) ackDelayOrDefault() time.Duration {
	if m.ackDelay > 0 {
		return m.ackDelay
	}
	return defaultAckDelay
}

// startDelayedAck schedules send once after delay. cancel suppresses the send
// if the turn finishes first; safe to call multiple times.
func startDelayedAck(delay time.Duration, send func()) (cancel func()) {
	var mu sync.Mutex
	done := false
	timer := time.AfterFunc(delay, func() {
		mu.Lock()
		if done {
			mu.Unlock()
			return
		}
		done = true
		mu.Unlock()
		send()
	})
	return func() {
		mu.Lock()
		done = true
		mu.Unlock()
		timer.Stop()
	}
}

// Deliver pushes cron/proactive text to a project's configured delivery channel.
// Implements the services.ChannelDeliverer interface.
func (m *Manager) Deliver(projectID, text string) error {
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
		return ErrNoDeliveryChannel
	}
	scene, conv := parseTarget(target.cfg.CronDeliverTarget)
	if conv == "" {
		return ErrNoDeliveryChannel
	}
	stripped, urls := splitImageURLs(text)
	ctx, cancel := context.WithTimeout(m.baseCtx, 60*time.Second)
	defer cancel()
	return target.adapter.Send(ctx, OutboundMessage{
		Scene: scene, ConversationID: conv, Text: stripped, ImageURLs: urls,
	})
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
