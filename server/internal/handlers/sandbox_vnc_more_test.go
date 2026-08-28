package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/cocofhu/approving/internal/browser"
	"github.com/cocofhu/approving/internal/database"
	"github.com/cocofhu/approving/internal/mcp"
	"github.com/cocofhu/approving/internal/models"
	"github.com/cocofhu/approving/internal/sandbox"
	"github.com/cocofhu/approving/internal/sandbox/sandboxtest"
	"github.com/cocofhu/approving/internal/services"

	"github.com/gin-gonic/gin"
)

func TestSandboxVNCEarlyNilAndNotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := database.OpenSQLiteTest(t.TempDir() + "/vnc4.db")
	if err != nil {
		t.Fatal(err)
	}
	fg := sandboxtest.New(t)
	mgr := sandbox.NewManager(fg.Client(), sandbox.ManagerOptions{WorkspaceDir: "/root/workspace"})
	skills := services.NewAgentService(t.TempDir())
	hostMCP := mcp.NewHost(services.NewArtifactService(db))
	sbx := services.NewSandboxService(db, mgr, skills, hostMCP, services.SandboxOptions{Max: 2, TTL: time.Minute})
	bsvc := browser.New(&nopSandboxExec{}, browser.Config{})

	r := gin.New()
	hNilBrowser := &Handlers{Sbx: sbx}
	r.GET("/ws/sandboxes/:sandboxId/vnc-nil-browser", hNilBrowser.SandboxVNC)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/ws/sandboxes/1/vnc-nil-browser", nil))
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("nil browser: %d %s", w.Code, w.Body.String())
	}

	hNilSbx := &Handlers{Browser: bsvc}
	r.GET("/ws/sandboxes/:sandboxId/vnc-nil-sbx", hNilSbx.SandboxVNC)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/ws/sandboxes/1/vnc-nil-sbx", nil))
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("nil sbx: %d %s", w.Code, w.Body.String())
	}

	h := &Handlers{Browser: bsvc, Sbx: sbx}
	r.GET("/ws/sandboxes/:sandboxId/vnc", h.SandboxVNC)

	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/ws/sandboxes/bad/vnc", nil))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("bad id: %d %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/ws/sandboxes/99/vnc", nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("missing: %d %s", w.Code, w.Body.String())
	}

	// Empty name row → not found
	row := models.Sandbox{Name: "", Purpose: "test", Status: "running"}
	if err := db.Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/ws/sandboxes/"+utoa(row.ID)+"/vnc", nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("empty name: %d %s", w.Code, w.Body.String())
	}
}

func TestPreviewVNCNilDeps(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := &Handlers{}
	r.GET("/ws/preview/:runId/:nodeId/:port/vnc", h.PreviewVNC)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/ws/preview/r/n/3000/vnc", nil))
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("nil browser: %d %s", w.Code, w.Body.String())
	}
}

func TestHydrateHelpers(t *testing.T) {
	db, err := database.OpenSQLiteTest(t.TempDir() + "/hyd.db")
	if err != nil {
		t.Fatal(err)
	}
	arts := services.NewArtifactService(db)
	h := &Handlers{Arts: arts}

	if got := h.hydrateTestResultJSON("", "r1"); got != "" {
		t.Fatalf("empty raw: %q", got)
	}
	if got := h.hydrateTestResultJSON(`{"x":1}`, ""); got != `{"x":1}` {
		t.Fatalf("empty run: %q", got)
	}

	h.hydrateTestResultOutputs(nil, "r1")
	outs := map[string]any{"other": 1}
	h.hydrateTestResultOutputs(outs, "r1")
	if outs["other"] != 1 {
		t.Fatal("non-string field mutated")
	}
	outs["test_result_json"] = `{"screenshots":[{"artifact":"shot.png"}]}`
	if _, err := arts.Save("r1", "n1", "shot.png", "image", "img"); err != nil {
		t.Fatal(err)
	}
	h.hydrateTestResultOutputs(outs, "r1")
	hydrated, _ := outs["test_result_json"].(string)
	// Hydrate inlines artifact bytes as data/mimeType and drops the artifact name.
	if !strings.Contains(hydrated, `"data":"img"`) || !strings.Contains(hydrated, "image/png") {
		t.Fatalf("hydrate did not inline artifact content: %s", hydrated)
	}

	nodeExecs := map[string][]gin.H{
		"n1": {
			{"outputs": map[string]any{"test_result_json": `{"summary":"ok","screenshots":[{"artifact":"shot.png"}]}`}},
			{"outputs": "bad"},
			{"no": "outputs"},
		},
	}
	h.hydrateNodeExecutions(nodeExecs, "r1")
	outs0, _ := nodeExecs["n1"][0]["outputs"].(map[string]any)
	got, _ := outs0["test_result_json"].(string)
	if !strings.Contains(got, `"data":"img"`) {
		t.Fatalf("hydrateNodeExecutions did not inline artifact: %v", outs0)
	}
}
