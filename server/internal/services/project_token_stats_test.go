package services

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/cocofhu/approving/internal/database"
	"github.com/cocofhu/approving/internal/models"
)

func TestTokenStatsAggregation(t *testing.T) {
	db, err := database.OpenSQLiteTest(filepath.Join(t.TempDir(), "token_stats.db"))
	if err != nil {
		t.Fatal(err)
	}
	s := NewProjectService(db)

	proj, err := s.Create("TokStats", "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	emptyProj, err := s.Create("EmptyTok", "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	must := func(v any) {
		t.Helper()
		if err := db.Create(v).Error; err != nil {
			t.Fatal(err)
		}
	}

	// Fixed "now" in UTC; client tz = Asia/Shanghai (UTC+8).
	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	ptr := func(tt time.Time) *time.Time { return &tt }

	must(&models.WorkflowDef{ID: "wf-a", ProjectID: proj.ID, Name: "approve-main", Status: "draft", Version: 1})
	must(&models.WorkflowDef{ID: "wf-b", ProjectID: proj.ID, Name: "doc-review", Status: "draft", Version: 1})
	must(&models.WorkflowDef{ID: "wf-c", ProjectID: proj.ID, Name: "misc", Status: "draft", Version: 1})

	// Day in window (2026-07-24 local): reported usage on completed run.
	dayIn := time.Date(2026, 7, 24, 10, 0, 0, 0, loc).UTC()
	must(&models.Run{
		ID: "run-a1", WorkflowID: "wf-a", WorkflowName: "approve-main",
		Status: "completed", StartedAt: dayIn,
	})
	must(&models.StateRun{
		RunID: "run-a1", NodeID: "n1", Status: "completed",
		StartedAt: ptr(dayIn),
		Usage:     &models.TokenUsage{InputTokens: 100, OutputTokens: 20},
	})
	// Nil usage on same run must not count as 0.
	must(&models.StateRun{
		RunID: "run-a1", NodeID: "n2", Status: "completed",
		StartedAt: ptr(dayIn),
	})

	// In-progress run with reported usage — must count.
	dayToday := time.Date(2026, 7, 25, 8, 0, 0, 0, loc).UTC()
	must(&models.Run{
		ID: "run-a2", WorkflowID: "wf-a", WorkflowName: "approve-main",
		Status: "running", StartedAt: dayToday,
	})
	must(&models.StateRun{
		RunID: "run-a2", NodeID: "n1", Status: "running",
		StartedAt: ptr(dayToday),
		Usage:     &models.TokenUsage{InputTokens: 5, CacheReadTokens: 3},
	})

	// Second workflow.
	must(&models.Run{
		ID: "run-b1", WorkflowID: "wf-b", WorkflowName: "doc-review",
		Status: "completed", StartedAt: dayIn,
	})
	must(&models.StateRun{
		RunID: "run-b1", NodeID: "n1", Status: "completed",
		StartedAt: ptr(dayIn),
		Usage:     &models.TokenUsage{OutputTokens: 40, CacheWriteTokens: 10},
	})

	// Outside 7d window but inside all-history (2026-06-01).
	old := time.Date(2026, 6, 1, 12, 0, 0, 0, loc).UTC()
	must(&models.Run{
		ID: "run-c1", WorkflowID: "wf-c", WorkflowName: "misc",
		Status: "completed", StartedAt: old,
	})
	must(&models.StateRun{
		RunID: "run-c1", NodeID: "n1", Status: "completed",
		StartedAt: ptr(old),
		Usage:     &models.TokenUsage{InputTokens: 1000},
	})

	// Explicit zero report — present, totals 0 contribution but marks reported.
	must(&models.Run{
		ID: "run-zero", WorkflowID: "wf-b", WorkflowName: "doc-review",
		Status: "cancelled", StartedAt: dayToday,
	})
	must(&models.StateRun{
		RunID: "run-zero", NodeID: "n1", Status: "cancelled",
		StartedAt: ptr(dayToday),
		Usage:     &models.TokenUsage{},
	})

	t.Run("empty_project_no_forged_series", func(t *testing.T) {
		res, err := s.TokenStats(context.Background(), emptyProj.ID, TokenStatsQuery{
			Window: TokenStatsWindow30d, Timezone: "Asia/Shanghai", Now: now,
		})
		if err != nil {
			t.Fatal(err)
		}
		if !res.Empty {
			t.Fatal("expected empty")
		}
		if len(res.Trend) != 0 {
			t.Fatalf("empty window must not forge zero trend, got %d buckets", len(res.Trend))
		}
		if res.Composition.Total != 0 || len(res.Workflows) != 0 {
			t.Fatalf("empty composition/workflows: %+v", res)
		}
	})

	t.Run("7d_day_buckets_and_null_not_zero", func(t *testing.T) {
		res, err := s.TokenStats(context.Background(), proj.ID, TokenStatsQuery{
			Window: TokenStatsWindow7d, Timezone: "Asia/Shanghai", Now: now,
		})
		if err != nil {
			t.Fatal(err)
		}
		if res.Empty {
			t.Fatal("expected non-empty")
		}
		if res.BucketWidth != TokenStatsBucketDay {
			t.Fatalf("bucketWidth=%s", res.BucketWidth)
		}
		if len(res.Trend) != 7 {
			t.Fatalf("want 7 day buckets, got %d", len(res.Trend))
		}
		// 120 (a1) + 8 (a2 in-progress) + 50 (b1) + 0 (zero report) = 178; old 1000 excluded.
		if res.Composition.Total != 178 {
			t.Fatalf("composition.total=%d want 178", res.Composition.Total)
		}
		if res.Composition.InputTokens != 105 || res.Composition.OutputTokens != 60 ||
			res.Composition.CacheReadTokens != 3 || res.Composition.CacheWriteTokens != 10 {
			t.Fatalf("composition parts: %+v", res.Composition)
		}
		var day24, day25 *TokenStatsBucket
		for i := range res.Trend {
			b := &res.Trend[i]
			switch b.Bucket {
			case "2026-07-24":
				day24 = b
			case "2026-07-25":
				day25 = b
			}
		}
		if day24 == nil || day24.Total != 170 {
			t.Fatalf("2026-07-24 = %+v want total 170", day24)
		}
		if day25 == nil || day25.Total != 8 {
			t.Fatalf("2026-07-25 = %+v want total 8 (in-progress counted)", day25)
		}
	})

	t.Run("all_matches_TotalTokens_and_weekly", func(t *testing.T) {
		res, err := s.TokenStats(context.Background(), proj.ID, TokenStatsQuery{
			Window: TokenStatsWindowAll, Timezone: "Asia/Shanghai", Now: now,
		})
		if err != nil {
			t.Fatal(err)
		}
		if res.BucketWidth != TokenStatsBucketWeek {
			t.Fatalf("bucketWidth=%s", res.BucketWidth)
		}
		tt := s.TotalTokens(proj.ID)
		if tt == nil {
			t.Fatal("TotalTokens nil")
		}
		if res.Composition.Total != *tt {
			t.Fatalf("window=all composition.total=%d TotalTokens=%d", res.Composition.Total, *tt)
		}
		// 178 + 1000 = 1178
		if res.Composition.Total != 1178 {
			t.Fatalf("all total=%d want 1178", res.Composition.Total)
		}
		if len(res.Trend) == 0 {
			t.Fatal("all trend should have week buckets")
		}
	})

	t.Run("top10_plus_other_sums_to_total", func(t *testing.T) {
		// Seed 12 workflows with distinct totals so other appears.
		extraProj, err := s.Create("TopN", "", nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		base := time.Date(2026, 7, 20, 9, 0, 0, 0, loc).UTC()
		var sum int64
		for i := 1; i <= 12; i++ {
			wfID := "wf-top-" + itoa(i)
			must(&models.WorkflowDef{ID: wfID, ProjectID: extraProj.ID, Name: "w" + itoa(i), Status: "draft", Version: 1})
			runID := "run-top-" + itoa(i)
			tokens := int64(i * 100)
			sum += tokens
			must(&models.Run{
				ID: runID, WorkflowID: wfID, WorkflowName: "w" + itoa(i),
				Status: "completed", StartedAt: base,
			})
			must(&models.StateRun{
				RunID: runID, NodeID: "n1", Status: "completed",
				StartedAt: ptr(base),
				Usage:     &models.TokenUsage{InputTokens: tokens},
			})
		}
		res, err := s.TokenStats(context.Background(), extraProj.ID, TokenStatsQuery{
			Window: TokenStatsWindow30d, Timezone: "Asia/Shanghai", Now: now,
		})
		if err != nil {
			t.Fatal(err)
		}
		if res.Composition.Total != sum {
			t.Fatalf("total=%d want %d", res.Composition.Total, sum)
		}
		if len(res.Workflows) != 11 { // Top10 + other
			t.Fatalf("workflows len=%d want 11", len(res.Workflows))
		}
		var gotSum int64
		var otherCount int
		for _, w := range res.Workflows {
			gotSum += w.Total
			if w.Other {
				otherCount++
			}
		}
		if otherCount != 1 {
			t.Fatalf("other count=%d", otherCount)
		}
		if gotSum != sum {
			t.Fatalf("Top10+other=%d want %d", gotSum, sum)
		}
		// Highest should be w12 = 1200
		if res.Workflows[0].Total != 1200 || res.Workflows[0].Other {
			t.Fatalf("top1=%+v", res.Workflows[0])
		}
		// Other = w1+w2 = 100+200 = 300
		last := res.Workflows[len(res.Workflows)-1]
		if !last.Other || last.Total != 300 {
			t.Fatalf("other=%+v want 300", last)
		}
	})

	t.Run("started_at_fallback_to_run", func(t *testing.T) {
		p, err := s.Create("Fallback", "", nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		ts := time.Date(2026, 7, 22, 15, 0, 0, 0, loc).UTC()
		must(&models.WorkflowDef{ID: "wf-fb", ProjectID: p.ID, Name: "fb", Status: "draft", Version: 1})
		must(&models.Run{
			ID: "run-fb", WorkflowID: "wf-fb", WorkflowName: "fb",
			Status: "completed", StartedAt: ts,
		})
		// StateRun without StartedAt → use Run.StartedAt
		must(&models.StateRun{
			RunID: "run-fb", NodeID: "n1", Status: "completed",
			Usage: &models.TokenUsage{InputTokens: 42},
		})
		res, err := s.TokenStats(context.Background(), p.ID, TokenStatsQuery{
			Window: TokenStatsWindow7d, Timezone: "Asia/Shanghai", Now: now,
		})
		if err != nil {
			t.Fatal(err)
		}
		if res.Composition.Total != 42 {
			t.Fatalf("total=%d", res.Composition.Total)
		}
		found := false
		for _, b := range res.Trend {
			if b.Bucket == "2026-07-22" && b.Total == 42 {
				found = true
			}
		}
		if !found {
			t.Fatalf("fallback bucket missing: %+v", res.Trend)
		}
	})

	t.Run("invalid_window", func(t *testing.T) {
		_, err := s.TokenStats(context.Background(), proj.ID, TokenStatsQuery{
			Window: "1y", Timezone: "UTC", Now: now,
		})
		if err != ErrInvalidTokenStatsWindow {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("pm_usage_in_trend_composition_rank", func(t *testing.T) {
		// g2.1–g2.4: PM ChatMessage.Usage merges into total/trend/composition/rank.
		p, err := s.Create("WithPM", "", nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		ts := time.Date(2026, 7, 24, 12, 0, 0, 0, loc).UTC()
		must(&models.WorkflowDef{ID: "wf-pm", ProjectID: p.ID, Name: "wf-pm", Status: "draft", Version: 1})
		must(&models.Run{
			ID: "run-pm-wf", WorkflowID: "wf-pm", WorkflowName: "wf-pm",
			Status: "completed", StartedAt: ts,
		})
		must(&models.StateRun{
			RunID: "run-pm-wf", NodeID: "n1", Status: "completed",
			StartedAt: ptr(ts),
			Usage:     &models.TokenUsage{InputTokens: 100, OutputTokens: 20},
		})
		must(&models.ChatThread{ID: "th-pm", ProjectID: p.ID, UserID: "u1", Title: "t"})
		// Historical assistant without Usage — must NOT backfill.
		must(&models.ChatMessage{
			ID: "msg-old", ThreadID: "th-pm", Role: "assistant", Content: "old",
			Status: "ok", CreatedAt: ts,
		})
		must(&models.ChatMessage{
			ID: "msg-pm", ThreadID: "th-pm", Role: "assistant", Content: "hi",
			Status: "ok", CreatedAt: ts,
			Usage: &models.TokenUsage{InputTokens: 40, OutputTokens: 10},
		})
		// Nil usage (unreported) assistant — not counted.
		must(&models.ChatMessage{
			ID: "msg-nil", ThreadID: "th-pm", Role: "assistant", Content: "nil",
			Status: "ok", CreatedAt: ts,
		})

		res, err := s.TokenStats(context.Background(), p.ID, TokenStatsQuery{
			Window: TokenStatsWindow7d, Timezone: "Asia/Shanghai", Now: now,
		})
		if err != nil {
			t.Fatal(err)
		}
		if res.Empty {
			t.Fatal("expected non-empty with PM+workflow")
		}
		// 120 workflow + 50 PM = 170
		if res.Composition.Total != 170 {
			t.Fatalf("composition.total=%d want 170", res.Composition.Total)
		}
		var day24 *TokenStatsBucket
		for i := range res.Trend {
			if res.Trend[i].Bucket == "2026-07-24" {
				day24 = &res.Trend[i]
				break
			}
		}
		if day24 == nil {
			t.Fatal("missing 2026-07-24 bucket")
		}
		if day24.WorkflowTotal != 120 || day24.PmTotal != 50 || day24.Total != 170 {
			t.Fatalf("day24=%+v want wf=120 pm=50 total=170", day24)
		}
		var sawPM, sawWF bool
		for _, w := range res.Workflows {
			switch w.Kind {
			case TokenStatsKindPM:
				sawPM = true
				if w.Total != 50 || w.Other {
					t.Fatalf("pm row=%+v", w)
				}
			case TokenStatsKindWorkflow:
				sawWF = true
			case TokenStatsKindOther:
				t.Fatalf("other must not absorb PM: %+v", w)
			}
		}
		if !sawPM || !sawWF {
			t.Fatalf("rank missing pm/workflow: %+v", res.Workflows)
		}

		bd := s.TokenBreakdown(p.ID)
		if bd.Total == nil || *bd.Total != 170 {
			t.Fatalf("breakdown.total=%v want 170", bd.Total)
		}
		if bd.Workflow == nil || *bd.Workflow != 120 {
			t.Fatalf("breakdown.workflow=%v want 120", bd.Workflow)
		}
		if bd.PM == nil || *bd.PM != 50 {
			t.Fatalf("breakdown.pm=%v want 50", bd.PM)
		}
	})
}

// TestBuildConsumptionRankOrder locks the fixed slot order: Top workflows → PM → other.
// Even when PM.total exceeds some workflow totals, PM must not insert mid-workflow (no 12PM34).
func TestBuildConsumptionRankOrder(t *testing.T) {
	assertOrder := func(t *testing.T, rows []TokenStatsWorkflow) {
		t.Helper()
		var sawPM, sawOther bool
		for i, w := range rows {
			switch w.Kind {
			case TokenStatsKindWorkflow:
				if sawPM || sawOther {
					t.Fatalf("workflow after pm/other at [%d]: %+v", i, rows)
				}
			case TokenStatsKindPM:
				if sawOther {
					t.Fatalf("pm after other at [%d]: %+v", i, rows)
				}
				if sawPM {
					t.Fatalf("duplicate pm at [%d]: %+v", i, rows)
				}
				sawPM = true
			case TokenStatsKindOther:
				if !w.Other {
					t.Fatalf("other kind without Other flag: %+v", w)
				}
				if sawOther {
					t.Fatalf("duplicate other at [%d]: %+v", i, rows)
				}
				sawOther = true
				if i != len(rows)-1 {
					t.Fatalf("other must be last: %+v", rows)
				}
			default:
				t.Fatalf("unexpected kind %q at [%d]", w.Kind, i)
			}
		}
	}

	t.Run("pm_between_top_and_other_even_when_pm_total_high", func(t *testing.T) {
		totals := map[string]int64{
			"wf-a": 1000,
			"wf-b": 100,
			"wf-c": 50,
		}
		names := map[string]string{
			"wf-a": "approve-main",
			"wf-b": "doc-review",
			"wf-c": "调研",
		}
		// PM total (800) sits between wf-a and wf-b; old full-sort would yield 12PM34.
		rows := buildConsumptionRank(totals, names, 800, true)
		assertOrder(t, rows)
		if len(rows) != 4 {
			t.Fatalf("len=%d want 4 (3 wf + pm)", len(rows))
		}
		if rows[0].Kind != TokenStatsKindWorkflow || rows[0].Total != 1000 {
			t.Fatalf("top1=%+v", rows[0])
		}
		if rows[1].Kind != TokenStatsKindWorkflow || rows[1].Total != 100 {
			t.Fatalf("top2=%+v", rows[1])
		}
		if rows[2].Kind != TokenStatsKindWorkflow || rows[2].Total != 50 {
			t.Fatalf("top3=%+v", rows[2])
		}
		if rows[3].Kind != TokenStatsKindPM || rows[3].Total != 800 {
			t.Fatalf("pm=%+v want last before other", rows[3])
		}
	})

	t.Run("with_other_pm_before_other", func(t *testing.T) {
		totals := map[string]int64{}
		names := map[string]string{}
		for i := 1; i <= 12; i++ {
			id := "w" + itoa(i)
			totals[id] = int64(i * 100)
			names[id] = id
		}
		rows := buildConsumptionRank(totals, names, 9999, true)
		assertOrder(t, rows)
		if len(rows) != 12 { // Top10 + PM + other
			t.Fatalf("len=%d want 12", len(rows))
		}
		if rows[10].Kind != TokenStatsKindPM || rows[10].Total != 9999 {
			t.Fatalf("pm slot=%+v", rows[10])
		}
		if rows[11].Kind != TokenStatsKindOther || !rows[11].Other {
			t.Fatalf("other slot=%+v", rows[11])
		}
		// Top block still desc by total.
		for i := 1; i < 10; i++ {
			if rows[i].Total > rows[i-1].Total {
				t.Fatalf("top not desc at %d: %+v", i, rows[:10])
			}
		}
	})

	t.Run("no_pm_no_pm_row", func(t *testing.T) {
		totals := map[string]int64{"a": 10, "b": 5}
		names := map[string]string{"a": "A", "b": "B"}
		rows := buildConsumptionRank(totals, names, 0, false)
		assertOrder(t, rows)
		for _, w := range rows {
			if w.Kind == TokenStatsKindPM {
				t.Fatalf("unexpected pm: %+v", rows)
			}
		}
	})

	t.Run("pm_without_other", func(t *testing.T) {
		totals := map[string]int64{"a": 10}
		names := map[string]string{"a": "A"}
		rows := buildConsumptionRank(totals, names, 3, true)
		assertOrder(t, rows)
		if len(rows) != 2 || rows[1].Kind != TokenStatsKindPM {
			t.Fatalf("want Top→PM: %+v", rows)
		}
	})
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [16]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
