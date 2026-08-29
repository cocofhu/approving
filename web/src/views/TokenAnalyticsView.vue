<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import VChart from 'vue-echarts'
import { registerECharts } from '@/components/charts/echartsSetup'
import {
  STATS_CHART_GRID,
  axisTooltip,
  chartTone,
  fmtCompactAxis,
  pieChartOption,
  statsAxis,
  statsLegend,
  statsTooltip,
} from '@/components/charts/chartTheme'
import { api } from '@/lib/api/api'
import type { GlobalTokenStats, TokenStatsWindow } from '@/lib/shared/types'
import { fmtCompactTokenCount, fmtTokenCount } from '@/lib/run/tokenUsage'
import { displayRunTitle } from '@/lib/run/runTitle'
import { truncateText } from '@/lib/shared/format'
import {
  TOKEN_PART_COLORS,
  TOKEN_PART_KEYS,
  TOKEN_SOURCE_COLORS,
  clientTimezoneParams,
  formatBucketLabel,
  type TokenPartKey,
} from '@/components/board/token-stats/tokenStatsShared'
import { useToast } from '@/lib/composables/useToast'
registerECharts()

const { t } = useI18n()
const router = useRouter()
const toast = useToast()

const WINDOWS: TokenStatsWindow[] = ['24h', '7d', '30d', '90d', 'all']

const windowSel = ref<TokenStatsWindow>('30d')
const sourceSel = ref<'all' | 'workflow' | 'pm'>('all')
const projectSel = ref('')
const modelSel = ref('')
const lineMode = ref<'total' | 'project' | 'model'>('total')
const areaMode = ref<'source' | 'comp'>('source')
const loading = ref(true)
const failed = ref(false)
const data = ref<GlobalTokenStats | null>(null)

let abort: AbortController | null = null
let generation = 0

const isEmpty = computed(() => !!data.value?.empty)

const deltaLabel = computed(() => {
  const d = data.value?.kpi.deltaPct
  if (d == null) return t('pages.tokenAnalytics.noPrev')
  const sign = d >= 0 ? '▲' : '▼'
  return `${sign} ${Math.abs(d).toFixed(1)}%`
})

const deltaClass = computed(() => {
  const d = data.value?.kpi.deltaPct
  if (d == null) return 'text-txt3'
  return d >= 0 ? 'text-emerald-600' : 'text-amber-600'
})

const PART_LABEL_KEYS: Record<TokenPartKey, string> = {
  input: 'pages.executionTimeline.partInput',
  output: 'pages.executionTimeline.partOutput',
  cacheRead: 'pages.executionTimeline.partCacheRead',
  cacheWrite: 'pages.executionTimeline.partCacheWrite',
}

function partLabel(key: TokenPartKey): string {
  return t(PART_LABEL_KEYS[key])
}

function bucketLabels(buckets: { bucket: string }[]): string[] {
  const bw = data.value?.bucketWidth || 'day'
  return buckets.map((b) => formatBucketLabel(b.bucket, bw))
}

function lineChartOption() {
  if (!data.value || data.value.empty) return null
  const bw = data.value.bucketWidth
  let series: { name: string; data: number[]; lineStyle?: { type?: string }; color?: string }[] = []
  const labels = bucketLabels(data.value.trend)

  if (lineMode.value === 'total') {
    series = [
      {
        name: t('pages.tokenAnalytics.lineModes.total'),
        data: data.value.trend.map((b) => b.total || 0),
        color: TOKEN_SOURCE_COLORS.workflow,
      },
      {
        name: t('pages.tokenAnalytics.linePrevWindow'),
        data: data.value.prevTrend.map((b) => b.total || 0),
        color: '#c4b5fd',
        lineStyle: { type: 'dashed' },
      },
    ]
  } else if (lineMode.value === 'project') {
    series = data.value.projectTrends.map((s, i) => ({
      name: s.name,
      data: alignSeries(s.trend, data.value!.trend),
      color: ['#4f46e5', '#7c6dff', '#a99cff', '#c9c0ff'][i % 4],
    }))
  } else {
    series = data.value.modelTrends.map((s, i) => ({
      name: s.name,
      data: alignSeries(s.trend, data.value!.trend),
      color: ['#5b4dff', '#818cf8', '#a78bfa', '#94a3b8'][i % 4],
    }))
  }

  return {
    grid: STATS_CHART_GRID,
    tooltip: axisTooltip(),
    legend: statsLegend(),
    xAxis: {
      type: 'category',
      data: labels,
      ...statsAxis(),
      splitLine: { show: false },
      axisLabel: { ...statsAxis().axisLabel, interval: 'auto' },
    },
    yAxis: {
      type: 'value',
      ...statsAxis(),
      axisLabel: { ...statsAxis().axisLabel, formatter: (v: number) => fmtCompactAxis(v) },
    },
    series: series.map((s) => ({
      type: 'line',
      name: s.name,
      data: s.data,
      smooth: true,
      showSymbol: lineMode.value === 'total',
      lineStyle: { width: 2, ...(s.lineStyle || {}) },
      itemStyle: { color: s.color },
      areaStyle: lineMode.value === 'total' && !s.lineStyle ? { opacity: 0.08 } : undefined,
    })),
  }
}

