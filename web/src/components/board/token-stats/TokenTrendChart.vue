<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  Chart as ChartJS,
  CategoryScale,
  LinearScale,
  PointElement,
  LineElement,
  Filler,
  Tooltip,
  type ChartOptions,
  type TooltipModel,
  type Chart,
} from 'chart.js'
import { Line } from 'vue-chartjs'
import type { TokenStatsBucket } from '@/lib/shared/types'
import { fmtTokenCount } from '@/lib/run/tokenUsage'
import { TOKEN_SOURCE_COLORS, formatBucketLabel, placeTrendTooltipAfter } from './tokenStatsShared'

ChartJS.register(CategoryScale, LinearScale, PointElement, LineElement, Filler, Tooltip)

const props = defineProps<{
  trend: TokenStatsBucket[]
  bucketWidth: string
}>()

const { t } = useI18n()

const CHART_HEIGHT_PX = 200
const WF_COLOR = TOKEN_SOURCE_COLORS.workflow
const PM_COLOR = TOKEN_SOURCE_COLORS.pm

const tip = ref({ show: false, x: 0, y: 0, idx: -1 })
const tipEl = ref<HTMLElement | null>(null)
const tipBucket = computed(() => (tip.value.idx >= 0 ? props.trend[tip.value.idx] : null))

type LastCaret = { canvas: HTMLCanvasElement; caretX: number; caretY: number }
let lastCaret: LastCaret | null = null

