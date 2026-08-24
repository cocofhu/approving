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

type markAllNotificationReadBody struct {
	RunIDs []string `json:"runIds"`
}

func (h *Handlers) notificationPrefsUsername(c *gin.Context) (string, bool) {
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

// GetNotificationReadPrefs returns the current user's prefs (creates baseline on first access).
func (h *Handlers) GetNotificationReadPrefs(c *gin.Context) {
	if h.NotificationReadPrefs == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "notification prefs unavailable"})
		return
	}
	username, ok := h.notificationPrefsUsername(c)
	if !ok {
		return
	}
	prefs, err := h.NotificationReadPrefs.GetOrInit(username)
	if err != nil {
		if errors.Is(err, services.ErrNotificationPrefsUsernameRequired) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, prefs)
}

// MarkNotificationRead marks one runId as read for the current user.
func (h *Handlers) MarkNotificationRead(c *gin.Context) {
	if h.NotificationReadPrefs == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "notification prefs unavailable"})
		return
	}
	username, ok := h.notificationPrefsUsername(c)
	if !ok {
		return
	}
	var body markNotificationReadBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	prefs, err := h.NotificationReadPrefs.MarkRead(username, body.RunID)
	if err != nil {
		if errors.Is(err, services.ErrNotificationPrefsRunIDRequired) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if errors.Is(err, services.ErrNotificationPrefsUsernameRequired) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, prefs)
}

// MarkAllNotificationsRead unions the given runIds into the current user's read set.
func (h *Handlers) MarkAllNotificationsRead(c *gin.Context) {
	if h.NotificationReadPrefs == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "notification prefs unavailable"})
		return
	}
	username, ok := h.notificationPrefsUsername(c)
	if !ok {
		return
	}
	var body markAllNotificationReadBody
	if err := c.ShouldBindJSON(&body); err != nil {
		// Empty body → treat as empty runIds (still ensures baseline exists).
		body.RunIDs = nil
	}
	prefs, err := h.NotificationReadPrefs.MarkAllRead(username, body.RunIDs)
	if err != nil {
		if errors.Is(err, services.ErrNotificationPrefsUsernameRequired) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, prefs)
}
