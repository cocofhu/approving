package handlers

import (
	"context"
	"net/http"

	"github.com/cocofhu/approving/internal/models"
	"github.com/cocofhu/approving/internal/services"
	"github.com/gin-gonic/gin"
)

// OriginAnnouncer tells a run's origin conversation that it is being detached
// or reconnected, and reports whether the notice actually went out. Satisfied
// by channels.Manager; nil when no IM channel is configured, in which case the
// binding still changes and nobody needs telling.
type OriginAnnouncer interface {
	AnnounceOriginBinding(ctx context.Context, projectID, runID string, bound bool) bool
}

type runOriginBindingBody struct {
	// Bound is required rather than a toggle: a toggle sent twice by a
	// double-click would silently undo itself.
	Bound *bool `json:"bound"`
}

// PatchRunOriginBinding detaches a run from the conversation that asked for it,
// or reconnects it.
//
// The order of the two writes matters and is the whole subtlety here. On
// detach the goodbye goes out first, because afterwards the delivery guard
// would swallow it and the requester would be left waiting on updates that are
// never coming. On reconnect the mark is cleared first, for the mirror-image
// reason.
func (h *Handlers) PatchRunOriginBinding(c *gin.Context) {
	if h.TaskContext == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "task context unavailable"})
		return
	}
	runID := c.Param("id")
	run, ok := h.Runs.Get(runID)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	var body runOriginBindingBody
	if err := c.ShouldBindJSON(&body); err != nil || body.Bound == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bound is required"})
		return
	}
	bound := *body.Bound

	projectID := ""
	if h.WF != nil {
		if wf, wfOK := h.WF.Get(run.WorkflowID); wfOK {
			projectID = wf.ProjectID
		}
	}
	if projectID == "" {
		c.JSON(http.StatusConflict, gin.H{"error": "run has no project"})
		return
	}

	current, err := h.TaskContext.IdentityForRun(runID, projectID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if current == nil || current.OriginConversationID == "" {
		// Nothing to unbind: this run was started from the web, so it never
		// reported to a conversation in the first place.
		c.JSON(http.StatusConflict, gin.H{"error": "run has no origin conversation"})
		return
	}
	if (current.OriginUnboundAt == nil) == bound {
		c.JSON(http.StatusOK, gin.H{"origin": h.runOriginResponse(runID), "noticeDelivered": false})
		return
	}

	noticed := false
	if bound {
		if _, err := h.TaskContext.SetOriginBinding(projectID, runID, true); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		noticed = h.announceOriginBinding(c.Request.Context(), projectID, runID, true)
	} else {
		noticed = h.announceOriginBinding(c.Request.Context(), projectID, runID, false)
		if _, err := h.TaskContext.SetOriginBinding(projectID, runID, false); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	summary := "unbind run from origin conversation"
	if bound {
		summary = "rebind run to origin conversation"
	}
	h.recordAudit(services.AuditRecord{
		ProjectID:    projectID,
		Actor:        h.auditActorFromContext(c),
		Action:       models.AuditActionRunOriginBinding,
		ResourceType: "run",
		ResourceID:   runID,
		RunID:        runID,
		Outcome:      models.AuditOutcomeOK,
		Summary:      summary,
		Payload: map[string]any{
			"runId": runID, "bound": bound,
			"conversationId": current.OriginConversationID,
			// Whether the person on the other end was actually told. A detach
			// they never heard about is the case worth being able to find later.
			"noticeDelivered": noticed,
		},
	})
	c.JSON(http.StatusOK, gin.H{"origin": h.runOriginResponse(runID), "noticeDelivered": noticed})
}

func (h *Handlers) announceOriginBinding(ctx context.Context, projectID, runID string, bound bool) bool {
	if h.OriginAnnouncer == nil {
		return false
	}
	return h.OriginAnnouncer.AnnounceOriginBinding(ctx, projectID, runID, bound)
}

func (h *Handlers) runOriginResponse(runID string) gin.H {
	origin, ok := h.Runs.RunOriginFor(runID)
	if !ok {
		return nil
	}
	return runOriginDTO(origin)
}
