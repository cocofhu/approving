<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { TokenStatsModel } from '@/lib/types'
import { fmtCompactTokenCount, fmtTokenCount } from '@/lib/tokenUsage'
import { MODEL_PALETTE, colorForModel } from './tokenModelColors'

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

const pieStops = computed(() => {
  let acc = 0
  const sum = total.value
  if (sum <= 0) return 'rgb(var(--c-elevated)) 0% 100%'
  return rows.value
    .map((r) => {
      const start = acc
      acc += (r.total / sum) * 100
      return `${r.color} ${start.toFixed(2)}% ${acc.toFixed(2)}%`
    })
    .join(', ')
})
</script>

<template>
  <div data-testid="token-model-composition" class="grid gap-3 sm:grid-cols-[120px_1fr] sm:items-center">
    <div
      class="mx-auto h-[110px] w-[110px] rounded-full"
      data-testid="token-model-pie"
      :style="{
        background: `conic-gradient(${pieStops})`,
        boxShadow: 'inset 0 0 0 28px rgb(var(--c-surface))',
      }"
      role="img"
      :aria-label="t('pages.board.tokenStats.modelCompositionTitle')"
    />
    <ul class="m-0 grid list-none gap-1.5 p-0" data-testid="token-model-legend">
      <li
        v-for="(r, i) in rows"
        :key="(r.modelKey || r.name) + '-' + i"
        class="grid grid-cols-[10px_1fr_auto] items-center gap-2 text-xs"
      >
        <span class="h-2.5 w-2.5" :style="{ background: r.color || MODEL_PALETTE[0] }" />
        <span
          class="min-w-0 truncate"
          :class="r.unknown ? 'text-txt3' : r.filled ? 'text-ok' : 'text-txt'"
          :title="r.name"
        >
          {{ r.name }}
          <span v-if="r.filled" class="ml-1 text-[10px] text-ok">{{ t('pages.board.tokenStats.filledTag') }}</span>
        </span>
        <span class="tabular-nums text-txt3" :title="fmtTokenCount(r.total)">
          {{ r.pct }}% · {{ fmtCompactTokenCount(r.total) }}
        </span>
      </li>
      <li v-if="!rows.length" class="text-xs text-txt3">{{ t('pages.board.tokenStats.emptyModelCompHint') }}</li>
    </ul>
  </div>
</template>
