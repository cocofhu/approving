package handlers

import (
	"net/http"

	"github.com/cocofhu/approving/internal/auth"

	"github.com/gin-gonic/gin"
)

type platformRuleBody struct {
	Content string `json:"content"`
}

func (h *Handlers) requireAdmin(c *gin.Context) bool {
	sess, ok := auth.GetSession(c)
	if !ok || h.Auth == nil || !h.Auth.IsAdmin(sess.Username) {
		c.JSON(http.StatusForbidden, gin.H{"error": "admin required"})
		return false
	}
	return true
}

// ListPlatformRules handles GET /api/platform-rules.
func (h *Handlers) ListPlatformRules(c *gin.Context) {
	items, err := h.PlatformRules.ListGlobal()
	if err != nil {
		_ = c.Error(err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

// GetPlatformRule handles GET /api/platform-rules/:file.
func (h *Handlers) GetPlatformRule(c *gin.Context) {
	item, err := h.PlatformRules.GetGlobal(c.Param("file"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, item)
}

// SavePlatformRule handles PUT /api/platform-rules/:file.
func (h *Handlers) SavePlatformRule(c *gin.Context) {
	if !h.requireAdmin(c) {
		return
	}
	var body platformRuleBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	item, err := h.PlatformRules.SaveGlobal(c.Param("file"), body.Content)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, item)
}

// DeletePlatformRule handles DELETE /api/platform-rules/:file.
func (h *Handlers) DeletePlatformRule(c *gin.Context) {
	if !h.requireAdmin(c) {
		return
	}
	if err := h.PlatformRules.DeleteGlobal(c.Param("file")); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}

// ResetPlatformRule handles POST /api/platform-rules/:file/reset.
func (h *Handlers) ResetPlatformRule(c *gin.Context) {
	if !h.requireAdmin(c) {
		return
	}
	item, err := h.PlatformRules.ResetGlobal(c.Param("file"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, item)
}

// GetPlatformRuleEmbed handles GET /api/platform-rules/:file/embed.
func (h *Handlers) GetPlatformRuleEmbed(c *gin.Context) {
	item, err := h.PlatformRules.ReadEmbedDefault(c.Param("file"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, item)
}

// ListAgentPlatformRules handles GET /api/agents/:name/platform-rules.
func (h *Handlers) ListAgentPlatformRules(c *gin.Context) {
	agent := c.Param("name")
	if !h.Agents.Exists(agent) {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	items, err := h.PlatformRules.ListAgent(agent)
	if err != nil {
		_ = c.Error(err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

// GetAgentPlatformRule handles GET /api/agents/:name/platform-rules/:file.
func (h *Handlers) GetAgentPlatformRule(c *gin.Context) {
	agent := c.Param("name")
	if !h.Agents.Exists(agent) {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	item, err := h.PlatformRules.GetAgent(agent, c.Param("file"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, item)
}

// SaveAgentPlatformRule handles PUT /api/agents/:name/platform-rules/:file.
func (h *Handlers) SaveAgentPlatformRule(c *gin.Context) {
	agent := c.Param("name")
	if !h.Agents.Exists(agent) {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	var body platformRuleBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	item, err := h.PlatformRules.SaveAgent(agent, c.Param("file"), body.Content)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, item)
}

// DeleteAgentPlatformRule handles DELETE /api/agents/:name/platform-rules/:file.
func (h *Handlers) DeleteAgentPlatformRule(c *gin.Context) {
	agent := c.Param("name")
	if !h.Agents.Exists(agent) {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	if err := h.PlatformRules.DeleteAgent(agent, c.Param("file")); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}
