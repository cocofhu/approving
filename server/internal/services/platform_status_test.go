package services

import (
	"context"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cocofhu/approving/internal/database"
	"github.com/cocofhu/approving/internal/models"
)

func TestPlatformStatus_emptyNullAndTrueZero(t *testing.T) {
	db, err := database.OpenSQLiteTest(filepath.Join(t.TempDir(), "platform_status_empty.db"))
	if err != nil {
		t.Fatal(err)
	}
	projects := NewProjectService(db)
	dash := NewDashboardService(db, projects)

	loc := time.FixedZone("UTC+8", 8*3600)
	now := time.Date(2026, 8, 12, 14, 7, 0, 0, loc)

	got, err := dash.PlatformStatus(context.Background(), PlatformStatusQuery{
		UTCOffsetMinutes: intPtr(8 * 60),
		Now:              now.UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.CumulativeTokens != nil || got.Current5mBucketTokens != nil || got.TodayMaxCompleted5mTokens != nil {
		t.Fatalf("want null token fields, got cum=%v cur=%v peak=%v",
			got.CumulativeTokens, got.Current5mBucketTokens, got.TodayMaxCompleted5mTokens)
	}
	if got.RunningCount != 0 || got.QueuedCount != 0 {
		t.Fatalf("want running/queued 0, got %d/%d", got.RunningCount, got.QueuedCount)
	}

	mustCreate := func(v any) {
		t.Helper()
		if err := db.Create(v).Error; err != nil {
			t.Fatal(err)
		}
	}
	mustCreate(&models.Run{ID: "r-run", Status: "running", StartedAt: now.UTC(), CreatedAt: now.UTC()})
	mustCreate(&models.Run{ID: "r-q", Status: "queued", CreatedAt: now.UTC()})
	mustCreate(&models.Run{ID: "r-wh", Status: "waiting_human", StartedAt: now.UTC(), CreatedAt: now.UTC()})

	dash.ClearPlatformStatusCacheForTest()
	got, err = dash.PlatformStatus(context.Background(), PlatformStatusQuery{
		UTCOffsetMinutes: intPtr(8 * 60),
		Now:              now.UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.RunningCount != 1 || got.QueuedCount != 1 {
		t.Fatalf("running/queued want 1/1 (waiting_human excluded), got %d/%d", got.RunningCount, got.QueuedCount)
	}
}

func TestPlatformStatus_fiveMinuteBucketsAndPeak(t *testing.T) {
	db, err := database.OpenSQLiteTest(filepath.Join(t.TempDir(), "platform_status_5m.db"))
	if err != nil {
		t.Fatal(err)
	}
	projects := NewProjectService(db)
	dash := NewDashboardService(db, projects)

	loc := time.FixedZone("UTC+8", 8*3600)
	// Current incomplete bucket: 14:05–14:10; now = 14:07
	now := time.Date(2026, 8, 12, 14, 7, 0, 0, loc)
	mustCreate := func(v any) {
		t.Helper()
		if err := db.Create(v).Error; err != nil {
			t.Fatal(err)
		}
	}

	p, err := projects.Create("P", "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	mustCreate(&models.WorkflowDef{ID: "wf1", ProjectID: p.ID, Name: "w", Status: "draft", Version: 1})

	// Completed bucket 11:20–11:25 → peak candidate 12104
	peakStart := time.Date(2026, 8, 12, 11, 22, 0, 0, loc)
	mustCreate(&models.Run{ID: "run-peak", WorkflowID: "wf1", Status: "completed", StartedAt: peakStart.UTC(), CreatedAt: peakStart.UTC()})
	srPeak := peakStart.UTC()
	mustCreate(&models.StateRun{
		RunID: "run-peak", NodeID: "n1", Status: "completed",
		Usage: &models.TokenUsage{InputTokens: 12104}, StartedAt: &srPeak,
	})

	// Smaller completed bucket 10:00–10:05
	smallStart := time.Date(2026, 8, 12, 10, 1, 0, 0, loc)
	mustCreate(&models.Run{ID: "run-small", WorkflowID: "wf1", Status: "completed", StartedAt: smallStart.UTC(), CreatedAt: smallStart.UTC()})
	srSmall := smallStart.UTC()
	mustCreate(&models.StateRun{
		RunID: "run-small", NodeID: "n1", Status: "completed",
		Usage: &models.TokenUsage{InputTokens: 100}, StartedAt: &srSmall,
	})

	// Current bucket 14:05–14:10 → 4812 (must NOT be peak)
	curStart := time.Date(2026, 8, 12, 14, 6, 0, 0, loc)
	mustCreate(&models.Run{ID: "run-cur", WorkflowID: "wf1", Status: "running", StartedAt: curStart.UTC(), CreatedAt: curStart.UTC()})
	srCur := curStart.UTC()
	mustCreate(&models.StateRun{
		RunID: "run-cur", NodeID: "n1", Status: "running",
		Usage: &models.TokenUsage{InputTokens: 4812}, StartedAt: &srCur,
	})

	// Yesterday usage should not affect today peak
	yest := time.Date(2026, 8, 11, 15, 0, 0, 0, loc)
	mustCreate(&models.Run{ID: "run-yest", WorkflowID: "wf1", Status: "completed", StartedAt: yest.UTC(), CreatedAt: yest.UTC()})
	srY := yest.UTC()
	mustCreate(&models.StateRun{
		RunID: "run-yest", NodeID: "n1", Status: "completed",
		Usage: &models.TokenUsage{InputTokens: 999999}, StartedAt: &srY,
	})

	got, err := dash.PlatformStatus(context.Background(), PlatformStatusQuery{
		UTCOffsetMinutes: intPtr(8 * 60),
		Now:              now.UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.CumulativeTokens == nil || *got.CumulativeTokens != 12104+100+4812+999999 {
		t.Fatalf("cumulative=%v", got.CumulativeTokens)
	}
	if got.Current5mBucketTokens == nil || *got.Current5mBucketTokens != 4812 {
		t.Fatalf("current5m=%v want 4812", got.Current5mBucketTokens)
	}
	if got.TodayMaxCompleted5mTokens == nil || *got.TodayMaxCompleted5mTokens != 12104 {
		t.Fatalf("peak=%v want 12104", got.TodayMaxCompleted5mTokens)
	}
	if got.CurrentBucketStart == nil || got.CurrentBucketEnd == nil {
		t.Fatal("expected current bucket bounds")
	}
	wantCurStart := time.Date(2026, 8, 12, 14, 5, 0, 0, loc)
	if !got.CurrentBucketStart.Equal(wantCurStart) {
		t.Fatalf("current start=%v want %v", got.CurrentBucketStart, wantCurStart)
	}
	if got.PeakBucketStart == nil {
		t.Fatal("expected peak bucket start")
	}
	wantPeakStart := time.Date(2026, 8, 12, 11, 20, 0, 0, loc)
	if !got.PeakBucketStart.In(loc).Equal(wantPeakStart) {
		t.Fatalf("peak start=%v want %v", got.PeakBucketStart.In(loc), wantPeakStart)
	}
}

func TestPlatformStatus_crossDayPeakReset(t *testing.T) {
	db, err := database.OpenSQLiteTest(filepath.Join(t.TempDir(), "platform_status_day.db"))
	if err != nil {
		t.Fatal(err)
	}
	projects := NewProjectService(db)
	dash := NewDashboardService(db, projects)
	loc := time.FixedZone("UTC+8", 8*3600)
	mustCreate := func(v any) {
		t.Helper()
		if err := db.Create(v).Error; err != nil {
			t.Fatal(err)
		}
	}

	p, err := projects.Create("P", "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	mustCreate(&models.WorkflowDef{ID: "wf1", ProjectID: p.ID, Name: "w", Status: "draft", Version: 1})

	day1 := time.Date(2026, 8, 11, 12, 2, 0, 0, loc)
	mustCreate(&models.Run{ID: "run-d1", WorkflowID: "wf1", Status: "completed", StartedAt: day1.UTC(), CreatedAt: day1.UTC()})
	sr1 := day1.UTC()
	mustCreate(&models.StateRun{
		RunID: "run-d1", NodeID: "n1", Status: "completed",
		Usage: &models.TokenUsage{InputTokens: 5000}, StartedAt: &sr1,
	})

	// New day 00:07 — no completed bucket yet today → peak null
	day2 := time.Date(2026, 8, 12, 0, 7, 0, 0, loc)
	got, err := dash.PlatformStatus(context.Background(), PlatformStatusQuery{
		UTCOffsetMinutes: intPtr(8 * 60),
		Now:              day2.UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.TodayMaxCompleted5mTokens != nil {
		t.Fatalf("cross-day peak should reset to null, got %v", *got.TodayMaxCompleted5mTokens)
	}
	// Current bucket 00:05–00:10 still empty → 0 (cumulative exists)
	if got.Current5mBucketTokens == nil || *got.Current5mBucketTokens != 0 {
		t.Fatalf("current bucket want 0, got %v", got.Current5mBucketTokens)
	}
}

func TestPlatformStatus_cacheHitSkipsRescan(t *testing.T) {
	db, err := database.OpenSQLiteTest(filepath.Join(t.TempDir(), "platform_status_cache.db"))
	if err != nil {
		t.Fatal(err)
	}
	projects := NewProjectService(db)
	dash := NewDashboardService(db, projects)
	loc := time.FixedZone("UTC+8", 8*3600)
	now := time.Date(2026, 8, 12, 14, 7, 0, 0, loc)
	mustCreate := func(v any) {
		t.Helper()
		if err := db.Create(v).Error; err != nil {
			t.Fatal(err)
		}
	}
	p, err := projects.Create("P", "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	mustCreate(&models.WorkflowDef{ID: "wf1", ProjectID: p.ID, Name: "w", Status: "draft", Version: 1})
	ts := time.Date(2026, 8, 12, 14, 6, 0, 0, loc).UTC()
	mustCreate(&models.Run{ID: "run1", WorkflowID: "wf1", Status: "completed", StartedAt: ts, CreatedAt: ts})
	mustCreate(&models.StateRun{
		RunID: "run1", NodeID: "n1", Status: "completed",
		Usage: &models.TokenUsage{InputTokens: 10}, StartedAt: &ts,
	})

	q := PlatformStatusQuery{UTCOffsetMinutes: intPtr(8 * 60), Now: now.UTC()}
	first, err := dash.PlatformStatus(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	if first.Current5mBucketTokens == nil || *first.Current5mBucketTokens != 10 {
		t.Fatalf("first current=%v", first.Current5mBucketTokens)
	}

	// Add more usage in same bucket; cache should still return 10 until TTL.
	ts2 := time.Date(2026, 8, 12, 14, 6, 30, 0, loc).UTC()
	mustCreate(&models.Run{ID: "run2", WorkflowID: "wf1", Status: "completed", StartedAt: ts2, CreatedAt: ts2})
	mustCreate(&models.StateRun{
		RunID: "run2", NodeID: "n1", Status: "completed",
		Usage: &models.TokenUsage{InputTokens: 90}, StartedAt: &ts2,
	})
	second, err := dash.PlatformStatus(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	if second.Current5mBucketTokens == nil || *second.Current5mBucketTokens != 10 {
		t.Fatalf("cache miss? current=%v want stale 10", second.Current5mBucketTokens)
	}

	// After TTL, recompute.
	dash.ClearPlatformStatusCacheForTest()
	third, err := dash.PlatformStatus(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	if third.Current5mBucketTokens == nil || *third.Current5mBucketTokens != 100 {
		t.Fatalf("after clear current=%v want 100", third.Current5mBucketTokens)
	}
}

func TestPlatformStatus_singleflightMerges(t *testing.T) {
	db, err := database.OpenSQLiteTest(filepath.Join(t.TempDir(), "platform_status_sf.db"))
	if err != nil {
		t.Fatal(err)
	}
	projects := NewProjectService(db)
	dash := NewDashboardService(db, projects)
	now := time.Date(2026, 8, 12, 14, 7, 0, 0, time.UTC)

	var started int32
	var release sync.WaitGroup
	release.Add(1)
	orig := dash.loadPlatformUsageHook
	dash.loadPlatformUsageHook = func() {
		atomic.AddInt32(&started, 1)
		release.Wait()
	}
	defer func() { dash.loadPlatformUsageHook = orig }()

	q := PlatformStatusQuery{Timezone: "UTC", Now: now}
	var wg sync.WaitGroup
	const n = 8
	wg.Add(n)
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			_, errs[i] = dash.PlatformStatus(context.Background(), q)
		}(i)
	}
	// Wait until the singleflight leader enters the hook (avoid fixed-sleep flake).
	deadline := time.Now().Add(2 * time.Second)
	for atomic.LoadInt32(&started) == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if got := atomic.LoadInt32(&started); got != 1 {
		release.Done()
		t.Fatalf("expected 1 in-flight compute, got %d", got)
	}
	release.Done()
	wg.Wait()
	for i, e := range errs {
		if e != nil {
			t.Fatalf("goroutine %d: %v", i, e)
		}
	}
}

func intPtr(v int) *int { return &v }
