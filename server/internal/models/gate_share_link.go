package models

import "time"

// GateShareLink is a one-shot external approval credential bound to a single
// human_gate instance. Only the SHA-256 hash of the token is persisted.
type GateShareLink struct {
	ID        string `gorm:"primaryKey;size:40" json:"id"`
	TokenHash string `gorm:"uniqueIndex;size:64;not null" json:"-"`
	RunID     string `gorm:"index:idx_gsl_instance,priority:1;index;size:64" json:"runId"`
	NodeID    string `gorm:"index:idx_gsl_instance,priority:2;size:128" json:"nodeId"`
	Iteration int    `gorm:"index:idx_gsl_instance,priority:3" json:"iteration"`
	GateID    uint   `gorm:"index" json:"gateId"`
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
