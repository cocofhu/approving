<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { api } from '@/lib/api'
import type { ProjectTokenStats, TokenStatsWindow } from '@/lib/types'
import { fmtCompactTokenCount } from '@/lib/tokenUsage'
import TokenTrendChart from './TokenTrendChart.vue'
import TokenDonutChart from './TokenDonutChart.vue'
import TokenWorkflowRank from './TokenWorkflowRank.vue'
import TokenModelComposition from './TokenModelComposition.vue'
import TokenModelRank from './TokenModelRank.vue'
import { clientTimezoneParams } from './tokenStatsShared'

const props = defineProps<{
  projectId: string
}>()

const { t } = useI18n()

const WINDOWS: TokenStatsWindow[] = ['7d', '30d', '90d', 'all']

const windowSel = ref<TokenStatsWindow>('30d')
const loading = ref(true)
const failed = ref(false)
const data = ref<ProjectTokenStats | null>(null)

let abort: AbortController | null = null
let generation = 0

const windowLabel = computed(() => t(`pages.board.tokenStats.windows.${windowSel.value}`))

const grainLabel = computed(() => {
  if (!data.value) return ''
  return data.value.bucketWidth === 'week'
    ? t('pages.board.tokenStats.grainWeek')
    : t('pages.board.tokenStats.grainDay')
})

const isEmpty = computed(() => !!data.value?.empty)

const modelCompTotal = computed(() =>
  (data.value?.modelComposition || []).reduce((s, m) => s + (m.total || 0), 0),
)

