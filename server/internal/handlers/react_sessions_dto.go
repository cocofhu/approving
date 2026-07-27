package handlers

import (
	"github.com/cocofhu/approving/internal/engine"

	"github.com/gin-gonic/gin"
)

// reactSessionsDTO maps engine review/clarify session snapshots to the
// per-node gin.H used by runDetailDTO and clarify inbox-context (refresh-resume).
func reactSessionsDTO(snaps []engine.ReviewSessionSnapshot) gin.H {
	if len(snaps) == 0 {
		return nil
	}
	byNode := gin.H{}
	for _, s := range snaps {
		byNode[s.NodeID] = gin.H{
			"kind": s.Kind, "waiting": s.Waiting, "busy": s.Busy,
			"items": s.Items, "activeItem": s.ActiveItem,
		}
	}
	return byNode
}
