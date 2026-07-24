package handlers_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/cocofhu/approving/internal/models"
)

func TestDeletePreviewIssueNilService(t *testing.T) {
	hn := newHarness(t)
	hn.h.Issues = nil
	w := hn.do("DELETE", "/api/runs/r/nodes/n/preview-issues/i1", nil)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("nil issues: %d %s", w.Code, w.Body.String())
	}
}

func TestPreviewIssueHandlers(t *testing.T) {
	hn := newHarness(t)
	run := models.Run{ID: "run-iss", WorkflowID: "wf", WorkflowName: "W", Status: "running"}
	if err := hn.db.Create(&run).Error; err != nil {
		t.Fatal(err)
	}

	// Create with neither body nor images → 400.
	w := hn.do(http.MethodPost, "/api/runs/run-iss/nodes/preview/preview-issues", map[string]any{})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("empty create: %d %s", w.Code, w.Body.String())
	}

	w = hn.do(http.MethodPost, "/api/runs/run-iss/nodes/preview/preview-issues", map[string]any{
		"body": "layout broken", "selector": ".hero", "port": 3000,
	})
	if w.Code != http.StatusOK {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	var created models.PreviewIssue
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil || created.ID == "" {
		t.Fatalf("create body: %v %s", err, w.Body.String())
	}

	w = hn.do(http.MethodGet, "/api/runs/run-iss/nodes/preview/preview-issues", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("list: %d %s", w.Code, w.Body.String())
	}
	var listed struct {
		Issues []models.PreviewIssue `json:"issues"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &listed); err != nil || len(listed.Issues) != 1 {
		t.Fatalf("list body: %v %s", err, w.Body.String())
	}

	w = hn.do(http.MethodDelete, "/api/runs/run-iss/nodes/preview/preview-issues/"+created.ID, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("delete: %d %s", w.Code, w.Body.String())
	}
	w = hn.do(http.MethodGet, "/api/runs/run-iss/nodes/preview/preview-issues", nil)
	_ = json.Unmarshal(w.Body.Bytes(), &listed)
	if len(listed.Issues) != 0 {
		t.Fatalf("expected empty after delete, got %d", len(listed.Issues))
	}
}
