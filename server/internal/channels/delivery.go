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
// ExternalMessageID is the channel's own id for the delivered message when the
// transport returned one.
type DeliveryResult struct {
	Decision          sendable.Decision
	Sent              bool
	ExternalMessageID string
}

// Reason exposes the policy reason behind the outcome ("" when sent without a
// specific reason).
func (r DeliveryResult) Reason() string { return r.Decision.Reason }

// Suppressed reports a delivery that policy intentionally withheld: rate
// limiting, dedupe/merge, an already-sent receipt, or a validation gate. It is a
// normal outcome, not a delivery failure, so callers must not retry it or report
// it as an error to an agent.
func (r DeliveryResult) Suppressed() bool {
	return !r.Sent && !deliveryFailureReasons[r.Decision.Reason]
}

// Failed reports a delivery that really did not reach the channel: no target,
// transport error, exhausted retries, or a failed-closed policy evaluation.
func (r DeliveryResult) Failed() bool {
	return !r.Sent && deliveryFailureReasons[r.Decision.Reason]
}

// deliveryFailureReasons enumerates the policy/transport reasons that mean the
// message did not reach the channel and the caller should surface an error.
// Every other reason is a deliberate suppression (see DeliveryResult.Suppressed).
var deliveryFailureReasons = map[string]bool{
	"no_adapter":         true,
	"policy_error":       true,
	"transport_failed":   true,
	"retry_claim_failed": true,
	"retry_exhausted":    true,
	"receipt_missing":    true,
	"missing_dedupe_key": true,
}

// ErrDeliveryFailed is returned when a delivery really failed (no adapter,
// transport error, retries exhausted). Policy suppression is NOT an error: it
// comes back as a result with Sent=false and a reason.
var ErrDeliveryFailed = errors.New("delivery failed after retries")

// DeliverSendable is the explicit external egress for orchestration. Every
// delivery still passes the Manager's single policy gate. Callers can tell
// sent / suppressed / failed apart from the result: only a real failure returns
// an error, so a rate-limited or deduplicated message never looks like a broken
// transport to the caller.
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
	if result.Failed() {
		return result, ErrDeliveryFailed
	}
	return result, nil
}

// ConversationReply is an answer the agent explicitly submitted for the current
// conversation turn.
type ConversationReply struct {
	ProjectID      string
	RunID          string
	Scene          Scene
	ConversationID string
	UserID         string
	Text           string
	ShortTitle     string
}

// DeliverConversationReply sends the agent's answer and records that this turn
// has been answered, so the turn's own wrap-up does not append a second message.
// This is the single egress for conversational answers: the model's raw output
// is never forwarded, which keeps reasoning and tool chatter inside the
// platform without needing to scrape a summary out of the transcript.
func (m *Manager) DeliverConversationReply(ctx context.Context, reply ConversationReply) (DeliveryResult, error) {
	text := ScrubInternalTerms(reply.Text)
	if text == "" {
		return DeliveryResult{}, errors.New("conversation reply text is empty")
	}
	scene := reply.Scene
	if scene == "" {
		scene = SceneC2C
	}
	// A synthesis turn borrows this conversation's agent to phrase a background
	// event. Its answer belongs to the reflow that asked for it, which delivers
	// it once with the right dedupe key, so it is collected here rather than
	// sent from under the agent.
	if m.captureReply(reply.ProjectID, scene, reply.ConversationID, text) {
		return DeliveryResult{Decision: sendable.Decision{Reason: "captured_for_reflow"}}, nil
	}
	// Most conversational answers belong to no Run at all — chat, an
	// explanation, a clarifying question — so the delivery scope falls back to
	// the conversation itself. Without it the answer carries no scope and the
	// policy drops it, which would silently mute ordinary conversation.
	scope := strings.TrimSpace(reply.ShortTitle)
	if scope == "" && strings.TrimSpace(reply.RunID) == "" {
		scope = conversationTurnScope(reply.ProjectID, scene, reply.ConversationID)
	}
	result, err := m.DeliverSendable(ctx, SendableRequest{
		ProjectID: reply.ProjectID, Scene: scene, ConversationID: reply.ConversationID,
		UserID: reply.UserID, RunID: strings.TrimSpace(reply.RunID),
		TaskContext: scope,
		Kind:        sendable.KindFinal, Reason: "pm_reply",
		Priority: sendable.PriorityCritical,
		// No explicit dedupe key: the policy derives one from the content, so a
		// retry of the same answer collapses while two different answers in the
		// same conversation both go out.
		Text: text,
	})
	if err != nil {
		return result, err
	}
	if result.Sent {
		m.MarkConversationReplied(reply.ProjectID, scene, reply.ConversationID)
	}
	return result, nil
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
	result, err := m.DeliverSendable(ctx, SendableRequest{
		ProjectID: ack.ProjectID, Scene: ack.Scene, ConversationID: ack.ConversationID,
		UserID: ack.UserID, RunID: runID,
		Kind: sendable.KindRunAcceptanceAck, Reason: "run_accepted",
		Priority:  sendable.PriorityHigh,
		DedupeKey: runAcceptanceDedupeKey(runID, ack.ConversationID, ack.UserID),
		Text:      runAcceptanceText(ack.ShortTitle, language),
	})
	// Handing work to the background is this turn's answer. Marking the turn
	// replied is what lets the conversation move on immediately instead of
	// waiting for the Run and then appending a second message about it.
	if result.Sent {
		scene := ack.Scene
		if scene == "" {
			scene = SceneC2C
		}
		m.MarkConversationReplied(ack.ProjectID, scene, ack.ConversationID)
	}
	return result, err
}

