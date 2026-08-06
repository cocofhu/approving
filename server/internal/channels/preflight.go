package channels

import (
	"errors"
	"fmt"
	"strings"

	"github.com/cocofhu/approving/internal/models"
	"github.com/cocofhu/approving/internal/services"
	"gorm.io/gorm"
)

type PreflightDisposition string

const (
	PreflightProceed PreflightDisposition = "proceed"
	PreflightRespond PreflightDisposition = "respond"
)

type InboundPreflightRequest struct {
	Channel ResolvedChannel
	Message InboundMessage
}

// InboundOrchestrationResult separates a safe immediate response from an
// agent turn and carries only a ticket-bound action into the latter.
type InboundOrchestrationResult struct {
	Disposition      PreflightDisposition
	Reply            Reply
	Task             *models.TaskIdentity
	AuthorizedAction string
	TicketID         string
}

// TaskUserScope is the external identity used for task isolation. Group/guild
// conversations append the platform user so two members never share tasks.
func TaskUserScope(channelType string, in InboundMessage) string {
	base := SyntheticUserID(channelType, in.Scene, in.ConversationID)
	if in.Scene != SceneC2C && strings.TrimSpace(in.UserID) != "" {
		return base + "|user:" + strings.TrimSpace(in.UserID)
	}
	return base
}

func (b *ChannelBridge) PreflightInbound(req InboundPreflightRequest) (InboundOrchestrationResult, error) {
	if b == nil || b.tasks == nil {
		return InboundOrchestrationResult{Disposition: PreflightProceed}, nil
	}
	rc, in := req.Channel, req.Message
	userScope := TaskUserScope(rc.Type, in)
	channel := rc.Type
	focus, _ := b.tasks.GetConversationFocus(rc.ProjectID, channel, in.ConversationID, userScope)
	language := DetectIMLanguage(in.Text, "")
	if !containsLanguageSignal(in.Text) {
		if remembered := b.tasks.ConversationLanguage(rc.ProjectID, channel, in.ConversationID, userScope); remembered != "" {
			language = remembered
		}
	}
	_ = b.tasks.RememberConversationLanguage(rc.ProjectID, channel, in.ConversationID, userScope, language)

	if _, ok := services.ParseConfirmationReply(in.Text); ok {
		return b.consumeConfirmation(rc, in, userScope, language)
	}

	if selected, err := b.tasks.SelectPendingCandidate(
		rc.ProjectID, channel, in.ConversationID, userScope, in.Text,
	); err == nil {
		b.bindInbound(rc, in, userScope, selected, "")
		b.announceRunAcceptance(rc, in, userScope, language, selected)
		return b.preflightResponse(selected, "selected",
			localized(language, "已选择该任务。", "Task selected."), language, rc.ReplyMetadata), nil
	}

	var bound *models.TaskIdentity
	if strings.TrimSpace(in.ReplyToMessageID) != "" {
		binding, err := b.tasks.GetMessageBinding(rc.ProjectID, channel, in.ConversationID, in.ReplyToMessageID)
		if err == nil && binding.UserID == userScope {
			if task, taskErr := b.tasks.GetTaskIdentity(rc.ProjectID, userScope, binding.RunID); taskErr == nil {
				bound = &task
			}
		}
	}

	actionKind, highRisk := highRiskAction(in.Text)
	query := strings.TrimSpace(in.Text)
	if highRisk {
		query = stripActionWords(query)
	}
	task, resolution, resolveErr := b.resolveInboundTask(rc, in, userScope, query, bound, focus, highRisk || looksLikeTaskReference(in.Text))
	if errors.Is(resolveErr, services.ErrTaskAmbiguous) {
		runIDs := make([]string, 0, len(resolution.Candidates))
		for _, candidate := range resolution.Candidates {
			runIDs = append(runIDs, candidate.Task.RunID)
		}
		_ = b.tasks.SetAmbiguityCandidates(rc.ProjectID, channel, in.ConversationID, userScope, runIDs)
		return b.ambiguityResponse(resolution.Candidates, language, rc.ReplyMetadata), nil
	}
	if errors.Is(resolveErr, services.ErrTaskNotFound) && (highRisk || looksLikeTaskReference(in.Text)) {
		return b.genericResponse("not_found",
			localized(language, "未找到唯一匹配的任务，请提供短标题。", "No unique task matched; provide its short title."),
			language, rc.ReplyMetadata), nil
	}
	if resolveErr != nil {
		return InboundOrchestrationResult{}, resolveErr
	}
	if task == nil {
		return InboundOrchestrationResult{Disposition: PreflightProceed}, nil
	}
	_ = b.tasks.TouchConversationFocus(rc.ProjectID, channel, in.ConversationID, userScope, task.RunID)
	b.bindInbound(rc, in, userScope, *task, actionKind)
	b.announceRunAcceptance(rc, in, userScope, language, *task)

	if highRisk {
		ticket, err := b.tasks.CreateRiskConfirmationWithKind(rc.ProjectID, userScope, task.RunID, actionKind, in.Text)
		if err != nil {
			return InboundOrchestrationResult{}, err
		}
		message := localized(language,
			fmt.Sprintf("高风险操作“%s”等待确认；5 分钟内回复“确认”或“取消”。", actionKind),
			fmt.Sprintf("High-risk action %q awaits confirmation; reply confirm or cancel within 5 minutes.", actionKind))
		result := b.preflightResponse(*task, "action_required", message, language, rc.ReplyMetadata)
		result.TicketID = ticket.ID
		return result, nil
	}
	if isStatusQuery(in.Text) {
		return b.preflightResponse(*task, "status",
			localized(language, "当前状态："+task.Status, "Current status: "+task.Status), language, rc.ReplyMetadata), nil
	}
	return InboundOrchestrationResult{Disposition: PreflightProceed, Task: task}, nil
}

