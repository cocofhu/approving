package channels

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/cocofhu/approving/internal/liveagent"
	"github.com/cocofhu/approving/internal/models"
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
	// Timeout is live_timeout_seconds from settings. Zero means "use caller fallback".
	Timeout() time.Duration
	Complete(ctx context.Context, req liveagent.Request) (liveagent.Result, error)
}

// The decisions the conversation layer can make about work. They exist as
// separate tools rather than one "escalate" because the difference matters at
// the point of the call: delegating starts something, refining hangs a follow-up
// on work already running, reading status must never start anything, and
// cancelling stops something. Collapsing them into one verb is what made
// "好了没" open a second sandbox — and "重点看 Release" dump a queue at the user.
const (
	dispatchPMTool = "dispatch_pm"
	refineWorkTool = "refine_work"
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

你手下有人干活。你的职责是接话、判断意图、派活或收窄已有任务，然后把真实进展和结论讲给对方听。

你可以直接回答：打招呼、闲聊、澄清对方的意思、解释你们刚才聊过的内容、回答你从这段对话里就能确定的事。

意图怎么认（先认意图，再动手）：
- 对方在补充、收窄、纠正、加重点（例如「重点看 Release 到现在」「别看旧的」「再加上导出」）——只要是挂在已有任务上，调用 refine_work，不要新开任务。
- 对方要一件和正在跑的事明显不同的新活 —— 调用 dispatch_pm。
- 要讲进度、状态或结论 —— 先 get_status，用返回内容说话。recent_terminal 里的 status 必须照实说：failed 就是失败，cancelled 就是取消；没有在跑的任务不等于做完了。
- 刚做完或刚汇报完之后，对方追问这次交付里的细节或产物——先看你上一条汇报和 get_status / recent_terminal 的 result_summary，用那里的事实答；有就直接给，没有就如实说这轮没留下。禁止抛开对话事实去做名词百科或教科书定义。
- 对方说不用弄了 / 停下 / 算了 —— 调用 cancel_work。
- 对方明确说重跑 / 再试 / 继续做刚才失败或取消的那件 —— 立刻 dispatch_pm（request 用原要求，short_title 沿用原标题）；user_reply 用活人话说明你正派人重新去做（时态是正在重试，不是已经重试完）；不要复述完整任务标题；不要只回空确认。

派活前判断难度：lookup=查一下就知道；heavy=要花好几分钟。
dispatch_pm / refine_work 都要用 user_reply 先接一句；平台会先发出去再去查。

规矩：
- 接话必须有内容。「稍等」「好的我看一下」「收到」这种空话不算接话。
- 任务 failed / cancelled 时：先如实说原因（有依据才说），然后让对方选下一步，例如重试、换范围/改方向、或先搁置；不要擅自说「我接着跑」「后面重新做」就开干，除非对方刚明确要你继续（重跑/再试等）。
- 任务已满时：若对方是在补充你们正在聊的那件，用 refine_work，不要把队列甩给对方让他挑停哪件。
- 只有对方明确要另开一件完全不同的新事、且确实满了，才简短说明手上忙、问要先停哪件。
- 不要编造任务状态、进度、代码内容或任何你没有依据的事实。
- 一次只说一件事，说人话，像微信上回同事，不要写工单腔，不要列举内部任务清单除非对方在问进度。
- 不要提到"沙箱""Agent""模型""工具""上下文"这些词。
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
	ensureTraceID(&in)
	if m.live == nil || !m.live.Configured() {
		// Still record a sample so every inbound turn has a TraceID row to join
		// sandbox/MCP/outbound against — otherwise "no Live" leaves a hole in
		// the call chain that debug cannot close.
		log.Debug().Str("project", rc.cfg.ProjectID).Str("trace", in.TraceID).
			Msg("no conversation model configured; this message goes straight to the agent")
		rec := m.newSampleRecorder(rc, in)
		rec.flag("live_not_configured")
		if outcome, ok := m.dispatchRetryFromLedger(ctx, rc, in, rec, ""); ok {
			return outcome
		}
		return liveOutcome{sampleID: rec.commit(routeDirect)}
	}

	rec := m.newSampleRecorder(rc, in)
	// "重跑啊" must not enter the full tool-routing loop: that is what timed
	// out into 「我这就去确认」. Phrase the ack with the fast model (tiny
	// prompt), then dispatch from the ledger.
	if outcome, ok := m.routeRetryAffirmation(ctx, rc, in, rec); ok {
		return outcome
	}

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
			level.Err(err).Str("project", rc.cfg.ProjectID).Str("trace", in.TraceID).
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
		case refineWorkTool:
			outcome, refused := m.refineWork(ctx, rc, in, res.Args, rec)
			if refused == "" {
				return outcome
			}
			rec.toolReturned(refineWorkTool, res.Args, refused)
			req.Messages = append(req.Messages, toolResultMessage(refineWorkTool, refused))
		default:
			return m.deliverDirectorReply(ctx, rc, in, res.Text, rec)
		}
	}

	// The model kept reaching for tools and never said anything. Handing the
	// turn on is the only path that still produces a reply.
	log.Info().Str("project", rc.cfg.ProjectID).Str("trace", in.TraceID).
		Msg("conversation model looped on tools without answering; handing turn to the agent")
	rec.flag("tool_loop_exhausted")
	m.ackFallthrough(ctx, rc, in, rec)
	return liveOutcome{reason: "会话层查了状态但没有给出答复", sampleID: rec.commit(routeFallthrough)}
}

