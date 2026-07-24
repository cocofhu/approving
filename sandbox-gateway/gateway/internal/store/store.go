package store

import (
	"context"
	"errors"

	"sandbox-gateway/internal/models"

	"gorm.io/gorm"
)

// ErrNotFound is returned when a sandbox record does not exist.
var ErrNotFound = errors.New("sandbox not found")

// Store persists sandbox metadata.
type Store struct {
	db *gorm.DB
}

// New builds a Store over the given GORM handle.
func New(db *gorm.DB) *Store { return &Store{db: db} }

func (s *Store) sess(ctx context.Context) *gorm.DB {
	if ctx == nil {
		return s.db
	}
	return s.db.WithContext(ctx)
}

// Create inserts a new sandbox record.
func (s *Store) Create(ctx context.Context, sb *models.Sandbox) error {
	return s.sess(ctx).Create(sb).Error
}

// Save upserts a sandbox record.
func (s *Store) Save(ctx context.Context, sb *models.Sandbox) error {
	return s.sess(ctx).Save(sb).Error
}

// Get returns a sandbox by id.
func (s *Store) Get(ctx context.Context, id string) (*models.Sandbox, error) {
	var sb models.Sandbox
	err := s.sess(ctx).First(&sb, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &sb, nil
}

// GetByName returns a sandbox by driver-native name (e.g. sbx-<id>).
func (s *Store) GetByName(ctx context.Context, name string) (*models.Sandbox, error) {
	var sb models.Sandbox
	err := s.sess(ctx).First(&sb, "name = ?", name).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &sb, nil
}

// ListFilter selects sandboxes. Empty Labels means no label filter.
type ListFilter struct {
	Labels map[string]string // all key/value pairs must match (AND)
}

// List returns sandbox records matching filter, newest first.
func (s *Store) List(ctx context.Context, filter ListFilter) ([]models.Sandbox, error) {
	var out []models.Sandbox
	err := s.sess(ctx).Order("created_at desc").Find(&out).Error
	if err != nil || len(filter.Labels) == 0 {
		return out, err
	}
	matched := make([]models.Sandbox, 0, len(out))
	for i := range out {
		labels := out[i].Labels()
		ok := true
		for k, v := range filter.Labels {
			if labels[k] != v {
				ok = false
				break
			}
		}
		if ok {
			matched = append(matched, out[i])
		}
	}
	return matched, nil
}

// Delete removes a sandbox record.
func (s *Store) Delete(ctx context.Context, id string) error {
	return s.sess(ctx).Delete(&models.Sandbox{}, "id = ?", id).Error
}
