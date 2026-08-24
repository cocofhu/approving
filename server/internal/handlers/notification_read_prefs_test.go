package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/cocofhu/approving/internal/models"
	"github.com/cocofhu/approving/internal/services"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupNotificationPrefsHandler(t *testing.T) (*Handlers, *gin.Engine, *services.NotificationReadPrefsService) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "h.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.NotificationReadPrefs{}, &models.Session{}); err != nil {
		t.Fatal(err)
	}
	svc := services.NewNotificationReadPrefsService(db)
	h := &Handlers{NotificationReadPrefs: svc}
	r := gin.New()
	r.GET("/api/notifications/prefs", func(c *gin.Context) {
		c.Set("auth_session", &models.Session{Username: c.GetHeader("X-Test-User")})
		h.GetNotificationReadPrefs(c)
	})
	r.POST("/api/notifications/prefs/read", func(c *gin.Context) {
		c.Set("auth_session", &models.Session{Username: c.GetHeader("X-Test-User")})
		h.MarkNotificationRead(c)
	})
	r.POST("/api/notifications/prefs/read-all", func(c *gin.Context) {
		c.Set("auth_session", &models.Session{Username: c.GetHeader("X-Test-User")})
		h.MarkAllNotificationsRead(c)
	})
	return h, r, svc
}

func TestNotificationReadPrefsHandlersIsolation(t *testing.T) {
	_, r, _ := setupNotificationPrefsHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/api/notifications/prefs/read", bytes.NewBufferString(`{"runId":"r1"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Test-User", "alice")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("alice mark: %d %s", w.Code, w.Body.String())
	}

	req2 := httptest.NewRequest(http.MethodGet, "/api/notifications/prefs", nil)
	req2.Header.Set("X-Test-User", "bob")
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("bob get: %d %s", w2.Code, w2.Body.String())
	}
	var bob services.NotificationPrefsDTO
	if err := json.Unmarshal(w2.Body.Bytes(), &bob); err != nil {
		t.Fatal(err)
	}
	if len(bob.ReadIDs) != 0 {
		t.Fatalf("bob saw alice reads: %+v", bob)
	}

	req3 := httptest.NewRequest(http.MethodGet, "/api/notifications/prefs", nil)
	req3.Header.Set("X-Test-User", "alice")
	w3 := httptest.NewRecorder()
	r.ServeHTTP(w3, req3)
	var alice services.NotificationPrefsDTO
	if err := json.Unmarshal(w3.Body.Bytes(), &alice); err != nil {
		t.Fatal(err)
	}
	if len(alice.ReadIDs) != 1 || alice.ReadIDs[0] != "r1" {
		t.Fatalf("alice=%+v", alice)
	}
}
