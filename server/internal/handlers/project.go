package handlers

import (
	"errors"
	"net/http"

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
	Name        *string                   `json:"name"`
	Description *string                   `json:"description"`
	SandboxEnv  *[]models.EnvEntry        `json:"sandboxEnv"`
	Variables   *[]models.ProjectVariable `json:"variables"`
}

func (h *Handlers) ListProjects(c *gin.Context) {
	if h.Projects == nil {
		c.JSON(http.StatusOK, []gin.H{})
		return
	}
	ps := h.Projects.List()
	out := make([]gin.H, 0, len(ps))
	for _, p := range ps {
		out = append(out, projectDTO(p, h.Projects.WorkflowCount(p.ID)))
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
	c.JSON(http.StatusOK, projectDTO(p, h.Projects.WorkflowCount(p.ID)))
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
	c.JSON(http.StatusOK, projectDTO(p, 0))
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
	p, err := h.Projects.Update(c.Param("id"), b.Name, b.Description, b.SandboxEnv, b.Variables)
	if err != nil {
		writeProjectErr(c, err)
		return
	}
	c.JSON(http.StatusOK, projectDTO(p, h.Projects.WorkflowCount(p.ID)))
}

func (h *Handlers) DeleteProject(c *gin.Context) {
	if h.Projects == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "projects unavailable"})
		return
	}
	id := c.Param("id")
	if err := h.Projects.Delete(id); err != nil {
		if errors.Is(err, services.ErrProjectHasWorkflows) {
			n := h.Projects.WorkflowCount(id)
			c.JSON(http.StatusConflict, gin.H{"error": services.FormatProjectHasWorkflowsError(n)})
			return
		}
		writeProjectErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}

func writeProjectErr(c *gin.Context, err error) {
	switch {
	case errors.Is(err, services.ErrEmptyProjectName),
		errors.Is(err, services.ErrPlatformAuthEnvKey),
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
