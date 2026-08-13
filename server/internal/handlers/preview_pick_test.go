package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestPreviewPickScript(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := &Handlers{}
	r.GET("/preview-pick.js", h.PreviewPickScript)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/preview-pick.js", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d", w.Code)
	}
	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "javascript") {
		t.Fatalf("content-type=%q", ct)
	}
	if w.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Fatalf("cors=%q", w.Header().Get("Access-Control-Allow-Origin"))
	}
	if w.Header().Get("Cross-Origin-Resource-Policy") != "cross-origin" {
		t.Fatalf("corp=%q", w.Header().Get("Cross-Origin-Resource-Policy"))
	}
	body := w.Body.String()
	for _, needle := range []string{
		"direct-preview-ready",
		"direct-preview-url",
		"direct-preview-picked",
		"direct-preview-canceled",
		"direct-preview-inspect",
		"direct-preview-nav",
	} {
		if !strings.Contains(body, needle) {
			t.Fatalf("script missing %q", needle)
		}
	}
}
