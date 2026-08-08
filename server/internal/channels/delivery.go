package channels

import (
	"context"
	"errors"
	"strings"

	"github.com/cocofhu/approving/internal/sendable"
	"github.com/cocofhu/approving/internal/services"

	"github.com/rs/zerolog/log"
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
	TraceID     string

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
		TraceID:   strings.TrimSpace(r.TraceID),
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

// ReasonAlreadyReplied marks an answer withheld because this turn was already
// answered. It is a deliberate suppression, not a failure: the agent is told so
// it stops rather than rewording and trying again.
const ReasonAlreadyReplied = "already_replied"

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
	TraceID        string
	Text           string
	ShortTitle     string
}

// DeliverConversationReply sends the agent's answer and records that this turn
// has been answered, so the turn's own wrap-up does not append a second message.
// This is the single egress for conversational answers: the model's raw output
// is never forwarded, which keeps reasoning and tool chatter inside the
// platform without needing to scrape a summary out of the transcript.
func (m *Manager) DeliverConversationReply(ctx context.Context, reply ConversationReply) (DeliveryResult, error) {
	// Pass through; sendOutboundResult applies ScrubForOutbound as the final gate.
	// Whitespace-only replies are rejected here; content that scrubs to empty
	// surfaces as empty_after_scrub at the gate (same external non-delivery).
	text := strings.TrimSpace(reply.Text)
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
	// One question, one answer. The dedupe key below is derived from the text,
	// which collapses a retry of the same answer but lets two differently worded
	// answers both through — and an agent that submits twice in one turn is
	// exactly what the user sees as the platform talking over itself. The
	// per-turn marker is the only thing that can tell those apart, so it is
	// checked here rather than only at the turn's wrap-up.
	turn := conversationTurnScope(reply.ProjectID, scene, reply.ConversationID)
	if m.hasAnswered(turn) {
		log.Info().Str("project", reply.ProjectID).Str("conversation", reply.ConversationID).
			Msg("second answer in one turn withheld; the agent is told it already replied")
		return DeliveryResult{Decision: sendable.Decision{Reason: ReasonAlreadyReplied}}, nil
	}
	// Most conversational answers belong to no Run at all — chat, an
	// explanation, a clarifying question — so the delivery scope falls back to
	// the conversation itself. Without it the answer carries no scope and the
	// policy drops it, which would silently mute ordinary conversation.
	scope := strings.TrimSpace(reply.ShortTitle)
	if scope == "" && strings.TrimSpace(reply.RunID) == "" {
		scope = turn
	}
	result, err := m.DeliverSendable(ctx, SendableRequest{
		ProjectID: reply.ProjectID, Scene: scene, ConversationID: reply.ConversationID,
		UserID: reply.UserID, RunID: strings.TrimSpace(reply.RunID),
		TaskContext: scope, TraceID: strings.TrimSpace(reply.TraceID),
		Kind: sendable.KindFinal, Reason: "pm_reply",
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
		m.markAnswered(turn)
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
	scene := ack.Scene
	if scene == "" {
		scene = SceneC2C
	}
	// The conversation layer may already have told the user this is being
	// picked up — that is what a heavy dispatch's acknowledgement is. Adding
	// the platform's own acceptance notice on top is the "我去看看" + "好的我去看看"
	// double-send that made delegation feel like the system talking to itself.
	turn := conversationTurnScope(ack.ProjectID, scene, ack.ConversationID)
	if m.hasAcknowledged(turn) {
		return DeliveryResult{Decision: sendable.Decision{Reason: "already_acknowledged"}}, nil
	}
	language := services.DetectLanguage("", ack.Language)
	// Phrased by the conversation model like every other acknowledgement; the
	// fixed line below is only what a missing or slow model falls back to.
	text := m.phraseRunAccepted(ctx, ack.ShortTitle, language)
	if text == "" {
		text = runAcceptanceText(ack.ShortTitle, language)
	}
	result, err := m.DeliverSendable(ctx, SendableRequest{
		ProjectID: ack.ProjectID, Scene: ack.Scene, ConversationID: ack.ConversationID,
		UserID: ack.UserID, RunID: runID,
		Kind: sendable.KindRunAcceptanceAck, Reason: "run_accepted",
		Priority:  sendable.PriorityHigh,
		DedupeKey: runAcceptanceDedupeKey(runID, ack.ConversationID, ack.UserID),
		Text:      text,
	})
	// Handing work to the background is this turn's answer: the user asked for
	// something and has been told it is being done. Marking the turn answered
	// is what lets the conversation move on immediately instead of waiting for
	// the Run and then appending a second message about it.
	// The marker deliberately says "answered", not "acknowledged": a repeat of
	// this notice is caught by its own per-run dedupe, which reports why more
	// precisely than a turn-level marker could.
	if result.Sent {
		m.MarkConversationReplied(ack.ProjectID, scene, ack.ConversationID)
		m.markAnswered(turn)
	}
	return result, err
}

// runAcceptanceText is the last-resort line when a Run is accepted but the
// conversation layer did not already speak.
//
// It says which task it is whenever the title is short enough to read as a
// name. This line used to drop the title on the grounds that a short_title is
// a truncated requirement rather than a name, and back then it was: they
// arrived as 「快模型和 wo」. Now that they are cut at word boundaries the only
// remaining objection is length, so length is what is actually checked —
// dropping the name outright produced 「好，那事我去弄」 in reply to 「修复下」,
// which names nothing at all.
//
// Do not append "feel free to keep chatting": the user already knows they can
// talk, and saying so reads like a helpdesk script.
func runAcceptanceText(shortTitle, language string) string {
	name := nameableTitle(shortTitle)
	if services.NormalizeLanguage(language) == "en" {
		if name != "" {
			return "Got it, " + name + " — I'll take it and come back when it's done."
		}
		return "Got it, I'll take that one and come back when it's done."
	}
	if name != "" {
		// The comma is doing work: a title can end in either script, and it is
		// the one separator that reads correctly after both.
		return "好，" + name + "，我去弄，完了告诉你。"
	}
	return "好，那事我去弄，完了告诉你。"
}

// nameableTitleRunes is where a name stops being a name. Past this a title is
// the request written out, and reading a request back at the person who just
// made it is the thing every ack prompt here is trying to avoid.
const nameableTitleRunes = 14

// nameableTitle returns the title if it can stand in as the name of a task, or
// empty if it cannot: too long to be anything but the requirement restated, or
// already marked as cut short.
func nameableTitle(shortTitle string) string {
	t := strings.TrimSpace(shortTitle)
	if t == "" || strings.Contains(t, "…") || strings.Contains(t, "...") {
		return ""
	}
	if len([]rune(t)) > nameableTitleRunes {
		return ""
	}
	return t
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

// Task addressing used to live here: an ordinal / short-title / focus / scored
// search resolver whose ambiguous branch asked the user to pick from a numbered
// list. It is gone on purpose. Scoring free-form Chinese against short titles
// could not tell one task from two, so "这两个都修一下" reached a numbered menu
// instead of anything able to read the repository, and the menu was a dead end:
// picking a number only re-entered the same resolver. Which task a message is
// about is now decided by whichever model handles the turn, with the whole
// conversation in view.
