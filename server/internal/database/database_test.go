package database

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/cocofhu/approving/internal/config"
	"github.com/cocofhu/approving/internal/models"
)

func TestOpenFileAndMigrate(t *testing.T) {
	db, err := OpenSQLite(filepath.Join(t.TempDir(), "x.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	// Every model table should be migrated.
	for _, m := range models.AllModels() {
		if !db.Migrator().HasTable(m) {
			t.Errorf("table missing for %T", m)
		}
	}
}

func TestOpenSQLiteTestClonesMigratedSchema(t *testing.T) {
	db, err := OpenSQLiteTest(filepath.Join(t.TempDir(), "clone.db"))
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	for _, m := range models.AllModels() {
		if !db.Migrator().HasTable(m) {
			t.Errorf("table missing for %T", m)
		}
	}
	if err := db.Create(&models.Run{ID: "r-clone", WorkflowID: "w", Status: "running"}).Error; err != nil {
		t.Fatalf("insert into cloned schema: %v", err)
	}
}

func TestOpenMemory(t *testing.T) {
	db, err := OpenSQLite(":memory:")
	if err != nil {
		t.Fatalf("open memory: %v", err)
	}
	if !db.Migrator().HasTable(&models.WorkflowDef{}) {
		t.Fatal("workflow table missing in memory db")
	}
}

func TestOpenWithExistingQueryAndBadPath(t *testing.T) {
	// A dsn that already carries query params skips the default-append branch.
	db, err := OpenSQLite(":memory:?cache=shared&_foreign_keys=on")
	if err != nil {
		t.Fatalf("open with query: %v", err)
	}
	if !db.Migrator().HasTable(&models.WorkflowDef{}) {
		t.Fatal("migrate failed for dsn with query")
	}
	// An unwritable path surfaces an open error rather than panicking.
	if _, err := OpenSQLite("/nonexistent-root-dir-xyz/nested/db.sqlite"); err == nil {
		t.Error("expected open error for unwritable path")
	}
}

func TestSeedNoOp(t *testing.T) {
	db, err := OpenSQLite(filepath.Join(t.TempDir(), "seed.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := Seed(db); err != nil {
		t.Fatalf("seed: %v", err)
	}
	var n int64
	db.Model(&models.WorkflowDef{}).Count(&n)
	if n != 0 {
		t.Fatalf("seed should not insert workflows, got %d", n)
	}
	if err := Seed(db); err != nil {
		t.Fatalf("seed again: %v", err)
	}
}

func TestOpenMySQLEmptyDSN(t *testing.T) {
	if _, err := Open(config.DatabaseConfig{Driver: "mysql", DSN: ""}); err == nil {
		t.Fatal("expected error for empty mysql DSN")
	}
	if _, err := Open(config.DatabaseConfig{Driver: "mysql", DSN: "   "}); err == nil {
		t.Fatal("expected error for whitespace mysql DSN")
	}
	if _, err := openMySQL("user:pass@tcp(127.0.0.1:1)/db?timeout=1s"); err == nil {
		t.Fatal("unreachable mysql should fail open")
	}
}

func TestOpenSQLiteDriver(t *testing.T) {
	db, err := Open(config.DatabaseConfig{Driver: "sqlite", Path: ":memory:"})
	if err != nil {
		t.Fatalf("open sqlite via Open: %v", err)
	}
	if !db.Migrator().HasTable(&models.WorkflowDef{}) {
		t.Fatal("workflow table missing")
	}
}

func TestBackfillWorkflowIDs(t *testing.T) {
	db, err := OpenSQLite(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	db.Create(&models.Run{ID: "r1", WorkflowID: "wf-1", Status: "completed", StartedAt: now})
	db.Create(&models.Gate{RunID: "r1", NodeID: "g1", Resolved: false, RequestedAt: now})
	db.Create(&models.Artifact{ID: "a1", RunID: "r1", Name: "doc", Content: "c", Kind: "markdown"})
	backfillWorkflowIDs(db)
	var gateWF, artWF string
	db.Raw("SELECT workflow_id FROM gates WHERE run_id = ?", "r1").Scan(&gateWF)
	db.Raw("SELECT workflow_id FROM artifacts WHERE id = ?", "a1").Scan(&artWF)
	if gateWF != "wf-1" || artWF != "wf-1" {
		t.Fatalf("backfill: gate=%q art=%q", gateWF, artWF)
	}
}

// TestUpgradeAddsProjectIDToExistingWorkflowDefs reproduces the preview PVC
// failure: an older SQLite file has workflow_defs rows but no project_id column.
// AutoMigrate must add the column and ensureDefaultProject must backfill it.
func TestUpgradeAddsProjectIDToExistingWorkflowDefs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	db, err := openSQLiteConn(path)
	if err != nil {
		t.Fatal(err)
	}
	err = db.Exec(`CREATE TABLE workflow_defs (
		id text PRIMARY KEY,
		name text,
		description text,
		status text,
		version integer,
		needs_repo numeric,
		graph text,
		created_at datetime,
		updated_at datetime,
		last_run_at datetime
	)`).Error
	if err != nil {
		t.Fatalf("create legacy table: %v", err)
	}
	err = db.Exec(`INSERT INTO workflow_defs (id, name, status, version, needs_repo, graph, created_at, updated_at)
		VALUES ('wf-legacy', 'Legacy', 'draft', 1, 0, '{}', datetime('now'), datetime('now'))`).Error
	if err != nil {
		t.Fatalf("insert legacy row: %v", err)
	}
	if err := closeGorm(db); err != nil {
		t.Fatal(err)
	}

	db2, err := OpenSQLite(path)
	if err != nil {
		t.Fatalf("upgrade migrate: %v", err)
	}
	var wf models.WorkflowDef
	if err := db2.First(&wf, "id = ?", "wf-legacy").Error; err != nil {
		t.Fatalf("load upgraded row: %v", err)
	}
	if wf.ProjectID != models.DefaultProjectID {
		t.Fatalf("expected project_id backfill %q, got %q", models.DefaultProjectID, wf.ProjectID)
	}
	var p models.Project
	if err := db2.Where("id = ?", models.DefaultProjectID).First(&p).Error; err != nil {
		t.Fatalf("default project missing after upgrade: %v", err)
	}
}
