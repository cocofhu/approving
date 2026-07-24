package services

import (
	"testing"

	"github.com/cocofhu/approving/internal/models"
)

func TestCurrentNodeIDsAndFailedError(t *testing.T) {
	db := newTestDB(t)
	s := NewRunService(db)
	g := models.Graph{Nodes: []models.Node{{ID: "a", Label: "A"}, {ID: "b", Label: "B"}}}
	db.Create(&models.Run{ID: "r1", Status: "running", Graph: g})
	db.Create(&models.Run{ID: "r2", Status: "waiting_human", Graph: g})
	db.Create(&models.Run{ID: "r3", Status: "failed", Graph: g})
	db.Create(&models.StateRun{RunID: "r1", NodeID: "a", Status: "running"})
	db.Create(&models.Gate{RunID: "r2", NodeID: "b", Title: "g"})
	db.Create(&models.StateRun{RunID: "r3", NodeID: "a", Status: "failed", Error: "boom"})

	ids := s.CurrentNodeIDs([]models.Run{
		{ID: "r1", Status: "running", Graph: g},
		{ID: "r2", Status: "waiting_human", Graph: g},
		{ID: "r3", Status: "failed", Graph: g},
	})
	if ids["r1"] != "a" || ids["r2"] != "b" {
		t.Fatalf("ids=%v", ids)
	}
	if _, ok := ids["r3"]; ok {
		t.Fatal("failed should omit")
	}
	if s.RunFailedError("r3") != "boom" {
		t.Fatalf("err=%q", s.RunFailedError("r3"))
	}
	if s.RunFailedError("missing") != "" {
		t.Fatal("missing")
	}
	if s.CurrentNodeIDs(nil) != nil {
		t.Fatal("empty")
	}
}

func TestPlatformRuleEmptyAgent(t *testing.T) {
	root := t.TempDir()
	svc, err := NewPlatformRuleService(root+"/g", root+"/p")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ListAgent(""); err == nil {
		t.Fatal("empty agent list")
	}
	if _, err := svc.GetAgent("", "test.md"); err == nil {
		t.Fatal("empty agent get")
	}
	if _, err := svc.SaveAgent("", "test.md", "x"); err == nil {
		t.Fatal("empty agent save")
	}
	if err := svc.DeleteAgent("", "test.md"); err == nil {
		t.Fatal("empty agent delete")
	}
	items, err := svc.ListAgent("Ag")
	if err != nil || len(items) == 0 {
		t.Fatalf("list agent: %v %d", err, len(items))
	}
}
