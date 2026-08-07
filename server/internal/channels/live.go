package channels

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/cocofhu/approving/internal/liveagent"
	"github.com/cocofhu/approving/internal/sendable"
	"github.com/cocofhu/approving/internal/services"

	"github.com/rs/zerolog/log"
)

// LiveModel is the conversation model. It is the only thing that talks to the
// user.
//
// The split is between talking and doing, not between easy and hard. The
// sandbox agent can read the repository and change it; it cannot hold a
// conversation, because every sentence it says costs a container and tens of
// seconds, and it has no way to know what the user was already told. So the
// model in front does the talking — answering what it can, delegating what it
// cannot, and reporting back what the work layer found — and the agent works
// behind it.
//
// The model is not a filter. It cannot read the repository and is never asked
// to pretend otherwise: anything it says about work in flight comes from
// get_status, and anything requiring facts it does not have goes to dispatch_pm.
type LiveModel interface {
	Configured() bool
	Complete(ctx context.Context, req liveagent.Request) (liveagent.Result, error)
}

// The three decisions the conversation layer can make about work. They exist as
// separate tools rather than one "escalate" because the difference matters at
// the point of the call: delegating starts something, reading status must never
// start anything, and cancelling stops something. Collapsing them into one verb
// is what made "好了没" open a second sandbox to find out.
const (
	dispatchPMTool = "dispatch_pm"
	getStatusTool  = "get_status"
	cancelWorkTool = "cancel_work"
)

// liveToolLoopLimit bounds one turn's tool calls. Two is enough for the shapes
// that occur — check status then answer, cancel then confirm — and a model that
// wants a third is looping rather than working.
const liveToolLoopLimit = 3

// liveSystemPrompt tells the model who it is and where the line is.
//
// The line is drawn by capability, not topic: it may say anything it can
// support from the conversation in front of it or from a tool result, and must
// delegate anything that needs the repository or an action. Uncertainty
// resolves toward delegating, because an invented answer is indistinguishable
// from a real one to the person reading it.
const liveSystemPrompt = `你是这个项目的负责人本人，正在 IM 上和同事聊天。你的回复会原样发给对方。

你手下有人干活。你的职责是接话、判断这件事有多大、派下去，然后把真实的进展和结论讲给对方听。

你可以直接回答：打招呼、闲聊、澄清对方的意思、解释你们刚才聊过的内容、回答你从这段对话里就能确定的事。

需要看代码或仓库、需要动手改东西、需要跑流程、以及任何你不能仅凭这段对话确定的事实 —— 调用 dispatch_pm 派下去。
要讲任务的状态、进度或结论 —— 先调用 get_status，用它返回的内容说话。
对方说不用弄了 / 停下 / 算了 —— 调用 cancel_work。

派活前先判断难度：
- lookup：查一下就知道的事。
- heavy：要花好几分钟的事。

派活时必须用 user_reply 先接一句，说清楚你要去确认的是哪件事；平台会先把这句发出去，再去查。

规矩：
- 接话必须有内容。「稍等」「好的我看一下」「收到」这种空话不算接话，要说清楚你要去确认的是哪件事。
- 不要编造任务状态、进度、代码内容或任何你没有依据的事实。宁可先派下去。
- 一次只说一件事，说人话，像微信上回同事，不要写工单腔。
- 不要提到"沙箱""Agent""模型""工具""上下文"这些词，对方只是在和你聊天。
- 你就是这个项目的负责人本人。不要说自己是 AI、模型或助手，也不要说出任何模型名称、版本或厂商；被问"你是什么模型"就当同事随口一问，把话题带回工作。`

// liveMaxTokens sizes the reply plus whatever the model thinks first. A
// reasoning model can spend most of a small budget deliberating over a one-line
// routing decision and then return nothing; the timeout, not this, is the cost
// control.
const liveMaxTokens = 2048

// SetLiveModel wires the conversation model. Leaving it unset means every
// message goes to a sandbox, which is slower but complete.
func (m *Manager) SetLiveModel(model LiveModel) { m.live = model }

// liveOutcome is what the conversation layer decided about one message.
type liveOutcome struct {
	// answered is true when the user has already been replied to and there is
	// nothing left for the work layer to do.
	answered bool
	// reason explains why the turn is being handed to the agent. Empty when the
	// model answered, or when there was no conversation model to ask.
	reason string
	// dispatch is the delegation the director decided on. nil when the turn
	// reached the agent by falling through rather than by decision.
	dispatch *WorkDispatch
	// sampleID links this turn's decision record, so the work layer's eventual
	// conclusion can be attached to the choice that produced it.
	sampleID string
}

