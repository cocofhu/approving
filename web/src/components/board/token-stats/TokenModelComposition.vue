<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import VChart from 'vue-echarts'
import { registerECharts } from '@/components/charts/echartsSetup'
import { pieChartOption } from '@/components/charts/chartTheme'
import type { TokenStatsModel } from '@/lib/shared/types'
import { colorForModel } from './tokenModelColors'

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
  return pieChartOption(
    visible.map((r, i) => ({
      name: r.name,
      value: r.total || 0,
      key: `${r.modelKey || r.name}-${i}`,
      color: r.color,
    })),
    true,
  )
})

/** Exposed for unit tests (ECharts option shape). */
const chartData = computed(
  () =>
    chartOption.value?.series?.[0]?.data?.map(
      (d: { name: string; value: number; itemStyle?: { color?: string } }, i: number) => ({
        name: d.name,
        value: d.value,
        color: d.itemStyle?.color || chartOption.value?.series?.[0]?.color?.[i],
      }),
    ) ?? [],
)

defineExpose({ chartOption, chartData })
</script>

<template>
  <div data-testid="token-model-composition" class="relative min-h-[168px] w-full overflow-visible">
    <div
      class="h-[168px] w-full"
      data-testid="token-model-pie"
      role="img"
      :aria-label="t('pages.board.tokenStats.modelCompositionTitle')"
    >
      <VChart v-if="chartOption" :option="chartOption" autoresize class="h-full w-full" />
      <div
        v-else
        class="h-full w-full bg-elevated"
        data-testid="token-model-pie-empty"
      />
    </div>
    <p
      v-if="!rows.length"
      class="m-0 mt-1 text-xs text-txt3"
      data-testid="token-model-legend"
    >
      {{ t('pages.board.tokenStats.emptyModelCompHint') }}
    </p>
  </div>
</template>
