package services

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/cocofhu/approving/internal/models"
)

// Token-stats window presets and errors.
const (
	TokenStatsWindow7d  = "7d"
	TokenStatsWindow30d = "30d"
	TokenStatsWindow90d = "90d"
	TokenStatsWindowAll = "all"

	TokenStatsBucketDay  = "day"
	TokenStatsBucketWeek = "week"

	// tokenStatsTimeout is the server-side soft deadline for large project scans.
	tokenStatsTimeout = 30 * time.Second
)

var (
	// ErrInvalidTokenStatsWindow is returned for unknown window query values.
	ErrInvalidTokenStatsWindow = errors.New("invalid token-stats window")
	// ErrInvalidTokenStatsTimezone is returned when IANA timezone cannot be loaded
	// and no usable utcOffsetMinutes fallback is available.
	ErrInvalidTokenStatsTimezone = errors.New("invalid token-stats timezone")
	// ErrTokenStatsTimeout is returned when aggregation exceeds the soft deadline
	// (clients should treat as retryable).
	ErrTokenStatsTimeout = errors.New("token-stats aggregation timed out")
)

// TokenStatsQuery is the input for project-level token chart aggregation.
type TokenStatsQuery struct {
	Window           string // 7d|30d|90d|all
	Timezone         string // preferred IANA name
	UTCOffsetMinutes *int   // fallback fixed offset (east of UTC positive)
	Now              time.Time
}

// TokenStatsBucket is one day or week bucket with totals, source split, and
// four components (sum of all reported sources in the bucket).
type TokenStatsBucket struct {
	Bucket           string `json:"bucket"`
	Total            int64  `json:"total"`
	WorkflowTotal    int64  `json:"workflowTotal"`
	PmTotal          int64  `json:"pmTotal"`
	InputTokens      int64  `json:"inputTokens"`
	OutputTokens     int64  `json:"outputTokens"`
	CacheReadTokens  int64  `json:"cacheReadTokens"`
	CacheWriteTokens int64  `json:"cacheWriteTokens"`
}

// TokenStatsComposition is the four-component sum for the window (workflow+PM).
type TokenStatsComposition struct {
	InputTokens      int64 `json:"inputTokens"`
	OutputTokens     int64 `json:"outputTokens"`
	CacheReadTokens  int64 `json:"cacheReadTokens"`
	CacheWriteTokens int64 `json:"cacheWriteTokens"`
	Total            int64 `json:"total"`
}

// Token-stats rank kinds.
const (
	TokenStatsKindWorkflow = "workflow"
	TokenStatsKindPM       = "pm"
	TokenStatsKindOther    = "other"
)

// TokenStatsWorkflow is one consumption-rank row (workflow Top-N, PM, or other).
type TokenStatsWorkflow struct {
	WorkflowID string `json:"workflowId,omitempty"`
	Name       string `json:"name"`
	Total      int64  `json:"total"`
	Other      bool   `json:"other,omitempty"`
	Kind       string `json:"kind,omitempty"` // workflow | pm | other
}

// TokenStatsModel is one model-composition / model-ranking row.
type TokenStatsModel struct {
	ModelKey string `json:"modelKey"`
	Name     string `json:"name"`
	Total    int64  `json:"total"`
	// Unknown marks the「未知/未分桶」bucket (≠ other).
	Unknown bool `json:"unknown,omitempty"`
	// Other marks Top10 remainder of identified models (≠ unknown).
	Other bool `json:"other,omitempty"`
	// Filled is true when the bucket includes ACP_BRIDGE_MODEL backfill.
	Filled bool `json:"filled,omitempty"`
	// Source: upstream | via ACP_BRIDGE_MODEL | unknown (omitted for other).
	Source string `json:"source,omitempty"`
}

// TokenStatsResult is the single-response payload for trend/composition/workflows
// plus model composition/ranking.
type TokenStatsResult struct {
	Window      string                `json:"window"`
	BucketWidth string                `json:"bucketWidth"`
	Timezone    string                `json:"timezone"`
	Empty       bool                  `json:"empty"`
	Trend       []TokenStatsBucket    `json:"trend"`
	Composition TokenStatsComposition `json:"composition"`
	Workflows   []TokenStatsWorkflow  `json:"workflows"`
	// ModelComposition is per-model four-component totals (window scope).
	ModelComposition []TokenStatsModel `json:"modelComposition"`
	// ModelRanking is Top10 models by total + optional other (unknown independent).
	ModelRanking []TokenStatsModel `json:"modelRanking"`
}

