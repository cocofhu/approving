package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/cocofhu/approving/internal/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// ListProjectTasks handles GET /api/projects/:id/pm/tasks.
//
// Query: active=1 (default) lists non-terminal rows for the project-management
// 待办 view; active=0 includes closed rows. limit caps the page size.
func (h *Handlers) ListProjectTasks(c *gin.Context) {
	projectID := c.Param("id")
	if !h.requireProjectAuditAccess(c, projectID) {
		return
	}
	if h.TaskContext == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "task store unavailable"})
		return
	}
	q := services.ProjectTaskQuery{ActiveOnly: true, Limit: 100}
	if active := strings.TrimSpace(c.Query("active")); active == "0" || strings.EqualFold(active, "false") {
		q.ActiveOnly = false
	}
	if lim, err := strconv.Atoi(c.Query("limit")); err == nil {
		q.Limit = lim
	}
	items, err := h.TaskContext.ListProjectTasks(projectID, q)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

type closeProjectTaskBody struct {
	Status string `json:"status"`
}

// CloseProjectTask handles POST /api/projects/:id/pm/tasks/:taskId/close.
// Body: { "status": "completed" | "cancelled" | "failed" }. Defaults to cancelled.
func (h *Handlers) CloseProjectTask(c *gin.Context) {
	projectID := c.Param("id")
	taskID := c.Param("taskId")
	if !h.requireProjectAuditAccess(c, projectID) {
		return
	}
	if h.TaskContext == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "task store unavailable"})
		return
	}
	var body closeProjectTaskBody
	_ = c.ShouldBindJSON(&body)
	status := strings.TrimSpace(body.Status)
	if status == "" {
		status = "cancelled"
	}
	updated, err := h.TaskContext.CloseProjectTask(projectID, taskID, status)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"task": updated})
}
