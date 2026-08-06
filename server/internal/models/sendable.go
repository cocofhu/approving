package models

import "time"

// SendableDeliveryReceipt is the durable idempotency record for one external
// delivery. Content is deliberately not persisted.
type SendableDeliveryReceipt struct {
	ID             uint      `gorm:"primaryKey" json:"id"`
	DedupeKey      string    `gorm:"size:128;uniqueIndex" json:"dedupeKey"`
	ProjectID      string    `gorm:"size:64;index" json:"projectId"`
	RunID          string    `gorm:"size:64;index:idx_sendable_bucket,priority:1" json:"runId"`
	TaskContext    string    `gorm:"size:191;index:idx_sendable_bucket,priority:2" json:"taskContext,omitempty"`
	ConversationID string    `gorm:"size:191;index:idx_sendable_bucket,priority:3" json:"conversationId"`
	Channel        string    `gorm:"size:32;index:idx_sendable_bucket,priority:4" json:"channel"`
	Kind           string    `gorm:"size:32" json:"kind"`
	ContentHash    string    `gorm:"size:64;index" json:"-"`
	Status         string    `gorm:"size:16;index" json:"status"` // pending | sent | failed
	Result         string    `gorm:"size:64" json:"result"`
	Attempts       int       `json:"attempts"`
	NextAttemptAt  time.Time `json:"nextAttemptAt"`
	LastAttemptAt  time.Time `json:"lastAttemptAt"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}
