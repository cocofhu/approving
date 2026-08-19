package engine

import (
	"testing"

	"github.com/cocofhu/approving/internal/models"
	"github.com/cocofhu/approving/internal/services"
)

func TestSetReactPreviewArtifact(t *testing.T) {
	eng, db, _ := setupEngineGraphP(t, reactOnlyGraph())
	run, err := eng.StartRun("wf", nil, "test")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	waitReactPause(t, db, run.ID, "clarify")

	if err := eng.setReactPreviewArtifact(run.ID, "clarify", "page.html"); err != nil {
		t.Fatalf("pin preview: %v", err)
	}
	var conv models.ReactConversation
	if err := db.Where("run_id = ? AND node_id = ?", run.ID, "clarify").
		Order("iteration desc").First(&conv).Error; err != nil {
		t.Fatalf("load conv: %v", err)
	}
	if conv.PreviewArtifact != "page.html" {
		t.Fatalf("previewArtifact=%q", conv.PreviewArtifact)
	}
	pinnedTurns := len(conv.Messages)

	arts := services.NewArtifactService(db)
	if _, err := arts.Save(run.ID, "clarify", "page.html", "html", "<p>v1</p>"); err != nil {
		t.Fatal(err)
	}
	rec, ok := arts.GetRecord(run.ID, "page.html")
	if !ok || rec.Revision != 1 {
		t.Fatalf("first write revision=%d ok=%v", rec.Revision, ok)
	}

	if err := db.Where("run_id = ? AND node_id = ?", run.ID, "clarify").
		Order("iteration desc").First(&conv).Error; err != nil {
		t.Fatalf("reload conv: %v", err)
	}
	if len(conv.Messages) != pinnedTurns {
		t.Fatalf("pin clobbered messages: before=%d after=%d", pinnedTurns, len(conv.Messages))
	}

	if err := eng.setReactPreviewArtifact(run.ID, "missing", "page.html"); err == nil {
		t.Fatal("expected missing conversation to fail")
	}
	if err := eng.setReactPreviewArtifact(run.ID, "clarify", "   "); err == nil {
		t.Fatal("expected blank preview name to fail")
	}
	if err := eng.setReactPreviewArtifact(run.ID, "", "page.html"); err == nil {
		t.Fatal("expected empty node id to fail")
	}
}
