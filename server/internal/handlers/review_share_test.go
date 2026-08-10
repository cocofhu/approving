package handlers_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/cocofhu/approving/internal/gateshare"
	"github.com/cocofhu/approving/internal/models"
)

func seedInboxReview(t *testing.T, h *harness, runID, nodeID string, withArtifact bool) {
	t.Helper()
	now := time.Now()
	h.db.Create(&models.WorkflowDef{
		ID: "wf-" + runID, ProjectID: models.DefaultProjectID, Name: "review-" + runID,
		Status: "published", Version: 1,
	})
	h.db.Create(&models.Run{
		ID: runID, WorkflowID: "wf-" + runID, WorkflowName: "review-" + runID, Status: "waiting_human",
		StartedAt: now, Title: "复审运行",
		Graph: models.Graph{Nodes: []models.Node{
			{ID: nodeID, Type: "research", Label: "调研"},
			{ID: "clarify", Type: "react", Label: "澄清"},
			{ID: "hg-r", Type: "human_gate", Label: "审",
				Config: map[string]any{"title": "审阅", "body_template": "请批准", "actions": []any{
					map[string]any{"id": "approve", "label": "批准"},
					map[string]any{"id": "revise", "label": "驳回", "requireForm": true},
				}}},
		}},
	})
	h.db.Create(&models.ReactConversation{
		RunID: runID, NodeID: nodeID, Iteration: 1, Done: false,
		Messages: []models.ReactMessage{{Role: "agent", Text: "请复审 research.json", At: now.Format(time.RFC3339)}},
	})
	h.db.Create(&models.StateRun{
		RunID: runID, NodeID: nodeID, Iteration: 1, Status: "waiting_human", NodeType: "research",
	})
	h.db.Create(&models.Gate{
		RunID: runID, NodeID: "hg-r", Iteration: 1, WorkflowID: "wf-" + runID, WorkflowName: "review-" + runID,
		Title: "审阅视觉稿", BodyMd: "请审阅",
		Actions: []models.GateAction{
			{ID: "approve", Label: "批准"},
			{ID: "revise", Label: "驳回", RequireForm: true},
		},
		Form:        []models.GateField{{Key: "comment", Label: "意见"}},
		RequestedAt: now,
	})
	if withArtifact {
		h.h.Arts.Save(runID, nodeID, "research.json", "json", `{"title":"调研摘要","goals":["g1"],"runId":"should-hide"}`)
		h.h.Arts.Save(runID, "other", "page.html", "html", `<html><body>LEAK-OTHER</body></html>`)
	}
}

