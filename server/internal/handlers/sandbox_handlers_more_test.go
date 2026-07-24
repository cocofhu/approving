package handlers_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/cocofhu/approving/internal/models"
)

func TestRunNodeSandboxHandler(t *testing.T) {
	hn := newHarness(t)
	w := hn.do("GET", "/api/runs/r1/nodes/n1/sandbox", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("missing: %d %s", w.Code, w.Body.String())
	}

	hn.fg.Seed("run-sb")
	row := models.Sandbox{
		Name: "run-sb", RunID: "r1", NodeID: "n1", Purpose: "run", Status: "running",
		Profile: "agent", CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if err := hn.db.Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	w = hn.do("GET", "/api/runs/r1/nodes/n1/sandbox", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("found: %d %s", w.Code, w.Body.String())
	}
}

func TestListGetSandboxHandlers(t *testing.T) {
	hn := newHarness(t)
	w := hn.do("GET", "/api/sandboxes", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("list: %d", w.Code)
	}
	w = hn.do("GET", "/api/sandboxes/abc", nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("bad id: %d", w.Code)
	}
	w = hn.do("GET", "/api/sandboxes/99", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("missing: %d", w.Code)
	}
	w = hn.do("POST", "/api/sandboxes/99/stop", nil)
	if w.Code != http.StatusBadRequest && w.Code != http.StatusConflict && w.Code != http.StatusNotFound {
		// Stop on missing may conflict/notfound depending on impl
		if w.Code == http.StatusOK {
			t.Fatal("unexpected ok")
		}
	}
}