type tokenUsageRow struct {
	ts           time.Time
	usage        models.TokenUsage
	byModel      models.TokenUsageByModel // effective (legacy → unknown)
	workflowID   string
	workflowName string
	source       string // workflow | pm
}

// TokenStats aggregates non-nil StateRun.Usage + assistant ChatMessage.Usage for
// one project into trend (workflow/pm split), composition (four parts), and
// consumption rank (workflow Top10 + PM + other). Stdio is never counted.
// Timestamp prefers StateRun.StartedAt / message CreatedAt.
func (s *ProjectService) TokenStats(ctx context.Context, projectID string, q TokenStatsQuery) (TokenStatsResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, tokenStatsTimeout)
	defer cancel()

	window := strings.TrimSpace(q.Window)
	if window == "" {
		window = TokenStatsWindow30d
	}
	days, bucketWidth, err := parseTokenStatsWindow(window)
	if err != nil {
		return TokenStatsResult{}, err
	}

	loc, tzLabel, err := resolveTokenStatsLocation(q.Timezone, q.UTCOffsetMinutes)
	if err != nil {
		return TokenStatsResult{}, err
	}

	now := q.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	nowLocal := now.In(loc)

	rows, err := s.loadTokenUsageRows(ctx, projectID)
	if err != nil {
		return TokenStatsResult{}, err
	}

	var windowStart time.Time
	var hasStart bool
	if days > 0 {
		// Inclusive local-day window: today and the previous (days-1) local days.
		startDay := truncateLocalDay(nowLocal).AddDate(0, 0, -(days - 1))
		windowStart = startDay
		hasStart = true
	}

	type agg struct {
		input, output, cacheRead, cacheWrite int64
		workflow, pm                         int64
	}
	buckets := map[string]*agg{}
	wfTotals := map[string]int64{}
	wfNames := map[string]string{}
	wfLatest := map[string]time.Time{}
	var pmTotal int64
	var hasPM bool
	var composition agg
	var hasAny bool
	modelTotals := map[string]*tokenModelAgg{}

	for _, row := range rows {
		if err := ctx.Err(); err != nil {
			return TokenStatsResult{}, ErrTokenStatsTimeout
		}
		local := row.ts.In(loc)
		if hasStart && local.Before(windowStart) {
			continue
		}
		hasAny = true
		key := bucketKey(local, bucketWidth)
		b := buckets[key]
		if b == nil {
			b = &agg{}
			buckets[key] = b
		}
		b.input += row.usage.InputTokens
		b.output += row.usage.OutputTokens
		b.cacheRead += row.usage.CacheReadTokens
		b.cacheWrite += row.usage.CacheWriteTokens

		composition.input += row.usage.InputTokens
		composition.output += row.usage.OutputTokens
		composition.cacheRead += row.usage.CacheReadTokens
		composition.cacheWrite += row.usage.CacheWriteTokens

		for mk, mu := range row.byModel {
			ma := modelTotals[mk]
			if ma == nil {
				ma = &tokenModelAgg{source: mu.Source}
				modelTotals[mk] = ma
			}
			ma.total += mu.Total()
			ma.filled = ma.filled || mu.Filled
			if mu.Filled {
				ma.source = models.TokenUsageSourceBridge
			} else if ma.source == "" {
				ma.source = mu.Source
			}
		}

		total := row.usage.Total()
		if row.source == TokenStatsKindPM {
			b.pm += total
			pmTotal += total
			hasPM = true
			continue
		}
		b.workflow += total

		wfID := row.workflowID
		if wfID == "" {
			wfID = "_unknown"
		}
		wfTotals[wfID] += total
		if prev, ok := wfLatest[wfID]; !ok || !local.Before(prev) {
			wfLatest[wfID] = local
			name := strings.TrimSpace(row.workflowName)
			if name == "" {
				name = wfID
			}
			wfNames[wfID] = name
		}
	}

	out := TokenStatsResult{
		Window:           window,
		BucketWidth:      bucketWidth,
		Timezone:         tzLabel,
		Empty:            !hasAny,
		Trend:            []TokenStatsBucket{},
		Composition:      TokenStatsComposition{},
		Workflows:        []TokenStatsWorkflow{},
		ModelComposition: []TokenStatsModel{},
		ModelRanking:     []TokenStatsModel{},
	}
	if !hasAny {
		// Empty window: no forged all-zero series.
		return out, nil
	}

	out.Composition = TokenStatsComposition{
		InputTokens:      composition.input,
		OutputTokens:     composition.output,
		CacheReadTokens:  composition.cacheRead,
		CacheWriteTokens: composition.cacheWrite,
		Total: composition.input + composition.output +
			composition.cacheRead + composition.cacheWrite,
	}

	presentKeys := make(map[string]struct{}, len(buckets))
	for k := range buckets {
		presentKeys[k] = struct{}{}
	}
	keys := fillBucketKeys(nowLocal, windowStart, hasStart, bucketWidth, presentKeys)
	out.Trend = make([]TokenStatsBucket, 0, len(keys))
	for _, k := range keys {
		b := buckets[k]
		if b == nil {
			out.Trend = append(out.Trend, TokenStatsBucket{Bucket: k})
			continue
		}
		out.Trend = append(out.Trend, TokenStatsBucket{
			Bucket:           k,
			Total:            b.workflow + b.pm,
			WorkflowTotal:    b.workflow,
			PmTotal:          b.pm,
			InputTokens:      b.input,
			OutputTokens:     b.output,
			CacheReadTokens:  b.cacheRead,
			CacheWriteTokens: b.cacheWrite,
		})
	}

	out.Workflows = buildConsumptionRank(wfTotals, wfNames, pmTotal, hasPM)
	out.ModelComposition, out.ModelRanking = buildModelStats(modelTotals)
	return out, nil
}

