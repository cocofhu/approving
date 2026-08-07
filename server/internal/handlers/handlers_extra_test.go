package handlers_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/cocofhu/approving/internal/auth"
	"github.com/cocofhu/approving/internal/liveagent"
	"github.com/cocofhu/approving/internal/models"
	"github.com/cocofhu/approving/internal/services"
	"github.com/cocofhu/approving/internal/shutdown"
)

func TestSettingsAndLiveEndpoints(t *testing.T) {
	h := newHarness(t)
	h.h.Settings = services.NewSettingsService(h.db, h.h.Eng, h.h.Sbx)
	h.h.Shutdown = shutdown.New(600 * time.Second)

	if w := h.do("GET", "/api/live", nil); w.Code != 200 {
		t.Fatalf("live: %d", w.Code)
	}
	if w := h.do("GET", "/api/settings", nil); w.Code != 200 {
		t.Fatalf("get settings: %d %s", w.Code, w.Body)
	}
	if w := h.do("PUT", "/api/settings", map[string]int{
		services.KeyMaxConcurrentRuns: 3,
	}); w.Code != 200 {
		t.Fatalf("update settings: %d %s", w.Code, w.Body)
	}
	if w := h.do("PUT", "/api/settings", "bad"); w.Code != 400 {
		t.Fatalf("update settings bad body: %d", w.Code)
	}
	if w := h.do("PUT", "/api/settings", map[string]int{
		services.KeyMaxConcurrentRuns: 0,
	}); w.Code != 400 {
		t.Fatalf("update settings below min: %d", w.Code)
	}

	h.h.Shutdown.BeginDraining()
	if w := h.do("GET", "/api/health", nil); w.Code != 503 {
		t.Fatalf("health draining: %d %s", w.Code, w.Body)
	}
}