async function load() {
  const gen = ++generation
  abort?.abort()
  abort = new AbortController()
  loading.value = true
  failed.value = false
  // Clear previous window data so UI never shows stale charts while loading.
  data.value = null

  const tz = clientTimezoneParams()
  try {
    const res = await api.getProjectTokenStats(
      props.projectId,
      {
        window: windowSel.value,
        timezone: tz.timezone,
        utcOffsetMinutes: tz.utcOffsetMinutes,
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

function selectWindow(w: TokenStatsWindow) {
  if (windowSel.value === w && !failed.value && data.value) return
  windowSel.value = w
  void load()
}

function retry() {
  void load()
}

watch(
  () => props.projectId,
  () => {
    windowSel.value = '30d'
    void load()
  },
)

onMounted(() => {
  void load()
})

onUnmounted(() => {
  generation += 1
  abort?.abort()
})
</script>

<template>
  <section
    data-testid="token-stats-panel"
    class="token-stats-panel mb-4 min-w-0 overflow-x-clip"
    aria-labelledby="token-stats-heading"
  >
    <div
      data-testid="token-stats-head"
      class="mb-3 flex flex-col items-stretch gap-3 md:flex-row md:flex-wrap md:items-center md:justify-between"
    >
      <h3 id="token-stats-heading" class="m-0 flex items-center gap-2 text-sm font-semibold text-txt">
        {{ t('pages.board.tokenStats.title') }}
        <span
          data-testid="token-stats-window-badge"
          class="rounded-full bg-accent-dim px-2 py-0.5 text-[11px] font-semibold text-accent-2"
        >
          {{ windowLabel }}
        </span>
      </h3>
      <div
        class="flex flex-wrap gap-2 rounded-[10px] bg-elevated p-0.5"
        role="tablist"
        :aria-label="t('pages.board.tokenStats.windowAria')"
        data-testid="token-stats-windows"
      >
        <button
          v-for="w in WINDOWS"
          :key="w"
          type="button"
          role="tab"
          class="min-h-11 rounded-lg border-0 px-3 py-2.5 text-sm transition md:min-h-0 md:px-2.5 md:py-1.5 md:text-xs"
          :class="
            windowSel === w
              ? 'bg-surface font-semibold text-txt shadow-sm'
              : 'bg-transparent text-txt3 hover:text-txt2'
          "
          :aria-selected="windowSel === w"
          :data-testid="`token-stats-window-${w}`"
          @click="selectWindow(w)"
        >
          {{ t(`pages.board.tokenStats.windows.${w}`) }}
        </button>
      </div>
    </div>

    <!-- Loading: whole panel placeholder, no stale charts -->
    <div
      v-if="loading"
      data-testid="token-stats-loading"
      class="flex min-h-[220px] items-center justify-center rounded-xl border border-line bg-surface text-sm text-txt3"
    >
      {{ t('pages.board.tokenStats.loading') }}
    </div>

    <!-- Failure: unified panel error + retry -->
    <div
      v-else-if="failed"
      data-testid="token-stats-error"
      class="flex min-h-[180px] flex-col items-center justify-center gap-3 rounded-xl border border-err/40 bg-err/10 px-4 py-6 text-center"
    >
      <p class="m-0 text-sm text-err">{{ t('pages.board.tokenStats.loadFailed') }}</p>
      <button
        type="button"
        class="border border-err/40 px-3 py-1.5 text-xs text-err hover:bg-err/10"
        data-testid="token-stats-retry"
        @click="retry"
      >
        {{ t('pages.board.tokenStats.retry') }}
      </button>
    </div>

    <!-- Empty: no reported usage in window -->
    <div
      v-else-if="isEmpty"
      data-testid="token-stats-empty"
      class="grid gap-3"
    >
      <div class="relative min-h-[200px] min-w-0 overflow-x-clip rounded-xl border border-line bg-surface p-3.5">
        <div class="mb-2 flex items-baseline justify-between gap-2">
          <h4 class="m-0 text-[13px] font-semibold text-txt">{{ t('pages.board.tokenStats.trendTitle') }}</h4>
          <span class="text-[11px] text-txt3">{{ grainLabel }}</span>
        </div>
        <div class="flex min-h-[160px] flex-col items-center justify-center gap-1.5 text-center">
          <strong class="text-[13px] text-txt">{{ t('pages.board.tokenStats.emptyTrendTitle') }}</strong>
          <span class="max-w-[28ch] text-xs text-txt3">{{ t('pages.board.tokenStats.emptyTrendHint') }}</span>
        </div>
      </div>
      <div class="grid min-w-0 grid-cols-1 gap-3 md:grid-cols-2">
        <div class="relative min-h-[200px] min-w-0 rounded-xl border border-line bg-surface p-3.5">
          <div class="mb-2 flex items-baseline justify-between gap-2">
            <h4 class="m-0 text-[13px] font-semibold text-txt">{{ t('pages.board.tokenStats.compositionTitle') }}</h4>
          </div>
          <div class="flex min-h-[160px] flex-col items-center justify-center gap-1.5 text-center">
            <strong class="text-[13px] text-txt">{{ t('pages.board.tokenStats.emptyCompTitle') }}</strong>
            <span class="max-w-[28ch] text-xs text-txt3">{{ t('pages.board.tokenStats.emptyCompHint') }}</span>
          </div>
        </div>
        <div class="relative min-h-[200px] min-w-0 rounded-xl border border-line bg-surface p-3.5">
          <div class="mb-2 flex flex-col gap-1">
            <div class="flex items-baseline justify-between gap-2">
              <h4 class="m-0 text-[13px] font-semibold text-txt">{{ t('pages.board.tokenStats.rankTitle') }}</h4>
              <span class="text-[11px] text-txt3">{{ t('pages.board.tokenStats.rankSub') }}</span>
            </div>
          </div>
          <div class="flex min-h-[160px] flex-col items-center justify-center gap-1.5 text-center">
            <strong class="text-[13px] text-txt">{{ t('pages.board.tokenStats.emptyRankTitle') }}</strong>
            <span class="max-w-[28ch] text-xs text-txt3">{{ t('pages.board.tokenStats.emptyRankHint') }}</span>
          </div>
        </div>
      </div>
    </div>

    <!-- Ready charts -->
    <div v-else-if="data" data-testid="token-stats-charts" class="grid gap-3">
      <div
        class="min-w-0 overflow-x-clip rounded-xl border border-line bg-surface p-3.5"
        data-testid="token-stats-trend-card"
      >
        <div class="mb-2 flex items-baseline justify-between gap-2">
          <h4 class="m-0 text-[13px] font-semibold text-txt">{{ t('pages.board.tokenStats.trendTitle') }}</h4>
          <span class="text-[11px] text-txt3">{{ grainLabel }}</span>
        </div>
        <TokenTrendChart :trend="data.trend" :bucket-width="data.bucketWidth" />
      </div>
      <div class="grid min-w-0 grid-cols-1 gap-3 md:grid-cols-2">
        <div class="min-w-0 rounded-xl border border-line bg-surface p-3.5" data-testid="token-stats-model-comp-card">
          <div class="mb-2 flex items-baseline justify-between gap-2">
            <h4 class="m-0 text-[13px] font-semibold text-txt">{{ t('pages.board.tokenStats.modelCompositionTitle') }}</h4>
            <span class="text-[11px] text-txt3">
              {{ t('pages.board.tokenStats.compTotal', { n: fmtCompactTokenCount(modelCompTotal) }) }}
            </span>
          </div>
          <TokenModelComposition :models="data.modelComposition || []" />
        </div>
        <div class="min-w-0 rounded-xl border border-line bg-surface p-3.5" data-testid="token-stats-model-rank-card">
          <div class="mb-2 flex items-baseline justify-between gap-2" data-testid="token-stats-model-rank-head">
            <h4 class="m-0 text-[13px] font-semibold text-txt">{{ t('pages.board.tokenStats.modelRankTitle') }}</h4>
            <span class="text-[11px] text-txt3">{{ t('pages.board.tokenStats.modelRankSub') }}</span>
          </div>
          <TokenModelRank :models="data.modelRanking || []" />
        </div>
      </div>
      <div class="grid min-w-0 grid-cols-1 gap-3 md:grid-cols-2">
        <div class="min-w-0 rounded-xl border border-line bg-surface p-3.5" data-testid="token-stats-comp-card">
          <div class="mb-2 flex items-baseline justify-between gap-2">
            <h4 class="m-0 text-[13px] font-semibold text-txt">{{ t('pages.board.tokenStats.compositionTitle') }}</h4>
            <span class="text-[11px] text-txt3">
              {{ t('pages.board.tokenStats.compTotal', { n: fmtCompactTokenCount(data.composition.total) }) }}
            </span>
          </div>
          <TokenDonutChart :composition="data.composition" />
        </div>
        <div class="min-w-0 rounded-xl border border-line bg-surface p-3.5" data-testid="token-stats-rank-card">
          <div class="mb-2 flex flex-col gap-1">
            <div class="flex items-baseline justify-between gap-2">
              <h4 class="m-0 text-[13px] font-semibold text-txt">{{ t('pages.board.tokenStats.rankTitle') }}</h4>
              <span class="text-[11px] text-txt3">{{ t('pages.board.tokenStats.rankSub') }}</span>
            </div>
            <p class="m-0 text-[11px] leading-snug text-txt3">{{ t('pages.board.tokenStats.rankHint') }}</p>
          </div>
          <TokenWorkflowRank :workflows="data.workflows" />
        </div>
      </div>
    </div>
  </section>
</template>