// runAcceptanceText confirms a delegation the way a colleague would: what was
// picked up, and that the user is free to keep talking. No ticket header, no
// promise to "report substantive progress".
func runAcceptanceText(shortTitle, language string) string {
	title := services.SanitizeShortTitle(shortTitle)
	if services.NormalizeLanguage(language) == "en" {
		if title == "" {
			return "Got it, I'll take that one and come back when it's done. Feel free to keep chatting in the meantime."
		}
		return "Got it — I'll go work on \"" + title + "\" and tell you when it's done. Feel free to keep chatting in the meantime."
	}
	if title == "" {
		return "好，我去弄，完了告诉你。你可以接着问别的。"
	}
	return "好，" + title + "这块我去弄，完了告诉你。你可以接着问别的。"
}

func runAcceptanceDedupeKey(runID, conversationID, userID string) string {
	return strings.Join([]string{
		"run-ack", runID, conversationID, userID, string(sendable.ChannelQQ),
	}, ":")
}

// resolveSendableTarget decides which conversation a message goes to.
//
// Precedence is deliberate: an explicit conversation wins, then the task's own
// origin conversation, and only then the project's push target. Falling back to
// the project target for Run traffic is what sent one user's results into an
// unrelated cron session, so it is the last resort rather than the default.
func (m *Manager) resolveSendableTarget(req SendableRequest) (*runningChannel, Scene, string, error) {
	scene, conv := req.Scene, strings.TrimSpace(req.ConversationID)
	if conv == "" {
		if s, c, ok := m.originConversationForRun(req.ProjectID, req.RunID); ok {
			scene, conv = s, c
		}
	}
	if conv != "" {
		m.mu.Lock()
		defer m.mu.Unlock()
		for _, rc := range m.running {
			if rc.cfg.ProjectID == req.ProjectID {
				if scene == "" {
					scene = SceneC2C
				}
				return rc, scene, conv, nil
			}
		}
		return nil, "", "", ErrNoSendableTarget
	}
	target, fallbackScene, fallbackConv, err := m.lookupRunNotifyTarget(req.ProjectID)
	if err != nil {
		return nil, "", "", ErrNoSendableTarget
	}
	return target, fallbackScene, fallbackConv, nil
}

// originConversationForRun looks up where a Run's task was created.
func (m *Manager) originConversationForRun(projectID, runID string) (Scene, string, bool) {
	if m.taskContext == nil || strings.TrimSpace(runID) == "" || strings.TrimSpace(projectID) == "" {
		return "", "", false
	}
	identity, err := m.taskContext.IdentityForRun(runID, projectID)
	if err != nil || identity == nil {
		return "", "", false
	}
	conv := strings.TrimSpace(identity.OriginConversationID)
	if conv == "" {
		return "", "", false
	}
	scene := Scene(strings.TrimSpace(identity.OriginScene))
	if scene == "" {
		scene = SceneC2C
	}
	return scene, conv, true
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

// taskStatusLabel states a task's state the way a person would say it, as a
// predicate that reads naturally after the task's name.
func taskStatusLabel(status, language string) string {
	if services.NormalizeLanguage(language) == "en" {
		switch strings.ToLower(strings.TrimSpace(status)) {
		case "completed", "done":
			return "is done."
		case "failed":
			return "didn't go through."
		case "cancelled", "canceled":
			return "was cancelled."
		default:
			return "is still running."
		}
	}
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "completed", "done":
		return "弄完了。"
	case "failed":
		return "没做成。"
	case "cancelled", "canceled":
		return "取消了。"
	default:
		return "还在做。"
	}
}

// soloTaskStatusText is the same state stated without naming the task, for a
// conversation where only one task could possibly be meant.
func soloTaskStatusText(status, language string) string {
	if services.NormalizeLanguage(language) == "en" {
		switch strings.ToLower(strings.TrimSpace(status)) {
		case "completed", "done":
			return "It's done."
		case "failed":
			return "It didn't go through."
		case "cancelled", "canceled":
			return "It was cancelled."
		default:
			return "Still running."
		}
	}
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "completed", "done":
		return "弄完了。"
	case "failed":
		return "没做成。"
	case "cancelled", "canceled":
		return "取消了。"
	default:
		return "还在做呢。"
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