function alignSeries(src: { bucket: string; total: number }[], master: { bucket: string }[]): number[] {
  const map = new Map(src.map((b) => [b.bucket, b.total || 0]))
  return master.map((b) => map.get(b.bucket) || 0)
}

function pieOption(
  slices: { name: string; value: number; key?: string; color?: string }[],
  donut = false,
) {
  return pieChartOption(slices, donut)
}

function buildPieSlices() {
  if (!data.value || data.value.empty) return { source: [], comp: [], proj: [], model: [], node: [], wf: [] }
  const k = data.value.kpi
  const c = data.value.composition
  return {
    source: [
      { name: t('pages.board.tokenStats.workflow'), value: k.workflowTotal, key: 'workflow', color: TOKEN_SOURCE_COLORS.workflow },
      { name: t('pages.board.tokenStats.pm'), value: k.pmTotal, key: 'pm', color: '#8B9CF7' },
    ].filter((x) => x.value > 0),
    comp: TOKEN_PART_KEYS.map((key) => ({
      name: partLabel(key),
      value: (c as Record<string, number>)[`${key}Tokens`] || (key === 'input' ? c.inputTokens : key === 'output' ? c.outputTokens : key === 'cacheRead' ? c.cacheReadTokens : c.cacheWriteTokens) || 0,
      key,
      color: TOKEN_PART_COLORS[key],
    })).filter((x) => x.value > 0),
    proj: data.value.projects.slice(0, 10).map((p, i) => ({
      name: p.name,
      value: p.total,
      key: p.projectId,
      color: ['#4f46e5', '#7c6dff', '#a99cff', '#c9c0ff'][i % 4],
    })),
    model: data.value.modelRanking.filter((m) => !m.other).slice(0, 10).map((m, i) => ({
      name: m.name,
      value: m.total,
      key: m.modelKey,
      color: ['#5b4dff', '#818cf8', '#a78bfa', '#94a3b8'][i % 4],
    })),
    node: data.value.nodeTypes.map((n, i) => ({
      name: n.name,
      value: n.total,
      color: ['#4f46e5', '#7c6dff', '#a99cff'][i % 3],
    })),
    wf: data.value.workflows.filter((w) => !w.other && w.kind === 'workflow').slice(0, 8).map((w, i) => ({
      name: w.name,
      value: w.total,
      color: ['#5b4dff', '#818cf8', '#a78bfa', '#94a3b8'][i % 4],
    })),
  }
}

const pieSlices = computed(() => buildPieSlices())

function barChartOption() {
  if (!data.value?.projects.length) return null
  const projs = data.value.projects.slice(0, 10)
  const partSeries = (key: TokenPartKey) => ({
    type: 'bar' as const,
    stack: 'total',
    name: partLabel(key),
    itemStyle: { color: TOKEN_PART_COLORS[key] },
    data: projs.map((p) => {
      if (key === 'input') return p.inputTokens
      if (key === 'output') return p.outputTokens
      if (key === 'cacheRead') return p.cacheReadTokens || 0
      return p.cacheWriteTokens || 0
    }),
  })
  return {
    grid: STATS_CHART_GRID,
    tooltip: axisTooltip({ axisPointer: { type: 'shadow' } }),
    legend: {
      ...statsLegend(),
      data: TOKEN_PART_KEYS.map((k) => partLabel(k)),
    },
    xAxis: {
      type: 'category',
      data: projs.map((p) => p.name),
      ...statsAxis(),
      splitLine: { show: false },
      axisLabel: { ...statsAxis().axisLabel, interval: 0, rotate: projs.length > 6 ? 30 : 0 },
    },
    yAxis: {
      type: 'value',
      ...statsAxis(),
      axisLabel: { ...statsAxis().axisLabel, formatter: (v: number) => fmtCompactAxis(v) },
    },
    series: TOKEN_PART_KEYS.map((k) => partSeries(k)),
  }
}

