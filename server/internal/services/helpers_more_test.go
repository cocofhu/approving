package services

import (
	"testing"

	"github.com/cocofhu/approving/internal/models"
)

func TestEscapeLikeAndInboxHelpers(t *testing.T) {
	if got := escapeLikePattern(`a%b_c\d`); got == `a%b_c\d` {
		t.Fatalf("escapeLikePattern unchanged: %q", got)
	}
	db := newTestDB(t)
	s := NewRunService(db)
	g := models.Graph{Nodes: []models.Node{{ID: "r1", Type: "react", Label: "澄清"}}}
	db.Create(&models.Run{ID: "run-ctx", Status: "waiting_human", Graph: g})
	db.Create(&models.ReactConversation{RunID: "run-ctx", NodeID: "r1", Iteration: 1})
	db.Create(&models.Gate{RunID: "run-ctx", NodeID: "r1", Iteration: 1, Title: "g", Resolved: false})

	if _, _, ok := s.ClarifyContext("run-ctx", "r1", 1); !ok {
		t.Fatal("ClarifyContext")
	}
	if _, ok := s.PendingGateAt("run-ctx", "r1", 1); !ok {
		t.Fatal("PendingGateAt")
	}
	if ClarifyLabel(g, "r1") == "" {
		t.Fatal("ClarifyLabel")
	}
	if _, _, ok := s.ClarifyContext("missing", "r1", 1); ok {
		t.Fatal("missing clarify")
	}
}

func TestArtifactAllPage(t *testing.T) {
	db := newTestDB(t)
	arts := NewArtifactService(db)
	db.Create(&models.Run{ID: "r-art", WorkflowID: "wf", WorkflowName: "W", Status: "succeeded"})
	if _, err := arts.Save("r-art", "n", "a.md", "markdown", "hi"); err != nil {
		t.Fatal(err)
	}
	page, total := arts.AllPage("wf", "", 1, 10, "")
	if total < 1 || len(page) < 1 {
		t.Fatalf("AllPage: total=%d n=%d", total, len(page))
	}
	page, total = arts.AllPage("wf", "", 1, 10, "a_")
	_ = page
	_ = total
}
