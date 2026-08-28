<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import VChart from 'vue-echarts'
import { registerECharts } from '@/components/charts/echartsSetup'
import {
  STATS_CHART_GRID,
  axisTooltip,
  fmtCompactAxis,
  statsAxis,
  statsLegend,
} from '@/components/charts/chartTheme'
import type { TokenStatsBucket } from '@/lib/shared/types'
import { fmtCompactTokenCount } from '@/lib/run/tokenUsage'
import { TOKEN_SOURCE_COLORS, formatBucketLabel } from './tokenStatsShared'

registerECharts()

const props = defineProps<{
  trend: TokenStatsBucket[]
  bucketWidth: string
}>()

const { t } = useI18n()

const CHART_HEIGHT_PX = 200
const WF_COLOR = TOKEN_SOURCE_COLORS.workflow
const PM_COLOR = TOKEN_SOURCE_COLORS.pm

const wfName = computed(() => t('pages.board.tokenStats.workflow'))
const pmName = computed(() => t('pages.board.tokenStats.pm'))

const chartOption = computed(() => {
  const labels = props.trend.map((b) => formatBucketLabel(b.bucket, props.bucketWidth))
  const workflow = props.trend.map((b) => b.workflowTotal || 0)
  const pm = props.trend.map((b) => b.pmTotal || 0)
  const axis = statsAxis()
  return {
    grid: STATS_CHART_GRID,
    tooltip: axisTooltip({
      className: 'token-stats-echarts-tooltip token-trend-tooltip',
      formatter: (params: unknown) => {
        const items = (Array.isArray(params) ? params : [params]) as { dataIndex?: number }[]
        const idx = items[0]?.dataIndex ?? 0
        const b = props.trend[idx]
        if (!b) return ''
        const label = formatBucketLabel(b.bucket, props.bucketWidth)
        const total = fmtCompactTokenCount(b.total)
        return `<div>${label} · ${total}</div>
          <div data-tip-row="workflow">${wfName.value} ${fmtCompactTokenCount(b.workflowTotal || 0)}</div>
          <div data-tip-row="pm">${pmName.value} ${fmtCompactTokenCount(b.pmTotal || 0)}</div>`
      },
    }),
    legend: {
      ...statsLegend(),
      data: [wfName.value, pmName.value],
    },
    xAxis: {
      type: 'category',
      data: labels,
      ...axis,
      splitLine: { show: false },
      axisLabel: { ...axis.axisLabel, maxInterval: Math.ceil(labels.length / 8) },
    },
    yAxis: {
      type: 'value',
      ...axis,
      axisLabel: { ...axis.axisLabel, formatter: (v: number) => fmtCompactAxis(v) },
    },
    series: [
      {
        type: 'line',
        name: wfName.value,
        stack: 'source',
        data: workflow,
        lineStyle: { color: WF_COLOR, width: 1.8 },
        areaStyle: { color: 'rgba(109, 92, 255, 0.28)' },
        showSymbol: false,
        smooth: true,
      },
      {
        type: 'line',
        name: pmName.value,
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

defineExpose({ chartOption, chartData })
</script>

<template>
  <div
    data-testid="token-trend-wrap"
    class="token-trend-wrap relative w-full min-w-0 overflow-x-clip overflow-y-visible"
    :style="{ height: `${CHART_HEIGHT_PX}px` }"
    role="img"
    :aria-label="t('pages.board.tokenStats.trendTitle')"
  >
    <div data-testid="token-trend-chart" class="h-full w-full overflow-visible">
      <VChart :option="chartOption" autoresize class="h-full w-full" />
    </div>
  </div>
</template>
