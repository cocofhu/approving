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
	if res.Projects[0].CacheReadTokens != 10 || res.Projects[0].CacheWriteTokens != 5 {
		t.Fatalf("expected project cache tokens 10/5, got %d/%d", res.Projects[0].CacheReadTokens, res.Projects[0].CacheWriteTokens)
	}
	if len(res.TopRuns) == 0 || res.TopRuns[0].RunID != "run-g1" {
		t.Fatalf("expected top run run-g1, got %+v", res.TopRuns)
	}
}

func TestGlobalTokenStatsModelRebucketByDefaultModel(t *testing.T) {
	db, err := database.OpenSQLiteTest(filepath.Join(t.TempDir(), "global_token_rebucket.db"))
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

	proj, err := s.Create("Approving", "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Update(proj.ID, nil, nil, nil, nil, nil, aliasPtr("kimi-k3")); err != nil {
		t.Fatal(err)
	}
	must(&models.WorkflowDef{ID: "wf-rb", ProjectID: proj.ID, Name: "main", Status: "draft", Version: 1})
	dayIn := time.Date(2026, 7, 24, 10, 0, 0, 0, loc).UTC()
	must(&models.Run{ID: "run-rb1", WorkflowID: "wf-rb", WorkflowName: "main", Status: "completed", StartedAt: dayIn, Title: "Rebucket Run"})
	must(&models.StateRun{
		RunID: "run-rb1", NodeID: "n1", NodeType: "agent", Status: "completed",
		StartedAt: ptr(dayIn),
		Usage:     &models.TokenUsage{InputTokens: 60, OutputTokens: 40},
		UsageByModel: models.TokenUsageByModel{
			models.TokenUsageModelUnknown: {InputTokens: 60, OutputTokens: 40, Source: models.TokenUsageSourceUnknown},
		},
	})
	must(&models.StateRun{
		RunID: "run-rb1", NodeID: "n2", NodeType: "agent", Status: "completed",
		StartedAt: ptr(dayIn),
		Usage:     &models.TokenUsage{InputTokens: 100, OutputTokens: 50},
		UsageByModel: models.TokenUsageByModel{
			"kimi-k3": {InputTokens: 100, OutputTokens: 50, Filled: true, Source: models.TokenUsageSourceUpstream},
		},
	})

	res, err := s.GlobalTokenStats(context.Background(), GlobalTokenStatsQuery{
		Window:   TokenStatsWindow7d,
		Timezone: "Asia/Shanghai",
		Now:      now,
	})
	if err != nil {
		t.Fatal(err)
	}
	var kimiTotal int64
	var hasUnknown bool
	for _, m := range res.ModelRanking {
		if m.ModelKey == "kimi-k3" {
			kimiTotal = m.Total
		}
		if m.ModelKey == models.TokenUsageModelUnknown {
			hasUnknown = true
		}
	}
	if kimiTotal != 250 {
		t.Fatalf("expected kimi-k3 total 250 after rebucket, got %d", kimiTotal)
	}
	if hasUnknown {
		t.Fatal("expected no unknown bucket when default model is configured")
	}
	if res.KPI.Total != 250 {
		t.Fatalf("expected KPI total unchanged at 250, got %d", res.KPI.Total)
	}
}

