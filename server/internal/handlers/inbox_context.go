package handlers

import (
	"net/http"
	"strconv"

	"github.com/cocofhu/approving/internal/services"

	"github.com/gin-gonic/gin"
)

// RunInboxContext returns gate or clarify approval context for a pending inbox
// item (GET /api/runs/:id/inbox-context?nodeId=&iteration=).
func (h *Handlers) RunInboxContext(c *gin.Context) {
	runID := c.Param("id")
	nodeID := c.Query("nodeId")
	iterStr := c.Query("iteration")
	if nodeID == "" || iterStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "nodeId and iteration are required"})
		return
	}
	iteration, err := strconv.Atoi(iterStr)
	if err != nil || iteration < 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "iteration must be a positive integer"})
		return
	}
	if _, ok := h.Runs.Get(runID); !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}

	kind, ok := h.Runs.InboxContextKind(runID, nodeID, iteration)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "no pending inbox item"})
		return
	}

	switch kind {
	case "gate":
		h.inboxContextGate(c, runID, nodeID, iteration)
	case "clarify":
		h.inboxContextClarify(c, runID, nodeID, iteration)
	default:
		c.JSON(http.StatusNotFound, gin.H{"error": "no pending inbox item"})
	}
}

func (h *Handlers) inboxContextGate(c *gin.Context, runID, gateNodeID string, iteration int) {
	run, ok := h.Runs.Get(runID)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	arts := h.Arts.ByRun(runID)
	artsDTO := make([]gin.H, 0, len(arts))
	for _, a := range arts {
		artsDTO = append(artsDTO, artifactMetaDTO(a))
	}

	gateNode := run.Graph.FindNode(gateNodeID)
	upstreamIDs := services.GateUpstreamNodeIDs(gateNode, arts)
	slimExecs := h.Runs.SlimNodeExecutions(runID, upstreamIDs)
	for _, execs := range slimExecs {
		for i := range execs {
			outs, _ := execs[i]["outputs"].(map[string]any)
			h.hydrateTestResultOutputs(outs, runID)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"type":           "gate",
		"nodes":          graphNodesDTO(run.Graph),
		"artifacts":      artsDTO,
		"nodeExecutions": slimExecs,
	})
}

func (h *Handlers) inboxContextClarify(c *gin.Context, runID, nodeID string, iteration int) {
	conv, run, ok := h.Runs.ClarifyContext(runID, nodeID, iteration)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	arts := h.Arts.ByRun(runID)
	artsDTO := make([]gin.H, 0, len(arts))
	for _, a := range arts {
		artsDTO = append(artsDTO, artifactMetaDTO(a))
	}

	clarifyNode := run.Graph.FindNode(nodeID)
	slimIDs := services.ClarifySlimNodeIDs(clarifyNode, nodeID, arts)
	slimExecs := h.Runs.SlimNodeExecutions(runID, slimIDs)
	for _, execs := range slimExecs {
		for i := range execs {
			outs, _ := execs[i]["outputs"].(map[string]any)
			h.hydrateTestResultOutputs(outs, runID)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"type":           "clarify",
		"status":         run.Status,
		"nodes":          graphNodesDTO(run.Graph),
		"artifacts":      artsDTO,
		"nodeExecutions": slimExecs,
		"clarify": gin.H{
			"nodeId":    conv.NodeID,
			"iteration": conv.Iteration,
			"turns":     conv.Messages,
			"done":      conv.Done,
			"label":     services.ClarifyLabel(run.Graph, conv.NodeID),
		},
	})
}
