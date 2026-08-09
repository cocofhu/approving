package handlers_test

import (
	"encoding/json"
	"net/http"
	"testing"
)

type eventRouteBody struct {
	Key            string   `json:"key"`
	ToLive         bool     `json:"toLive"`
	ToTemplate     bool     `json:"toTemplate"`
	Unbindable     bool     `json:"unbindable"`
	NoEgress       bool     `json:"noEgress"`
	TemplateActive bool     `json:"templateActive"`
	ActiveKinds    []string `json:"activeKinds"`
}

func projectEventRoutes(t *testing.T, h *harness, projectID string) map[string]eventRouteBody {
	t.Helper()
	w := h.do("GET", "/api/projects/"+projectID+"/event-routing", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("event-routing: %d %s", w.Code, w.Body)
	}
	var body struct {
		Routes []eventRouteBody `json:"routes"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	out := make(map[string]eventRouteBody, len(body.Routes))
	for _, route := range body.Routes {
		out[route.Key] = route
	}
	return out
}

// TestProjectEventRoutingReflectsNotifyPolicy is the point of serving this
// table per project rather than as a static document: a route the project
// switched off has to read as off, otherwise the page describes what the code
// can do instead of what it will do.
func TestProjectEventRoutingReflectsNotifyPolicy(t *testing.T) {
	h := newHarness(t)
	w := h.do("POST", "/api/projects", map[string]any{"name": "RoutingHome"})
	if w.Code != http.StatusOK {
		t.Fatalf("create project: %d %s", w.Code, w.Body)
	}
	var proj map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &proj); err != nil {
		t.Fatal(err)
	}
	pid, _ := proj["id"].(string)

	routes := projectEventRoutes(t, h, pid)

	paused, ok := routes["task_paused"]
	if !ok {
		t.Fatal("task_paused missing from the routing table")
	}
	if !paused.ToLive || !paused.Unbindable {
		t.Fatalf("a pause reaches the origin conversation and can be unbound, got %+v", paused)
	}

	notify, ok := routes["run_notification"]
	if !ok {
		t.Fatal("run_notification missing from the routing table")
	}
	if notify.Unbindable {
		t.Fatal("the project template push does not go through the per-Run guard")
	}
	if !notify.TemplateActive || len(notify.ActiveKinds) != 2 {
		t.Fatalf("a fresh project pushes both kinds, got %+v", notify)
	}

	gate, ok := routes["gate_auto_invoke"]
	if !ok {
		t.Fatal("gate_auto_invoke missing; the automatic handover fan-out stays invisible without it")
	}
	if !gate.NoEgress || gate.ToLive || gate.ToTemplate {
		t.Fatalf("gate auto invoke never reaches IM, got %+v", gate)
	}

	// Switching the project kill-switch off must show up here, not just in the
	// notify panel.
	if w := h.do("PATCH", "/api/projects/"+pid, map[string]any{
		"notifyPolicy": map[string]any{"enabled": false, "defaultEvents": []string{"waiting_human", "failed"}},
	}); w.Code != http.StatusOK {
		t.Fatalf("disable notify: %d %s", w.Code, w.Body)
	}

	routes = projectEventRoutes(t, h, pid)
	notify = routes["run_notification"]
	if notify.TemplateActive || len(notify.ActiveKinds) != 0 {
		t.Fatalf("a disabled project pushes nothing, got %+v", notify)
	}
	// The conversation side is a different audience and is not governed by the
	// notify policy at all.
	if !routes["task_paused"].ToLive {
		t.Fatal("turning off the project push must not mute the origin conversation")
	}
}

func TestProjectEventRoutingUnknownProject(t *testing.T) {
	h := newHarness(t)
	if w := h.do("GET", "/api/projects/nope/event-routing", nil); w.Code != http.StatusNotFound {
		t.Fatalf("unknown project: %d", w.Code)
	}
}
