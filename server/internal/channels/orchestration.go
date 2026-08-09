package channels

import (
	"context"
	"errors"
	"strings"

	"github.com/cocofhu/approving/internal/models"
	"github.com/cocofhu/approving/internal/sendable"
	"github.com/cocofhu/approving/internal/services"

	"github.com/rs/zerolog/log"
)

// RiskActionExecutor performs a confirmed high-risk action. Injected from main
// so channels stay free of engine/pmmcp imports.
type RiskActionExecutor func(projectID, runID, action string, meta map[string]string) error

// RunLifecycleHooks lets workflow/MCP layers notify IM when a real Run is
// accepted or when orchestration has a structured progress snapshot.
type RunLifecycleHooks struct {
	OnRunAccepted func(ctx context.Context, projectID, runID, userID string) error
}

func (m *Manager) SetRiskActionExecutor(exec RiskActionExecutor) {
	m.riskExecutor = exec
}

// handleFastPath is the only non-model shortcut left in the Live inbound
// pipeline: it settles a confirmation the platform has already put in front of
// the user.
//
// Deciding what a message means is the model's job — liveagent when a
// conversation model is configured, the sandbox agent otherwise. Keyword tables
// cannot tell "修复登录页" from "这两个都修一下", and when they guessed wrong the
// platform answered on the agent's behalf and the turn never reached anything
// that could read the conversation. A pending confirmation ticket is the one
// exception, and not because it is faster: the ticket is a form the platform
// showed the user, so a paraphrase must never be re-interpreted into
// authorizing a destructive action.
//
// Everything else continues into the routing layers with its text and its
// attachments intact.
func (m *Manager) handleFastPath(ctx context.Context, rc *runningChannel, in *InboundMessage) bool {
	if in == nil || m.riskConfirmation == nil {
		return false
	}
	text := strings.TrimSpace(in.Text)
	if text == "" {
		return false
	}
	// Only an explicit confirm/cancel answer settles a ticket. Anything else,
	// including a request that sounds destructive, goes to the model: creating
	// the ticket is the agent's call, made through the MCP write it guards.
	if services.ParseRiskDecisionPublic(text) == "" {
		return false
	}
	return m.tryResolveRiskConfirmation(ctx, rc, *in,
		services.SyntheticQQUserID(in.UserID), text, services.DetectLanguage(text, ""))
}

func (m *Manager) tryResolveRiskConfirmation(ctx context.Context, rc *runningChannel, in InboundMessage, scopeUser, text, language string) bool {
	// Only a question the user actually received can be what they just answered.
	ticket, err := m.riskConfirmation.LatestAnswerable(scopeUser, rc.cfg.ProjectID)
	if err != nil {
		log.Warn().Err(err).Msg("list pending risk ticket failed")
		return false
	}
	if ticket == nil {
		// Duplicate confirm/cancel against an already settled ticket.
		ticket, err = m.riskConfirmation.LatestAny(scopeUser, rc.cfg.ProjectID)
		if err != nil || ticket == nil || ticket.Status == "pending" {
			return false
		}
	}
	resolved, err := m.riskConfirmation.ResolveTicket(services.RiskTicketInput{
		ProjectID: rc.cfg.ProjectID, UserID: scopeUser,
		RunID: ticket.RunID, Action: ticket.Action, Language: language,
	}, text)
	if err != nil {
		log.Warn().Err(err).Msg("risk ticket resolve failed")
		return false
	}
	message := resolved.Message
	if resolved.Execute && m.riskExecutor != nil {
		meta := map[string]string{}
		if node := riskMetaNode(ticket.Action); node != "" {
			meta["nodeId"] = node
		}
		if action := riskMetaGateAction(ticket.Action); action != "" {
			meta["gateAction"] = action
		}
		if err := m.riskExecutor(rc.cfg.ProjectID, ticket.RunID, riskBaseAction(ticket.Action), meta); err != nil {
			log.Warn().Err(err).Str("run", ticket.RunID).Str("action", ticket.Action).
				Msg("confirmed risk action failed")
			// The user confirmed and it still did not happen. Say that plainly;
			// the underlying error is a diagnostic, not an answer.
			m.sendOrchestrationReply(ctx, rc, in, m.speakOperationalLine(ctx, operationalLine{
				Situation: "对方确认要做的那个操作没执行成功，状态一点没变。如实说清楚，再问他要不要你再试一次。",
				Language:  language,
				Fallback:  riskExecutionFailedText(language),
			}))
			return true
		}
		// Re-render now that the action has run. The text ResolveTicket returned
		// was built from the status as it was before the cancel, which is how
		// 「已经取消了。现在是 running」 reached a user.
		message = m.riskConfirmation.StatusMessageFor(resolved.Ticket)
	}
	m.sendOrchestrationReply(ctx, rc, in, message)
	return true
}