func TestGlobalTokenStatsModelFilterAfterRebucket(t *testing.T) {
	db, err := database.OpenSQLiteTest(filepath.Join(t.TempDir(), "global_token_rebucket_filter.db"))
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

	proj, err := s.Create("Approving", "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Update(proj.ID, nil, nil, nil, nil, nil, aliasPtr("kimi-k3")); err != nil {
		t.Fatal(err)
	}
	must(&models.WorkflowDef{ID: "wf-rbf", ProjectID: proj.ID, Name: "main", Status: "draft", Version: 1})
	dayIn := time.Date(2026, 7, 24, 10, 0, 0, 0, loc).UTC()
	must(&models.Run{ID: "run-rbf1", WorkflowID: "wf-rbf", WorkflowName: "main", Status: "completed", StartedAt: dayIn, Title: "Rebucket Filter Run"})
	must(&models.StateRun{
		RunID: "run-rbf1", NodeID: "n1", NodeType: "agent", Status: "completed",
		StartedAt: ptr(dayIn),
		Usage:     &models.TokenUsage{InputTokens: 60, OutputTokens: 40},
		UsageByModel: models.TokenUsageByModel{
			models.TokenUsageModelUnknown: {InputTokens: 60, OutputTokens: 40, Source: models.TokenUsageSourceUnknown},
		},
	})
	must(&models.StateRun{
		RunID: "run-rbf1", NodeID: "n2", NodeType: "agent", Status: "completed",
		StartedAt: ptr(dayIn),
		Usage:     &models.TokenUsage{InputTokens: 100, OutputTokens: 50},
		UsageByModel: models.TokenUsageByModel{
			"kimi-k3": {InputTokens: 100, OutputTokens: 50, Filled: true, Source: models.TokenUsageSourceUpstream},
		},
	})

	unfiltered, err := s.GlobalTokenStats(context.Background(), GlobalTokenStatsQuery{
		Window:   TokenStatsWindow7d,
		Timezone: "Asia/Shanghai",
		Now:      now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if unfiltered.KPI.Total != 250 {
		t.Fatalf("expected unfiltered KPI total 250, got %d", unfiltered.KPI.Total)
	}

	filtered, err := s.GlobalTokenStats(context.Background(), GlobalTokenStatsQuery{
		Window:   TokenStatsWindow7d,
		Timezone: "Asia/Shanghai",
		ModelKey: "kimi-k3",
		Now:      now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if filtered.KPI.Total != 250 {
		t.Fatalf("expected filtered kimi-k3 KPI total 250, got %d", filtered.KPI.Total)
	}
	var kimiTotal int64
	for _, m := range filtered.ModelRanking {
		if m.ModelKey == "kimi-k3" {
			kimiTotal = m.Total
		}
	}
	if kimiTotal != 250 {
		t.Fatalf("expected filtered kimi-k3 ranking total 250, got %d", kimiTotal)
	}
}

func TestGlobalTokenStats24hPrevWindowDelta(t *testing.T) {
	db, err := database.OpenSQLiteTest(filepath.Join(t.TempDir(), "global_token_24h.db"))
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
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	ptr := func(tt time.Time) *time.Time { return &tt }
	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC) // 20:00 Shanghai
	proj, err := s.Create("G24h", "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	must(&models.WorkflowDef{ID: "wf-g24", ProjectID: proj.ID, Name: "main", Status: "draft", Version: 1})

	// Previous 24h: [Jul 23 20:00, Jul 24 20:00) Shanghai — 100 tokens at Jul 24 10:00.
	prevTS := time.Date(2026, 7, 24, 10, 0, 0, 0, loc).UTC()
	must(&models.Run{ID: "run-prev", WorkflowID: "wf-g24", WorkflowName: "main", Status: "completed", StartedAt: prevTS, Title: "Prev"})
	must(&models.StateRun{
		RunID: "run-prev", NodeID: "n1", NodeType: "agent", Status: "completed",
		StartedAt: ptr(prevTS),
		Usage:     &models.TokenUsage{InputTokens: 100},
	})
	// Current 24h: [Jul 24 20:00, Jul 25 20:00] — 200 tokens at Jul 25 08:00.
	curTS := time.Date(2026, 7, 25, 8, 0, 0, 0, loc).UTC()
	must(&models.Run{ID: "run-cur", WorkflowID: "wf-g24", WorkflowName: "main", Status: "completed", StartedAt: curTS, Title: "Cur"})
	must(&models.StateRun{
		RunID: "run-cur", NodeID: "n1", NodeType: "agent", Status: "completed",
		StartedAt: ptr(curTS),
		Usage:     &models.TokenUsage{InputTokens: 200},
	})

	res, err := s.GlobalTokenStats(context.Background(), GlobalTokenStatsQuery{
		Window:   TokenStatsWindow24h,
		Timezone: "Asia/Shanghai",
		Now:      now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Empty {
		t.Fatal("expected non-empty")
	}
	if res.Window != TokenStatsWindow24h || res.BucketWidth != TokenStatsBucketHour {
		t.Fatalf("window=%s bucketWidth=%s", res.Window, res.BucketWidth)
	}
	if res.KPI.Total != 200 {
		t.Fatalf("kpi.total=%d want 200", res.KPI.Total)
	}
	if res.KPI.PrevTotal == nil || *res.KPI.PrevTotal != 100 {
		t.Fatalf("kpi.prevTotal=%v want 100", res.KPI.PrevTotal)
	}
	if res.KPI.DeltaPct == nil || *res.KPI.DeltaPct != 100 {
		t.Fatalf("kpi.deltaPct=%v want 100", res.KPI.DeltaPct)
	}
	if len(res.Trend) != 25 {
		t.Fatalf("trend len=%d want 25 hour buckets", len(res.Trend))
	}
	if len(res.PrevTrend) != 25 {
		t.Fatalf("prevTrend len=%d want 25", len(res.PrevTrend))
	}
}

func TestGlobalTokenStats24hEmptyWindow(t *testing.T) {
	db, err := database.OpenSQLiteTest(filepath.Join(t.TempDir(), "global_token_24h_empty.db"))
	if err != nil {
		t.Fatal(err)
	}
	s := NewProjectService(db)
	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	if _, err := s.Create("EmptyG24h", "", nil, nil); err != nil {
		t.Fatal(err)
	}
	res, err := s.GlobalTokenStats(context.Background(), GlobalTokenStatsQuery{
		Window:   TokenStatsWindow24h,
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

func TestGlobalTokenStatsOmitsWindowDefaultsAll(t *testing.T) {
	db, err := database.OpenSQLiteTest(filepath.Join(t.TempDir(), "global_token_omit_window.db"))
	if err != nil {
		t.Fatal(err)
	}
	s := NewProjectService(db)
	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	res, err := s.GlobalTokenStats(context.Background(), GlobalTokenStatsQuery{
		Timezone: "UTC",
		Now:      now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Window != TokenStatsWindowAll {
		t.Fatalf("omitted window want all, got %s", res.Window)
	}
	if res.BucketWidth != TokenStatsBucketWeek {
		t.Fatalf("all should use week buckets, got %s", res.BucketWidth)
	}
}

func aliasPtr(s string) *string { return &s }