// ackFallthrough tells the user something is happening when the conversation
// layer could not finish and the sandbox is about to take over. The spoken
// line must come from the fast model — never a platform-stitched「确认」stamp.
// If phrasing also fails, stay silent; the sandbox answer still follows.
func (m *Manager) ackFallthrough(ctx context.Context, rc *runningChannel, in InboundMessage, rec *sampleRecorder) {
	title := m.focusShortTitle(rc, in)
	user := "对方说：" + strings.TrimSpace(in.Text) + "\n"
	if title != "" && !echoesUserText(title, in.Text) {
		user += "（内部参考，勿原样复述）正在跟进的事：" + title + "\n"
	}
	user += "会话层这一步没谈完，工作层马上接手。用一两句人话接住对方。"
	text := strings.TrimSpace(m.phraseThroughLive(ctx, fallthroughAckPhrasePrompt, user))
	if text == "" || strings.Contains(text, "确认") || spokenLineSoundsFinished(text) {
		rec.flag("fallthrough_ack_omitted")
		return
	}
	rec.flag("fallthrough_ack_live")
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

// routeRetryAffirmation handles "重跑啊" before the slow full Live tool loop.
// The user-facing line is phrased by the fast model; the ledger supplies the
// work brief. That is what keeps the voice GM-like when Ollama would otherwise
// burn the whole timeout on routing tools and fall back to 「确认」.
func (m *Manager) routeRetryAffirmation(ctx context.Context, rc *runningChannel, in InboundMessage, rec *sampleRecorder) (liveOutcome, bool) {
	if !looksLikeRetryAffirmation(in.Text) {
		return liveOutcome{}, false
	}
	failed := m.latestRetryableTerminal(rc, in)
	if failed == nil {
		return liveOutcome{}, false
	}
	title := services.SanitizeShortTitle(failed.ShortTitle)
	req := strings.TrimSpace(failed.OriginalRequirement)
	// Context for the fast model only — the user already said retry, so the
	// spoken line must not paste the ledger title back at them.
	userContent := "对方说：" + strings.TrimSpace(in.Text) + "\n"
	if title != "" {
		userContent += "（内部参考，勿原样复述）要重跑的事：" + title + "\n"
	}
	if req != "" {
		userContent += "（内部参考，勿原样复述）原来的要求：" + truncateRunes(req, 200) + "\n"
	}
	ack := strings.TrimSpace(m.phraseThroughLive(ctx, retryAckPhrasePrompt, userContent))
	if retryAckUnusable(ack, title, req) {
		ack = ""
		rec.flag("retry_ack_skipped")
	} else {
		rec.flag("retry_ack_live")
	}
	return m.dispatchRetryFromLedger(ctx, rc, in, rec, ack)
}

// dispatchRetryFromLedger rebuilds a dispatch from a recently failed/cancelled
// task. preferredAck must already be Live-phrased; empty means send no ack
// (never a platform-stitched sentence).
func (m *Manager) dispatchRetryFromLedger(ctx context.Context, rc *runningChannel, in InboundMessage, rec *sampleRecorder, preferredAck string) (liveOutcome, bool) {
	if !looksLikeRetryAffirmation(in.Text) {
		return liveOutcome{}, false
	}
	failed := m.latestRetryableTerminal(rc, in)
	if failed == nil {
		return liveOutcome{}, false
	}
	brief := strings.TrimSpace(failed.OriginalRequirement)
	if brief == "" {
		brief = strings.TrimSpace(failed.ShortTitle)
	}
	if brief == "" {
		return liveOutcome{}, false
	}
	title := services.SanitizeShortTitle(failed.ShortTitle)
	userReply := strings.TrimSpace(preferredAck)
	if retryAckUnusable(userReply, title, brief) {
		userReply = ""
	}
	outcome, refused := m.dispatchWork(ctx, rc, in, map[string]string{
		"request": brief, "short_title": title,
		"difficulty": string(DifficultyHeavy),
		"user_reply": userReply,
		"ack_mode":   "live_only",
	}, rec)
	if refused != "" {
		log.Info().Str("project", rc.cfg.ProjectID).Str("trace", in.TraceID).
			Str("detail", refused).Msg("retry dispatch refused; using generic fallthrough")
		return liveOutcome{}, false
	}
	rec.flag("retry_dispatch")
	return outcome, true
}

// retryAckUnusable rejects empty lines, title echoes, and "already done" tense.
func retryAckUnusable(ack, title, req string) bool {
	ack = strings.TrimSpace(ack)
	if ack == "" || strings.Contains(ack, "确认") {
		return true
	}
	if retryAckEchoesBrief(ack, title, req) {
		return true
	}
	return spokenLineSoundsFinished(ack)
}

// spokenLineSoundsFinished catches "already done / already retried" tense on
// lines that should mean work is just starting or still in flight.
func spokenLineSoundsFinished(text string) bool {
	for _, bad := range []string{
		"已经重新", "已经重试", "重新重试", "已经跑完", "已经重新跑过",
		"已经派完", "已经开完", "已经进到队列", "已经在队列", "已经修好", "已经完成",
	} {
		if strings.Contains(text, bad) {
			return true
		}
	}
	return false
}

// retryAckEchoesBrief is true when the spoken line pastes the ledger title /
// requirement back — the failure mode behind quoting
// 「重新执行上次因服务重启而中断的 Approvin」.
func retryAckEchoesBrief(ack, title, req string) bool {
	ack = strings.TrimSpace(ack)
	if ack == "" {
		return false
	}
	if strings.Contains(ack, "「") || strings.Contains(ack, "」") {
		return true
	}
	for _, s := range []string{title, req} {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if len([]rune(s)) >= 6 && strings.Contains(ack, s) {
			return true
		}
		if r := []rune(s); len(r) >= 10 && strings.Contains(ack, string(r[:10])) {
			return true
		}
	}
	return false
}

// phraseThroughLive asks the fast model for one short IM line. Empty means the
// caller must not invent a platform sentence to stand in for it.
func (m *Manager) phraseThroughLive(ctx context.Context, system, user string) string {
	if m == nil || m.live == nil || !m.live.Configured() {
		return ""
	}
	system, user = strings.TrimSpace(system), strings.TrimSpace(user)
	if system == "" || user == "" {
		return ""
	}
	// Keep this hop short: it is only one sentence. A stuck Ollama must not
	// burn the full live_timeout before we skip the spoken ack.
	timeout := 20 * time.Second
	if d := m.live.Timeout(); d > 0 && d < timeout {
		timeout = d
	}
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	res, err := m.live.Complete(callCtx, liveagent.Request{
		System: system, Messages: []liveagent.Message{{Role: "user", Content: user}},
		MaxTokens: 256,
	})
	if err != nil {
		return ""
	}
	return strings.TrimSpace(res.Text)
}

const retryAckPhrasePrompt = `你是这个项目的负责人本人，正在 IM 上和同事聊天。你的回复会原样发给对方。

对方刚明确说要重跑/再试一件刚失败的事。用一两句人话告诉对方：你正派人重新去做那件事——时态是正在重试，不是已经做完。

规矩：
- 像同事当面说，不要工单腔，不要「我这就去确认」「收到」「稍等」。
- 不要复述任务标题或原要求，也不要用书名号/引号把标题括回去——对方刚说了重试，知道是哪件；用「那事」「那块」指代即可。
- 不要提优先级、任务编号、工作流、沙箱、跟进页面、Approving。
- 禁止「已经重新跑过了 / 已经重试过了 / 重新重试过了 / 已经进到队列」——现在才刚开干，还在进行中。
- 只输出要发给对方的那句话。`

const fallthroughAckPhrasePrompt = `你是这个项目的负责人本人，正在 IM 上和同事聊天。你的回复会原样发给对方。

你这边还要再查一下才能答，人手马上接着干。用一两句人话接住对方。

规矩：
- 像同事当面说，不要工单腔，不要「我这就去确认」「收到」「稍等」。
- 不要复述对方原话，也不要用书名号把长标题括回去。
- 时态是正在查/正在弄，不是已经查完。
- 不要提优先级、任务编号、工作流、沙箱、跟进页面、Approving。
- 只输出要发给对方的那句话。`

const dispatchAckPhrasePrompt = `你是这个项目的负责人本人，正在 IM 上和同事聊天。你的回复会原样发给对方。

你刚把一件事派人去干了。用一两句人话告诉对方你正让人做这件事——时态是正在做，不是做完了。

规矩：
- 像同事当面说，不要工单腔，不要「我这就去确认」「收到」「稍等」。
- 不要复述完整任务标题或原要求；用「那事」「那块」或极短口语指代即可。
- 不要提优先级、任务编号、工作流、沙箱、跟进页面、Approving。
- 禁止「已经重试过了 / 已经跑完了 / 已经进到队列」。
- 只输出要发给对方的那句话。`

const refineAckPhrasePrompt = `你是这个项目的负责人本人，正在 IM 上和同事聊天。你的回复会原样发给对方。

对方刚补充/收窄了正在做的事。用一两句人话告诉对方你会按新重点继续——时态是接着做，不是做完了。

规矩：
- 像同事当面说，不要工单腔，不要「我这就去确认」「收到」「稍等」。
- 不要复述完整任务标题；不要用书名号把标题括回去。
- 不要提优先级、任务编号、工作流、沙箱、跟进页面、Approving。
- 只输出要发给对方的那句话。`

func (m *Manager) latestRetryableTerminal(rc *runningChannel, in InboundMessage) *models.TaskIdentity {
	if m.taskContext == nil {
		return nil
	}
	rows, err := m.taskContext.RecentTerminalTasksForConversation(
		m.taskScopeFor(rc, in), ledgerLimit, recentTerminalWindow)
	if err != nil || len(rows) == 0 {
		return nil
	}
	var cancelled *models.TaskIdentity
	for i := range rows {
		switch strings.ToLower(strings.TrimSpace(rows[i].Status)) {
		case "failed":
			return &rows[i]
		case "cancelled", "canceled":
			if cancelled == nil {
				cancelled = &rows[i]
			}
		}
	}
	return cancelled
}

// looksLikeRetryAffirmation detects short "yes, retry that" replies. Negations
// ("不要重跑") never match. Longer free-form messages stay with the model.
func looksLikeRetryAffirmation(text string) bool {
	t := strings.TrimSpace(text)
	if t == "" {
		return false
	}
	lower := strings.ToLower(t)
	for _, neg := range []string{"不要", "别", "不用", "别再", "don't", "do not", "no "} {
		if strings.Contains(lower, neg) {
			return false
		}
	}
	if len([]rune(t)) > 24 {
		return false
	}
	for _, a := range []string{
		"重跑", "再跑", "重试", "再试", "再来一次", "重新做", "重新派", "再弄",
		"继续做", "接着做", "接着干", "继续",
		"retry", "try again", "run again", "do it again",
	} {
		if lower == a || strings.Contains(lower, a) {
			return true
		}
	}
	return false
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
		focus := m.focusTaskID(rc, in)
		return liveOutcome{}, encodeToolResult(map[string]any{
			"rejected":       "这个会话同时在跑的任务已经到上限了，没有派下去。",
			"running":        running,
			"focus_task_id":  focus,
			"hint":           "若对方是在补充/收窄正在聊的那件事，改用 refine_work（可挂到 focus_task_id），不要让对方从队列里选。只有对方明确要另开一件完全不同的新事时，才简短问要先停哪件。",
		})
	}

	dispatch := &WorkDispatch{
		Brief: brief, Difficulty: difficulty, ShortTitle: title,
		Attachments: m.conversationAttachments(rc, in),
	}
	m.ensureTaskIdentity(rc, in, dispatch)

	// Prefer a spoken ack before the sandbox turn starts. Never stitch a
	// platform sentence — Live-phrased user_reply, or a tiny Live hop, or omit.
	userReply := strings.TrimSpace(args["user_reply"])
	liveOnly := strings.TrimSpace(args["ack_mode"]) == "live_only"
	text, flags := gateAcknowledgement(userReply, title, in.Text)
	rec.flag(flags...)
	if text == "" && !liveOnly {
		text = m.phraseDispatchAck(ctx, in.Text, title, brief)
		if text != "" {
			rec.flag("dispatch_ack_live")
		} else {
			rec.flag("ack_omitted")
		}
	} else if text == "" {
		rec.flag("ack_omitted_live_only")
	}
	if text != "" {
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
	}

	return liveOutcome{
		reason: brief, dispatch: dispatch, sampleID: rec.commit(routeDispatch),
	}, ""
}

