package handlers_test

import (
	"encoding/json"
	"testing"

	"github.com/cocofhu/approving/internal/models"
)

// TestSaveWorkflowStatusGuard covers f6/f7: after publish, PUT with no graph
// diff keeps published (even when client always sends draft intent); graph
// change downgrades; metadata-only updates land without downgrade. Also
// exercises LiftInputVariables so variables in input.config do not look like
// a spurious graph diff vs DB Graph.Variables.
func TestSaveWorkflowStatusGuard(t *testing.T) {
	h := newHarness(t)

	body := map[string]any{
		"name": "Guard", "projectId": models.DefaultProjectID, "description": "",
		"nodes": []map[string]any{
			{
				"id": "in", "type": "input", "label": "Start",
				"position": map[string]any{"x": 0, "y": 0},
				"config": map[string]any{
					"variables": []map[string]any{
						{"name": "repo", "type": "string", "value": "a"},
					},
				},
			},
			{"id": "out", "type": "output", "label": "End", "position": map[string]any{"x": 1, "y": 0}, "config": map[string]any{}},
		},
		"edges": []map[string]any{{"id": "e", "source": "in", "target": "out"}},
	}
	w := h.do("POST", "/api/workflows", body)
	if w.Code != 200 {
		t.Fatalf("create: %d %s", w.Code, w.Body)
	}
	var created map[string]any
	json.Unmarshal(w.Body.Bytes(), &created)
	id, _ := created["id"].(string)
	if id == "" {
		t.Fatal("no id")
	}
	if created["status"] != "draft" {
		t.Fatalf("create status=%v", created["status"])
	}

	if w := h.do("POST", "/api/workflows/"+id+"/publish", nil); w.Code != 200 {
		t.Fatalf("publish: %d %s", w.Code, w.Body)
	}

	// Same graph (variables still in input.config as DTO shape) + rename.
	body["name"] = "Guard Renamed"
	body["id"] = id
	w = h.do("PUT", "/api/workflows/"+id, body)
	if w.Code != 200 {
		t.Fatalf("meta put: %d %s", w.Code, w.Body)
	}
	var afterMeta map[string]any
	json.Unmarshal(w.Body.Bytes(), &afterMeta)
	if afterMeta["status"] != "published" {
		t.Fatalf("meta-only PUT should stay published, got %v", afterMeta["status"])
	}
	if afterMeta["name"] != "Guard Renamed" {
		t.Fatalf("name not updated: %v", afterMeta["name"])
	}

	// Identical PUT (no meta/graph change) stays published.
	w = h.do("PUT", "/api/workflows/"+id, body)
	if w.Code != 200 {
		t.Fatalf("noop put: %d", w.Code)
	}
	var afterNoop map[string]any
	json.Unmarshal(w.Body.Bytes(), &afterNoop)
	if afterNoop["status"] != "published" {
		t.Fatalf("noop PUT should stay published, got %v", afterNoop["status"])
	}

	// Graph change → draft.
	nodes := body["nodes"].([]map[string]any)
	nodes[0]["label"] = "Start Edited"
	w = h.do("PUT", "/api/workflows/"+id, body)
	if w.Code != 200 {
		t.Fatalf("graph put: %d %s", w.Code, w.Body)
	}
	var afterGraph map[string]any
	json.Unmarshal(w.Body.Bytes(), &afterGraph)
	if afterGraph["status"] != "draft" {
		t.Fatalf("graph change should draft, got %v", afterGraph["status"])
	}
}
