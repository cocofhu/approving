package services

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/cocofhu/approving/internal/models"
)

const (
	GlobalTokenStatsSourceAll      = "all"
	GlobalTokenStatsSourceWorkflow = "workflow"
	GlobalTokenStatsSourcePM       = "pm"

	globalTokenStatsTopProjects = 10
	globalTokenStatsTopModels   = 10
	globalTokenStatsTopWorkflows = 10
	globalTokenStatsTopRuns     = 20
)

// GlobalTokenStatsQuery filters cross-project token aggregation.
type GlobalTokenStatsQuery struct {
	Window           string // 7d|30d|90d|all
	Timezone         string
	UTCOffsetMinutes *int
	Source           string // all|workflow|pm
	ProjectID        string
	ModelKey         string
	Now              time.Time
}

// GlobalTokenStatsKPI is overview counters for the analytics page.
type GlobalTokenStatsKPI struct {
	Total            int64    `json:"total"`
	PrevTotal        *int64   `json:"prevTotal,omitempty"`
	DeltaPct         *float64 `json:"deltaPct,omitempty"`
	InputTokens      int64    `json:"inputTokens"`
	OutputTokens     int64    `json:"outputTokens"`
	CacheReadTokens  int64    `json:"cacheReadTokens"`
	CacheWriteTokens int64    `json:"cacheWriteTokens"`
	WorkflowTotal    int64    `json:"workflowTotal"`
	PmTotal          int64    `json:"pmTotal"`
	ProjectCount     int      `json:"projectCount"`
	RunCount         int      `json:"runCount"`
	ModelCount       int      `json:"modelCount"`
}

// GlobalTokenStatsProjectRow is one project breakdown row.
type GlobalTokenStatsProjectRow struct {
	ProjectID        string   `json:"projectId"`
	Name             string   `json:"name"`
	Total            int64    `json:"total"`
	InputTokens      int64    `json:"inputTokens"`
	OutputTokens     int64    `json:"outputTokens"`
	CacheReadTokens  int64    `json:"cacheReadTokens"`
	CacheWriteTokens int64    `json:"cacheWriteTokens"`
	DeltaPct         *float64 `json:"deltaPct,omitempty"`
}

// GlobalTokenStatsRunRow is a Top-N run consumption row.
type GlobalTokenStatsRunRow struct {
	RunID        string `json:"runId"`
	Title        string `json:"title"`
	ProjectID    string `json:"projectId"`
	ProjectName  string `json:"projectName"`
	WorkflowName string `json:"workflowName"`
	ModelKey     string `json:"modelKey"`
	ModelName    string `json:"modelName"`
	Total        int64  `json:"total"`
}

// GlobalTokenStatsNamedBucket is a generic name→total slice (node types, etc.).
type GlobalTokenStatsNamedBucket struct {
	Name  string `json:"name"`
	Total int64  `json:"total"`
	Other bool   `json:"other,omitempty"`
}

// GlobalTokenStatsHeatmap is model×project matrix (TopN + other).
type GlobalTokenStatsHeatmap struct {
	Rows []string  `json:"rows"`
	Cols []string  `json:"cols"`
	Grid [][]int64 `json:"grid"`
}

// GlobalTokenStatsSeries is a multi-line trend group (Top projects/models).
type GlobalTokenStatsSeries struct {
	Key   string             `json:"key"`
	Name  string             `json:"name"`
	Trend []TokenStatsBucket `json:"trend"`
}

// GlobalTokenStatsFilterOption is a dropdown option for project/model filters.
type GlobalTokenStatsFilterOption struct {
	Key  string `json:"key"`
	Name string `json:"name"`
}

// GlobalTokenStatsFilterOptions lists available filter values in the window.
type GlobalTokenStatsFilterOptions struct {
	Projects []GlobalTokenStatsFilterOption `json:"projects"`
	Models   []GlobalTokenStatsFilterOption `json:"models"`
}

