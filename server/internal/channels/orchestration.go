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

// handleInboundOrchestration runs before the PM turn: risk confirmation,
// high-risk intent tickets, and natural-language task addressing. Returns true
// when the inbound message was fully handled (do not start a PM turn).
func (m *Manager) handleInboundOrchestration(ctx context.Context, rc *runningChannel, in *InboundMessage) bool {
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
	if isContinuation(text) {
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
		if isContinuation(text) {
			// Focus already renewed by ResolveTask; continue into the PM turn
			// with the resolved run context prepended.
			in.Text = prependFocusContext(text, res.Task)
			return false
		}
		if isStatusQuery(text) || isBareTaskSelection(text, res) || resolveText != text {
			// Re-render status with the original user wording language.
			msg := FormatTaskMessage(res.Task.ShortTitle, taskStatusLabel(res.Task.Status, language),
				"", text, language)
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
			m.sendOrchestrationReply(ctx, rc, in, resolved.Message+" "+friendlyErr(err))
			return true
		}
	}
	m.sendOrchestrationReply(ctx, rc, in, resolved.Message)
	return true
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
	needles := []string{"怎么样", "怎么了", "进度", "状态", "如何了", "好了吗", "how is", "how's", "status", "progress", "any update"}
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
		"的进度", "进度如何", "进度", "的状态", "状态", "如何了", "好了吗",
		"how is", "how's", "status of", "status", "progress on", "progress", "any update",
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

func detectHighRiskIntent(text string) (action, query string) {
	t := strings.TrimSpace(text)
	lower := strings.ToLower(t)
	type rule struct {
		action string
		keys   []string
	}
	rules := []rule{
		{"cancel_run", []string{"取消任务", "取消这个", "取消该", "取消运行", "cancel run", "cancel the task", "取消"}},
		{"approve_gate", []string{"批准", "同意门禁", "approve", "批准门禁"}},
		{"reject_gate", []string{"拒绝门禁", "驳回", "reject gate"}},
		{"delete_run", []string{"删除任务", "删掉", "delete task"}},
	}
	for _, r := range rules {
		for _, key := range r.keys {
			if strings.Contains(lower, strings.ToLower(key)) {
				query = strings.TrimSpace(strings.ReplaceAll(t, key, ""))
				query = strings.TrimSpace(strings.ReplaceAll(query, strings.ToUpper(key), ""))
				if query == "" {
					query = t
				}
				return r.action, query
			}
		}
	}
	return "", ""
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
