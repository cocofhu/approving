package services

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/cocofhu/approving/internal/models"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// ProjectMcpApiKeyService manages project-scoped external MCP API keys.
type ProjectMcpApiKeyService struct{ db *gorm.DB }

// NewProjectMcpApiKeyService builds the service.
func NewProjectMcpApiKeyService(db *gorm.DB) *ProjectMcpApiKeyService {
	return &ProjectMcpApiKeyService{db: db}
}

// CreateProjectMcpKeyResult holds the one-time plaintext key.
type CreateProjectMcpKeyResult struct {
	Key       models.ProjectMcpApiKey
	Plaintext string
}

// Create generates a named key for a project. Plaintext is returned once.
func (s *ProjectMcpApiKeyService) Create(projectID, name string) (CreateProjectMcpKeyResult, error) {
	projectID = strings.TrimSpace(projectID)
	name = strings.TrimSpace(name)
	if projectID == "" {
		return CreateProjectMcpKeyResult{}, errors.New("project id required")
	}
	if name == "" {
		return CreateProjectMcpKeyResult{}, errors.New("name is required")
	}
	var proj models.Project
	if err := s.db.Select("id").First(&proj, "id = ?", projectID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return CreateProjectMcpKeyResult{}, ErrProjectNotFound
		}
		return CreateProjectMcpKeyResult{}, err
	}
	plain, err := generateProjectMcpKeyPlaintext()
	if err != nil {
		return CreateProjectMcpKeyResult{}, err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	if err != nil {
		return CreateProjectMcpKeyResult{}, fmt.Errorf("hash key: %w", err)
	}
	prefix := plain
	if len(plain) > 4 {
		prefix = "cf_proj_" + strings.Repeat("•", 16) + plain[len(plain)-4:]
	}
	key := models.ProjectMcpApiKey{
		ID:        "pmkey-" + uuid.NewString()[:8],
		ProjectID: projectID,
		Name:      name,
		KeyPrefix: prefix,
		KeyHash:   string(hash),
		CreatedAt: time.Now(),
	}
	if err := s.db.Create(&key).Error; err != nil {
		return CreateProjectMcpKeyResult{}, err
	}
	return CreateProjectMcpKeyResult{Key: key, Plaintext: plain}, nil
}

// List returns active keys for a project, newest first.
func (s *ProjectMcpApiKeyService) List(projectID string) []models.ProjectMcpApiKey {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return nil
	}
	var keys []models.ProjectMcpApiKey
	s.db.Where("project_id = ? AND revoked_at IS NULL", projectID).
		Order("created_at desc").Find(&keys)
	return keys
}

// Revoke marks a key revoked. Returns false when not found or already revoked.
func (s *ProjectMcpApiKeyService) Revoke(projectID, keyID string) bool {
	projectID = strings.TrimSpace(projectID)
	keyID = strings.TrimSpace(keyID)
	if projectID == "" || keyID == "" {
		return false
	}
	now := time.Now()
	res := s.db.Model(&models.ProjectMcpApiKey{}).
		Where("id = ? AND project_id = ? AND revoked_at IS NULL", keyID, projectID).
		Update("revoked_at", now)
	return res.RowsAffected > 0
}

// ValidateBearer checks plaintext and returns project + key metadata when valid.
func (s *ProjectMcpApiKeyService) ValidateBearer(plain string) (projectID, keyID, keyName string, ok bool) {
	plain = strings.TrimSpace(plain)
	if plain == "" || !strings.HasPrefix(plain, "cf_proj_") {
		return "", "", "", false
	}
	var keys []models.ProjectMcpApiKey
	s.db.Where("revoked_at IS NULL").Find(&keys)
	for _, k := range keys {
		if bcrypt.CompareHashAndPassword([]byte(k.KeyHash), []byte(plain)) == nil {
			return k.ProjectID, k.ID, k.Name, true
		}
	}
	return "", "", "", false
}

func generateProjectMcpKeyPlaintext() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "cf_proj_" + hex.EncodeToString(b), nil
}