func TestReviewShareCreateLookupConfirmAndInboxStatus(t *testing.T) {
	h := newHarness(t)
	seedInboxReview(t, h, "run-rev-1", "research1", true)

	w := h.do(http.MethodPost, "/api/runs/run-rev-1/reviews/research1/share-link", map[string]any{"ttlTier": "24h"})
	if w.Code != http.StatusOK {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	created := parseJSON(t, w)
	url, _ := created["url"].(string)
	if !strings.Contains(url, "/public/gate-approvals#t=") {
		t.Fatalf("url fragment: %s", url)
	}
	token := strings.TrimPrefix(url[strings.Index(url, "#t="):], "#t=")
	if !gateshare.ValidTokenShape(token) {
		t.Fatalf("token shape: %s", token)
	}
	if strings.Contains(w.Body.String(), `"token"`) && !strings.Contains(url, token) {
		t.Fatal("unexpected token field")
	}

	var row models.GateShareLink
	if err := h.db.Where("run_id = ? AND node_id = ?", "run-rev-1", "research1").First(&row).Error; err != nil {
		t.Fatalf("load link: %v", err)
	}
	if row.Kind != models.ShareLinkKindReview {
		t.Fatalf("kind=%q", row.Kind)
	}
	if row.GateID != nil {
		t.Fatalf("review link must not fake GateID: %+v", row.GateID)
	}

	st := parseJSON(t, h.do(http.MethodGet, "/api/runs/run-rev-1/reviews/research1/share-link", nil))
	if st["state"] != models.ShareLinkStateActive {
		t.Fatalf("status: %+v", st)
	}
	if _, ok := st["url"]; ok {
		t.Fatalf("status leaked url: %+v", st)
	}

	list := h.do(http.MethodGet, "/api/gates", nil)
	if list.Code != 200 {
		t.Fatalf("list: %d", list.Code)
	}
	body := list.Body.String()
	if !strings.Contains(body, `"kind":"review"`) {
		t.Fatalf("inbox missing review item: %s", body)
	}
	if !strings.Contains(body, `"shareLink"`) {
		t.Fatalf("inbox missing shareLink: %s", body)
	}
	if strings.Contains(body, token) {
		t.Fatal("inbox leaked plaintext token")
	}

	prev := h.doPublic(http.MethodGet, "/public/gate-approvals/preview", nil, map[string]string{headerShareToken: token})
	p := parseJSON(t, prev)
	if p["status"] != models.ShareLinkStateActive {
		t.Fatalf("preview: %s", prev.Body.String())
	}
	if p["kind"] != models.ShareLinkKindReview {
		t.Fatalf("preview kind: %+v", p)
	}
	if p["actions"] == nil {
		t.Fatalf("preview missing actions: %+v", p)
	}
	actions, _ := p["actions"].(map[string]any)
	if actions["confirm"] != "confirm" || actions["approve"] != nil || actions["reject"] != nil {
		t.Fatalf("review preview must not expose gate reject/approve: %+v", actions)
	}
	if strings.Contains(prev.Body.String(), "run-rev-1") || strings.Contains(prev.Body.String(), "should-hide") || strings.Contains(prev.Body.String(), "LEAK-OTHER") {
		t.Fatalf("preview leak: %s", prev.Body.String())
	}
	if p["structured"] == nil {
		t.Fatalf("expected research.json structured preview: %+v", p)
	}
	if p["productKind"] != "structured" && p["productName"] != "research.json" {
		if p["productName"] != "research.json" {
			t.Fatalf("expected structured research.json product, got kind=%v name=%v", p["productKind"], p["productName"])
		}
	}
	turns, _ := p["turns"].([]any)
	if len(turns) == 0 {
		t.Fatalf("expected sanitized turns: %+v", p)
	}

	nonce := publicPreviewNonce(t, h, token)
	dec := h.doPublic(http.MethodPost, "/public/gate-approvals/decide", map[string]any{
		"token": token, "action": "confirm", "nonce": nonce,
	}, map[string]string{headerShareRequest: "1", "Origin": "http://" + publicHost})
	out := parseJSON(t, dec)
	if dec.Code != 200 || out["status"] != "confirmed" {
		t.Fatalf("decide: %d %s", dec.Code, dec.Body.String())
	}

	prevUsed := h.doPublic(http.MethodGet, "/public/gate-approvals/preview", nil, map[string]string{headerShareToken: token})
	stUsed := parseJSON(t, prevUsed)["status"]
	if stUsed != models.ShareLinkStateUsed {
		t.Fatalf("after confirm preview=%v body=%s", stUsed, prevUsed.Body.String())
	}
	re := h.do(http.MethodPost, "/api/runs/run-rev-1/reviews/research1/share-link", map[string]any{"ttlTier": "24h"})
	if re.Code != http.StatusConflict {
		t.Fatalf("recreate after confirm: %d %s", re.Code, re.Body.String())
	}
}

func TestReviewShareKindIsolationAndHumanGateUnchanged(t *testing.T) {
	h := newHarness(t)
	seedInboxReview(t, h, "run-rev-iso", "research1", true)

	rev := parseJSON(t, h.do(http.MethodPost, "/api/runs/run-rev-iso/reviews/research1/share-link", map[string]any{"ttlTier": "24h"}))
	gate := parseJSON(t, h.do(http.MethodPost, "/api/runs/run-rev-iso/gates/hg-r/share-link", map[string]any{"ttlTier": "8h"}))
	revURL, _ := rev["url"].(string)
	gateURL, _ := gate["url"].(string)
	revTok := strings.TrimPrefix(revURL[strings.Index(revURL, "#t="):], "#t=")
	gateTok := strings.TrimPrefix(gateURL[strings.Index(gateURL, "#t="):], "#t=")

	if w := h.do(http.MethodPost, "/api/runs/run-rev-iso/gates/research1/share-link", map[string]any{"ttlTier": "24h"}); w.Code == 200 {
		t.Fatalf("gates API on research must not mint a link: %s", w.Body.String())
	} else if !strings.Contains(w.Body.String(), "not_human_gate") && !strings.Contains(w.Body.String(), "gate_not_pending") {
		t.Fatalf("gates API on research: %d %s", w.Code, w.Body.String())
	}
	if w := h.do(http.MethodPost, "/api/runs/run-rev-iso/reviews/hg-r/share-link", map[string]any{"ttlTier": "24h"}); w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "not_review_session") {
		t.Fatalf("reviews API on human_gate: %d %s", w.Code, w.Body.String())
	}
	if w := h.do(http.MethodPost, "/api/runs/run-rev-iso/reviews/clarify/share-link", map[string]any{"ttlTier": "24h"}); w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "not_review_session") {
		t.Fatalf("reviews API on react clarify: %d %s", w.Code, w.Body.String())
	}

	revPrev := parseJSON(t, h.doPublic(http.MethodGet, "/public/gate-approvals/preview", nil, map[string]string{headerShareToken: revTok}))
	gatePrev := parseJSON(t, h.doPublic(http.MethodGet, "/public/gate-approvals/preview", nil, map[string]string{headerShareToken: gateTok}))
	if revPrev["kind"] != models.ShareLinkKindReview {
		t.Fatalf("review preview kind: %+v", revPrev)
	}
	if gatePrev["kind"] != models.ShareLinkKindHumanGate && gatePrev["kind"] != nil && gatePrev["kind"] != "" {
		// human_gate preview historically omitted kind or set human_gate
		if gatePrev["status"] != models.ShareLinkStateActive {
			t.Fatalf("gate preview: %+v", gatePrev)
		}
	}
	if gatePrev["status"] != models.ShareLinkStateActive {
		t.Fatalf("gate preview inactive: %+v", gatePrev)
	}
	gActions, _ := gatePrev["actions"].(map[string]any)
	if gActions["approve"] == nil || gActions["reject"] == nil {
		t.Fatalf("gate preview must keep approve/reject: %+v", gActions)
	}

	regen := h.do(http.MethodPost, "/api/runs/run-rev-iso/reviews/research1/share-link/regen", nil)
	if regen.Code != 200 {
		t.Fatalf("regen review: %d %s", regen.Code, regen.Body.String())
	}
	newRevURL, _ := parseJSON(t, regen)["url"].(string)
	newRevTok := strings.TrimPrefix(newRevURL[strings.Index(newRevURL, "#t="):], "#t=")
	gateStill := parseJSON(t, h.doPublic(http.MethodGet, "/public/gate-approvals/preview", nil, map[string]string{headerShareToken: gateTok}))
	if gateStill["status"] != models.ShareLinkStateActive {
		t.Fatalf("regen review must not revoke gate link: %+v", gateStill)
	}

	revNonce := publicPreviewNonce(t, h, newRevTok)
	bad := h.doPublic(http.MethodPost, "/public/gate-approvals/decide", map[string]any{
		"token": newRevTok, "action": "approve", "nonce": revNonce,
	}, map[string]string{headerShareRequest: "1", "Origin": "http://" + publicHost})
	if bad.Code != http.StatusBadRequest || !strings.Contains(bad.Body.String(), "unsupported_action") {
		t.Fatalf("review decide approve must fail: %d %s", bad.Code, bad.Body.String())
	}

	nonce := publicPreviewNonce(t, h, gateTok)
	dec := h.doPublic(http.MethodPost, "/public/gate-approvals/decide", map[string]any{
		"token": gateTok, "action": "approve", "comment": "可以流转", "name": "Jordan", "nonce": nonce,
	}, map[string]string{headerShareRequest: "1", "Origin": "http://" + publicHost})
	if dec.Code != 200 || parseJSON(t, dec)["status"] != "approved" {
		t.Fatalf("gate decide regression: %d %s", dec.Code, dec.Body.String())
	}
}

