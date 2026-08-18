package handlers_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/cocofhu/approving/internal/version"
)

func TestHealthCommitOptional(t *testing.T) {
	h := newHarness(t)

	t.Run("valid revision returns 7-char commit and keeps probe fields", func(t *testing.T) {
		restore := version.Override("B01BB39abcdef0123456789")
		t.Cleanup(restore)

		w := h.do("GET", "/api/health", nil)
		if w.Code != http.StatusOK {
			t.Fatalf("health: %d %s", w.Code, w.Body.String())
		}
		var body map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body["status"] != "ok" {
			t.Fatalf("status=%v", body["status"])
		}
		if body["ready"] != true {
			t.Fatalf("ready=%v", body["ready"])
		}
		if _, ok := body["vnc_preview"]; !ok {
			t.Fatalf("missing vnc_preview: %v", body)
		}
		commit, _ := body["commit"].(string)
		if commit != "b01bb39" {
			t.Fatalf("commit=%q want b01bb39 (fixture, not repo HEAD)", commit)
		}
	})

	t.Run("empty revision omits commit", func(t *testing.T) {
		restore := version.Override("")
		t.Cleanup(restore)

		w := h.do("GET", "/api/health", nil)
		if w.Code != http.StatusOK {
			t.Fatalf("health: %d %s", w.Code, w.Body.String())
		}
		var body map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if _, ok := body["commit"]; ok {
			t.Fatalf("commit should be omitted when empty: %v", body)
		}
		if body["status"] != "ok" || body["ready"] != true {
			t.Fatalf("probe fields changed: %v", body)
		}
	})

	t.Run("invalid revision omits commit", func(t *testing.T) {
		restore := version.Override("not-a-sha")
		t.Cleanup(restore)

		w := h.do("GET", "/api/health", nil)
		if w.Code != http.StatusOK {
			t.Fatalf("health: %d %s", w.Code, w.Body.String())
		}
		var body map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if _, ok := body["commit"]; ok {
			t.Fatalf("commit should be omitted when invalid: %v", body)
		}
	})
}
