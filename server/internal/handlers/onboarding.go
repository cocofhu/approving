package handlers

import (
	"errors"
	"net/http"
	"strings"

	"github.com/cocofhu/approving/internal/services"

	"github.com/gin-gonic/gin"
)

// BootstrapProjectOnboarding handles POST /api/projects/:id/bootstrap-onboarding.
// First-install only: default project shared auth, 综合项目组, 默认工作流.
// It never starts a Run. Missing apiKey → 400 with no partial resources created.
func (h *Handlers) BootstrapProjectOnboarding(c *gin.Context) {
	if h.Onboarding == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "onboarding unavailable"})
		return
	}
	projectID := strings.TrimSpace(c.Param("id"))
	var req services.OnboardingBootstrapRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	result, err := h.Onboarding.Bootstrap(projectID, req)
	if err != nil {
		switch {
		case errors.Is(err, services.ErrOnboardingAPIKeyRequired),
			errors.Is(err, services.ErrOnboardingNotDefaultProject):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		case errors.Is(err, services.ErrOnboardingProjectNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		case errors.Is(err, services.ErrOnboardingAgentConflict):
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		}
		return
	}
	c.JSON(http.StatusOK, result)
}
