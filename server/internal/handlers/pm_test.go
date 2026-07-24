package handlers_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/cocofhu/approving/internal/config"
	"github.com/cocofhu/approving/internal/models"
	"github.com/cocofhu/approving/internal/pmmcp"
	"github.com/cocofhu/approving/internal/services"
)

func TestPmLeaderBindingMemoryAndThreadGate(t *testing.T) {
	hn := newHarness(t)
	enableAdmin(t)

	pm := services.NewPmService(hn.db, hn.h.Skill)
	progress := services.NewPmProgress(pm, hn.h.Runs, hn.h.Arts)
	hn.h.Pm = pm
	hn.h.PmProgress = progress
	hn.h.PMMCP = pmmcp.NewHost(pm, progress, nil, hn.h.Runs, nil)

	w := hn.do(http.MethodPost, "/api/projects", map[string]any{"name": "PMProj"})
	if w.Code != 200 {
		t.Fatalf("create project: %d %s", w.Code, w.Body.String())
	}
	var proj map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &proj)
	pid := proj["id"].(string)

	w = hn.do(http.MethodPut, "/api/projects/"+pid+"/pm-leader", map[string]any{
		"enabled": true, "agentConfigRef": "no-such-agent",
	})
	if w.Code != http.StatusConflict {
		t.Fatalf("enable missing agent: %d %s", w.Code, w.Body.String())
	}

	w = hn.do(http.MethodPut, "/api/projects/"+pid+"/pm-leader", map[string]any{
		"enabled": false, "agentConfigRef": "",
	})
	if w.Code != 200 {
		t.Fatalf("disable: %d %s", w.Code, w.Body.String())
	}

	w = hn.do(http.MethodGet, "/api/projects/"+pid+"/pm-leader", nil)
	if w.Code != 200 {
		t.Fatalf("get binding: %d", w.Code)
	}

	w = hn.do(http.MethodPost, "/api/projects/"+pid+"/pm/memories", map[string]any{
		"title": "背景", "content": "Go 项目",
	})
	if w.Code != 200 {
		t.Fatalf("upsert memory: %d %s", w.Code, w.Body.String())
	}
	w = hn.do(http.MethodGet, "/api/projects/"+pid+"/pm/memories", nil)
	if w.Code != 200 {
		t.Fatalf("list memories: %d", w.Code)
	}

	w = hn.do(http.MethodPost, "/api/projects/"+pid+"/pm/threads", map[string]any{})
	if w.Code != http.StatusConflict {
		t.Fatalf("create thread while disabled want 409 got %d %s", w.Code, w.Body.String())
	}

	if err := hn.h.Skill.Save(services.Agent{Name: "pm-demo", ProjectID: pid}); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	w = hn.do(http.MethodPut, "/api/projects/"+pid+"/pm-leader", map[string]any{
		"enabled": true, "agentConfigRef": "pm-demo",
	})
	if w.Code != 200 {
		t.Fatalf("enable: %d %s", w.Code, w.Body.String())
	}

	w = hn.do(http.MethodPost, "/api/projects/"+pid+"/pm/threads", map[string]any{"title": "问进度"})
	if w.Code != 200 {
		t.Fatalf("create thread: %d %s", w.Code, w.Body.String())
	}
	var thr map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &thr)
	tid := thr["id"].(string)

	w = hn.do(http.MethodPost, "/api/projects/"+pid+"/pm/threads/"+tid+"/messages", map[string]any{
		"role": "user", "content": "整体进度如何？",
	})
	if w.Code != 200 {
		t.Fatalf("append message: %d %s", w.Code, w.Body.String())
	}

	w = hn.do(http.MethodGet, "/api/projects/"+pid+"/pm/threads/"+tid+"/messages", nil)
	if w.Code != 200 {
		t.Fatalf("list messages: %d", w.Code)
	}
	var msgResp struct {
		Items []map[string]any `json:"items"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &msgResp)
	if len(msgResp.Items) != 1 {
		t.Fatalf("messages=%d", len(msgResp.Items))
	}
	mid, _ := msgResp.Items[0]["id"].(string)
	if mid == "" {
		t.Fatal("missing message id")
	}
	if st, _ := msgResp.Items[0]["status"].(string); st != "ok" && st != "" {
		t.Fatalf("default status=%v", msgResp.Items[0]["status"])
	}

	w = hn.do(http.MethodPatch, "/api/projects/"+pid+"/pm/threads/"+tid+"/messages/"+mid, map[string]any{
		"status": "failed", "failKind": "connection",
	})
	if w.Code != 200 {
		t.Fatalf("patch fail: %d %s", w.Code, w.Body.String())
	}
	var patched map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &patched)
	if patched["status"] != "failed" || patched["failKind"] != "connection" {
		t.Fatalf("patched=%v", patched)
	}

	w = hn.do(http.MethodPatch, "/api/projects/"+pid+"/pm/threads/"+tid+"/messages/"+mid, map[string]any{
		"status": "ok",
	})
	if w.Code != 200 {
		t.Fatalf("patch clear: %d %s", w.Code, w.Body.String())
	}
	_ = json.Unmarshal(w.Body.Bytes(), &patched)
	if patched["status"] != "ok" {
		t.Fatalf("cleared status=%v", patched["status"])
	}

	w = hn.do(http.MethodDelete, "/api/projects/"+pid+"/pm/memories", nil)
	if w.Code != 200 {
		t.Fatalf("clear memories: %d %s", w.Code, w.Body.String())
	}
}

func TestGetPmDraftReconcilesStaleStreamingWhenNotActive(t *testing.T) {
	hn := newHarness(t)
	enableAdmin(t)

	pm := services.NewPmService(hn.db, hn.h.Skill)
	hn.h.Pm = pm
	hn.h.PmProgress = services.NewPmProgress(pm, hn.h.Runs, hn.h.Arts)
	hn.h.PMMCP = pmmcp.NewHost(pm, hn.h.PmProgress, nil, hn.h.Runs, nil)
	hn.h.PmTurns = services.NewPmTurnRunner(pm, nil)

	w := hn.do(http.MethodPost, "/api/projects", map[string]any{"name": "DraftReconcile"})
	if w.Code != 200 {
		t.Fatalf("create project: %d %s", w.Code, w.Body.String())
	}
	var proj map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &proj)
	pid := proj["id"].(string)
	if err := hn.h.Skill.Save(services.Agent{Name: "pm-draft", ProjectID: pid}); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	w = hn.do(http.MethodPut, "/api/projects/"+pid+"/pm-leader", map[string]any{
		"enabled": true, "agentConfigRef": "pm-draft",
	})
	if w.Code != 200 {
		t.Fatalf("enable: %d %s", w.Code, w.Body.String())
	}
	w = hn.do(http.MethodPost, "/api/projects/"+pid+"/pm/threads", map[string]any{"title": "t"})
	if w.Code != 200 {
		t.Fatalf("thread: %d %s", w.Code, w.Body.String())
	}
	var thr map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &thr)
	tid := thr["id"].(string)

	w = hn.do(http.MethodPost, "/api/projects/"+pid+"/pm/threads/"+tid+"/messages", map[string]any{
		"role": "user", "content": "进度？",
	})
	if w.Code != 200 {
		t.Fatalf("message: %d %s", w.Code, w.Body.String())
	}
	var userMsg map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &userMsg)
	uid := userMsg["id"].(string)

	if _, err := pm.UpsertDraft(tid, uid, "half done…", services.PmDraftStreaming, 1, 0, 0); err != nil {
		t.Fatalf("upsert draft: %v", err)
	}

	w = hn.do(http.MethodGet, "/api/projects/"+pid+"/pm/threads/"+tid+"/draft", nil)
	if w.Code != 200 {
		t.Fatalf("get draft: %d %s", w.Code, w.Body.String())
	}
	var resp struct {
		Live  bool `json:"live"`
		Draft *struct {
			Status      string `json:"status"`
			FailKind    string `json:"failKind"`
			PartialText string `json:"partialText"`
			UserMsgID   string `json:"userMsgId"`
		} `json:"draft"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Live {
		t.Fatal("expected live=false when no Active turn")
	}
	if resp.Draft == nil {
		t.Fatal("expected reconciled draft")
	}
	if resp.Draft.Status != services.PmDraftFailed || resp.Draft.FailKind != services.PmFailConnection {
		t.Fatalf("draft=%+v want failed/connection", resp.Draft)
	}
	if resp.Draft.PartialText != "half done…" {
		t.Fatalf("partialText=%q want preserved", resp.Draft.PartialText)
	}

	w = hn.do(http.MethodGet, "/api/projects/"+pid+"/pm/threads/"+tid+"/messages", nil)
	if w.Code != 200 {
		t.Fatalf("messages: %d", w.Code)
	}
	var msgResp struct {
		Items []map[string]any `json:"items"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &msgResp)
	if len(msgResp.Items) != 1 {
		t.Fatalf("messages=%d", len(msgResp.Items))
	}
	if msgResp.Items[0]["status"] != "failed" || msgResp.Items[0]["failKind"] != services.PmFailConnection {
		t.Fatalf("user msg=%v want failed/connection", msgResp.Items[0])
	}
}

func TestGetPmDraftKeepsStreamingWhenActive(t *testing.T) {
	hn := newHarness(t)
	enableAdmin(t)

	pm := services.NewPmService(hn.db, hn.h.Skill)
	hn.h.Pm = pm
	hn.h.PmProgress = services.NewPmProgress(pm, hn.h.Runs, hn.h.Arts)
	hn.h.PMMCP = pmmcp.NewHost(pm, hn.h.PmProgress, nil, hn.h.Runs, nil)
	runner := services.NewPmTurnRunner(pm, nil)
	hn.h.PmTurns = runner

	w := hn.do(http.MethodPost, "/api/projects", map[string]any{"name": "DraftLive"})
	if w.Code != 200 {
		t.Fatalf("create project: %d %s", w.Code, w.Body.String())
	}
	var proj map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &proj)
	pid := proj["id"].(string)
	if err := hn.h.Skill.Save(services.Agent{Name: "pm-live", ProjectID: pid}); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	w = hn.do(http.MethodPut, "/api/projects/"+pid+"/pm-leader", map[string]any{
		"enabled": true, "agentConfigRef": "pm-live",
	})
	if w.Code != 200 {
		t.Fatalf("enable: %d %s", w.Code, w.Body.String())
	}
	w = hn.do(http.MethodPost, "/api/projects/"+pid+"/pm/threads", map[string]any{"title": "t"})
	if w.Code != 200 {
		t.Fatalf("thread: %d %s", w.Code, w.Body.String())
	}
	var thr map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &thr)
	tid := thr["id"].(string)

	w = hn.do(http.MethodPost, "/api/projects/"+pid+"/pm/threads/"+tid+"/messages", map[string]any{
		"role": "user", "content": "进度？",
	})
	if w.Code != 200 {
		t.Fatalf("message: %d %s", w.Code, w.Body.String())
	}
	var userMsg map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &userMsg)
	uid := userMsg["id"].(string)

	if _, err := pm.UpsertDraft(tid, uid, "live partial", services.PmDraftStreaming, 1, 0, 0); err != nil {
		t.Fatalf("upsert draft: %v", err)
	}
	// Simulate an in-process turn without starting Chat (sbx nil).
	runner.ForceActiveForTest(tid, uid)

	w = hn.do(http.MethodGet, "/api/projects/"+pid+"/pm/threads/"+tid+"/draft", nil)
	if w.Code != 200 {
		t.Fatalf("get draft: %d %s", w.Code, w.Body.String())
	}
	var resp struct {
		Live  bool `json:"live"`
		Draft *struct {
			Status string `json:"status"`
		} `json:"draft"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !resp.Live {
		t.Fatal("expected live=true while Active")
	}
	if resp.Draft == nil || resp.Draft.Status != services.PmDraftStreaming {
		t.Fatalf("draft=%+v want streaming preserved", resp.Draft)
	}
}

func TestPmLeaderNonAdminForbidden(t *testing.T) {
	hn := newHarness(t)
	cfg := config.GetConfig()
	users := make([]config.AuthUser, len(cfg.Auth.Users))
	copy(users, cfg.Auth.Users)
	for i := range users {
		users[i].IsAdmin = false
	}
	cfg.Auth.Users = users
	config.StoreConfig(cfg)

	pm := services.NewPmService(hn.db, hn.h.Skill)
	hn.h.Pm = pm
	hn.h.PMMCP = pmmcp.NewHost(pm, services.NewPmProgress(pm, hn.h.Runs, hn.h.Arts), nil, hn.h.Runs, nil)

	w := hn.do(http.MethodPost, "/api/projects", map[string]any{"name": "NoAdminProj"})
	if w.Code != 200 {
		t.Fatalf("create: %d", w.Code)
	}
	var proj map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &proj)
	pid := proj["id"].(string)

	// Non-admin may enable/disable PM Leader binding.
	w = hn.do(http.MethodPut, "/api/projects/"+pid+"/pm-leader", map[string]any{
		"enabled": false,
	})
	if w.Code != http.StatusOK {
		t.Fatalf("non-admin pm-leader update want 200 got %d %s", w.Code, w.Body.String())
	}

	// Memory human writes remain admin-only.
	w = hn.do(http.MethodPost, "/api/projects/"+pid+"/pm/memories", map[string]any{
		"title": "x", "content": "y",
	})
	if w.Code != http.StatusForbidden {
		t.Fatalf("non-admin memory want 403 got %d", w.Code)
	}
}

