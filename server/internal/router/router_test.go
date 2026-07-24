package router

import (
	"net/http/httptest"
	"testing"

	"github.com/cocofhu/approving/internal/handlers"

	"github.com/gin-gonic/gin"
)

func TestNewBuildsAndCORS(t *testing.T) {
	gin.SetMode(gin.TestMode)
	// An empty Handlers is enough to register routes; we only hit endpoints
	// that don't dereference nil service fields (health, CORS preflight).
	r := New(&handlers.Handlers{})
	if r == nil {
		t.Fatal("New returned nil")
	}

	// Health works without any services.
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/api/health", nil))
	if w.Code != 200 {
		t.Fatalf("health: %d", w.Code)
	}

	// CORS preflight is short-circuited with 204.
	w = httptest.NewRecorder()
	req := httptest.NewRequest("OPTIONS", "/api/health", nil)
	r.ServeHTTP(w, req)
	if w.Code != 204 {
		t.Fatalf("OPTIONS preflight = %d, want 204", w.Code)
	}
	if w.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Fatal("missing CORS header")
	}

	// SPA fallback for an unknown API path returns 404.
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/api/nope", nil))
	if w.Code != 404 {
		t.Fatalf("api 404 fallback = %d", w.Code)
	}
}
