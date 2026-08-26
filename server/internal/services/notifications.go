package services

import (
	"errors"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/cocofhu/approving/internal/models"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const NotificationPoolSize = 50

var (
	ErrNotificationUsernameRequired = errors.New("username required")
	ErrNotificationRunIDRequired    = errors.New("runId required")
)

// Progress / in-progress wording that must not appear as notification titles.
var noisyNotificationTitle = regexp.MustCompile(
	`(?i)运行中|排队中?|等待人工|等待中|(?:^|[\s·\-_/，,])(queued|running|waiting)(?:[\s·\-_/，,]|$)|waiting[_ ]human|in\s*progress`,
)

// NotificationService lists terminal-run notifications and records per-run reads.
type NotificationService struct {
	db   *gorm.DB
	runs *RunService
}

// NewNotificationService builds the service. runs may be nil (a RunService is created).
func NewNotificationService(db *gorm.DB, runs *RunService) *NotificationService {
	if runs == nil {
		runs = NewRunService(db)
	}
	return &NotificationService{db: db, runs: runs}
}

// NotificationItemDTO is one inbox row. Unread is computed server-side; no id array.
type NotificationItemDTO struct {
	RunID          string `json:"runId"`
	Status         string `json:"status"`
	Title          string `json:"title"`
	TitleNeutral   bool   `json:"titleNeutral"`
	WorkflowName   string `json:"workflowName"`
	StartedAt      string `json:"startedAt"`
	FinishedApprox string `json:"finishedApprox"`
	Unread         bool   `json:"unread"`
	BeforeBaseline bool   `json:"beforeBaseline"`
}

// List returns the current user's notification pool (up to 50 terminal runs).
// First access stamps EnabledAt=now so existing pool items are history (read).
func (s *NotificationService) List(username string) ([]NotificationItemDTO, error) {
	username, err := requireUsername(username)
	if err != nil {
		return nil, err
	}
	baseline, err := s.ensureBaseline(username)
	if err != nil {
		return nil, err
	}
	runs, _ := s.runs.ListPage(
		[]string{"completed", "failed"},
		"", "", 1, NotificationPoolSize,
		"started_at", "desc",
	)
	items := make([]NotificationItemDTO, 0, len(runs))
	ids := make([]string, 0, len(runs))
	for _, r := range runs {
		item, ok := MapRunToNotification(r)
		if !ok {
			continue
		}
		items = append(items, item)
		ids = append(ids, item.RunID)
	}
	sort.SliceStable(items, func(i, j int) bool {
		ti := parseMillis(items[i].FinishedApprox, items[i].StartedAt)
		tj := parseMillis(items[j].FinishedApprox, items[j].StartedAt)
		if ti != tj {
			return ti > tj
		}
		return items[i].RunID > items[j].RunID
	})
	read, err := s.readSet(username, ids)
	if err != nil {
		return nil, err
	}
	enabledAt := baseline.EnabledAt
	for i := range items {
		finished := parseTime(items[i].FinishedApprox, items[i].StartedAt)
		before := !finished.IsZero() && !enabledAt.IsZero() && !finished.After(enabledAt)
		items[i].BeforeBaseline = before
		_, marked := read[items[i].RunID]
		items[i].Unread = !before && !marked && !finished.IsZero() && finished.After(enabledAt)
	}
	return items, nil
}

// MarkRead inserts an ignore row for (username, runID). Idempotent.
func (s *NotificationService) MarkRead(username, runID string) error {
	username, err := requireUsername(username)
	if err != nil {
		return err
	}
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return ErrNotificationRunIDRequired
	}
	if _, err := s.ensureBaseline(username); err != nil {
		return err
	}
	return s.insertRead(username, runID)
}

