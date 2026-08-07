package models

import "time"

// TaskIdentity is the stable, user-facing identity of a Run.
// Search visibility is bounded by both project and external user identity.
type TaskIdentity struct {
	ID                  string   `gorm:"primaryKey;size:64" json:"id"`
	RunID               string   `gorm:"size:64;uniqueIndex:idx_task_run_project,priority:1" json:"runId"`
	ProjectID           string   `gorm:"size:64;uniqueIndex:idx_task_run_project,priority:2;index:idx_task_project_title,priority:1;index:idx_task_scope_title,priority:1" json:"projectId"`
	UserID              string   `gorm:"size:191;index:idx_task_scope_title,priority:2" json:"userId,omitempty"`
	ShortTitle          string   `gorm:"size:160;index:idx_task_project_title,priority:2;index:idx_task_scope_title,priority:3" json:"shortTitle"`
	OriginalRequirement string   `gorm:"type:text" json:"originalRequirement"`
	Aliases             []string `gorm:"serializer:json" json:"aliases"`
	Keywords            []string `gorm:"serializer:json" json:"keywords"`
	// Origin conversation. A task's results belong in the conversation that
	// asked for them, so this is persisted rather than derived: it must survive
	// a restart and must not fall back to a project-wide push target.
	// Scene is stored as a plain string because models must not depend on the
	// channels package.
	OriginChannel        string     `gorm:"size:32;index:idx_task_origin,priority:1" json:"originChannel,omitempty"`
	OriginScene          string     `gorm:"size:32" json:"originScene,omitempty"`
	OriginConversationID string     `gorm:"size:191;index:idx_task_origin,priority:2" json:"originConversationId,omitempty"`
	OriginExternalUserID string     `gorm:"size:191" json:"originExternalUserId,omitempty"`
	Language             string     `gorm:"size:16" json:"language,omitempty"`
	RecentContext        string     `gorm:"type:text" json:"recentContext,omitempty"`
	Status               string     `gorm:"size:32;index" json:"status"`
	TerminalAt           *time.Time `gorm:"index" json:"terminalAt,omitempty"`
	CreatedAt            time.Time  `json:"createdAt"`
	UpdatedAt            time.Time  `json:"updatedAt"`
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

// PendingChannelTurn records a foreground turn that is currently running.
//
// A restart kills the sandbox turn in flight, and without this row the user's
// message simply disappears — they asked something and nothing ever came back.
// The row is written when the turn starts and deleted when it ends, so anything
// still present at boot is a turn that died with the process.
type PendingChannelTurn struct {
	ID             string    `gorm:"primaryKey;size:191" json:"id"`
	ProjectID      string    `gorm:"size:64;index" json:"projectId"`
	Channel        string    `gorm:"size:32" json:"channel"`
	Scene          string    `gorm:"size:32" json:"scene"`
	ConversationID string    `gorm:"size:191;index" json:"conversationId"`
	ExternalUserID string    `gorm:"size:191" json:"externalUserId"`
	MessageID      string    `gorm:"size:191" json:"messageId"`
	Language       string    `gorm:"size:16" json:"language"`
	StartedAt      time.Time `json:"startedAt"`
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
	Action   string `gorm:"size:191;index:idx_risk_ticket,priority:4" json:"action"`
	Status   string `gorm:"size:16;index" json:"status"` // pending | confirmed | cancelled | expired
	Language string `gorm:"size:16" json:"language"`
	// PromptedAt records when this ticket's question actually reached the user.
	// Nil means it never did — delivery can be suppressed or fail — and such a
	// ticket must never be settled by a bare "确认", because the user cannot
	// have been answering a question they were never asked.
	PromptedAt *time.Time `json:"promptedAt,omitempty"`
	ExpiresAt  time.Time  `gorm:"index" json:"expiresAt"`
	ResolvedAt *time.Time `json:"resolvedAt,omitempty"`
	CreatedAt  time.Time  `json:"createdAt"`
	UpdatedAt  time.Time  `json:"updatedAt"`
}
