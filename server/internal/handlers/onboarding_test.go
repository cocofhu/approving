package handlers_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/cocofhu/approving/internal/models"
	"github.com/cocofhu/approving/internal/services"
)

func TestBootstrapOnboardingAPI(t *testing.T) {
	hn := newHarness(t)
	pid := models.DefaultProjectID

	w := hn.do("POST", "/api/projects/"+pid+"/bootstrap-onboarding", map[string]any{
		"acpBackend": "cursor",
		"apiKey":     "",
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("no key: %d %s", w.Code, w.Body.String())
	}

	w = hn.do("POST", "/api/projects", map[string]any{"name": "BootProj"})
	if w.Code != http.StatusOK {
		t.Fatalf("create project: %d %s", w.Code, w.Body.String())
	}
	other := jsonField(w.Body.String(), "id")
	w = hn.do("POST", "/api/projects/"+other+"/bootstrap-onboarding", map[string]any{
		"acpBackend": "cursor",
		"apiKey":     "crsr_test",
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("non-default: %d %s", w.Code, w.Body.String())
	}

	w = hn.do("POST", "/api/projects/"+pid+"/bootstrap-onboarding", map[string]any{
		"acpBackend": "cursor",
		"apiKey":     "crsr_test",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("bootstrap: %d %s", w.Code, w.Body.String())
	}
	var res services.OnboardingBootstrapResult
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if len(res.AgentIDs) != 6 || res.WorkflowID == "" || !res.Published {
		t.Fatalf("bad result: %+v", res)
	}

	w = hn.do("POST", "/api/projects/"+pid+"/bootstrap-onboarding", map[string]any{
		"acpBackend": "cursor",
		"apiKey":     "crsr_test2",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("second: %d %s", w.Code, w.Body.String())
	}
	var res2 services.OnboardingBootstrapResult
	_ = json.Unmarshal(w.Body.Bytes(), &res2)
	if res2.WorkflowID != res.WorkflowID {
		t.Fatalf("workflow id changed")
	}

	w = hn.do("GET", "/api/workflows?projectId="+pid, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("list wf: %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), services.OnboardingWorkflowName) {
		t.Fatalf("workflow missing in list: %s", w.Body.String())
	}
}

func TestCreateWorkflowFromBaselineAPI(t *testing.T) {
	hn := newHarness(t)
	pid := models.DefaultProjectID

	if w := hn.do("POST", "/api/projects/"+pid+"/bootstrap-onboarding", map[string]any{
		"acpBackend": "cursor",
		"apiKey":     "crsr_test",
	}); w.Code != http.StatusOK {
		t.Fatalf("bootstrap: %d %s", w.Code, w.Body.String())
	}

	if w := hn.do("POST", "/api/workflows/from-baseline", map[string]any{
		"projectId": pid,
		"repos":     []map[string]any{{"url": ""}},
	}); w.Code != http.StatusBadRequest {
		t.Fatalf("empty repos: %d %s", w.Code, w.Body.String())
	}

	w := hn.do("POST", "/api/workflows/from-baseline", map[string]any{
		"projectId": pid,
		"repos": []map[string]any{
			{"url": "https://github.com/acme/app.git", "name": "", "branch": "main"},
			{"url": "https://gitlab.com/acme/app.git", "name": ""},
			{"url": "https://github.com/acme/ignored.git", "name": "docs"},
			{"url": "  "},
		},
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create baseline: %d %s", w.Code, w.Body.String())
	}
	var created map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created["name"] != "app" || created["status"] != "published" || created["needsRepo"] != true {
		t.Fatalf("unexpected workflow: %#v", created)
	}
	if created["id"] == "" {
		t.Fatal("missing new workflow id")
	}
	variables, ok := created["variables"].([]any)
	if !ok {
		t.Fatalf("missing variables: %#v", created["variables"])
	}
	var repos []any
	for _, raw := range variables {
		v, _ := raw.(map[string]any)
		if v["name"] == "repos" {
			repos, _ = v["value"].([]any)
		}
	}
	if len(repos) != 3 {
		t.Fatalf("repos length = %d, want 3: %#v", len(repos), repos)
	}
	if second, _ := repos[1].(map[string]any); second["name"] != "app-2" {
		t.Fatalf("second repo not disambiguated: %#v", second)
	}

	w = hn.do("POST", "/api/workflows/from-baseline", map[string]any{
		"projectId": pid,
		"repos":     []map[string]any{{"url": "https://github.com/acme/app.git"}},
	})
	if w.Code != http.StatusCreated || jsonField(w.Body.String(), "name") != "app (2)" {
		t.Fatalf("workflow name not disambiguated: %d %s", w.Code, w.Body.String())
	}

	w = hn.do("GET", "/api/workflows?projectId="+pid, nil)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), services.OnboardingWorkflowName) {
		t.Fatalf("default workflow was overwritten: %d %s", w.Code, w.Body.String())
	}
}