// riskExecutionFailedText reports a confirmed action that did not go through.
// It says what the user needs to know — it did not happen, and nothing changed
// — without repeating the internal error that explains why.
func riskExecutionFailedText(language string) string {
	if services.NormalizeLanguage(language) == "en" {
		return "I couldn't carry that out just now, so nothing has changed. Want me to try again?"
	}
	return "这个操作没执行成功，状态没变。要我再试一次吗？"
}

func (m *Manager) sendOrchestrationReply(ctx context.Context, rc *runningChannel, in InboundMessage, text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	m.sendOutbound(ctx, rc, OutboundMessage{
		Scene: in.Scene, ConversationID: in.ConversationID,
		ReplyToMessageID: in.MessageID, Text: text,
		Envelope: turnEnvelope(rc, in, sendable.KindActionRequired, "orchestration_reply", sendable.PriorityCritical),
	})
}

// ReportRunProgress is the orchestration-layer path for a fact the work layer
// reported about a Run in flight.
//
// Two things changed here from the obvious "deliver what the worker wrote".
//
// The fact is recorded on the task ledger whatever happens to the message,
// because the ledger is what answers "how's it going" later, and it used to be
// written only on the inbound-turn path — so a report that arrived through the
// MCP tool left no trace at all and a follow-up question was answered from the
// status the task had when it was created.
//
// And plain progress no longer interrupts anyone. A worker that reports every
// step turns the conversation into a build log, which is what made people stop
// reading it; the interesting cases (blocked, needs a decision, done) still
// come through, and long-running work is surfaced by the heartbeat instead —
// on the platform's schedule, not the worker's.
func (m *Manager) ReportRunProgress(ctx context.Context, req SendableRequest) (DeliveryResult, error) {
	if req.Kind == "" {
		req.Kind = sendable.KindProgress
	}
	if req.Reason == "" {
		req.Reason = ReasonPMNotifyProgress
	}
	if req.Priority == "" {
		req.Priority = sendable.PriorityNormal
	}
	if req.Kind == sendable.KindBlocked || req.Kind == sendable.KindActionRequired {
		req.Priority = sendable.PriorityCritical
	}
	m.noteWorkProgress(req.ProjectID, req.RunID, reportedStage(req), req.Progress.Blocked)

	if req.Kind == sendable.KindProgress {
		return DeliveryResult{Decision: sendable.Decision{Reason: ReasonLedgerOnly}}, nil
	}
	// The user hears one voice. The worker supplies the facts — it is the only
	// layer that checked them — and the conversation layer says them the way it
	// says everything else. Phrasing that is unavailable degrades to the
	// worker's own words rather than to silence.
	if phrased := m.phraseRunReport(ctx, req); strings.TrimSpace(phrased) != "" {
		req.Text = phrased
	}
	return m.DeliverSendable(ctx, req)
}

// reportedStage is the one line worth remembering from a report: what is
// happening right now, falling back to the conclusion and then to the message.
func reportedStage(req SendableRequest) string {
	for _, candidate := range []string{
		req.Progress.Stage, req.Progress.Conclusion, req.Text,
	} {
		if s := strings.TrimSpace(candidate); s != "" {
			return s
		}
	}
	return ""
}

// phraseRunReport hands the report to the conversation layer to say. Returns
// empty when there is nothing to phrase against, which leaves the worker's
// text as-is.
func (m *Manager) phraseRunReport(ctx context.Context, req SendableRequest) string {
	identity := m.identityForRun(req.RunID, req.ProjectID)
	if identity == nil {
		return ""
	}
	language := taskLanguage(identity)
	return m.synthesizeForTask(ctx, identity, normalizeScene(req.Scene), req.ConversationID,
		req.TraceID, runReportBrief(identity, req, language), req.Text)
}

