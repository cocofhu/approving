<script setup lang="ts">
import { computed, ref, useId, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import StatsPieChart from './StatsPieChart.vue'
import Icon from '../ui/Icon.vue'
import TruncatedTextTooltip from '../ui/TruncatedTextTooltip.vue'
import { api, isPaginated } from '@/lib/api'
import { NODE_DEFS } from '@/data/nodeRegistry'
import { fmtCompactDuration, fmtDuration } from '@/lib/format'
import { resolveNodeDisplayLabel } from '@/lib/resolveNodeDisplayLabel'
import {
  aggregateMultiRuns,
  aggregateSingleRun,
  bottleneckDisplayName,
  flattenProcesses,
  resolveRunWallSec,
  type MultiDimension,
  type SingleDimension,
} from '@/lib/runStats'
import {
  fmtCompactTokenCount,
  fmtCompactTokenRate,
  fmtTokenCount,
  fmtTokenRate,
  mergeTokenUsage,
} from '@/lib/tokenUsage'
import type { Run, TokenUsage, WFNode } from '@/lib/types'

type StatsTab = 'single' | 'multi'
type KpiTipId = 'wall' | 'nodeSum' | 'gap' | 'tokens' | 'rate'

const props = withDefaults(
  defineProps<{
    run: Run
    nodes: WFNode[]
    /** Live wall-clock seconds for the current run (from RunDetailView.elapsedSec). */
    wallSec: number
    /** Shared 1s tick from the parent. */
    nowMs: number
    /** Controlled by RunDetailView top tab bar when provided. */
    statsTab?: StatsTab
  }>(),
  { statsTab: undefined },
)

const emit = defineEmits<{ (e: 'update:statsTab', tab: StatsTab): void }>()

const { t } = useI18n()

const localTab = ref<StatsTab>('single')
const tabControlled = computed(() => props.statsTab != null)
const statsTab = computed({
  get: () => props.statsTab ?? localTab.value,
  set: (v: StatsTab) => {
    if (props.statsTab != null) emit('update:statsTab', v)
    else localTab.value = v
  },
})
const singleDim = ref<SingleDimension>('process')
const multiDim = ref<MultiDimension>('node')

function labelFn(label: string | undefined, type: WFNode['type'], nodeId: string) {
  return resolveNodeDisplayLabel(label, type, t, { nodeId })
}

function typeLabel(type: WFNode['type']): string {
  const key = NODE_DEFS[type]?.label
  return key ? t(key) : type
}

function localizeItemLabel(label: string, type: WFNode['type'], isTypeDim: boolean): string {
  if (isTypeDim) return typeLabel(type)
  // NODE_DEFS keys left on merged type rows somehow
  if (label.startsWith('nodes.')) return t(label)
  return label
}

const singleSummary = computed(() =>
  aggregateSingleRun(props.run, props.nodes, singleDim.value, props.wallSec, props.nowMs, labelFn),
)

const singleItems = computed(() =>
  singleSummary.value.items.map((it) => ({
    ...it,
    label: localizeItemLabel(it.label, it.type, singleDim.value === 'type'),
  })),
)

const singleBottleneck = computed(() => {
  const bn = singleSummary.value.bottleneck
  if (!bn) return null
  const localized = {
    ...bn,
    label: localizeItemLabel(bn.label, bn.type, singleDim.value === 'type'),
  }
  return {
    item: localized,
    name: bottleneckDisplayName(localized, {
      iterationLabel: (n) => t('common.iterationN', { n }),
      mergeLabel: (n) => t('pages.executionStats.mergeCount', { n }),
    }),
  }
})

/** Panel-side merge of process usage for total-token tip (Timeline-aligned parts). */
const mergedUsage = computed((): TokenUsage => {
  const processes = flattenProcesses(props.run, props.nodes, props.nowMs, labelFn)
  const merged = mergeTokenUsage(...processes.map((p) => p.usage))
  return (
    merged ?? {
      inputTokens: 0,
      outputTokens: 0,
      cacheReadTokens: 0,
      cacheWriteTokens: 0,
    }
  )
})

const tipUsageParts = computed(() => {
  const u = mergedUsage.value
  return [
    {
      key: 'input',
      label: t('pages.executionTimeline.partInput'),
      full: u.inputTokens || 0,
      highlight: false,
    },
    {
      key: 'output',
      label: t('pages.executionTimeline.partOutput'),
      full: u.outputTokens || 0,
      highlight: false,
    },
    {
      key: 'cacheRead',
      label: t('pages.executionTimeline.partCacheRead'),
      full: u.cacheReadTokens || 0,
      highlight: true,
    },
    {
      key: 'cacheWrite',
      label: t('pages.executionTimeline.partCacheWrite'),
      full: u.cacheWriteTokens || 0,
      highlight: false,
    },
  ]
})

function durationTipText(sec: number): string {
  return t('pages.executionStats.tipDuration', {
    clock: fmtDuration(sec),
    sec: Math.floor(Math.max(0, sec)),
  })
}

function compactTokensMain(n: number | null | undefined): string {
  if (n == null) return t('pages.executionStats.dash')
  return `${fmtCompactTokenCount(n)} ${t('pages.executionStats.tokenUnit')}`
}

function compactRateMain(totalTokens: number | null, wallSec: number): string {
  if (totalTokens == null) return t('pages.executionStats.dash')
  return fmtCompactTokenRate(totalTokens, wallSec)
}

function tokenRateTipText(totalTokens: number | null, wallSec: number): string {
  if (wallSec <= 0) return t('pages.executionStats.tipTokenRateZeroWall')
  const tokens = totalTokens ?? 0
  const rate = fmtTokenRate(tokens, wallSec) ?? '0'
  return t('pages.executionStats.tipTokenRate', {
    tokens: fmtTokenCount(tokens),
    sec: Math.floor(wallSec),
    rate,
  })
}

// LiveLog-style self-managed tip (hover + keyboard focus).
const openTip = ref<KpiTipId | null>(null)
const tipUid = useId().replace(/:/g, '')
function tipDomId(id: KpiTipId): string {
  return `stats-kpi-tip-${id}-${tipUid}`
}
function showTip(id: KpiTipId) {
  openTip.value = id
}
function hideTip(id: KpiTipId) {
  if (openTip.value === id) openTip.value = null
}

// —— Multi-run candidate list + cache ——
interface Candidate {
  id: string
  label: string
  listWallSec: number
  startedAt: string
}

const candidates = ref<Candidate[]>([])
const selectedIds = ref<Set<string>>(new Set())
const detailCache = ref<Record<string, Run>>({})
const detailErrors = ref<Record<string, string>>({})
const candidatesLoading = ref(false)
const detailsLoading = ref(false)
const candidatesError = ref<string | null>(null)
/** True until the first successful candidate load for the current run. */
const selectionInitialized = ref(false)
let loadGen = 0

function defaultSelectedIds(list: Candidate[], currentId: string): Set<string> {
  const defaults = new Set<string>()
  defaults.add(currentId)
  for (const c of list) {
    if (defaults.size >= 3) break
    defaults.add(c.id)
  }
  return defaults
}

async function loadCandidates() {
  const wfId = props.run.workflowId
  if (!wfId) {
    candidates.value = []
    return
  }
  candidatesLoading.value = true
  candidatesError.value = null
  try {
    const data = await api.listRuns({ wf: wfId, page: 1, pageSize: 20 })
    const items = isPaginated(data) ? data.items : data
    let nextCandidates: Candidate[] = items.map((r) => ({
      id: r.id,
      label: '#' + r.id.replace(/^run-/, ''),
      listWallSec: r.durationSec || 0,
      startedAt: r.startedAt || r.createdAt || '',
    }))
    // If current run missing from page, still keep it selected.
    if (!nextCandidates.some((c) => c.id === props.run.id)) {
      nextCandidates = [
        {
          id: props.run.id,
          label: '#' + props.run.id.replace(/^run-/, ''),
          listWallSec: props.wallSec,
          startedAt: props.run.startedAt,
        },
        ...nextCandidates,
      ].slice(0, 20)
    }
    candidates.value = nextCandidates

    if (!selectionInitialized.value) {
      selectedIds.value = defaultSelectedIds(nextCandidates, props.run.id)
      selectionInitialized.value = true
    } else {
      // Refresh: keep ids still present in the new candidate set.
      const valid = new Set(nextCandidates.map((c) => c.id))
      const kept = new Set([...selectedIds.value].filter((id) => valid.has(id)))
      if (!kept.size) {
        selectedIds.value = defaultSelectedIds(nextCandidates, props.run.id)
      } else {
        // Ensure current run stays selected when still in candidates.
        if (valid.has(props.run.id)) kept.add(props.run.id)
        selectedIds.value = kept
      }
    }
  } catch (e: any) {
    candidatesError.value = e?.message || t('pages.executionStats.loadCandidatesFailed')
  } finally {
    candidatesLoading.value = false
  }
}

watch(
  () => [statsTab.value, props.run.workflowId, props.run.id] as const,
  ([tab], prev) => {
    const prevId = prev?.[2]
    if (prevId != null && prevId !== props.run.id) {
      selectionInitialized.value = false
      selectedIds.value = new Set()
      detailCache.value = {}
      detailErrors.value = {}
    }
    if (tab === 'multi') loadCandidates()
  },
  { immediate: true },
)

watch(
  selectedIds,
  async (ids) => {
    const gen = ++loadGen
    const missing = [...ids].filter((id) => id !== props.run.id && !detailCache.value[id])
    if (!missing.length) return
    detailsLoading.value = true
    try {
      await Promise.all(
        missing.map(async (id) => {
          try {
            const r = await api.getRun(id)
            if (gen !== loadGen) return
            detailCache.value = { ...detailCache.value, [id]: r }
            const { [id]: _, ...rest } = detailErrors.value
            detailErrors.value = rest
          } catch {
            if (gen !== loadGen) return
            detailErrors.value = {
              ...detailErrors.value,
              [id]: t('pages.executionStats.loadRunFailed', { id: id.replace(/^run-/, '') }),
            }
          }
        }),
      )
    } finally {
      if (gen === loadGen) detailsLoading.value = false
    }
  },
  { deep: true },
)

function toggleRun(id: string) {
  const next = new Set(selectedIds.value)
  if (next.has(id)) {
    if (next.size <= 1) return
    next.delete(id)
  } else {
    next.add(id)
  }
  selectedIds.value = next
}

const multiInputs = computed(() => {
  const out: { run: Run; wallSec: number; nodes?: WFNode[] }[] = []
  for (const id of selectedIds.value) {
    if (id === props.run.id) {
      out.push({ run: props.run, wallSec: props.wallSec, nodes: props.nodes })
      continue
    }
    const r = detailCache.value[id]
    if (!r) continue
    let wall = resolveRunWallSec(r, props.nowMs)
    if (wall <= 0) {
      wall = candidates.value.find((c) => c.id === id)?.listWallSec || r.durationSec || 0
    }
    out.push({ run: r, wallSec: wall, nodes: r.nodes })
  }
  return out
})

const multiSummary = computed(() =>
  aggregateMultiRuns(multiInputs.value, multiDim.value, props.nowMs, labelFn),
)

const multiItems = computed(() =>
  multiSummary.value.items.map((it) => ({
    ...it,
    label: localizeItemLabel(it.label, it.type, multiDim.value === 'type'),
  })),
)

const pendingDetailCount = computed(
  () =>
    [...selectedIds.value].filter((id) => id !== props.run.id && !detailCache.value[id] && !detailErrors.value[id])
      .length,
)

const multiReady = computed(() => pendingDetailCount.value === 0 && !detailsLoading.value)

const selectedCountDisplay = computed(() => selectedIds.value.size)

const pieFootnote = computed(() =>
  singleSummary.value.gapSec > 0 ? t('pages.executionStats.pieRelativeNote') : undefined,
)

const dimOptionsSingle = computed(() => [
  { id: 'process' as const, label: t('pages.executionStats.dimProcess') },
  { id: 'node' as const, label: t('pages.executionStats.dimNode') },
  { id: 'type' as const, label: t('pages.executionStats.dimType') },
])

const dimOptionsMulti = computed(() => [
  { id: 'node' as const, label: t('pages.executionStats.dimNode') },
  { id: 'type' as const, label: t('pages.executionStats.dimType') },
])

const singleNote = computed(() => {
  if (singleDim.value === 'process') return t('pages.executionStats.noteProcess')
  if (singleDim.value === 'node') return t('pages.executionStats.noteNode')
  return t('pages.executionStats.noteType')
})

const pieCenterSub = computed(() => {
  if (singleDim.value === 'process') return t('pages.executionStats.pieProcess')
  if (singleDim.value === 'node') return t('pages.executionStats.pieNode')
  return t('pages.executionStats.pieType')
})

const maxMultiAvg = computed(() => multiItems.value[0]?.avgSec || 1)

function fmtTokensOrDash(n: number | null | undefined): string {
  if (n == null) return t('pages.executionStats.dash')
  return fmtTokenCount(n)
}

const singleTokenHint = computed(() =>
  singleSummary.value.totalTokens == null
    ? t('pages.executionStats.hintTotalTokensNone')
    : t('pages.executionStats.hintTotalTokens'),
)

const singleRateHint = computed(() =>
  singleSummary.value.totalTokens == null
    ? t('pages.executionStats.hintTokenRateNone')
    : t('pages.executionStats.hintTokenRate'),
)

const multiSumHint = computed(() =>
  multiSummary.value.totalTokens == null
    ? t('pages.executionStats.hintSumTokensNone')
    : t('pages.executionStats.hintSumTokens'),
)

const multiAvgHint = computed(() =>
  multiSummary.value.totalTokens == null
    ? t('pages.executionStats.hintAvgTokensNone')
    : t('pages.executionStats.hintAvgTokens', { n: multiSummary.value.usageRunCount }),
)

const multiRateHint = computed(() =>
  multiSummary.value.totalTokens == null
    ? t('pages.executionStats.hintMultiTokenRateNone')
    : t('pages.executionStats.hintMultiTokenRate'),
)
</script>

<style scoped>
.stats-panel {
  --stats-token: #0b6e99;
  --stats-time: #b45309;
}
.stats-kpi-time {
  color: var(--stats-time);
}
.stats-kpi-token {
  color: var(--stats-token);
}
.stats-tok-val {
  color: var(--stats-token);
}
.stats-tok-na {
  color: inherit;
}
.stats-kpi-value-wrap {
  position: relative;
  display: inline-flex;
  max-width: 100%;
  align-self: flex-start;
}
.stats-kpi-value {
  cursor: help;
  border-radius: 2px;
  border-bottom: 1px dotted color-mix(in srgb, currentColor 35%, transparent);
  outline: none;
}
.stats-kpi-value:hover,
.stats-kpi-value:focus-visible {
  background: color-mix(in srgb, currentColor 8%, transparent);
}
.stats-kpi-tip {
  position: absolute;
  top: calc(100% + 6px);
  z-index: 30;
  max-width: min(280px, 70vw);
  border: 1px solid rgb(var(--c-line-strong, 203 213 225));
  background: rgb(var(--c-overlay, 15 23 42));
  color: rgb(var(--c-txt2, 226 232 240));
  padding: 6px 10px;
  font-size: 11px;
  font-weight: 500;
  line-height: 1.4;
  white-space: nowrap;
  box-shadow: 0 8px 20px rgba(0, 0, 0, 0.22);
  pointer-events: none;
}
.stats-kpi-tip-left {
  left: 0;
}
.stats-kpi-tip-center {
  left: 50%;
  transform: translateX(-50%);
}
.stats-kpi-tip-right {
  right: 0;
  left: auto;
}
.stats-kpi-tip-rich {
  white-space: normal;
  width: 248px;
  max-width: min(248px, 70vw);
  padding: 10px 11px 9px;
  text-align: left;
}
.stats-kpi-tip-total {
  margin-bottom: 8px;
  padding-bottom: 7px;
  border-bottom: 1px solid rgba(248, 250, 252, 0.14);
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font-size: 12px;
  font-weight: 700;
  color: #f8fafc;
}
.stats-kpi-tip-parts {
  display: grid;
  gap: 5px;
}
.stats-kpi-tip-part {
  display: grid;
  grid-template-columns: 52px 1fr auto;
  gap: 8px;
  align-items: center;
  font-size: 11.5px;
}
.stats-kpi-tip-part-k {
  color: #94a3b8;
}
.stats-kpi-tip-part-k-hl {
  color: #c4b5fd;
  font-weight: 650;
}
.stats-kpi-tip-part-n {
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font-weight: 650;
  color: #f8fafc;
  text-align: right;
}
.stats-kpi-tip-part-c {
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font-size: 10.5px;
  color: #64748b;
  min-width: 42px;
}
.stats-kpi-tip-part-hl {
  margin: 0 -6px;
  padding: 4px 6px;
  border-radius: 5px;
  background: rgba(124, 58, 237, 0.18);
}
</style>

<template>
  <div
    data-testid="execution-stats-panel"
    class="stats-panel flex h-full min-h-0 min-w-0 w-full max-w-full flex-col bg-base"
  >
    <!-- Tab bar only when parent does not control statsTab (standalone / tests). -->
    <div
      v-if="!tabControlled"
      class="flex shrink-0 border-b border-line bg-surface px-3 sm:px-4"
    >
      <button
        type="button"
        class="border-b-2 px-3 py-2.5 text-[13px] font-medium transition-colors"
        :class="
          statsTab === 'single'
            ? 'border-accent-2 text-accent-2'
            : 'border-transparent text-txt3 hover:text-txt2'
        "
        @click="statsTab = 'single'"
      >
        {{ t('pages.executionStats.tabSingle') }}
      </button>
      <button
        type="button"
        class="border-b-2 px-3 py-2.5 text-[13px] font-medium transition-colors"
        :class="
          statsTab === 'multi'
            ? 'border-accent-2 text-accent-2'
            : 'border-transparent text-txt3 hover:text-txt2'
        "
        @click="statsTab = 'multi'"
      >
        {{ t('pages.executionStats.tabMulti') }}
      </button>
    </div>

    <div
      data-testid="execution-stats-scroll"
      class="scroll-area safe-area-bottom min-h-0 min-w-0 w-full max-w-full flex-1 overflow-x-clip overflow-y-auto px-4 py-4 sm:px-5"
    >
      <!-- Single run -->
      <template v-if="statsTab === 'single'">
        <div class="mb-3.5 grid grid-cols-1 gap-2 md:grid-cols-3 xl:grid-cols-5">
          <div class="border border-line bg-surface px-3 py-2.5" data-testid="stats-kpi-wall">
            <div class="mb-1 text-[11px] text-txt3">{{ t('pages.executionStats.wallClock') }}</div>
            <div class="stats-kpi-value-wrap">
              <div
                class="stats-kpi-value stats-kpi-time text-[16px] font-semibold tabular-nums tracking-tight"
                tabindex="0"
                data-testid="stats-kpi-wall-value"
                :aria-describedby="openTip === 'wall' ? tipDomId('wall') : undefined"
                @mouseenter="showTip('wall')"
                @mouseleave="hideTip('wall')"
                @focus="showTip('wall')"
                @blur="hideTip('wall')"
              >
                {{ fmtCompactDuration(singleSummary.wallSec) }}
              </div>
              <span
                v-if="openTip === 'wall'"
                :id="tipDomId('wall')"
                role="tooltip"
                class="stats-kpi-tip stats-kpi-tip-left"
                data-testid="stats-kpi-wall-tip"
              >
                {{ durationTipText(singleSummary.wallSec) }}
              </span>
            </div>
            <div class="mt-0.5 text-[10px] text-txt3">{{ t('pages.executionStats.wallHint') }}</div>
          </div>
          <div class="border border-line bg-surface px-3 py-2.5" data-testid="stats-kpi-node-sum">
            <div class="mb-1 text-[11px] text-txt3">{{ t('pages.executionStats.nodeSum') }}</div>
            <div class="stats-kpi-value-wrap">
              <div
                class="stats-kpi-value stats-kpi-time text-[16px] font-semibold tabular-nums tracking-tight"
                tabindex="0"
                data-testid="stats-kpi-node-sum-value"
                :aria-describedby="openTip === 'nodeSum' ? tipDomId('nodeSum') : undefined"
                @mouseenter="showTip('nodeSum')"
                @mouseleave="hideTip('nodeSum')"
                @focus="showTip('nodeSum')"
                @blur="hideTip('nodeSum')"
              >
                {{ fmtCompactDuration(singleSummary.nodeSumSec) }}
              </div>
              <span
                v-if="openTip === 'nodeSum'"
                :id="tipDomId('nodeSum')"
                role="tooltip"
                class="stats-kpi-tip stats-kpi-tip-left"
                data-testid="stats-kpi-node-sum-tip"
              >
                {{ durationTipText(singleSummary.nodeSumSec) }}
              </span>
            </div>
            <div class="mt-0.5 text-[10px] text-txt3">{{ t('pages.executionStats.nodeSumHint') }}</div>
          </div>
          <div class="border border-line bg-surface px-3 py-2.5" data-testid="stats-kpi-gap">
            <div class="mb-1 text-[11px] text-txt3">{{ t('pages.executionStats.gap') }}</div>
            <div class="stats-kpi-value-wrap">
              <div
                class="stats-kpi-value stats-kpi-time text-[16px] font-semibold tabular-nums tracking-tight"
                tabindex="0"
                data-testid="stats-kpi-gap-value"
                :aria-describedby="openTip === 'gap' ? tipDomId('gap') : undefined"
                @mouseenter="showTip('gap')"
                @mouseleave="hideTip('gap')"
                @focus="showTip('gap')"
                @blur="hideTip('gap')"
              >
                {{ fmtCompactDuration(singleSummary.gapSec) }}
              </div>
              <span
                v-if="openTip === 'gap'"
                :id="tipDomId('gap')"
                role="tooltip"
                class="stats-kpi-tip stats-kpi-tip-center"
                data-testid="stats-kpi-gap-tip"
              >
                {{ durationTipText(singleSummary.gapSec) }}
              </span>
            </div>
            <div class="mt-0.5 text-[10px] text-txt3">{{ t('pages.executionStats.gapHint') }}</div>
          </div>
          <div class="border border-line bg-surface px-3 py-2.5" data-testid="stats-kpi-total-tokens">
            <div class="mb-1 text-[11px] text-txt3">{{ t('pages.executionStats.totalTokens') }}</div>
            <div class="stats-kpi-value-wrap">
              <div
                class="stats-kpi-value text-[16px] font-semibold tabular-nums tracking-tight"
                :class="singleSummary.totalTokens == null ? 'text-txt3' : 'stats-kpi-token'"
                tabindex="0"
                data-testid="stats-kpi-total-tokens-value"
                :aria-describedby="openTip === 'tokens' ? tipDomId('tokens') : undefined"
                @mouseenter="showTip('tokens')"
                @mouseleave="hideTip('tokens')"
                @focus="showTip('tokens')"
                @blur="hideTip('tokens')"
              >
                {{ compactTokensMain(singleSummary.totalTokens) }}
              </div>
              <span
                v-if="openTip === 'tokens'"
                :id="tipDomId('tokens')"
                role="tooltip"
                class="stats-kpi-tip stats-kpi-tip-right stats-kpi-tip-rich"
                data-testid="stats-kpi-total-tokens-tip"
              >
                <div class="stats-kpi-tip-total">
                  {{
                    t('pages.executionStats.tipTokenTotal', {
                      n: fmtTokenCount(singleSummary.totalTokens ?? 0),
                    })
                  }}
                </div>
                <div class="stats-kpi-tip-parts">
                  <div
                    v-for="part in tipUsageParts"
                    :key="part.key"
                    class="stats-kpi-tip-part"
                    :class="{ 'stats-kpi-tip-part-hl': part.highlight }"
                    :data-testid="`stats-kpi-token-part-${part.key}`"
                  >
                    <span
                      class="stats-kpi-tip-part-k"
                      :class="{ 'stats-kpi-tip-part-k-hl': part.highlight }"
                    >
                      {{ part.label }}
                    </span>
                    <span class="stats-kpi-tip-part-n">{{ fmtTokenCount(part.full) }}</span>
                    <span class="stats-kpi-tip-part-c">{{ fmtCompactTokenCount(part.full) }}</span>
                  </div>
                </div>
              </span>
            </div>
            <div class="mt-0.5 text-[10px] text-txt3">{{ singleTokenHint }}</div>
          </div>
          <div class="border border-line bg-surface px-3 py-2.5" data-testid="stats-kpi-token-rate">
            <div class="mb-1 text-[11px] text-txt3">{{ t('pages.executionStats.tokenRate') }}</div>
            <div class="stats-kpi-value-wrap">
              <div
                class="stats-kpi-value text-[16px] font-semibold tabular-nums tracking-tight"
                :class="singleSummary.totalTokens == null ? 'text-txt3' : 'stats-kpi-token'"
                tabindex="0"
                data-testid="stats-kpi-token-rate-value"
                :aria-describedby="openTip === 'rate' ? tipDomId('rate') : undefined"
                @mouseenter="showTip('rate')"
                @mouseleave="hideTip('rate')"
                @focus="showTip('rate')"
                @blur="hideTip('rate')"
              >
                {{ compactRateMain(singleSummary.totalTokens, singleSummary.wallSec) }}
              </div>
              <span
                v-if="openTip === 'rate'"
                :id="tipDomId('rate')"
                role="tooltip"
                class="stats-kpi-tip stats-kpi-tip-right"
                data-testid="stats-kpi-token-rate-tip"
              >
                {{ tokenRateTipText(singleSummary.totalTokens, singleSummary.wallSec) }}
              </span>
            </div>
            <div class="mt-0.5 text-[10px] text-txt3">{{ singleRateHint }}</div>
          </div>
        </div>

        <div
          v-if="singleBottleneck"
          class="mb-3.5 border border-err/35 bg-err/6 px-3 py-2.5"
        >
          <div class="mb-1.5 flex items-center justify-between gap-2">
            <span class="text-[11px] font-semibold uppercase tracking-wide text-err">
              {{ t('pages.executionStats.bottleneck') }}
            </span>
            <span
              v-if="singleBottleneck.item.hasHumanWait"
              class="shrink-0 border border-warn/40 bg-warn/10 px-1.5 py-px text-[10px] text-warn"
            >
              {{ t('pages.executionStats.hasHumanWait') }}
            </span>
          </div>
          <div class="text-[13px] font-semibold text-txt">{{ singleBottleneck.name }}</div>
          <div class="mt-1 flex flex-wrap gap-x-3.5 gap-y-1 text-[12px] text-txt2">
            <span>
              {{ t('pages.executionStats.duration') }}
              <code class="font-mono text-accent-2">{{ fmtDuration(singleBottleneck.item.durationSec) }}</code>
            </span>
            <span>
              {{ t('pages.executionStats.share') }}
              <code class="font-mono text-accent-2">
                {{ singleBottleneck.item.sharePct == null ? '—' : singleBottleneck.item.sharePct + '%' }}
              </code>
            </span>
          </div>
        </div>

        <div class="mb-3.5 border border-line bg-surface p-3">
          <StatsPieChart
            :items="singleItems"
            :center-value="fmtDuration(singleSummary.nodeSumSec)"
            :center-sub="pieCenterSub"
            :empty-label="t('pages.executionStats.empty')"
            :footnote="pieFootnote"
          />
        </div>

        <div class="mb-3 flex flex-wrap items-center gap-2">
          <span class="text-[11px] text-txt3">{{ t('pages.executionStats.dimension') }}</span>
          <div class="inline-flex border border-line bg-surface text-[12px]">
            <button
              v-for="opt in dimOptionsSingle"
              :key="opt.id"
              type="button"
              class="px-2.5 py-1 font-medium transition-colors"
              :class="
                singleDim === opt.id
                  ? 'bg-accent-dim text-accent-2'
                  : 'text-txt3 hover:bg-elevated hover:text-txt2'
              "
              @click="singleDim = opt.id"
            >
              {{ opt.label }}
            </button>
          </div>
        </div>
        <p class="mb-3 text-[11px] leading-snug text-txt3">{{ singleNote }}</p>

        <div v-if="!singleItems.length" class="py-8 text-center text-[12px] text-txt3">
          {{ t('pages.executionStats.empty') }}
        </div>
        <div v-else class="flex flex-col gap-2">
          <div
            v-for="it in singleItems"
            :key="it.key"
            class="border bg-surface px-3 py-2.5"
            :class="
              singleBottleneck && it.key === singleBottleneck.item.key
                ? 'border-err/40'
                : 'border-line'
            "
          >
            <div class="mb-1.5 flex items-center gap-2">
              <TruncatedTextTooltip
                :text="it.label"
                class="min-w-0 flex-1 truncate text-[13px] font-semibold text-txt"
                data-testid="stats-ranking-label"
              >
                <span
                  v-if="it.live"
                  class="mr-1 inline-block h-1.5 w-1.5 animate-pulse bg-info align-middle"
                />
                {{ it.label }}
              </TruncatedTextTooltip>
              <span
                v-if="it.isProcess && it.iteration && it.iteration > 1"
                class="shrink-0 border border-warn/40 bg-warn/10 px-1.5 py-px text-[10px] text-warn"
              >
                {{ t('common.iterationN', { n: it.iteration }) }}
              </span>
              <span
                v-if="!it.isProcess && it.count > 1"
                class="shrink-0 border border-info/40 bg-info/10 px-1.5 py-px text-[10px] text-info"
              >
                {{ t('pages.executionStats.mergeCount', { n: it.count }) }}
              </span>
              <span
                v-if="it.hasHumanWait"
                class="shrink-0 border border-warn/40 bg-warn/10 px-1.5 py-px text-[10px] text-warn"
              >
                {{ t('pages.executionStats.hasHumanWait') }}
              </span>
              <div class="flex shrink-0 items-baseline gap-2 tabular-nums">
                <span class="stats-kpi-time text-[12px] font-medium">{{ fmtDuration(it.durationSec) }}</span>
                <span class="min-w-[36px] text-right text-[12px] font-semibold text-txt">
                  {{ it.sharePct == null ? t('pages.executionStats.dash') : it.sharePct + '%' }}
                </span>
                <span
                  class="min-w-[72px] text-right text-[12px] font-semibold tabular-nums"
                  :class="it.totalTokens == null ? 'text-txt3 stats-tok-na' : 'stats-tok-val'"
                  data-testid="stats-rank-tokens"
                >
                  {{ t('pages.executionStats.tokensCol') }}
                  {{ fmtTokensOrDash(it.totalTokens) }}
                </span>
              </div>
            </div>
            <div class="h-1 overflow-hidden bg-elevated">
              <div
                class="h-full transition-all duration-300"
                :style="{
                  width: (it.sharePct == null ? 0 : Math.min(100, it.sharePct)) + '%',
                  background:
                    singleBottleneck && it.key === singleBottleneck.item.key
                      ? 'rgb(var(--c-err))'
                      : it.color,
                }"
              />
            </div>
          </div>
        </div>
      </template>

      <!-- Multi run -->
      <template v-else>
        <div class="mb-3">
          <div class="mb-2 flex items-center justify-between gap-2">
            <h3 class="text-[12px] font-medium text-txt3">{{ t('pages.executionStats.pickRuns') }}</h3>
            <button
              type="button"
              class="inline-flex items-center gap-1 text-[11px] text-txt3 hover:text-txt2"
              :disabled="candidatesLoading"
              @click="loadCandidates"
            >
              <Icon name="refresh" :size="12" :class="{ 'animate-spin': candidatesLoading }" />
              {{ t('common.buttons.refresh') }}
            </button>
          </div>
          <p v-if="candidatesError" class="mb-2 text-[12px] text-err">{{ candidatesError }}</p>
          <div v-else-if="candidatesLoading && !candidates.length" class="py-4 text-[12px] text-txt3">
            {{ t('pages.executionStats.loadingCandidates') }}
          </div>
          <div v-else class="flex flex-wrap gap-1.5">
            <button
              v-for="c in candidates"
              :key="c.id"
              type="button"
              class="inline-flex items-center gap-1.5 border px-2 py-1 text-[12px] transition-colors"
              :class="
                selectedIds.has(c.id)
                  ? 'border-accent/50 bg-accent-dim text-accent-2'
                  : 'border-line bg-surface text-txt2 hover:bg-elevated'
              "
              @click="toggleRun(c.id)"
            >
              <span
                class="h-1.5 w-1.5 shrink-0"
                :class="selectedIds.has(c.id) ? 'bg-accent-2' : 'bg-line-strong'"
              />
              {{ c.label }}
              <span class="text-txt3">{{ fmtDuration(c.id === run.id ? wallSec : c.listWallSec) }}</span>
            </button>
          </div>
          <p
            v-for="(msg, id) in detailErrors"
            :key="id"
            class="mt-1.5 text-[11px] text-warn"
          >
            {{ msg }}
          </p>
          <p v-if="detailsLoading || pendingDetailCount" class="mt-2 text-[11px] text-txt3">
            {{ t('pages.executionStats.loadingDetails') }}
          </p>
        </div>

        <div class="mb-3.5 grid grid-cols-1 gap-2 md:grid-cols-3 xl:grid-cols-6">
          <div class="border border-line bg-surface px-3 py-2.5" data-testid="stats-kpi-selected">
            <div class="mb-1 text-[11px] text-txt3">{{ t('pages.executionStats.selectedRuns') }}</div>
            <div class="text-[16px] font-semibold tabular-nums text-txt">{{ selectedCountDisplay }}</div>
          </div>
          <div class="border border-line bg-surface px-3 py-2.5" data-testid="stats-kpi-avg-wall">
            <div class="mb-1 text-[11px] text-txt3">{{ t('pages.executionStats.avgWall') }}</div>
            <div class="stats-kpi-time text-[16px] font-semibold tabular-nums">
              {{
                multiReady && multiSummary.selectedCount
                  ? fmtDuration(Math.round(multiSummary.wallSumSec / multiSummary.selectedCount))
                  : t('pages.executionStats.dash')
              }}
            </div>
          </div>
          <div class="border border-line bg-surface px-3 py-2.5" data-testid="stats-kpi-process-count">
            <div class="mb-1 text-[11px] text-txt3">{{ t('pages.executionStats.processCount') }}</div>
            <div class="text-[16px] font-semibold tabular-nums text-txt">
              {{ multiReady ? multiSummary.processCount : t('pages.executionStats.dash') }}
            </div>
          </div>
          <div class="border border-line bg-surface px-3 py-2.5" data-testid="stats-kpi-sum-tokens">
            <div class="mb-1 text-[11px] text-txt3">{{ t('pages.executionStats.sumTokens') }}</div>
            <div
              class="text-[16px] font-semibold tabular-nums tracking-tight"
              :class="!multiReady || multiSummary.totalTokens == null ? 'text-txt3' : 'stats-kpi-token'"
            >
              {{ multiReady ? fmtTokensOrDash(multiSummary.totalTokens) : t('pages.executionStats.dash') }}
            </div>
            <div class="mt-0.5 text-[10px] text-txt3">{{ multiSumHint }}</div>
          </div>
          <div class="border border-line bg-surface px-3 py-2.5" data-testid="stats-kpi-avg-tokens">
            <div class="mb-1 text-[11px] text-txt3">{{ t('pages.executionStats.avgTokens') }}</div>
            <div
              class="text-[16px] font-semibold tabular-nums tracking-tight"
              :class="!multiReady || multiSummary.avgTokens == null ? 'text-txt3' : 'stats-kpi-token'"
            >
              {{ multiReady ? fmtTokensOrDash(multiSummary.avgTokens) : t('pages.executionStats.dash') }}
            </div>
            <div class="mt-0.5 text-[10px] text-txt3">{{ multiAvgHint }}</div>
          </div>
          <div class="border border-line bg-surface px-3 py-2.5" data-testid="stats-kpi-multi-token-rate">
            <div class="mb-1 text-[11px] text-txt3">{{ t('pages.executionStats.tokenRate') }}</div>
            <div
              class="text-[16px] font-semibold tabular-nums tracking-tight"
              :class="!multiReady || multiSummary.tokenRate == null ? 'text-txt3' : 'stats-kpi-token'"
            >
              {{
                multiReady
                  ? (multiSummary.tokenRate ?? t('pages.executionStats.dash'))
                  : t('pages.executionStats.dash')
              }}
            </div>
            <div class="mt-0.5 text-[10px] text-txt3">{{ multiRateHint }}</div>
          </div>
        </div>

        <p v-if="!multiReady" class="mb-3.5 py-6 text-center text-[12px] text-txt3">
          {{ t('pages.executionStats.multiPending') }}
        </p>
        <template v-else>
        <div v-if="multiItems[0]" class="mb-3.5 border border-err/35 bg-err/6 px-3 py-2.5">
          <div class="mb-1 text-[11px] font-semibold uppercase tracking-wide text-err">
            {{ t('pages.executionStats.topAvg') }}
          </div>
          <div class="text-[13px] font-semibold text-txt">{{ multiItems[0].label }}</div>
          <div class="mt-1 text-[12px] text-txt2">
            {{ t('pages.executionStats.avg') }}
            {{ fmtDuration(multiItems[0].avgSec) }}
            ·
            {{ t('pages.executionStats.share') }}
            {{ multiItems[0].sharePct == null ? '—' : multiItems[0].sharePct + '%' }}
          </div>
        </div>

        <div class="mb-3 flex flex-wrap items-center gap-2">
          <span class="text-[11px] text-txt3">{{ t('pages.executionStats.dimension') }}</span>
          <div class="inline-flex border border-line bg-surface text-[12px]">
            <button
              v-for="opt in dimOptionsMulti"
              :key="opt.id"
              type="button"
              class="px-2.5 py-1 font-medium transition-colors"
              :class="
                multiDim === opt.id
                  ? 'bg-accent-dim text-accent-2'
                  : 'text-txt3 hover:bg-elevated hover:text-txt2'
              "
              @click="multiDim = opt.id"
            >
              {{ opt.label }}
            </button>
          </div>
        </div>

        <div class="mb-3.5 border border-line bg-surface p-3">
          <StatsPieChart
            :items="multiItems"
            :center-value="fmtDuration(multiSummary.wallSumSec ? multiItems.reduce((a, i) => a + i.durationSec, 0) : 0)"
            :center-sub="t('pages.executionStats.pieMulti')"
            :empty-label="t('pages.executionStats.empty')"
          />
        </div>

        <h3 class="mb-2 text-[12px] font-medium text-txt3">
          {{
            multiDim === 'node'
              ? t('pages.executionStats.rankByNode')
              : t('pages.executionStats.rankByType')
          }}
        </h3>
        <p class="mb-3 text-[11px] leading-snug text-txt3">{{ t('pages.executionStats.noteMulti') }}</p>

        <div v-if="!multiItems.length" class="py-8 text-center text-[12px] text-txt3">
          {{ t('pages.executionStats.empty') }}
        </div>
        <div v-else class="flex flex-col gap-2">
          <div
            v-for="(it, i) in multiItems"
            :key="it.key"
            class="border bg-surface px-3 py-2.5"
            :class="i === 0 ? 'border-err/40' : 'border-line'"
          >
            <div class="mb-1.5 flex items-center gap-2">
              <span class="w-5 shrink-0 text-[12px] font-semibold tabular-nums text-txt3">{{ i + 1 }}</span>
              <TruncatedTextTooltip
                :text="it.label"
                class="min-w-0 flex-1 truncate text-[13px] font-semibold text-txt"
                data-testid="stats-ranking-label"
              >
                {{ it.label }}
              </TruncatedTextTooltip>
              <span
                v-if="it.hasHumanWait"
                class="shrink-0 border border-warn/40 bg-warn/10 px-1.5 py-px text-[10px] text-warn"
              >
                {{ t('pages.executionStats.hasHumanWait') }}
              </span>
            </div>
            <div class="mb-1.5 flex flex-wrap items-baseline gap-x-2 gap-y-0.5 pl-7 text-[11px] text-txt2">
              <span>
                {{ t('pages.executionStats.avg') }}
                <span class="stats-kpi-time font-medium">{{ fmtDuration(it.avgSec) }}</span>
              </span>
              <span>· Σ {{ fmtDuration(it.durationSec) }}</span>
              <span>· {{ it.sharePct == null ? t('pages.executionStats.dash') : it.sharePct + '%' }}</span>
              <span>· {{ t('pages.executionStats.times', { n: it.count }) }}</span>
              <span
                class="font-semibold tabular-nums"
                :class="it.totalTokens == null ? 'text-txt3' : 'stats-tok-val'"
                data-testid="stats-rank-tokens"
              >
                · {{ t('pages.executionStats.tokensCol') }}
                {{ fmtTokensOrDash(it.totalTokens) }}
              </span>
            </div>
            <div class="h-1 overflow-hidden bg-elevated pl-0">
              <div
                class="ml-7 h-full transition-all duration-300"
                :style="{
                  width: (maxMultiAvg > 0 ? Math.round((it.avgSec / maxMultiAvg) * 100) : 0) + '%',
                  background: it.color,
                }"
              />
            </div>
          </div>
        </div>
        </template>
      </template>
    </div>
  </div>
</template>
