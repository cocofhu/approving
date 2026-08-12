package services

import (
	"context"
	"sync"
	"time"

	"github.com/cocofhu/approving/internal/models"
)

const (
	platformStatusTTL     = 12 * time.Second
	platformStatusTimeout = 15 * time.Second
	fiveMinute            = 5 * time.Minute
)

// PlatformStatusQuery controls timezone for calendar-aligned 5m buckets.
type PlatformStatusQuery struct {
	Timezone         string
	UTCOffsetMinutes *int
	Now              time.Time // tests may inject; zero → time.Now().UTC()
}

// PlatformStatusMetrics is the AppTopbar StatusMetrics payload.
// Token fields use pointers: JSON null = unavailable / never reported; 0 is a real zero.
type PlatformStatusMetrics struct {
	CumulativeTokens          *int64     `json:"cumulativeTokens"`
	Current5mBucketTokens     *int64     `json:"current5mBucketTokens"`
	TodayMaxCompleted5mTokens *int64     `json:"todayMaxCompleted5mTokens"`
	RunningCount              int64      `json:"runningCount"`
	QueuedCount               int64      `json:"queuedCount"`
	CurrentBucketStart        *time.Time `json:"currentBucketStart,omitempty"`
	CurrentBucketEnd          *time.Time `json:"currentBucketEnd,omitempty"`
	PeakBucketStart           *time.Time `json:"peakBucketStart,omitempty"`
	PeakBucketEnd             *time.Time `json:"peakBucketEnd,omitempty"`
	AsOf                      time.Time  `json:"asOf"`
	Timezone                  string     `json:"timezone"`
}

type platformStatusCacheEntry struct {
	metrics PlatformStatusMetrics
	expires time.Time
}

type platformStatusCall struct {
	wg  sync.WaitGroup
	val PlatformStatusMetrics
}

type platformUsagePoint struct {
	ts    time.Time
	total int64
}

// PlatformStatus returns running/queued counts, cumulative tokens, and calendar
// 5m current-bucket + today peak. 5m aggregation is process-cached (TTL≈12s)
// with singleflight so topbar polling does not re-scan every request.
func (s *DashboardService) PlatformStatus(ctx context.Context, q PlatformStatusQuery) (PlatformStatusMetrics, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	loc, tzLabel, err := resolveTokenStatsLocation(q.Timezone, q.UTCOffsetMinutes)
	if err != nil {
		return PlatformStatusMetrics{}, err
	}

	now := q.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}

	// Cheap counts always fresh; token/5m path may hit cache.
	running, queued := s.countRunStatuses()
	var cumulative *int64
	if s.projects != nil {
		cumulative = s.projects.PlatformTokenBreakdown().Total
	}

	five, err := s.cachedFiveMinuteMetrics(ctx, tzLabel, loc, now)
	if err != nil {
		return PlatformStatusMetrics{}, err
	}

	out := PlatformStatusMetrics{
		CumulativeTokens:          cumulative,
		RunningCount:              running,
		QueuedCount:               queued,
		AsOf:                      now,
		Timezone:                  tzLabel,
		CurrentBucketStart:        five.currentStart,
		CurrentBucketEnd:          five.currentEnd,
		PeakBucketStart:           five.peakStart,
		PeakBucketEnd:             five.peakEnd,
		TodayMaxCompleted5mTokens: five.peakTokens,
	}
	if cumulative == nil {
		// Never reported → rate/peak stay null (UI "—"), not fake zeros.
		out.Current5mBucketTokens = nil
		out.TodayMaxCompleted5mTokens = nil
		out.PeakBucketStart = nil
		out.PeakBucketEnd = nil
	} else {
		out.Current5mBucketTokens = five.currentTokens
	}
	return out, nil
}

type fiveMinuteBundle struct {
	currentTokens *int64
	peakTokens    *int64
	currentStart  *time.Time
	currentEnd    *time.Time
	peakStart     *time.Time
	peakEnd       *time.Time
}

func (s *DashboardService) countRunStatuses() (running, queued int64) {
	count := func(status string) int64 {
		var n int64
		s.db.Model(&models.Run{}).Where("status = ?", status).Count(&n)
		return n
	}
	return count("running"), count("queued")
}