// GlobalTokenStatsResult is GET /api/stats/token payload.
type GlobalTokenStatsResult struct {
	Window         string                        `json:"window"`
	BucketWidth    string                        `json:"bucketWidth"`
	Timezone       string                        `json:"timezone"`
	Empty          bool                          `json:"empty"`
	KPI            GlobalTokenStatsKPI           `json:"kpi"`
	Trend          []TokenStatsBucket            `json:"trend"`
	PrevTrend      []TokenStatsBucket            `json:"prevTrend"`
	Composition    TokenStatsComposition         `json:"composition"`
	Projects       []GlobalTokenStatsProjectRow  `json:"projects"`
	ModelRanking   []TokenStatsModel             `json:"modelRanking"`
	NodeTypes      []GlobalTokenStatsNamedBucket `json:"nodeTypes"`
	Workflows      []TokenStatsWorkflow          `json:"workflows"`
	Heatmap        GlobalTokenStatsHeatmap       `json:"heatmap"`
	TopRuns        []GlobalTokenStatsRunRow      `json:"topRuns"`
	ProjectTrends  []GlobalTokenStatsSeries      `json:"projectTrends"`
	ModelTrends    []GlobalTokenStatsSeries      `json:"modelTrends"`
	FilterOptions  GlobalTokenStatsFilterOptions `json:"filterOptions"`
}

type globalProjOpt struct {
	id, name string
}

type globalTokenUsageRow struct {
	ts           time.Time
	usage        models.TokenUsage
	byModel      models.TokenUsageByModel
	projectID    string
	projectName  string
	runID        string
	runTitle     string
	workflowID   string
	workflowName string
	nodeType     string
	source       string
}

type windowSlice struct {
	start    time.Time
	end      time.Time
	hasStart bool
}

// GlobalTokenStats aggregates usage across all projects with optional filters.
func (s *ProjectService) GlobalTokenStats(ctx context.Context, q GlobalTokenStatsQuery) (GlobalTokenStatsResult, error) {
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
		return GlobalTokenStatsResult{}, err
	}

	loc, tzLabel, err := resolveTokenStatsLocation(q.Timezone, q.UTCOffsetMinutes)
	if err != nil {
		return GlobalTokenStatsResult{}, err
	}

	now := q.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	nowLocal := now.In(loc)

	sourceFilter := strings.TrimSpace(q.Source)
	if sourceFilter == "" {
		sourceFilter = GlobalTokenStatsSourceAll
	}
	projectFilter := strings.TrimSpace(q.ProjectID)
	modelFilter := strings.TrimSpace(q.ModelKey)

	curWin := buildWindowSlice(nowLocal, days)
	prevWin := buildPrevWindowSlice(curWin, days)

	rows, err := s.loadGlobalTokenUsageRows(ctx)
	if err != nil {
		return GlobalTokenStatsResult{}, err
	}

	unknownAliases := map[string]string{}
	for _, p := range s.List() {
		unknownAliases[p.ID] = ResolveUnknownModelDisplayName(p.UnknownModelDisplayName)
	}

	type projOpt = globalProjOpt
	projSeen := map[string]projOpt{}
	modelSeen := map[string]string{}

	var filtered []globalTokenUsageRow
	for _, row := range rows {
		if err := ctx.Err(); err != nil {
			return GlobalTokenStatsResult{}, ErrTokenStatsTimeout
		}
		u, ok := filterGlobalRowUsage(row, sourceFilter, projectFilter, modelFilter)
		if !ok || u.Total() <= 0 {
			continue
		}
		row.usage = u
		if row.byModel != nil {
			if modelFilter != "" {
				if b, ok := row.byModel[modelFilter]; ok {
					row.byModel = models.TokenUsageByModel{modelFilter: b}
				}
			}
		}
		filtered = append(filtered, row)
		for mk := range row.byModel {
			if mk == "" {
				continue
			}
			effective := rebucketGlobalModelKey(mk, row.projectID, unknownAliases)
			modelSeen[effective] = effective
		}
		projSeen[row.projectID] = projOpt{id: row.projectID, name: row.projectName}
	}

	if len(filtered) == 0 {
		return GlobalTokenStatsResult{
			Window:      window,
			BucketWidth: bucketWidth,
			Timezone:    tzLabel,
			Empty:       true,
			KPI:         GlobalTokenStatsKPI{},
			Trend:       []TokenStatsBucket{},
			PrevTrend:   []TokenStatsBucket{},
			Projects:    []GlobalTokenStatsProjectRow{},
			ModelRanking: []TokenStatsModel{},
			NodeTypes:   []GlobalTokenStatsNamedBucket{},
			Workflows:   []TokenStatsWorkflow{},
			Heatmap:     GlobalTokenStatsHeatmap{Rows: []string{}, Cols: []string{}, Grid: [][]int64{}},
			TopRuns:     []GlobalTokenStatsRunRow{},
			ProjectTrends: []GlobalTokenStatsSeries{},
			ModelTrends:   []GlobalTokenStatsSeries{},
			FilterOptions: buildFilterOptions(projSeen, modelSeen),
		}, nil
	}

	curRows := filterRowsByWindow(filtered, curWin, loc)
	prevRows := filterRowsByWindow(filtered, prevWin, loc)

	if len(curRows) == 0 {
		out := GlobalTokenStatsResult{
			Window:      window,
			BucketWidth: bucketWidth,
			Timezone:    tzLabel,
			Empty:       true,
			FilterOptions: buildFilterOptions(projSeen, modelSeen),
		}
		out.Trend = fillTrendBuckets(nowLocal, curWin, bucketWidth, map[string]*tokenBucketAgg{})
		return out, nil
	}

	curAgg := aggregateGlobalRows(curRows, loc, bucketWidth, nowLocal, curWin, unknownAliases)
	prevAgg := aggregateGlobalRows(prevRows, loc, bucketWidth, nowLocal, prevWin, unknownAliases)

	kpi := buildGlobalKPI(curAgg, prevAgg)
	trend := bucketsFromAgg(curAgg.buckets, nowLocal, curWin, bucketWidth)
	prevTrend := bucketsFromAgg(prevAgg.buckets, nowLocal.AddDate(0, 0, -days), prevWin, bucketWidth)

	projRows, projTrends := buildProjectStats(curAgg, prevAgg, globalTokenStatsTopProjects)
	modelRank, modelTrends := buildGlobalModelStats(curAgg, prevAgg, unknownAliases, globalTokenStatsTopModels)
	nodeTypes := buildNodeTypeStats(curAgg)
	workflows := buildGlobalWorkflowRank(curAgg)
	heatmap := buildHeatmap(curAgg, globalTokenStatsTopModels, globalTokenStatsTopProjects)
	topRuns := buildTopRuns(curAgg, unknownAliases, globalTokenStatsTopRuns)

	return GlobalTokenStatsResult{
		Window:        window,
		BucketWidth:   bucketWidth,
		Timezone:      tzLabel,
		Empty:         false,
		KPI:           kpi,
		Trend:         trend,
		PrevTrend:     prevTrend,
		Composition:   curAgg.composition,
		Projects:      projRows,
		ModelRanking:  modelRank,
		NodeTypes:     nodeTypes,
		Workflows:     workflows,
		Heatmap:       heatmap,
		TopRuns:       topRuns,
		ProjectTrends: projTrends,
		ModelTrends:   modelTrends,
		FilterOptions: buildFilterOptions(projSeen, modelSeen),
	}, nil
}

