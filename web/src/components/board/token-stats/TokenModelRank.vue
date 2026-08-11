<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { TokenStatsModel } from '@/lib/shared/types'
import { fmtCompactTokenCount, fmtTokenCount } from '@/lib/run/tokenUsage'
import { colorForModel } from './tokenModelColors'

const props = defineProps<{
  models: TokenStatsModel[]
}>()

const { t } = useI18n()
const maxTotal = computed(() => Math.max(1, ...props.models.map((m) => m.total || 0)))

function displayName(m: TokenStatsModel): string {
  if (m.other) return t('pages.board.tokenStats.modelOther')
  return m.name || m.modelKey || '—'
}

function barWidth(total: number): string {
  return `${Math.max(2, Math.round((total / maxTotal.value) * 100))}%`
}
</script>

<template>
  <ul data-testid="token-model-rank" class="m-0 grid list-none gap-2 p-0">
    <li
      v-for="(m, i) in models"
      :key="(m.modelKey || m.name) + '-' + i"
      class="grid grid-cols-[1fr_auto] items-center gap-2"
      :data-unknown="m.unknown ? '1' : '0'"
      :data-other="m.other ? '1' : '0'"
      :data-filled="m.filled ? '1' : '0'"
    >
      <div class="min-w-0">
        <div
          class="truncate text-xs font-medium"
          :class="m.unknown ? 'text-txt3' : m.filled ? 'text-ok' : 'text-txt'"
          :title="displayName(m)"
        >
          {{ displayName(m) }}
        </div>
        <div class="mt-1 h-2 overflow-hidden bg-elevated">
          <div
            class="h-full transition-[width] duration-300"
            :style="{ width: barWidth(m.total), background: colorForModel(m, i) }"
          />
        </div>
      </div>
      <span
        class="whitespace-nowrap text-xs tabular-nums text-txt3"
        :title="fmtTokenCount(m.total)"
      >{{ fmtCompactTokenCount(m.total) }}</span>
    </li>
    <li v-if="!models.length" class="text-xs text-txt3">{{ t('pages.board.tokenStats.emptyModelRankHint') }}</li>
  </ul>
</template>