// applyTo carries the decision onto the message the work layer will receive.
func (o liveOutcome) applyTo(in *InboundMessage) {
	if in == nil {
		return
	}
	in.EscalationReason = o.reason
	in.Dispatch = o.dispatch
	in.DecisionSampleID = o.sampleID
}

// routeThroughLiveModel asks the conversation layer what to do with a message,
// runs whatever tools it asks for, and returns what is left for the work layer.
//
// Every failure mode ends in the same place: the sandbox. A model that is
// unconfigured, unreachable, out of budget or simply unsure must never cause a
// message to be dropped or answered with an apology — the agent can still
// handle it, just more slowly.
func (m *Manager) routeThroughLiveModel(ctx context.Context, rc *runningChannel, in InboundMessage) liveOutcome {
	if m.live == nil || !m.live.Configured() {
		// Silence here is what made "is the conversation model actually being
		// used?" unanswerable: an unconfigured endpoint skipped this layer
		// without leaving a trace, and every reply looked the same from the
		// outside. Deliveries the model does make are audited as live_reply, so
		// this is the other half of that answer.
		log.Debug().Str("project", rc.cfg.ProjectID).
			Msg("no conversation model configured; this message goes straight to the agent")
		return liveOutcome{}
	}

	rec := m.newSampleRecorder(rc, in)
	briefing := m.buildDirectorContext(rc, in)
	rec.briefedWith(briefing)

	req := liveagent.Request{
		System:    liveSystemPrompt + "\n\n" + briefing.render(),
		Messages:  m.liveMessages(rc, in),
		Tools:     directorTools(),
		MaxTokens: liveMaxTokens,
	}
	rec.shown(req.Messages)

	for step := 0; step < liveToolLoopLimit; step++ {
		res, err := m.live.Complete(ctx, req)
		if err != nil {
			// Not a user-visible failure: the message still gets answered, by
			// the agent. Logged so an endpoint failing every call is visible as
			// something other than "the assistant got slow".
			level := log.Warn()
			if errors.Is(err, liveagent.ErrNotConfigured) {
				level = log.Debug()
			}
			level.Err(err).Str("project", rc.cfg.ProjectID).
				Msg("conversation model unavailable; handing turn to the agent")
			rec.failed(err)
			// The sandbox still answers, but it can take a minute to come up.
			// Saying nothing until then is how "项目的错误处理完整吗" sat on
			// screen with a running timer and no bubble — the model timed out
			// and the fallthrough used to be silent.
			m.ackFallthrough(ctx, rc, in, rec)
			return liveOutcome{reason: "会话层没能给出答复", sampleID: rec.commit(routeFallthrough)}
		}
		rec.completed(res)

		switch res.ToolName {
		case getStatusTool:
			out := m.runGetStatus(rc, in, res.Args["task_id"])
			rec.toolReturned(getStatusTool, res.Args, out)
			req.Messages = append(req.Messages, toolResultMessage(getStatusTool, out))
		case cancelWorkTool:
			out := m.runCancelWork(ctx, rc, in, res.Args["task_id"])
			rec.toolReturned(cancelWorkTool, res.Args, out)
			req.Messages = append(req.Messages, toolResultMessage(cancelWorkTool, out))
		case dispatchPMTool:
			outcome, refused := m.dispatchWork(ctx, rc, in, res.Args, rec)
			if refused == "" {
				return outcome
			}
			rec.toolReturned(dispatchPMTool, res.Args, refused)
			req.Messages = append(req.Messages, toolResultMessage(dispatchPMTool, refused))
		default:
			return m.deliverDirectorReply(ctx, rc, in, res.Text, rec)
		}
	}

	// The model kept reaching for tools and never said anything. Handing the
	// turn on is the only path that still produces a reply.
	log.Info().Str("project", rc.cfg.ProjectID).
		Msg("conversation model looped on tools without answering; handing turn to the agent")
	rec.flag("tool_loop_exhausted")
	m.ackFallthrough(ctx, rc, in, rec)
	return liveOutcome{reason: "会话层查了状态但没有给出答复", sampleID: rec.commit(routeFallthrough)}
}

