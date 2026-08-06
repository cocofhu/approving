package models

import "time"

const (
	DeliveryReceiptPending = "pending"
	DeliveryReceiptSent    = "sent"
	DeliveryReceiptFailed  = "failed"
)

// Risk ticket lifecycle. "confirmed" is the authorization grant; only
// "executed" is reached by the destructive method that actually consumes it.
const (
	RiskTicketPending   = "pending"
	RiskTicketConfirmed = "confirmed"
	RiskTicketExecuted  = "executed"
	RiskTicketCancelled = "cancelled"
	RiskTicketExpired   = "expired"
)

// TaskIdentity is the durable, user-scoped name and search document for a Run.
// RunID is the stable identity; titles may change while previous titles remain
// searchable through Aliases.
type TaskIdentity struct {
	ID                  uint      `gorm:"primaryKey" json:"-"`
	RunID               string    `gorm:"size:64;uniqueIndex" json:"runId"`
	ProjectID           string    `gorm:"size:64;index:idx_task_scope" json:"projectId"`
	UserID              string    `gorm:"size:191;index:idx_task_scope" json:"userId"`
	ShortTitle          string    `gorm:"size:160;index" json:"shortTitle"`
	OriginalRequirement string    `gorm:"type:text" json:"originalRequirement"`
	Aliases             []string  `gorm:"serializer:json" json:"aliases"`
	Keywords            []string  `gorm:"serializer:json" json:"keywords"`
	Status              string    `gorm:"size:32;index" json:"status"`
	CreatedAt           time.Time `json:"createdAt"`
	UpdatedAt           time.Time `gorm:"index" json:"updatedAt"`
}

// MessageBinding connects a platform message to the Run it referred to.
type MessageBinding struct {
	ID             uint      `gorm:"primaryKey" json:"-"`
	ProjectID      string    `gorm:"size:64;uniqueIndex:idx_message_binding" json:"projectId"`
	Channel        string    `gorm:"size:32;uniqueIndex:idx_message_binding" json:"channel"`
	ConversationID string    `gorm:"size:191;uniqueIndex:idx_message_binding" json:"conversationId"`
	MessageID      string    `gorm:"size:191;uniqueIndex:idx_message_binding" json:"messageId"`
	UserID         string    `gorm:"size:191;index" json:"userId"`
	RunID          string    `gorm:"size:64;index" json:"runId"`
	NodeID         string    `gorm:"size:128;index" json:"nodeId,omitempty"`
	GateID         string    `gorm:"size:128;index" json:"gateId,omitempty"`
	Action         string    `gorm:"size:191" json:"action,omitempty"`
	Direction      string    `gorm:"size:16" json:"direction,omitempty"` // inbound|outbound
	CreatedAt      time.Time `json:"createdAt"`
}

// ConversationFocus records the most recently selected Run in a scoped
// conversation. ExpiresAt is extended on every successful reference.
type ConversationFocus struct {
	ID             uint      `gorm:"primaryKey" json:"-"`
	ProjectID      string    `gorm:"size:64;uniqueIndex:idx_conversation_focus" json:"projectId"`
	Channel        string    `gorm:"size:32;uniqueIndex:idx_conversation_focus" json:"channel"`
	ConversationID string    `gorm:"size:191;uniqueIndex:idx_conversation_focus" json:"conversationId"`
	UserID         string    `gorm:"size:191;uniqueIndex:idx_conversation_focus" json:"userId"`
	RunID          string    `gorm:"size:64;index" json:"runId"`
	PendingRunIDs  []string  `gorm:"serializer:json" json:"pendingRunIds,omitempty"`
	Language       string    `gorm:"size:16" json:"language,omitempty"`
	ExpiresAt      time.Time `gorm:"index" json:"expiresAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

// RiskConfirmationTicket is a short-lived, single-use authorization for one
// high-risk action. It stores identity and state only, never credentials.
// A confirmed ticket is the persistent grant that the destructive server-side
// method must consume; ThreadID scopes it to the PM thread that may spend it.
type RiskConfirmationTicket struct {
	ID             string     `gorm:"primaryKey;size:64" json:"id"`
	ProjectID      string     `gorm:"size:64;index:idx_risk_ticket_scope" json:"projectId"`
	UserID         string     `gorm:"size:191;index:idx_risk_ticket_scope" json:"userId"`
	RunID          string     `gorm:"size:64;index:idx_risk_ticket_scope" json:"runId"`
	Action         string     `gorm:"size:191;index:idx_risk_ticket_scope" json:"action"`
	ActionKind     string     `gorm:"size:32;index" json:"actionKind"`
	ThreadID       string     `gorm:"size:64;index" json:"threadId,omitempty"`
	Status         string     `gorm:"size:16;index" json:"status"` // pending|confirmed|executed|cancelled|expired
	ExpiresAt      time.Time  `gorm:"index" json:"expiresAt"`
	GrantExpiresAt *time.Time `gorm:"index" json:"grantExpiresAt,omitempty"`
	ConsumedAt     *time.Time `json:"consumedAt,omitempty"`
	ExecutedAt     *time.Time `json:"executedAt,omitempty"`
	CreatedAt      time.Time  `json:"createdAt"`
	UpdatedAt      time.Time  `json:"updatedAt"`
}

// ChannelActionGuard marks a PM thread as channel-originated. Destructive PM
// MCP mutations on a guarded thread are denied unless a confirmed ticket
// authorizes exactly that target and action. Web/API routes keep their own
// authentication and are never guarded.
type ChannelActionGuard struct {
	ID        uint      `gorm:"primaryKey" json:"-"`
	ProjectID string    `gorm:"size:64;uniqueIndex:idx_channel_action_guard" json:"projectId"`
	ThreadID  string    `gorm:"size:64;uniqueIndex:idx_channel_action_guard" json:"threadId"`
	Channel   string    `gorm:"size:32" json:"channel"`
	UserID    string    `gorm:"size:191" json:"userId"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// DeliveryReceipt makes external delivery retryable and idempotent.
type DeliveryReceipt struct {
	ID          uint       `gorm:"primaryKey" json:"-"`
	DedupeKey   string     `gorm:"size:255;uniqueIndex" json:"dedupeKey"`
	Status      string     `gorm:"size:16;index" json:"status"` // pending|sent|failed
	Attempts    int        `json:"attempts"`
	LastError   string     `gorm:"size:500" json:"lastError,omitempty"`
	LastTriedAt *time.Time `json:"lastTriedAt,omitempty"`
	SentAt      *time.Time `json:"sentAt,omitempty"`
	CreatedAt   time.Time  `json:"createdAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`
}
