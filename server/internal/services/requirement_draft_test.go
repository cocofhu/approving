package services

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/cocofhu/approving/internal/models"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupRequirementDraftDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&models.Project{}, &models.RequirementDraft{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	now := time.Now()
	if err := db.Create(&models.Project{ID: "proj-1", Name: "P1", CreatedAt: now, UpdatedAt: now}).Error; err != nil {
		t.Fatalf("seed project: %v", err)
	}
	return db
}

func TestRequirementDraftCRUD(t *testing.T) {
	db := setupRequirementDraftDB(t)
	svc := NewRequirementDraftService(db)

	created, err := svc.Create("proj-1")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.Title != DefaultRequirementDraftTitle || created.Status != models.RequirementDraftStatusOpen || created.BodyMarkdown != "" {
		t.Fatalf("unexpected create: %+v", created)
	}

	saved, err := svc.UpdateContent("proj-1", created.ID, "  支付失败重试  ", "## 要点\n- a")
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if saved.Title != "支付失败重试" || saved.BodyMarkdown != "## 要点\n- a" {
		t.Fatalf("unexpected save: %+v", saved)
	}

	if _, err := svc.UpdateContent("proj-1", created.ID, "   ", ""); !errors.Is(err, ErrRequirementDraftEmptyTitle) {
		t.Fatalf("empty title err=%v", err)
	}

	done, err := svc.UpdateStatus("proj-1", created.ID, models.RequirementDraftStatusDone)
	if err != nil || done.Status != models.RequirementDraftStatusDone {
		t.Fatalf("status: %+v err=%v", done, err)
	}

	openList, err := svc.List("proj-1", RequirementDraftListFilter{Status: "open"})
	if err != nil || len(openList) != 0 {
		t.Fatalf("open list=%v err=%v", openList, err)
	}
	all, err := svc.List("proj-1", RequirementDraftListFilter{Status: "all", Query: "支付"})
	if err != nil || len(all) != 1 {
		t.Fatalf("search=%v err=%v", all, err)
	}

	if err := svc.Delete("proj-1", created.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := svc.Get("proj-1", created.ID); !errors.Is(err, ErrRequirementDraftNotFound) {
		t.Fatalf("get after delete: %v", err)
	}
}

func TestRequirementDraftProjectScopeAndCascade(t *testing.T) {
	db := setupRequirementDraftDB(t)
	svc := NewRequirementDraftService(db)
	projSvc := NewProjectService(db)

	d1, err := svc.Create("proj-1")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := svc.Get("missing", d1.ID); !errors.Is(err, ErrProjectNotFound) {
		t.Fatalf("cross/missing project: %v", err)
	}
	if _, err := svc.Create("missing"); !errors.Is(err, ErrProjectNotFound) {
		t.Fatalf("create missing project: %v", err)
	}

	if err := projSvc.Delete("proj-1"); err != nil {
		t.Fatalf("delete project: %v", err)
	}
	var n int64
	db.Model(&models.RequirementDraft{}).Where("project_id = ?", "proj-1").Count(&n)
	if n != 0 {
		t.Fatalf("cascade left %d drafts", n)
	}
}