func (m *Manager) phraseDispatchAck(ctx context.Context, userText, title, brief string) string {
	user := "对方说：" + strings.TrimSpace(userText) + "\n"
	if title != "" && !echoesUserText(title, userText) {
		user += "（内部参考，勿原样复述）短标题：" + title + "\n"
	}
	if brief != "" {
		user += "（内部参考，勿原样复述）派下去的要求：" + truncateRunes(brief, 160) + "\n"
	}
	out := strings.TrimSpace(m.phraseThroughLive(ctx, dispatchAckPhrasePrompt, user))
	if out == "" || strings.Contains(out, "确认") || spokenLineSoundsFinished(out) {
		return ""
	}
	if retryAckEchoesBrief(out, title, brief) {
		return ""
	}
	return out
}

// refineWork hangs a follow-up onto an existing task instead of opening a new
// one. This is the intentional path for "重点看 Release 到现在": the director
// recognizes a scope change, the ledger stays the same size, and the user is
// not asked to triage a queue.
func (m *Manager) refineWork(ctx context.Context, rc *runningChannel, in InboundMessage,
	args map[string]string, rec *sampleRecorder) (outcome liveOutcome, refused string) {
	addition := strings.TrimSpace(args["request"])
	if addition == "" {
		addition = strings.TrimSpace(in.Text)
	}
	if addition == "" {
		return liveOutcome{}, encodeToolResult(map[string]any{
			"rejected": "没有收到要补充的内容。",
		})
	}
	target := m.resolveLedgerTask(rc, in, args["task_id"])
	if target == nil {
		tasks := m.taskLedger(rc, in)
		return liveOutcome{}, encodeToolResult(map[string]any{
			"rejected": "找不到要补充的任务。",
			"running":  tasks,
			"hint":     "有明确 taskId 就填上；只有一件在跑或你们刚在聊一件时可以不填。若其实是全新的事，改用 dispatch_pm。",
		})
	}

	m.rememberTaskRefinement(rc, in, target, addition)

	difficulty := DifficultyLookup
	if strings.TrimSpace(args["difficulty"]) != "" {
		difficulty = parseDifficulty(args["difficulty"])
	}
	brief := "【补充要求】" + addition + "（挂在已有任务「" + target.ShortTitle + "」上，按这个重点继续，不要另开新任务）"
	dispatch := &WorkDispatch{
		Brief: brief, Difficulty: difficulty,
		TaskID: target.TaskID, ShortTitle: target.ShortTitle,
		Attachments: m.conversationAttachments(rc, in),
	}

	userReply := strings.TrimSpace(args["user_reply"])
	text, flags := gateAcknowledgement(userReply, target.ShortTitle, in.Text)
	rec.flag(flags...)
	if text == "" {
		user := "对方说：" + strings.TrimSpace(in.Text) + "\n"
		user += "补充重点：" + truncateRunes(addition, 120) + "\n"
		if t := services.SanitizeShortTitle(target.ShortTitle); t != "" {
			user += "（内部参考，勿原样复述）原任务：" + t + "\n"
		}
		text = strings.TrimSpace(m.phraseThroughLive(ctx, refineAckPhrasePrompt, user))
		if text == "" || strings.Contains(text, "确认") || spokenLineSoundsFinished(text) ||
			retryAckEchoesBrief(text, target.ShortTitle, addition) {
			text = ""
			rec.flag("refine_ack_omitted")
		} else {
			rec.flag("refine_ack_live")
		}
	}
	rec.flag("refine_work")
	if text != "" {
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
	return liveOutcome{
		reason: brief, dispatch: dispatch, sampleID: rec.commit(routeRefine),
	}, ""
}

// resolveLedgerTask picks the task a refine/cancel should act on.
func (m *Manager) resolveLedgerTask(rc *runningChannel, in InboundMessage, taskID string) *ledgerEntry {
	tasks := m.taskLedger(rc, in)
	taskID = strings.TrimSpace(taskID)
	switch {
	case taskID != "":
		for i := range tasks {
			if tasks[i].TaskID == taskID || tasks[i].RunID == taskID {
				return &tasks[i]
			}
		}
		return nil
	case len(tasks) == 1:
		return &tasks[0]
	case len(tasks) > 1:
		if focus := m.focusTaskID(rc, in); focus != "" {
			for i := range tasks {
				if tasks[i].TaskID == focus {
					return &tasks[i]
				}
			}
		}
	}
	return nil
}

// rememberTaskRefinement writes the follow-up into the ledger and reasserts focus.
func (m *Manager) rememberTaskRefinement(rc *runningChannel, in InboundMessage, target *ledgerEntry, addition string) {
	if m.taskContext == nil || target == nil {
		return
	}
	scope := m.taskScopeFor(rc, in)
	identity, err := m.taskContext.UpdateIdentity(services.EnsureTaskIdentityInput{
		RunID: target.RunID, ProjectID: rc.cfg.ProjectID, UserID: scope.UserID,
		RecentContext: addition, Status: "running",
	})
	if err != nil || identity == nil {
		log.Debug().Err(err).Str("task", target.TaskID).Msg("task refinement not recorded in the ledger")
		return
	}
	if _, err := m.taskContext.SetFocus(scope, identity, identity.Language); err != nil {
		log.Debug().Err(err).Msg("conversation focus not updated for refined task")
	}
	if msgID := strings.TrimSpace(in.MessageID); msgID != "" {
		if err := m.taskContext.BindMessage(scope, msgID, identity); err != nil {
			log.Debug().Err(err).Msg("refined task not bound to the inbound message")
		}
	}
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
	started := time.Now()
	if m.live == nil || !m.live.Configured() || plain == "" {
		m.appendTraceSpan(in.TraceID, finishSpan("director_report", "skipped", "degraded", started))
		return plain, true
	}
	callCtx, cancel := context.WithTimeout(ctx, m.liveCallTimeout(directorReportFallbackTimeout))
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
		log.Info().Err(err).Str("project", rc.cfg.ProjectID).Str("trace", in.TraceID).
			Msg("conclusion reported in the work layer's own words; the conversation model did not phrase it")
		detail := "empty"
		if err != nil {
			detail = err.Error()
		}
		m.appendTraceSpan(in.TraceID, finishSpan("director_report", "error", detail, started))
		return plain, true
	}
	m.appendTraceSpan(in.TraceID, finishSpan("director_report", "ok", "", started))
	return strings.TrimSpace(res.Text), false
}

