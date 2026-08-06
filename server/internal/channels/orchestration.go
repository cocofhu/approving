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

// handleFastPath answers the deterministic Live intents — task_query,
// task_control and clarification_reply — straight from stored task state.
//
// It runs before the per-conversation queue, so these messages are never
// blocked by an in-flight agent turn. Returns true when the message was fully
// handled; returns false (possibly after enriching in.Text with task context)
// when the agent still needs to see it.
func (m *Manager) handleFastPath(ctx context.Context, rc *runningChannel, in *InboundMessage) bool {
	if in == nil || (m.taskContext == nil && m.riskConfirmation == nil) {
		return false
	}
	text := strings.TrimSpace(in.Text)
	if text == "" {
		return false
	}
	scopeUser := services.SyntheticQQUserID(in.UserID)
	language := services.DetectLanguage(text, "")

	if m.riskConfirmation != nil {
		decision := services.ParseRiskDecisionPublic(text)
		if decision != "" {
			if handled := m.tryResolveRiskConfirmation(ctx, rc, *in, scopeUser, text, language); handled {
				return true
			}
		}
		if handled := m.tryCreateRiskConfirmation(ctx, rc, *in, scopeUser, text, language); handled {
			return true
		}
	}
	if m.taskContext == nil {
		return false
	}
	if !looksLikeTaskAddressing(text) && !m.hasTaskCandidate(rc, *in, text) {
		return false
	}
	resolveText := text
	amendment := looksLikeAmendment(text)
	if isContinuation(text) || amendment {
		resolveText = "" // force conversation-focus binding
	} else if isStatusQuery(text) {
		resolveText = stripStatusQueryNoise(text)
	}
	res, err := m.ResolveTaskReference(TaskReferenceRequest{
		ProjectID: rc.cfg.ProjectID, ChannelType: rc.cfg.Type, Scene: in.Scene,
		ConversationID: in.ConversationID, QQUserID: in.UserID, Text: resolveText,
	})
	if err != nil {
		log.Warn().Err(err).Str("project", rc.cfg.ProjectID).Msg("task reference resolve failed")
		return false
	}
	switch res.Status {
	case TaskReferenceAmbiguous, TaskReferenceNotFound, TaskReferenceNoContext:
		m.sendOrchestrationReply(ctx, rc, *in, res.Message)
		return true
	case TaskReferenceResolved:
		if res.Task == nil {
			return false
		}
		if amendment {
			// FR-3: revising an in-flight task must reach that Run instead of
			// silently becoming a new delegation. A finished task cannot absorb
			// the change, so say so rather than dropping it.
			if services.IsTerminalTaskStatus(res.Task.Status) {
				m.sendOrchestrationReply(ctx, rc, *in,
					amendAfterTerminalText(res.Task.ShortTitle, taskStatusLabel(res.Task.Status, language), language))
				return true
			}
			m.recordTaskContext(rc, res.Task, text)
			in.Text = prependAmendmentContext(text, res.Task)
			return false
		}
		if isContinuation(text) {
			// Focus already renewed by ResolveTask; continue into the PM turn
			// with the resolved run context prepended.
			m.recordTaskContext(rc, res.Task, text)
			in.Text = prependFocusContext(text, res.Task)
			return false
		}
		if isStatusQuery(text) || isBareTaskSelection(text, res) || resolveText != text {
			// Re-render status with the original user wording language.
			msg := m.formatTaskStatusReply(rc, *in, res.Task, text, language)
			m.sendOrchestrationReply(ctx, rc, *in, msg)
			return true
		}
		// Named a task but wants the agent to work — keep focus, continue turn.
		return false
	default:
		return false
	}
}

func (m *Manager) tryResolveRiskConfirmation(ctx context.Context, rc *runningChannel, in InboundMessage, scopeUser, text, language string) bool {
	ticket, err := m.riskConfirmation.LatestPending(scopeUser, rc.cfg.ProjectID)
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
			m.sendOrchestrationReply(ctx, rc, in, riskExecutionFailedText(language))
			return true
		}
	}
	m.sendOrchestrationReply(ctx, rc, in, resolved.Message)
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