func buildWindowSlice(nowLocal time.Time, days int) windowSlice {
	if days <= 0 {
		return windowSlice{hasStart: false}
	}
	startDay := truncateLocalDay(nowLocal).AddDate(0, 0, -(days - 1))
	end := nowLocal
	return windowSlice{start: startDay, end: end, hasStart: true}
}

func buildPrevWindowSlice(cur windowSlice, days int) windowSlice {
	if !cur.hasStart || days <= 0 {
		return windowSlice{hasStart: false}
	}
	prevEnd := cur.start.Add(-time.Nanosecond)
	prevStart := truncateLocalDay(prevEnd).AddDate(0, 0, -(days - 1))
	return windowSlice{start: prevStart, end: prevEnd, hasStart: true}
}

func filterGlobalRowUsage(row globalTokenUsageRow, source, projectID, modelKey string) (models.TokenUsage, bool) {
	if projectID != "" && row.projectID != projectID {
		return models.TokenUsage{}, false
	}
	if source == GlobalTokenStatsSourceWorkflow && row.source != TokenStatsKindWorkflow {
		return models.TokenUsage{}, false
	}
	if source == GlobalTokenStatsSourcePM && row.source != TokenStatsKindPM {
		return models.TokenUsage{}, false
	}
	if modelKey != "" {
		if row.byModel == nil {
			return models.TokenUsage{}, false
		}
		b, ok := row.byModel[modelKey]
		if !ok {
			return models.TokenUsage{}, false
		}
		return models.TokenUsage{
			InputTokens:      b.InputTokens,
			OutputTokens:     b.OutputTokens,
			CacheReadTokens:  b.CacheReadTokens,
			CacheWriteTokens: b.CacheWriteTokens,
		}, true
	}
	return row.usage, true
}