func TestListPmMemoriesScopedByRole(t *testing.T) {
	hn := newHarness(t)
	enableAdmin(t)

	pm := services.NewPmService(hn.db, nil)
	hn.h.Pm = pm
	hn.h.PMMCP = pmmcp.NewHost(pm, services.NewPmProgress(pm, hn.h.Runs, hn.h.Arts), nil, hn.h.Runs, nil)

	w := hn.do(http.MethodPost, "/api/projects", map[string]any{"name": "MemScopeProj"})
	if w.Code != 200 {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	var proj map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &proj)
	pid := proj["id"].(string)

	if _, err := pm.UpsertMemory(pid, "agent-a", "A", "secret-a", "agent", "a"); err != nil {
		t.Fatal(err)
	}
	if _, err := pm.UpsertMemory(pid, "agent-b", "B", "secret-b", "agent", "b"); err != nil {
		t.Fatal(err)
	}
	en := true
	agentA := "agent-a"
	if _, err := pm.UpdateBinding(pid, &en, &agentA, nil); err != nil {
		t.Fatal(err)
	}

	w = hn.do(http.MethodGet, "/api/projects/"+pid+"/pm/memories", nil)
	if w.Code != 200 {
		t.Fatalf("admin list: %d %s", w.Code, w.Body.String())
	}
	var all struct {
		Items []map[string]any `json:"items"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &all)
	if len(all.Items) != 2 {
		t.Fatalf("admin want 2 items got %d", len(all.Items))
	}

	w = hn.do(http.MethodGet, "/api/projects/"+pid+"/pm/memories?agent=agent-b", nil)
	if w.Code != 200 {
		t.Fatalf("admin filter: %d", w.Code)
	}
	var filtered struct {
		Items []map[string]any `json:"items"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &filtered)
	if len(filtered.Items) != 1 || filtered.Items[0]["agentName"] != "agent-b" {
		t.Fatalf("admin ?agent=agent-b got %+v", filtered.Items)
	}

	cfg := config.GetConfig()
	users := make([]config.AuthUser, len(cfg.Auth.Users))
	copy(users, cfg.Auth.Users)
	for i := range users {
		users[i].IsAdmin = false
	}
	cfg.Auth.Users = users
	config.StoreConfig(cfg)

	w = hn.do(http.MethodGet, "/api/projects/"+pid+"/pm/memories?agent=agent-b", nil)
	if w.Code != 200 {
		t.Fatalf("non-admin list: %d %s", w.Code, w.Body.String())
	}
	var scoped struct {
		Items []map[string]any `json:"items"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &scoped)
	if len(scoped.Items) != 1 || scoped.Items[0]["agentName"] != "agent-a" {
		t.Fatalf("non-admin must see only PM agent-a, got %+v", scoped.Items)
	}

	empty := ""
	dis := false
	if _, err := pm.UpdateBinding(pid, &dis, &empty, nil); err != nil {
		t.Fatal(err)
	}
	w = hn.do(http.MethodGet, "/api/projects/"+pid+"/pm/memories", nil)
	if w.Code != 200 {
		t.Fatalf("unbound list: %d", w.Code)
	}
	var none struct {
		Items []map[string]any `json:"items"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &none)
	if len(none.Items) != 0 {
		t.Fatalf("unbound non-admin want empty got %+v", none.Items)
	}
}

