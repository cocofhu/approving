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

func (n channelIMNotifier) NotifyProgress(projectID, runID string, target pmmcp.IMTarget, kind, text, stage, conclusion string, blocked, actionRequired bool) error {
	if n.mgr == nil {
		return nil
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
	_, err := n.mgr.ReportRunProgress(context.Background(), channels.SendableRequest{
		ProjectID: projectID, RunID: runID, Scene: channels.Scene(target.Scene),
		ConversationID: target.ConversationID, UserID: target.UserID,
		Kind: skind, Reason: "pm_notify_progress", Priority: priority,
		Progress: sendable.ProgressFields{
			Stage: stage, Blocked: blocked, ActionRequired: actionRequired, Conclusion: conclusion,
		},
		Text: text,
	})
	return err
}
