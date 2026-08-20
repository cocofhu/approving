package handlers_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/cocofhu/approving/internal/models"
)

func TestStartRunAcceptsTitleOverride(t *testing.T) {
	h := newHarness(t)
	reposJSON := `[{"name":"approving","url":"https://git.example/approving.git"}]`
	if err := h.h.WF.Save(&models.WorkflowDef{
		ID: "wf-title", ProjectID: models.DefaultProjectID, Name: "WF Title",
		Graph: models.Graph{
			Variables: []models.Variable{{Name: "repos", Type: "repos", Ask: true, Value: reposJSON}},
			Nodes:     []models.Node{{ID: "in", Type: "input"}, {ID: "out", Type: "output"}},
			Edges:     []models.Edge{{ID: "e1", Source: "in", Target: "out"}},
		},
	}); err != nil {
		t.Fatalf("save workflow: %v", err)
	}

	w := h.do(http.MethodPost, "/api/workflows/wf-title/runs", map[string]any{
		"trigger": "manual",
		"title":   "  用户第一句话  ",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var res map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	run, ok := h.h.Runs.Get(res["id"])
	if !ok {
		t.Fatal("run missing")
	}
	if run.Title != "用户第一句话" {
		t.Fatalf("title=%q want 用户第一句话", run.Title)
	}
}
