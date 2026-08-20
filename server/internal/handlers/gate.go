package handlers

import (
	"net/http"
	"strings"

	"github.com/cocofhu/approving/internal/engine"
	"github.com/cocofhu/approving/internal/models"
	"github.com/cocofhu/approving/internal/nodereg"
	"github.com/gin-gonic/gin"
)

type gateResumeBody struct {
	Action string         `json:"action"`
	Form   map[string]any `json:"form"`
}

func (h *Handlers) ResumeGate(c *gin.Context) {
	var b gateResumeBody
	if err := c.ShouldBindJSON(&b); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	actor := h.auditActorFromContext(c)
	reviewer := ""
	if !actor.Unattributable {
		reviewer = actor.Username
	}
	if err := h.Eng.ResumeGateAs(c.Param("id"), c.Param("nodeId"), b.Action, b.Form, reviewer); err != nil {
		_ = c.Error(err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "resumed"})
}

type gateArtifactSaveBody struct {
	Content string `json:"content"`
}

// ListGatePrimaryArtifacts returns the editable primary products for a pending gate.
func (h *Handlers) ListGatePrimaryArtifacts(c *gin.Context) {
	items, err := h.Eng.ListGatePrimaryProducts(c.Param("id"), c.Param("nodeId"))
	if err != nil {
		_ = c.Error(err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if items == nil {
		c.JSON(http.StatusOK, gin.H{"items": []any{}})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

// SaveGateArtifact updates a gate-scoped primary artifact (waiting_human only).
// Optional If-Match header enables external-change detection (409 on mismatch).
func (h *Handlers) SaveGateArtifact(c *gin.Context) {
	var b gateArtifactSaveBody
	if err := c.ShouldBindJSON(&b); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	name := c.Param("name")
	ifMatch := strings.TrimSpace(c.GetHeader("If-Match"))
	res, err := h.Eng.SaveGateArtifact(c.Param("id"), c.Param("nodeId"), name, b.Content, ifMatch)
	if err != nil {
		_ = c.Error(err)
		if engine.IsArtifactConflict(err) {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.Header("ETag", res.ETag)
	c.JSON(http.StatusOK, gin.H{
		"id": res.ID, "name": res.Name, "kind": res.Kind, "sizeBytes": res.SizeBytes,
		"updatedAt": res.UpdatedAt, "etag": res.ETag, "nodeId": res.NodeID,
		"content": res.Content,
	})
}

// SaveAnnotationArtifact upserts the CommentPin package (preview_annotations.json).
// Independent of SaveGateArtifact whitelist and PreviewIssue lifecycle.
func (h *Handlers) SaveAnnotationArtifact(c *gin.Context) {
	var doc engine.AnnotationArtifactDoc
	if err := c.ShouldBindJSON(&doc); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	res, err := h.Eng.SaveAnnotationArtifact(c.Param("id"), c.Param("nodeId"), doc)
	if err != nil {
		_ = c.Error(err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.Header("ETag", res.ETag)
	c.JSON(http.StatusOK, gin.H{
		"id": res.ID, "name": res.Name, "kind": res.Kind, "sizeBytes": res.SizeBytes,
		"updatedAt": res.UpdatedAt, "etag": res.ETag, "nodeId": res.NodeID,
		"content": res.Content, "cleared": res.Cleared,
	})
}

type reactReplyBody struct {
	Text   string               `json:"text"`
	Images []models.PromptImage `json:"images"`
	// Annotations are precise field/element references (JSON path or DOM
	// selector + note) the human attached to this review turn.
	Annotations []models.ReactAnnotation `json:"annotations"`
	// Force finishes the clarification/review early: the agent is asked to wrap
	// up and the node completes regardless of any further questions. For a
	// review node force=true is "确认并流转"; force=false is one in-place edit.
	Force bool `json:"force"`
}

func (h *Handlers) ReactReply(c *gin.Context) {
	var b reactReplyBody
	if err := c.ShouldBindJSON(&b); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	runID, nodeID := c.Param("id"), c.Param("nodeId")
	if err := h.Eng.ReactReply(runID, nodeID, b.Text, b.Images, b.Annotations, b.Force); err != nil {
		_ = c.Error(err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if !b.Force {
		if w, thinking := h.Eng.ReviewSessionState(runID, nodeID); thinking || w > 0 {
			c.JSON(http.StatusOK, gin.H{"status": "accepted", "waiting": w})
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// ReactCancel aborts the current turn. Review clears the pending FIFO (#77);
// classic clarify keeps the queue and lets the pump start the next item (Demo).
func (h *Handlers) ReactCancel(c *gin.Context) {
	runID, nodeID := c.Param("id"), c.Param("nodeId")
	clearQueue := true
	if run, ok := h.Runs.Get(runID); ok {
		if n := run.Graph.FindNode(nodeID); n != nil && nodereg.ClarifyInteractive(n.Type) {
			clearQueue = false
		}
	}
	var err error
	if clearQueue {
		err = h.Eng.CancelReviewSession(runID, nodeID)
	} else {
		err = h.Eng.CancelClarifyTurn(runID, nodeID)
	}
	if err != nil {
		_ = c.Error(err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

type gateReactReviseBody struct {
	Text        string                   `json:"text"`
	Images      []models.PromptImage     `json:"images"`
	Annotations []models.ReactAnnotation `json:"annotations"`
}

// GateReactRevise issues a ReAct reject-and-annotate against a pending approval
// gate: the annotation/text/images are sent to the upstream producer's still-
// alive sandbox session, which edits the product in place; the gate body is
// refreshed and stays pending for further rounds.
func (h *Handlers) GateReactRevise(c *gin.Context) {
	var b gateReactReviseBody
	if err := c.ShouldBindJSON(&b); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if strings.TrimSpace(b.Text) == "" && len(b.Images) == 0 && len(b.Annotations) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "text, images, or annotations required"})
		return
	}
	runID, gateNodeID := c.Param("id"), c.Param("nodeId")
	if err := h.Eng.GateReactRevise(runID, gateNodeID, b.Text, b.Images, b.Annotations); err != nil {
		_ = c.Error(err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	producerID, _ := h.Eng.GateReactInfo(runID, gateNodeID)
	waiting, _ := h.Eng.ReviewSessionState(runID, producerID)
	c.JSON(http.StatusOK, gin.H{"status": "accepted", "waiting": waiting, "producerNodeId": producerID})
}

// GateReactCancel cancels the upstream producer's review turn/queue from a gate.
func (h *Handlers) GateReactCancel(c *gin.Context) {
	runID, gateNodeID := c.Param("id"), c.Param("nodeId")
	producerID, alive := h.Eng.GateReactInfo(runID, gateNodeID)
	if producerID == "" || !alive {
		c.JSON(http.StatusBadRequest, gin.H{"error": "上游复审会话不可用"})
		return
	}
	if err := h.Eng.CancelReviewSession(runID, producerID); err != nil {
		_ = c.Error(err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok", "producerNodeId": producerID})
}

func (h *Handlers) ListGates(c *gin.Context) {
	wf := c.Query("wf")
	projectID := c.Query("projectId")
	tags := parseRunTags(append(c.QueryArray("tag"), c.Query("tag"))...)
	pg, ok := parsePagination(c)
	if !ok {
		return
	}
	if !pg.Active {
		items, _ := h.Runs.PendingInboxItems(wf, projectID, tags, 0, 0)
		if h.GateShare != nil {
			h.GateShare.AttachInboxStatus(items)
		}
		c.JSON(http.StatusOK, items)
		return
	}
	offset := (pg.Page - 1) * pg.PageSize
	items, total := h.Runs.PendingInboxItems(wf, projectID, tags, offset, pg.PageSize)
	if h.GateShare != nil {
		h.GateShare.AttachInboxStatus(items)
	}
	c.JSON(http.StatusOK, paginatedResponse(items, total, pg.Page, pg.PageSize))
}
