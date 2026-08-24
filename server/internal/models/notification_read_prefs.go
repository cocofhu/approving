package models

import "time"

// NotificationReadPrefs is the server-authoritative per-user read state for
// run-terminal notifications (enabledAt baseline + readIds set).
// Table: notification_read_prefs. One row per authenticated username.
type NotificationReadPrefs struct {
	Username  string    `gorm:"primaryKey;size:191" json:"username"`
	EnabledAt time.Time `json:"enabledAt"`
	ReadIDs   []string  `gorm:"serializer:json" json:"readIds"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// TableName pins the clarified table name.
func (NotificationReadPrefs) TableName() string {
	return "notification_read_prefs"
}
