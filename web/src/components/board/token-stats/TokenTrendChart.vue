<script setup lang="ts">
import { computed, ref, watch } from 'vue'
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
import type { TokenStatsBucket } from '@/lib/types'
import { fmtTokenCount } from '@/lib/tokenUsage'
import { TOKEN_SOURCE_COLORS, formatBucketLabel } from './tokenStatsShared'

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
const tipBucket = computed(() => (tip.value.idx >= 0 ? props.trend[tip.value.idx] : null))

function fmtCompact(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`
  if (n >= 1000) return `${(n / 1000).toFixed(1)}K`
  return String(n)
}

function hideTip() {
  tip.value = { ...tip.value, show: false, idx: -1 }
}

function externalTooltip(context: { chart: Chart; tooltip: TooltipModel<'line'> }) {
  const { chart, tooltip } = context
  const wrap = chart.canvas.closest('[data-testid="token-trend-wrap"]') as HTMLElement | null
  if (!wrap) return

  if (tooltip.opacity === 0 || !tooltip.dataPoints?.length) {
    hideTip()
    return
  }

  const idx = tooltip.dataPoints[0]!.dataIndex
  const rect = wrap.getBoundingClientRect()
  const canvasRect = chart.canvas.getBoundingClientRect()
  const caretX = canvasRect.left - rect.left + tooltip.caretX
  const caretY = canvasRect.top - rect.top + tooltip.caretY

  tip.value = {
    show: true,
    x: Math.min(Math.max(caretX, 8), rect.width - 160),
    y: Math.max(caretY - 8, 8),
    idx,
  }
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

/** Exposed for unit tests: option mapping (tick thinning / aspect / tooltip). */
defineExpose({ chartOptions, chartData })
</script>

<template>
  <div
    data-testid="token-trend-wrap"
    class="token-trend-wrap relative w-full min-w-0 overflow-x-clip"
    :style="{ height: `${CHART_HEIGHT_PX}px` }"
    role="img"
    :aria-label="t('pages.board.tokenStats.trendTitle')"
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
    <div
      v-if="tip.show && tipBucket"
      data-testid="token-trend-tooltip"
      class="pointer-events-none absolute z-10 min-w-[140px] rounded-lg bg-[#1a1d23] px-2.5 py-2 text-[11px] text-white shadow-lg"
      :style="{ left: tip.x + 'px', top: tip.y + 'px', transform: 'translate(-50%, -100%)' }"
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
  </div>
</template>
