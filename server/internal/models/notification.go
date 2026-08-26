package models

import "time"

// NotificationRead records that a user has marked one terminal run as read.
// Table: notification_reads. One row per (username, run_id); insert-ignore.
type NotificationRead struct {
	ID       uint      `gorm:"primaryKey" json:"-"`
	Username string    `gorm:"size:191;not null;uniqueIndex:idx_notif_read_user_run" json:"username"`
	RunID    string    `gorm:"size:191;not null;uniqueIndex:idx_notif_read_user_run" json:"runId"`
	ReadAt   time.Time `json:"readAt"`
}

func (NotificationRead) TableName() string {
	return "notification_reads"
}

// NotificationBaseline is the per-user enable time. Terminal runs at or before
// EnabledAt are history (always read) without a notification_reads row.
// Written once on first server sight; never updated.
type NotificationBaseline struct {
	Username  string    `gorm:"primaryKey;size:191" json:"username"`
	EnabledAt time.Time `json:"enabledAt"`
	CreatedAt time.Time `json:"createdAt"`
}

func (NotificationBaseline) TableName() string {
	return "notification_baselines"
}