func (b *ChannelBridge) resolveInboundTask(
	rc ResolvedChannel,
	in InboundMessage,
	userScope, query string,
	bound *models.TaskIdentity,
	focus models.ConversationFocus,
	mustResolve bool,
) (*models.TaskIdentity, services.TaskResolution, error) {
	if bound != nil {
		return bound, services.TaskResolution{Task: bound}, nil
	}
	if focus.RunID != "" && (isContinuation(in.Text) || strings.TrimSpace(query) == "") {
		task, err := b.tasks.GetTaskIdentity(rc.ProjectID, userScope, focus.RunID)
		if err == nil {
			return &task, services.TaskResolution{Task: &task}, nil
		}
	}
	if !mustResolve {
		return nil, services.TaskResolution{}, nil
	}
	resolution, err := b.tasks.ResolveTask(services.TaskResolveRequest{
		ProjectID: rc.ProjectID, UserID: userScope, Query: query,
		Channel: rc.Type, ConversationID: in.ConversationID,
	})
	return resolution.Task, resolution, err
}

func (b *ChannelBridge) consumeConfirmation(rc ResolvedChannel, in InboundMessage, userScope, language string) (InboundOrchestrationResult, error) {
	result, err := b.tasks.ConsumeLatestRiskConfirmation(rc.ProjectID, userScope, in.Text)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return b.genericResponse("stale",
			localized(language, "没有待确认操作。", "There is no pending action."), language, rc.ReplyMetadata), nil
	}
	if errors.Is(err, services.ErrConfirmationStale) {
		status := ""
		if result.Latest != nil {
			status = result.Latest.Status
		}
		return b.genericResponse("stale",
			localized(language, "该确认已失效或已处理，最新状态："+status, "That confirmation is stale or consumed; latest state: "+status),
			language, rc.ReplyMetadata), nil
	}
	if err != nil {
		return InboundOrchestrationResult{}, err
	}
	task, err := b.tasks.GetTaskIdentity(rc.ProjectID, userScope, result.Ticket.RunID)
	if err != nil {
		return InboundOrchestrationResult{}, err
	}
	b.bindInbound(rc, in, userScope, task, result.Ticket.ActionKind)
	if result.Cancel {
		return b.preflightResponse(task, "cancelled",
			localized(language, "已取消，不会执行该操作。", "Cancelled; the action will not run."), language, rc.ReplyMetadata), nil
	}
	_ = b.tasks.TouchConversationFocus(rc.ProjectID, rc.Type, in.ConversationID, userScope, task.RunID)
	b.announceRunAcceptance(rc, in, userScope, language, task)
	return InboundOrchestrationResult{
		Disposition: PreflightProceed, Task: &task,
		AuthorizedAction: result.Ticket.Action, TicketID: result.Ticket.ID,
	}, nil
}

// announceRunAcceptance reports the first association of this conversation with
// a Run. Repeat associations are deduped by the delivery policy and receipt.
func (b *ChannelBridge) announceRunAcceptance(
	rc ResolvedChannel, in InboundMessage, userScope, language string, task models.TaskIdentity,
) {
	if b.accepted == nil || strings.TrimSpace(task.RunID) == "" {
		return
	}
	b.accepted(RunAcceptance{
		ProjectID: rc.ProjectID, Channel: rc.Type, Scene: in.Scene,
		ConversationID: in.ConversationID, UserID: userScope,
		RunID: task.RunID, ShortTitle: task.ShortTitle, Language: language,
	})
}

