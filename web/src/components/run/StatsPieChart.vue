<script setup lang="ts">
import { computed } from 'vue'
import TruncatedTextTooltip from '@/components/ui/TruncatedTextTooltip.vue'

export interface PieSlice {
  key: string
  label: string
  durationSec: number
  color: string
  sharePct: number | null
}

const props = defineProps<{
  items: PieSlice[]
  centerValue: string
  centerSub: string
  emptyLabel?: string
  /** Shown under the legend when slices use node-sum (not wall) as denominator. */
  footnote?: string
}>()

function polarToCartesian(cx: number, cy: number, r: number, angleDeg: number) {
  const rad = ((angleDeg - 90) * Math.PI) / 180
  return { x: cx + r * Math.cos(rad), y: cy + r * Math.sin(rad) }
}

function describeSlice(cx: number, cy: number, r: number, startAngle: number, endAngle: number): string {
  if (endAngle - startAngle >= 359.99) {
    const m = startAngle + 180
    const p0 = polarToCartesian(cx, cy, r, startAngle)
    const p1 = polarToCartesian(cx, cy, r, m)
    const p2 = polarToCartesian(cx, cy, r, endAngle)
    return `M ${p0.x} ${p0.y} A ${r} ${r} 0 1 1 ${p1.x} ${p1.y} A ${r} ${r} 0 1 1 ${p2.x} ${p2.y} Z`
  }
  const start = polarToCartesian(cx, cy, r, endAngle)
  const end = polarToCartesian(cx, cy, r, startAngle)
  const large = endAngle - startAngle > 180 ? 1 : 0
  return `M ${cx} ${cy} L ${end.x} ${end.y} A ${r} ${r} 0 ${large} 1 ${start.x} ${start.y} Z`
}

const total = computed(() => props.items.reduce((a, it) => a + (it.durationSec || 0), 0))

/** Legend % matches slice angles (relative to node/contribution sum), not wall sharePct. */
function slicePct(durationSec: number): number | null {
  const t = total.value
  if (t <= 0) return null
  return Math.round((durationSec / t) * 100)
}

const slices = computed(() => {
  const t = total.value
  if (t <= 0) return [] as { key: string; d: string; color: string }[]
  let angle = 0
  const out: { key: string; d: string; color: string }[] = []
  for (const it of props.items) {
    const frac = (it.durationSec || 0) / t
    const sweep = frac * 360
    if (sweep <= 0) continue
    out.push({
      key: it.key,
      d: describeSlice(50, 50, 42, angle, angle + sweep),
      color: it.color,
    })
    angle += sweep
  }
  return out
})
</script>

<template>
  <div data-testid="stats-pie-query" class="stats-pie-query min-w-0 w-full max-w-full">
    <div data-testid="stats-pie-layout" class="stats-pie-layout flex min-w-0 flex-col items-center gap-4">
      <div data-testid="stats-pie-chart" class="relative mx-auto h-[140px] w-[140px] shrink-0">
      <svg data-testid="stats-pie-svg" viewBox="0 0 100 100" class="h-full w-full" role="img" :aria-label="centerSub">
        <circle cx="50" cy="50" r="34" class="fill-surface" />
        <template v-if="total > 0">
          <path
            v-for="s in slices"
            :key="s.key"
            :d="s.d"
            :fill="s.color"
            opacity="0.92"
          />
          <circle cx="50" cy="50" r="26" class="fill-surface" />
        </template>
        <circle
          v-else
          cx="50"
          cy="50"
          r="40"
          class="fill-elevated stroke-line"
          stroke-width="1"
        />
      </svg>
        <div data-testid="stats-pie-center" class="pointer-events-none absolute inset-0 flex flex-col items-center justify-center text-center">
          <span class="text-[15px] font-bold tabular-nums text-txt">{{ centerValue }}</span>
          <span class="mt-0.5 text-[10px] text-txt3">{{ centerSub }}</span>
        </div>
      </div>
      <div data-testid="stats-pie-legend" class="min-w-0 w-full flex-1 space-y-1.5">
        <div
          v-for="it in items"
          :key="it.key"
          class="flex min-w-0 items-center gap-2 text-[12px]"
        >
          <span class="h-2.5 w-2.5 shrink-0" :style="{ background: it.color }" />
          <TruncatedTextTooltip
            :text="it.label"
            class="min-w-0 flex-1 truncate text-txt2"
            data-testid="stats-pie-legend-label"
          />
          <span class="shrink-0 font-medium tabular-nums text-txt">
            {{ slicePct(it.durationSec) == null ? '—' : slicePct(it.durationSec) + '%' }}
          </span>
        </div>
        <p v-if="!items.length" class="text-[11px] text-txt3">{{ emptyLabel || '—' }}</p>
        <p v-if="footnote && items.length" class="pt-0.5 text-[10px] leading-snug text-txt3">{{ footnote }}</p>
      </div>
    </div>
  </div>
</template>

<style scoped>
.stats-pie-query {
  container-type: inline-size;
}

@container (min-width: 430px) {
  .stats-pie-layout {
    flex-direction: row;
    align-items: center;
  }
}
</style>
