package engine

import (
	"strings"
	"testing"

	"github.com/cocofhu/approving/internal/models"
)

func TestStartRunRejectsDeniedEnvWithoutCreatingRun(t *testing.T) {
	g := models.Graph{
		Nodes: []models.Node{{ID: "in", Type: "input"}, {ID: "out", Type: "output"}},
		Edges: []models.Edge{{ID: "e1", Source: "in", Target: "out"}},
	}
	eng, db := setupEngineGraph(t, g)

	_, err := eng.StartRunWithPriority("wf", nil, "test", "normal", nil, []models.EnvEntry{
		{Key: "CURSOR_API_KEY", Value: "x"},
		{Key: "LOG_LEVEL", Value: "debug"},
	})
	if err == nil || !strings.Contains(err.Error(), "CURSOR_API_KEY") {
		t.Fatalf("expected deny error, got %v", err)
	}
	var n int64
	if err := db.Model(&models.Run{}).Count(&n).Error; err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("run count=%d want 0", n)
	}
}

func TestStartRunSnapshotsSandboxEnv(t *testing.T) {
	g := models.Graph{
		Nodes: []models.Node{{ID: "in", Type: "input"}, {ID: "out", Type: "output"}},
		Edges: []models.Edge{{ID: "e1", Source: "in", Target: "out"}},
	}
	eng, db := setupEngineGraph(t, g)

	run, err := eng.StartRunWithPriority("wf", nil, "test", "normal", nil, []models.EnvEntry{
		{Key: "LOG_LEVEL", Value: "debug"},
		{Key: "DB_PASSWORD", Value: "s3cret", Secret: true},
		{Key: "", Value: ""},
	})
	if err != nil {
		t.Fatal(err)
	}
	var got models.Run
	if err := db.First(&got, "id = ?", run.ID).Error; err != nil {
		t.Fatal(err)
	}
	if len(got.SandboxEnv) != 2 {
		t.Fatalf("snapshot=%+v", got.SandboxEnv)
	}
	if got.SandboxEnv[0].Key != "LOG_LEVEL" || got.SandboxEnv[0].Value != "debug" {
		t.Fatalf("entry0=%+v", got.SandboxEnv[0])
	}
	if !got.SandboxEnv[1].Secret || got.SandboxEnv[1].Value != "s3cret" {
		t.Fatalf("secret entry=%+v", got.SandboxEnv[1])
	}
}

func TestStartRunNilEnvCompatible(t *testing.T) {
	g := models.Graph{
		Nodes: []models.Node{{ID: "in", Type: "input"}, {ID: "out", Type: "output"}},
		Edges: []models.Edge{{ID: "e1", Source: "in", Target: "out"}},
	}
	eng, _ := setupEngineGraph(t, g)
	run, err := eng.StartRunWithPriority("wf", nil, "test", "normal", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(run.SandboxEnv) != 0 {
		t.Fatalf("want empty snapshot, got %+v", run.SandboxEnv)
	}
}