func (b *ChannelBridge) bindInbound(rc ResolvedChannel, in InboundMessage, userScope string, task models.TaskIdentity, action string) {
	if strings.TrimSpace(in.MessageID) == "" {
		return
	}
	_ = b.tasks.BindExternalMessage(models.MessageBinding{
		ProjectID: rc.ProjectID, Channel: rc.Type, ConversationID: in.ConversationID,
		MessageID: in.MessageID, UserID: userScope, RunID: task.RunID,
		NodeID: in.NodeID, GateID: in.GateID, Action: action, Direction: "inbound",
	})
}

func (b *ChannelBridge) preflightResponse(task models.TaskIdentity, typ, body, language string, replyMetadata bool) InboundOrchestrationResult {
	text := body
	if !replyMetadata {
		text = TaskMessagePrefix(task.ShortTitle, IMTypeLabel(typ, language)) + body
	}
	return InboundOrchestrationResult{
		Disposition: PreflightRespond, Task: &task,
		Reply: Reply{
			RunID: task.RunID, ShortTitle: task.ShortTitle,
			Final: &TurnFinalReport{OK: true, Summary: text},
		},
	}
}

func (b *ChannelBridge) genericResponse(typ, body, language string, replyMetadata bool) InboundOrchestrationResult {
	text := body
	if !replyMetadata {
		text = TaskMessagePrefix(localized(language, "任务", "Task"), IMTypeLabel(typ, language)) + body
	}
	return InboundOrchestrationResult{
		Disposition: PreflightRespond,
		Reply:       Reply{RunID: "preflight:" + deliveryKey(body), Final: &TurnFinalReport{OK: true, Summary: text}},
	}
}

func (b *ChannelBridge) ambiguityResponse(candidates []services.TaskCandidate, language string, replyMetadata bool) InboundOrchestrationResult {
	lines := []string{localized(language, "找到多个任务，请回复序号或完整短标题：", "Multiple tasks matched; reply with a number or exact short title:")}
	for i, candidate := range candidates {
		lines = append(lines, fmt.Sprintf("%d. %s (%s)", i+1, candidate.Task.ShortTitle, candidate.Task.Status))
	}
	return b.genericResponse("ambiguity", strings.Join(lines, "\n"), language, replyMetadata)
}

func localized(language, zh, en string) string {
	if language == "en" {
		return en
	}
	return zh
}

func containsLanguageSignal(text string) bool {
	for _, r := range text {
		if r > 127 || (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') {
			return true
		}
	}
	return false
}

func isStatusQuery(text string) bool {
	lower := strings.ToLower(text)
	return containsAny(lower, "状态", "进度", "怎么样", "如何了", "status", "progress", "how is")
}

func looksLikeTaskReference(text string) bool {
	lower := strings.ToLower(text)
	return isStatusQuery(text) || containsAny(lower, "任务", "这个", "那个", "继续", "task", "run ", "continue")
}

func isContinuation(text string) bool {
	lower := strings.ToLower(strings.TrimSpace(text))
	return containsAny(lower, "这个", "那个", "它", "继续", "当前", "status", "progress", "continue", "it")
}

func highRiskAction(text string) (string, bool) {
	lower := strings.ToLower(text)
	for kind, words := range map[string][]string{
		"cancel":    {"取消运行", "取消任务", "cancel"},
		"delete":    {"删除", "delete", "remove"},
		"authorize": {"授权", "authorize", "grant"},
		"approve":   {"批准", "审批通过", "同意执行", "approve"},
		"reject":    {"拒绝", "驳回", "reject"},
	} {
		for _, word := range words {
			if strings.Contains(lower, word) {
				return kind, true
			}
		}
	}
	return "", false
}

func stripActionWords(text string) string {
	replacer := strings.NewReplacer(
		"取消运行", "", "取消任务", "", "删除", "", "授权", "", "批准", "", "审批通过", "", "同意执行", "", "拒绝", "", "驳回", "",
		"cancel", "", "delete", "", "remove", "", "authorize", "", "grant", "", "approve", "", "reject", "",
		"任务", "", "run", "", "task", "",
	)
	return strings.TrimSpace(replacer.Replace(strings.ToLower(text)))
}