// The settings page has to be able to answer two different questions: can this
// endpoint work, and is it being used. Neither may fail loudly — an endpoint
// that is misconfigured is a finding, not a server error.
func TestConversationModelTestAndStatusEndpoints(t *testing.T) {
	h := newHarness(t)
	h.h.Settings = services.NewSettingsService(h.db, h.h.Eng, h.h.Sbx)

	// Without a client wired, the test is unavailable but status still answers,
	// because "not configured" is exactly what the page needs to render.
	if w := h.do("POST", "/api/settings/live/test", map[string]any{}); w.Code != 503 {
		t.Fatalf("probe without a client: %d %s", w.Code, w.Body)
	}
	w := h.do("GET", "/api/settings/live/status", nil)
	if w.Code != 200 {
		t.Fatalf("status without a client: %d %s", w.Code, w.Body)
	}
	var status struct {
		Configured bool `json:"configured"`
		Stats      struct {
			Calls int `json:"calls"`
		} `json:"stats"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if status.Configured || status.Stats.Calls != 0 {
		t.Fatalf("status = %+v", status)
	}

	h.h.LiveModel = liveagent.New()
	// An incomplete form is reported as a failed check rather than a 400: the
	// page shows why, in the same place every other result appears.
	w = h.do("POST", "/api/settings/live/test", map[string]any{
		services.KeyLiveBaseURL: "", services.KeyLiveModel: "",
	})
	if w.Code != 200 {
		t.Fatalf("probe with an empty form: %d %s", w.Code, w.Body)
	}
	var report struct {
		Configured bool `json:"configured"`
		OK         bool `json:"ok"`
		Checks     []struct {
			Name   string `json:"name"`
			OK     bool   `json:"ok"`
			Reason string `json:"reason"`
		} `json:"checks"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if report.Configured || report.OK {
		t.Fatalf("empty form reported as working: %+v", report)
	}
	if len(report.Checks) == 0 || report.Checks[0].Reason == "" {
		t.Fatalf("empty form gave no reason: %+v", report)
	}

	// An unreachable address is also a finding, not an error.
	w = h.do("POST", "/api/settings/live/test", map[string]any{
		services.KeyLiveBaseURL:        "http://127.0.0.1:1/v1",
		services.KeyLiveModel:          "m",
		services.KeyLiveTimeoutSeconds: 1,
	})
	if w.Code != 200 {
		t.Fatalf("probe of an unreachable endpoint: %d %s", w.Code, w.Body)
	}
	if err := json.Unmarshal(w.Body.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if !report.Configured || report.OK {
		t.Fatalf("unreachable endpoint reported as working: %+v", report)
	}
	// And the manual test must not show up as traffic, or the status panel stops
	// answering whether real messages are going through this layer.
	w = h.do("GET", "/api/settings/live/status", nil)
	if err := json.Unmarshal(w.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if status.Stats.Calls != 0 {
		t.Fatalf("a probe counted as traffic: %+v", status)
	}
}

func TestWorkflowImportCopyAndVersionGraph(t *testing.T) {
	h := newHarness(t)
	body := map[string]any{
		"schemaVersion": 1,
		"name":          "Import WF",
		"graph": map[string]any{
			"nodes": []map[string]any{
				{"id": "in", "type": "input", "label": "S"},
				{"id": "out", "type": "output", "label": "E"},
			},
			"edges": []map[string]any{{"id": "e", "source": "in", "target": "out"}},
		},
	}
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/workflows/import", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: h.cookie})
	w := httptest.NewRecorder()
	h.r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("import: %d %s", w.Code, w.Body)
	}
	var imported map[string]any
	json.Unmarshal(w.Body.Bytes(), &imported)
	id, _ := imported["id"].(string)
	if id == "" {
		t.Fatal("import missing id")
	}

	if w := h.do("POST", "/api/workflows/import", "bad"); w.Code != 400 {
		t.Fatalf("import bad: %d", w.Code)
	}
	if w := h.do("GET", "/api/workflows/"+id+"/copy-preview", nil); w.Code != 200 {
		t.Fatalf("copy preview: %d %s", w.Code, w.Body)
	}
	if w := h.do("GET", "/api/workflows/ghost/copy-preview", nil); w.Code != 404 {
		t.Fatalf("copy preview missing: %d", w.Code)
	}
	if w := h.do("POST", "/api/workflows/"+id+"/copy", map[string]string{"name": "Copy Of Import"}); w.Code != 201 {
		t.Fatalf("copy: %d %s", w.Code, w.Body)
	}
	if w := h.do("POST", "/api/workflows/"+id+"/copy", map[string]string{"name": ""}); w.Code != 400 {
		t.Fatalf("copy empty name: %d", w.Code)
	}
	if w := h.do("POST", "/api/workflows/ghost/copy", map[string]string{"name": "x"}); w.Code != 404 {
		t.Fatalf("copy missing: %d", w.Code)
	}

	if w := h.do("POST", "/api/workflows/"+id+"/publish", nil); w.Code != 200 {
		t.Fatalf("publish: %d", w.Code)
	}
	if w := h.do("GET", "/api/workflows/"+id+"/versions/2/graph", nil); w.Code != 200 {
		t.Fatalf("version graph: %d %s", w.Code, w.Body)
	}
	if w := h.do("GET", "/api/workflows/"+id+"/versions/bad/graph", nil); w.Code != 400 {
		t.Fatalf("version graph bad: %d", w.Code)
	}
	if w := h.do("GET", "/api/workflows/ghost/versions/1/graph", nil); w.Code != 404 {
		t.Fatalf("version graph missing wf: %d", w.Code)
	}
}

func TestResumeRunEndpoint(t *testing.T) {
	h := newHarness(t)
	now := time.Now()
	h.db.Create(&models.Run{
		ID: "r-failed", Status: "failed", StartedAt: now,
		Graph: models.Graph{Nodes: []models.Node{{ID: "in", Type: "input"}, {ID: "out", Type: "output"}}},
	})
	if w := h.do("POST", "/api/runs/r-failed/resume", map[string]string{"nodeId": "in"}); w.Code != 200 {
		t.Fatalf("resume: %d %s", w.Code, w.Body)
	}
	if w := h.do("POST", "/api/runs/ghost/resume", nil); w.Code != 404 {
		t.Fatalf("resume missing: %d", w.Code)
	}
	if w := h.do("POST", "/api/runs/r-failed/resume", "bad"); w.Code != 400 {
		t.Fatalf("resume bad body: %d", w.Code)
	}
}
