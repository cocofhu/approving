package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/cocofhu/approving/internal/services"
	"github.com/gin-gonic/gin"
)

// GetGlobalTokenStats returns cross-project token analytics for /stats.
// Query: window=7d|30d|90d|all (default 30d), timezone, utcOffsetMinutes,
// source=all|workflow|pm, projectId, modelKey.
func (h *Handlers) GetGlobalTokenStats(c *gin.Context) {
	if h.Projects == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "projects unavailable"})
		return
	}

	q := services.GlobalTokenStatsQuery{
		Window:    c.DefaultQuery("window", services.TokenStatsWindow30d),
		Timezone:  c.Query("timezone"),
		Source:    c.DefaultQuery("source", services.GlobalTokenStatsSourceAll),
		ProjectID: strings.TrimSpace(c.Query("projectId")),
		ModelKey:  strings.TrimSpace(c.Query("modelKey")),
	}
	if raw := strings.TrimSpace(c.Query("utcOffsetMinutes")); raw != "" {
		mins, err := strconv.Atoi(raw)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid utcOffsetMinutes"})
			return
		}
		q.UTCOffsetMinutes = &mins
	}

	result, err := h.Projects.GlobalTokenStats(c.Request.Context(), q)
	if err != nil {
		switch {
		case errors.Is(err, services.ErrInvalidTokenStatsWindow),
			errors.Is(err, services.ErrInvalidTokenStatsTimezone):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		case errors.Is(err, services.ErrTokenStatsTimeout):
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"error":     err.Error(),
				"retryable": true,
			})
		default:
			_ = c.Error(err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}
	c.JSON(http.StatusOK, result)
}