function areaChartOption() {
  if (!data.value?.trend.length) return null
  const labels = bucketLabels(data.value.trend)
  if (areaMode.value === 'source') {
    return {
      grid: STATS_CHART_GRID,
      tooltip: axisTooltip(),
      legend: statsLegend(),
      xAxis: {
        type: 'category',
        data: labels,
        ...statsAxis(),
        splitLine: { show: false },
        axisLabel: { ...statsAxis().axisLabel, interval: 'auto' },
      },
      yAxis: {
        type: 'value',
        ...statsAxis(),
        axisLabel: { ...statsAxis().axisLabel, formatter: (v: number) => fmtCompactAxis(v) },
      },
      series: [
        {
          type: 'line',
          stack: 'a',
          areaStyle: { opacity: 0.55 },
          name: t('pages.board.tokenStats.workflow'),
          data: data.value.trend.map((b) => b.workflowTotal || 0),
          itemStyle: { color: TOKEN_SOURCE_COLORS.workflow },
        },
        {
          type: 'line',
          stack: 'a',
          areaStyle: { opacity: 0.55 },
          name: t('pages.board.tokenStats.pm'),
          data: data.value.trend.map((b) => b.pmTotal || 0),
          itemStyle: { color: '#8B9CF7' },
        },
      ],
    }
  }
  return {
    grid: STATS_CHART_GRID,
    tooltip: axisTooltip(),
    legend: statsLegend(),
    xAxis: {
      type: 'category',
      data: labels,
      ...statsAxis(),
      splitLine: { show: false },
      axisLabel: { ...statsAxis().axisLabel, interval: 'auto' },
    },
    yAxis: {
      type: 'value',
      ...statsAxis(),
      axisLabel: { ...statsAxis().axisLabel, formatter: (v: number) => fmtCompactAxis(v) },
    },
    series: TOKEN_PART_KEYS.map((partKey) => ({
      type: 'line',
      stack: 'a',
      areaStyle: { opacity: 0.7 },
      name: partLabel(partKey),
      data: data.value!.trend.map((b) => {
        if (partKey === 'input') return b.inputTokens || 0
        if (partKey === 'output') return b.outputTokens || 0
        if (partKey === 'cacheRead') return b.cacheReadTokens || 0
        return b.cacheWriteTokens || 0
      }),
      itemStyle: { color: TOKEN_PART_COLORS[partKey] },
    })),
  }
}

function heatmapOption() {
  const hm = data.value?.heatmap
  if (!hm?.rows.length || !hm.cols.length) return null
  const flat: [number, number, number][] = []
  hm.grid.forEach((row, ri) => {
    row.forEach((v, ci) => flat.push([ci, ri, v]))
  })
  const max = Math.max(1, ...flat.map((f) => f[2]))
  const tone = chartTone()
  return {
    grid: { left: 176, right: 12, top: 12, bottom: 28, containLabel: false },
    tooltip: statsTooltip({
      position: 'top',
      formatter: (p: { data: [number, number, number] }) => {
      const [x, y, v] = p.data
      const compact = fmtCompactTokenCount(v)
      const exact = fmtTokenCount(v)
      return `${hm.rows[y]} × ${hm.cols[x]}<br/>${compact}<br/><span style="opacity:0.75;font-size:11px">${exact}</span>`
    }}),
    xAxis: {
      type: 'category',
      data: hm.cols,
      splitArea: { show: true },
      axisLabel: { fontSize: 10, color: tone.axisLabel },
    },
    yAxis: {
      type: 'category',
      data: hm.rows,
      splitArea: { show: true },
      axisLabel: { fontSize: 10, color: tone.axisLabel, width: 160, overflow: 'break' },
    },
    visualMap: {
      min: 0,
      max,
      calculable: false,
      orient: 'horizontal',
      left: 'center',
      bottom: 0,
      show: false,
      inRange: { color: [tone.heatLow, tone.heatHigh] },
    },
    series: [{
      type: 'heatmap',
      data: flat,
      label: {
        show: true,
        color: tone.pieLabel,
        formatter: (p: { data: [number, number, number] }) => fmtCompactTokenCount(p.data[2]),
      },
    }],
  }
}

async function load() {
  const gen = ++generation
  abort?.abort()
  abort = new AbortController()
  loading.value = true
  failed.value = false
  data.value = null
  const tz = clientTimezoneParams()
  try {
    const res = await api.getGlobalTokenStats(
      {
        window: windowSel.value,
        timezone: tz.timezone,
        utcOffsetMinutes: tz.utcOffsetMinutes,
        source: sourceSel.value,
        projectId: projectSel.value || undefined,
        modelKey: modelSel.value || undefined,
      },
      { signal: abort.signal },
    )
    if (gen !== generation) return
    data.value = res
    failed.value = false
  } catch (e: unknown) {
    if (gen !== generation) return
    if ((e as { name?: string })?.name === 'AbortError') return
    failed.value = true
    data.value = null
  } finally {
    if (gen === generation) loading.value = false
  }
}

function retry() {
  void load()
}

function clearFilters() {
  sourceSel.value = 'all'
  projectSel.value = ''
  modelSel.value = ''
  void load()
}

function applySource(s: 'all' | 'workflow' | 'pm') {
  sourceSel.value = s
  void load()
}

function onBarClick(params: unknown) {
  const ev = params as { name?: string; componentType?: string }
  if (ev.componentType !== 'series' || !ev.name) return
  const proj = data.value?.projects.find((p) => p.name === ev.name)
  if (!proj) return
  projectSel.value = proj.projectId
  toast.show(t('pages.tokenAnalytics.filterApplied', { name: proj.name }))
  void load()
}

