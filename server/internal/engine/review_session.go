package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/cocofhu/approving/internal/blob"
	"github.com/cocofhu/approving/internal/mcp"
	"github.com/cocofhu/approving/internal/models"
	"github.com/cocofhu/approving/internal/runtime"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

// MaxReviewQueueItems caps pending (not-yet-started) review/clarify turns per
// parked producer session. Aligned with the sandbox-gateway bridge PromptQueue
// (MaxPromptQueueItems=32). The platform FIFO is authoritative for UI waiting.
// Review Cancel clears this queue; clarify Cancel keeps it and pumps the next item.
const MaxReviewQueueItems = 32

// sessionKind distinguishes post-run review (ReviseInPlace) from classic react
// clarify (ReactReply). Both share FIFO / WS frames; Cancel policy forks.
type sessionKind string

const (
	sessionKindReview  sessionKind = "review"
	sessionKindClarify sessionKind = "clarify"
)

// reviewQueueItem is one not-yet-started (or active) turn on a producer
// session. Source distinguishes node-inline review vs gate hot-revise for
// turn_begin/gate BodyMd refresh; both share the same FIFO.
type reviewQueueItem struct {
	ID          string
	Text        string // raw human text (persisted on turn_begin)
	Effective   string // annotations folded (sent to agent)
	Images      []models.PromptImage
	Annotations []models.ReactAnnotation
	Source      string // "node" | "gate"
	GateNodeID  string
	Force       bool // clarify force wrap-up (rare via enqueue; normally sync)
}

// reviewSession is the platform-authoritative controller for one parked
// producer ACP session: FIFO pending queue + single worker pump, mirroring
// SandboxChat's per-conn queue/worker model. Used by both review and clarify.
type reviewSession struct {
	runID      string
	producerID string
	kind       sessionKind

	mu       sync.Mutex
	queue    []*reviewQueueItem
	waiting  int
	active   *reviewQueueItem
	cancelFn context.CancelFunc
	pumping  bool
	// cancelRequested is set by Cancel; the active turn saves partial narration
	// as interrupted when the provider returns.
	cancelRequested bool
}

func (e *Engine) reviewSessionKey(runID, producerID string) string {
	return runID + "|" + producerID
}

func (e *Engine) getOrCreateReviewSession(runID, producerID string, kind sessionKind) *reviewSession {
	key := e.reviewSessionKey(runID, producerID)
	e.reviewMu.Lock()
	defer e.reviewMu.Unlock()
	if e.reviewSess == nil {
		e.reviewSess = map[string]*reviewSession{}
	}
	s := e.reviewSess[key]
	if s == nil {
		s = &reviewSession{runID: runID, producerID: producerID, kind: kind}
		e.reviewSess[key] = s
	} else if s.kind == "" {
		s.kind = kind
	}
	return s
}

func (e *Engine) dropReviewSessionIfIdle(runID, producerID string) {
	key := e.reviewSessionKey(runID, producerID)
	e.reviewMu.Lock()
	defer e.reviewMu.Unlock()
	s := e.reviewSess[key]
	if s == nil {
		return
	}
	s.mu.Lock()
	idle := s.active == nil && s.waiting == 0 && !s.pumping
	s.mu.Unlock()
	if idle {
		delete(e.reviewSess, key)
	}
}

// HasLiveReviewSession reports whether a parked ACP review session is still alive
// (hot ReAct). Distinct from ReviewSessionReady, which is true both when idle-hot
// and when no session exists (cold).
func (e *Engine) HasLiveReviewSession(runID, nodeID string) bool {
	rp, ok := e.provider.(runtime.ReviewProvider)
	if !ok {
		return false
	}
	return rp.HasLiveSession(runID, nodeID)
}