func parseTokenStatsWindow(window string) (days int, bucketWidth string, err error) {
	switch window {
	case TokenStatsWindow7d:
		return 7, TokenStatsBucketDay, nil
	case TokenStatsWindow30d:
		return 30, TokenStatsBucketDay, nil
	case TokenStatsWindow90d:
		return 90, TokenStatsBucketDay, nil
	case TokenStatsWindowAll:
		return 0, TokenStatsBucketWeek, nil
	default:
		return 0, "", ErrInvalidTokenStatsWindow
	}
}

func resolveTokenStatsLocation(iana string, offsetMinutes *int) (*time.Location, string, error) {
	iana = strings.TrimSpace(iana)
	if iana != "" {
		loc, err := time.LoadLocation(iana)
		if err == nil {
			return loc, iana, nil
		}
		// Fall through to offset if provided.
		if offsetMinutes == nil {
			return nil, "", fmt.Errorf("%w: %s", ErrInvalidTokenStatsTimezone, iana)
		}
	}
	if offsetMinutes != nil {
		mins := *offsetMinutes
		if mins < -14*60 || mins > 14*60 {
			return nil, "", ErrInvalidTokenStatsTimezone
		}
		name := fmt.Sprintf("UTC%+d", mins)
		return time.FixedZone(name, mins*60), name, nil
	}
	if iana == "" {
		return time.UTC, "UTC", nil
	}
	return nil, "", ErrInvalidTokenStatsTimezone
}

func truncateLocalDay(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}

func bucketKey(local time.Time, width string) string {
	if width == TokenStatsBucketWeek {
		y, w := local.ISOWeek()
		return fmt.Sprintf("%04d-W%02d", y, w)
	}
	return local.Format("2006-01-02")
}

func fillBucketKeys(nowLocal, windowStart time.Time, hasStart bool, width string, present map[string]struct{}) []string {
	if width == TokenStatsBucketWeek {
		var start time.Time
		if hasStart {
			start = windowStart
		} else {
			// Earliest present week, or now if somehow empty (caller guards empty).
			start = nowLocal
			for k := range present {
				t, ok := parseWeekKey(k, nowLocal.Location())
				if !ok {
					continue
				}
				if t.Before(start) {
					start = t
				}
			}
		}
		start = startOfISOWeek(start)
		end := startOfISOWeek(nowLocal)
		var keys []string
		for t := start; !t.After(end); t = t.AddDate(0, 0, 7) {
			keys = append(keys, bucketKey(t, TokenStatsBucketWeek))
		}
		return keys
	}

	start := windowStart
	if !hasStart {
		start = truncateLocalDay(nowLocal)
		for k := range present {
			t, err := time.ParseInLocation("2006-01-02", k, nowLocal.Location())
			if err != nil {
				continue
			}
			if t.Before(start) {
				start = t
			}
		}
	}
	end := truncateLocalDay(nowLocal)
	var keys []string
	for t := start; !t.After(end); t = t.AddDate(0, 0, 1) {
		keys = append(keys, t.Format("2006-01-02"))
	}
	return keys
}

func startOfISOWeek(t time.Time) time.Time {
	t = truncateLocalDay(t)
	// Go Weekday: Sunday=0 … Saturday=6; ISO week starts Monday.
	wd := int(t.Weekday())
	if wd == 0 {
		wd = 7
	}
	return t.AddDate(0, 0, -(wd - 1))
}

