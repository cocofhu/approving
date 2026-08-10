<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import type { TokenStatsComposition } from '@/lib/types'
import { fmtCompactTokenCount, fmtTokenCount } from '@/lib/tokenUsage'
import { TOKEN_PART_COLORS, TOKEN_PART_KEYS, type TokenPartKey } from './tokenStatsShared'

const props = defineProps<{
  composition: TokenStatsComposition
}>()

const { t } = useI18n()
const active = ref<TokenPartKey | null>(null)

/** Reuse executionTimeline part* as the authoritative UI labels (g2.1/g2.2). */
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

const visibleParts = computed(() => parts.value.filter((p) => p.value > 0))

function polar(cx: number, cy: number, r: number, angleDeg: number) {
  const rad = ((angleDeg - 90) * Math.PI) / 180
  return { x: cx + r * Math.cos(rad), y: cy + r * Math.sin(rad) }
}

function arcPath(cx: number, cy: number, rOuter: number, rInner: number, start: number, end: number): string {
  if (end - start >= 359.99) {
    const mid = start + 180
    const o0 = polar(cx, cy, rOuter, start)
    const o1 = polar(cx, cy, rOuter, mid)
    const o2 = polar(cx, cy, rOuter, end)
    const i0 = polar(cx, cy, rInner, end)
    const i1 = polar(cx, cy, rInner, mid)
    const i2 = polar(cx, cy, rInner, start)
    return [
      `M ${o0.x} ${o0.y}`,
      `A ${rOuter} ${rOuter} 0 1 1 ${o1.x} ${o1.y}`,
      `A ${rOuter} ${rOuter} 0 1 1 ${o2.x} ${o2.y}`,
      `L ${i0.x} ${i0.y}`,
      `A ${rInner} ${rInner} 0 1 0 ${i1.x} ${i1.y}`,
      `A ${rInner} ${rInner} 0 1 0 ${i2.x} ${i2.y}`,
      'Z',
    ].join(' ')
  }
  const large = end - start > 180 ? 1 : 0
  const oStart = polar(cx, cy, rOuter, start)
  const oEnd = polar(cx, cy, rOuter, end)
  const iEnd = polar(cx, cy, rInner, end)
  const iStart = polar(cx, cy, rInner, start)
  return [
    `M ${oStart.x} ${oStart.y}`,
    `A ${rOuter} ${rOuter} 0 ${large} 1 ${oEnd.x} ${oEnd.y}`,
    `L ${iEnd.x} ${iEnd.y}`,
    `A ${rInner} ${rInner} 0 ${large} 0 ${iStart.x} ${iStart.y}`,
    'Z',
  ].join(' ')
}

const slices = computed(() => {
  const list = visibleParts.value
  const total = list.reduce((s, p) => s + p.value, 0)
  if (total <= 0) return [] as { key: TokenPartKey; d: string; color: string }[]
  let angle = 0
  const out: { key: TokenPartKey; d: string; color: string }[] = []
  for (const p of list) {
    const sweep = (p.value / total) * 360
    if (sweep <= 0) continue
    out.push({
      key: p.key,
      d: arcPath(60, 60, 52, 32, angle, angle + sweep),
      color: p.color,
    })
    angle += sweep
  }
  return out
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
    <svg
      data-testid="token-donut-svg"
      class="h-[120px] w-[120px] shrink-0 sm:h-[150px] sm:w-[150px]"
      viewBox="0 0 120 120"
      role="img"
      :aria-label="t('pages.board.tokenStats.compositionTitle')"
    >
      <path
        v-for="s in slices"
        :key="s.key"
        :d="s.d"
        :fill="s.color"
        class="cursor-pointer transition-opacity"
        :opacity="active && active !== s.key ? 0.45 : 0.95"
        @mouseenter="activate(s.key, $event)"
        @mousemove="activate(s.key, $event)"
        @mouseleave="deactivate"
        @click="activate(s.key, $event)"
      />
      <text x="60" y="56" text-anchor="middle" class="fill-txt3" font-size="11">
        {{ t('pages.board.tokenStats.donutCenter') }}
      </text>
      <text x="60" y="74" text-anchor="middle" class="fill-txt" font-size="15" font-weight="700">
        {{ fmtCompactTokenCount(composition.total) }}
      </text>
    </svg>
    <ul data-testid="token-donut-legend" class="m-0 grid w-full min-w-0 list-none gap-2 p-0 sm:flex-1">
      <li
        v-for="p in parts"
        :key="p.key"
        class="grid cursor-pointer grid-cols-[10px_1fr_auto] items-center gap-2 rounded-md px-1.5 py-1 text-xs"
        :class="active === p.key ? 'bg-elevated' : 'hover:bg-elevated/80'"
        @mouseenter="activate(p.key)"
        @mouseleave="deactivate"
        @click="activate(p.key)"
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
