<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import VChart from 'vue-echarts'
import { registerECharts } from '@/components/charts/echartsSetup'
import { pieChartOption } from '@/components/charts/chartTheme'
import type { TokenStatsComposition } from '@/lib/shared/types'
import { TOKEN_PART_COLORS, TOKEN_PART_KEYS, type TokenPartKey } from './tokenStatsShared'

registerECharts()

const props = defineProps<{
  composition: TokenStatsComposition
}>()

const { t } = useI18n()

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
  return pieChartOption(
    visible.map((p) => ({ name: partLabel(p.key), value: p.value, key: p.key, color: p.color })),
    false,
  )
})

defineExpose({ chartOption, parts })
</script>

<template>
  <div
    data-testid="token-donut-row"
    class="token-donut-row relative min-h-[168px] w-full overflow-visible"
  >
    <div
      data-testid="token-donut-chart"
      class="h-[168px] w-full"
      role="img"
      :aria-label="t('pages.board.tokenStats.compositionTitle')"
    >
      <VChart v-if="chartOption" :option="chartOption" autoresize class="h-full w-full" />
    </div>
  </div>
</template>
