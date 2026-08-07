package channels

import (
	"context"
	"errors"
	"strings"

	"github.com/cocofhu/approving/internal/liveagent"
	"github.com/cocofhu/approving/internal/sendable"

	"github.com/rs/zerolog/log"
)

// LiveModel is the conversation model consulted before a message reaches the
// sandbox. It answers what it can from the conversation alone and hands over
// everything else.
//
// Splitting the work this way is what makes an IM conversation feel like one:
// a sandbox turn costs a container and tens of seconds even to say hello, while
// most of what a colleague sends is chat, clarification, or a question about
// something already discussed. The model is not a filter in front of the agent
// — it cannot read the repository and is not asked to pretend otherwise — it
// just keeps the conversation moving while real work happens behind it.
type LiveModel interface {
	Configured() bool
	Complete(ctx context.Context, req liveagent.Request) (liveagent.Result, error)
}

// askProjectAgentTool is the only decision the conversation model can hand
// back. There is deliberately no "start a task" or "cancel that" tool: acting
// on the project is the agent's job, and a second place that can trigger it is
// a second place that can trigger it wrongly.
const askProjectAgentTool = "ask_project_agent"

// liveSystemPrompt tells the model who it is and where the line is.
//
// The line is drawn by capability, not topic: it may say anything it can
// support from the conversation in front of it, and must hand over anything
// that needs the repository, real task state, or an action. Uncertainty
// resolves toward handing over, because an invented answer is indistinguishable
// from a real one to the person reading it.
const liveSystemPrompt = `你是这个项目的负责人本人，正在 IM 上和同事聊天。你的回复会原样发给对方。

你可以直接回答：打招呼、闲聊、澄清对方的意思、解释你们刚才聊过的内容、回答你从这段对话里就能确定的事。

你必须调用 ask_project_agent 交给项目 Agent 的：需要看代码或仓库、需要知道任务的真实状态或进度、需要动手改东西、需要跑流程或发起工作、以及任何你不能仅凭这段对话就确定的事。

规则：
- 直接回答时只说一句自然的人话，就像微信上回同事。不要写"好的我看一下""稍等""已收到"这类过程话——你要么现在就回答，要么交出去。
- 不要编造任务状态、进度、代码内容或任何你没有依据的事实。宁可交给 Agent。
- 不要提到"沙箱""Agent""模型""工具""上下文"这些词，对方只是在和你聊天。
- 不确定就调用 ask_project_agent。`

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
	// answered is true when the user has already been replied to.
	answered bool
	// reason explains why the turn is being handed to the agent. Empty when the
	// model answered, or when there was no conversation model to ask.
	reason string
}

// routeThroughLiveModel asks the conversation model what to do with a message.
//
// Every failure mode ends in the same place: the sandbox. A model that is
// unconfigured, unreachable, out of budget or simply unsure must never cause a
// message to be dropped or answered with an apology — the agent can still
// handle it, just more slowly.
func (m *Manager) routeThroughLiveModel(ctx context.Context, rc *runningChannel, in InboundMessage) liveOutcome {
	if m.live == nil || !m.live.Configured() {
		return liveOutcome{}
	}
	req := liveagent.Request{
		System:    liveSystemPrompt,
		Messages:  m.liveMessages(rc, in),
		Tools:     []liveagent.ToolSpec{askProjectAgentSpec()},
		MaxTokens: liveMaxTokens,
	}
	res, err := m.live.Complete(ctx, req)
	if err != nil {
		// Not a user-visible failure: the message still gets answered, by the
		// agent. Logged so an endpoint that is failing every call is visible
		// as something other than "the assistant got slow".
		level := log.Warn()
		if errors.Is(err, liveagent.ErrNotConfigured) {
			level = log.Debug()
		}
		level.Err(err).Str("project", rc.cfg.ProjectID).Msg("conversation model unavailable; handing turn to the agent")
		return liveOutcome{reason: "会话层没能给出答复"}
	}
	if res.ToolName == askProjectAgentTool {
		reason := strings.TrimSpace(res.Args["request"])
		if reason == "" {
			reason = "需要读仓库或任务真实状态"
		}
		return liveOutcome{reason: reason}
	}
	answer := strings.TrimSpace(res.Text)
	if answer == "" {
		return liveOutcome{reason: "会话层没能给出答复"}
	}
	sent := m.sendOutboundResult(ctx, rc, OutboundMessage{
		Scene: in.Scene, ConversationID: in.ConversationID,
		ReplyToMessageID: in.MessageID, Text: answer,
		Envelope: turnEnvelope(rc, in, sendable.KindFinal, "live_reply", sendable.PriorityNormal),
	})
	if !sent.Sent {
		// The answer never reached anyone, so the turn is still unanswered.
		// Handing it to the agent is the only path that can still produce a
		// reply; claiming success here is what leaves a user waiting in silence.
		log.Warn().Str("project", rc.cfg.ProjectID).Str("reason", sent.Decision.Reason).
			Msg("conversation model answer was not delivered; handing turn to the agent")
		return liveOutcome{reason: "会话层的回复没有送达"}
	}
	m.markReplied(conversationTurnScope(rc.cfg.ProjectID, in.Scene, in.ConversationID))
	return liveOutcome{answered: true}
}

func askProjectAgentSpec() liveagent.ToolSpec {
	return liveagent.ToolSpec{
		Name: askProjectAgentTool,
		Description: "把这条消息交给能读代码仓库、能查任务真实状态、能动手干活的项目 Agent。" +
			"任何需要事实依据或需要执行的事情都用它，不要自己猜。",
		Params: []liveagent.Param{{
			Name:        "request",
			Description: "用一句话说清对方要什么，供 Agent 直接接手。",
			Required:    true,
		}},
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
