package handler

import (
	"net/http"

	"backend/internal/service"

	"github.com/gin-gonic/gin"
)

// PromptQueue 只读队列快照（queue_* 等字段）。
func PromptQueue(bridge *service.Bridge) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, bridge.PromptQueueSnapshot())
	}
}