func TestUpdatePmLeaderEnabledMcps(t *testing.T) {
	hn := newHarness(t)
	enableAdmin(t)

	pm := services.NewPmService(hn.db, nil)
	hn.h.Pm = pm

	w := hn.do(http.MethodPost, "/api/projects", map[string]any{"name": "McpBindProj"})
	if w.Code != 200 {
		t.Fatalf("create: %d", w.Code)
	}
	var proj map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &proj)
	pid := proj["id"].(string)

	w = hn.do(http.MethodPut, "/api/projects/"+pid+"/pm-leader", map[string]any{
		"enabled": true, "agentConfigRef": "agent-a",
		"enabledMcps": []string{"pm-progress", "memory-store", "pm-workflow-read"},
	})
	if w.Code != 200 {
		t.Fatalf("update: %d %s", w.Code, w.Body.String())
	}
	var binding struct {
		EnabledMcps []string `json:"enabledMcps"`
		Enabled     bool     `json:"enabled"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &binding)
	if !binding.Enabled {
		t.Fatal("want enabled")
	}
	if len(binding.EnabledMcps) != 2 {
		t.Fatalf("want only pm-* mcps, got %v", binding.EnabledMcps)
	}
	for _, id := range binding.EnabledMcps {
		if id != "pm-progress" && id != "pm-workflow-read" {
			t.Fatalf("unexpected mcp %q", id)
		}
	}

	w = hn.do(http.MethodPut, "/api/projects/"+pid+"/pm-leader", map[string]any{
		"enabledMcps": []string{"pm-workflow-read"},
	})
	if w.Code != 200 {
		t.Fatalf("patch mcps: %d", w.Code)
	}
	_ = json.Unmarshal(w.Body.Bytes(), &binding)
	if len(binding.EnabledMcps) != 1 || binding.EnabledMcps[0] != "pm-workflow-read" {
		t.Fatalf("got %v", binding.EnabledMcps)
	}

	w = hn.do(http.MethodGet, "/api/projects/"+pid+"/pm-leader", nil)
	if w.Code != 200 {
		t.Fatalf("get: %d", w.Code)
	}
	_ = json.Unmarshal(w.Body.Bytes(), &binding)
	if len(binding.EnabledMcps) != 1 || binding.EnabledMcps[0] != "pm-workflow-read" {
		t.Fatalf("persisted %v", binding.EnabledMcps)
	}
}

func TestProjectCronJobsListAndPatch(t *testing.T) {
	hn := newHarness(t)
	enableAdmin(t)

	pm := services.NewPmService(hn.db, hn.h.Skill)
	hn.h.Pm = pm

	w := hn.do(http.MethodPost, "/api/projects", map[string]any{"name": "CronProjA"})
	if w.Code != 200 {
		t.Fatalf("create a: %d %s", w.Code, w.Body.String())
	}
	var projA map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &projA)
	pidA := projA["id"].(string)

	w = hn.do(http.MethodPost, "/api/projects", map[string]any{"name": "CronProjB"})
	if w.Code != 200 {
		t.Fatalf("create b: %d", w.Code)
	}
	var projB map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &projB)
	pidB := projB["id"].(string)

	now := time.Now().UTC()
	jobs := []models.AgentCronJob{
		{
			ID: "cron-a1", AgentName: "agent-a", ProjectID: pidA, ThreadID: "th-a1",
			Name: "每日汇报", Prompt: "汇报", ScheduleKind: "cron", ScheduleExpr: "0 9 * * *",
			Enabled: true, DeliverToChannel: false, CreatedAt: now, UpdatedAt: now.Add(time.Minute),
		},
		{
			ID: "cron-a2", AgentName: "agent-b", ProjectID: pidA, ThreadID: "th-a2",
			Name: "每周扫描", Prompt: "扫描", ScheduleKind: "every", ScheduleExpr: "7d",
			Enabled: true, DeliverToChannel: true, CreatedAt: now, UpdatedAt: now,
		},
		{
			ID: "cron-b1", AgentName: "agent-a", ProjectID: pidB, ThreadID: "th-b1",
			Name: "其他项目", Prompt: "x", ScheduleKind: "at", ScheduleExpr: now.Format(time.RFC3339),
			Enabled: true, DeliverToChannel: false, CreatedAt: now, UpdatedAt: now,
		},
	}
	for i := range jobs {
		if err := hn.db.Create(&jobs[i]).Error; err != nil {
			t.Fatalf("seed job: %v", err)
		}
	}

	w = hn.do(http.MethodGet, "/api/projects/"+pidA+"/cron-jobs", nil)
	if w.Code != 200 {
		t.Fatalf("list: %d %s", w.Code, w.Body.String())
	}
	var list struct {
		Items []models.AgentCronJob `json:"items"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &list)
	if len(list.Items) != 2 {
		t.Fatalf("want 2 jobs for project A, got %d", len(list.Items))
	}
	if list.Items[0].ID != "cron-a1" {
		t.Fatalf("want updated_at desc first cron-a1, got %s", list.Items[0].ID)
	}
	agents := map[string]bool{}
	for _, j := range list.Items {
		agents[j.AgentName] = true
		if j.ProjectID != pidA {
			t.Fatalf("cross-project leak: %+v", j)
		}
	}
	if !agents["agent-a"] || !agents["agent-b"] {
		t.Fatalf("want both agents, got %v", agents)
	}

	w = hn.do(http.MethodGet, "/api/projects/"+pidB+"/cron-jobs", nil)
	if w.Code != 200 {
		t.Fatalf("list b: %d", w.Code)
	}
	_ = json.Unmarshal(w.Body.Bytes(), &list)
	if len(list.Items) != 1 || list.Items[0].ID != "cron-b1" {
		t.Fatalf("project B isolation: %+v", list.Items)
	}

	w = hn.do(http.MethodPatch, "/api/projects/"+pidA+"/cron-jobs/cron-a1", map[string]any{
		"deliverToChannel": true,
	})
	if w.Code != 200 {
		t.Fatalf("patch: %d %s", w.Code, w.Body.String())
	}
	var patched models.AgentCronJob
	_ = json.Unmarshal(w.Body.Bytes(), &patched)
	if !patched.DeliverToChannel {
		t.Fatal("want deliverToChannel true")
	}

	var stored models.AgentCronJob
	if err := hn.db.Where("id = ?", "cron-a1").First(&stored).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if !stored.DeliverToChannel {
		t.Fatal("deliverToChannel not persisted")
	}

	w = hn.do(http.MethodPatch, "/api/projects/"+pidA+"/cron-jobs/cron-b1", map[string]any{
		"deliverToChannel": true,
	})
	if w.Code != http.StatusNotFound {
		t.Fatalf("cross-project patch want 404 got %d %s", w.Code, w.Body.String())
	}
}

