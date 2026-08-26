package services

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/cocofhu/approving/internal/database"
	"github.com/cocofhu/approving/internal/models"
)

func TestGlobalTokenStatsEmpty(t *testing.T) {
	db, err := database.OpenSQLiteTest(filepath.Join(t.TempDir(), "global_token_empty.db"))
	if err != nil {
		t.Fatal(err)
	}
	s := NewProjectService(db)
	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)

	res, err := s.GlobalTokenStats(context.Background(), GlobalTokenStatsQuery{
		Window:   TokenStatsWindow30d,
		Timezone: "Asia/Shanghai",
		Now:      now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Empty {
		t.Fatalf("expected empty, got total=%d", res.KPI.Total)
	}
}

func TestGlobalTokenStatsAggregation(t *testing.T) {
	db, err := database.OpenSQLiteTest(filepath.Join(t.TempDir(), "global_token_agg.db"))
	if err != nil {
		t.Fatal(err)
	}
	s := NewProjectService(db)
	must := func(v any) {
		t.Helper()
		if err := db.Create(v).Error; err != nil {
			t.Fatal(err)
		}
	}

	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	ptr := func(tt time.Time) *time.Time { return &tt }

	proj, err := s.Create("GlobalStats", "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	must(&models.WorkflowDef{ID: "wf-g", ProjectID: proj.ID, Name: "main", Status: "draft", Version: 1})
	dayIn := time.Date(2026, 7, 24, 10, 0, 0, 0, loc).UTC()
	must(&models.Run{ID: "run-g1", WorkflowID: "wf-g", WorkflowName: "main", Status: "completed", StartedAt: dayIn, Title: "Global Run"})
	must(&models.StateRun{
		RunID: "run-g1", NodeID: "n1", NodeType: "agent", Status: "completed",
		StartedAt: ptr(dayIn),
		Usage:     &models.TokenUsage{InputTokens: 100, OutputTokens: 50, CacheReadTokens: 10, CacheWriteTokens: 5},
	})

	res, err := s.GlobalTokenStats(context.Background(), GlobalTokenStatsQuery{
		Window:   TokenStatsWindow7d,
		Timezone: "Asia/Shanghai",
		Now:      now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Empty {
		t.Fatal("expected non-empty global stats")
	}
	if res.KPI.Total <= 0 {
		t.Fatalf("expected positive total, got %d", res.KPI.Total)
	}
	if len(res.Trend) == 0 {
		t.Fatal("expected trend buckets")
	}
	if len(res.Projects) == 0 {
		t.Fatal("expected project rows")
	}
	if len(res.TopRuns) == 0 || res.TopRuns[0].RunID != "run-g1" {
		t.Fatalf("expected top run run-g1, got %+v", res.TopRuns)
	}
}
