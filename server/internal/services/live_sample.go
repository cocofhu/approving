package services

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/cocofhu/approving/internal/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// LiveSampleService persists conversation-layer decisions for later review and
// tuning.
//
// Recording is best-effort by design. A sample that fails to write must never
// cost the user their reply, so every call site logs and moves on; the dataset
// is allowed to have holes, the conversation is not.
type LiveSampleService struct {
	db  *gorm.DB
	now func() time.Time
}

func NewLiveSampleService(db *gorm.DB) *LiveSampleService {
	return &LiveSampleService{db: db, now: time.Now}
}

// SetClock overrides the sample timestamp source (tests).
func (s *LiveSampleService) SetClock(now func() time.Time) {
	if now != nil {
		s.now = now
	}
}

// Record stores one decision and returns its id.
func (s *LiveSampleService) Record(sample models.LiveDecisionSample) (string, error) {
	if s == nil || s.db == nil {
		return "", errors.New("live sample database is unavailable")
	}
	if strings.TrimSpace(sample.ID) == "" {
		sample.ID = "lds-" + uuid.NewString()[:12]
	}
	if sample.CreatedAt.IsZero() {
		sample.CreatedAt = s.now()
	}
	if err := s.db.Create(&sample).Error; err != nil {
		return "", err
	}
	return sample.ID, nil
}

// AttachOutcome fills in what the work layer concluded for a decision that was
// recorded before the work finished. The two halves of a delegation are minutes
// apart, and a dataset that only holds the first half cannot show whether the
// routing call was right.
func (s *LiveSampleService) AttachOutcome(id, outcome, egress string) error {
	if s == nil || s.db == nil {
		return errors.New("live sample database is unavailable")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return nil
	}
	updates := map[string]any{}
	if outcome = strings.TrimSpace(outcome); outcome != "" {
		updates["pm_outcome"] = outcome
	}
	if egress = strings.TrimSpace(egress); egress != "" {
		updates["egress"] = egress
	}
	if len(updates) == 0 {
		return nil
	}
	return s.db.Model(&models.LiveDecisionSample{}).Where("id = ?", id).Updates(updates).Error
}

// AppendSpanByTrace adds a call-chain step after the sample was committed
// (sandbox turn, synthesis, late delivery). Matching is by TraceID so the
// sandbox does not need the sample primary key.
func (s *LiveSampleService) AppendSpanByTrace(traceID string, span any) error {
	if s == nil || s.db == nil {
		return errors.New("live sample database is unavailable")
	}
	traceID = strings.TrimSpace(traceID)
	if traceID == "" || span == nil {
		return nil
	}
	var sample models.LiveDecisionSample
	if err := s.db.Where("trace_id = ?", traceID).Order("created_at DESC").First(&sample).Error; err != nil {
		return err
	}
	var spans []json.RawMessage
	if strings.TrimSpace(sample.Spans) != "" {
		_ = json.Unmarshal([]byte(sample.Spans), &spans)
	}
	raw, err := json.Marshal(span)
	if err != nil {
		return err
	}
	spans = append(spans, raw)
	body, err := json.Marshal(spans)
	if err != nil {
		return err
	}
	return s.db.Model(&models.LiveDecisionSample{}).Where("id = ?", sample.ID).
		Update("spans", string(body)).Error
}

// SampleQuery bounds an export or debug listing.
type SampleQuery struct {
	ProjectID      string
	ConversationID string
	TraceID        string
	Since          time.Time
	Limit          int
}

// List returns samples newest first, for offline review and export.
func (s *LiveSampleService) List(q SampleQuery) ([]models.LiveDecisionSample, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("live sample database is unavailable")
	}
	limit := q.Limit
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	db := s.db.Model(&models.LiveDecisionSample{})
	if p := strings.TrimSpace(q.ProjectID); p != "" {
		db = db.Where("project_id = ?", p)
	}
	if c := strings.TrimSpace(q.ConversationID); c != "" {
		db = db.Where("conversation_id = ?", c)
	}
	if t := strings.TrimSpace(q.TraceID); t != "" {
		db = db.Where("trace_id = ?", t)
	}
	if !q.Since.IsZero() {
		db = db.Where("created_at >= ?", q.Since)
	}
	var out []models.LiveDecisionSample
	if err := db.Order("created_at DESC").Limit(limit).Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

// GetByTrace returns the newest sample for a trace id inside a project.
func (s *LiveSampleService) GetByTrace(projectID, traceID string) (*models.LiveDecisionSample, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("live sample database is unavailable")
	}
	projectID, traceID = strings.TrimSpace(projectID), strings.TrimSpace(traceID)
	if projectID == "" || traceID == "" {
		return nil, errors.New("project_id and trace_id are required")
	}
	var sample models.LiveDecisionSample
	err := s.db.Where("project_id = ? AND trace_id = ?", projectID, traceID).
		Order("created_at DESC").First(&sample).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &sample, nil
}