func TestProjectCronJobsPatchAllowedForNonAdmin(t *testing.T) {
	hn := newHarness(t)
	enableAdmin(t)

	pm := services.NewPmService(hn.db, hn.h.Skill)
	hn.h.Pm = pm

	w := hn.do(http.MethodPost, "/api/projects", map[string]any{"name": "CronNoAdmin"})
	if w.Code != 200 {
		t.Fatalf("create: %d", w.Code)
	}
	var proj map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &proj)
	pid := proj["id"].(string)

	now := time.Now().UTC()
	job := models.AgentCronJob{
		ID: "cron-na1", AgentName: "agent-a", ProjectID: pid, ThreadID: "th-na1",
		Name: "任务", Prompt: "p", ScheduleKind: "every", ScheduleExpr: "1h",
		Enabled: true, DeliverToChannel: false, CreatedAt: now, UpdatedAt: now,
	}
	if err := hn.db.Create(&job).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	cfg := config.GetConfig()
	users := make([]config.AuthUser, len(cfg.Auth.Users))
	copy(users, cfg.Auth.Users)
	for i := range users {
		users[i].IsAdmin = false
	}
	cfg.Auth.Users = users
	config.StoreConfig(cfg)

	w = hn.do(http.MethodGet, "/api/projects/"+pid+"/cron-jobs", nil)
	if w.Code != 200 {
		t.Fatalf("non-admin list want 200 got %d", w.Code)
	}

	w = hn.do(http.MethodPatch, "/api/projects/"+pid+"/cron-jobs/cron-na1", map[string]any{
		"deliverToChannel": true,
	})
	if w.Code != 200 {
		t.Fatalf("non-admin patch want 200 got %d %s", w.Code, w.Body.String())
	}
	var patched models.AgentCronJob
	_ = json.Unmarshal(w.Body.Bytes(), &patched)
	if !patched.DeliverToChannel {
		t.Fatal("want deliverToChannel true for non-admin patch")
	}

	var stored models.AgentCronJob
	if err := hn.db.Where("id = ?", "cron-na1").First(&stored).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if !stored.DeliverToChannel {
		t.Fatal("deliverToChannel not persisted for non-admin patch")
	}
}