// liveCallTimeout prefers the settings-page live_timeout_seconds so director
// report / synthesis cannot sit on a shorter hard-coded ceiling while route
// uses 300s.
func (m *Manager) liveCallTimeout(fallback time.Duration) time.Duration {
	if m != nil && m.live != nil {
		if d := m.live.Timeout(); d > 0 {
			return d
		}
	}
	if fallback > 0 {
		return fallback
	}
	return directorReportFallbackTimeout
}

// directorReportFallbackTimeout is used only when Live has no configured
// timeout (tests / miswired client). Production picks up live_timeout_seconds.
const directorReportFallbackTimeout = 45 * time.Second

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
- 不要出现任务编号、工作流名、执行环境、工具名、优先级这些内部说法。
- 不要说「你那边跟进看看」「请前往 Approving」这类把人打发走的话。
- 若事实只是刚重新开跑 / 还在队列或执行中，用现在进行时（正在重试、刚派下去），禁止「已经重试过了 / 重新重试过了 / 已经跑完了」。
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

// gateAcknowledgement accepts a Live-authored acknowledgement, or rejects
// filler / empty lines. It never stitches a platform sentence — that was how
// 「我这就去确认」leaked back into GM voice.
func gateAcknowledgement(reply, shortTitle, userText string) (string, []string) {
	_ = shortTitle
	_ = userText
	text := strings.TrimSpace(reply)
	if informative := stripFiller(text); len([]rune(informative)) >= 6 {
		if strings.Contains(text, "确认") || spokenLineSoundsFinished(text) {
			return "", []string{"empty_ack"}
		}
		return text, nil
	}
	return "", []string{"empty_ack"}
}