func TestReviewShareValidationFailureDoesNotBurnLink(t *testing.T) {
	h := newHarness(t)
	seedInboxReview(t, h, "run-rev-val", "research1", false)

	created := parseJSON(t, h.do(http.MethodPost, "/api/runs/run-rev-val/reviews/research1/share-link", map[string]any{"ttlTier": "24h"}))
	url, _ := created["url"].(string)
	token := strings.TrimPrefix(url[strings.Index(url, "#t="):], "#t=")

	nonce := publicPreviewNonce(t, h, token)
	dec := h.doPublic(http.MethodPost, "/public/gate-approvals/decide", map[string]any{
		"token": token, "action": "confirm", "nonce": nonce,
	}, map[string]string{headerShareRequest: "1", "Origin": "http://" + publicHost})
	out := parseJSON(t, dec)
	if out["status"] != "validation_failed" && out["error"] != "review_validation_failed" {
		t.Fatalf("expected validation_failed: %d %s", dec.Code, dec.Body.String())
	}

	prev := h.doPublic(http.MethodGet, "/public/gate-approvals/preview", nil, map[string]string{headerShareToken: token})
	if parseJSON(t, prev)["status"] != models.ShareLinkStateActive {
		t.Fatalf("link burned after validation failure: %s", prev.Body.String())
	}

	h.h.Arts.Save("run-rev-val", "research1", "research.json", "json", `{"title":"调研摘要","goals":["g1"]}`)
	nonce2 := publicPreviewNonce(t, h, token)
	dec2 := h.doPublic(http.MethodPost, "/public/gate-approvals/decide", map[string]any{
		"token": token, "action": "confirm", "nonce": nonce2,
	}, map[string]string{headerShareRequest: "1", "Origin": "http://" + publicHost})
	if dec2.Code != 200 || parseJSON(t, dec2)["status"] != "confirmed" {
		t.Fatalf("retry after artifact: %d %s", dec2.Code, dec2.Body.String())
	}
}

