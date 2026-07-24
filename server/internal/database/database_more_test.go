package database

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cocofhu/approving/internal/models"
)

func TestOpenSQLiteTestBadParent(t *testing.T) {
	base := t.TempDir()
	blocker := filepath.Join(base, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenSQLiteTest(filepath.Join(blocker, "nested.db")); err == nil {
		t.Fatal("expected mkdir failure when parent is a file")
	}
}

func TestEnsureDefaultProjectUsesOldestWhenDefaultMissing(t *testing.T) {
	db, err := OpenSQLite(filepath.Join(t.TempDir(), "oldest.db"))
	if err != nil {
		t.Fatal(err)
	}
	db.Exec("DELETE FROM projects")
	now := time.Now()
	if err := db.Create(&models.Project{
		ID: "custom-oldest", Name: "Oldest", SandboxEnv: []models.EnvEntry{},
		Variables: []models.ProjectVariable{}, CreatedAt: now.Add(-time.Hour), UpdatedAt: now,
	}).Error; err != nil {
		t.Fatal(err)
	}
	db.Exec(`INSERT INTO workflow_defs (id, name, status, version, needs_repo, graph, project_id, created_at, updated_at)
		VALUES ('wf-empty-pid', 'W', 'draft', 1, 0, '{}', '', datetime('now'), datetime('now'))`)
	ensureDefaultProject(db)
	var pid string
	db.Raw("SELECT project_id FROM workflow_defs WHERE id = ?", "wf-empty-pid").Scan(&pid)
	if pid != "custom-oldest" {
		t.Fatalf("expected backfill to oldest project, got %q", pid)
	}
}
