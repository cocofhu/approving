package main

import (
	"context"
	"strings"

	"github.com/cocofhu/approving/internal/channels"
	"github.com/cocofhu/approving/internal/sendable"
)

// channelIMNotifier adapts channels.Manager to pmmcp.ExternalIMNotifier.
type channelIMNotifier struct {
	mgr *channels.Manager
}

func (n channelIMNotifier) NotifyRunAccepted(projectID, runID, userID, shortTitle, language string) error {
	if n.mgr == nil {
		return nil
	}
	qqUser := strings.TrimPrefix(userID, "qq:")
	_, err := n.mgr.SendRunAcceptanceAck(context.Background(), channels.RunAcceptanceAck{
		ProjectID: projectID, RunID: runID, Scene: channels.SceneC2C,
		ConversationID: "", UserID: qqUser, ShortTitle: shortTitle, Language: language,
	})
	return err
}

func (n channelIMNotifier) NotifyProgress(projectID, runID, userID, kind, text, stage, conclusion string, blocked, actionRequired bool) error {
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
	qqUser := strings.TrimPrefix(userID, "qq:")
	_, err := n.mgr.ReportRunProgress(context.Background(), channels.SendableRequest{
		ProjectID: projectID, RunID: runID, UserID: qqUser,
		Kind: skind, Reason: "pm_notify_progress", Priority: priority,
		Progress: sendable.ProgressFields{
			Stage: stage, Blocked: blocked, ActionRequired: actionRequired, Conclusion: conclusion,
		},
		Text: text,
	})
	return err
}
