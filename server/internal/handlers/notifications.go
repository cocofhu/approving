package handlers

import (
	"errors"
	"net/http"
	"strconv"
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

// ListNotifications returns a paginated slice of terminal-run inbox rows plus true totals.
func (h *Handlers) ListNotifications(c *gin.Context) {
	if h.Notifications == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "notifications unavailable"})
		return
	}
	username, ok := h.notificationUsername(c)
	if !ok {
		return
	}

	page := 1
	pageSize := services.DefaultNotificationPageSize
	filter := strings.TrimSpace(strings.ToLower(c.Query("filter")))
	if filter == "" {
		filter = "all"
	}
	if v := strings.TrimSpace(c.Query("page")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			page = n
		}
	}
	if v := strings.TrimSpace(c.Query("pageSize")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			pageSize = n
		}
	}

	result, err := h.Notifications.ListPage(username, filter, page, pageSize)
	if err != nil {
		if errors.Is(err, services.ErrNotificationUsernameRequired) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if result.Items == nil {
		result.Items = []services.NotificationItemDTO{}
	}
	c.JSON(http.StatusOK, result)
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

// MarkAllNotificationsRead marks every unread terminal run as read for the user.
// The client must not send an id list; the server scans the inbox itself.
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