func echoesUserText(title, userText string) bool {
	user := strings.TrimSpace(userText)
	t := strings.TrimSpace(title)
	if t == "" || user == "" {
		return false
	}
	return t == user || strings.HasPrefix(user, t)
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
			Description: "把一件新事派给能读代码仓库、能查真实状态、能动手干活的人。" +
				"对方是在补充/收窄已有任务时不要用这个，改用 refine_work。",
			Params: []liveagent.Param{
				{Name: "request", Description: "用一句话说清对方要什么，供接手的人直接开工。", Required: true},
				{
					Name:        "difficulty",
					Description: "lookup=查一下就知道；heavy=要花好几分钟。",
					Enum:        []string{string(DifficultyLookup), string(DifficultyHeavy)},
					Required:    true,
				},
				{Name: "short_title", Description: "给这件事起个对方看得懂的短名字，例如「登录页报错」。", Required: true},
				{Name: "user_reply", Description: "现在就发给对方的一句话，必填；活人话说明你正派人去做，时态是正在做不是做完了；不要复述长标题。", Required: true},
			},
		},
		{
			Name: refineWorkTool,
			Description: "把对方的补充、收窄或新重点挂到已有任务上继续查，不新开任务、也不占并发名额。" +
				"例如「重点看 Release 到现在」「别看旧分支」——认准是跟进就用它。",
			Params: []liveagent.Param{
				{Name: "request", Description: "用一句话说清这次补充/收窄的重点。", Required: true},
				{Name: "task_id", Description: "挂到哪件任务；不填则用你们正在聊的那件（focus），或唯一在跑的那件。"},
				{
					Name:        "difficulty",
					Description: "lookup=查一下就知道；heavy=要花好几分钟。默认 lookup。",
					Enum:        []string{string(DifficultyLookup), string(DifficultyHeavy)},
				},
				{Name: "user_reply", Description: "现在就发给对方的一句话；活人话说明按新重点接着做；不要复述长标题。"},
			},
		},
		{
			Name: getStatusTool,
			Description: "查这个会话里任务的真实状态（在跑的 + 刚结束的 recent_terminal）。" +
				"要跟对方讲进度、状态或结论之前必须先调它，不要凭印象说。" +
				"recent_terminal.status 为 failed/cancelled/completed 时必须照实转述；空的在跑列表不等于都成功了。" +
				"若有 failed/cancelled：回复里要让对方选下一步（重试 / 换做法 / 先搁置），不要擅自继续派活。",
			Params: []liveagent.Param{
				{Name: "task_id", Description: "只查某一件事时填它的 taskId；不填就列出在跑的和刚结束的。"},
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
