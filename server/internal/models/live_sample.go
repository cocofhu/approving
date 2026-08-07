package models

import "time"

// LiveDecisionSample is one complete record of the conversation layer deciding
// what to do with a user message.
//
// It exists because every earlier attempt at this routing was corrected by
// guesswork: a bad reply was reported, a phrase was added to a ban list, and
// nobody could tell whether the next version routed better or merely differently.
// A sample holds everything needed to replay the decision offline — what the
// model was shown, what it produced, what the tools returned, and what the user
// actually received — so the next change can be measured instead of argued.
//
// Attachments appear as manifests, never as bytes. A sample that carried images
// would grow without limit and could not be exported for review.
type LiveDecisionSample struct {
	ID             string `gorm:"primaryKey;size:64" json:"id"`
	ProjectID      string `gorm:"size:64;index:idx_live_sample_scope,priority:1" json:"projectId"`
	Channel        string `gorm:"size:32" json:"channel,omitempty"`
	Scene          string `gorm:"size:32" json:"scene,omitempty"`
	ConversationID string `gorm:"size:191;index:idx_live_sample_scope,priority:2" json:"conversationId"`
	TurnID         string `gorm:"size:191;index" json:"turnId,omitempty"`
	UserMessageID  string `gorm:"size:191" json:"userMessageId,omitempty"`

	UserText string `gorm:"type:text" json:"userText"`
	// DirectorContext is the ledger snapshot the model was briefed with.
	DirectorContext string `gorm:"type:text" json:"directorContext,omitempty"`
	// Transcript is the conversation window the model was shown.
	Transcript string `gorm:"type:text" json:"transcript,omitempty"`

	Model string `gorm:"size:191" json:"model,omitempty"`
	// RawCompletion is what the model produced, tool calls included, before the
	// platform decided what to do with it.
	RawCompletion string `gorm:"type:text" json:"rawCompletion,omitempty"`
	// ToolResults is what each tool handed back, in call order.
	ToolResults string `gorm:"type:text" json:"toolResults,omitempty"`
	// Actions is what actually left the platform: reason and text per message.
	Actions string `gorm:"type:text" json:"actions,omitempty"`
	// Route names the decision in one word (reply / dispatch / status /
	// cancel / fallthrough), so samples can be counted without parsing.
	Route string `gorm:"size:32;index" json:"route,omitempty"`
	// PMOutcome is what the work layer eventually concluded, filled in when the
	// result comes back rather than at decision time.
	PMOutcome string `gorm:"type:text" json:"pmOutcome,omitempty"`
	// Egress records which layer spoke to the user, so the direct-send fallback
	// can be counted rather than assumed rare.
	Egress string `gorm:"size:32" json:"egress,omitempty"`

	LatencyMs int  `json:"latencyMs"`
	Degraded  bool `gorm:"index" json:"degraded"`
	// QualityFlags marks decisions worth looking at, e.g. an acknowledgement
	// that promised nothing verifiable.
	QualityFlags []string  `gorm:"serializer:json" json:"qualityFlags,omitempty"`
	CreatedAt    time.Time `gorm:"index" json:"createdAt"`
}
