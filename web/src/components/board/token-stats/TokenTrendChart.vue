<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import VChart from 'vue-echarts'
import type { ECElementEvent } from 'echarts/core'
import { registerECharts } from '@/components/charts/echartsSetup'
import { CHART_AXIS, CHART_GRID, fmtCompactAxis } from '@/components/charts/chartTheme'
import type { TokenStatsBucket } from '@/lib/shared/types'
import { fmtTokenCount } from '@/lib/run/tokenUsage'
import { TOKEN_SOURCE_COLORS, formatBucketLabel, placeTrendTooltipAfter } from './tokenStatsShared'

registerECharts()

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

function hideTip() {
  tip.value = { ...tip.value, show: false, idx: -1 }
}

function onChartMouseMove(params: ECElementEvent) {
  if (params.componentType !== 'series' || params.dataIndex == null) return
  const idx = params.dataIndex
  tip.value = { show: true, x: tip.value.x, y: tip.value.y, idx }
  const ev = params.event?.event as MouseEvent | undefined
  if (!ev || !tipEl.value) return
  const pos = placeTrendTooltipAfter({
    caretX: ev.clientX,
    caretY: ev.clientY,
    tipW: tipEl.value.offsetWidth || 168,
    tipH: tipEl.value.offsetHeight || 72,
  })
  tip.value = { ...tip.value, x: pos.left, y: pos.top }
}

const chartOption = computed(() => {
  const labels = props.trend.map((b) => formatBucketLabel(b.bucket, props.bucketWidth))
  const workflow = props.trend.map((b) => b.workflowTotal || 0)
  const pm = props.trend.map((b) => b.pmTotal || 0)
  return {
    grid: CHART_GRID,
    tooltip: { show: false },
    xAxis: {
      type: 'category',
      data: labels,
      ...CHART_AXIS,
      splitLine: { show: false },
      axisLabel: { ...CHART_AXIS.axisLabel, maxInterval: Math.ceil(labels.length / 8) },
    },
    yAxis: {
      type: 'value',
      ...CHART_AXIS,
      axisLabel: { ...CHART_AXIS.axisLabel, formatter: (v: number) => fmtCompactAxis(v) },
    },
    series: [
      {
        type: 'line',
        name: 'workflow',
        stack: 'source',
        data: workflow,
        lineStyle: { color: WF_COLOR, width: 1.8 },
        areaStyle: { color: 'rgba(109, 92, 255, 0.28)' },
        showSymbol: false,
        smooth: true,
      },
      {
        type: 'line',
        name: 'pm',
        stack: 'source',
        data: pm,
        lineStyle: { color: PM_COLOR, width: 1.8, type: [5, 4] as unknown as 'dashed' },
        areaStyle: { color: 'rgba(109, 92, 255, 0.14)' },
        symbol: 'circle',
        symbolSize: 7,
        itemStyle: { color: '#fff', borderColor: PM_COLOR, borderWidth: 2 },
        smooth: true,
      },
    ],
  }
})

/** Legacy expose for unit tests (ECharts option shape). */
const chartOptions = computed(() => ({
  maintainAspectRatio: false,
  scales: { x: { ticks: { maxTicksLimit: 8, autoSkip: true } } },
  plugins: { tooltip: { enabled: false, external: externalTooltip } },
}))

const chartData = computed(() => ({
  labels: props.trend.map((b) => formatBucketLabel(b.bucket, props.bucketWidth)),
  datasets: [
    { label: 'workflow', data: props.trend.map((b) => b.workflowTotal || 0), borderColor: WF_COLOR },
    {
      label: 'pm',
      data: props.trend.map((b) => b.pmTotal || 0),
      borderColor: PM_COLOR,
      borderDash: [5, 4],
    },
  ],
}))

function externalTooltip(context: {
  chart: { canvas: HTMLCanvasElement }
  tooltip: { opacity: number; caretX: number; caretY: number; dataPoints?: { dataIndex: number }[] }
}) {
  const { tooltip } = context
  if (tooltip.opacity === 0 || !tooltip.dataPoints?.length) {
    hideTip()
    return
  }
  const idx = tooltip.dataPoints[0]!.dataIndex
  tip.value = { show: true, x: tooltip.caretX, y: tooltip.caretY, idx }
}

function onViewportChange() {
  if (tip.value.show && tipEl.value) {
    // reposition handled on next mousemove
  }
}

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
      <VChart :option="chartOption" autoresize class="h-full w-full" @mousemove="onChartMouseMove" @globalout="hideTip" />
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