func TestReviewShareLoginConfirmRevokesUnusedLink(t *testing.T) {
	h := newHarness(t)
	seedInboxReview(t, h, "run-rev-login", "research1", true)

	created := parseJSON(t, h.do(http.MethodPost, "/api/runs/run-rev-login/reviews/research1/share-link", map[string]any{"ttlTier": "24h"}))
	url, _ := created["url"].(string)
	token := strings.TrimPrefix(url[strings.Index(url, "#t="):], "#t=")

	w := h.do(http.MethodPost, "/api/runs/run-rev-login/react/research1/reply", map[string]any{
		"text": "确认并流转", "force": true,
	})
	if w.Code != http.StatusOK {
		t.Fatalf("login review reply: %d %s", w.Code, w.Body.String())
	}
	prev := h.doPublic(http.MethodGet, "/public/gate-approvals/preview", nil, map[string]string{headerShareToken: token})
	st := parseJSON(t, prev)["status"]
	if st != models.ShareLinkStateRevoked && st != models.ShareLinkStateUsed {
		t.Fatalf("after login confirm preview=%v body=%s", st, prev.Body.String())
	}
	re := h.do(http.MethodPost, "/api/runs/run-rev-login/reviews/research1/share-link", map[string]any{"ttlTier": "24h"})
	if re.Code != http.StatusConflict {
		t.Fatalf("recreate after login confirm: %d %s", re.Code, re.Body.String())
	}
}

func TestReviewShareDoneConversationCannotCreate(t *testing.T) {
	h := newHarness(t)
	seedInboxReview(t, h, "run-rev-done", "research1", true)
	if err := h.db.Model(&models.ReactConversation{}).Where("run_id = ?", "run-rev-done").Update("done", true).Error; err != nil {
		t.Fatalf("mark done: %v", err)
	}
	w := h.do(http.MethodPost, "/api/runs/run-rev-done/reviews/research1/share-link", map[string]any{"ttlTier": "24h"})
	if w.Code != http.StatusConflict && w.Code != http.StatusNotFound {
		t.Fatalf("create on done conv: %d %s", w.Code, w.Body.String())
	}
}

