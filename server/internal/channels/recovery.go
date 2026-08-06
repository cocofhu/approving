package channels

import (
	"context"
	"strings"
	"time"

	"github.com/cocofhu/approving/internal/models"
	"github.com/cocofhu/approving/internal/sendable"
	"github.com/cocofhu/approving/internal/services"

	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

// A foreground turn lives entirely in memory: a restart takes the sandbox with
// it and the user's message is simply never answered. Recording the turn while
// it runs, and sweeping what is left at boot, is what turns that silent loss
// into a sentence the user can act on.

func (m *Manager) beginPendingTurn(rc *runningChannel, in InboundMessage) string {
	db := m.taskContextDB()
	if db == nil || rc == nil {
		return ""
	}
	id := turnScope(rc, in)
	row := models.PendingChannelTurn{
		ID: id, ProjectID: rc.cfg.ProjectID, Channel: rc.cfg.Type,
		Scene: string(in.Scene), ConversationID: in.ConversationID,
		ExternalUserID: in.UserID, MessageID: in.MessageID,
		Language:  services.DetectLanguage(in.Text, ""),
		StartedAt: time.Now(),
	}
	if err := db.Save(&row).Error; err != nil {
		log.Warn().Err(err).Str("conversation", in.ConversationID).
			Msg("recording in-flight turn failed; a restart would lose this message silently")
		return ""
	}
	return id
}

func (m *Manager) endPendingTurn(id string) {
	db := m.taskContextDB()
	if db == nil || strings.TrimSpace(id) == "" {
		return
	}
	if err := db.Delete(&models.PendingChannelTurn{}, "id = ?", id).Error; err != nil {
		log.Warn().Err(err).Str("turn", id).Msg("clearing in-flight turn record failed")
	}
}

// RecoverInterruptedTurns tells anyone whose turn died in a restart what
// happened, and clears the records. Called once after adapters are up.
func (m *Manager) RecoverInterruptedTurns(ctx context.Context) {
	db := m.taskContextDB()
	if db == nil {
		return
	}
	var rows []models.PendingChannelTurn
	if err := db.Find(&rows).Error; err != nil {
		log.Warn().Err(err).Msg("scanning interrupted turns failed")
		return
	}
	if len(rows) == 0 {
		return
	}
	if ctx == nil {
		ctx = m.baseCtx
	}
	for _, row := range rows {
		log.Info().Str("conversation", row.ConversationID).Str("project", row.ProjectID).
			Time("started", row.StartedAt).Msg("recovering a turn interrupted by restart")
		_, err := m.DeliverSendable(ctx, SendableRequest{
			ProjectID: row.ProjectID, Scene: Scene(row.Scene), ConversationID: row.ConversationID,
			UserID: row.ExternalUserID, ReplyToMessageID: row.MessageID,
			TaskContext: "recovered:" + row.ID,
			Kind:        sendable.KindFinal, Reason: "turn_interrupted_recovery",
			Priority:  sendable.PriorityHigh,
			DedupeKey: "turn-recovery:" + row.ID,
			Text:      interruptedTurnText(row.Language),
		})
		if err != nil {
			log.Warn().Err(err).Str("conversation", row.ConversationID).
				Msg("notifying about an interrupted turn failed")
			continue
		}
		m.endPendingTurn(row.ID)
	}
}

// interruptedTurnText explains the gap without blaming the user or exposing
// what actually broke, and asks for the one thing that resolves it.
func interruptedTurnText(language string) string {
	if services.NormalizeLanguage(language) == "en" {
		return "Sorry — I got restarted while working on your last message and lost it. Could you send it again?"
	}
	return "抱歉，我这边刚重启了一下，你上一条消息没处理完就断了。麻烦再发一次。"
}

// taskContextDB exposes the shared database handle. Recovery deliberately
// piggybacks on the task-context connection rather than taking its own.
func (m *Manager) taskContextDB() *gorm.DB {
	if m.taskContext == nil {
		return nil
	}
	return m.taskContext.DB()
}
