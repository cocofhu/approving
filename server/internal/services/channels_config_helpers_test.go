package services

import (
	"errors"
	"strings"

	"github.com/cocofhu/approving/internal/models"
	"gorm.io/gorm"
)

// GetByID returns one channel DTO or ErrChannelNotFound (test helper).
func (s *ChannelConfigService) GetByID(id string) (ChannelConfigDTO, error) {
	var row models.ChannelConfig
	if err := s.db.First(&row, "id = ?", strings.TrimSpace(id)).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ChannelConfigDTO{}, ErrChannelNotFound
		}
		return ChannelConfigDTO{}, err
	}
	return toChannelDTO(row), nil
}