function onPieClick(kind: 'source' | 'project' | 'model', params: unknown) {
  const ev = params as { data?: { key?: string; name?: string } }
  const key = ev.data?.key
  const name = ev.data?.name || key || ''
  if (kind === 'source' && key) {
    sourceSel.value = key === 'workflow' ? 'workflow' : key === 'pm' ? 'pm' : 'all'
  } else if (kind === 'project' && key) {
    projectSel.value = key
  } else if (kind === 'model' && key) {
    modelSel.value = key
  }
  toast.show(t('pages.tokenAnalytics.filterApplied', { name }))
  void load()
}

function goBoard(projectId: string) {
  void router.push({ path: `/projects/${projectId}`, query: { tab: 'board' } })
}

function goRun(runId: string) {
  void router.push(`/runs/${runId}`)
}

const RUN_TITLE_MAX_DISPLAY = 60

/** Session-only widths for the "highest-usage runs" table (g1.2). Refresh restores defaults. */
type RunsColKey = 'run' | 'project' | 'model' | 'total'

const RUNS_COL_DEFS = [
  { key: 'run' as const, labelKey: 'pages.tokenAnalytics.tables.colRun', align: 'left' as const, defaultWidth: 220, minWidth: 80 },
  { key: 'project' as const, labelKey: 'pages.tokenAnalytics.tables.colProject', align: 'left' as const, defaultWidth: 140, minWidth: 64 },
  { key: 'model' as const, labelKey: 'pages.tokenAnalytics.tables.colModel', align: 'left' as const, defaultWidth: 140, minWidth: 64 },
  { key: 'total' as const, labelKey: 'pages.tokenAnalytics.tables.colTotal', align: 'right' as const, defaultWidth: 88, minWidth: 56 },
] as const

const RUNS_COL_MIN: Record<RunsColKey, number> = {
  run: 80,
  project: 64,
  model: 64,
  total: 56,
}

const runsColWidths = ref<Record<RunsColKey, number>>({
  run: 220,
  project: 140,
  model: 140,
  total: 88,
})
const runsColDragging = ref<RunsColKey | null>(null)
let runsColDragStartX = 0
let runsColDragStartW = 0

function clampRunsColWidth(key: RunsColKey, width: number): number {
  return Math.max(RUNS_COL_MIN[key], width)
}

function runsColSashLabel(key: RunsColKey): string {
  const def = RUNS_COL_DEFS.find((c) => c.key === key)!
  return t('pages.tokenAnalytics.tables.resizeCol', { col: t(def.labelKey) })
}

function onRunsColSashPointerDown(key: RunsColKey, e: PointerEvent) {
  runsColDragging.value = key
  runsColDragStartX = e.clientX
  runsColDragStartW = runsColWidths.value[key]
  const el = e.currentTarget as HTMLElement
  if (typeof el.setPointerCapture === 'function') el.setPointerCapture(e.pointerId)
  e.preventDefault()
  e.stopPropagation()
}

function onRunsColSashPointerMove(e: PointerEvent) {
  const key = runsColDragging.value
  if (!key) return
  runsColWidths.value[key] = clampRunsColWidth(key, runsColDragStartW + (e.clientX - runsColDragStartX))
  e.preventDefault()
}

function onRunsColSashPointerUp() {
  runsColDragging.value = null
}

function runTitleReadable(raw: string): string {
  return displayRunTitle(raw).replace(/\s+/g, ' ').trim()
}

function runTitleDisplay(raw: string): string {
  return truncateText(runTitleReadable(raw), RUN_TITLE_MAX_DISPLAY)
}

function runTitleTooltip(raw: string): string | undefined {
  const readable = runTitleReadable(raw)
  return readable.length > RUN_TITLE_MAX_DISPLAY ? readable : undefined
}

onMounted(() => {
  void load()
})

onBeforeUnmount(() => {
  abort?.abort()
  runsColDragging.value = null
})

watch([windowSel], () => void load())
</script>

