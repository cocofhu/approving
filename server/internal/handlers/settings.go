package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// GetSettings returns the effective platform scheduling params (value +
// provenance + env-lock) for the settings page.
func (h *Handlers) GetSettings(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"items": h.Settings.Effective()})
}

// UpdateSettings persists a patch of platform scheduling params and applies
// them at runtime. Only keys present are changed; env-locked keys are ignored.
func (h *Handlers) UpdateSettings(c *gin.Context) {
	var body map[string]int
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	items, err := h.Settings.Update(body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}
