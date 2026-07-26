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
import { TOKEN_PART_COLORS, TOKEN_PART_KEYS, formatBucketLabel } from './tokenStatsShared'

ChartJS.register(CategoryScale, LinearScale, PointElement, LineElement, Filler, Tooltip)

const props = defineProps<{
  trend: TokenStatsBucket[]
  bucketWidth: string
}>()

const { t } = useI18n()

const LINE_COLOR = '#6d5cff'
const CHART_HEIGHT_PX = 200

const tip = ref({ show: false, x: 0, y: 0, idx: -1 })
const tipBucket = computed(() => (tip.value.idx >= 0 ? props.trend[tip.value.idx] : null))

function fmtCompact(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`
  if (n >= 1000) return `${(n / 1000).toFixed(1)}K`
  return String(n)
}

function partValue(bucket: TokenStatsBucket, key: (typeof TOKEN_PART_KEYS)[number]): number {
  if (key === 'input') return bucket.inputTokens
  if (key === 'output') return bucket.outputTokens
  if (key === 'cacheRead') return bucket.cacheReadTokens
  return bucket.cacheWriteTokens
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
  const totals = props.trend.map((b) => b.total || 0)
  return {
    labels,
    datasets: [
      {
        label: t('pages.board.tokenStats.trendTitle'),
        data: totals,
        borderColor: LINE_COLOR,
        backgroundColor: 'rgba(109, 92, 255, 0.18)',
        borderWidth: 2.2,
        fill: true,
        tension: 0.15,
        pointRadius: 3.5,
        pointHoverRadius: 5,
        pointBackgroundColor: '#fff',
        pointBorderColor: LINE_COLOR,
        pointBorderWidth: 2,
        pointHoverBackgroundColor: '#fff',
        pointHoverBorderColor: LINE_COLOR,
        pointHoverBorderWidth: 2,
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
    <div data-testid="token-trend-chart" class="h-full w-full">
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
      <div
        v-for="key in TOKEN_PART_KEYS"
        :key="key"
        class="flex justify-between gap-3 text-[#c7cbd4]"
      >
        <span class="inline-flex items-center gap-1.5">
          <i class="inline-block h-2 w-2 rounded-sm" :style="{ background: TOKEN_PART_COLORS[key] }" />
          {{ key }}
        </span>
        <b class="font-normal tabular-nums text-white">{{ fmtTokenCount(partValue(tipBucket, key)) }}</b>
      </div>
    </div>
  </div>
</template>
