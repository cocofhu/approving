<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import VChart from 'vue-echarts'
import { registerECharts } from '@/components/charts/echartsSetup'
import { BOARD_CHART_TOOLTIP } from '@/components/charts/chartTheme'
import type { TokenStatsComposition } from '@/lib/shared/types'
import { fmtCompactTokenCount, fmtTokenCount } from '@/lib/run/tokenUsage'
import { TOKEN_PART_COLORS, TOKEN_PART_KEYS, type TokenPartKey } from './tokenStatsShared'

registerECharts()

const props = defineProps<{
  composition: TokenStatsComposition
}>()

const { t } = useI18n()
const active = ref<TokenPartKey | null>(null)

const PART_LABEL_KEYS: Record<TokenPartKey, string> = {
  input: 'pages.executionTimeline.partInput',
  output: 'pages.executionTimeline.partOutput',
  cacheRead: 'pages.executionTimeline.partCacheRead',
  cacheWrite: 'pages.executionTimeline.partCacheWrite',
}

function partLabel(key: TokenPartKey): string {
  return t(PART_LABEL_KEYS[key])
}

const parts = computed(() => {
  const c = props.composition
  const values: Record<TokenPartKey, number> = {
    input: c.inputTokens || 0,
    output: c.outputTokens || 0,
    cacheRead: c.cacheReadTokens || 0,
    cacheWrite: c.cacheWriteTokens || 0,
  }
  const total = c.total || TOKEN_PART_KEYS.reduce((s, k) => s + values[k], 0)
  return TOKEN_PART_KEYS.map((key) => ({
    key,
    value: values[key],
    pct: total > 0 ? Math.round((values[key] / total) * 1000) / 10 : 0,
    color: TOKEN_PART_COLORS[key],
  })).filter((p) => p.value > 0 || total === 0)
})

const chartOption = computed(() => {
  const visible = parts.value.filter((p) => p.value > 0)
  if (!visible.length) return null
  return {
    tooltip: { trigger: 'item', ...BOARD_CHART_TOOLTIP, formatter: '{b}: {c} ({d}%)' },
    series: [
      {
        type: 'pie',
        radius: ['32%', '52%'],
        center: ['50%', '50%'],
        data: visible.map((p) => ({ name: partLabel(p.key), value: p.value, key: p.key })),
        color: visible.map((p) => p.color),
        label: { show: false },
        emphasis: {
          scale: true,
          itemStyle: { opacity: active.value && active.value !== undefined ? 0.45 : 0.95 },
        },
      },
    ],
  }
})

const tip = ref({ show: false, x: 0, y: 0, key: null as TokenPartKey | null })

function activate(key: TokenPartKey, ev?: MouseEvent) {
  active.value = key
  if (!ev) return
  const wrap = (ev.currentTarget as HTMLElement).closest('[data-testid="token-donut-row"]') as HTMLElement | null
  if (!wrap) return
  const rect = wrap.getBoundingClientRect()
  tip.value = {
    show: true,
    x: ev.clientX - rect.left,
    y: ev.clientY - rect.top,
    key,
  }
}

function deactivate() {
  active.value = null
  tip.value = { ...tip.value, show: false }
}

const tipPart = computed(() => parts.value.find((p) => p.key === tip.value.key) || null)
</script>

<template>
  <div
    data-testid="token-donut-row"
    class="token-donut-row relative flex min-h-[180px] flex-col items-center gap-4 sm:flex-row sm:items-center"
  >
    <div
      data-testid="token-donut-chart"
      class="relative h-[120px] w-[120px] shrink-0 sm:h-[150px] sm:w-[150px]"
      role="img"
      :aria-label="t('pages.board.tokenStats.compositionTitle')"
    >
      <VChart v-if="chartOption" :option="chartOption" autoresize class="h-full w-full" />
      <div class="pointer-events-none absolute inset-0 flex flex-col items-center justify-center">
        <span class="text-[11px] text-txt3">{{ t('pages.board.tokenStats.donutCenter') }}</span>
        <span class="text-[15px] font-bold text-txt">{{ fmtCompactTokenCount(composition.total) }}</span>
      </div>
    </div>
    <ul data-testid="token-donut-legend" class="m-0 grid w-full min-w-0 list-none gap-2 p-0 sm:flex-1">
      <li
        v-for="p in parts"
        :key="p.key"
        class="grid cursor-pointer grid-cols-[10px_1fr_auto] items-center gap-2 rounded-md px-1.5 py-1 text-xs"
        :class="active === p.key ? 'bg-elevated' : 'hover:bg-elevated/80'"
        @mouseenter="activate(p.key)"
        @mouseleave="deactivate"
        @click="activate(p.key, $event)"
      >
        <i class="h-2 w-2 rounded-sm" :style="{ background: p.color }" />
        <span class="truncate text-txt">{{ partLabel(p.key) }}</span>
        <span class="tabular-nums text-txt3">{{ p.pct }}%</span>
      </li>
    </ul>
    <div
      v-if="tip.show && tipPart"
      data-testid="token-donut-tooltip"
      class="pointer-events-none absolute z-10 min-w-[120px] rounded-lg bg-[#1a1d23] px-2.5 py-2 text-[11px] text-white shadow-lg"
      :style="{ left: tip.x + 'px', top: tip.y + 'px', transform: 'translate(-20%, -110%)' }"
    >
      <div class="mb-1 font-semibold">{{ partLabel(tipPart.key) }}</div>
      <div class="flex justify-between gap-3 text-[#c7cbd4]">
        <span>{{ t('pages.board.tokenStats.value') }}</span>
        <b class="font-normal tabular-nums text-white">{{ fmtTokenCount(tipPart.value) }}</b>
      </div>
      <div class="flex justify-between gap-3 text-[#c7cbd4]">
        <span>{{ t('pages.board.tokenStats.share') }}</span>
        <b class="font-normal tabular-nums text-white">{{ tipPart.pct }}%</b>
      </div>
    </div>
  </div>
</template>
