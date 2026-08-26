package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/cocofhu/approving/internal/models"
	"github.com/cocofhu/approving/internal/services"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupNotificationHandler(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "h.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(
		&models.NotificationRead{},
		&models.NotificationBaseline{},
		&models.Run{},
		&models.Session{},
	); err != nil {
		t.Fatal(err)
	}
	svc := services.NewNotificationService(db, nil)
	h := &Handlers{Notifications: svc}
	r := gin.New()
	r.GET("/api/notifications", func(c *gin.Context) {
		c.Set("auth_session", &models.Session{Username: c.GetHeader("X-Test-User")})
		h.ListNotifications(c)
	})
	r.POST("/api/notifications/read", func(c *gin.Context) {
		c.Set("auth_session", &models.Session{Username: c.GetHeader("X-Test-User")})
		h.MarkNotificationRead(c)
	})
	r.POST("/api/notifications/read-all", func(c *gin.Context) {
		c.Set("auth_session", &models.Session{Username: c.GetHeader("X-Test-User")})
		h.MarkAllNotificationsRead(c)
	})
	return r, db
}

func TestNotificationHandlersIsolationAndMarkAll(t *testing.T) {
	r, db := setupNotificationHandler(t)
	start := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	if err := db.Create(&models.Run{
		ID: "r1", WorkflowID: "wf", WorkflowName: "demo", Title: "one",
		Status: "completed", StartedAt: start, DurationSec: 1, Progress: 100,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.Run{
		ID: "r2", WorkflowID: "wf", WorkflowName: "demo", Title: "two",
		Status: "failed", StartedAt: start.Add(time.Minute), DurationSec: 1, Progress: 100,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.NotificationBaseline{
		Username:  "alice",
		EnabledAt: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
		CreatedAt: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
	}).Error; err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/notifications/read", bytes.NewBufferString(`{"runId":"r1"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Test-User", "alice")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("alice mark: %d %s", w.Code, w.Body.String())
	}

	req2 := httptest.NewRequest(http.MethodGet, "/api/notifications", nil)
	req2.Header.Set("X-Test-User", "bob")
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("bob get: %d %s", w2.Code, w2.Body.String())
	}
	var bob struct {
		Items []services.NotificationItemDTO `json:"items"`
	}
	if err := json.Unmarshal(w2.Body.Bytes(), &bob); err != nil {
		t.Fatal(err)
	}
	if len(bob.Items) == 0 {
		t.Fatal("bob empty list")
	}
	for _, it := range bob.Items {
		if it.Unread {
			t.Fatalf("bob first GET must be history: %+v", it)
		}
	}

	req3 := httptest.NewRequest(http.MethodGet, "/api/notifications", nil)
	req3.Header.Set("X-Test-User", "alice")
	w3 := httptest.NewRecorder()
	r.ServeHTTP(w3, req3)
	var alice struct {
		Items []services.NotificationItemDTO `json:"items"`
	}
	if err := json.Unmarshal(w3.Body.Bytes(), &alice); err != nil {
		t.Fatal(err)
	}
	var aUnread, bUnread bool
	for _, it := range alice.Items {
		if it.RunID == "r1" {
			aUnread = it.Unread
		}
		if it.RunID == "r2" {
			bUnread = it.Unread
		}
	}
	if aUnread || !bUnread {
		t.Fatalf("alice=%+v", alice.Items)
	}

	req4 := httptest.NewRequest(http.MethodPost, "/api/notifications/read-all", nil)
	req4.Header.Set("X-Test-User", "alice")
	w4 := httptest.NewRecorder()
	r.ServeHTTP(w4, req4)
	if w4.Code != http.StatusOK {
		t.Fatalf("mark-all: %d %s", w4.Code, w4.Body.String())
	}
	req5 := httptest.NewRequest(http.MethodGet, "/api/notifications", nil)
	req5.Header.Set("X-Test-User", "alice")
	w5 := httptest.NewRecorder()
	r.ServeHTTP(w5, req5)
	if err := json.Unmarshal(w5.Body.Bytes(), &alice); err != nil {
		t.Fatal(err)
	}
	for _, it := range alice.Items {
		if it.Unread {
			t.Fatalf("after mark-all still unread: %+v", it)
		}
	}
}
