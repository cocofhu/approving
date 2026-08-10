package models

import "time"

// Share-link kinds (one table; gate vs review sessions must not mix).
const (
	ShareLinkKindHumanGate = "human_gate"
	ShareLinkKindReview    = "review"
)

// GateShareLink is a one-shot external credential bound to a single human_gate
// or inbox review instance. Only the SHA-256 hash of the token is persisted.
// Review rows keep GateID nil (no fake Gate); human_gate rows always set it.
type GateShareLink struct {
	ID        string `gorm:"primaryKey;size:40" json:"id"`
	TokenHash string `gorm:"uniqueIndex;size:64;not null" json:"-"`
	Kind      string `gorm:"size:32;index:idx_gsl_kind_instance,priority:1;not null;default:human_gate" json:"kind"`
	RunID     string `gorm:"index:idx_gsl_instance,priority:1;index:idx_gsl_kind_instance,priority:2;index;size:64" json:"runId"`
	NodeID    string `gorm:"index:idx_gsl_instance,priority:2;index:idx_gsl_kind_instance,priority:3;size:128" json:"nodeId"`
	Iteration int    `gorm:"index:idx_gsl_instance,priority:3;index:idx_gsl_kind_instance,priority:4" json:"iteration"`
	GateID    *uint  `gorm:"index" json:"gateId,omitempty"`
	CreatedBy string `gorm:"size:128" json:"createdBy"`
	TTLTier   string `gorm:"size:8" json:"ttlTier"`
	ExpiresAt time.Time  `gorm:"index" json:"expiresAt"`
	RevokedAt *time.Time `json:"revokedAt,omitempty"`
	UsedAt    *time.Time `json:"usedAt,omitempty"`
	UsedAction string    `gorm:"size:64" json:"usedAction,omitempty"`
	CreatedAt time.Time  `json:"createdAt"`
}

// Share-link TTL tiers (product: default 24h).
const (
	ShareTTLTier1h  = "1h"
	ShareTTLTier8h  = "8h"
	ShareTTLTier24h = "24h"
	ShareTTLTier72h = "72h"
	ShareTTLTier7d  = "7d"
)

// Share-link product states (no plaintext token).
const (
	ShareLinkStateNone    = "none"
	ShareLinkStateActive  = "active"
	ShareLinkStateUsed    = "used"
	ShareLinkStateRevoked = "revoked"
	ShareLinkStateExpired = "expired"
)
