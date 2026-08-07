package main

import (
	"context"
	"strings"

	"github.com/cocofhu/approving/internal/channels"
	"github.com/cocofhu/approving/internal/pmmcp"
	"github.com/cocofhu/approving/internal/sendable"
)

// channelIMNotifier adapts channels.Manager to pmmcp.ExternalIMNotifier.
type channelIMNotifier struct {
	mgr *channels.Manager
}

func (n channelIMNotifier) NotifyRunAccepted(projectID, runID string, target pmmcp.IMTarget, shortTitle, language string) error {
	if n.mgr == nil {
		return nil
	}
	_, err := n.mgr.SendRunAcceptanceAck(context.Background(), channels.RunAcceptanceAck{
		ProjectID: projectID, RunID: runID, Scene: channels.Scene(target.Scene),
		ConversationID: target.ConversationID, UserID: target.UserID,
		ShortTitle: shortTitle, Language: language,
	})
	return err
}

func (n channelIMNotifier) NotifyReply(projectID, runID string, target pmmcp.IMTarget, text, shortTitle string) (pmmcp.IMDeliveryOutcome, error) {
	if n.mgr == nil {
		return pmmcp.IMDeliveryOutcome{}, nil
	}
	result, err := n.mgr.DeliverConversationReply(context.Background(), channels.ConversationReply{
		ProjectID: projectID, RunID: runID, Scene: channels.Scene(target.Scene),
		ConversationID: target.ConversationID, UserID: target.UserID,
		Text: text, ShortTitle: shortTitle,
	})
	if err != nil {
		return pmmcp.IMDeliveryOutcome{}, err
	}
	return pmmcp.IMDeliveryOutcome{
		Sent: result.Sent, Reason: result.Reason(),
		AlreadyReplied: result.Reason() == channels.ReasonAlreadyReplied,
	}, nil
}

func (n channelIMNotifier) NotifyProgress(projectID, runID string, target pmmcp.IMTarget, kind, text, stage, conclusion string, blocked, actionRequired bool) (pmmcp.IMDeliveryOutcome, error) {
	if n.mgr == nil {
		return pmmcp.IMDeliveryOutcome{}, nil
	}
	skind := sendable.KindProgress
	priority := sendable.PriorityNormal
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "blocked":
		skind, priority, blocked = sendable.KindBlocked, sendable.PriorityCritical, true
	case "action_required", "confirm":
		skind, priority, actionRequired = sendable.KindActionRequired, sendable.PriorityCritical, true
	case "final":
		skind, priority = sendable.KindFinal, sendable.PriorityCritical
	}
	if stage == "" && !blocked && !actionRequired && conclusion == "" {
		stage = strings.TrimSpace(text)
	}
	result, err := n.mgr.ReportRunProgress(context.Background(), channels.SendableRequest{
		ProjectID: projectID, RunID: runID, Scene: channels.Scene(target.Scene),
		ConversationID: target.ConversationID, UserID: target.UserID,
		Kind: skind, Reason: "pm_notify_progress", Priority: priority,
		Progress: sendable.ProgressFields{
			Stage: stage, Blocked: blocked, ActionRequired: actionRequired, Conclusion: conclusion,
		},
		Text: text,
	})
	if err != nil {
		return pmmcp.IMDeliveryOutcome{}, err
	}
	// Only a real failure reaches the error branch above; a suppressed delivery
	// is reported as a structured outcome so the agent does not retry it.
	return pmmcp.IMDeliveryOutcome{Sent: result.Sent, Reason: result.Reason()}, nil
}
