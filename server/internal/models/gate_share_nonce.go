package models

import "time"

// GateShareNonce is a one-time preview nonce keyed by share-link token hash.
// Plaintext share tokens are never stored; nonce hex is short-lived (15m) and
// capped per link so multi-replica preview→decide can share the same bucket.
type GateShareNonce struct {
	ID        uint      `gorm:"primaryKey" json:"-"`
	TokenHash string    `gorm:"index:idx_gsn_token_hash;size:64;not null" json:"-"`
	Nonce     string    `gorm:"size:64;not null" json:"-"`
	ExpiresAt time.Time `gorm:"index" json:"-"`
	CreatedAt time.Time `json:"-"`
}