// ReviewSessionReady reports whether the producer session has no in-flight turn
// and an empty pending queue (FR4 ready gate for force confirm).
func (e *Engine) ReviewSessionReady(runID, producerID string) bool {
	e.reviewMu.Lock()
	s := e.reviewSess[e.reviewSessionKey(runID, producerID)]
	e.reviewMu.Unlock()
	if s == nil {
		return true
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.active == nil && s.waiting == 0
}

// ReviewSessionState returns waiting count and whether a turn is in flight.
func (e *Engine) ReviewSessionState(runID, producerID string) (waiting int, thinking bool) {
	snap, ok := e.ReviewSessionSnapshotFor(runID, producerID)
	if !ok {
		return 0, false
	}
	return snap.Waiting, snap.Busy
}

// ReviewSessionSnapshotFor returns the leak-safe queue snapshot for one parked session.
func (e *Engine) ReviewSessionSnapshotFor(runID, producerID string) (ReviewSessionSnapshot, bool) {
	e.reviewMu.Lock()
	s := e.reviewSess[e.reviewSessionKey(runID, producerID)]
	e.reviewMu.Unlock()
	if s == nil {
		return ReviewSessionSnapshot{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	snap := ReviewSessionSnapshot{
		NodeID:  s.producerID,
		Kind:    string(s.kind),
		Waiting: s.waiting,
		Busy:    s.active != nil,
		Items:   s.queueSnapshotLocked(),
	}
	if s.active != nil {
		snap.ActiveItem = map[string]any{
			"id":          s.active.ID,
			"text":        s.active.Text,
			"annotations": s.active.Annotations,
		}
	}
	return snap, true
}

// ReviewSessionSnapshot is the refresh-resume DTO for one parked session.
type ReviewSessionSnapshot struct {
	NodeID     string           `json:"nodeId"`
	Kind       string           `json:"kind"`
	Waiting    int              `json:"waiting"`
	Busy       bool             `json:"busy"`
	Items      []map[string]any `json:"items"`
	ActiveItem map[string]any   `json:"activeItem,omitempty"`
}

// ReviewSessionsForRun returns authoritative busy/queue snapshots for refresh resume.
func (e *Engine) ReviewSessionsForRun(runID string) []ReviewSessionSnapshot {
	e.reviewMu.Lock()
	defer e.reviewMu.Unlock()
	var out []ReviewSessionSnapshot
	for _, s := range e.reviewSess {
		if s == nil || s.runID != runID {
			continue
		}
		s.mu.Lock()
		snap := ReviewSessionSnapshot{
			NodeID:  s.producerID,
			Kind:    string(s.kind),
			Waiting: s.waiting,
			Busy:    s.active != nil,
			Items:   s.queueSnapshotLocked(),
		}
		if s.active != nil {
			snap.ActiveItem = map[string]any{
				"id":          s.active.ID,
				"text":        s.active.Text,
				"images":      s.active.Images,
				"annotations": s.active.Annotations,
			}
		}
		s.mu.Unlock()
		out = append(out, snap)
	}
	return out
}

// BroadcastReviewSessions re-emits queue_state (+ active hint) for every session
// on a run — used on WS connect so refresh can resume busy/queue/stream.
func (e *Engine) BroadcastReviewSessions(runID string) {
	for _, snap := range e.ReviewSessionsForRun(runID) {
		payload := map[string]any{
			"waiting": snap.Waiting,
			"items":   snap.Items,
			"busy":    snap.Busy,
			"kind":    snap.Kind,
		}
		if snap.ActiveItem != nil {
			payload["activeItem"] = snap.ActiveItem
		}
		e.publishReview(runID, snap.NodeID, "queue_state", payload)
	}
}

// EnqueueReviewTurn queues a revise turn for the parked producer session and
// returns immediately with the new waiting count. Serial pump starts if idle.
// Both ReactReply(force=false) and GateReactRevise share this entry (FR5).
func (e *Engine) EnqueueReviewTurn(runID, producerID, text string, images []models.PromptImage, annotations []models.ReactAnnotation, source, gateNodeID string) (waiting int, err error) {
	return e.enqueueReactTurn(runID, producerID, text, images, annotations, source, gateNodeID, sessionKindReview)
}

// EnqueueClarifyTurn queues a classic react (需求澄清) turn onto the same platform
// FIFO / WS frame protocol as review, returning immediately.
func (e *Engine) EnqueueClarifyTurn(runID, nodeID, text string, images []models.PromptImage, annotations []models.ReactAnnotation) (waiting int, err error) {
	return e.enqueueReactTurn(runID, nodeID, text, images, annotations, "node", "", sessionKindClarify)
}

func (e *Engine) enqueueReactTurn(runID, producerID, text string, images []models.PromptImage, annotations []models.ReactAnnotation, source, gateNodeID string, kind sessionKind) (waiting int, err error) {
	if e.IsHalted() {
		return 0, errors.New("server is shutting down")
	}
	if strings.TrimSpace(text) == "" && len(images) == 0 && len(annotations) == 0 {
		return 0, errors.New("text, images, or annotations required")
	}
	images, err = blob.IngestPromptImages(context.Background(), e.blobs, images)
	if err != nil {
		return 0, fmt.Errorf("ingest attachments: %w", err)
	}
	s := e.getOrCreateReviewSession(runID, producerID, kind)
	item := &reviewQueueItem{
		ID:          uuid.NewString(),
		Text:        text,
		Effective:   renderReviewHuman(text, annotations),
		Images:      images,
		Annotations: annotations,
		Source:      source,
		GateNodeID:  gateNodeID,
	}

	s.mu.Lock()
	if s.waiting >= MaxReviewQueueItems {
		s.mu.Unlock()
		fullMsg := "复审消息队列已满,请稍候"
		if kind == sessionKindClarify {
			fullMsg = "澄清消息队列已满,请稍候"
		}
		return 0, errors.New(fullMsg)
	}
	s.queue = append(s.queue, item)
	s.waiting++
	waiting = s.waiting
	startPump := !s.pumping
	if startPump {
		s.pumping = true
	}
	s.mu.Unlock()

	e.publishReview(runID, producerID, "queue_state", map[string]any{
		"waiting": waiting,
		"items":   s.queueSnapshot(),
		"busy":    true,
		"kind":    string(kind),
	})
	if startPump {
		go e.pumpReviewSession(s)
	}
	return waiting, nil
}

func (s *reviewSession) queueSnapshot() []map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.queueSnapshotLocked()
}

func (s *reviewSession) queueSnapshotLocked() []map[string]any {
	out := make([]map[string]any, 0, len(s.queue))
	for _, it := range s.queue {
		out = append(out, map[string]any{
			"id":   it.ID,
			"text": it.Text,
		})
	}
	return out
}

// CancelReviewSession clears all not-yet-started queue items, requests ACP
// Cancel on the active turn (bridge PromptQueue cleared via session/cancel),
// and keeps the session parked for further edits. Does not AbortRun / RetireSession.
// Used by post-run review / gate hot-revise (#77 clear-queue semantics).
func (e *Engine) CancelReviewSession(runID, producerID string) error {
	return e.cancelReactSession(runID, producerID, true)
}

// CancelClarifyTurn stops only the in-flight clarify turn, keeps the pending
// FIFO, and lets the pump start the next item (Demo keep-queue+pump-next).
func (e *Engine) CancelClarifyTurn(runID, nodeID string) error {
	return e.cancelReactSession(runID, nodeID, false)
}

// cancelReactSession aborts the active turn. When clearQueue is true (review),
// pending items are dropped; when false (clarify), they remain and the pump continues.
func (e *Engine) cancelReactSession(runID, producerID string, clearQueue bool) error {
	e.reviewMu.Lock()
	s := e.reviewSess[e.reviewSessionKey(runID, producerID)]
	e.reviewMu.Unlock()
	if s == nil {
		// Still best-effort ACP cancel in case a legacy sync turn is in flight.
		e.cancelProviderTurn(runID, producerID)
		e.publishReview(runID, producerID, "queue_state", map[string]any{"waiting": 0, "items": []any{}, "busy": false})
		return nil
	}

	s.mu.Lock()
	if clearQueue {
		s.queue = nil
		s.waiting = 0
	}
	s.cancelRequested = true
	waiting := s.waiting
	items := s.queueSnapshotLocked()
	busy := s.active != nil
	kind := string(s.kind)
	cancelFn := s.cancelFn
	s.mu.Unlock()

	e.publishReview(runID, producerID, "queue_state", map[string]any{
		"waiting": waiting,
		"items":   items,
		"busy":    busy,
		"kind":    kind,
	})
	e.cancelProviderTurn(runID, producerID)
	if cancelFn != nil {
		cancelFn()
	}
	return nil
}

func (e *Engine) cancelProviderTurn(runID, producerID string) {
	if cp, ok := e.provider.(runtime.ReviewTurnCanceller); ok {
		cp.CancelSessionTurn(runID, producerID)
	}
}

func (e *Engine) pumpReviewSession(s *reviewSession) {
	defer func() {
		s.mu.Lock()
		s.pumping = false
		idle := s.active == nil && s.waiting == 0
		s.mu.Unlock()
		if idle {
			e.dropReviewSessionIfIdle(s.runID, s.producerID)
		} else {
			// More items arrived while we were finishing; restart pump.
			s.mu.Lock()
			restart := s.waiting > 0 && !s.pumping
			if restart {
				s.pumping = true
			}
			s.mu.Unlock()
			if restart {
				go e.pumpReviewSession(s)
			}
		}
	}()

	for {
		s.mu.Lock()
		if len(s.queue) == 0 {
			s.mu.Unlock()
			return
		}
		item := s.queue[0]
		s.queue = s.queue[1:]
		if s.waiting > 0 {
			s.waiting--
		}
		waiting := s.waiting
		s.active = item
		s.cancelRequested = false
		ctx, cancel := context.WithCancel(context.Background())
		s.cancelFn = cancel
		s.mu.Unlock()

		e.publishReview(s.runID, s.producerID, "queue_state", map[string]any{
			"waiting": waiting,
			"items":   s.queueSnapshot(),
			"busy":    true,
			"kind":    string(s.kind),
		})
		e.publishReview(s.runID, s.producerID, "turn_begin", map[string]any{
			"item": map[string]any{
				"id":          item.ID,
				"text":        item.Text,
				"images":      item.Images,
				"annotations": item.Annotations,
			},
			"gateNodeId": item.GateNodeID,
			"source":     item.Source,
			"kind":       string(s.kind),
		})

		var interrupted bool
		var turnErr error
		if s.kind == sessionKindClarify {
			interrupted, turnErr = e.executeClarifyTurn(ctx, s, item)
		} else {
			interrupted, turnErr = e.executeReviewTurn(ctx, s, item)
		}

		cancel()
		s.mu.Lock()
		s.active = nil
		s.cancelFn = nil
		wasCancel := s.cancelRequested || interrupted
		s.cancelRequested = false
		s.mu.Unlock()

		if turnErr != nil && !wasCancel {
			e.publishReview(s.runID, s.producerID, "error", map[string]any{
				"message":     turnErr.Error(),
				"interrupted": false,
			})
		} else {
			e.publishReview(s.runID, s.producerID, "turn_done", map[string]any{
				"interrupted": wasCancel,
			})
		}
	}
}

// executeClarifyTurn persists the human turn, runs provider.ReactReply (with
// cancelable ctx), persists the agent turn (marking interrupted on cancel), and
// finalizes the node when the agent is Done — mirroring the former sync ReactReply
// path but under the shared FIFO pump so refresh can resume busy/queue/stream.
func (e *Engine) executeClarifyTurn(ctx context.Context, s *reviewSession, item *reviewQueueItem) (interrupted bool, err error) {
	unlock := e.lockResume(s.runID + ":" + s.producerID)
	defer unlock()

	c, loadErr := e.loadCtx(s.runID)
	if loadErr != nil {
		return false, loadErr
	}
	node := c.graph.FindNode(s.producerID)
	if node == nil || node.Type != "react" {
		return false, errors.New("react node not found")
	}

	var conv models.ReactConversation
	if cerr := e.db.Where("run_id = ? AND node_id = ?", s.runID, s.producerID).
		Order("iteration desc, id desc").First(&conv).Error; cerr != nil {
		return false, errors.New("no react conversation")
	}
	if conv.Done {
		return false, errors.New("react already done")
	}

	now := time.Now().Format(time.RFC3339)
	conv.Messages = append(conv.Messages, models.ReactMessage{
		Role: "human", Text: item.Text, At: now,
		Images: item.Images, Annotations: item.Annotations,
	})
	logDB(e.db.Save(&conv), s.runID, "save clarify human turn (turn_begin)")
	e.broker.Publish(s.runID, jsonMsg("react", s.runID, s.producerID))

	req := e.nodeReq(c, node)
	force := item.Force
	t := e.provider.ReactReply(ctx, req, conv.Messages, item.Effective, item.Images, force)

	s.mu.Lock()
	cancelled := s.cancelRequested || ctx.Err() != nil
	s.mu.Unlock()
	if cancelled {
		interrupted = true
	}

	agentMsg := models.ReactMessage{
		Role: "agent", Text: t.Msg, At: time.Now().Format(time.RFC3339),
		Questions: t.Questions, Interrupted: interrupted,
	}
	if interrupted && strings.TrimSpace(agentMsg.Text) == "" {
		agentMsg.Text = "(已中断)"
	}
	conv.Messages = append(conv.Messages, agentMsg)

	if interrupted {
		logDB(e.db.Save(&conv), s.runID, "save clarify agent turn (interrupted)")
		e.flushMcpCalls(s.runID, s.producerID)
		e.flushTokenUsage(s.runID, s.producerID, t.Usage, t.UsageByModel)
		log.Info().Str("run_id", s.runID).Str("node", s.producerID).
			Msg("clarify turn interrupted by Cancel; queue kept for next item")
		e.broker.Publish(s.runID, jsonMsg("react", s.runID, s.producerID))
		return true, nil
	}

	// Auto-clarify: recommended options while auto_var is on.
	// autoAdvanceReact already appends human/agent turns to conv.
	if !force && !t.Done && len(t.Questions) > 0 && e.autoReactEnabled(c, node) {
		t = e.autoAdvanceReact(c, node, &conv, req, t)
	}

	if !t.Done {
		logDB(e.db.Save(&conv), s.runID, "save clarify conversation")
		e.flushMcpCalls(s.runID, s.producerID)
		e.flushTokenUsage(s.runID, s.producerID, t.Usage, t.UsageByModel)
		e.broker.Publish(s.runID, jsonMsg("react", s.runID, s.producerID))
		return false, nil
	}

	conv.Done = true
	logDB(e.db.Save(&conv), s.runID, "finish clarify conversation")

	if t.Err != nil {
		outcome := nodeOutcome{status: "failed", err: t.Err.Error(), outputMd: t.Msg, events: t.Events, usage: t.Usage, usageByModel: t.UsageByModel}
		e.saveState(c, node, outcome)
		e.appendTrace(c, models.TraceEntry{NodeID: s.producerID, Event: "resume", Detail: "react 失败"})
		next := e.routeFailure(c, node, outcome)
		if next == "" {
			e.finish(s.runID, "failed")
			return false, nil
		}
		go e.resumeAdmitted(s.runID, next)
		return false, nil
	}

	outcome := e.finishAgentOutcome(c, node, t.Result, func(r runtime.NodeResult) nodeOutcome {
		return e.completeProduces(c, node, r)
	})
	e.saveState(c, node, outcome)
	e.appendTrace(c, models.TraceEntry{NodeID: s.producerID, Event: "resume", Detail: "react 完成"})

	if outcome.status == "failed" {
		next := e.routeFailure(c, node, outcome)
		if next == "" {
			e.finish(s.runID, "failed")
			return false, nil
		}
		go e.resumeAdmitted(s.runID, next)
		return false, nil
	}

	c.nodeOutputs[s.producerID] = t.Result.Outputs
	e.appendTrace(c, models.TraceEntry{NodeID: s.producerID, Event: "exit"})

	next := e.routeSuccess(c, node, outcome)
	if next == "" {
		e.finish(s.runID, "completed")
		return false, nil
	}
	go e.resumeAdmitted(s.runID, next)
	return false, nil
}

// executeReviewTurn persists the human turn, runs ReviseInPlace, persists the
// agent turn (marking interrupted on cancel), and refreshes outputs/gates.
func (e *Engine) executeReviewTurn(ctx context.Context, s *reviewSession, item *reviewQueueItem) (interrupted bool, err error) {
	// Serialize against force confirm / legacy paths on the same producer key.
	unlock := e.lockResume(s.runID + ":" + s.producerID)
	defer unlock()

	c, loadErr := e.loadCtx(s.runID)
	if loadErr != nil {
		return false, loadErr
	}
	producer := c.graph.FindNode(s.producerID)
	if producer == nil {
		return false, errors.New("上游生产节点不存在")
	}
	rp, ok := e.provider.(runtime.ReviewProvider)
	if !ok {
		return false, errors.New("当前执行后端不支持 ReAct 复审")
	}

	iter := c.iter[s.producerID]
	if iter < 1 {
		iter = 1
	}
	var conv models.ReactConversation
	cerr := e.db.Where("run_id = ? AND node_id = ? AND iteration = ?", s.runID, s.producerID, iter).
		Order("id desc").First(&conv).Error
	if cerr != nil {
		if item.Source == "gate" {
			conv = models.ReactConversation{RunID: s.runID, NodeID: s.producerID, Iteration: iter, Done: false,
				Messages: []models.ReactMessage{{Role: "agent", Text: e.reviewSummaryMarkdown(c, producer),
					At: time.Now().Format(time.RFC3339)}}}
			logDB(e.db.Create(&conv), s.runID, "seed gate-react producer conversation")
		} else {
			return false, errors.New("no react conversation")
		}
	}
	if conv.Done {
		return false, errors.New("上游复审会话已结束")
	}

	now := time.Now().Format(time.RFC3339)
	conv.Messages = append(conv.Messages, models.ReactMessage{
		Role: "human", Text: item.Text, At: now,
		Images: item.Images, Annotations: item.Annotations,
	})
	logDB(e.db.Save(&conv), s.runID, "save review human turn (turn_begin)")

	req := e.nodeReq(c, producer)
	e.host.SetActiveReview(s.runID, true)

	// Publish react so UIs that refetch on turn_begin see the human bubble.
	e.broker.Publish(s.runID, jsonMsg("react", s.runID, s.producerID))

	t := rp.ReviseInPlace(ctx, req, conv.Messages, item.Effective, item.Images)

	s.mu.Lock()
	cancelled := s.cancelRequested || ctx.Err() != nil
	s.mu.Unlock()
	// Cancel and revise failure share Interrupted so UI never shows Done/已完成.
	// Surface failure in chat; do not fail the enqueue API or the Run.
	if cancelled || t.Err != nil {
		interrupted = true
	}

	agentMsg := models.ReactMessage{
		Role: "agent", Text: t.Msg, At: time.Now().Format(time.RFC3339),
		Questions: t.Questions, Interrupted: interrupted,
	}
	if interrupted && strings.TrimSpace(agentMsg.Text) == "" && t.Err != nil {
		agentMsg.Text = "(已中断)"
	}
	conv.Messages = append(conv.Messages, agentMsg)
	logDB(e.db.Save(&conv), s.runID, "save review agent turn")

	e.flushMcpCalls(s.runID, s.producerID)
	e.flushTokenUsage(s.runID, s.producerID, t.Usage, t.UsageByModel)

	if interrupted {
		if cancelled {
			log.Info().Str("run_id", s.runID).Str("producer", s.producerID).
				Msg("review turn interrupted by Cancel; session kept parked")
		} else {
			log.Warn().Err(t.Err).Str("run_id", s.runID).Str("producer", s.producerID).
				Msg("review revise turn failed (session kept for retry)")
		}
		e.broker.Publish(s.runID, jsonMsg("react", s.runID, s.producerID))
		return true, nil
	}

	e.refreshProducerOutputs(c, producer)
	if item.Source == "gate" && item.GateNodeID != "" {
		e.refreshGateBodyAfterRevise(c, item.GateNodeID)
		e.appendTrace(c, models.TraceEntry{NodeID: item.GateNodeID, Event: "resume",
			Detail: "ReAct 打回 → 上游 " + s.producerID + " 就地修改"})
		e.broker.Publish(s.runID, jsonMsg("artifact_edit", s.runID, item.GateNodeID))
	} else {
		e.refreshPendingGatesForProducer(c, s.producerID)
		e.broker.Publish(s.runID, jsonMsg("artifact_edit", s.runID, s.producerID))
	}
	e.broker.Publish(s.runID, jsonMsg("react", s.runID, s.producerID))
	return false, nil
}

func (e *Engine) refreshGateBodyAfterRevise(c *execCtx, gateNodeID string) {
	gateNode := c.graph.FindNode(gateNodeID)
	if gateNode == nil {
		return
	}
	var gate models.Gate
	if err := e.db.Where("run_id = ? AND node_id = ? AND resolved = ?", c.run.ID, gateNodeID, false).
		Order("id desc").First(&gate).Error; err != nil {
		return
	}
	c2, err2 := e.loadCtx(c.run.ID)
	if err2 != nil {
		log.Warn().Err(err2).Str("run_id", c.run.ID).Str("gate", gateNodeID).
			Msg("reload ctx after gate-react revise failed; gate body not refreshed")
		return
	}
	if bt, _ := gateNode.Config["body_template"].(string); strings.TrimSpace(bt) != "" {
		gate.BodyMd = e.interpolate(c2, bt)
		logDB(e.db.Save(&gate), c.run.ID, "refresh gate body after gate-react revise")
	} else if gateNode.Type == "proposal_select" {
		from := firstNonEmptyStr(str(gateNode.Config["from"]), mcp.ProposalsArtifactName)
		if s, ok := e.store.Get(c.run.ID, from); ok {
			gate.BodyMd = mcp.RenderProposalsMarkdown(s)
			logDB(e.db.Save(&gate), c.run.ID, "refresh proposal_select body after gate-react revise")
		}
	}
}

// publishReview emits a SandboxChat-shaped control/event frame on the run
// broker, nested under type:"review" so LiveLog can ignore it while ClarifyChat
// / GateApproval consume queue_state / turn_begin / turn_done / error.
// ACP stream continues via existing type:"acp" (publishAcp) — dialogue surfaces
// filter by nodeId (producer).
func (e *Engine) publishReview(runID, nodeID, event string, extra map[string]any) {
	payload := map[string]any{
		"type": "review", "runId": runID, "nodeId": nodeID, "event": event,
	}
	for k, v := range extra {
		payload[k] = v
	}
	msg, err := json.Marshal(payload)
	if err != nil {
		return
	}
	e.broker.Publish(runID, msg)
}
