package handlers_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cocofhu/approving/internal/auth"
	"github.com/cocofhu/approving/internal/contextmcp"
	"github.com/cocofhu/approving/internal/handlers"
	"github.com/cocofhu/approving/internal/memorymcp"
	"github.com/cocofhu/approving/internal/pmmcp"
	"github.com/cocofhu/approving/internal/schedulermcp"
	"github.com/cocofhu/approving/internal/services"

	"github.com/gin-gonic/gin"
)

func TestEnsurePmSandboxOpensAndBuildsPreamble(t *testing.T) {
	hn, pid, _ := setupPmEnabledHarness(t)
	hn.h.PMMCP = pmmcp.NewHost(hn.h.Pm, hn.h.PmProgress, hn.h.WF, hn.h.Runs, hn.h.Arts, nil)

	w := hn.do(http.MethodPost, "/api/projects/"+pid+"/pm/threads", map[string]any{"title": "沙箱"})
	if w.Code != 200 {
		t.Fatalf("thread: %d %s", w.Code, w.Body.String())
	}
	var thr map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &thr)
	tid := thr["id"].(string)

	w = hn.do(http.MethodPost, "/api/projects/"+pid+"/pm/threads/"+tid+"/messages", map[string]any{
		"role": "user", "content": "历史消息",
	})
	if w.Code != 200 {
		t.Fatalf("msg: %d", w.Code)
	}

	w = hn.do(http.MethodPost, "/api/projects/"+pid+"/pm/threads/"+tid+"/sandbox", map[string]any{
		"injectHistory": true,
		"attachedContext": map[string]any{
			"kind": "run", "id": "run-1", "label": "某次运行",
		},
	})
	if w.Code != 200 {
		t.Fatalf("sandbox: %d %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["preamble"] == nil || resp["sandbox"] == nil {
		t.Fatalf("resp=%v", resp)
	}

	// Second call should hit reuse path (same thread sandbox).
	w = hn.do(http.MethodPost, "/api/projects/"+pid+"/pm/threads/"+tid+"/sandbox", map[string]any{
		"injectHistory": false,
	})
	if w.Code != 200 {
		t.Fatalf("reuse sandbox: %d %s", w.Code, w.Body.String())
	}
}

func TestPmMCPRPCPostInitialize(t *testing.T) {
	hn, pid, _ := setupPmEnabledHarness(t)
	progress := services.NewPmProgress(hn.h.Pm, hn.h.Runs, hn.h.Arts)
	hn.h.PmProgress = progress
	hn.h.PMMCP = pmmcp.NewHost(hn.h.Pm, progress, hn.h.WF, hn.h.Runs, hn.h.Arts, nil)
	tok := hn.h.PMMCP.Register(pid, "thr-rpc", "admin", "pm-agent")
	body, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "initialize",
		"params": map[string]any{"protocolVersion": "2024-11-05"},
	})
	req := httptest.NewRequest(http.MethodPost, "/mcp/pm/"+pid+"/pm-progress", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tok)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: hn.cookie})
	w := httptest.NewRecorder()
	hn.r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("rpc init: %d %s", w.Code, w.Body.String())
	}
}

func TestEnsurePmSandboxUnavailable(t *testing.T) {
	hn := newHarness(t)
	enableAdmin(t)
	hn.cookie = hn.login(t)
	pm := services.NewPmService(hn.db, hn.h.Skill)
	hn.h.Pm = pm
	hn.h.PmProgress = services.NewPmProgress(pm, hn.h.Runs, hn.h.Arts)
	hn.h.PMMCP = nil
	w := hn.do(http.MethodPost, "/api/projects", map[string]any{"name": "NoMCP"})
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	var proj map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &proj)
	pid := proj["id"].(string)
	if err := hn.h.Skill.Save(services.Agent{Name: "pm-agent", ProjectID: pid, Env: map[string]string{"APPROVING_CURSOR_API_KEY": "test-key"}}); err != nil {
		t.Fatal(err)
	}
	w = hn.do(http.MethodPut, "/api/projects/"+pid+"/pm-leader", map[string]any{
		"enabled": true, "agentConfigRef": "pm-agent",
	})
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	w = hn.do(http.MethodPost, "/api/projects/"+pid+"/pm/threads", map[string]any{})
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	var thr map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &thr)
	tid := thr["id"].(string)
	w = hn.do(http.MethodPost, "/api/projects/"+pid+"/pm/threads/"+tid+"/sandbox", nil)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("unavailable want 500 got %d", w.Code)
	}
}

