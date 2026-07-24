package database

import (
	"testing"
	"time"

	"github.com/cocofhu/approving/internal/models"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestBackfillLegacyProjectMemories(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:backfill_"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(models.AllModels()...); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if err := db.Create(&models.Project{
		ID: "p1", Name: "P", PmLeaderAgent: "pm-agent",
		SandboxEnv: []models.EnvEntry{}, Variables: []models.ProjectVariable{},
		CreatedAt: now, UpdatedAt: now,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.ProjectMemoryItem{
		ID: "m1", ProjectID: "p1", Title: "legacy", Content: "c", AgentName: "",
		CreatedAt: now, UpdatedAt: now,
	}).Error; err != nil {
		t.Fatal(err)
	}
	backfillLegacyProjectMemories(db)
	var item models.ProjectMemoryItem
	if err := db.First(&item, "id = ?", "m1").Error; err != nil {
		t.Fatal(err)
	}
	if item.AgentName != "pm-agent" {
		t.Fatalf("agent_name=%q", item.AgentName)
	}
}