func (s *DashboardService) cachedFiveMinuteMetrics(ctx context.Context, cacheKey string, loc *time.Location, now time.Time) (fiveMinuteBundle, error) {
	s.statusMu.Lock()
	if s.statusCache == nil {
		s.statusCache = map[string]platformStatusCacheEntry{}
	}
	if ent, ok := s.statusCache[cacheKey]; ok && now.Before(ent.expires) {
		m := ent.metrics
		s.statusMu.Unlock()
		return fiveMinuteBundleFromMetrics(m), nil
	}
	if s.statusInflight == nil {
		s.statusInflight = map[string]*platformStatusCall{}
	}
	if call, ok := s.statusInflight[cacheKey]; ok {
		s.statusMu.Unlock()
		call.wg.Wait()
		return fiveMinuteBundleFromMetrics(call.val), nil
	}
	call := &platformStatusCall{}
	call.wg.Add(1)
	s.statusInflight[cacheKey] = call
	s.statusMu.Unlock()

	bundle, err := s.computeFiveMinuteMetrics(ctx, loc, now)
	metrics := PlatformStatusMetrics{
		Current5mBucketTokens:     bundle.currentTokens,
		TodayMaxCompleted5mTokens: bundle.peakTokens,
		CurrentBucketStart:        bundle.currentStart,
		CurrentBucketEnd:          bundle.currentEnd,
		PeakBucketStart:           bundle.peakStart,
		PeakBucketEnd:             bundle.peakEnd,
		AsOf:                      now,
		Timezone:                  cacheKey,
	}

	s.statusMu.Lock()
	if err == nil {
		s.statusCache[cacheKey] = platformStatusCacheEntry{
			metrics: metrics,
			expires: now.Add(platformStatusTTL),
		}
	}
	delete(s.statusInflight, cacheKey)
	call.val = metrics
	call.wg.Done()
	s.statusMu.Unlock()

	if err != nil {
		return fiveMinuteBundle{}, err
	}
	return bundle, nil
}

func fiveMinuteBundleFromMetrics(m PlatformStatusMetrics) fiveMinuteBundle {
	return fiveMinuteBundle{
		currentTokens: m.Current5mBucketTokens,
		peakTokens:    m.TodayMaxCompleted5mTokens,
		currentStart:  m.CurrentBucketStart,
		currentEnd:    m.CurrentBucketEnd,
		peakStart:     m.PeakBucketStart,
		peakEnd:       m.PeakBucketEnd,
	}
}

func (s *DashboardService) computeFiveMinuteMetrics(ctx context.Context, loc *time.Location, now time.Time) (fiveMinuteBundle, error) {
	ctx, cancel := context.WithTimeout(ctx, platformStatusTimeout)
	defer cancel()

	nowLocal := now.In(loc)
	dayStart := truncateLocalDay(nowLocal)
	currentStart := truncateFiveMinutes(nowLocal)
	currentEnd := currentStart.Add(fiveMinute)

	// Scan from local midnight (UTC instant) so "today" peak is complete.
	points, err := s.loadPlatformUsageSince(ctx, dayStart.UTC().Add(-14*time.Hour))
	if err != nil {
		return fiveMinuteBundle{}, err
	}

	buckets := map[int64]int64{} // bucket start unix → sum
	var hasAny bool
	for _, p := range points {
		local := p.ts.In(loc)
		if local.Before(dayStart) {
			continue
		}
		hasAny = true
		bStart := truncateFiveMinutes(local)
		buckets[bStart.Unix()] += p.total
	}

	cur := buckets[currentStart.Unix()]
	curPtr := &cur

	var peak *int64
	var peakStart, peakEnd *time.Time
	for unix, sum := range buckets {
		bStart := time.Unix(unix, 0).In(loc)
		bEnd := bStart.Add(fiveMinute)
		// Peak = max of completed buckets only (exclude current incomplete).
		if !bEnd.After(currentStart) && (peak == nil || sum > *peak) {
			v := sum
			peak = &v
			ps, pe := bStart, bEnd
			peakStart = &ps
			peakEnd = &pe
		}
	}
	_ = hasAny

	cs, ce := currentStart, currentEnd
	return fiveMinuteBundle{
		currentTokens: curPtr,
		peakTokens:    peak,
		currentStart:  &cs,
		currentEnd:    &ce,
		peakStart:     peakStart,
		peakEnd:       peakEnd,
	}, nil
}

