package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/cocofhu/approving/internal/models"
	"github.com/cocofhu/approving/internal/services"

	"github.com/gin-gonic/gin"
)

type projectCreateBody struct {
	Name        string                   `json:"name"`
	Description string                   `json:"description"`
	SandboxEnv  []models.EnvEntry        `json:"sandboxEnv"`
	Variables   []models.ProjectVariable `json:"variables"`
}

type projectUpdateBody struct {
	Name         *string                     `json:"name"`
	Description  *string                     `json:"description"`
	SandboxEnv   *[]models.EnvEntry          `json:"sandboxEnv"`
	Variables    *[]models.ProjectVariable   `json:"variables"`
	NotifyPolicy *models.ProjectNotifyPolicy `json:"notifyPolicy"`
}

func (h *Handlers) ListProjects(c *gin.Context) {
	if h.Projects == nil {
		c.JSON(http.StatusOK, []gin.H{})
		return
	}
	ps := h.Projects.List()
	ids := make([]string, len(ps))
	for i, p := range ps {
		ids[i] = p.ID
	}
	tokens := h.Projects.TokenBreakdownByProjectIDs(ids)
	out := make([]gin.H, 0, len(ps))
	for _, p := range ps {
		out = append(out, projectDTO(p, h.Projects.WorkflowCount(p.ID), tokens[p.ID]))
	}
	c.JSON(http.StatusOK, out)
}

func (h *Handlers) GetProject(c *gin.Context) {
	if h.Projects == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	p, ok := h.Projects.Get(c.Param("id"))
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	c.JSON(http.StatusOK, projectDTO(p, h.Projects.WorkflowCount(p.ID), h.Projects.TokenBreakdown(p.ID)))
}

func (h *Handlers) ListProjectRunTags(c *gin.Context) {
	if h.Projects == nil || h.Runs == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	if _, ok := h.Projects.Get(c.Param("id")); !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"tags": h.Runs.ProjectRunTags(c.Param("id"))})
}

func (h *Handlers) CreateProject(c *gin.Context) {
	if h.Projects == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "projects unavailable"})
		return
	}
	var b projectCreateBody
	if err := c.ShouldBindJSON(&b); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	p, err := h.Projects.Create(b.Name, b.Description, b.SandboxEnv, b.Variables)
	if err != nil {
		writeProjectErr(c, err)
		return
	}
	h.recordAudit(services.AuditRecord{
		ProjectID:    p.ID,
		Actor:        h.auditActorFromContext(c),
		Action:       models.AuditActionProjectConfig,
		ResourceType: "project",
		ResourceID:   p.ID,
		Outcome:      models.AuditOutcomeOK,
		Summary:      "create project",
		Payload: map[string]any{
			"name":        p.Name,
			"description": p.Description,
			"sandboxEnv":  services.MaskSandboxEnvForAudit(p.SandboxEnv),
			"variables":   services.MaskProjectVarsForAudit(p.Variables),
		},
	})
	c.JSON(http.StatusOK, projectDTO(p, 0, services.ProjectTokenBreakdown{}))
}

func (h *Handlers) UpdateProject(c *gin.Context) {
	if h.Projects == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "projects unavailable"})
		return
	}
	var b projectUpdateBody
	if err := c.ShouldBindJSON(&b); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	id := c.Param("id")
	p, err := h.Projects.Update(id, b.Name, b.Description, b.SandboxEnv, b.Variables, b.NotifyPolicy)
	if err != nil {
		writeProjectErr(c, err)
		return
	}
	changed := []string{}
	if b.Name != nil {
		changed = append(changed, "name")
	}
	if b.Description != nil {
		changed = append(changed, "description")
	}
	if b.SandboxEnv != nil {
		changed = append(changed, "sandboxEnv")
	}
	if b.Variables != nil {
		changed = append(changed, "variables")
	}
	if b.NotifyPolicy != nil {
		changed = append(changed, "notifyPolicy")
	}
	payload := map[string]any{"changed": changed, "name": p.Name}
	if b.SandboxEnv != nil {
		payload["sandboxEnv"] = services.MaskSandboxEnvForAudit(p.SandboxEnv)
	}
	if b.Variables != nil {
		payload["variables"] = services.MaskProjectVarsForAudit(p.Variables)
	}
	h.recordAudit(services.AuditRecord{
		ProjectID:    p.ID,
		Actor:        h.auditActorFromContext(c),
		Action:       models.AuditActionProjectConfig,
		ResourceType: "project",
		ResourceID:   p.ID,
		Outcome:      models.AuditOutcomeOK,
		Summary:      "update project config",
		Payload:      payload,
	})
	c.JSON(http.StatusOK, projectDTO(p, h.Projects.WorkflowCount(p.ID), h.Projects.TokenBreakdown(p.ID)))
}

func (h *Handlers) DeleteProject(c *gin.Context) {
	if h.Projects == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "projects unavailable"})
		return
	}
	id := c.Param("id")
	actor := h.auditActorFromContext(c)
	if err := h.Projects.Delete(id); err != nil {
		if errors.Is(err, services.ErrProjectHasWorkflows) {
			n := h.Projects.WorkflowCount(id)
			c.JSON(http.StatusConflict, gin.H{"error": services.FormatProjectHasWorkflowsError(n)})
			return
		}
		writeProjectErr(c, err)
		return
	}
	h.recordAudit(services.AuditRecord{
		ProjectID:    id,
		Actor:        actor,
		Action:       models.AuditActionProjectConfig,
		ResourceType: "project",
		ResourceID:   id,
		Outcome:      models.AuditOutcomeOK,
		Summary:      "delete project",
		Payload:      map[string]any{"deleted": true},
	})
	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}

// GetProjectTokenStats returns trend/composition/workflows for board Token charts.
// Query: window=7d|30d|90d|all (default 30d), timezone=IANA (preferred),
// utcOffsetMinutes=int (fallback fixed offset, east of UTC positive).
func (h *Handlers) GetProjectTokenStats(c *gin.Context) {
	if h.Projects == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "projects unavailable"})
		return
	}
	id := c.Param("id")
	if _, ok := h.Projects.Get(id); !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}

	q := services.TokenStatsQuery{
		Window:   c.DefaultQuery("window", services.TokenStatsWindow30d),
		Timezone: c.Query("timezone"),
	}
	if raw := strings.TrimSpace(c.Query("utcOffsetMinutes")); raw != "" {
		mins, err := strconv.Atoi(raw)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid utcOffsetMinutes"})
			return
		}
		q.UTCOffsetMinutes = &mins
	}

	result, err := h.Projects.TokenStats(c.Request.Context(), id, q)
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

func writeProjectErr(c *gin.Context, err error) {
	switch {
	case errors.Is(err, services.ErrEmptyProjectName),
		errors.Is(err, services.ErrSecretPlaceholderOnNewKey):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, services.ErrProjectNameExists):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	case errors.Is(err, services.ErrProjectNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	case errors.Is(err, services.ErrProjectHasWorkflows):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	default:
		_ = c.Error(err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	}
}
