package handler

import (
	"net/http"
	"strings"

	"backend/internal/service"

	"github.com/gin-gonic/gin"
)

// ModelsGET 返回可用模型列表与当前选中模型。
func ModelsGET(bridge *service.Bridge) gin.HandlerFunc {
	return func(c *gin.Context) {
		models, err := service.ListAgentModels()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		current := bridge.Model()
		c.JSON(http.StatusOK, gin.H{
			"models":  models,
			"current": current,
			"fixed":   bridge.ModelFixed(),
		})
	}
}

// ModelPOST 设置模型并重启 Agent。
func ModelPOST(bridge *service.Bridge) gin.HandlerFunc {
	return func(c *gin.Context) {
		if bridge.ModelFixed() {
			c.JSON(http.StatusForbidden, gin.H{"error": "模型已由启动参数锁定，不可切换"})
			return
		}
		var body struct {
			Model string `json:"model"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON"})
			return
		}
		model := strings.TrimSpace(body.Model)
		prev := bridge.Model()
		bridge.SetModel(model, false)
		sess, err := bridge.RestartAgent()
		if err != nil {
			bridge.SetModel(prev, false)
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		bridge.Broadcast(bridge.ConnectedPayload(sess))
		bridge.BroadcastQueueState()
		c.JSON(http.StatusOK, gin.H{
			"model":     bridge.Model(),
			"sessionId": sess.SessionID(),
		})
	}
}