// runReportRunes bounds how much of the worker's own text may be quoted into
// the brief. Same reasoning as a pause ask: it was written for the platform,
// not for the user, so the readable part goes in and the work log does not.
const runReportRunes = 200

// runReportBrief states what the conversation layer may say about a report
// from the work layer.
func runReportBrief(identity *models.TaskIdentity, req SendableRequest, language string) string {
	var b strings.Builder
	switch req.Kind {
	case sendable.KindBlocked:
		b.WriteString("后台任务卡住了，干不下去，请用一到两句话说清楚卡在哪。\n")
	case sendable.KindActionRequired:
		b.WriteString("后台任务需要用户拍板才能继续，请用一到两句话说清楚要对方定什么。\n")
	default:
		b.WriteString("后台任务有结论要交给用户，请用一到两句话转述。\n")
	}
	if title := services.SanitizeShortTitle(identity.ShortTitle); title != "" {
		b.WriteString("任务：" + title + "\n")
	}
	if want := strings.TrimSpace(identity.OriginalRequirement); want != "" {
		b.WriteString("用户当初的要求：" + truncateRunes(want, 200) + "\n")
	}
	if stage := strings.TrimSpace(req.Progress.Stage); stage != "" {
		b.WriteString("当前进行到：" + truncateRunes(stage, 120) + "\n")
	}
	said := leadingConclusion(req.Progress.Conclusion, runReportRunes)
	if said == "" {
		said = leadingConclusion(req.Text, runReportRunes)
	}
	if said != "" {
		b.WriteString("执行方原话：" + said + "\n")
	} else {
		b.WriteString("这一轮没有留下可读的内容。如实说情况，不要编具体细节。\n")
	}
	b.WriteString("要求：说人话，像同事顺口说一句；不要出现任务编号、工作流名、节点名、执行环境、工具名；")
	b.WriteString("不要说「请前往 Approving 查看」。")
	if req.Kind != sendable.KindFinal {
		b.WriteString("不要说任务已经完成或失败——它还没结束。")
	}
	if services.NormalizeLanguage(language) == "en" {
		b.WriteString("\n用英文回答。")
	}
	return b.String()
}

// EnsureRunAccepted materializes task identity and sends the once-per-run ACK.
func (m *Manager) EnsureRunAccepted(ctx context.Context, projectID, runID, qqUserID, conversationID string, scene Scene, language string) (DeliveryResult, error) {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return DeliveryResult{}, errors.New("run acceptance requires a real run id")
	}
	shortTitle := runID
	userID := services.SyntheticQQUserID(qqUserID)
	if m.taskContext != nil {
		var run models.Run
		if err := m.taskContext.DB().Where("id = ?", runID).First(&run).Error; err == nil {
			identity, err := m.taskContext.EnsureIdentityForRun(run, projectID, userID)
			if err == nil && identity != nil {
				shortTitle = identity.ShortTitle
			}
		}
	}
	scene = normalizeScene(scene)
	return m.SendRunAcceptanceAck(ctx, RunAcceptanceAck{
		ProjectID: projectID, RunID: runID, Scene: scene,
		ConversationID: conversationID, UserID: qqUserID,
		ShortTitle: shortTitle, Language: language,
	})
}

func riskBaseAction(action string) string {
	action = strings.TrimSpace(action)
	if i := strings.IndexByte(action, ':'); i >= 0 {
		return action[:i]
	}
	return action
}

func riskMetaNode(action string) string {
	// action form: resume_gate:nodeId:approve
	parts := strings.Split(action, ":")
	if len(parts) >= 2 && parts[0] == "resume_gate" {
		return parts[1]
	}
	return ""
}

func riskMetaGateAction(action string) string {
	parts := strings.Split(action, ":")
	if len(parts) >= 3 && parts[0] == "resume_gate" {
		return parts[2]
	}
	if parts[0] == "approve_gate" {
		return "approve"
	}
	if parts[0] == "reject_gate" {
		return "reject"
	}
	return ""
}
