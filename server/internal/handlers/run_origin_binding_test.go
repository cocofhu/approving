package handlers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/cocofhu/approving/internal/models"
	"github.com/cocofhu/approving/internal/services"
)

// stubAnnouncer stands in for the channel manager. It records the order the
// handler calls it in relative to the ledger write, which is the part of this
// endpoint that is easy to get backwards.
type stubAnnouncer struct {
	svc       *services.TaskContextService
	projectID string
	calls     []bool
	// boundAtCall is what the ledger said each time the announcer was asked to
	// speak. On detach it must still read bound, or the goodbye was sent too
	// late to get out.
	boundAtCall []bool
	deliver     bool
}

func (s *stubAnnouncer) AnnounceOriginBinding(_ context.Context, projectID, runID string, bound bool) bool {
	s.calls = append(s.calls, bound)
	identity, _ := s.svc.IdentityForRun(runID, projectID)
	s.boundAtCall = append(s.boundAtCall, identity != nil && identity.OriginUnboundAt == nil)
	return s.deliver
}

func seedOriginRun(t *testing.T, h *harness) (runID string, svc *services.TaskContextService, announcer *stubAnnouncer) {
	t.Helper()
	w := h.do("POST", "/api/projects", map[string]any{"name": "BindingHome"})
	if w.Code != http.StatusOK {
		t.Fatalf("create project: %d %s", w.Code, w.Body)
	}
	var proj map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &proj); err != nil {
		t.Fatal(err)
	}
	pid, _ := proj["id"].(string)

	wf := models.WorkflowDef{ID: "wf-binding", ProjectID: pid, Name: "支付登录页", Status: "published"}
	if err := h.db.Create(&wf).Error; err != nil {
		t.Fatal(err)
	}
	run := models.Run{ID: "run-binding", WorkflowID: wf.ID, WorkflowName: wf.Name, Status: "running"}
	if err := h.db.Create(&run).Error; err != nil {
		t.Fatal(err)
	}

	svc = services.NewTaskContextService(h.db)
	if _, err := svc.EnsureIdentity(services.EnsureTaskIdentityInput{
		RunID: run.ID, ProjectID: pid, UserID: "user1", ShortTitle: "支付登录页",
		Status: "active", OriginChannel: "qq", OriginScene: "c2c",
		OriginConversationID: "conv-1", OriginExternalUserID: "u1",
	}); err != nil {
		t.Fatal(err)
	}
	announcer = &stubAnnouncer{svc: svc, projectID: pid, deliver: true}
	h.h.TaskContext = svc
	h.h.OriginAnnouncer = announcer
	return run.ID, svc, announcer
}

