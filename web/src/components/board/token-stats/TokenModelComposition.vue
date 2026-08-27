<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import VChart from 'vue-echarts'
import { registerECharts } from '@/components/charts/echartsSetup'
import { BOARD_CHART_TOOLTIP } from '@/components/charts/chartTheme'
import type { TokenStatsModel } from '@/lib/shared/types'
import { fmtCompactTokenCount, fmtTokenCount, shouldShowUnknownVisual } from '@/lib/run/tokenUsage'
import { MODEL_PALETTE, colorForModel } from './tokenModelColors'
import UnknownModelBadge from '@/components/ui/UnknownModelBadge.vue'

registerECharts()

const props = defineProps<{
  models: TokenStatsModel[]
}>()

const { t } = useI18n()

const total = computed(() => props.models.reduce((s, m) => s + (m.total || 0), 0))

const rows = computed(() => {
  const sum = total.value || 1
  return props.models.map((m, i) => ({
    ...m,
    pct: Math.round(((m.total || 0) / sum) * 1000) / 10,
    color: colorForModel(m, i),
  }))
})

const chartOption = computed(() => {
  const sum = total.value
  if (sum <= 0) return null
  const visible = rows.value.filter((r) => (r.total || 0) > 0)
  if (!visible.length) return null
  return {
    tooltip: {
      trigger: 'item',
      ...BOARD_CHART_TOOLTIP,
      formatter: (p: { name: string; value: number; percent: number }) =>
        `${p.name}: ${fmtTokenCount(p.value)} (${p.percent}%)`,
    },
    series: [
      {
        type: 'pie',
        radius: '100%',
        center: ['50%', '50%'],
        data: visible.map((r, i) => ({
          name: r.name,
          value: r.total || 0,
          itemStyle: { color: r.color },
          key: `${r.modelKey || r.name}-${i}`,
        })),
        label: { show: false },
        emphasis: { scale: true, scaleSize: 4 },
      },
    ],
  }
})

/** Exposed for unit tests (ECharts option shape). */
const chartData = computed(() =>
  chartOption.value?.series?.[0]?.data?.map((d: { name: string; value: number; itemStyle?: { color: string } }) => ({
    name: d.name,
    value: d.value,
    color: d.itemStyle?.color,
  })) ?? [],
)

defineExpose({ chartOption, chartData })
</script>

<template>
  <div data-testid="token-model-composition" class="grid gap-3 sm:grid-cols-[120px_1fr] sm:items-center">
    <div
      class="mx-auto h-[110px] w-[110px]"
      data-testid="token-model-pie"
      role="img"
      :aria-label="t('pages.board.tokenStats.modelCompositionTitle')"
    >
      <VChart v-if="chartOption" :option="chartOption" autoresize class="h-full w-full" />
      <div
        v-else
        class="h-full w-full rounded-full bg-elevated"
        data-testid="token-model-pie-empty"
      />
    </div>
    <ul class="m-0 grid list-none gap-1.5 p-0" data-testid="token-model-legend">
      <li
        v-for="(r, i) in rows"
        :key="(r.modelKey || r.name) + '-' + i"
        class="grid grid-cols-[10px_1fr_auto] items-center gap-2 text-xs"
      >
        <span class="h-2.5 w-2.5" :style="{ background: r.color || MODEL_PALETTE[0] }" />
        <span
          class="flex min-w-0 items-center gap-1.5 truncate"
          :class="shouldShowUnknownVisual(r.unknown, r.name) ? 'text-txt3' : r.filled ? 'text-ok' : 'text-txt'"
          :title="r.name"
        >
          <span class="min-w-0 truncate">{{ r.name }}</span>
          <UnknownModelBadge v-if="shouldShowUnknownVisual(r.unknown, r.name)" />
        </span>
        <span class="tabular-nums text-txt3" :title="fmtTokenCount(r.total)">
          {{ r.pct }}% · {{ fmtCompactTokenCount(r.total) }}
        </span>
      </li>
      <li v-if="!rows.length" class="text-xs text-txt3">{{ t('pages.board.tokenStats.emptyModelCompHint') }}</li>
    </ul>
  </div>
</template>