func filterRowsByWindow(rows []globalTokenUsageRow, win windowSlice, loc *time.Location) []globalTokenUsageRow {
	if !win.hasStart {
		return rows
	}
	out := make([]globalTokenUsageRow, 0, len(rows))
	for _, row := range rows {
		local := row.ts.In(loc)
		if local.Before(win.start) || local.After(win.end) {
			continue
		}
		out = append(out, row)
	}
	return out
}

type tokenBucketAgg struct {
	input, output, cacheRead, cacheWrite int64
	workflow, pm                         int64
}

type globalAgg struct {
	buckets      map[string]*tokenBucketAgg
	composition  TokenStatsComposition
	workflowTot  int64
	pmTot        int64
	projects     map[string]*globalProjectAgg
	models       map[string]*tokenModelAgg
	nodeTypes    map[string]int64
	workflows    map[string]int64
	wfNames      map[string]string
	runs         map[string]*globalRunAgg
	projBuckets  map[string]map[string]*tokenBucketAgg
	modelBuckets map[string]map[string]*tokenBucketAgg
	heat         map[string]map[string]int64 // modelKey → projectID → total
	runSet       map[string]struct{}
}

type globalProjectAgg struct {
	name                    string
	total                   int64
	input, output           int64
	cacheRead, cacheWrite   int64
}

type globalRunAgg struct {
	runID, title, projectID, projectName, workflowName string
	total                                            int64
	topModelKey, topModelName                        string
}

// rebucketGlobalModelKey merges per-project unknown usage into the configured default model.
func rebucketGlobalModelKey(mk, projectID string, unknownAliases map[string]string) string {
	if mk != models.TokenUsageModelUnknown {
		return mk
	}
	alias := strings.TrimSpace(unknownAliases[projectID])
	if alias != "" && alias != models.TokenUsageModelUnknown {
		return alias
	}
	return mk
}

