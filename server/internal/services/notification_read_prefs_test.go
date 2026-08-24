package services

import (
	"path/filepath"
	"testing"

	"github.com/cocofhu/approving/internal/models"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupNotificationReadPrefsDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "prefs.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.NotificationReadPrefs{}); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestNotificationReadPrefsGetOrInitAndMark(t *testing.T) {
	db := setupNotificationReadPrefsDB(t)
	svc := NewNotificationReadPrefsService(db)

	first, err := svc.GetOrInit("alice")
	if err != nil {
		t.Fatal(err)
	}
	if first.EnabledAt == "" || len(first.ReadIDs) != 0 {
		t.Fatalf("init=%+v", first)
	}

	again, err := svc.GetOrInit("alice")
	if err != nil {
		t.Fatal(err)
	}
	if again.EnabledAt != first.EnabledAt {
		t.Fatalf("enabledAt changed on re-get: %s vs %s", again.EnabledAt, first.EnabledAt)
	}

	marked, err := svc.MarkRead("alice", "run-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(marked.ReadIDs) != 1 || marked.ReadIDs[0] != "run-1" {
		t.Fatalf("mark=%+v", marked)
	}

	// Idempotent
	marked2, err := svc.MarkRead("alice", "run-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(marked2.ReadIDs) != 1 {
		t.Fatalf("idempotent mark grew: %+v", marked2)
	}

	all, err := svc.MarkAllRead("alice", []string{"run-1", "run-2", "run-3"})
	if err != nil {
		t.Fatal(err)
	}
	if len(all.ReadIDs) != 3 {
		t.Fatalf("mark all=%+v", all)
	}
}

func TestNotificationReadPrefsUserIsolation(t *testing.T) {
	db := setupNotificationReadPrefsDB(t)
	svc := NewNotificationReadPrefsService(db)

	if _, err := svc.MarkRead("alice", "a"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.MarkRead("bob", "b"); err != nil {
		t.Fatal(err)
	}

	alice, err := svc.GetOrInit("alice")
	if err != nil {
		t.Fatal(err)
	}
	bob, err := svc.GetOrInit("bob")
	if err != nil {
		t.Fatal(err)
	}
	if len(alice.ReadIDs) != 1 || alice.ReadIDs[0] != "a" {
		t.Fatalf("alice=%+v", alice)
	}
	if len(bob.ReadIDs) != 1 || bob.ReadIDs[0] != "b" {
		t.Fatalf("bob=%+v", bob)
	}
}

func TestNotificationReadPrefsSurviveReopen(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "prefs.db")
	db1, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db1.AutoMigrate(&models.NotificationReadPrefs{}); err != nil {
		t.Fatal(err)
	}
	svc1 := NewNotificationReadPrefsService(db1)
	if _, err := svc1.MarkRead("alice", "persist-me"); err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db1.DB()
	_ = sqlDB.Close()

	// Simulate server restart: reopen same SQLite file.
	db2, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db2.AutoMigrate(&models.NotificationReadPrefs{}); err != nil {
		t.Fatal(err)
	}
	svc2 := NewNotificationReadPrefsService(db2)
	prefs, err := svc2.GetOrInit("alice")
	if err != nil {
		t.Fatal(err)
	}
	if len(prefs.ReadIDs) != 1 || prefs.ReadIDs[0] != "persist-me" {
		t.Fatalf("after reopen=%+v", prefs)
	}
}

func TestNotificationReadPrefsEmptyMarkAllDoesNotClear(t *testing.T) {
	db := setupNotificationReadPrefsDB(t)
	svc := NewNotificationReadPrefsService(db)
	if _, err := svc.MarkRead("alice", "kept"); err != nil {
		t.Fatal(err)
	}
	// Empty pool mark-all must not wipe existing readIds.
	prefs, err := svc.MarkAllRead("alice", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(prefs.ReadIDs) != 1 || prefs.ReadIDs[0] != "kept" {
		t.Fatalf("empty mark-all cleared: %+v", prefs)
	}
}

func TestNotificationReadPrefsValidation(t *testing.T) {
	db := setupNotificationReadPrefsDB(t)
	svc := NewNotificationReadPrefsService(db)
	if _, err := svc.GetOrInit("  "); err == nil {
		t.Fatal("expected username required")
	}
	if _, err := svc.MarkRead("alice", "  "); err == nil {
		t.Fatal("expected runId required")
	}
}
