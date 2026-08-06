package models

import "time"

// TaskIdentity is the stable, user-facing identity of a Run.
type TaskIdentity struct {
	ID                  string     `gorm:"primaryKey;size:64" json:"id"`
	RunID               string     `gorm:"size:64;uniqueIndex" json:"runId"`
	ProjectID           string     `gorm:"size:64;index:idx_task_scope,priority:1" json:"projectId"`
	UserID              string     `gorm:"size:191;index:idx_task_scope,priority:2" json:"userId"`
	ShortTitle          string     `gorm:"size:160;index:idx_task_scope,priority:3" json:"shortTitle"`
	OriginalRequirement string     `gorm:"type:text" json:"originalRequirement"`
	Aliases             []string   `gorm:"serializer:json" json:"aliases"`
	Keywords            []string   `gorm:"serializer:json" json:"keywords"`
	Status              string     `gorm:"size:32;index" json:"status"`
	TerminalAt          *time.Time `gorm:"index" json:"terminalAt,omitempty"`
	CreatedAt           time.Time  `json:"createdAt"`
	UpdatedAt           time.Time  `json:"updatedAt"`
}

// MessageBinding gives an explicit quoted/replied message precedence over
// fuzzy title matching.
type MessageBinding struct {
	ID             uint      `gorm:"primaryKey" json:"id"`
	ProjectID      string    `gorm:"size:64;uniqueIndex:idx_message_binding,priority:1" json:"projectId"`
	UserID         string    `gorm:"size:191;uniqueIndex:idx_message_binding,priority:2" json:"userId"`
	Channel        string    `gorm:"size:32;uniqueIndex:idx_message_binding,priority:3" json:"channel"`
	MessageID      string    `gorm:"size:191;uniqueIndex:idx_message_binding,priority:4" json:"messageId"`
	ConversationID string    `gorm:"size:191;index" json:"conversationId"`
	TaskIdentityID string    `gorm:"size:64;index" json:"taskIdentityId"`
	RunID          string    `gorm:"size:64;index" json:"runId"`
	CreatedAt      time.Time `json:"createdAt"`
}

// ConversationFocus is a short-lived, renewable conversation→task pointer.
type ConversationFocus struct {
	ID             uint      `gorm:"primaryKey" json:"id"`
	ProjectID      string    `gorm:"size:64;uniqueIndex:idx_conversation_focus,priority:1" json:"projectId"`
	UserID         string    `gorm:"size:191;uniqueIndex:idx_conversation_focus,priority:2" json:"userId"`
	Channel        string    `gorm:"size:32;uniqueIndex:idx_conversation_focus,priority:3" json:"channel"`
	ConversationID string    `gorm:"size:191;uniqueIndex:idx_conversation_focus,priority:4" json:"conversationId"`
	TaskIdentityID string    `gorm:"size:64;index" json:"taskIdentityId"`
	RunID          string    `gorm:"size:64;index" json:"runId"`
	Language       string    `gorm:"size:16" json:"language"`
	ExpiresAt      time.Time `gorm:"index" json:"expiresAt"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

// RiskConfirmationTicket authorizes one high-risk action once.
type RiskConfirmationTicket struct {
	ID        string `gorm:"primaryKey;size:64" json:"id"`
	ProjectID string `gorm:"size:64;index:idx_risk_ticket,priority:1" json:"projectId"`
	UserID    string `gorm:"size:191;index:idx_risk_ticket,priority:2" json:"userId"`
	RunID     string `gorm:"size:64;index:idx_risk_ticket,priority:3" json:"runId"`
	// ShortTitle snapshots the task title at creation time so every prompt and
	// every later status reply echoes the task the user was actually asked about.
	ShortTitle string     `gorm:"size:160" json:"shortTitle"`
	Action     string     `gorm:"size:191;index:idx_risk_ticket,priority:4" json:"action"`
	Status     string     `gorm:"size:16;index" json:"status"` // pending | confirmed | cancelled | expired
	Language   string     `gorm:"size:16" json:"language"`
	ExpiresAt  time.Time  `gorm:"index" json:"expiresAt"`
	ResolvedAt *time.Time `json:"resolvedAt,omitempty"`
	CreatedAt  time.Time  `json:"createdAt"`
	UpdatedAt  time.Time  `json:"updatedAt"`
}