func setupPmEnabledHarness(t *testing.T) (*harness, string, string) {
	t.Helper()
	hn := newHarness(t)
	enableAdmin(t)
	hn.cookie = hn.login(t)
	pm := services.NewPmService(hn.db, hn.h.Skill)
	hn.h.Pm = pm
	hn.h.PmProgress = services.NewPmProgress(pm, hn.h.Runs, hn.h.Arts)
	hn.h.PMMCP = pmmcp.NewHost(pm, hn.h.PmProgress, nil, hn.h.Runs, nil)
	w := hn.do(http.MethodPost, "/api/projects", map[string]any{"name": "PmHTTP"})
	if w.Code != 200 {
		t.Fatalf("create project: %d %s", w.Code, w.Body.String())
	}
	var proj map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &proj)
	pid := proj["id"].(string)
	// A PM Leader must have this project as its home project (Agent↔project binding).
	if err := hn.h.Skill.Save(services.Agent{Name: "pm-agent", ProjectID: pid, Env: map[string]string{"APPROVING_CURSOR_API_KEY": "test-key"}}); err != nil {
		t.Fatal(err)
	}
	w = hn.do(http.MethodPut, "/api/projects/"+pid+"/pm-leader", map[string]any{
		"enabled": true, "agentConfigRef": "pm-agent",
	})
	if w.Code != 200 {
		t.Fatalf("enable pm: %d %s", w.Code, w.Body.String())
	}
	return hn, pid, ""
}