function fmtCompact(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`
  if (n >= 1000) return `${(n / 1000).toFixed(1)}K`
  return String(n)
}

function hideTip() {
  lastCaret = null
  tip.value = { ...tip.value, show: false, idx: -1 }
}

function repositionTip() {
  const el = tipEl.value
  if (!tip.value.show || !lastCaret || !el) return
  const { canvas, caretX, caretY } = lastCaret
  const canvasRect = canvas.getBoundingClientRect()
  const pos = placeTrendTooltipAfter({
    caretX: canvasRect.left + caretX,
    caretY: canvasRect.top + caretY,
    tipW: el.offsetWidth,
    tipH: el.offsetHeight,
  })
  if (tip.value.x !== pos.left || tip.value.y !== pos.top) {
    tip.value = { ...tip.value, x: pos.left, y: pos.top }
  }
}

async function schedulePlaceTip() {
  await nextTick()
  repositionTip()
  if (tipEl.value && (tipEl.value.offsetWidth === 0 || tipEl.value.offsetHeight === 0)) {
    requestAnimationFrame(() => repositionTip())
  }
}

function externalTooltip(context: { chart: Chart; tooltip: TooltipModel<'line'> }) {
  const { chart, tooltip } = context

  if (tooltip.opacity === 0 || !tooltip.dataPoints?.length) {
    hideTip()
    return
  }

  const idx = tooltip.dataPoints[0]!.dataIndex
  lastCaret = { canvas: chart.canvas, caretX: tooltip.caretX, caretY: tooltip.caretY }
  tip.value = {
    show: true,
    x: tip.value.show ? tip.value.x : 0,
    y: tip.value.show ? tip.value.y : 0,
    idx,
  }
  return schedulePlaceTip()
}

function onViewportChange() {
  if (tip.value.show) repositionTip()
}

const chartData = computed(() => {
  const labels = props.trend.map((b) => formatBucketLabel(b.bucket, props.bucketWidth))
  const workflow = props.trend.map((b) => b.workflowTotal || 0)
  const pm = props.trend.map((b) => b.pmTotal || 0)
  return {
    labels,
    datasets: [
      {
        label: 'workflow',
        data: workflow,
        borderColor: WF_COLOR,
        backgroundColor: 'rgba(109, 92, 255, 0.28)',
        borderWidth: 1.8,
        fill: true,
        tension: 0.15,
        pointRadius: 0,
        pointHoverRadius: 0,
        stack: 'source',
      },
      {
        label: 'pm',
        data: pm,
        borderColor: PM_COLOR,
        backgroundColor: 'rgba(109, 92, 255, 0.14)',
        borderWidth: 1.8,
        borderDash: [5, 4],
        fill: true,
        tension: 0.15,
        pointRadius: 3.5,
        pointHoverRadius: 5,
        pointBackgroundColor: '#fff',
        pointBorderColor: PM_COLOR,
        pointBorderWidth: 2,
        pointHoverBackgroundColor: '#fff',
        pointHoverBorderColor: PM_COLOR,
        pointHoverBorderWidth: 2,
        stack: 'source',
      },
    ],
  }
})

const chartOptions = computed<ChartOptions<'line'>>(() => ({
  responsive: true,
  maintainAspectRatio: false,
  interaction: { mode: 'index', intersect: false },
  plugins: {
    legend: { display: false },
    tooltip: {
      enabled: false,
      external: externalTooltip,
    },
  },
  scales: {
    x: {
      stacked: true,
      grid: { display: false },
      ticks: {
        maxTicksLimit: 8,
        autoSkip: true,
        maxRotation: 0,
        minRotation: 0,
        color: '#9aa1ad',
        font: { size: 10 },
      },
      border: { display: false },
    },
    y: {
      stacked: true,
      beginAtZero: true,
      grid: { color: '#eef0f3' },
      ticks: {
        maxTicksLimit: 4,
        color: '#9aa1ad',
        font: { size: 10 },
        callback: (value) => fmtCompact(Number(value)),
      },
      border: { display: false },
    },
  },
}))

watch(
  () => props.trend,
  () => hideTip(),
)

onMounted(() => {
  window.addEventListener('resize', onViewportChange)
  window.addEventListener('scroll', onViewportChange, true)
})

onBeforeUnmount(() => {
  window.removeEventListener('resize', onViewportChange)
  window.removeEventListener('scroll', onViewportChange, true)
  hideTip()
})

/** Exposed for unit tests: option mapping + external tooltip trigger. */
defineExpose({ chartOptions, chartData, externalTooltip, hideTip })
</script>

<template>
  <div
    data-testid="token-trend-wrap"
    class="token-trend-wrap relative w-full min-w-0 overflow-x-clip"
    :style="{ height: `${CHART_HEIGHT_PX}px` }"
    role="img"
    :aria-label="t('pages.board.tokenStats.trendTitle')"
    @mouseleave="hideTip"
  >
    <div
      data-testid="token-trend-legend"
      class="mb-1 flex flex-wrap items-center gap-3 text-[11px] text-txt3"
    >
      <span class="inline-flex items-center gap-1.5" data-kind="workflow">
        <i class="inline-block h-2 w-2 rounded-sm" :style="{ background: WF_COLOR }" />
        {{ t('pages.board.tokenStats.workflow') }}
      </span>
      <span class="inline-flex items-center gap-1.5" data-kind="pm">
        <i
          class="inline-block h-0 w-3 border-t-2 border-dashed"
          :style="{ borderColor: PM_COLOR }"
          aria-hidden="true"
        />
        {{ t('pages.board.tokenStats.pm') }}
      </span>
    </div>
    <div data-testid="token-trend-chart" class="h-[calc(100%-22px)] w-full">
      <Line :data="chartData" :options="chartOptions" />
    </div>
  </div>
  <Teleport to="body">
    <div
      v-if="tip.show && tipBucket"
      ref="tipEl"
      data-testid="token-trend-tooltip"
      class="pointer-events-none fixed z-[100] min-w-[140px] rounded-lg bg-[#1a1d23] px-2.5 py-2 text-[11px] text-white shadow-lg"
      :style="{ left: tip.x + 'px', top: tip.y + 'px' }"
    >
      <div class="mb-1 font-semibold">
        {{ formatBucketLabel(tipBucket.bucket, bucketWidth) }}
        · {{ fmtTokenCount(tipBucket.total) }}
      </div>
      <div class="flex justify-between gap-3 text-[#c7cbd4]" data-tip-row="workflow">
        <span class="inline-flex items-center gap-1.5">
          <i class="inline-block h-2 w-2 rounded-sm" :style="{ background: WF_COLOR }" />
          {{ t('pages.board.tokenStats.workflow') }}
        </span>
        <b class="font-normal tabular-nums text-white">{{ fmtTokenCount(tipBucket.workflowTotal || 0) }}</b>
      </div>
      <div class="flex justify-between gap-3 text-[#c7cbd4]" data-tip-row="pm">
        <span class="inline-flex items-center gap-1.5">
          <i
            class="inline-block h-0 w-3 border-t-2 border-dashed"
            :style="{ borderColor: PM_COLOR }"
            aria-hidden="true"
          />
          {{ t('pages.board.tokenStats.pm') }}
        </span>
        <b class="font-normal tabular-nums text-white">{{ fmtTokenCount(tipBucket.pmTotal || 0) }}</b>
      </div>
    </div>
  </Teleport>
</template>
