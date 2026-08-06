package channels

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/cocofhu/approving/internal/models"
	"github.com/cocofhu/approving/internal/sendable"
	"github.com/cocofhu/approving/internal/services"
)

// ErrNoSendableTarget is returned when no running channel can reach the
// requested project conversation.
var ErrNoSendableTarget = errors.New("项目未配置可用的外发渠道目标")

// SendableRequest is the explicit external-delivery request used by
// orchestration. Callers must state a real RunID (Run lifecycle traffic) or a
// TaskContext (conversation-scoped traffic); an inbound platform message id is
// never a Run id.
type SendableRequest struct {
	ProjectID        string
	Scene            Scene
	ConversationID   string
	UserID           string
	ReplyToMessageID string

	RunID       string
	TaskContext string

	Kind      sendable.Kind
	Reason    string
	Priority  sendable.Priority
	DedupeKey string
	Progress  sendable.ProgressFields

	Text      string
	ImageURLs []string
}

// Envelope renders the request as a channel-bound delivery envelope. It is
// exported so callers can assert the delivery contract before sending.
func (r SendableRequest) Envelope() sendable.DeliveryEnvelope {
	priority := r.Priority
	if priority == "" {
		priority = sendable.PriorityNormal
	}
	e := sendable.DeliveryEnvelope{
		Priority: priority, RunID: strings.TrimSpace(r.RunID),
		TaskContext: strings.TrimSpace(r.TaskContext), ProjectID: r.ProjectID,
		ConversationID: r.ConversationID, UserID: r.UserID,
		DedupeKey: strings.TrimSpace(r.DedupeKey), Reason: r.Reason,
		Kind: r.Kind, Progress: r.Progress,
		// Everything routed through this API is composed by orchestration, not
		// copied from raw model output.
		Structured: true,
	}
	return sendable.AppendSendable(e, sendable.ChannelQQ)
}

// DeliveryResult reports whether an outbound attempt was sent, suppressed, or
// failed after policy evaluation and bounded transport retries.
type DeliveryResult struct {
	Decision sendable.Decision
	Sent     bool
}

// ErrDeliverySuppressed is returned when policy intentionally blocked a send.
var ErrDeliverySuppressed = errors.New("delivery suppressed by sendable policy")

// ErrDeliveryFailed is returned when transport retries are exhausted.
var ErrDeliveryFailed = errors.New("delivery failed after retries")

// DeliverSendable is the explicit external egress for orchestration. Every
// delivery still passes the Manager's single policy gate. Callers can tell
// sent / suppressed / failed apart from the result.
func (m *Manager) DeliverSendable(ctx context.Context, req SendableRequest) (DeliveryResult, error) {
	if strings.TrimSpace(req.Text) == "" {
		return DeliveryResult{}, errors.New("sendable text is empty")
	}
	target, scene, conv, err := m.resolveSendableTarget(req)
	if err != nil {
		return DeliveryResult{}, err
	}
	req.Scene, req.ConversationID = scene, conv
	if ctx == nil {
		ctx = m.baseCtx
	}
	result := m.sendOutboundResult(ctx, target, OutboundMessage{
		Scene: scene, ConversationID: conv, ReplyToMessageID: req.ReplyToMessageID,
		Text: req.Text, ImageURLs: req.ImageURLs, Envelope: req.Envelope(),
	})
	if result.Sent {
		return result, nil
	}
	switch result.Decision.Reason {
	case "no_adapter", "policy_error", "transport_failed", "retry_claim_failed":
		return result, ErrDeliveryFailed
	default:
		return result, ErrDeliverySuppressed
	}
}

// RunAcceptanceAck confirms that a real Run was accepted for a user. It is
// distinct from the per-turn processing ACK and is delivered at most once per
// run × conversation/user × channel.
type RunAcceptanceAck struct {
	ProjectID      string
	RunID          string
	Scene          Scene
	ConversationID string
	UserID         string
	ShortTitle     string
	Language       string
}

// SendRunAcceptanceAck delivers the once-per-run acceptance ACK.
func (m *Manager) SendRunAcceptanceAck(ctx context.Context, ack RunAcceptanceAck) (DeliveryResult, error) {
	runID := strings.TrimSpace(ack.RunID)
	if runID == "" {
		return DeliveryResult{}, errors.New("run acceptance ack requires a real run id")
	}
	language := services.DetectLanguage("", ack.Language)
	kindLabel := "已接单"
	body := "任务已接单，我会在有实质进展时同步。"
	if language == "en" {
		kindLabel, body = "Accepted", "Task accepted. I will report substantive progress."
	}
	return m.DeliverSendable(ctx, SendableRequest{
		ProjectID: ack.ProjectID, Scene: ack.Scene, ConversationID: ack.ConversationID,
		UserID: ack.UserID, RunID: runID,
		Kind: sendable.KindRunAcceptanceAck, Reason: "run_accepted",
		Priority:  sendable.PriorityHigh,
		DedupeKey: runAcceptanceDedupeKey(runID, ack.ConversationID, ack.UserID),
		Text:      services.FormatTaskType(ack.ShortTitle, kindLabel, language) + " " + body,
	})
}