// TestPatchRunOriginBindingSaysGoodbyeBeforeGoingQuiet locks the ordering the
// whole feature rests on. If the mark were written first the delivery guard
// would swallow the goodbye, and the person who asked for the work would just
// stop hearing back with no explanation.
func TestPatchRunOriginBindingSaysGoodbyeBeforeGoingQuiet(t *testing.T) {
	h := newHarness(t)
	runID, svc, announcer := seedOriginRun(t, h)

	w := h.do("PATCH", "/api/runs/"+runID+"/origin-binding", map[string]any{"bound": false})
	if w.Code != http.StatusOK {
		t.Fatalf("unbind: %d %s", w.Code, w.Body)
	}
	var body struct {
		Origin struct {
			ConversationID string `json:"conversationId"`
			Unbound        bool   `json:"unbound"`
		} `json:"origin"`
		NoticeDelivered bool `json:"noticeDelivered"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if !body.Origin.Unbound || !body.NoticeDelivered {
		t.Fatalf("unbind response = %+v", body)
	}
	if body.Origin.ConversationID != "conv-1" {
		t.Fatalf("origin conversation = %q; detaching must not erase where it came from",
			body.Origin.ConversationID)
	}
	if len(announcer.calls) != 1 || announcer.calls[0] {
		t.Fatalf("announcer calls = %v want one detach notice", announcer.calls)
	}
	if !announcer.boundAtCall[0] {
		t.Fatal("the goodbye was attempted after the mark was written, so the guard would eat it")
	}

	identity, err := svc.IdentityForRun(runID, announcer.projectID)
	if err != nil || identity == nil || identity.OriginUnboundAt == nil {
		t.Fatalf("unbind mark not persisted: %+v err=%v", identity, err)
	}

	// Reconnecting is the mirror image: clear the mark first, then speak.
	w = h.do("PATCH", "/api/runs/"+runID+"/origin-binding", map[string]any{"bound": true})
	if w.Code != http.StatusOK {
		t.Fatalf("rebind: %d %s", w.Code, w.Body)
	}
	if len(announcer.calls) != 2 || !announcer.calls[1] {
		t.Fatalf("announcer calls = %v want a follow-up hello", announcer.calls)
	}
	if !announcer.boundAtCall[1] {
		t.Fatal("the hello must be spoken after the run is bound again")
	}
}

// TestPatchRunOriginBindingReportsAnUnheardGoodbye: the caller needs to know
// the requester was never told, because from their side the run just goes
// silent.
func TestPatchRunOriginBindingReportsAnUnheardGoodbye(t *testing.T) {
	h := newHarness(t)
	runID, _, announcer := seedOriginRun(t, h)
	announcer.deliver = false

	w := h.do("PATCH", "/api/runs/"+runID+"/origin-binding", map[string]any{"bound": false})
	if w.Code != http.StatusOK {
		t.Fatalf("unbind: %d %s", w.Code, w.Body)
	}
	var body struct {
		NoticeDelivered bool `json:"noticeDelivered"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.NoticeDelivered {
		t.Fatal("an undelivered goodbye must not be reported as delivered")
	}
}

// TestPatchRunOriginBindingIsIdempotent: a double-click must not send a second
// goodbye or flip the state back.
func TestPatchRunOriginBindingIsIdempotent(t *testing.T) {
	h := newHarness(t)
	runID, _, announcer := seedOriginRun(t, h)

	for i := 0; i < 2; i++ {
		if w := h.do("PATCH", "/api/runs/"+runID+"/origin-binding",
			map[string]any{"bound": false}); w.Code != http.StatusOK {
			t.Fatalf("unbind %d: %d %s", i, w.Code, w.Body)
		}
	}
	if len(announcer.calls) != 1 {
		t.Fatalf("announcer calls = %v; repeating the request must not repeat the notice", announcer.calls)
	}
}

func TestPatchRunOriginBindingRejectsRunsWithNothingToUnbind(t *testing.T) {
	h := newHarness(t)
	runID, _, _ := seedOriginRun(t, h)

	if w := h.do("PATCH", "/api/runs/missing/origin-binding",
		map[string]any{"bound": false}); w.Code != http.StatusNotFound {
		t.Fatalf("unknown run: %d", w.Code)
	}
	if w := h.do("PATCH", "/api/runs/"+runID+"/origin-binding",
		map[string]any{}); w.Code != http.StatusBadRequest {
		t.Fatal("bound is required rather than a toggle; a missing field must be rejected")
	}

	// A run started from the web has no conversation to detach from.
	wf := models.WorkflowDef{ID: "wf-web", ProjectID: "p-web", Name: "网页发起", Status: "published"}
	if err := h.db.Create(&wf).Error; err != nil {
		t.Fatal(err)
	}
	if err := h.db.Create(&models.Run{ID: "run-web", WorkflowID: wf.ID, Status: "running"}).Error; err != nil {
		t.Fatal(err)
	}
	if w := h.do("PATCH", "/api/runs/run-web/origin-binding",
		map[string]any{"bound": false}); w.Code != http.StatusConflict {
		t.Fatalf("web-triggered run: %d %s", w.Code, w.Body)
	}
}