// ackFallthrough tells the user something is happening when the conversation
// layer could not finish and the sandbox is about to take over. Without it a
// timed-out Live call looks like the platform ignored the message.
func (m *Manager) ackFallthrough(ctx context.Context, rc *runningChannel, in InboundMessage, rec *sampleRecorder) {
	title := truncateRunes(strings.TrimSpace(in.Text), 20)
	text, flags := gateAcknowledgement("", title, in.Text)
	rec.flag(flags...)
	rec.flag("fallthrough_ack")
	sent := m.sendOutboundResult(ctx, rc, OutboundMessage{
		Scene: in.Scene, ConversationID: in.ConversationID,
		ReplyToMessageID: in.MessageID, Text: text,
		Envelope: turnEnvelope(rc, in, sendable.KindTurnProcessingAck, "live_ack", sendable.PriorityHigh),
	})
	rec.acted("live_ack", text, sent)
	if sent.Sent {
		scope := conversationTurnScope(rc.cfg.ProjectID, in.Scene, in.ConversationID)
		m.markReplied(scope)
		m.markAcknowledged(scope)
	}
}

// deliverDirectorReply sends what the model said, as it said it.
func (m *Manager) deliverDirectorReply(ctx context.Context, rc *runningChannel, in InboundMessage,
	text string, rec *sampleRecorder) liveOutcome {
	answer := strings.TrimSpace(text)
	if answer == "" {
		m.ackFallthrough(ctx, rc, in, rec)
		return liveOutcome{reason: "会话层没能给出答复", sampleID: rec.commit(routeFallthrough)}
	}
	sent := m.sendOutboundResult(ctx, rc, OutboundMessage{
		Scene: in.Scene, ConversationID: in.ConversationID,
		ReplyToMessageID: in.MessageID, Text: answer,
		Envelope: turnEnvelope(rc, in, sendable.KindFinal, "live_reply", sendable.PriorityNormal),
	})
	rec.acted("live_reply", answer, sent)
	if !sent.Sent {
		// The answer never reached anyone, so the turn is still unanswered.
		// Handing it to the agent is the only path that can still produce a
		// reply; claiming success here is what leaves a user waiting in silence.
		log.Warn().Str("project", rc.cfg.ProjectID).Str("reason", sent.Decision.Reason).
			Msg("conversation model answer was not delivered; handing turn to the agent")
		m.ackFallthrough(ctx, rc, in, rec)
		return liveOutcome{reason: "会话层的回复没有送达", sampleID: rec.commit(routeFallthrough)}
	}
	scope := conversationTurnScope(rc.cfg.ProjectID, in.Scene, in.ConversationID)
	m.markReplied(scope)
	m.markAnswered(scope)
	return liveOutcome{answered: true, sampleID: rec.commit(routeReply)}
}

// dispatchWork turns a delegation into a real task and, when the work will
// outlive the conversation's patience, answers the user before starting it.
//
// A non-empty refusal means the delegation did not happen and the string is a
// tool result the director must deal with — the platform declines, the director
// explains. Wording a refusal here is how a fixed template ends up in the
// conversation again.
func (m *Manager) dispatchWork(ctx context.Context, rc *runningChannel, in InboundMessage,
	args map[string]string, rec *sampleRecorder) (outcome liveOutcome, refused string) {
	brief := strings.TrimSpace(args["request"])
	if brief == "" {
		brief = strings.TrimSpace(in.Text)
	}
	if brief == "" {
		brief = "需要读仓库或任务真实状态"
	}
	difficulty := parseDifficulty(args["difficulty"])
	title := services.SanitizeShortTitle(args["short_title"])
	if title == "" {
		title = truncateRunes(strings.TrimSpace(in.Text), 40)
	}
	if running := m.taskLedger(rc, in); len(running) >= maxConcurrentWork {
		return liveOutcome{}, encodeToolResult(map[string]any{
			"rejected": "这个会话同时在跑的任务已经到上限了，没有派下去。",
			"running":  running,
			"hint":     "跟对方说清楚现在手上有哪几件事，问他要先停掉哪件，或者要不要排在后面。",
		})
	}

	dispatch := &WorkDispatch{
		Brief: brief, Difficulty: difficulty, ShortTitle: title,
		Attachments: m.conversationAttachments(rc, in),
	}
	m.ensureTaskIdentity(rc, in, dispatch)

	// Every sandbox-bound dispatch is acknowledged first.
	//
	// An earlier "wait=true means stay silent until the answer" shape assumed a
	// short synchronous lookup. What we actually start here is a sandbox turn
	// that can take tens of seconds — silence for that long is exactly the
	// "快模型没有先回复" failure. Until a real sub-second SoT path exists,
	// acknowledging is not optional.
	text, flags := gateAcknowledgement(args["user_reply"], title, in.Text)
	rec.flag(flags...)
	sent := m.sendOutboundResult(ctx, rc, OutboundMessage{
		Scene: in.Scene, ConversationID: in.ConversationID,
		ReplyToMessageID: in.MessageID, Text: text,
		// An acknowledgement is not the answer. It marks the turn as having
		// spoken, never as having answered, so the conclusion that follows
		// is not suppressed as a duplicate.
		Envelope: turnEnvelope(rc, in, sendable.KindTurnProcessingAck, "live_ack", sendable.PriorityHigh),
	})
	rec.acted("live_ack", text, sent)
	if sent.Sent {
		scope := conversationTurnScope(rc.cfg.ProjectID, in.Scene, in.ConversationID)
		m.markReplied(scope)
		m.markAcknowledged(scope)
	}

	return liveOutcome{
		reason: brief, dispatch: dispatch, sampleID: rec.commit(routeDispatch),
	}, ""
}