func parseWeekKey(key string, loc *time.Location) (time.Time, bool) {
	// "YYYY-Www"
	if len(key) != 8 || key[4] != '-' || key[5] != 'W' {
		return time.Time{}, false
	}
	y, err1 := strconv.Atoi(key[:4])
	w, err2 := strconv.Atoi(key[6:])
	if err1 != nil || err2 != nil || w < 1 || w > 53 {
		return time.Time{}, false
	}
	// Find Monday of ISO week w in year y.
	jan4 := time.Date(y, 1, 4, 0, 0, 0, 0, loc)
	start := startOfISOWeek(jan4).AddDate(0, 0, (w-1)*7)
	return start, true
}

type tokenModelAgg struct {
	total  int64
	filled bool
	source string
}

// buildModelStats builds model composition (all buckets desc by total) and
// Top10 + other ranking. 「未知/未分桶」is independent and never folded into other.
func buildModelStats(totals map[string]*tokenModelAgg) (composition []TokenStatsModel, ranking []TokenStatsModel) {
	type item struct {
		key    string
		total  int64
		filled bool
		source string
		unk    bool
	}
	list := make([]item, 0, len(totals))
	for k, a := range totals {
		if a == nil {
			continue
		}
		list = append(list, item{
			key:    k,
			total:  a.total,
			filled: a.filled,
			source: a.source,
			unk:    k == models.TokenUsageModelUnknown,
		})
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].total != list[j].total {
			return list[i].total > list[j].total
		}
		return list[i].key < list[j].key
	})

	composition = make([]TokenStatsModel, 0, len(list))
	for _, it := range list {
		composition = append(composition, TokenStatsModel{
			ModelKey: it.key,
			Name:     it.key,
			Total:    it.total,
			Unknown:  it.unk,
			Filled:   it.filled,
			Source:   it.source,
		})
	}

	const topN = 10
	ranking = make([]TokenStatsModel, 0, topN+1)
	var other int64
	for i, it := range list {
		if i < topN {
			ranking = append(ranking, TokenStatsModel{
				ModelKey: it.key,
				Name:     it.key,
				Total:    it.total,
				Unknown:  it.unk,
				Filled:   it.filled,
				Source:   it.source,
			})
			continue
		}
		other += it.total
	}
	if other > 0 {
		ranking = append(ranking, TokenStatsModel{
			Name:  "other",
			Total: other,
			Other: true,
		})
	}
	return composition, ranking
}

func buildConsumptionRank(totals map[string]int64, names map[string]string, pmTotal int64, hasPM bool) []TokenStatsWorkflow {
	type item struct {
		id    string
		name  string
		total int64
	}
	list := make([]item, 0, len(totals))
	for id, total := range totals {
		name := names[id]
		if name == "" {
			name = id
		}
		list = append(list, item{id: id, name: name, total: total})
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].total != list[j].total {
			return list[i].total > list[j].total
		}
		return list[i].name < list[j].name
	})

	const topN = 10
	out := make([]TokenStatsWorkflow, 0, topN+2)
	var other int64
	for i, it := range list {
		if i < topN {
			id := it.id
			if id == "_unknown" {
				id = ""
			}
			out = append(out, TokenStatsWorkflow{
				WorkflowID: id,
				Name:       it.name,
				Total:      it.total,
				Kind:       TokenStatsKindWorkflow,
			})
			continue
		}
		other += it.total
	}
	if hasPM {
		out = append(out, TokenStatsWorkflow{
			Name:  "PM",
			Total: pmTotal,
			Kind:  TokenStatsKindPM,
		})
	}
	if other > 0 {
		out = append(out, TokenStatsWorkflow{
			Name:  "other",
			Total: other,
			Other: true,
			Kind:  TokenStatsKindOther,
		})
	}
	// Fixed slot order: Top workflows (already desc) → PM (if any) → other (if any).
	// Do not re-sort by Total — that would insert PM between workflow rows (12PM34).
	return out
}

func (s *ProjectService) loadTokenUsageRows(ctx context.Context, projectID string) ([]tokenUsageRow, error) {
	wfRows, err := s.loadWorkflowTokenUsageRows(ctx, projectID)
	if err != nil {
		return nil, err
	}
	pmRows, err := s.loadPMTokenUsageRows(ctx, projectID)
	if err != nil {
		return nil, err
	}
	out := make([]tokenUsageRow, 0, len(wfRows)+len(pmRows))
	out = append(out, wfRows...)
	out = append(out, pmRows...)
	return out, nil
}

