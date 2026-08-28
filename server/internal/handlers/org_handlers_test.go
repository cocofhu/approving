package handlers_test

import (
	"net/http"
	"testing"

	"github.com/cocofhu/approving/internal/services"
)

func TestGetAgentsOrgEmptyWhenNilService(t *testing.T) {
	hn := newHarness(t)
	hn.h.Org = nil
	w := hn.do(http.MethodGet, "/api/agents/org", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("get org: %d %s", w.Code, w.Body.String())
	}
}

func TestPutAgentsOrgRoundTrip(t *testing.T) {
	hn := newHarness(t)
	enableAdmin(t)
	root := t.TempDir()
	skills := services.NewAgentService(root)
	hn.h.Org = services.NewOrgService(root, skills)
	w := hn.do(http.MethodPut, "/api/agents/org", map[string]any{
		"revision": 0,
		"groups":   []map[string]any{{"id": "g1", "name": "Group"}},
		"agents":   map[string]any{},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("put org: %d %s", w.Code, w.Body.String())
	}
	w = hn.do(http.MethodGet, "/api/agents/org", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("get org: %d", w.Code)
	}
}

func TestPutAgentsOrgUnavailable(t *testing.T) {
	hn := newHarness(t)
	hn.h.Org = nil
	w := hn.do(http.MethodPut, "/api/agents/org", map[string]any{"revision": 0})
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("put nil org: %d", w.Code)
	}
}
