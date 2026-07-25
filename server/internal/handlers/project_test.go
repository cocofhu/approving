package handlers_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/cocofhu/approving/internal/models"
	"github.com/cocofhu/approving/internal/services"
)

func TestProjectCRUDAndErrors(t *testing.T) {
	hn := newHarness(t)

	// List (includes default project from migrate)
	w := hn.do("GET", "/api/projects", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("list: %d %s", w.Code, w.Body.String())
	}

	// Create
	w = hn.do("POST", "/api/projects", map[string]any{
		"name":        "ProjA",
		"description": "desc",
		"sandboxEnv":  []map[string]any{{"key": "FOO", "value": "bar"}},
		"variables":   []map[string]any{{"name": "x", "type": "string", "value": "1"}},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	id := jsonField(w.Body.String(), "id")
	if id == "" {
		t.Fatalf("missing id: %s", w.Body.String())
	}

	// Get
	w = hn.do("GET", "/api/projects/"+id, nil)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "ProjA") {
		t.Fatalf("get: %d %s", w.Code, w.Body.String())
	}

	// Get missing
	w = hn.do("GET", "/api/projects/missing-id", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("get missing: %d", w.Code)
	}

	// Create empty name
	w = hn.do("POST", "/api/projects", map[string]any{"name": "  "})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("empty name: %d %s", w.Code, w.Body.String())
	}

	// Create duplicate name
	w = hn.do("POST", "/api/projects", map[string]any{"name": "ProjA"})
	if w.Code != http.StatusConflict {
		t.Fatalf("dup name: %d %s", w.Code, w.Body.String())
	}

	// Create bad JSON
	w = hn.do("POST", "/api/projects", "not-an-object")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("bad json create: %d", w.Code)
	}

	// Official ACP auth key accepted (forced secret, masked in response)
	w = hn.do("POST", "/api/projects", map[string]any{
		"name":       "AuthEnv",
		"sandboxEnv": []map[string]any{{"key": "CURSOR_API_KEY", "value": "x", "secret": true}},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("auth env: %d %s", w.Code, w.Body.String())
	}
	var authEnvProj models.Project
	if err := json.Unmarshal(w.Body.Bytes(), &authEnvProj); err != nil {
		t.Fatalf("auth env decode: %v", err)
	}
	if len(authEnvProj.SandboxEnv) != 1 || authEnvProj.SandboxEnv[0].Key != "CURSOR_API_KEY" {
		t.Fatalf("auth env sandboxEnv: %+v", authEnvProj.SandboxEnv)
	}
	if authEnvProj.SandboxEnv[0].Value != services.SecretMask || !authEnvProj.SandboxEnv[0].Secret {
		t.Fatalf("auth env mask/secret: %+v", authEnvProj.SandboxEnv[0])
	}

	// Secret placeholder on new key
	w = hn.do("POST", "/api/projects", map[string]any{
		"name":       "BadSecret",
		"sandboxEnv": []map[string]any{{"key": "SECRET", "value": services.SecretMask, "secret": true}},
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("secret mask: %d %s", w.Code, w.Body.String())
	}

	// Update
	newName := "ProjA2"
	newDesc := "updated"
	w = hn.do("PUT", "/api/projects/"+id, map[string]any{
		"name":        newName,
		"description": newDesc,
		"sandboxEnv":  []map[string]any{{"key": "BAR", "value": "baz"}},
		"variables":   []map[string]any{{"name": "y", "type": "string", "value": "2"}},
	})
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "ProjA2") {
		t.Fatalf("update: %d %s", w.Code, w.Body.String())
	}

	// Update missing
	w = hn.do("PUT", "/api/projects/missing-id", map[string]any{"name": "x"})
	if w.Code != http.StatusNotFound {
		t.Fatalf("update missing: %d", w.Code)
	}

	// Update empty name
	empty := "  "
	w = hn.do("PATCH", "/api/projects/"+id, map[string]any{"name": empty})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("update empty name: %d %s", w.Code, w.Body.String())
	}

	// Update bad JSON
	w = hn.do("PUT", "/api/projects/"+id, "bad")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("bad json update: %d", w.Code)
	}

	// Delete ok
	w = hn.do("DELETE", "/api/projects/"+id, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("delete: %d %s", w.Code, w.Body.String())
	}

	// Delete missing
	w = hn.do("DELETE", "/api/projects/"+id, nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("delete missing: %d", w.Code)
	}
}

func TestProjectDeleteWithWorkflows(t *testing.T) {
	hn := newHarness(t)
	w := hn.do("POST", "/api/projects", map[string]any{"name": "HasWF"})
	if w.Code != http.StatusOK {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	id := jsonField(w.Body.String(), "id")

	wf := models.WorkflowDef{
		ID: "wf-proj-block", Name: "blocked", Status: "draft", Version: 1,
		ProjectID: id, Graph: models.Graph{},
	}
	if err := hn.db.Create(&wf).Error; err != nil {
		t.Fatal(err)
	}

	w = hn.do("DELETE", "/api/projects/"+id, nil)
	if w.Code != http.StatusConflict {
		t.Fatalf("delete with workflows: %d %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "流水线") {
		t.Fatalf("expected workflows error body: %s", w.Body.String())
	}
}

func TestProjectNilService(t *testing.T) {
	hn := newHarness(t)
	hn.h.Projects = nil

	if w := hn.do("GET", "/api/projects", nil); w.Code != http.StatusOK {
		t.Fatalf("list nil: %d", w.Code)
	}
	if w := hn.do("GET", "/api/projects/x", nil); w.Code != http.StatusNotFound {
		t.Fatalf("get nil: %d", w.Code)
	}
	if w := hn.do("POST", "/api/projects", map[string]any{"name": "n"}); w.Code != http.StatusInternalServerError {
		t.Fatalf("create nil: %d", w.Code)
	}
	if w := hn.do("PUT", "/api/projects/x", map[string]any{"name": "n"}); w.Code != http.StatusInternalServerError {
		t.Fatalf("update nil: %d", w.Code)
	}
	if w := hn.do("DELETE", "/api/projects/x", nil); w.Code != http.StatusInternalServerError {
		t.Fatalf("delete nil: %d", w.Code)
	}
}

func jsonField(body, key string) string {
	// tiny extractor: "key":"value"
	needle := `"` + key + `":"`
	i := strings.Index(body, needle)
	if i < 0 {
		return ""
	}
	rest := body[i+len(needle):]
	j := strings.IndexByte(rest, '"')
	if j < 0 {
		return ""
	}
	return rest[:j]
}
