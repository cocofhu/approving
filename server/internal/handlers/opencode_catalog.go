package handlers

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

type openCodeProviderDTO struct {
	ID     string `json:"id"`
	Name   string `json:"name,omitempty"`
	API    string `json:"api,omitempty"`
	KeyEnv string `json:"keyEnv,omitempty"`
	// Models counts the catalog entries, so the picker can say up front that a
	// provider has none to offer without asking for the list.
	Models int `json:"models"`
}

// ListOpenCodeProviders answers the vendors OpenCode can resolve models for.
// Models are omitted here: the whole catalog is megabytes, and the picker only
// needs one vendor's list at a time.
func (h *Handlers) ListOpenCodeProviders(c *gin.Context) {
	if h.OpenCodeCatalog == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "opencode catalog unavailable"})
		return
	}
	providers, fetchedAt, err := h.OpenCodeCatalog.Providers(c.Request.Context())
	if err != nil {
		// The catalog is a convenience: the caller falls back to typing ids by
		// hand, so report the reason rather than failing the whole form.
		c.JSON(http.StatusOK, gin.H{"providers": []openCodeProviderDTO{}, "error": err.Error()})
		return
	}
	out := make([]openCodeProviderDTO, 0, len(providers))
	for _, p := range providers {
		out = append(out, openCodeProviderDTO{
			ID: p.ID, Name: p.Name, API: p.API, KeyEnv: p.KeyEnv, Models: len(p.Models),
		})
	}
	c.JSON(http.StatusOK, gin.H{"providers": out, "fetchedAt": fetchedAt})
}

// ListOpenCodeProviderModels answers one vendor's models. An unknown vendor —
// a company gateway, say — yields an empty list, not an error: its ids are typed
// in by hand and work just the same.
func (h *Handlers) ListOpenCodeProviderModels(c *gin.Context) {
	if h.OpenCodeCatalog == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "opencode catalog unavailable"})
		return
	}
	provider := strings.TrimSpace(c.Param("provider"))
	if provider == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "provider required"})
		return
	}
	models, err := h.OpenCodeCatalog.Models(c.Request.Context(), provider)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"models": []any{}, "error": err.Error()})
		return
	}
	out := make([]gin.H, 0, len(models))
	for _, m := range models {
		out = append(out, gin.H{"id": m.ID, "name": m.Name})
	}
	c.JSON(http.StatusOK, gin.H{"models": out})
}