func TestReviewShareReplyAndCancelDoNotConsume(t *testing.T) {
	h := newHarness(t)
	seedInboxReview(t, h, "run-rev-reply", "research1", true)
	created := parseJSON(t, h.do(http.MethodPost, "/api/runs/run-rev-reply/reviews/research1/share-link", map[string]any{"ttlTier": "24h"}))
	url, _ := created["url"].(string)
	token := strings.TrimPrefix(url[strings.Index(url, "#t="):], "#t=")

	reply := h.doPublic(http.MethodPost, "/public/gate-approvals/reply", map[string]any{
		"token": token, "text": "请把摘要写短一点",
	}, map[string]string{headerShareRequest: "1", "Origin": "http://" + publicHost})
	if reply.Code == http.StatusForbidden {
		t.Fatalf("reply csrf: %s", reply.Body.String())
	}
	prev := h.doPublic(http.MethodGet, "/public/gate-approvals/preview", nil, map[string]string{headerShareToken: token})
	if parseJSON(t, prev)["status"] != models.ShareLinkStateActive {
		t.Fatalf("reply burned token: %d %s preview=%s", reply.Code, reply.Body.String(), prev.Body.String())
	}

	cancel := h.doPublic(http.MethodPost, "/public/gate-approvals/cancel", map[string]any{
		"token": token,
	}, map[string]string{headerShareRequest: "1", "Origin": "http://" + publicHost})
	if cancel.Code == http.StatusForbidden {
		t.Fatalf("cancel csrf: %s", cancel.Body.String())
	}
	prev2 := h.doPublic(http.MethodGet, "/public/gate-approvals/preview", nil, map[string]string{headerShareToken: token})
	if parseJSON(t, prev2)["status"] != models.ShareLinkStateActive {
		t.Fatalf("cancel burned token: %d %s preview=%s", cancel.Code, cancel.Body.String(), prev2.Body.String())
	}

	nonce := publicPreviewNonce(t, h, token)
	dec := h.doPublic(http.MethodPost, "/public/gate-approvals/decide", map[string]any{
		"token": token, "action": "confirm", "nonce": nonce,
	}, map[string]string{headerShareRequest: "1", "Origin": "http://" + publicHost})
	out := parseJSON(t, dec)
	if dec.Code != 200 || (out["status"] != "confirmed" && out["status"] != "validation_failed" && out["status"] != "busy") {
		t.Fatalf("decide after reply: %d %s", dec.Code, dec.Body.String())
	}
}

func seedAppPreviewReview(t *testing.T, h *harness, runID, nodeID string) {
	t.Helper()
	now := time.Now()
	h.db.Create(&models.WorkflowDef{
		ID: "wf-" + runID, ProjectID: models.DefaultProjectID, Name: "preview-" + runID,
		Status: "published", Version: 1,
	})
	h.db.Create(&models.Run{
		ID: runID, WorkflowID: "wf-" + runID, WorkflowName: "preview-" + runID, Status: "waiting_human",
		StartedAt: now, Title: "应用预览运行",
		Graph: models.Graph{Nodes: []models.Node{
			{ID: nodeID, Type: "app_preview", Label: "应用预览"},
		}},
	})
	h.db.Create(&models.ReactConversation{
		RunID: runID, NodeID: nodeID, Iteration: 1, Done: false,
		Messages: []models.ReactMessage{{Role: "agent", Text: "应用预览已就绪", At: now.Format(time.RFC3339)}},
	})
	h.db.Create(&models.StateRun{
		RunID: runID, NodeID: nodeID, Iteration: 1, Status: "waiting_human", NodeType: "app_preview",
	})
}

