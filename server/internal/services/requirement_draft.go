package services

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/cocofhu/approving/internal/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	// DefaultRequirementDraftTitle is created on「新建」before the user edits.
	DefaultRequirementDraftTitle = "未命名需求"
	// MaxRequirementDraftTitleRunes caps the trimmed title length.
	MaxRequirementDraftTitleRunes = 200
	// MaxRequirementDraftBodyRunes caps Markdown body size (reasonable TEXT upper bound).
	MaxRequirementDraftBodyRunes = 200_000
)

var (
	// ErrRequirementDraftNotFound is returned when a draft id is missing in the project.
	ErrRequirementDraftNotFound = errors.New("requirement draft not found")
	// ErrRequirementDraftEmptyTitle is returned when save title trims to empty.
	ErrRequirementDraftEmptyTitle = errors.New("title must not be empty")
	// ErrRequirementDraftInvalidStatus is returned for status outside open|done.
	ErrRequirementDraftInvalidStatus = errors.New("invalid status")
	// ErrRequirementDraftTitleTooLong is returned when title exceeds the rune cap.
	ErrRequirementDraftTitleTooLong = errors.New("title too long")
	// ErrRequirementDraftBodyTooLong is returned when body exceeds the rune cap.
	ErrRequirementDraftBodyTooLong = errors.New("body too long")
)

// RequirementDraftService persists project-scoped requirement drafts.
type RequirementDraftService struct {
	db *gorm.DB
}

// NewRequirementDraftService builds the service.
func NewRequirementDraftService(db *gorm.DB) *RequirementDraftService {
	return &RequirementDraftService{db: db}
}

// ListFilter controls list status + title search.
type RequirementDraftListFilter struct {
	// Status is open|done|all (empty treated as all).
	Status string
	// Query is a case-insensitive title substring filter (empty = no filter).
	Query string
}

// List returns drafts for a project ordered by updated_at DESC.
func (s *RequirementDraftService) List(projectID string, f RequirementDraftListFilter) ([]models.RequirementDraft, error) {
	if err := s.requireProject(projectID); err != nil {
		return nil, err
	}
	q := s.db.Where("project_id = ?", projectID)
	switch strings.TrimSpace(f.Status) {
	case "", "all":
		// no status filter
	case models.RequirementDraftStatusOpen, models.RequirementDraftStatusDone:
		q = q.Where("status = ?", strings.TrimSpace(f.Status))
	default:
		return nil, ErrRequirementDraftInvalidStatus
	}
	if qq := strings.TrimSpace(f.Query); qq != "" {
		like := "%" + escapeLikePattern(qq) + "%"
		q = q.Where("title LIKE ? ESCAPE ?", like, `\`)
	}
	var rows []models.RequirementDraft
	if err := q.Order("updated_at DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	if rows == nil {
		rows = []models.RequirementDraft{}
	}
	return rows, nil
}

// Get returns one draft scoped to projectID.
func (s *RequirementDraftService) Get(projectID, id string) (models.RequirementDraft, error) {
	if err := s.requireProject(projectID); err != nil {
		return models.RequirementDraft{}, err
	}
	var row models.RequirementDraft
	err := s.db.Where("id = ? AND project_id = ?", id, projectID).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return models.RequirementDraft{}, ErrRequirementDraftNotFound
	}
	if err != nil {
		return models.RequirementDraft{}, err
	}
	return row, nil
}

// Create inserts a new open draft with default title and empty body.
func (s *RequirementDraftService) Create(projectID string) (models.RequirementDraft, error) {
	if err := s.requireProject(projectID); err != nil {
		return models.RequirementDraft{}, err
	}
	now := time.Now()
	row := models.RequirementDraft{
		ID:           "rd-" + uuid.NewString()[:8],
		ProjectID:    projectID,
		Title:        DefaultRequirementDraftTitle,
		BodyMarkdown: "",
		Status:       models.RequirementDraftStatusOpen,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := s.db.Create(&row).Error; err != nil {
		return models.RequirementDraft{}, err
	}
	return row, nil
}

// UpdateContent updates title + body (explicit save). Title must trim non-empty.
func (s *RequirementDraftService) UpdateContent(projectID, id, title, body string) (models.RequirementDraft, error) {
	row, err := s.Get(projectID, id)
	if err != nil {
		return models.RequirementDraft{}, err
	}
	title = strings.TrimSpace(title)
	if title == "" {
		return models.RequirementDraft{}, ErrRequirementDraftEmptyTitle
	}
	if utf8.RuneCountInString(title) > MaxRequirementDraftTitleRunes {
		return models.RequirementDraft{}, ErrRequirementDraftTitleTooLong
	}
	if utf8.RuneCountInString(body) > MaxRequirementDraftBodyRunes {
		return models.RequirementDraft{}, ErrRequirementDraftBodyTooLong
	}
	row.Title = title
	row.BodyMarkdown = body // empty body allowed
	row.UpdatedAt = time.Now()
	if err := s.db.Save(&row).Error; err != nil {
		return models.RequirementDraft{}, err
	}
	return row, nil
}

// UpdateStatus toggles or sets status to open|done and refreshes updatedAt.
func (s *RequirementDraftService) UpdateStatus(projectID, id, status string) (models.RequirementDraft, error) {
	row, err := s.Get(projectID, id)
	if err != nil {
		return models.RequirementDraft{}, err
	}
	status = strings.TrimSpace(status)
	if status != models.RequirementDraftStatusOpen && status != models.RequirementDraftStatusDone {
		return models.RequirementDraft{}, ErrRequirementDraftInvalidStatus
	}
	row.Status = status
	row.UpdatedAt = time.Now()
	if err := s.db.Save(&row).Error; err != nil {
		return models.RequirementDraft{}, err
	}
	return row, nil
}

// Delete hard-removes a draft within the project scope.
func (s *RequirementDraftService) Delete(projectID, id string) error {
	if err := s.requireProject(projectID); err != nil {
		return err
	}
	res := s.db.Where("id = ? AND project_id = ?", id, projectID).Delete(&models.RequirementDraft{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrRequirementDraftNotFound
	}
	return nil
}

// DeleteByProject hard-removes all drafts for a project (cascade helper).
func (s *RequirementDraftService) DeleteByProject(tx *gorm.DB, projectID string) error {
	db := s.db
	if tx != nil {
		db = tx
	}
	return db.Where("project_id = ?", projectID).Delete(&models.RequirementDraft{}).Error
}

func (s *RequirementDraftService) requireProject(projectID string) error {
	var n int64
	if err := s.db.Model(&models.Project{}).Where("id = ?", projectID).Count(&n).Error; err != nil {
		return err
	}
	if n == 0 {
		return ErrProjectNotFound
	}
	return nil
}