// MarkAllRead inserts a read row for every run currently in the terminal pool.
// The client does not send an id list. Existing rows are left in place.
func (s *NotificationService) MarkAllRead(username string) error {
	username, err := requireUsername(username)
	if err != nil {
		return err
	}
	if _, err := s.ensureBaseline(username); err != nil {
		return err
	}
	runs, _ := s.runs.ListPage(
		[]string{"completed", "failed"},
		"", "", 1, NotificationPoolSize,
		"started_at", "desc",
	)
	for _, r := range runs {
		if r.Status != "completed" && r.Status != "failed" {
			continue
		}
		id := strings.TrimSpace(r.ID)
		if id == "" {
			continue
		}
		if err := s.insertRead(username, id); err != nil {
			return err
		}
	}
	return nil
}

func (s *NotificationService) insertRead(username, runID string) error {
	row := models.NotificationRead{
		Username: username,
		RunID:    runID,
		ReadAt:   time.Now().UTC(),
	}
	return s.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "username"}, {Name: "run_id"}},
		DoNothing: true,
	}).Create(&row).Error
}

func (s *NotificationService) ensureBaseline(username string) (models.NotificationBaseline, error) {
	var row models.NotificationBaseline
	err := s.db.Where("username = ?", username).First(&row).Error
	if err == nil {
		return row, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return models.NotificationBaseline{}, err
	}
	now := time.Now().UTC()
	row = models.NotificationBaseline{
		Username:  username,
		EnabledAt: now,
		CreatedAt: now,
	}
	if err := s.db.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
		return models.NotificationBaseline{}, err
	}
	if err := s.db.Where("username = ?", username).First(&row).Error; err != nil {
		return models.NotificationBaseline{}, err
	}
	return row, nil
}

func (s *NotificationService) readSet(username string, runIDs []string) (map[string]struct{}, error) {
	out := make(map[string]struct{}, len(runIDs))
	if len(runIDs) == 0 {
		return out, nil
	}
	var rows []models.NotificationRead
	if err := s.db.Where("username = ? AND run_id IN ?", username, runIDs).Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, r := range rows {
		out[r.RunID] = struct{}{}
	}
	return out, nil
}

func requireUsername(username string) (string, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return "", ErrNotificationUsernameRequired
	}
	return username, nil
}

// IsNoisyNotificationTitle reports progress-noise wording that must not be shown.
func IsNoisyNotificationTitle(title string) bool {
	s := strings.TrimSpace(title)
	if s == "" {
		return false
	}
	return noisyNotificationTitle.MatchString(s)
}

// MapRunToNotification projects a terminal run into an inbox item (no unread flags).
func MapRunToNotification(r models.Run) (NotificationItemDTO, bool) {
	if r.Status != "completed" && r.Status != "failed" {
		return NotificationItemDTO{}, false
	}
	raw := strings.TrimSpace(r.Title)
	workflowName := strings.TrimSpace(r.WorkflowName)
	finished := finishedApprox(r)
	started := r.StartedAt
	if started.IsZero() {
		started = r.CreatedAt
	}

	title := raw
	if title == "" {
		title = workflowName
	}
	if title == "" {
		title = r.ID
	}
	titleNeutral := false
	if IsNoisyNotificationTitle(title) {
		name := workflowName
		if name == "" {
			name = r.ID
		}
		title = name + " · " + r.Status
		titleNeutral = true
	}

	return NotificationItemDTO{
		RunID:          r.ID,
		Status:         r.Status,
		Title:          title,
		TitleNeutral:   titleNeutral,
		WorkflowName:   workflowName,
		StartedAt:      formatTimeUTC(started),
		FinishedApprox: formatTimeUTC(finished),
	}, true
}

func finishedApprox(r models.Run) time.Time {
	start := r.StartedAt
	if start.IsZero() {
		start = r.CreatedAt
	}
	if r.DurationSec > 0 && !start.IsZero() {
		return start.Add(time.Duration(r.DurationSec) * time.Second)
	}
	return start
}

func formatTimeUTC(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func parseTime(finishedApprox, startedAt string) time.Time {
	for _, s := range []string{finishedApprox, startedAt} {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
			return t
		}
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

func parseMillis(finishedApprox, startedAt string) int64 {
	t := parseTime(finishedApprox, startedAt)
	if t.IsZero() {
		return 0
	}
	return t.UnixMilli()
}
