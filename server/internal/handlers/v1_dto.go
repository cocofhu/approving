package handlers

import (
	"github.com/cocofhu/approving/internal/models"

	"github.com/gin-gonic/gin"
)

// v1RunDTO shapes a run for external /v1 API (snake_case, minimal fields).
func v1RunDTO(r models.Run, currentNode string, errMsg string) gin.H {
	out := gin.H{
		"run_id":  r.ID,
		"status":  r.Status,
		"trigger": r.Trigger,
	}
	if currentNode != "" {
		out["current_node"] = currentNode
	}
	if errMsg != "" {
		out["error"] = errMsg
	}
	return out
}

// v1StartRunDTO shapes the POST /v1/workflows/{id}/runs response.
func v1StartRunDTO(r models.Run) gin.H {
	return gin.H{"run_id": r.ID, "status": r.Status}
}

// v1ArtifactDTO shapes one artifact for external /v1 API.
func v1ArtifactDTO(a models.Artifact) gin.H {
	return gin.H{
		"artifact_id": a.ID,
		"name":        a.Name,
		"kind":        a.Kind,
		"size_bytes":  a.SizeBytes,
	}
}

// apiKeyDTO shapes a workflow API key for list responses (masked).
func apiKeyDTO(k models.WorkflowAPIKey) gin.H {
	return gin.H{
		"id":         k.ID,
		"name":       k.Name,
		"key_prefix": k.KeyPrefix,
		"created_at": k.CreatedAt,
	}
}