func aggregateGlobalRows(rows []globalTokenUsageRow, loc *time.Location, bucketWidth string, nowLocal time.Time, win windowSlice, unknownAliases map[string]string) *globalAgg {
	agg := &globalAgg{
		buckets:      map[string]*tokenBucketAgg{},
		projects:     map[string]*globalProjectAgg{},
		models:       map[string]*tokenModelAgg{},
		nodeTypes:    map[string]int64{},
		workflows:    map[string]int64{},
		wfNames:      map[string]string{},
		runs:         map[string]*globalRunAgg{},
		projBuckets:  map[string]map[string]*tokenBucketAgg{},
		modelBuckets: map[string]map[string]*tokenBucketAgg{},
		heat:         map[string]map[string]int64{},
		runSet:       map[string]struct{}{},
	}
	for _, row := range rows {
		local := row.ts.In(loc)
		if win.hasStart && (local.Before(win.start) || local.After(win.end)) {
			continue
		}
		key := bucketKey(local, bucketWidth)
		b := agg.buckets[key]
		if b == nil {
			b = &tokenBucketAgg{}
			agg.buckets[key] = b
		}
		addUsageToBucket(b, row.usage, row.source)

		agg.composition.InputTokens += row.usage.InputTokens
		agg.composition.OutputTokens += row.usage.OutputTokens
		agg.composition.CacheReadTokens += row.usage.CacheReadTokens
		agg.composition.CacheWriteTokens += row.usage.CacheWriteTokens
		agg.composition.Total += row.usage.Total()

		if row.source == TokenStatsKindPM {
			agg.pmTot += row.usage.Total()
		} else {
			agg.workflowTot += row.usage.Total()
		}

		pa := agg.projects[row.projectID]
		if pa == nil {
			pa = &globalProjectAgg{name: row.projectName}
			agg.projects[row.projectID] = pa
		}
		pa.total += row.usage.Total()
		pa.input += row.usage.InputTokens
		pa.output += row.usage.OutputTokens
		pa.cacheRead += row.usage.CacheReadTokens
		pa.cacheWrite += row.usage.CacheWriteTokens

		pb := agg.projBuckets[row.projectID]
		if pb == nil {
			pb = map[string]*tokenBucketAgg{}
			agg.projBuckets[row.projectID] = pb
		}
		pbk := pb[key]
		if pbk == nil {
			pbk = &tokenBucketAgg{}
			pb[key] = pbk
		}
		addUsageToBucket(pbk, row.usage, row.source)

		if row.nodeType != "" {
			agg.nodeTypes[row.nodeType] += row.usage.Total()
		}

		if row.source == TokenStatsKindWorkflow && row.workflowID != "" {
			agg.workflows[row.workflowID] += row.usage.Total()
			if row.workflowName != "" {
				agg.wfNames[row.workflowID] = row.workflowName
			}
		}

		if row.runID != "" {
			agg.runSet[row.runID] = struct{}{}
			ra := agg.runs[row.runID]
			if ra == nil {
				ra = &globalRunAgg{
					runID: row.runID, title: row.runTitle,
					projectID: row.projectID, projectName: row.projectName,
					workflowName: row.workflowName,
				}
				agg.runs[row.runID] = ra
			}
			ra.total += row.usage.Total()
		}

		by := row.byModel
		if by == nil {
			by = models.EffectiveUsageByModel(&row.usage, nil)
		}
		for mk, bu := range by {
			tot := bu.Total()
			if tot <= 0 {
				continue
			}
			targetKey := rebucketGlobalModelKey(mk, row.projectID, unknownAliases)
			ma := agg.models[targetKey]
			if ma == nil {
				ma = &tokenModelAgg{}
				agg.models[targetKey] = ma
			}
			ma.total += tot
			if bu.Filled {
				ma.filled = true
			}
			if bu.Source != "" {
				ma.source = bu.Source
			}

			mb := agg.modelBuckets[targetKey]
			if mb == nil {
				mb = map[string]*tokenBucketAgg{}
				agg.modelBuckets[targetKey] = mb
			}
			mbk := mb[key]
			if mbk == nil {
				mbk = &tokenBucketAgg{}
				mb[key] = mbk
			}
			mbk.input += bu.InputTokens
			mbk.output += bu.OutputTokens
			mbk.cacheRead += bu.CacheReadTokens
			mbk.cacheWrite += bu.CacheWriteTokens
			mbk.workflow += tot

			hm := agg.heat[targetKey]
			if hm == nil {
				hm = map[string]int64{}
				agg.heat[targetKey] = hm
			}
			hm[row.projectID] += tot

			if row.runID != "" {
				ra := agg.runs[row.runID]
				if ra != nil && tot >= ra.total/2 {
					ra.topModelKey = targetKey
				}
			}
		}
	}
	return agg
}

func addUsageToBucket(b *tokenBucketAgg, u models.TokenUsage, source string) {
	b.input += u.InputTokens
	b.output += u.OutputTokens
	b.cacheRead += u.CacheReadTokens
	b.cacheWrite += u.CacheWriteTokens
	tot := u.Total()
	if source == TokenStatsKindPM {
		b.pm += tot
	} else {
		b.workflow += tot
	}
}

func bucketsFromAgg(buckets map[string]*tokenBucketAgg, nowLocal time.Time, win windowSlice, bucketWidth string) []TokenStatsBucket {
	present := map[string]struct{}{}
	for k := range buckets {
		present[k] = struct{}{}
	}
	var start time.Time
	hasStart := win.hasStart
	if hasStart {
		start = win.start
	}
	keys := fillBucketKeys(nowLocal, start, hasStart, bucketWidth, present)
	out := make([]TokenStatsBucket, 0, len(keys))
	for _, k := range keys {
		b := buckets[k]
		if b == nil {
			out = append(out, TokenStatsBucket{Bucket: k})
			continue
		}
		out = append(out, TokenStatsBucket{
			Bucket:           k,
			Total:            b.input + b.output + b.cacheRead + b.cacheWrite,
			WorkflowTotal:    b.workflow,
			PmTotal:          b.pm,
			InputTokens:      b.input,
			OutputTokens:     b.output,
			CacheReadTokens:  b.cacheRead,
			CacheWriteTokens: b.cacheWrite,
		})
	}
	return out
}