func runAcceptanceDedupeKey(runID, conversationID, userID string) string {
	return strings.Join([]string{
		"run-ack", runID, conversationID, userID, string(sendable.ChannelQQ),
	}, ":")
}

func (m *Manager) resolveSendableTarget(req SendableRequest) (*runningChannel, Scene, string, error) {
	if strings.TrimSpace(req.ConversationID) != "" {
		m.mu.Lock()
		defer m.mu.Unlock()
		for _, rc := range m.running {
			if rc.cfg.ProjectID == req.ProjectID {
				scene := req.Scene
				if scene == "" {
					scene = SceneC2C
				}
				return rc, scene, req.ConversationID, nil
			}
		}
		return nil, "", "", ErrNoSendableTarget
	}
	target, scene, conv, err := m.lookupRunNotifyTarget(req.ProjectID)
	if err != nil {
		return nil, "", "", ErrNoSendableTarget
	}
	return target, scene, conv, nil
}

// TaskReferenceStatus is the outcome of resolving which task a user meant.
type TaskReferenceStatus string

const (
	TaskReferenceResolved  TaskReferenceStatus = "resolved"
	TaskReferenceAmbiguous TaskReferenceStatus = "ambiguous"
	TaskReferenceNotFound  TaskReferenceStatus = "not_found"
	TaskReferenceNoContext TaskReferenceStatus = "no_context"
)

// TaskReferenceRequest is one inbound attempt to address an existing task.
type TaskReferenceRequest struct {
	ProjectID      string
	ChannelType    string
	Scene          Scene
	ConversationID string
	QQUserID       string
	Text           string
	// ReplyToMessageID is honoured only when the channel actually supports
	// reply references; on QQ it is ignored and the user is told how to select.
	ReplyToMessageID string
}

// TaskReferenceResult carries the resolution and the natural-language reply the
// orchestration layer should surface.
type TaskReferenceResult struct {
	Status            TaskReferenceStatus
	Task              *models.TaskIdentity
	Options           []string
	Message           string
	Language          string
	ReplyRefSupported bool
	Reason            string
}

// ResolveTaskReference composes the IM-side task addressing flow: channel
// capability detection → reply binding (skipped when unsupported) → ordinal /
// short-title selection → conversation focus → scored search. Candidates are
// always confined to this project and this synthetic QQ identity.
func (m *Manager) ResolveTaskReference(req TaskReferenceRequest) (TaskReferenceResult, error) {
	svc := m.taskContext
	if svc == nil {
		return TaskReferenceResult{}, errors.New("task context service is not configured")
	}
	channelType := strings.TrimSpace(req.ChannelType)
	if channelType == "" {
		channelType = models.ChannelTypeQQ
	}
	scope := services.TaskScope{
		ProjectID:      req.ProjectID,
		UserID:         services.SyntheticQQUserID(req.QQUserID),
		Channel:        channelType,
		ConversationID: req.ConversationID,
	}
	language := m.referenceLanguage(svc, scope, req.Text)
	replySupported := SupportsReplyReference(channelType)

	result := TaskReferenceResult{Language: language, ReplyRefSupported: replySupported}
	text := strings.TrimSpace(req.Text)

	resolveIn := services.ResolveTaskInput{Scope: scope, Query: text}
	if replySupported {
		resolveIn.ReplyMessageID = req.ReplyToMessageID
	}
	if ordinal := services.ParseOrdinal(text); ordinal > 0 {
		// A bare number selects from the options the previous ambiguity prompt
		// actually showed the user.
		resolveIn.Ordinal = ordinal
		resolveIn.Query = ""
		resolveIn.Candidates = m.lastAmbiguityOptions(scope)
	} else {
		candidates, err := svc.Search(scope, text)
		if err != nil {
			return TaskReferenceResult{}, err
		}
		resolveIn.Candidates = candidates
	}

	res, err := svc.ResolveTask(resolveIn)
	if err != nil {
		return TaskReferenceResult{}, err
	}
	result.Reason = res.Reason
	switch {
	case res.Identity != nil:
		result.Status = TaskReferenceResolved
		result.Task = res.Identity
		result.Message = FormatTaskMessage(res.Identity.ShortTitle, taskStatusLabel(res.Identity.Status, language),
			"", req.Text, language)
	case res.Ambiguous:
		result.Status = TaskReferenceAmbiguous
		result.Options = shortTitles(res.Candidates)
		result.Message = ambiguityPrompt(result.Options, language, replySupported)
		m.rememberAmbiguity(scope, res.Candidates)
	case res.Reason == "focus_missing_or_expired":
		result.Status = TaskReferenceNoContext
		result.Message = noContextPrompt(language, replySupported)
	default:
		result.Status = TaskReferenceNotFound
		result.Message = notFoundPrompt(language, replySupported)
	}
	return result, nil
}