// foregroundCapture holds a turn's diverted agent replies.
//
// The zero value is a no-op capture that yields nothing, which is exactly what
// a turn without a conversation layer needs: pm_reply goes straight out as it
// always did, and this side of the code does not have to know the difference.
type foregroundCapture struct {
	collect func() string
	taken   bool
	text    string
	enabled bool
}

// beginForegroundCapture diverts this turn's pm_reply into the conversation
// layer, when there is one to divert it into.
//
// Without a conversation model there is nobody to do the reporting, so the
// agent keeps speaking for itself. That is a degradation, not a mode: the user
// hears the work layer's own register, and the sample says so.
func (m *Manager) beginForegroundCapture(rc *runningChannel, in InboundMessage) *foregroundCapture {
	if m.live == nil || !m.live.Configured() {
		return &foregroundCapture{}
	}
	collect, ok := m.captureAgentReplies(rc.cfg.ProjectID, in.Scene, in.ConversationID)
	if !ok {
		return &foregroundCapture{}
	}
	return &foregroundCapture{collect: collect, enabled: true}
}

// take ends the capture and returns what the agent submitted. Calling it twice
// is safe: the second call returns the same text rather than an empty string,
// so a deferred release cannot erase the answer.
func (c *foregroundCapture) take() string {
	if c == nil {
		return ""
	}
	if c.taken {
		return c.text
	}
	c.taken = true
	if c.collect != nil {
		c.text = strings.TrimSpace(ScrubInternalTerms(c.collect()))
	}
	return c.text
}

func (c *foregroundCapture) release() {
	if c != nil {
		_ = c.take()
	}
}

// egress names who ended up speaking, for the decision record.
func (c *foregroundCapture) egress(degraded bool) string {
	if c == nil || !c.enabled || degraded {
		return egressPMDirect
	}
	return egressDirector
}

// reportThroughDirector puts the work layer's conclusion into the voice the
// user has been hearing all along.
//
// This is rephrasing, not rewriting. The facts come from the agent, which is
// the only layer that checked them; the conversation layer is only allowed to
// say them the way it says everything else. When it cannot — no model, a slow
// endpoint, an empty completion — the agent's own words go out scrubbed, which
// reads slightly off but is true. degraded reports which of the two happened.
func (m *Manager) reportThroughDirector(ctx context.Context, rc *runningChannel, in InboundMessage,
	conclusion string) (text string, degraded bool) {
	plain := strings.TrimSpace(ScrubInternalTerms(conclusion))
	if m.live == nil || !m.live.Configured() || plain == "" {
		return plain, true
	}
	callCtx, cancel := context.WithTimeout(ctx, directorReportTimeout)
	defer cancel()

	res, err := m.live.Complete(callCtx, liveagent.Request{
		System: directorReportPrompt,
		Messages: []liveagent.Message{
			{Role: "user", Content: "对方问的是：" + truncateRunes(strings.TrimSpace(in.Text), 200)},
			{Role: "user", Content: "查到的结果：" + truncateRunes(plain, 3000)},
		},
		MaxTokens: directorReportMaxTokens,
	})
	if err != nil || strings.TrimSpace(res.Text) == "" {
		log.Info().Err(err).Str("project", rc.cfg.ProjectID).
			Msg("conclusion reported in the work layer's own words; the conversation model did not phrase it")
		return plain, true
	}
	return strings.TrimSpace(res.Text), false
}