func fillTrendBuckets(nowLocal time.Time, win windowSlice, bucketWidth string, buckets map[string]*tokenBucketAgg) []TokenStatsBucket {
	return bucketsFromAgg(buckets, nowLocal, win, bucketWidth)
}

func buildGlobalKPI(cur, prev *globalAgg) GlobalTokenStatsKPI {
	kpi := GlobalTokenStatsKPI{
		Total:            cur.composition.Total,
		InputTokens:      cur.composition.InputTokens,
		OutputTokens:     cur.composition.OutputTokens,
		CacheReadTokens:  cur.composition.CacheReadTokens,
		CacheWriteTokens: cur.composition.CacheWriteTokens,
		WorkflowTotal:    cur.workflowTot,
		PmTotal:          cur.pmTot,
		ProjectCount:     len(cur.projects),
		RunCount:         len(cur.runSet),
		ModelCount:       len(cur.models),
	}
	if prev != nil && prev.composition.Total > 0 {
		pt := prev.composition.Total
		kpi.PrevTotal = &pt
		delta := (float64(cur.composition.Total-prev.composition.Total) / float64(prev.composition.Total)) * 100
		kpi.DeltaPct = &delta
	}
	return kpi
}

func buildProjectStats(cur, prev *globalAgg, topN int) ([]GlobalTokenStatsProjectRow, []GlobalTokenStatsSeries) {
	type item struct {
		id              string
		name            string
		total           int64
		in, out         int64
		cacheRead, cacheWrite int64
	}
	list := make([]item, 0, len(cur.projects))
	for id, p := range cur.projects {
		list = append(list, item{
			id: id, name: p.name, total: p.total,
			in: p.input, out: p.output,
			cacheRead: p.cacheRead, cacheWrite: p.cacheWrite,
		})
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].total != list[j].total {
			return list[i].total > list[j].total
		}
		return list[i].id < list[j].id
	})

	rows := make([]GlobalTokenStatsProjectRow, 0, len(list))
	series := make([]GlobalTokenStatsSeries, 0, topN)
	for i, it := range list {
		row := GlobalTokenStatsProjectRow{
			ProjectID: it.id, Name: it.name, Total: it.total,
			InputTokens: it.in, OutputTokens: it.out,
			CacheReadTokens: it.cacheRead, CacheWriteTokens: it.cacheWrite,
		}
		if prev != nil {
			if pp, ok := prev.projects[it.id]; ok && pp.total > 0 {
				delta := (float64(it.total-pp.total) / float64(pp.total)) * 100
				row.DeltaPct = &delta
			}
		}
		rows = append(rows, row)
		if i < topN {
			series = append(series, GlobalTokenStatsSeries{
				Key: it.id, Name: it.name,
				Trend: projectTrendFromBuckets(cur.projBuckets[it.id]),
			})
		}
	}
	return rows, series
}

func projectTrendFromBuckets(buckets map[string]*tokenBucketAgg) []TokenStatsBucket {
	if len(buckets) == 0 {
		return []TokenStatsBucket{}
	}
	keys := make([]string, 0, len(buckets))
	for k := range buckets {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]TokenStatsBucket, 0, len(keys))
	for _, k := range keys {
		b := buckets[k]
		out = append(out, TokenStatsBucket{
			Bucket: k,
			Total:  b.input + b.output + b.cacheRead + b.cacheWrite,
			WorkflowTotal: b.workflow,
			PmTotal: b.pm,
		})
	}
	return out
}

func buildGlobalModelStats(cur, prev *globalAgg, unknownAliases map[string]string, topN int) ([]TokenStatsModel, []GlobalTokenStatsSeries) {
	_, ranking := buildModelStats(cur.models, "")
	series := make([]GlobalTokenStatsSeries, 0, topN)
	for i, m := range ranking {
		if m.Other {
			continue
		}
		if i >= topN {
			break
		}
		name := m.Name
		if m.Unknown {
			for _, alias := range unknownAliases {
				if alias != "" {
					name = alias
					break
				}
			}
		}
		series = append(series, GlobalTokenStatsSeries{
			Key: m.ModelKey, Name: name,
			Trend: projectTrendFromBuckets(cur.modelBuckets[m.ModelKey]),
		})
	}
	return ranking, series
}