func (m *Manager) referenceLanguage(svc *services.TaskContextService, scope services.TaskScope, text string) string {
	recent := ""
	if focus, err := svc.GetFocus(scope, false); err == nil {
		recent = focus.Language
	}
	return services.DetectLanguage(text, recent)
}

// ambiguityMemory keeps the option list offered for the last ambiguous prompt so
// a following bare ordinal selects from exactly what the user saw.
type ambiguityMemory struct {
	candidates []services.TaskCandidate
	at         time.Time
}

func (m *Manager) rememberAmbiguity(scope services.TaskScope, candidates []services.TaskCandidate) {
	m.ambiguityMu.Lock()
	defer m.ambiguityMu.Unlock()
	if m.ambiguity == nil {
		m.ambiguity = map[string]ambiguityMemory{}
	}
	m.ambiguity[ambiguityKey(scope)] = ambiguityMemory{candidates: candidates, at: time.Now()}
}

func (m *Manager) lastAmbiguityOptions(scope services.TaskScope) []services.TaskCandidate {
	m.ambiguityMu.Lock()
	defer m.ambiguityMu.Unlock()
	entry, ok := m.ambiguity[ambiguityKey(scope)]
	if !ok || time.Since(entry.at) > services.TaskFocusTTL {
		return nil
	}
	return entry.candidates
}

func ambiguityKey(scope services.TaskScope) string {
	return strings.Join([]string{scope.ProjectID, scope.UserID, scope.Channel, scope.ConversationID}, "|")
}

func shortTitles(candidates []services.TaskCandidate) []string {
	out := make([]string, 0, len(candidates))
	for _, c := range candidates {
		out = append(out, c.Identity.ShortTitle)
	}
	return out
}

func taskStatusLabel(status, language string) string {
	if services.NormalizeLanguage(language) == "en" {
		switch strings.ToLower(strings.TrimSpace(status)) {
		case "completed", "done":
			return "Completed"
		case "failed":
			return "Failed"
		case "cancelled", "canceled":
			return "Cancelled"
		default:
			return "In progress"
		}
	}
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "completed", "done":
		return "已完成"
	case "failed":
		return "已失败"
	case "cancelled", "canceled":
		return "已取消"
	default:
		return "进行中"
	}
}

func ambiguityPrompt(options []string, language string, replySupported bool) string {
	en := services.NormalizeLanguage(language) == "en"
	var b strings.Builder
	if en {
		b.WriteString("Several tasks match. Which one do you mean?")
	} else {
		b.WriteString("匹配到多个任务，你指的是哪一个？")
	}
	for i, option := range options {
		b.WriteString("\n")
		b.WriteString(strconv.Itoa(i + 1))
		b.WriteString(". ")
		b.WriteString(option)
	}
	b.WriteString("\n")
	if !replySupported {
		b.WriteString(QQReplyFallback("", language))
		return b.String()
	}
	if en {
		b.WriteString("Reply with the number or the short title.")
	} else {
		b.WriteString("请回复序号或短标题。")
	}
	return b.String()
}

func noContextPrompt(language string, replySupported bool) string {
	en := services.NormalizeLanguage(language) == "en"
	base := "当前会话没有正在跟进的任务（焦点已过期）。"
	if en {
		base = "This conversation has no active task in focus (it expired)."
	}
	if !replySupported {
		return base + QQReplyFallback("", language)
	}
	if en {
		return base + " Please name the task by short title."
	}
	return base + "请用短标题指明任务。"
}

func notFoundPrompt(language string, replySupported bool) string {
	en := services.NormalizeLanguage(language) == "en"
	base := "没有找到匹配的任务。"
	if en {
		base = "No matching task was found."
	}
	if !replySupported {
		return base + QQReplyFallback("", language)
	}
	if en {
		return base + " Please check the short title."
	}
	return base + "请确认短标题是否正确。"
}
