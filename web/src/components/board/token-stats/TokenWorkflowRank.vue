<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import VChart from 'vue-echarts'
import { registerECharts } from '@/components/charts/echartsSetup'
import { BOARD_CHART_TOOLTIP } from '@/components/charts/chartTheme'
import type { TokenStatsWorkflow } from '@/lib/shared/types'
import { fmtCompactTokenCount, fmtTokenCount } from '@/lib/run/tokenUsage'

registerECharts()

const WF_BAR_GRADIENT = {
  type: 'linear' as const,
  x: 0,
  y: 0,
  x2: 1,
  y2: 0,
  colorStops: [
    { offset: 0, color: '#6d5cff' },
    { offset: 1, color: '#9b8cff' },
  ],
}

const OTHER_BAR_GRADIENT = {
  type: 'linear' as const,
  x: 0,
  y: 0,
  x2: 1,
  y2: 0,
  colorStops: [
    { offset: 0, color: '#94a3b8' },
    { offset: 1, color: '#cbd5e1' },
  ],
}

const props = defineProps<{
  workflows: TokenStatsWorkflow[]
}>()

const { t } = useI18n()
const activeIdx = ref<number | null>(null)

const maxTotal = computed(() => Math.max(1, ...props.workflows.map((w) => w.total || 0)))

function isPM(w: TokenStatsWorkflow): boolean {
  return w.kind === 'pm' || (!w.other && w.name === 'PM' && !w.workflowId)
}

function isOther(w: TokenStatsWorkflow): boolean {
  return !!w.other || w.kind === 'other'
}

function displayName(w: TokenStatsWorkflow): string {
  if (isOther(w)) return t('pages.board.tokenStats.other')
  if (isPM(w)) return t('pages.board.tokenStats.pm')
  return w.name || w.workflowId || '—'
}

function badgeLabel(w: TokenStatsWorkflow, i: number): string {
  if (isOther(w)) return '·'
  let n = 0
  for (let j = 0; j <= i; j++) {
    const row = props.workflows[j]
    if (row && !isOther(row)) n += 1
  }
  return String(n)
}

function badgeClass(w: TokenStatsWorkflow, i: number): string {
  if (isOther(w)) return 'w-[22px] bg-elevated text-txt3'
  const n = Number(badgeLabel(w, i))
  return n <= 3
    ? 'w-[22px] bg-accent-dim text-accent-2'
    : 'w-[22px] bg-elevated text-txt3'
}

function barColor(w: TokenStatsWorkflow) {
  return isOther(w) ? OTHER_BAR_GRADIENT : WF_BAR_GRADIENT
}

function rowChartOption(w: TokenStatsWorkflow) {
  return {
    animation: false,
    grid: { left: 0, right: 0, top: 0, bottom: 0 },
    xAxis: { type: 'value', show: false, max: maxTotal.value },
    yAxis: { type: 'category', data: [''], show: false },
    tooltip: {
      trigger: 'item',
      ...BOARD_CHART_TOOLTIP,
      formatter: () => fmtTokenCount(w.total),
    },
    series: [
      {
        type: 'bar',
        data: [w.total || 0],
        barWidth: 8,
        itemStyle: { color: barColor(w), borderRadius: [0, 999, 999, 0] },
        showBackground: true,
        backgroundStyle: { color: '#f1f3f6' },
      },
    ],
  }
}

const rowOptions = computed(() => props.workflows.map((w) => rowChartOption(w)))

const tip = ref({ show: false, x: 0, y: 0, idx: -1 })

function showTip(i: number, ev: MouseEvent) {
  activeIdx.value = i
  const wrap = (ev.currentTarget as HTMLElement).closest('[data-testid="token-rank-list"]') as HTMLElement | null
  if (!wrap) return
  const rect = wrap.getBoundingClientRect()
  tip.value = {
    show: true,
    x: ev.clientX - rect.left,
    y: ev.clientY - rect.top,
    idx: i,
  }
}

function hideTip() {
  activeIdx.value = null
  tip.value = { ...tip.value, show: false }
}

const tipRow = computed(() => (tip.value.idx >= 0 ? props.workflows[tip.value.idx] : null))

defineExpose({ rowOptions, maxTotal, barColor })
</script>

<template>
  <ul data-testid="token-rank-list" class="token-rank relative m-0 grid list-none gap-2 p-0">
    <li
      v-for="(w, i) in workflows"
      :key="(w.kind || '') + '-' + (w.workflowId || w.name) + '-' + i"
      class="grid cursor-pointer grid-cols-[22px_1fr_auto] items-center gap-2 rounded-md px-0.5 py-1"
      :class="[
        isOther(w) ? 'token-rank-other' : '',
        isPM(w) ? 'token-rank-pm' : '',
        activeIdx === i ? 'bg-elevated' : 'hover:bg-elevated/80',
      ]"
      :data-kind="w.kind || (isPM(w) ? 'pm' : isOther(w) ? 'other' : 'workflow')"
      @mouseenter="showTip(i, $event)"
      @mousemove="showTip(i, $event)"
      @mouseleave="hideTip"
      @click="showTip(i, $event)"
    >
      <span
        class="grid h-[22px] place-items-center rounded-md text-[11px] font-bold"
        :class="badgeClass(w, i)"
      >
        {{ badgeLabel(w, i) }}
      </span>
      <div class="min-w-0">
        <div class="truncate text-xs font-medium text-txt">{{ displayName(w) }}</div>
        <div class="mt-1 h-2 overflow-hidden rounded-full" data-testid="token-rank-bar">
          <VChart :option="rowOptions[i]" autoresize class="h-full w-full" />
        </div>
      </div>
      <span
        class="whitespace-nowrap text-xs tabular-nums text-txt3"
        :title="fmtTokenCount(w.total)"
        data-testid="token-rank-value"
      >{{ fmtCompactTokenCount(w.total) }}</span>
    </li>
    <div
      v-if="tip.show && tipRow"
      data-testid="token-rank-tooltip"
      class="pointer-events-none absolute z-10 min-w-[120px] rounded-lg bg-[#1a1d23] px-2.5 py-2 text-[11px] text-white shadow-lg"
      :style="{ left: tip.x + 'px', top: tip.y + 'px', transform: 'translate(-10%, -110%)' }"
    >
      <div class="mb-1 font-semibold">{{ displayName(tipRow) }}</div>
      <div class="flex justify-between gap-3 text-[#c7cbd4]">
        <span>{{ t('pages.board.tokenStats.value') }}</span>
        <b class="font-normal tabular-nums text-white">{{ fmtTokenCount(tipRow.total) }}</b>
      </div>
    </div>
  </ul>
</template>