func buildNodeTypeStats(cur *globalAgg) []GlobalTokenStatsNamedBucket {
	type item struct {
		name  string
		total int64
	}
	list := make([]item, 0, len(cur.nodeTypes))
	for n, t := range cur.nodeTypes {
		list = append(list, item{name: n, total: t})
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].total != list[j].total {
			return list[i].total > list[j].total
		}
		return list[i].name < list[j].name
	})
	out := make([]GlobalTokenStatsNamedBucket, 0, len(list))
	for _, it := range list {
		out = append(out, GlobalTokenStatsNamedBucket{Name: it.name, Total: it.total})
	}
	return out
}

func buildGlobalWorkflowRank(cur *globalAgg) []TokenStatsWorkflow {
	totals := cur.workflows
	names := cur.wfNames
	return buildConsumptionRank(totals, names, 0, false)
}

func buildHeatmap(cur *globalAgg, topModels, topProjects int) GlobalTokenStatsHeatmap {
	type mItem struct{ key, name string; total int64 }
	type pItem struct{ id, name string; total int64 }

	modelList := make([]mItem, 0, len(cur.models))
	for k, a := range cur.models {
		if a == nil {
			continue
		}
		modelList = append(modelList, mItem{key: k, name: k, total: a.total})
	}
	sort.Slice(modelList, func(i, j int) bool {
		if modelList[i].total != modelList[j].total {
			return modelList[i].total > modelList[j].total
		}
		return modelList[i].key < modelList[j].key
	})

	projList := make([]pItem, 0, len(cur.projects))
	for id, p := range cur.projects {
		projList = append(projList, pItem{id: id, name: p.name, total: p.total})
	}
	sort.Slice(projList, func(i, j int) bool {
		if projList[i].total != projList[j].total {
			return projList[i].total > projList[j].total
		}
		return projList[i].id < projList[j].id
	})

	modelKeys := make([]string, 0, topModels+1)
	modelNames := make([]string, 0, topModels+1)
	var modelOther int64
	for i, m := range modelList {
		if i < topModels {
			modelKeys = append(modelKeys, m.key)
			modelNames = append(modelNames, m.name)
		} else {
			modelOther += m.total
		}
	}
	if modelOther > 0 {
		modelKeys = append(modelKeys, "_other")
		modelNames = append(modelNames, "other")
	}

	projIDs := make([]string, 0, topProjects+1)
	projNames := make([]string, 0, topProjects+1)
	var projOtherID string
	for i, p := range projList {
		if i < topProjects {
			projIDs = append(projIDs, p.id)
			projNames = append(projNames, p.name)
		} else if projOtherID == "" {
			projOtherID = "_other"
		}
	}

	grid := make([][]int64, len(modelKeys))
	for mi, mk := range modelKeys {
		row := make([]int64, len(projIDs)+boolToInt(projOtherID != ""))
		for pi, pid := range projIDs {
			if mk == "_other" {
				var sum int64
				for j, m := range modelList {
					if j >= topModels {
						if hm, ok := cur.heat[m.key]; ok {
							sum += hm[pid]
						}
					}
				}
				row[pi] = sum
			} else if hm, ok := cur.heat[mk]; ok {
				row[pi] = hm[pid]
			}
		}
		if projOtherID != "" {
			idx := len(projIDs)
			var sum int64
			for j, p := range projList {
				if j >= topProjects {
					if mk == "_other" {
						for jj, m := range modelList {
							if jj >= topModels {
								if hm, ok := cur.heat[m.key]; ok {
									sum += hm[p.id]
								}
							}
						}
					} else if hm, ok := cur.heat[mk]; ok {
						sum += hm[p.id]
					}
				}
			}
			row[idx] = sum
		}
		grid[mi] = row
	}

	cols := projNames
	if projOtherID != "" {
		cols = append(cols, "other")
	}
	return GlobalTokenStatsHeatmap{Rows: modelNames, Cols: cols, Grid: grid}
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func buildTopRuns(cur *globalAgg, unknownAliases map[string]string, topN int) []GlobalTokenStatsRunRow {
	type item struct {
		*globalRunAgg
	}
	list := make([]item, 0, len(cur.runs))
	for _, r := range cur.runs {
		list = append(list, item{r})
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].total != list[j].total {
			return list[i].total > list[j].total
		}
		return list[i].runID < list[j].runID
	})
	out := make([]GlobalTokenStatsRunRow, 0, topN)
	for i, it := range list {
		if i >= topN {
			break
		}
		name := it.topModelKey
		if it.topModelKey == models.TokenUsageModelUnknown {
			name = unknownAliases[it.projectID]
		}
		title := it.title
		if title == "" {
			title = it.workflowName
		}
		if title == "" {
			title = it.runID
		}
		out = append(out, GlobalTokenStatsRunRow{
			RunID: it.runID, Title: title,
			ProjectID: it.projectID, ProjectName: it.projectName,
			WorkflowName: it.workflowName,
			ModelKey: it.topModelKey, ModelName: name,
			Total: it.total,
		})
	}
	return out
}

