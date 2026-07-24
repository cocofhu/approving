package handlers_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cocofhu/approving/internal/config"
	"github.com/cocofhu/approving/internal/services"
)

func enableAdmin(t *testing.T) {
	t.Helper()
	cfg := config.GetConfig()
	if cfg == nil {
		t.Fatal("no config")
	}
	users := make([]config.AuthUser, len(cfg.Auth.Users))
	copy(users, cfg.Auth.Users)
	for i := range users {
		if users[i].Username == "admin" {
			users[i].IsAdmin = true
		}
	}
	cfg.Auth.Users = users
	config.StoreConfig(cfg)
}

func seedAgent(t *testing.T, hn *harness, name string) {
	t.Helper()
	if err := hn.h.Skill.Save(services.Agent{
		Name:  name,
		Files: []services.AgentFile{{Path: "AGENTS.md", Content: "# hi"}},
	}); err != nil {
		t.Fatalf("save agent: %v", err)
	}
}

func TestPlatformRulesCRUD(t *testing.T) {
	hn := newHarness(t)
	enableAdmin(t)

	w := hn.do("GET", "/api/platform-rules", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("list: %d %s", w.Code, w.Body.String())
	}

	w = hn.do("GET", "/api/platform-rules/test.md", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("get: %d %s", w.Code, w.Body.String())
	}

	w = hn.do("GET", "/api/platform-rules/test.md/embed", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("embed: %d %s", w.Code, w.Body.String())
	}

	w = hn.do("PUT", "/api/platform-rules/test.md", map[string]string{"content": "# custom"})
	if w.Code != http.StatusOK {
		t.Fatalf("save: %d %s", w.Code, w.Body.String())
	}

	w = hn.do("PUT", "/api/platform-rules/test.md", "not-json")
	if w.Code != http.StatusBadRequest {
		// do() always marshals body as JSON; use raw request for invalid body
		req := httptest.NewRequest(http.MethodPut, "/api/platform-rules/test.md", bytes.NewReader([]byte(`{`)))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: "cf_session", Value: hn.cookie})
		w = httptest.NewRecorder()
		hn.r.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("invalid body: %d", w.Code)
		}
	}

	w = hn.do("POST", "/api/platform-rules/test.md/reset", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("reset: %d %s", w.Code, w.Body.String())
	}

	w = hn.do("DELETE", "/api/platform-rules/test.md", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("delete: %d %s", w.Code, w.Body.String())
	}
}

func TestPlatformRulesRequireAdmin(t *testing.T) {
	hn := newHarness(t)
	// admin user without IsAdmin
	w := hn.do("PUT", "/api/platform-rules/test.md", map[string]string{"content": "x"})
	if w.Code != http.StatusForbidden {
		t.Fatalf("want 403 got %d %s", w.Code, w.Body.String())
	}
	w = hn.do("DELETE", "/api/platform-rules/test.md", nil)
	if w.Code != http.StatusForbidden {
		t.Fatalf("delete want 403 got %d", w.Code)
	}
	w = hn.do("POST", "/api/platform-rules/test.md/reset", nil)
	if w.Code != http.StatusForbidden {
		t.Fatalf("reset want 403 got %d", w.Code)
	}
}

func TestAgentPlatformRules(t *testing.T) {
	hn := newHarness(t)
	seedAgent(t, hn, "RulesAgent")

	w := hn.do("GET", "/api/agents/missing/platform-rules", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("missing agent list: %d", w.Code)
	}

	w = hn.do("GET", "/api/agents/RulesAgent/platform-rules", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("list: %d %s", w.Code, w.Body.String())
	}

	w = hn.do("GET", "/api/agents/RulesAgent/platform-rules/test.md", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("get: %d %s", w.Code, w.Body.String())
	}

	w = hn.do("PUT", "/api/agents/RulesAgent/platform-rules/test.md", map[string]string{"content": "# agent"})
	if w.Code != http.StatusOK {
		t.Fatalf("save: %d %s", w.Code, w.Body.String())
	}

	req := httptest.NewRequest(http.MethodPut, "/api/agents/RulesAgent/platform-rules/test.md", bytes.NewReader([]byte(`{`)))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "cf_session", Value: hn.cookie})
	w = httptest.NewRecorder()
	hn.r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("invalid body: %d", w.Code)
	}

	w = hn.do("DELETE", "/api/agents/RulesAgent/platform-rules/test.md", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("delete: %d %s", w.Code, w.Body.String())
	}

	w = hn.do("GET", "/api/agents/missing/platform-rules/test.md", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("missing get: %d", w.Code)
	}
	w = hn.do("PUT", "/api/agents/missing/platform-rules/test.md", map[string]string{"content": "x"})
	if w.Code != http.StatusNotFound {
		t.Fatalf("missing save: %d", w.Code)
	}
	w = hn.do("DELETE", "/api/agents/missing/platform-rules/test.md", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("missing delete: %d", w.Code)
	}
}

func TestPlatformRulesBadFile(t *testing.T) {
	hn := newHarness(t)
	enableAdmin(t)
	w := hn.do("GET", "/api/platform-rules/nope.md", nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("bad get: %d %s", w.Code, w.Body.String())
	}
	w = hn.do("GET", "/api/platform-rules/nope.md/embed", nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("bad embed: %d", w.Code)
	}
	w = hn.do("PUT", "/api/platform-rules/nope.md", map[string]string{"content": "x"})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("bad save: %d %s", w.Code, w.Body.String())
	}
	w = hn.do("DELETE", "/api/platform-rules/nope.md", nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("bad delete: %d", w.Code)
	}
	w = hn.do("POST", "/api/platform-rules/nope.md/reset", nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("bad reset: %d", w.Code)
	}
	_ = json.Valid([]byte(`{}`))
}
