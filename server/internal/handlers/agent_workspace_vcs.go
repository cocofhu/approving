package handlers

import (
	"errors"
	"net/http"
	"strings"

	"github.com/cocofhu/approving/internal/models"
	"github.com/cocofhu/approving/internal/services"
	"github.com/gin-gonic/gin"
)

func sessionUsername(c *gin.Context) string {
	if sess, ok := c.Get("auth_session"); ok {
		if s, ok := sess.(*models.Session); ok && s != nil {
			return strings.TrimSpace(s.Username)
		}
	}
	return "studio"
}

// ListAgentWorkspaceRevisions handles GET /api/agents/:name/workspace/revisions
func (h *Handlers) ListAgentWorkspaceRevisions(c *gin.Context) {
	name := c.Param("name")
	if _, ok := h.Agents.Get(name); !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	revs, err := h.Agents.Vcs.ListRevisions(name, 100)
	if err != nil {
		_ = c.Error(err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"revisions": revs})
}

// GetAgentWorkspaceRevisionDiff handles GET /api/agents/:name/workspace/revisions/:sha/diff
func (h *Handlers) GetAgentWorkspaceRevisionDiff(c *gin.Context) {
	name := c.Param("name")
	sha := c.Param("sha")
	if _, ok := h.Agents.Get(name); !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	diff, err := h.Agents.Vcs.DiffRevision(name, sha)
	if err != nil {
		if errors.Is(err, services.ErrVcsRevisionMiss) {
			c.JSON(http.StatusNotFound, gin.H{"error": "revision not found"})
			return
		}
		_ = c.Error(err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"sha": sha, "diff": diff})
}

type restoreWorkspaceBody struct {
	Reason string `json:"reason"`
}

// RestoreAgentWorkspaceRevision handles POST /api/agents/:name/workspace/revisions/:sha/restore
func (h *Handlers) RestoreAgentWorkspaceRevision(c *gin.Context) {
	name := c.Param("name")
	sha := c.Param("sha")
	if _, ok := h.Agents.Get(name); !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	var b restoreWorkspaceBody
	if err := c.ShouldBindJSON(&b); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	reason := strings.TrimSpace(b.Reason)
	if reason == "" {
		reason = "回滚到 " + sha
	}
	newSha, err := h.Agents.RestoreWorkspaceVcs(name, sha, sessionUsername(c), reason)
	if err != nil {
		if errors.Is(err, services.ErrVcsReasonRequired) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if errors.Is(err, services.ErrVcsRevisionMiss) {
			c.JSON(http.StatusNotFound, gin.H{"error": "revision not found"})
			return
		}
		_ = c.Error(err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	a, _ := h.Agents.Get(name)
	c.JSON(http.StatusOK, gin.H{"status": "restored", "sha": newSha, "agent": a})
}
