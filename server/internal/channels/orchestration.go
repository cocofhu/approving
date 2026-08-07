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
			m.sendOrchestrationReply(ctx, rc, in, riskExecutionFailedText(language))
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