func TestPmMCPRPCEndpoints(t *testing.T) {
	gin.SetMode(gin.TestMode)
	hn := newHarness(t)
	pm := services.NewPmService(hn.db, hn.h.Skill)
	progress := services.NewPmProgress(pm, hn.h.Runs, hn.h.Arts)
	hn.h.Pm = pm
	hn.h.PmProgress = progress
	hn.h.PMMCP = pmmcp.NewHost(pm, progress, hn.h.WF, hn.h.Runs, hn.h.Arts, nil)

	w := hn.do(http.MethodGet, "/mcp/pm/proj-1", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("GET pm mcp: %d", w.Code)
	}
	w = hn.do(http.MethodDelete, "/mcp/pm/proj-1", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("DELETE pm mcp: %d", w.Code)
	}

	hn.h.PMMCP = nil
	w = hn.do(http.MethodPost, "/mcp/pm/proj-1", map[string]any{"jsonrpc": "2.0", "id": 1})
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("nil host: %d", w.Code)
	}
}

func TestMemoryMCPRPCGetDelete(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &handlers.Handlers{}
	r := gin.New()
	r.GET("/mcp/memory-store/:projectId", h.MemoryMCPRPC)
	r.DELETE("/mcp/memory-store/:projectId", h.MemoryMCPRPC)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/mcp/memory-store/p1", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("GET: %d", w.Code)
	}
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/mcp/memory-store/p1", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("DELETE: %d", w.Code)
	}
}

func TestPlatformMCPUnavailable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &handlers.Handlers{}
	r := gin.New()
	r.POST("/mcp/memory-store/:projectId", h.MemoryMCPRPC)
	w := doRaw(r, http.MethodPost, "/mcp/memory-store/p1", `{}`, "")
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("memory mcp: %d", w.Code)
	}
	r.POST("/mcp/context-store/:projectId", h.ContextMCPRPC)
	w = doRaw(r, http.MethodPost, "/mcp/context-store/p1", `{}`, "")
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("context mcp: %d", w.Code)
	}
	r.POST("/mcp/task-scheduler/:agentName", h.SchedulerMCPRPC)
	w = doRaw(r, http.MethodPost, "/mcp/task-scheduler/a1", `{}`, "")
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("scheduler mcp: %d", w.Code)
	}
}

func TestClearPmMemoriesHandler(t *testing.T) {
	hn, pid, _ := setupPmEnabledHarness(t)
	w := hn.do(http.MethodPost, "/api/projects/"+pid+"/pm/memories", map[string]any{
		"title": "x", "content": "y",
	})
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	w = hn.do(http.MethodDelete, "/api/projects/"+pid+"/pm/memories", nil)
	if w.Code != 200 {
		t.Fatalf("clear: %d %s", w.Code, w.Body.String())
	}
}