func TestPmMemoryUpdateDeleteAndWritePmErr(t *testing.T) {
	hn, pid, _ := setupPmEnabledHarness(t)

	w := hn.do(http.MethodPost, "/api/projects/"+pid+"/pm/memories", map[string]any{
		"title": "背景", "content": "Go 项目",
	})
	if w.Code != 200 {
		t.Fatalf("upsert: %d %s", w.Code, w.Body.String())
	}
	var mem map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &mem)
	mid := mem["id"].(string)

	w = hn.do(http.MethodPut, "/api/projects/"+pid+"/pm/memories/"+mid, map[string]any{
		"title": "更新", "content": "新内容",
	})
	if w.Code != 200 {
		t.Fatalf("update: %d %s", w.Code, w.Body.String())
	}
	var updated map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &updated)
	if updated["content"] != "新内容" {
		t.Fatalf("updated=%v", updated)
	}

	w = hn.do(http.MethodPut, "/api/projects/"+pid+"/pm/memories/missing-id", map[string]any{
		"title": "x", "content": "y",
	})
	if w.Code != http.StatusNotFound {
		t.Fatalf("update missing want 404 got %d", w.Code)
	}

	w = hn.do(http.MethodDelete, "/api/projects/"+pid+"/pm/memories/"+mid, nil)
	if w.Code != 200 {
		t.Fatalf("delete: %d %s", w.Code, w.Body.String())
	}
	w = hn.do(http.MethodDelete, "/api/projects/"+pid+"/pm/memories/"+mid, nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("delete twice want 404 got %d", w.Code)
	}
}