<template>
  <div class="flex h-full min-h-0 flex-col" data-testid="token-analytics-page">
    <div class="token-analytics-main min-h-0 flex-1 overflow-auto px-5 py-4 pb-14">
      <div class="mb-3 flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 class="m-0 text-[22px] font-semibold">{{ t('pages.tokenAnalytics.title') }}</h1>
          <p class="mt-1 text-xs text-txt3">{{ t('pages.tokenAnalytics.subtitle') }}</p>
        </div>
        <div
          class="flex gap-1 bg-elevated p-1"
          role="group"
          :aria-label="t('pages.board.tokenStats.windowAria')"
          data-testid="token-analytics-window"
        >
          <button
            v-for="w in WINDOWS"
            :key="w"
            type="button"
            class="px-2.5 py-1.5 text-xs"
            :class="windowSel === w ? 'bg-surface font-semibold text-txt shadow-sm' : 'text-txt3'"
            :data-testid="`token-analytics-window-${w}`"
            @click="windowSel = w"
          >
            {{ t(`pages.board.tokenStats.windows.${w}`) }}
          </button>
        </div>
      </div>

      <div class="mb-3 flex flex-wrap gap-2" data-testid="token-analytics-filters">
        <button
          type="button"
          class="chip border px-2.5 py-1.5 text-xs"
          :class="sourceSel === 'all' ? 'border-accent/40 bg-accent-dim font-semibold text-accent-2' : 'border-line bg-surface text-txt2'"
          @click="applySource('all')"
        >
          {{ t('pages.tokenAnalytics.sourceAll') }}
        </button>
        <button
          type="button"
          class="chip border px-2.5 py-1.5 text-xs"
          :class="sourceSel === 'workflow' ? 'border-accent/40 bg-accent-dim font-semibold text-accent-2' : 'border-line bg-surface text-txt2'"
          @click="applySource('workflow')"
        >
          {{ t('pages.tokenAnalytics.sourceWorkflow') }}
        </button>
        <button
          type="button"
          class="chip border px-2.5 py-1.5 text-xs"
          :class="sourceSel === 'pm' ? 'border-accent/40 bg-accent-dim font-semibold text-accent-2' : 'border-line bg-surface text-txt2'"
          @click="applySource('pm')"
        >
          {{ t('pages.tokenAnalytics.sourcePm') }}
        </button>
        <select v-model="projectSel" class="border border-line bg-surface px-2 py-1.5 text-xs text-txt2" @change="load()">
          <option value="">{{ t('pages.tokenAnalytics.projectAll') }}</option>
          <option v-for="p in data?.filterOptions.projects || []" :key="p.key" :value="p.key">{{ p.name }}</option>
        </select>
        <select v-model="modelSel" class="border border-line bg-surface px-2 py-1.5 text-xs text-txt2" @change="load()">
          <option value="">{{ t('pages.tokenAnalytics.modelAll') }}</option>
          <option v-for="m in data?.filterOptions.models || []" :key="m.key" :value="m.key">{{ m.name }}</option>
        </select>
        <button type="button" class="border border-line bg-surface px-2.5 py-1.5 text-xs text-txt2" @click="clearFilters">
          {{ t('pages.tokenAnalytics.clearFilters') }}
        </button>
      </div>

      <div v-if="loading" class="py-16 text-center text-sm text-txt3" data-testid="token-analytics-loading">
        {{ t('pages.tokenAnalytics.loading') }}
      </div>
      <div v-else-if="failed" class="py-16 text-center" data-testid="token-analytics-error">
        <p class="text-sm text-txt3">{{ t('pages.tokenAnalytics.loadFailed') }}</p>
        <button type="button" class="mt-2 text-sm text-accent-2" @click="retry">{{ t('pages.tokenAnalytics.retry') }}</button>
      </div>
      <div v-else-if="isEmpty" class="py-16 text-center" data-testid="token-analytics-empty">
        <p class="font-medium">{{ t('pages.tokenAnalytics.emptyTitle') }}</p>
        <p class="mt-1 text-sm text-txt3">{{ t('pages.tokenAnalytics.emptyHint') }}</p>
      </div>
      <template v-else-if="data">
        <section id="overview" class="mb-3 grid grid-cols-1 gap-2.5 sm:grid-cols-2 xl:grid-cols-3" data-testid="token-analytics-kpis">
          <div class="border border-line bg-surface p-3.5" data-testid="token-analytics-kpi-total">
            <div class="text-[11px] text-txt3">{{ t('pages.tokenAnalytics.kpiTotal') }}</div>
            <div class="mt-1 text-[22px] font-bold tabular-nums">{{ fmtCompactTokenCount(data.kpi.total) }}</div>
            <div class="mt-1 text-[11px]" :class="deltaClass">{{ deltaLabel }}</div>
          </div>
          <div
            class="token-analytics-kpi-merge relative border border-line bg-surface p-3.5 outline-none"
            tabindex="0"
            data-testid="token-analytics-kpi-merge"
          >
            <div class="text-[11px] text-txt3">{{ t('pages.tokenAnalytics.kpiInOutCache') }}</div>
            <div class="mt-2.5 flex justify-between gap-3 text-[13px] text-txt2">
              <span>{{ t('pages.executionTimeline.partInput') }}</span>
              <b class="text-base font-bold tabular-nums text-txt">{{ fmtCompactTokenCount(data.kpi.inputTokens) }}</b>
            </div>
            <div class="mt-2.5 flex justify-between gap-3 text-[13px] text-txt2">
              <span>{{ t('pages.executionTimeline.partOutput') }}</span>
              <b class="text-base font-bold tabular-nums text-txt">{{ fmtCompactTokenCount(data.kpi.outputTokens) }}</b>
            </div>
            <div
              class="token-analytics-kpi-tip absolute left-3.5 top-[calc(100%-8px)] z-10 hidden min-w-[200px] border border-line bg-elevated p-2.5 text-xs shadow-md"
              data-testid="token-analytics-kpi-detail"
            >
              <div class="flex justify-between gap-4 py-0.5">
                <span>{{ t('pages.executionTimeline.partInput') }}</span>
                <span class="tabular-nums">{{ fmtTokenCount(data.kpi.inputTokens) }}</span>
              </div>
              <div class="flex justify-between gap-4 py-0.5">
                <span>{{ t('pages.executionTimeline.partOutput') }}</span>
                <span class="tabular-nums">{{ fmtTokenCount(data.kpi.outputTokens) }}</span>
              </div>
              <div class="flex justify-between gap-4 py-0.5">
                <span>{{ t('pages.executionTimeline.partCacheRead') }}</span>
                <span class="tabular-nums">{{ fmtTokenCount(data.kpi.cacheReadTokens) }}</span>
              </div>
              <div class="flex justify-between gap-4 py-0.5">
                <span>{{ t('pages.executionTimeline.partCacheWrite') }}</span>
                <span class="tabular-nums">{{ fmtTokenCount(data.kpi.cacheWriteTokens) }}</span>
              </div>
            </div>
          </div>
          <div class="border border-line bg-surface p-3.5" data-testid="token-analytics-kpi-scope">
            <div class="text-[11px] text-txt3">{{ t('pages.tokenAnalytics.kpiScope') }}</div>
            <div class="mt-1 text-[22px] font-bold">{{ t('pages.tokenAnalytics.kpiProjects', { n: data.kpi.projectCount }) }}</div>
            <div class="mt-1 text-[11px] text-txt3">
              {{ t('pages.tokenAnalytics.kpiRuns', { n: data.kpi.runCount }) }} ·
              {{ t('pages.tokenAnalytics.kpiModels', { n: data.kpi.modelCount }) }}
            </div>
          </div>
        </section>

        <section id="lines" class="mb-3 border border-line bg-surface p-3.5" data-testid="token-analytics-lines">
          <h2 class="m-0 text-sm font-semibold">
            {{ t('pages.tokenAnalytics.charts.lines') }}
            <em class="ml-2 text-[11px] font-normal text-txt3">{{ t('pages.tokenAnalytics.charts.linesHint') }}</em>
          </h2>
          <div class="mt-2 flex flex-wrap gap-1.5">
            <button
              v-for="m in (['total', 'project', 'model'] as const)"
              :key="m"
              type="button"
              class="px-2.5 py-1 text-xs"
              :class="lineMode === m ? 'bg-elevated font-semibold text-txt' : 'text-txt3'"
              @click="lineMode = m"
            >
              {{ t(`pages.tokenAnalytics.lineModes.${m}`) }}
            </button>
          </div>
          <div class="token-analytics-plot mt-2 h-[200px] overflow-visible" data-testid="token-analytics-plot-lines">
            <VChart v-if="lineChartOption()" :option="lineChartOption()!" autoresize class="h-full w-full" />
          </div>
        </section>

        <section id="pies" class="mb-3 border border-line bg-surface p-3.5" data-testid="token-analytics-pies">
          <h2 class="m-0 text-sm font-semibold">
            {{ t('pages.tokenAnalytics.charts.pies') }}
            <em class="ml-2 text-[11px] font-normal text-txt3">{{ t('pages.tokenAnalytics.charts.piesHint') }}</em>
          </h2>
          <div class="mt-2 grid grid-cols-1 gap-2.5 sm:grid-cols-2 xl:grid-cols-4">
            <div v-for="cfg in [
              { key: 'source', label: 'sourcePie', donut: true, kind: 'source' },
              { key: 'comp', label: 'compPie', donut: false, kind: null },
              { key: 'proj', label: 'projPie', donut: true, kind: 'project' },
              { key: 'model', label: 'modelPie', donut: true, kind: 'model' },
            ]" :key="cfg.key" class="text-center">
              <VChart
                v-if="pieOption(pieSlices[cfg.key as keyof typeof pieSlices], cfg.donut)"
                :option="pieOption(pieSlices[cfg.key as keyof typeof pieSlices], cfg.donut)!"
                autoresize
                class="mx-auto h-[168px] w-full"
                @click="cfg.kind ? onPieClick(cfg.kind as 'source' | 'project' | 'model', $event) : undefined"
              />
              <p class="m-0 mt-1.5 text-[13px] font-semibold">{{ t(`pages.tokenAnalytics.charts.${cfg.label}`) }}</p>
            </div>
          </div>
        </section>

        <section id="bars" class="mb-3 border border-line bg-surface p-3.5" data-testid="token-analytics-bars">
          <h2 class="m-0 text-sm font-semibold">
            {{ t('pages.tokenAnalytics.charts.bars') }}
            <em class="ml-2 text-[11px] font-normal text-txt3">{{ t('pages.tokenAnalytics.charts.barsHint') }}</em>
          </h2>
          <div class="token-analytics-plot mt-2 h-[200px] overflow-visible" data-testid="token-analytics-plot-bars">
            <VChart
              v-if="barChartOption()"
              :option="barChartOption()!"
              autoresize
              class="h-full w-full"
              @click="onBarClick"
            />
          </div>
        </section>

        <section id="area" class="mb-3 border border-line bg-surface p-3.5" data-testid="token-analytics-area">
          <h2 class="m-0 text-sm font-semibold">
            {{ t('pages.tokenAnalytics.charts.area') }}
            <em class="ml-2 text-[11px] font-normal text-txt3">{{ t('pages.tokenAnalytics.charts.areaHint') }}</em>
          </h2>
          <div class="mt-2 flex gap-1.5">
            <button
              v-for="m in (['source', 'comp'] as const)"
              :key="m"
              type="button"
              class="px-2.5 py-1 text-xs"
              :class="areaMode === m ? 'bg-elevated font-semibold' : 'text-txt3'"
              @click="areaMode = m"
            >
              {{ t(`pages.tokenAnalytics.areaModes.${m}`) }}
            </button>
          </div>
          <div class="token-analytics-plot mt-2 h-[200px] overflow-visible" data-testid="token-analytics-plot-area">
            <VChart v-if="areaChartOption()" :option="areaChartOption()!" autoresize class="h-full w-full" />
          </div>
        </section>

        <section id="heat" class="mb-3 border border-line bg-surface p-3.5" data-testid="token-analytics-heat">
          <h2 class="m-0 text-sm font-semibold">{{ t('pages.tokenAnalytics.charts.heat') }}</h2>
          <div class="token-analytics-plot mt-2 h-[240px] overflow-visible" data-testid="token-analytics-plot-heat">
            <VChart v-if="heatmapOption()" :option="heatmapOption()!" autoresize class="h-full w-full" />
          </div>
        </section>

        <section id="nodeWf" class="mb-3 border border-line bg-surface p-3.5" data-testid="token-analytics-node-wf">
          <h2 class="m-0 text-sm font-semibold">{{ t('pages.tokenAnalytics.charts.nodeWf') }}</h2>
          <div class="mt-2 grid grid-cols-1 gap-2 sm:grid-cols-2">
            <div>
              <VChart v-if="pieOption(pieSlices.node, true)" :option="pieOption(pieSlices.node, true)!" autoresize class="h-[168px] w-full" />
              <p class="text-center text-[13px] font-semibold">{{ t('pages.tokenAnalytics.charts.nodePie') }}</p>
            </div>
            <div>
              <VChart v-if="pieOption(pieSlices.wf, false)" :option="pieOption(pieSlices.wf, false)!" autoresize class="h-[168px] w-full" />
              <p class="text-center text-[13px] font-semibold">{{ t('pages.tokenAnalytics.charts.wfPie') }}</p>
            </div>
          </div>
        </section>

        <section id="tables" class="grid grid-cols-1 gap-2.5 lg:grid-cols-2">
          <div class="flex max-h-[280px] min-h-0 flex-col border border-line bg-surface p-3.5">
            <h2 class="m-0 mb-2 shrink-0 text-sm font-semibold">{{ t('pages.tokenAnalytics.tables.projects') }}</h2>
            <div class="token-analytics-table-scroll min-h-0 flex-1 overflow-auto">
              <table class="w-full border-collapse text-xs">
                <thead>
                  <tr class="text-txt3">
                    <th class="border-b border-line py-1.5 text-left font-semibold">{{ t('pages.tokenAnalytics.tables.colProject') }}</th>
                    <th class="border-b border-line py-1.5 text-right font-semibold">{{ t('pages.tokenAnalytics.tables.colTotal') }}</th>
                    <th class="border-b border-line py-1.5 text-right font-semibold">{{ t('pages.tokenAnalytics.tables.colInput') }}</th>
                    <th class="border-b border-line py-1.5 text-right font-semibold">{{ t('pages.tokenAnalytics.tables.colOutput') }}</th>
                    <th class="border-b border-line py-1.5 text-right font-semibold">{{ t('pages.tokenAnalytics.tables.colDelta') }}</th>
                  </tr>
                </thead>
                <tbody>
                  <tr v-for="p in data.projects" :key="p.projectId" class="hover:bg-accent-dim/40">
                    <td class="border-b border-line/60 py-1.5">
                      <button type="button" class="text-accent-2" @click="goBoard(p.projectId)">{{ p.name }}</button>
                    </td>
                    <td class="border-b border-line/60 py-1.5 text-right tabular-nums">{{ fmtCompactTokenCount(p.total) }}</td>
                    <td class="border-b border-line/60 py-1.5 text-right tabular-nums">{{ fmtCompactTokenCount(p.inputTokens) }}</td>
                    <td class="border-b border-line/60 py-1.5 text-right tabular-nums">{{ fmtCompactTokenCount(p.outputTokens) }}</td>
                    <td class="border-b border-line/60 py-1.5 text-right tabular-nums">
                      {{ p.deltaPct != null ? `${p.deltaPct >= 0 ? '+' : ''}${p.deltaPct.toFixed(1)}%` : t('pages.tokenAnalytics.noPrev') }}
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>
          </div>
          <div class="flex max-h-[280px] min-h-0 flex-col border border-line bg-surface p-3.5" data-testid="token-analytics-runs-table">
            <h2 class="m-0 mb-2 shrink-0 text-sm font-semibold">{{ t('pages.tokenAnalytics.tables.runs') }}</h2>
            <div class="token-analytics-table-scroll min-h-0 flex-1 overflow-auto">
              <!-- plan coverage: g1.1 / g1.2 / g1.3 — header sashes, session col widths, truncate without misfire -->
              <table
                class="token-analytics-runs-grid min-w-full border-collapse text-xs"
                :class="runsColDragging ? 'select-none' : ''"
                data-testid="token-analytics-runs-grid"
              >
                <colgroup>
                  <col
                    v-for="col in RUNS_COL_DEFS"
                    :key="col.key"
                    :data-testid="`token-analytics-runs-col-${col.key}`"
                    :style="{ width: `${runsColWidths[col.key]}px` }"
                  />
                </colgroup>
                <thead>
                  <tr class="text-txt3">
                    <th
                      v-for="col in RUNS_COL_DEFS"
                      :key="col.key"
                      class="relative select-none border-b border-line py-1.5 font-semibold"
                      :class="col.align === 'right' ? 'text-right' : 'text-left'"
                    >
                      {{ t(col.labelKey) }}
                      <div
                        class="token-analytics-runs-col-sash absolute top-0 z-[2] h-full w-1.5 cursor-col-resize touch-none hover:bg-accent"
                        :class="runsColDragging === col.key ? 'bg-accent' : ''"
                        role="separator"
                        aria-orientation="vertical"
                        :aria-valuemin="col.minWidth"
                        :aria-valuenow="runsColWidths[col.key]"
                        :aria-label="runsColSashLabel(col.key)"
                        :title="runsColSashLabel(col.key)"
                        :data-testid="`token-analytics-runs-col-sash-${col.key}`"
                        @pointerdown="onRunsColSashPointerDown(col.key, $event)"
                        @pointermove="onRunsColSashPointerMove"
                        @pointerup="onRunsColSashPointerUp"
                        @pointercancel="onRunsColSashPointerUp"
                      />
                    </th>
                  </tr>
                </thead>
                <tbody>
                  <tr v-for="r in data.topRuns" :key="r.runId" class="hover:bg-accent-dim/40">
                    <td class="overflow-hidden text-ellipsis whitespace-nowrap border-b border-line/60 py-1.5">
                      <button
                        type="button"
                        class="block w-full max-w-full truncate text-left text-accent-2"
                        :title="runTitleTooltip(r.title)"
                        @click="goRun(r.runId)"
                      >{{ runTitleDisplay(r.title) }}</button>
                    </td>
                    <td class="overflow-hidden text-ellipsis whitespace-nowrap border-b border-line/60 py-1.5">{{ r.projectName }}</td>
                    <td class="overflow-hidden text-ellipsis whitespace-nowrap border-b border-line/60 py-1.5">{{ r.modelName || r.modelKey }}</td>
                    <td class="overflow-hidden whitespace-nowrap border-b border-line/60 py-1.5 text-right tabular-nums">{{ fmtCompactTokenCount(r.total) }}</td>
                  </tr>
                </tbody>
              </table>
            </div>
          </div>
        </section>
      </template>
    </div>
  </div>
</template>

<style scoped>
.token-analytics-main,
.token-analytics-table-scroll {
  scrollbar-width: thin;
  scrollbar-color: rgb(var(--c-line)) rgb(var(--c-base));
}
.token-analytics-main::-webkit-scrollbar,
.token-analytics-table-scroll::-webkit-scrollbar {
  width: 8px;
  height: 8px;
}
.token-analytics-main::-webkit-scrollbar-thumb,
.token-analytics-table-scroll::-webkit-scrollbar-thumb {
  background: rgb(var(--c-line));
}
.token-analytics-main::-webkit-scrollbar-track,
.token-analytics-table-scroll::-webkit-scrollbar-track {
  background: rgb(var(--c-base));
}
.token-analytics-kpi-merge:hover,
.token-analytics-kpi-merge:focus-within {
  border-color: #a1a1aa;
}
.token-analytics-kpi-merge:hover .token-analytics-kpi-tip,
.token-analytics-kpi-merge:focus .token-analytics-kpi-tip,
.token-analytics-kpi-merge:focus-within .token-analytics-kpi-tip {
  display: block;
}
.token-analytics-runs-grid {
  table-layout: fixed;
  width: max-content;
}
.token-analytics-runs-col-sash {
  right: -3px;
}
</style>