func truncateFiveMinutes(t time.Time) time.Time {
	t = t.In(t.Location())
	m := t.Minute() - (t.Minute() % 5)
	return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), m, 0, 0, t.Location())
}

func (s *DashboardService) loadPlatformUsageSince(ctx context.Context, since time.Time) ([]platformUsagePoint, error) {
	if s.loadPlatformUsageHook != nil {
		s.loadPlatformUsageHook()
	}
	wf, err := s.loadPlatformWorkflowUsageSince(ctx, since)
	if err != nil {
		return nil, err
	}
	pm, err := s.loadPlatformPMUsageSince(ctx, since)
	if err != nil {
		return nil, err
	}
	out := make([]platformUsagePoint, 0, len(wf)+len(pm))
	out = append(out, wf...)
	out = append(out, pm...)
	return out, nil
}

func (s *DashboardService) loadPlatformWorkflowUsageSince(ctx context.Context, since time.Time) ([]platformUsagePoint, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var srs []models.StateRun
	if err := s.db.WithContext(ctx).Model(&models.StateRun{}).
		Select("run_id", "usage", "started_at").
		Where("usage IS NOT NULL").
		Find(&srs).Error; err != nil {
		return nil, err
	}
	if len(srs) == 0 {
		return nil, nil
	}

	runIDs := make([]string, 0, len(srs))
	seen := map[string]struct{}{}
	for _, sr := range srs {
		if _, ok := seen[sr.RunID]; ok {
			continue
		}
		seen[sr.RunID] = struct{}{}
		runIDs = append(runIDs, sr.RunID)
	}

	type runRow struct {
		ID        string
		StartedAt time.Time
	}
	runStarted := map[string]time.Time{}
	for i := 0; i < len(runIDs); i += tokenAggChunk {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		end := i + tokenAggChunk
		if end > len(runIDs) {
			end = len(runIDs)
		}
		var chunk []runRow
		if err := s.db.WithContext(ctx).Model(&models.Run{}).
			Select("id", "started_at").
			Where("id IN ?", runIDs[i:end]).
			Find(&chunk).Error; err != nil {
			return nil, err
		}
		for _, r := range chunk {
			runStarted[r.ID] = r.StartedAt
		}
	}

	var out []platformUsagePoint
	for _, sr := range srs {
		if sr.Usage == nil {
			continue
		}
		ts := runStarted[sr.RunID]
		if sr.StartedAt != nil && !sr.StartedAt.IsZero() {
			ts = *sr.StartedAt
		}
		if ts.IsZero() || ts.Before(since) {
			continue
		}
		out = append(out, platformUsagePoint{ts: ts, total: sr.Usage.Total()})
	}
	return out, nil
}

func (s *DashboardService) loadPlatformPMUsageSince(ctx context.Context, since time.Time) ([]platformUsagePoint, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var msgs []models.ChatMessage
	if err := s.db.WithContext(ctx).Model(&models.ChatMessage{}).
		Select("usage", "created_at").
		Where("role = ? AND usage IS NOT NULL AND created_at >= ?", "assistant", since).
		Find(&msgs).Error; err != nil {
		return nil, err
	}
	var out []platformUsagePoint
	for _, m := range msgs {
		if m.Usage == nil || m.CreatedAt.IsZero() {
			continue
		}
		out = append(out, platformUsagePoint{ts: m.CreatedAt, total: m.Usage.Total()})
	}
	return out, nil
}

// ClearPlatformStatusCacheForTest resets the 5m cache (tests only).
func (s *DashboardService) ClearPlatformStatusCacheForTest() {
	s.statusMu.Lock()
	defer s.statusMu.Unlock()
	s.statusCache = nil
	s.statusInflight = nil
}
