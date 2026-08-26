package handlers

import (
	"errors"
	"net/http"
	"strings"

	"github.com/cocofhu/approving/internal/auth"
	"github.com/cocofhu/approving/internal/services"

	"github.com/gin-gonic/gin"
)

type markNotificationReadBody struct {
	RunID string `json:"runId"`
}

func (h *Handlers) notificationUsername(c *gin.Context) (string, bool) {
	sess, ok := auth.GetSession(c)
	if !ok || sess == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return "", false
	}
	username := strings.TrimSpace(sess.Username)
	if username == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return "", false
	}
	return username, true
}

// ListNotifications returns the current user's terminal-run inbox with unread flags.
func (h *Handlers) ListNotifications(c *gin.Context) {
	if h.Notifications == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "notifications unavailable"})
		return
	}
	username, ok := h.notificationUsername(c)
	if !ok {
		return
	}
	items, err := h.Notifications.List(username)
	if err != nil {
		if errors.Is(err, services.ErrNotificationUsernameRequired) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if items == nil {
		items = []services.NotificationItemDTO{}
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

// MarkNotificationRead inserts a read row for one runId.
func (h *Handlers) MarkNotificationRead(c *gin.Context) {
	if h.Notifications == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "notifications unavailable"})
		return
	}
	username, ok := h.notificationUsername(c)
	if !ok {
		return
	}
	var body markNotificationReadBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	if err := h.Notifications.MarkRead(username, body.RunID); err != nil {
		if errors.Is(err, services.ErrNotificationRunIDRequired) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if errors.Is(err, services.ErrNotificationUsernameRequired) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// MarkAllNotificationsRead marks every run in the current pool as read.
// The client must not send an id list; the server scans the pool itself.
func (h *Handlers) MarkAllNotificationsRead(c *gin.Context) {
	if h.Notifications == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "notifications unavailable"})
		return
	}
	username, ok := h.notificationUsername(c)
	if !ok {
		return
	}
	if err := h.Notifications.MarkAllRead(username); err != nil {
		if errors.Is(err, services.ErrNotificationUsernameRequired) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