func TestClearPmThreadContextHandler(t *testing.T) {
	hn, pid, _ := setupPmEnabledHarness(t)
	hn.h.TaskContext = services.NewTaskContextService(hn.db)
	th, err := hn.h.Pm.CreateThread(pid, "qq:c2c:u-clear", "渠道", "pm-agent", "user")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := hn.h.Pm.AppendMessage(th.ID, "user", "之前聊过", nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := hn.h.TaskContext.EnsureIdentity(services.EnsureTaskIdentityInput{
		RunID: "run-ctx", ProjectID: pid, UserID: services.SyntheticQQUserID("u-clear"),
		ShortTitle: "旧任务", Status: "active",
		OriginChannel: "qq", OriginConversationID: "u-clear",
	}); err != nil {
		t.Fatal(err)
	}

	w := hn.do(http.MethodDelete, "/api/projects/"+pid+"/pm/threads/"+th.ID+"/context", nil)
	if w.Code != 200 {
		t.Fatalf("clear context: %d %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["status"] != "cleared" || resp["messagesCleared"].(float64) < 1 {
		t.Fatalf("resp=%v", resp)
	}
	msgs, err := hn.h.Pm.ListMessages(th.ID)
	if err != nil || len(msgs) != 0 {
		t.Fatalf("messages left=%d err=%v", len(msgs), err)
	}
	if _, err := hn.h.Pm.GetThreadByID(th.ID); err != nil {
		t.Fatal("thread must remain")
	}
}

func TestGetPmLeaderProjectMissing(t *testing.T) {
	hn, _, _ := setupPmEnabledHarness(t)
	w := hn.do(http.MethodGet, "/api/projects/does-not-exist/pm-leader", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("missing project: %d", w.Code)
	}
}

func setupPmMCPStack(t *testing.T) (*harness, string) {
	t.Helper()
	hn, pid, _ := setupPmEnabledHarness(t)
	hn.h.MemoryMCP = memorymcp.NewHost(hn.h.Pm)
	hn.h.ContextMCP = contextmcp.NewHost(hn.h.Pm)
	hn.h.SchedulerMCP = schedulermcp.NewHost(hn.db, hn.h.Pm)
	return hn, pid
}

func TestPmDeleteThreadUnregistersPlatformMCP(t *testing.T) {
	hn, pid := setupPmMCPStack(t)
	w := hn.do(http.MethodPost, "/api/projects/"+pid+"/pm/threads", map[string]any{"title": "del"})
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	var thr map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &thr)
	tid := thr["id"].(string)

	w = hn.do(http.MethodPost, "/api/projects/"+pid+"/pm/threads/"+tid+"/sandbox", map[string]any{})
	if w.Code != 200 {
		t.Fatalf("sandbox: %d %s", w.Code, w.Body.String())
	}
	if _, ok := hn.h.PMMCP.TokenForThread(pid, tid); !ok {
		t.Fatal("expected pm token after sandbox")
	}

	w = hn.do(http.MethodDelete, "/api/projects/"+pid+"/pm/threads/"+tid, nil)
	if w.Code != 200 {
		t.Fatalf("delete thread: %d %s", w.Code, w.Body.String())
	}
	if _, ok := hn.h.PMMCP.TokenForThread(pid, tid); ok {
		t.Fatal("token should be unregistered")
	}
}

func TestSchedulerMCPRPCHandler(t *testing.T) {
	hn, pid := setupPmMCPStack(t)
	agent := "pm-agent"
	tok := hn.h.SchedulerMCP.Register(pid, agent, "thr-s", "admin", true)

	w := doRaw(hn.r, http.MethodGet, "/mcp/task-scheduler/"+agent, "", "")
	if w.Code != http.StatusOK {
		t.Fatalf("GET: %d", w.Code)
	}
	w = doRaw(hn.r, http.MethodDelete, "/mcp/task-scheduler/"+agent, "", "")
	if w.Code != http.StatusOK {
		t.Fatalf("DELETE: %d", w.Code)
	}

	body, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "initialize",
		"params": map[string]any{"protocolVersion": "2024-11-05"},
	})
	req := httptest.NewRequest(http.MethodPost, "/mcp/task-scheduler/"+agent, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tok)
	w = httptest.NewRecorder()
	hn.r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("POST init: %d %s", w.Code, w.Body.String())
	}
}

func TestContextMCPRPCPostInitialize(t *testing.T) {
	hn, pid := setupPmMCPStack(t)
	tok := hn.h.ContextMCP.Register(pid, "pm-agent", "thr-ctx", "admin")
	body, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "initialize",
	})
	req := httptest.NewRequest(http.MethodPost, "/mcp/context-store/"+pid, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	hn.r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("context init: %d %s", w.Code, w.Body.String())
	}
}

func doRaw(r *gin.Engine, method, path, body, cookie string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if cookie != "" {
		req.Header.Set("Cookie", cookie)
	}
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	return w
}