func (m *Manager) tryCreateRiskConfirmation(ctx context.Context, rc *runningChannel, in InboundMessage, scopeUser, text, language string) bool {
	action, query := detectHighRiskIntent(text)
	if action == "" {
		return false
	}
	res, err := m.ResolveTaskReference(TaskReferenceRequest{
		ProjectID: rc.cfg.ProjectID, ChannelType: rc.cfg.Type, Scene: in.Scene,
		ConversationID: in.ConversationID, QQUserID: in.UserID, Text: query,
	})
	if err != nil {
		return false
	}
	if res.Status != TaskReferenceResolved || res.Task == nil {
		if res.Message != "" {
			m.sendOrchestrationReply(ctx, rc, in, res.Message)
			return true
		}
		return false
	}
	if action == "approve_gate" || action == "reject_gate" {
		gateAction := "approve"
		if action == "reject_gate" {
			gateAction = "reject"
		}
		var gate models.Gate
		err := m.taskContext.DB().Where("run_id = ? AND resolved = ?", res.Task.RunID, false).
			Order("iteration desc, id desc").First(&gate).Error
		if err != nil || strings.TrimSpace(gate.NodeID) == "" {
			kind, body := "需澄清", "未找到该任务当前待处理的门禁，请先确认任务门禁状态。"
			if language == "en" {
				kind, body = "Action required", "No pending gate was found for this task. Check its gate status first."
			}
			m.sendOrchestrationReply(ctx, rc, in,
				FormatTaskMessage(res.Task.ShortTitle, kind, body, text, language))
			return true
		}
		action = "resume_gate:" + gate.NodeID + ":" + gateAction
	}
	ticket, err := m.riskConfirmation.CreateTicket(services.RiskTicketInput{
		ProjectID: rc.cfg.ProjectID, UserID: scopeUser,
		RunID: res.Task.RunID, Action: action, Language: language,
		ShortTitle: res.Task.ShortTitle,
	})
	if err != nil {
		log.Warn().Err(err).Msg("create risk ticket failed")
		return false
	}
	m.sendOrchestrationReply(ctx, rc, in, m.riskConfirmation.ConfirmationPrompt(*ticket))
	return true
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

// ReportRunProgress is the orchestration-layer explicit progress sendable path.
func (m *Manager) ReportRunProgress(ctx context.Context, req SendableRequest) (DeliveryResult, error) {
	if req.Kind == "" {
		req.Kind = sendable.KindProgress
	}
	if req.Reason == "" {
		req.Reason = "explicit_progress"
	}
	if req.Priority == "" {
		req.Priority = sendable.PriorityNormal
	}
	if req.Kind == sendable.KindBlocked || req.Kind == sendable.KindActionRequired {
		req.Priority = sendable.PriorityCritical
	}
	return m.DeliverSendable(ctx, req)
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
	if scene == "" {
		scene = SceneC2C
	}
	return m.SendRunAcceptanceAck(ctx, RunAcceptanceAck{
		ProjectID: projectID, RunID: runID, Scene: scene,
		ConversationID: conversationID, UserID: qqUserID,
		ShortTitle: shortTitle, Language: language,
	})
}

func looksLikeTaskAddressing(text string) bool {
	if services.ParseOrdinal(text) > 0 {
		return true
	}
	if isContinuation(text) || isStatusQuery(text) {
		return true
	}
	// Short bare titles / keyword mentions are handled when Resolve finds them;
	// avoid hijacking every casual chat line.
	lower := strings.ToLower(text)
	needles := []string{"任务", "怎么样", "进度", "状态", "task", "status", "progress", "how is", "what's"}
	for _, n := range needles {
		if strings.Contains(lower, strings.ToLower(n)) {
			return true
		}
	}
	return false
}

func (m *Manager) hasTaskCandidate(rc *runningChannel, in InboundMessage, text string) bool {
	if m == nil || m.taskContext == nil || rc == nil {
		return false
	}
	text = strings.TrimSpace(text)
	if text == "" || strings.ContainsAny(text, "\r\n") || len([]rune(text)) > 80 {
		return false
	}
	candidates, err := m.taskContext.Search(services.TaskScope{
		ProjectID: rc.cfg.ProjectID, UserID: services.SyntheticQQUserID(in.UserID),
		Channel: rc.cfg.Type, ConversationID: in.ConversationID,
	}, text)
	return err == nil && len(candidates) > 0
}

func isContinuation(text string) bool {
	t := strings.TrimSpace(strings.ToLower(text))
	switch t {
	case "那继续吧", "继续吧", "继续", "接着做", "接着干", "go on", "continue", "keep going", "that continue":
		return true
	default:
		return strings.Contains(t, "继续") && len([]rune(t)) <= 12
	}
}

func isStatusQuery(text string) bool {
	t := strings.TrimSpace(text)
	if t == "" {
		return false
	}
	needles := []string{
		"怎么样", "怎么了", "什么情况", "到哪了", "到哪一步",
		"进度", "进展", "状态", "如何了", "好了吗", "完了吗", "完了没", "有结果了吗",
		"how is", "how's", "status", "progress", "any update", "done yet",
	}
	lower := strings.ToLower(t)
	for _, n := range needles {
		if strings.Contains(lower, strings.ToLower(n)) {
			return true
		}
	}
	return false
}

func stripStatusQueryNoise(text string) string {
	out := text
	for _, noise := range []string{
		"的事情怎么样了", "事情怎么样了", "怎么样了", "怎么样", "怎么了",
		"现在什么进展了", "什么进展了", "什么情况", "到哪一步了", "到哪了",
		"的进度", "进度如何", "进度", "进展", "的状态", "状态", "如何了",
		"好了吗", "完了吗", "完了没", "有结果了吗",
		"how is", "how's", "status of", "status", "progress on", "progress",
		"any update", "done yet",
		"的事情", "事情",
	} {
		out = strings.ReplaceAll(out, noise, " ")
		out = strings.ReplaceAll(out, strings.ToUpper(noise), " ")
	}
	return strings.Join(strings.Fields(strings.TrimSpace(out)), " ")
}

func isBareTaskSelection(text string, res TaskReferenceResult) bool {
	if res.Task == nil {
		return false
	}
	t := strings.TrimSpace(text)
	if services.ParseOrdinal(t) > 0 {
		return true
	}
	return strings.EqualFold(t, res.Task.ShortTitle)
}

func prependFocusContext(text string, task *models.TaskIdentity) string {
	if task == nil {
		return text
	}
	return "（当前焦点任务：" + task.ShortTitle + " / run " + task.RunID + "）\n" + text
}

// prependAmendmentContext tells the agent the user is revising an existing
// task, so it amends that Run instead of starting another one.
func prependAmendmentContext(text string, task *models.TaskIdentity) string {
	if task == nil {
		return text
	}
	return "（用户在修改已有任务的要求：" + task.ShortTitle + " / run " + task.RunID +
		"。请把下面的补充要求并入该任务，不要新建任务。）\n" + text
}

func amendAfterTerminalText(shortTitle, statusLabel, language string) string {
	if services.NormalizeLanguage(language) == "en" {
		return "\"" + shortTitle + "\" is already " + statusLabel +
			", so I can't fold that into it. Want me to start a new task for it?"
	}
	return "「" + shortTitle + "」已经" + statusLabel + "了，改不进去。要我按这个新开一个任务吗？"
}

// recordTaskContext keeps the task's most recent conversational context fresh
// so later status answers and background reflow can reference what the user
// actually said (FR-6).
func (m *Manager) recordTaskContext(rc *runningChannel, task *models.TaskIdentity, text string) {
	if m.taskContext == nil || task == nil || rc == nil {
		return
	}
	if _, err := m.taskContext.UpdateIdentity(services.EnsureTaskIdentityInput{
		RunID: task.RunID, ProjectID: rc.cfg.ProjectID, UserID: task.UserID,
		RecentContext: strings.TrimSpace(text),
		Language:      services.TaskLanguageFor(task.Language, text),
	}); err != nil {
		log.Warn().Err(err).Str("run", task.RunID).Msg("record task recent context failed")
	}
}

// formatTaskReply renders a task-scoped reply, naming the task only when the
// name carries information.
//
// It carries information in two cases: several tasks are live, so the answer
// would otherwise be ambiguous; or the user named the task themselves, where
// repeating it confirms which one was understood. Everywhere else the name is
// noise — a single-task conversation is already unambiguous, and stamping a
// label on every line is what made the old output read like a ticket queue.
func (m *Manager) formatTaskStatusReply(rc *runningChannel, in InboundMessage,
	task *models.TaskIdentity, currentMessage, recentLanguage string) string {
	if task == nil {
		return ""
	}
	if m.shouldNameTask(rc, in, task, currentMessage) {
		return FormatTaskMessage(task.ShortTitle,
			taskStatusLabel(task.Status, recentLanguage), "", currentMessage, recentLanguage)
	}
	return soloTaskStatusText(task.Status, recentLanguage)
}

func (m *Manager) shouldNameTask(rc *runningChannel, in InboundMessage,
	task *models.TaskIdentity, currentMessage string) bool {
	return m.liveTaskCount(rc, in) > 1 || mentionsTask(currentMessage, task.ShortTitle)
}

// mentionsTask reports whether the user's own words picked out this task, so a
// reply that repeats the name is confirming rather than labelling.
func mentionsTask(message, shortTitle string) bool {
	title := services.SanitizeShortTitle(shortTitle)
	msg := strings.ToLower(strings.TrimSpace(message))
	if title == "" || msg == "" {
		return false
	}
	lower := strings.ToLower(title)
	if strings.Contains(msg, lower) {
		return true
	}
	// A bare selection may be a prefix of the stored title ("结算页" for
	// "结算页性能"); short fragments are too weak to count as a reference.
	return len([]rune(msg)) >= 3 && strings.Contains(lower, msg)
}

// liveTaskCount counts this user's non-terminal tasks in the conversation.
func (m *Manager) liveTaskCount(rc *runningChannel, in InboundMessage) int {
	if m.taskContext == nil || rc == nil {
		return 0
	}
	n, err := m.taskContext.CountActive(services.TaskScope{
		ProjectID: rc.cfg.ProjectID, UserID: services.SyntheticQQUserID(in.UserID),
		Channel: rc.cfg.Type, ConversationID: in.ConversationID,
	})
	if err != nil {
		log.Warn().Err(err).Str("project", rc.cfg.ProjectID).Msg("count active tasks failed")
		return 0
	}
	return n
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
