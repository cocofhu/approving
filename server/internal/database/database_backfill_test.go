package database

import (
	"reflect"
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

func TestBackfillChannelEnabledMcpsFromProject(t *testing.T) {
	// plan g1.4 / review v1: legacy Channel.EnabledMcps=nil must snapshot
	// Project.PmEnabledMcps so channel turns keep the tightened MCP set.
	db, err := gorm.Open(sqlite.Open("file:ch_mcp_"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(models.AllModels()...); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if err := db.Create(&models.Project{
		ID: "p-mcp", Name: "P", PmLeaderAgent: "pm-agent",
		PmEnabledMcps: []string{"pm-progress"},
		SandboxEnv:    []models.EnvEntry{}, Variables: []models.ProjectVariable{},
		CreatedAt: now, UpdatedAt: now,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.ChannelConfig{
		ID: "chn-legacy", Type: models.ChannelTypeQQ, Name: "QQ",
		Enabled: true, ProjectID: "p-mcp", AgentName: "", IsPrimary: false,
		EnabledMcps: nil, AppID: "app-legacy",
		CreatedAt: now, UpdatedAt: now,
	}).Error; err != nil {
		t.Fatal(err)
	}
	backfillChannelPrimaryAndAgent(db)

	var ch models.ChannelConfig
	if err := db.First(&ch, "id = ?", "chn-legacy").Error; err != nil {
		t.Fatal(err)
	}
	if !ch.IsPrimary {
		t.Fatal("expected primary backfill")
	}
	if ch.AgentName != "pm-agent" {
		t.Fatalf("agent=%q", ch.AgentName)
	}
	if !reflect.DeepEqual(ch.EnabledMcps, []string{"pm-progress"}) {
		t.Fatalf("enabled_mcps=%v want [pm-progress]", ch.EnabledMcps)
	}

	// Explicit empty project list must persist as empty (not defaults).
	if err := db.Create(&models.Project{
		ID: "p-empty", Name: "E", PmLeaderAgent: "pm-b",
		PmEnabledMcps: []string{},
		SandboxEnv:    []models.EnvEntry{}, Variables: []models.ProjectVariable{},
		CreatedAt: now, UpdatedAt: now,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.ChannelConfig{
		ID: "chn-empty", Type: models.ChannelTypeQQ, Name: "QQ2",
		Enabled: true, ProjectID: "p-empty", AgentName: "pm-b", IsPrimary: true,
		EnabledMcps: nil, AppID: "app-empty",
		CreatedAt: now, UpdatedAt: now,
	}).Error; err != nil {
		t.Fatal(err)
	}
	backfillChannelPrimaryAndAgent(db)
	var chEmpty models.ChannelConfig
	if err := db.First(&chEmpty, "id = ?", "chn-empty").Error; err != nil {
		t.Fatal(err)
	}
	if chEmpty.EnabledMcps == nil || len(chEmpty.EnabledMcps) != 0 {
		t.Fatalf("empty project mcps: got %#v", chEmpty.EnabledMcps)
	}

	// Project nil → channel stays nil (Effective = defaults).
	if err := db.Create(&models.Project{
		ID: "p-nil", Name: "N", PmLeaderAgent: "pm-c",
		PmEnabledMcps: nil,
		SandboxEnv:    []models.EnvEntry{}, Variables: []models.ProjectVariable{},
		CreatedAt: now, UpdatedAt: now,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.ChannelConfig{
		ID: "chn-nil", Type: models.ChannelTypeQQ, Name: "QQ3",
		Enabled: true, ProjectID: "p-nil", AgentName: "pm-c", IsPrimary: true,
		EnabledMcps: nil, AppID: "app-nil",
		CreatedAt: now, UpdatedAt: now,
	}).Error; err != nil {
		t.Fatal(err)
	}
	backfillChannelPrimaryAndAgent(db)
	var chNil models.ChannelConfig
	if err := db.First(&chNil, "id = ?", "chn-nil").Error; err != nil {
		t.Fatal(err)
	}
	if chNil.EnabledMcps != nil {
		t.Fatalf("nil project should leave channel nil, got %#v", chNil.EnabledMcps)
	}
}