func buildFilterOptions(projects map[string]globalProjOpt, models map[string]string) GlobalTokenStatsFilterOptions {
	pList := make([]GlobalTokenStatsFilterOption, 0, len(projects))
	for _, p := range projects {
		pList = append(pList, GlobalTokenStatsFilterOption{Key: p.id, Name: p.name})
	}
	sort.Slice(pList, func(i, j int) bool { return pList[i].Name < pList[j].Name })

	mList := make([]GlobalTokenStatsFilterOption, 0, len(models))
	for k, n := range models {
		mList = append(mList, GlobalTokenStatsFilterOption{Key: k, Name: n})
	}
	sort.Slice(mList, func(i, j int) bool { return mList[i].Name < mList[j].Name })

	return GlobalTokenStatsFilterOptions{Projects: pList, Models: mList}
}

func (s *ProjectService) loadGlobalTokenUsageRows(ctx context.Context) ([]globalTokenUsageRow, error) {
	projects := s.List()
	if len(projects) == 0 {
		return nil, nil
	}
	out := make([]globalTokenUsageRow, 0, 256)
	for _, p := range projects {
		if err := ctx.Err(); err != nil {
			return nil, ErrTokenStatsTimeout
		}
		wfRows, err := s.loadGlobalWorkflowTokenUsageRows(ctx, p.ID, p.Name)
		if err != nil {
			return nil, err
		}
		pmRows, err := s.loadGlobalPMTokenUsageRows(ctx, p.ID, p.Name)
		if err != nil {
			return nil, err
		}
		out = append(out, wfRows...)
		out = append(out, pmRows...)
	}
	return out, nil
}

func (s *ProjectService) loadGlobalWorkflowTokenUsageRows(ctx context.Context, projectID, projectName string) ([]globalTokenUsageRow, error) {
	var wfIDs []string
	if err := s.db.WithContext(ctx).Model(&models.WorkflowDef{}).
		Select("id").Where("project_id = ?", projectID).Pluck("id", &wfIDs).Error; err != nil {
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
		Title        string
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
			Select("id", "workflow_id", "workflow_name", "title", "started_at").
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

	var out []globalTokenUsageRow
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
			Select("run_id", "node_type", "usage", "usage_by_model", "started_at").
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
			out = append(out, globalTokenUsageRow{
				ts:           ts,
				usage:        *sr.Usage,
				byModel:      models.EffectiveUsageByModel(sr.Usage, sr.UsageByModel),
				projectID:    projectID,
				projectName:  projectName,
				runID:        meta.ID,
				runTitle:     meta.Title,
				workflowID:   meta.WorkflowID,
				workflowName: meta.WorkflowName,
				nodeType:     sr.NodeType,
				source:       TokenStatsKindWorkflow,
			})
		}
	}
	return out, nil
}

func (s *ProjectService) loadGlobalPMTokenUsageRows(ctx context.Context, projectID, projectName string) ([]globalTokenUsageRow, error) {
	var threadIDs []string
	if err := s.db.WithContext(ctx).Model(&models.ChatThread{}).
		Select("id").Where("project_id = ?", projectID).Pluck("id", &threadIDs).Error; err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			return nil, ErrTokenStatsTimeout
		}
		return nil, err
	}
	if len(threadIDs) == 0 {
		return nil, nil
	}

	var out []globalTokenUsageRow
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
			out = append(out, globalTokenUsageRow{
				ts:          m.CreatedAt,
				usage:       *m.Usage,
				byModel:     models.EffectiveUsageByModel(m.Usage, m.UsageByModel),
				projectID:   projectID,
				projectName: projectName,
				source:      TokenStatsKindPM,
			})
		}
	}
	return out, nil
}
