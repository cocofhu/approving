package handler

import (
	"net/http"

	"backend/internal/qqbot"

	"github.com/gin-gonic/gin"
)

func QQConfig(bot *qqbot.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		if bot == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "qqbot not initialized"})
			return
		}
		c.JSON(http.StatusOK, bot.PublicConfigWithStatus())
	}
}

func QQConfigUpdate(bot *qqbot.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		if bot == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "qqbot not initialized"})
			return
		}
		var cfg qqbot.Config
		if err := c.ShouldBindJSON(&cfg); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if err := bot.UpdateConfig(cfg); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, bot.PublicConfigWithStatus())
	}
}
