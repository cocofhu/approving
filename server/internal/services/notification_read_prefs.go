package services

import (
	"errors"
	"strings"
	"time"

	"github.com/cocofhu/approving/internal/models"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrNotificationPrefsUsernameRequired = errors.New("username required")
	ErrNotificationPrefsRunIDRequired    = errors.New("runId required")
)

// NotificationReadPrefsService persists per-user notification read prefs.
type NotificationReadPrefsService struct {
	db *gorm.DB
}

// NewNotificationReadPrefsService builds the service.
func NewNotificationReadPrefsService(db *gorm.DB) *NotificationReadPrefsService {
	return &NotificationReadPrefsService{db: db}
}

// NotificationPrefsDTO is the API-facing prefs shape (no username leak needed
// for the owner; callers already know their identity).
type NotificationPrefsDTO struct {
	EnabledAt string   `json:"enabledAt"`
	ReadIDs   []string `json:"readIds"`
}

func toDTO(row models.NotificationReadPrefs) NotificationPrefsDTO {
	ids := row.ReadIDs
	if ids == nil {
		ids = []string{}
	}
	return NotificationPrefsDTO{
		EnabledAt: row.EnabledAt.UTC().Format(time.RFC3339Nano),
		ReadIDs:   append([]string(nil), ids...),
	}
}

// GetOrInit returns prefs for username, creating enabledAt=now + empty readIds
// on first access.
func (s *NotificationReadPrefsService) GetOrInit(username string) (NotificationPrefsDTO, error) {
	row, err := s.getOrInitRow(username)
	if err != nil {
		return NotificationPrefsDTO{}, err
	}
	return toDTO(row), nil
}

// MarkRead adds runID to the user's readIds (idempotent).
func (s *NotificationReadPrefsService) MarkRead(username, runID string) (NotificationPrefsDTO, error) {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return NotificationPrefsDTO{}, ErrNotificationPrefsRunIDRequired
	}
	row, err := s.getOrInitRow(username)
	if err != nil {
		return NotificationPrefsDTO{}, err
	}
	if notificationReadIDContains(row.ReadIDs, runID) {
		return toDTO(row), nil
	}
	row.ReadIDs = append(append([]string(nil), row.ReadIDs...), runID)
	row.UpdatedAt = time.Now().UTC()
	if err := s.db.Save(&row).Error; err != nil {
		return NotificationPrefsDTO{}, err
	}
	return toDTO(row), nil
}

// MarkAllRead adds every non-empty runID in runIDs to the user's read set
// (idempotent union). Empty input still ensures the row exists (baseline).
func (s *NotificationReadPrefsService) MarkAllRead(username string, runIDs []string) (NotificationPrefsDTO, error) {
	row, err := s.getOrInitRow(username)
	if err != nil {
		return NotificationPrefsDTO{}, err
	}
	seen := make(map[string]struct{}, len(row.ReadIDs)+len(runIDs))
	next := make([]string, 0, len(row.ReadIDs)+len(runIDs))
	for _, id := range row.ReadIDs {
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		next = append(next, id)
	}
	changed := false
	for _, id := range runIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		next = append(next, id)
		changed = true
	}
	if !changed {
		return toDTO(row), nil
	}
	row.ReadIDs = next
	row.UpdatedAt = time.Now().UTC()
	if err := s.db.Save(&row).Error; err != nil {
		return NotificationPrefsDTO{}, err
	}
	return toDTO(row), nil
}

func (s *NotificationReadPrefsService) getOrInitRow(username string) (models.NotificationReadPrefs, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return models.NotificationReadPrefs{}, ErrNotificationPrefsUsernameRequired
	}
	var row models.NotificationReadPrefs
	err := s.db.Where("username = ?", username).First(&row).Error
	if err == nil {
		if row.ReadIDs == nil {
			row.ReadIDs = []string{}
		}
		return row, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return models.NotificationReadPrefs{}, err
	}
	now := time.Now().UTC()
	row = models.NotificationReadPrefs{
		Username:  username,
		EnabledAt: now,
		ReadIDs:   []string{},
		CreatedAt: now,
		UpdatedAt: now,
	}
	// Concurrent first-access: lose the insert race → load the winner.
	err = s.db.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error
	if err != nil {
		return models.NotificationReadPrefs{}, err
	}
	err = s.db.Where("username = ?", username).First(&row).Error
	if err != nil {
		return models.NotificationReadPrefs{}, err
	}
	if row.ReadIDs == nil {
		row.ReadIDs = []string{}
	}
	return row, nil
}

func notificationReadIDContains(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}
