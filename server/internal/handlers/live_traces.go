package handlers

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/cocofhu/approving/internal/services"

	"github.com/gin-gonic/gin"
)

// ListLiveTraces handles GET /api/projects/:id/live-traces.
//
// Query params:
//   - conversationId: filter to one IM conversation
//   - traceId: fetch one turn's call chain
//   - limit: max rows (default 50, max 200)
//   - since: RFC3339 lower bound on createdAt
//
// This is the debug entry point for "what did Live / sandbox / MCP do for this
// chat turn?" — samples are best-effort and may have holes.
func (h *Handlers) ListLiveTraces(c *gin.Context) {
	projectID := c.Param("id")
	if !h.requireProjectAuditAccess(c, projectID) {
		return
	}
	if h.LiveSamples == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "live trace store unavailable"})
		return
	}
	q := services.SampleQuery{
		ProjectID:      projectID,
		ConversationID: strings.TrimSpace(c.Query("conversationId")),
		TraceID:        strings.TrimSpace(c.Query("traceId")),
		Limit:          50,
	}
	if lim, err := strconv.Atoi(c.Query("limit")); err == nil {
		q.Limit = lim
	}
	if since := strings.TrimSpace(c.Query("since")); since != "" {
		if t, err := time.Parse(time.RFC3339, since); err == nil {
			q.Since = t
		}
	}
	if q.TraceID != "" && q.ConversationID == "" {
		sample, err := h.LiveSamples.GetByTrace(projectID, q.TraceID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if sample == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "trace not found"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"items": []any{sample}})
		return
	}
	items, err := h.LiveSamples.List(q)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}