// TestAppPreviewShareCreateAttachPreviewAndGateAPIRejected covers plan g1/g3/g4.2:
// waiting_human app_preview can mint review share links, inbox attaches shareLink,
// public preview is productKind=app_preview (read-only), and gates API must fail.
func TestAppPreviewShareCreateAttachPreviewAndGateAPIRejected(t *testing.T) {
	h := newHarness(t)
	seedAppPreviewReview(t, h, "run-ap-share", "app_preview1")

	w := h.do(http.MethodPost, "/api/runs/run-ap-share/reviews/app_preview1/share-link", map[string]any{"ttlTier": "24h"})
	if w.Code != http.StatusOK {
		t.Fatalf("create app_preview share: %d %s", w.Code, w.Body.String())
	}
	created := parseJSON(t, w)
	url, _ := created["url"].(string)
	if !strings.Contains(url, "/public/gate-approvals#t=") {
		t.Fatalf("url fragment: %s", url)
	}
	token := strings.TrimPrefix(url[strings.Index(url, "#t="):], "#t=")
	if !gateshare.ValidTokenShape(token) {
		t.Fatalf("token shape: %s", token)
	}

	var row models.GateShareLink
	if err := h.db.Where("run_id = ? AND node_id = ?", "run-ap-share", "app_preview1").First(&row).Error; err != nil {
		t.Fatalf("load link: %v", err)
	}
	if row.Kind != models.ShareLinkKindReview {
		t.Fatalf("kind=%q want review", row.Kind)
	}
	if row.GateID != nil {
		t.Fatalf("app_preview link must not fake GateID: %+v", row.GateID)
	}

	st := parseJSON(t, h.do(http.MethodGet, "/api/runs/run-ap-share/reviews/app_preview1/share-link", nil))
	if st["state"] != models.ShareLinkStateActive {
		t.Fatalf("status: %+v", st)
	}

	list := h.do(http.MethodGet, "/api/gates", nil)
	if list.Code != 200 {
		t.Fatalf("list: %d", list.Code)
	}
	body := list.Body.String()
	if !strings.Contains(body, `"kind":"app_preview"`) {
		t.Fatalf("inbox missing app_preview item: %s", body)
	}
	if !strings.Contains(body, `"shareLink"`) {
		t.Fatalf("inbox missing shareLink for app_preview: %s", body)
	}
	if strings.Contains(body, token) {
		t.Fatal("inbox leaked plaintext token")
	}

	prev := h.doPublic(http.MethodGet, "/public/gate-approvals/preview", nil, map[string]string{headerShareToken: token})
	p := parseJSON(t, prev)
	if p["status"] != models.ShareLinkStateActive {
		t.Fatalf("preview: %s", prev.Body.String())
	}
	if p["kind"] != models.ShareLinkKindReview {
		t.Fatalf("preview kind: %+v", p)
	}
	if p["productKind"] != "app_preview" {
		t.Fatalf("productKind=%v want app_preview", p["productKind"])
	}
	if strings.Contains(prev.Body.String(), "novnc") || strings.Contains(prev.Body.String(), "inspect") {
		t.Fatalf("public preview must not expose novnc/inspect: %s", prev.Body.String())
	}
	actions, _ := p["actions"].(map[string]any)
	if actions["confirm"] != "confirm" {
		t.Fatalf("preview actions: %+v", actions)
	}

	// g3.3: app_preview must not succeed via human_gate share API
	gateW := h.do(http.MethodPost, "/api/runs/run-ap-share/gates/app_preview1/share-link", map[string]any{"ttlTier": "24h"})
	if gateW.Code == http.StatusOK {
		t.Fatalf("gates API on app_preview must fail: %s", gateW.Body.String())
	}
	gateBody := gateW.Body.String()
	if !strings.Contains(gateBody, "not_human_gate") && !strings.Contains(gateBody, "gate_not_pending") {
		t.Fatalf("gates API on app_preview: %d %s", gateW.Code, gateBody)
	}

	nonce := publicPreviewNonce(t, h, token)
	dec := h.doPublic(http.MethodPost, "/public/gate-approvals/decide", map[string]any{
		"token": token, "action": "confirm", "nonce": nonce,
	}, map[string]string{headerShareRequest: "1", "Origin": "http://" + publicHost})
	out := parseJSON(t, dec)
	if dec.Code != 200 || (out["status"] != "confirmed" && out["status"] != "busy" && out["status"] != "validation_failed") {
		t.Fatalf("decide app_preview: %d %s", dec.Code, dec.Body.String())
	}
}
