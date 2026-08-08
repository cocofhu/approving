package apierr

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestInternalDoesNotLeakErrText(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/x", func(c *gin.Context) {
		Internal(c, errors.New("secret dial tcp 10.0.0.1:5432: connection refused"))
	})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x", nil))
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", w.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["error_code"] != CodeInternal {
		t.Fatalf("error_code = %v", body["error_code"])
	}
	if body["message"] != PublicInternalMessage || body["error"] != PublicInternalMessage {
		t.Fatalf("public fields = %#v", body)
	}
	raw := w.Body.String()
	if strings.Contains(raw, "10.0.0.1") || strings.Contains(raw, "connection refused") || strings.Contains(raw, "secret dial") {
		t.Fatalf("leaked internal err in body: %s", raw)
	}
}
