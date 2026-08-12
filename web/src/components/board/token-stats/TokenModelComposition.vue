<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { TokenStatsModel } from '@/lib/shared/types'
import { fmtCompactTokenCount, fmtTokenCount } from '@/lib/run/tokenUsage'
import { MODEL_PALETTE, colorForModel } from './tokenModelColors'
import UnknownModelBadge from '@/components/ui/UnknownModelBadge.vue'

const props = defineProps<{
  models: TokenStatsModel[]
}>()

const { t } = useI18n()

const SIZE = 110
const CX = SIZE / 2
const CY = SIZE / 2
const R = SIZE / 2

const total = computed(() => props.models.reduce((s, m) => s + (m.total || 0), 0))

const rows = computed(() => {
  const sum = total.value || 1
  return props.models.map((m, i) => ({
    ...m,
    pct: Math.round(((m.total || 0) / sum) * 1000) / 10,
    color: colorForModel(m, i),
  }))
})

function polar(cx: number, cy: number, r: number, angleDeg: number) {
  const rad = ((angleDeg - 90) * Math.PI) / 180
  return { x: cx + r * Math.cos(rad), y: cy + r * Math.sin(rad) }
}

/** Solid pie slice from center (Demo / StatsPieChart-aligned). Full circle = two semicircles. */
function describeSlice(cx: number, cy: number, r: number, startDeg: number, endDeg: number): string {
  if (endDeg - startDeg >= 359.999) {
    const mid = polar(cx, cy, r, startDeg + 180)
    const end = polar(cx, cy, r, startDeg + 360)
    const start = polar(cx, cy, r, startDeg)
    return [
      `M ${cx} ${cy}`,
      `L ${start.x} ${start.y}`,
      `A ${r} ${r} 0 1 1 ${mid.x} ${mid.y}`,
      `A ${r} ${r} 0 1 1 ${end.x} ${end.y}`,
      'Z',
    ].join(' ')
  }
  const start = polar(cx, cy, r, startDeg)
  const end = polar(cx, cy, r, endDeg)
  const large = endDeg - startDeg > 180 ? 1 : 0
  return [
    `M ${cx} ${cy}`,
    `L ${start.x} ${start.y}`,
    `A ${r} ${r} 0 ${large} 1 ${end.x} ${end.y}`,
    'Z',
  ].join(' ')
}

const slices = computed(() => {
  const sum = total.value
  if (sum <= 0) return [] as { key: string; d: string; color: string }[]
  let angle = 0
  const out: { key: string; d: string; color: string }[] = []
  rows.value.forEach((r, i) => {
    const sweep = ((r.total || 0) / sum) * 360
    if (sweep <= 0) return
    out.push({
      key: `${r.modelKey || r.name}-${i}`,
      d: describeSlice(CX, CY, R, angle, angle + sweep),
      color: r.color,
    })
    angle += sweep
  })
  return out
})
</script>

<template>
  <div data-testid="token-model-composition" class="grid gap-3 sm:grid-cols-[120px_1fr] sm:items-center">
    <!-- SVG solid pie: path geometry ignores global border-radius:0 (Demo「修复后」) -->
    <div
      class="mx-auto h-[110px] w-[110px]"
      data-testid="token-model-pie"
      role="img"
      :aria-label="t('pages.board.tokenStats.modelCompositionTitle')"
    >
      <svg
        v-if="slices.length"
        class="block h-full w-full"
        :viewBox="`0 0 ${SIZE} ${SIZE}`"
        width="110"
        height="110"
        aria-hidden="true"
      >
        <path
          v-for="s in slices"
          :key="s.key"
          :d="s.d"
          :fill="s.color"
          data-testid="token-model-pie-slice"
        />
      </svg>
      <svg
        v-else
        class="block h-full w-full"
        :viewBox="`0 0 ${SIZE} ${SIZE}`"
        width="110"
        height="110"
        aria-hidden="true"
      >
        <circle :cx="CX" :cy="CY" :r="R" fill="rgb(var(--c-elevated))" />
      </svg>
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
          :class="r.unknown ? 'text-txt3' : r.filled ? 'text-ok' : 'text-txt'"
          :title="r.name"
        >
          <span class="min-w-0 truncate">{{ r.name }}</span>
          <UnknownModelBadge v-if="r.unknown" />
        </span>
        <span class="tabular-nums text-txt3" :title="fmtTokenCount(r.total)">
          {{ r.pct }}% · {{ fmtCompactTokenCount(r.total) }}
        </span>
      </li>
      <li v-if="!rows.length" class="text-xs text-txt3">{{ t('pages.board.tokenStats.emptyModelCompHint') }}</li>
    </ul>
  </div>
</template>