// directorReportTimeout bounds the extra hop between having an answer and
// sending it. Local reasoning models routinely need tens of seconds; a 4s
// budget was what forced every Ollama conclusion down the degraded "paste the
// whole agent dump" path in live verification.
const directorReportTimeout = 45 * time.Second

// Reasoning models on local Ollama often spend most of a small budget on the
// side-channel "reasoning" field and return empty content with finish_reason
// length. 2048 is what actually left room for a two-sentence IM reply in live
// verification against genesis-hermes-v7.
const directorReportMaxTokens = 2048

// directorReportPrompt is deliberately narrow. Anything that invites the model
// to add, judge, or expand turns a verified conclusion into a partly invented
// one, which is the exact failure this whole layer exists to prevent.
const directorReportPrompt = `你是这个项目的负责人本人，正在 IM 上和同事聊天。你的回复会原样发给对方。

下面给你的是查到的结果。用一两段人话把结论讲给对方听。

规矩：
- 只讲给出的事实。不要补充、不要推测、不要下额外结论、不要建议。
- 先给结论，必要细节最多再补两三句；不要把长报告或分点清单原样贴出去。
- 说人话，像同事当面说，不要写工单腔。
- 不要出现任务编号、工作流名、执行环境、工具名这些内部说法。
- 只输出要发出去的话，不要加前缀、标题或解释，也不要写到一半截断。`

// warmWorkLayer brings the agent's sandbox up in the background.
//
// It runs at most once per conversation. Waking a container on every chat
// message would spend more than it saves, and a conversation that never
// delegates should not be paying for an agent it does not use.
func (m *Manager) warmWorkLayer(rc *runningChannel, in InboundMessage) {
	if m.bridge == nil {
		return
	}
	key := convKey(rc.cfg.ProjectID, in.Scene, in.ConversationID)
	m.warmMu.Lock()
	if m.warmed == nil {
		m.warmed = map[string]bool{}
	}
	if m.warmed[key] {
		m.warmMu.Unlock()
		return
	}
	m.warmed[key] = true
	m.warmMu.Unlock()

	bridge, ref, caps := m.bridge, conversationRefFor(rc, in), SessionCapsFromConfig(rc.cfg.Config)
	projectID := rc.cfg.ProjectID
	go func() {
		ctx, cancel := context.WithTimeout(m.baseCtx, sandboxOpenBudget)
		defer cancel()
		bridge.Warm(ctx, projectID, ref, caps)
	}()
}

// conversationAttachments names the files already in this conversation so the
// delegation can point at them. The conversation layer cannot see their
// contents; it only has to know they exist and where.
func (m *Manager) conversationAttachments(rc *runningChannel, in InboundMessage) []AttachmentRef {
	b, ok := m.transcript.(*ChannelBridge)
	if !ok || b == nil {
		return nil
	}
	return b.RecentAttachments(conversationRefFor(rc, in))
}

// gateAcknowledgement keeps an acknowledgement from being a fixed template
// wearing the model's voice.
//
// An earlier version of this platform shipped "稍等，我看一下" as a literal
// constant before every turn, which is why that phrase is now banned outright.
// The ban alone is not the fix: a model that says the same empty thing in its
// own words leaves the user exactly as uninformed. So an acknowledgement has to
// name what is being checked, and one that does not is replaced by a line that
// does — and flagged, because the replacement is a symptom worth counting.
func gateAcknowledgement(reply, shortTitle, userText string) (string, []string) {
	text := strings.TrimSpace(reply)
	if informative := stripFiller(text); len([]rune(informative)) >= 6 {
		return text, nil
	}
	title := services.SanitizeShortTitle(shortTitle)
	if title == "" {
		title = truncateRunes(strings.TrimSpace(userText), 20)
	}
	flags := []string{"empty_ack"}
	if title == "" {
		return "我这就去确认，有结果马上回你。", flags
	}
	return "「" + title + "」我这就去确认，有结果马上回你。", flags
}

// ackFiller lists the phrases that carry no information on their own. This is
// not a routing table — nothing is decided from it. It only measures how much
// of the platform's own acknowledgement would still be there once the polite
// noise is removed.
var ackFiller = []string{
	"稍等", "等一下", "等等", "好的", "好嘞", "行", "收到", "我看一下", "看一下",
	"我来看看", "看看", "马上", "这就", "我处理一下", "处理一下", "ok", "okay",
	"sure", "let me", "one moment", "a moment", "hold on", "on it", "got it",
}

