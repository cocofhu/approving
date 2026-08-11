package handlers

import (
	"errors"
	"net/http"
	"strings"

	"github.com/cocofhu/approving/internal/services"

	"github.com/gin-gonic/gin"
)

type updateRequirementDraftBody struct {
	Title        *string `json:"title"`
	BodyMarkdown *string `json:"bodyMarkdown"`
}

type patchRequirementDraftStatusBody struct {
	Status string `json:"status"`
}

// ListRequirementDrafts returns project-scoped drafts.
// Query: status=open|done|all (default all), q=title substring.
func (h *Handlers) ListRequirementDrafts(c *gin.Context) {
	if h.RequirementDrafts == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "requirement drafts unavailable"})
		return
	}
	projectID := c.Param("id")
	items, err := h.RequirementDrafts.List(projectID, services.RequirementDraftListFilter{
		Status: c.DefaultQuery("status", "all"),
		Query:  c.Query("q"),
	})
	if err != nil {
		writeRequirementDraftErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

// GetRequirementDraft returns one draft within the project.
func (h *Handlers) GetRequirementDraft(c *gin.Context) {
	if h.RequirementDrafts == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "requirement drafts unavailable"})
		return
	}
	item, err := h.RequirementDrafts.Get(c.Param("id"), c.Param("draftId"))
	if err != nil {
		writeRequirementDraftErr(c, err)
		return
	}
	c.JSON(http.StatusOK, item)
}

// CreateRequirementDraft creates an open「未命名需求」draft and returns it.
func (h *Handlers) CreateRequirementDraft(c *gin.Context) {
	if h.RequirementDrafts == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "requirement drafts unavailable"})
		return
	}
	item, err := h.RequirementDrafts.Create(c.Param("id"))
	if err != nil {
		writeRequirementDraftErr(c, err)
		return
	}
	c.JSON(http.StatusOK, item)
}

// UpdateRequirementDraft saves title + body (explicit save path).
func (h *Handlers) UpdateRequirementDraft(c *gin.Context) {
	if h.RequirementDrafts == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "requirement drafts unavailable"})
		return
	}
	var body updateRequirementDraftBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	if body.Title == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "title required"})
		return
	}
	md := ""
	if body.BodyMarkdown != nil {
		md = *body.BodyMarkdown
	}
	item, err := h.RequirementDrafts.UpdateContent(c.Param("id"), c.Param("draftId"), *body.Title, md)
	if err != nil {
		writeRequirementDraftErr(c, err)
		return
	}
	c.JSON(http.StatusOK, item)
}

// PatchRequirementDraftStatus sets status to open|done.
func (h *Handlers) PatchRequirementDraftStatus(c *gin.Context) {
	if h.RequirementDrafts == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "requirement drafts unavailable"})
		return
	}
	var body patchRequirementDraftStatusBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	item, err := h.RequirementDrafts.UpdateStatus(c.Param("id"), c.Param("draftId"), strings.TrimSpace(body.Status))
	if err != nil {
		writeRequirementDraftErr(c, err)
		return
	}
	c.JSON(http.StatusOK, item)
}

// DeleteRequirementDraft hard-deletes one draft.
func (h *Handlers) DeleteRequirementDraft(c *gin.Context) {
	if h.RequirementDrafts == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "requirement drafts unavailable"})
		return
	}
	if err := h.RequirementDrafts.Delete(c.Param("id"), c.Param("draftId")); err != nil {
		writeRequirementDraftErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func writeRequirementDraftErr(c *gin.Context, err error) {
	switch {
	case errors.Is(err, services.ErrProjectNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	case errors.Is(err, services.ErrRequirementDraftNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	case errors.Is(err, services.ErrRequirementDraftEmptyTitle),
		errors.Is(err, services.ErrRequirementDraftInvalidStatus),
		errors.Is(err, services.ErrRequirementDraftTitleTooLong),
		errors.Is(err, services.ErrRequirementDraftBodyTooLong):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	default:
		_ = c.Error(err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	}
}