func TestPmThreadCRUDAndMessages(t *testing.T) {
	hn, pid, _ := setupPmEnabledHarness(t)

	w := hn.do(http.MethodPost, "/api/projects/"+pid+"/pm/threads", map[string]any{"title": "进度"})
	if w.Code != 200 {
		t.Fatalf("create thread: %d %s", w.Code, w.Body.String())
	}
	var thr map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &thr)
	tid := thr["id"].(string)

	w = hn.do(http.MethodGet, "/api/projects/"+pid+"/pm/threads", nil)
	if w.Code != 200 {
		t.Fatalf("list threads: %d", w.Code)
	}
	w = hn.do(http.MethodGet, "/api/projects/"+pid+"/pm/threads/"+tid, nil)
	if w.Code != 200 {
		t.Fatalf("get thread: %d %s", w.Code, w.Body.String())
	}

	for i := 0; i < 3; i++ {
		w = hn.do(http.MethodPost, "/api/projects/"+pid+"/pm/threads/"+tid+"/messages", map[string]any{
			"role": "user", "content": "msg",
		})
		if w.Code != 200 {
			t.Fatalf("append %d: %d %s", i, w.Code, w.Body.String())
		}
	}
	w = hn.do(http.MethodGet, "/api/projects/"+pid+"/pm/threads/"+tid+"/messages", nil)
	if w.Code != 200 {
		t.Fatalf("list messages: %d", w.Code)
	}
	var msgs struct {
		Items []map[string]any `json:"items"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &msgs)
	if len(msgs.Items) != 3 {
		t.Fatalf("messages=%d", len(msgs.Items))
	}

	w = hn.do(http.MethodDelete, "/api/projects/"+pid+"/pm/threads/"+tid, nil)
	if w.Code != 200 {
		t.Fatalf("delete thread: %d %s", w.Code, w.Body.String())
	}
	w = hn.do(http.MethodGet, "/api/projects/"+pid+"/pm/threads/"+tid, nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("get deleted want 404 got %d", w.Code)
	}
}

func TestPatchPmMessageFailureMetadata(t *testing.T) {
	hn, pid, _ := setupPmEnabledHarness(t)
	w := hn.do(http.MethodPost, "/api/projects/"+pid+"/pm/threads", map[string]any{"title": "patch"})
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	var thr map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &thr)
	tid := thr["id"].(string)
	w = hn.do(http.MethodPost, "/api/projects/"+pid+"/pm/threads/"+tid+"/messages", map[string]any{
		"role": "user", "content": "fail me",
	})
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	var msg map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &msg)
	mid := msg["id"].(string)

	w = hn.do(http.MethodPatch, "/api/projects/"+pid+"/pm/threads/"+tid+"/messages/"+mid, map[string]any{
		"status": "failed", "failKind": "connection",
	})
	if w.Code != 200 {
		t.Fatalf("patch: %d %s", w.Code, w.Body.String())
	}
	w = hn.do(http.MethodPatch, "/api/projects/"+pid+"/pm/threads/"+tid+"/messages/missing", map[string]any{
		"status": "failed",
	})
	if w.Code != http.StatusNotFound {
		t.Fatalf("patch missing want 404 got %d", w.Code)
	}
}

func TestPmUpsertMemoryBadRequest(t *testing.T) {
	hn, pid, _ := setupPmEnabledHarness(t)
	w := hn.do(http.MethodPost, "/api/projects/"+pid+"/pm/memories", map[string]any{
		"title": "", "content": "",
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("empty memory want 400 got %d %s", w.Code, w.Body.String())
	}
}

func TestPmThreadDisabledReturnsConflict(t *testing.T) {
	hn := newHarness(t)
	enableAdmin(t)
	pm := services.NewPmService(hn.db, nil)
	hn.h.Pm = pm
	w := hn.do(http.MethodPost, "/api/projects", map[string]any{"name": "DisabledPM"})
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	var proj map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &proj)
	pid := proj["id"].(string)
	w = hn.do(http.MethodPost, "/api/projects/"+pid+"/pm/threads", map[string]any{})
	if w.Code != http.StatusConflict {
		t.Fatalf("disabled thread create want 409 got %d", w.Code)
	}
}

func TestPmChannelThreadWriteDeleteForbidden(t *testing.T) {
	hn, pid, _ := setupPmEnabledHarness(t)
	hn.h.PmTurns = services.NewPmTurnRunner(hn.h.Pm, hn.h.Sbx)

	channel, err := hn.h.Pm.CreateThread(pid, "qq:guild:ch1", "频道会话", "pm-agent", "user")
	if err != nil {
		t.Fatalf("create channel thread: %v", err)
	}

	w := hn.do(http.MethodPost, "/api/projects/"+pid+"/pm/threads", map[string]any{"title": "Web会话"})
	if w.Code != 200 {
		t.Fatalf("create web thread: %d %s", w.Code, w.Body.String())
	}
	var web map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &web)
	webTID := web["id"].(string)

	assertChannelReadOnly := func(t *testing.T, label string, code int, body string) {
		t.Helper()
		if code != http.StatusForbidden {
			t.Fatalf("%s want 403 got %d %s", label, code, body)
		}
		if !strings.Contains(body, "渠道") || !strings.Contains(body, "只读") {
			t.Fatalf("%s want channel read-only error, got %s", label, body)
		}
	}

	w = hn.do(http.MethodPost, "/api/projects/"+pid+"/pm/threads/"+channel.ID+"/messages", map[string]any{
		"role": "user", "content": "不应发送",
	})
	assertChannelReadOnly(t, "append channel", w.Code, w.Body.String())

	w = hn.do(http.MethodDelete, "/api/projects/"+pid+"/pm/threads/"+channel.ID, nil)
	assertChannelReadOnly(t, "delete channel", w.Code, w.Body.String())

	w = hn.do(http.MethodGet, "/api/projects/"+pid+"/pm/threads/"+channel.ID+"/chat", nil)
	assertChannelReadOnly(t, "ws chat channel", w.Code, w.Body.String())

	w = hn.do(http.MethodPost, "/api/projects/"+pid+"/pm/threads/"+webTID+"/messages", map[string]any{
		"role": "user", "content": "web ok",
	})
	if w.Code != 200 {
		t.Fatalf("append web: %d %s", w.Code, w.Body.String())
	}
	w = hn.do(http.MethodDelete, "/api/projects/"+pid+"/pm/threads/"+webTID, nil)
	if w.Code != 200 {
		t.Fatalf("delete web: %d %s", w.Code, w.Body.String())
	}
}