func stripFiller(s string) string {
	lower := strings.ToLower(s)
	for _, f := range ackFiller {
		lower = strings.ReplaceAll(lower, f, "")
	}
	return strings.TrimFunc(lower, func(r rune) bool {
		return strings.ContainsRune(" \t\n，。！!,.…~、？?", r)
	})
}

func parseDifficulty(v string) Difficulty {
	if strings.EqualFold(strings.TrimSpace(v), string(DifficultyHeavy)) {
		return DifficultyHeavy
	}
	return DifficultyLookup
}

// parseLooseBool accepts what models actually emit for a boolean argument.
func parseLooseBool(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "true", "1", "yes", "y", "是":
		return true
	}
	return false
}

// toolResultMessage feeds a tool result back as conversation.
//
// The transport speaks plain chat completions against any OpenAI-compatible
// endpoint, several of which handle a "tool" role badly or not at all. A
// labelled user message is understood everywhere and costs nothing in accuracy
// for results this small.
func toolResultMessage(tool, result string) liveagent.Message {
	return liveagent.Message{
		Role: "user",
		Content: "[系统] " + tool + " 返回：" + result +
			"\n用这个结果直接回答对方，不要再调用同一个工具。",
	}
}

func directorTools() []liveagent.ToolSpec {
	return []liveagent.ToolSpec{
		{
			Name: dispatchPMTool,
			Description: "把这件事派给能读代码仓库、能查真实状态、能动手干活的人。" +
				"任何需要事实依据或需要执行的事情都用它，不要自己猜。",
			Params: []liveagent.Param{
				{Name: "request", Description: "用一句话说清对方要什么，供接手的人直接开工。", Required: true},
				{
					Name:        "difficulty",
					Description: "lookup=查一下就知道；heavy=要花好几分钟。",
					Enum:        []string{string(DifficultyLookup), string(DifficultyHeavy)},
					Required:    true,
				},
				{Name: "short_title", Description: "给这件事起个对方看得懂的短名字，例如「登录页报错」。", Required: true},
				{Name: "user_reply", Description: "现在就发给对方的一句话，必填；要说清楚你去确认的是哪件事。", Required: true},
			},
		},
		{
			Name: getStatusTool,
			Description: "查这个会话里正在跑的任务的真实状态。" +
				"要跟对方讲进度、状态或结论之前必须先调它，不要凭印象说。",
			Params: []liveagent.Param{
				{Name: "task_id", Description: "只查某一件事时填它的 taskId；不填就列出全部。"},
			},
		},
		{
			Name:        cancelWorkTool,
			Description: "停掉正在跑的任务。对方说不用弄了、停下、算了的时候用。",
			Params: []liveagent.Param{
				{Name: "task_id", Description: "要停的那件事的 taskId。只有一件在跑时可以不填。"},
			},
		},
	}
}

// liveMessages renders the canonical transcript for the model. The current
// message is already in it — the Manager records inbound before routing — so it
// is not appended again.
func (m *Manager) liveMessages(rc *runningChannel, in InboundMessage) []liveagent.Message {
	var out []liveagent.Message
	if m.transcript != nil {
		entries, err := m.transcript.Window(conversationRefFor(rc, in), transcriptWindow)
		if err != nil {
			log.Warn().Err(err).Str("project", rc.cfg.ProjectID).
				Msg("conversation history unavailable for the conversation model")
		}
		for _, e := range entries {
			text := strings.TrimSpace(e.Text)
			if len(e.Images) > 0 {
				text += attachmentNote(len(e.Images))
			}
			if text == "" {
				continue
			}
			out = append(out, liveagent.Message{Role: e.Role, Content: text})
		}
	}
	if len(out) == 0 {
		// No stored history: answer the message on its own rather than nothing.
		text := strings.TrimSpace(in.Text)
		if len(in.Images) > 0 {
			text += attachmentNote(len(in.Images))
		}
		if text == "" {
			return nil
		}
		out = append(out, liveagent.Message{Role: "user", Content: text})
	}
	return out
}

// attachmentNote tells the model an attachment exists without pretending it can
// see it. The bytes go to the agent, which can actually open them.
func attachmentNote(n int) string {
	if n <= 0 {
		return ""
	}
	return "（本条消息带了附件，你看不到内容）"
}
