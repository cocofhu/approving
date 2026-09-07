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
