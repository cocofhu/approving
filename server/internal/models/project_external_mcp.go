package models

import "time"

// ProjectExternalMcpSettings stores per-project external MCP access control.
// Default enabled=false; packs are independent from Project.PmEnabledMcps.
type ProjectExternalMcpSettings struct {
	ProjectID    string    `gorm:"primaryKey" json:"projectId"`
	Enabled      bool      `json:"enabled"`
	EnabledPacks []string  `gorm:"serializer:json" json:"enabledPacks"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

// ProjectMcpApiKey is a project-scoped external MCP credential. Plaintext is
// shown only once at creation; KeyHash is bcrypt. Revoked keys fail immediately.
type ProjectMcpApiKey struct {
	ID        string     `gorm:"primaryKey" json:"id"`
	ProjectID string     `gorm:"index" json:"projectId"`
	Name      string     `json:"name"`
	KeyPrefix string     `json:"keyPrefix"`
	KeyHash   string     `json:"-"`
	CreatedAt time.Time  `json:"createdAt"`
	RevokedAt *time.Time `json:"revokedAt,omitempty"`
}