func (s *ProjectService) loadWorkflowTokenUsageRows(ctx context.Context, projectID string) ([]tokenUsageRow, error) {
	var wfIDs []string
	if err := s.db.WithContext(ctx).Model(&models.WorkflowDef{}).
		Select("id").
		Where("project_id = ?", projectID).
		Pluck("id", &wfIDs).Error; err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			return nil, ErrTokenStatsTimeout
		}
		return nil, err
	}
	if len(wfIDs) == 0 {
		return nil, nil
	}

	type runRow struct {
		ID           string
		WorkflowID   string
		WorkflowName string
		StartedAt    time.Time
	}
	var runs []runRow
	for i := 0; i < len(wfIDs); i += tokenAggChunk {
		if err := ctx.Err(); err != nil {
			return nil, ErrTokenStatsTimeout
		}
		end := i + tokenAggChunk
		if end > len(wfIDs) {
			end = len(wfIDs)
		}
		var chunk []runRow
		if err := s.db.WithContext(ctx).Model(&models.Run{}).
			Select("id", "workflow_id", "workflow_name", "started_at").
			Where("workflow_id IN ?", wfIDs[i:end]).
			Find(&chunk).Error; err != nil {
			if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
				return nil, ErrTokenStatsTimeout
			}
			return nil, err
		}
		runs = append(runs, chunk...)
	}
	if len(runs) == 0 {
		return nil, nil
	}

	runMeta := make(map[string]runRow, len(runs))
	runIDs := make([]string, 0, len(runs))
	for _, r := range runs {
		runMeta[r.ID] = r
		runIDs = append(runIDs, r.ID)
	}

	var out []tokenUsageRow
	for i := 0; i < len(runIDs); i += tokenAggChunk {
		if err := ctx.Err(); err != nil {
			return nil, ErrTokenStatsTimeout
		}
		end := i + tokenAggChunk
		if end > len(runIDs) {
			end = len(runIDs)
		}
		var srs []models.StateRun
		if err := s.db.WithContext(ctx).Model(&models.StateRun{}).
			Select("run_id", "usage", "usage_by_model", "started_at").
			Where("run_id IN ? AND usage IS NOT NULL", runIDs[i:end]).
			Find(&srs).Error; err != nil {
			if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
				return nil, ErrTokenStatsTimeout
			}
			return nil, err
		}
		for _, sr := range srs {
			if sr.Usage == nil {
				continue
			}
			meta, ok := runMeta[sr.RunID]
			if !ok {
				continue
			}
			ts := meta.StartedAt
			if sr.StartedAt != nil && !sr.StartedAt.IsZero() {
				ts = *sr.StartedAt
			}
			if ts.IsZero() {
				continue
			}
			by := models.EffectiveUsageByModel(sr.Usage, sr.UsageByModel)
			out = append(out, tokenUsageRow{
				ts:           ts,
				usage:        *sr.Usage,
				byModel:      by,
				workflowID:   meta.WorkflowID,
				workflowName: meta.WorkflowName,
				source:       TokenStatsKindWorkflow,
			})
		}
	}
	return out, nil
}

func (s *ProjectService) loadPMTokenUsageRows(ctx context.Context, projectID string) ([]tokenUsageRow, error) {
	var threadIDs []string
	if err := s.db.WithContext(ctx).Model(&models.ChatThread{}).
		Select("id").
		Where("project_id = ?", projectID).
		Pluck("id", &threadIDs).Error; err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			return nil, ErrTokenStatsTimeout
		}
		return nil, err
	}
	if len(threadIDs) == 0 {
		return nil, nil
	}

	var out []tokenUsageRow
	for i := 0; i < len(threadIDs); i += tokenAggChunk {
		if err := ctx.Err(); err != nil {
			return nil, ErrTokenStatsTimeout
		}
		end := i + tokenAggChunk
		if end > len(threadIDs) {
			end = len(threadIDs)
		}
		var msgs []models.ChatMessage
		if err := s.db.WithContext(ctx).Model(&models.ChatMessage{}).
			Select("usage", "usage_by_model", "created_at").
			Where("thread_id IN ? AND role = ? AND usage IS NOT NULL", threadIDs[i:end], "assistant").
			Find(&msgs).Error; err != nil {
			if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
				return nil, ErrTokenStatsTimeout
			}
			return nil, err
		}
		for _, m := range msgs {
			if m.Usage == nil || m.CreatedAt.IsZero() {
				continue
			}
			by := models.EffectiveUsageByModel(m.Usage, m.UsageByModel)
			out = append(out, tokenUsageRow{
				ts:      m.CreatedAt,
				usage:   *m.Usage,
				byModel: by,
				source:  TokenStatsKindPM,
			})
		}
	}
	return out, nil
}
