package services

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/cocofhu/approving/internal/database"
	"github.com/cocofhu/approving/internal/models"
)

func TestRunListPage(t *testing.T) {
	db, err := database.OpenSQLiteTest(filepath.Join(t.TempDir(), "runs.db"))
	if err != nil {
		t.Fatal(err)
	}
	s := NewRunService(db)
	now := time.Now()
	for i, id := range []string{"r1", "r2", "r3"} {
		if err := db.Create(&models.Run{
			ID: id, WorkflowID: "wf", Status: "completed", StartedAt: now.Add(time.Duration(i) * time.Second),
		}).Error; err != nil {
			t.Fatal(err)
		}
	}
	page, total := s.ListPage(nil, "wf", "", 1, 2)
	if total != 3 || len(page) != 2 {
		t.Fatalf("page=%d total=%d", len(page), total)
	}
	page2, total2 := s.ListPage(nil, "wf", "", 2, 2)
	if total2 != 3 || len(page2) != 1 {
		t.Fatalf("page2=%d total2=%d", len(page2), total2)
	}
}
