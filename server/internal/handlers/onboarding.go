package handlers

import (
	"errors"
	"net/http"
	"strings"

	"github.com/cocofhu/approving/internal/apierr"
	"github.com/cocofhu/approving/internal/services"

	"github.com/gin-gonic/gin"
)

// BootstrapProjectOnboarding handles POST /api/projects/:id/bootstrap-onboarding.
// It writes project auth, creates/reuses 5 agents, and publishes the light workflow.
// It never starts a Run. Missing apiKey → 400 with no partial resources created
// (auth/agents/workflow are only written after the key check).
func (h *Handlers) BootstrapProjectOnboarding(c *gin.Context) {
	if h.Onboarding == nil {
		apierr.Internal(c, errors.New("onboarding unavailable"))
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
		case errors.Is(err, services.ErrOnboardingAPIKeyRequired):
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
