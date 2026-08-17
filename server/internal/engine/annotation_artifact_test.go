package engine

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/cocofhu/approving/internal/models"
)

func TestSaveAnnotationArtifactUpsertAndIsolation(t *testing.T) {
	eng, db := setupEngine(t)
	runID := "run-ann-art"
	now := time.Now()
	g := models.Graph{
		Nodes: []models.Node{
			{ID: "visual", Type: "visual"},
			{ID: "gate", Type: "human_gate", Config: map[string]any{
				"title":         "审阅",
				"body_template": "{{nodes.visual.outputs.page}}",
				"actions":       []any{map[string]any{"id": "pass", "label": "通过"}},
			}},
		},
	}
	if err := db.Create(&models.Run{
		ID: runID, WorkflowID: "w", WorkflowName: "w", Status: "waiting_human",
		Graph: g, StartedAt: now, CreatedAt: now,
	}).Error; err != nil {
		t.Fatal(err)
	}
	db.Create(&models.StateRun{
		RunID: runID, NodeID: "gate", NodeType: "human_gate", Iteration: 1, Status: "waiting_human",
		Outputs: map[string]any{},
	})
	db.Create(&models.Gate{
		RunID: runID, NodeID: "gate", Iteration: 1, Title: "审阅", RequestedAt: now,
		UpstreamNodeID: "visual", UpstreamIteration: 1,
		Actions: []models.GateAction{{ID: "pass", Label: "通过"}},
	})

	doc := AnnotationArtifactDoc{
		Annotations: []AnnotationPinEntry{
			{
				Seq: 1, Selector: "div.hero", Comment: "标题偏大",
				CurrentText: "定价", Screenshot: "MISSING", MarkKind: "click",
			},
			{
				Seq: 2, Selector: "div.hero", Comment: "再标一条",
				Screenshot: "present", ImageDataUrl: "data:image/png;base64,aaa",
				MarkKind: "click",
				Bounds:   &AnnotationBounds{Left: 10, Top: 20, Width: 100, Height: 40},
			},
		},
	}
	res, err := eng.SaveAnnotationArtifact(runID, "gate", doc)
	if err != nil || res == nil {
		t.Fatalf("save: res=%+v err=%v", res, err)
	}
	if res.Name != PreviewAnnotationsArtifactName || res.Kind != PreviewAnnotationsKind {
		t.Fatalf("meta name=%s kind=%s", res.Name, res.Kind)
	}
	content, ok := eng.store.Get(runID, PreviewAnnotationsArtifactName)
	if !ok || !strings.Contains(content, `"hardScope"`) || !strings.Contains(content, AnnotationHardScope) {
		t.Fatalf("store missing hardScope: %q", content)
	}
	if !strings.Contains(content, `"screenshot": "MISSING"`) {
		t.Fatalf("expected MISSING marker: %q", content)
	}
	if !strings.Contains(content, `"route": "artifact_only"`) {
		t.Fatalf("expected artifact_only route: %q", content)
	}

	// Vars synced for downstream.
	var rv models.RunVariable
	if err := db.Where("run_id = ? AND name = ?", runID, "preview_annotations").First(&rv).Error; err != nil {
		t.Fatalf("vars.preview_annotations: %v", err)
	}
	raw, _ := json.Marshal(rv.Value)
	if !strings.Contains(string(raw), "div.hero") {
		t.Fatalf("var value=%s", raw)
	}

	// Must not create PreviewIssue rows.
	var issueCount int64
	db.Model(&models.PreviewIssue{}).Where("run_id = ?", runID).Count(&issueCount)
	if issueCount != 0 {
		t.Fatalf("preview issues created: %d", issueCount)
	}

	// Cover write replaces prior package.
	doc2 := AnnotationArtifactDoc{
		Annotations: []AnnotationPinEntry{
			{Seq: 3, Selector: "footer", Comment: "仅保留这条", Screenshot: "MISSING", MarkKind: "click"},
		},
	}
	res2, err := eng.SaveAnnotationArtifact(runID, "gate", doc2)
	if err != nil || res2 == nil {
		t.Fatalf("cover save: %v", err)
	}
	content2, _ := eng.store.Get(runID, PreviewAnnotationsArtifactName)
	if strings.Contains(content2, "标题偏大") || !strings.Contains(content2, "仅保留这条") {
		t.Fatalf("cover write failed: %q", content2)
	}
	var countVar models.RunVariable
	if err := db.Where("run_id = ? AND name = ?", runID, "preview_annotations_count").First(&countVar).Error; err != nil {
		t.Fatal(err)
	}
	if n, _ := countVar.Value.(float64); n != 1 && countVar.Value != 1 {
		// sqlite/json may decode numbers as float64
		if f, ok := countVar.Value.(float64); !ok || f != 1 {
			if i, ok := countVar.Value.(int); !ok || i != 1 {
				t.Fatalf("count var=%v (%T)", countVar.Value, countVar.Value)
			}
		}
	}

	// Reject empty package.
	if _, err := eng.SaveAnnotationArtifact(runID, "gate", AnnotationArtifactDoc{}); err == nil {
		t.Fatal("empty annotations should fail")
	}
	// Reject missing selector.
	if _, err := eng.SaveAnnotationArtifact(runID, "gate", AnnotationArtifactDoc{
		Annotations: []AnnotationPinEntry{{Seq: 1, Comment: "x", Screenshot: "MISSING"}},
	}); err == nil {
		t.Fatal("empty selector should fail")
	}

	// Primary whitelist still rejects this name via SaveGateArtifact.
	if _, err := eng.SaveGateArtifact(runID, "gate", PreviewAnnotationsArtifactName, `{}`, ""); err == nil {
		t.Fatal("SaveGateArtifact must not allow annotation sidecar via primary path")
	}
}

func TestSaveAnnotationArtifactRequiresPendingGate(t *testing.T) {
	eng, db := setupEngine(t)
	runID := "run-ann-ended"
	now := time.Now()
	g := models.Graph{Nodes: []models.Node{{ID: "gate", Type: "human_gate"}}}
	db.Create(&models.Run{ID: runID, WorkflowID: "w", WorkflowName: "w", Status: "completed", Graph: g, StartedAt: now})
	doc := AnnotationArtifactDoc{
		Annotations: []AnnotationPinEntry{
			{Seq: 1, Selector: "a", Comment: "x", Screenshot: "MISSING", MarkKind: "click"},
		},
	}
	if _, err := eng.SaveAnnotationArtifact(runID, "gate", doc); err == nil {
		t.Fatal("completed run should reject")
	}
}
