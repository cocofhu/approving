package handlers

import (
	"errors"
	"net/http"

	"github.com/cocofhu/approving/internal/engine"
	"github.com/cocofhu/approving/internal/services"
	"github.com/gin-gonic/gin"
)

func (h *Handlers) ListArtifacts(c *gin.Context) {
	wf := c.Query("wf")
	projectID := c.Query("projectId")
	pg, ok := parsePagination(c)
	if !ok {
		return
	}
	if !pg.Active {
		c.JSON(http.StatusOK, h.Arts.All())
		return
	}
	q := c.Query("q")

	if c.Query("groupBy") == "run" {
		arts, total := h.Arts.AllPageByRun(wf, projectID, pg.Page, pg.PageSize, q)
		c.JSON(http.StatusOK, paginatedResponse(arts, int(total), pg.Page, pg.PageSize))
		return
	}
	arts, total := h.Arts.AllPage(wf, projectID, pg.Page, pg.PageSize, q)
	c.JSON(http.StatusOK, paginatedResponse(arts, int(total), pg.Page, pg.PageSize))
}

// ArtifactContent returns a single artifact's full record including its
// content (the list/run DTOs omit content to stay lightweight).
func (h *Handlers) ArtifactContent(c *gin.Context) {
	a, ok := h.Arts.GetByID(c.Param("id"))
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}

	content := a.Content
	etag := engine.ArtifactETag(content, a.SizeBytes, a.UpdatedAt)
	out := gin.H{
		"id": a.ID, "runId": a.RunID, "nodeId": a.NodeID, "workflowId": a.WorkflowID, "workflowName": a.WorkflowName,
		"name": a.Name, "kind": a.Kind, "sizeBytes": a.SizeBytes,
		"createdAt": a.CreatedAt, "content": content, "etag": etag,
	}
	if !a.UpdatedAt.IsZero() {
		out["updatedAt"] = a.UpdatedAt
	}
	c.Header("ETag", etag)
	c.JSON(http.StatusOK, out)
}

func (h *Handlers) DownloadArtifact(c *gin.Context) {
	a, ok := h.Arts.GetByID(c.Param("id"))
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	body, mime := decodeArtifactDownloadBody(a)
	c.Header("Content-Disposition", "attachment; filename="+a.Name)
	c.Data(http.StatusOK, mime, body)
}

// DeleteArtifact hard-deletes one artifact by id. Success is 204 No Content
// (no body). Missing id → 404; owning run not terminal → 409.
func (h *Handlers) DeleteArtifact(c *gin.Context) {
	if err := h.Arts.DeleteByID(c.Param("id")); err != nil {
		switch {
		case errors.Is(err, services.ErrArtifactNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		case errors.Is(err, services.ErrArtifactRunNotTerminal):
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		default:
			_ = c.Error(err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}
	c.Status(http.StatusNoContent)
}
