<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import type { TokenStatsBucket } from '@/lib/types'
import { fmtTokenCount } from '@/lib/tokenUsage'
import { TOKEN_PART_COLORS, TOKEN_PART_KEYS, formatBucketLabel } from './tokenStatsShared'

const props = defineProps<{
  trend: TokenStatsBucket[]
  bucketWidth: string
}>()

const { t } = useI18n()

const W = 640
const H = 200
const PAD = { t: 16, r: 12, b: 28, l: 44 }

const activeIdx = ref<number | null>(null)
const tip = ref({ show: false, x: 0, y: 0, idx: -1 })

const maxTotal = computed(() => {
  const m = Math.max(0, ...props.trend.map((b) => b.total || 0))
  return m > 0 ? m : 1
})

const points = computed(() => {
  const n = props.trend.length
  if (n === 0) return [] as { x: number; y: number; i: number }[]
  const innerW = W - PAD.l - PAD.r
  const innerH = H - PAD.t - PAD.b
  return props.trend.map((b, i) => {
    const x = PAD.l + (n === 1 ? innerW / 2 : (i / (n - 1)) * innerW)
    const y = PAD.t + innerH * (1 - (b.total || 0) / maxTotal.value)
    return { x, y, i }
  })
})

const linePath = computed(() => {
  if (points.value.length === 0) return ''
  return points.value.map((p, i) => `${i === 0 ? 'M' : 'L'} ${p.x.toFixed(1)} ${p.y.toFixed(1)}`).join(' ')
})

const areaPath = computed(() => {
  if (points.value.length === 0) return ''
  const first = points.value[0]!
  const last = points.value[points.value.length - 1]!
  const baseY = H - PAD.b
  return `${linePath.value} L ${last.x.toFixed(1)} ${baseY} L ${first.x.toFixed(1)} ${baseY} Z`
})

const xLabels = computed(() => {
  const n = props.trend.length
  if (n === 0) return [] as { x: number; label: string }[]
  const step = n <= 8 ? 1 : n <= 16 ? 2 : Math.ceil(n / 8)
  const out: { x: number; label: string }[] = []
  for (let i = 0; i < n; i += step) {
    const p = points.value[i]
    if (!p) continue
    out.push({
      x: p.x,
      label: formatBucketLabel(props.trend[i]!.bucket, props.bucketWidth),
    })
  }
  return out
})

const yTicks = computed(() => {
  const innerH = H - PAD.t - PAD.b
  const ticks = [0, 0.5, 1]
  return ticks.map((f) => ({
    y: PAD.t + innerH * (1 - f),
    label: fmtCompact(Math.round(maxTotal.value * f)),
  }))
})

function fmtCompact(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`
  if (n >= 1000) return `${(n / 1000).toFixed(1)}K`
  return String(n)
}

function showTip(i: number, ev: MouseEvent) {
  activeIdx.value = i
  const wrap = (ev.currentTarget as SVGElement).closest('[data-testid="token-trend-wrap"]') as HTMLElement | null
  if (!wrap) return
  const rect = wrap.getBoundingClientRect()
  tip.value = {
    show: true,
    x: Math.min(Math.max(ev.clientX - rect.left, 8), rect.width - 160),
    y: Math.max(ev.clientY - rect.top - 8, 8),
    idx: i,
  }
}

function hideTip() {
  activeIdx.value = null
  tip.value = { ...tip.value, show: false }
}

const tipBucket = computed(() => (tip.value.idx >= 0 ? props.trend[tip.value.idx] : null))
</script>

<template>
  <div data-testid="token-trend-wrap" class="token-trend-wrap relative h-[200px] w-full">
    <svg
      data-testid="token-trend-svg"
      viewBox="0 0 640 200"
      preserveAspectRatio="none"
      class="h-full w-full"
      role="img"
      :aria-label="t('pages.board.tokenStats.trendTitle')"
    >
      <defs>
        <linearGradient id="tokenTrendArea" x1="0" y1="0" x2="0" y2="1">
          <stop offset="0%" stop-color="#6d5cff" stop-opacity="0.22" />
          <stop offset="100%" stop-color="#6d5cff" stop-opacity="0.02" />
        </linearGradient>
      </defs>
      <line
        v-for="(tk, i) in yTicks"
        :key="'g' + i"
        :x1="PAD.l"
        :x2="W - PAD.r"
        :y1="tk.y"
        :y2="tk.y"
        stroke="#eef0f3"
        stroke-width="1"
      />
      <text
        v-for="(tk, i) in yTicks"
        :key="'yl' + i"
        :x="PAD.l - 6"
        :y="tk.y + 3"
        text-anchor="end"
        fill="#9aa1ad"
        font-size="10"
      >
        {{ tk.label }}
      </text>
      <path v-if="areaPath" :d="areaPath" fill="url(#tokenTrendArea)" />
      <path
        v-if="linePath"
        :d="linePath"
        fill="none"
        stroke="#6d5cff"
        stroke-width="2.2"
        stroke-linecap="round"
        stroke-linejoin="round"
      />
      <g v-for="p in points" :key="p.i">
        <circle
          :cx="p.x"
          :cy="p.y"
          :r="activeIdx === p.i ? 5 : 3.5"
          fill="#fff"
          stroke="#6d5cff"
          stroke-width="2"
          class="cursor-pointer"
          @mouseenter="showTip(p.i, $event)"
          @mousemove="showTip(p.i, $event)"
          @mouseleave="hideTip"
          @click="showTip(p.i, $event)"
        />
      </g>
      <text
        v-for="(xl, i) in xLabels"
        :key="'xl' + i"
        :x="xl.x"
        :y="H - 8"
        text-anchor="middle"
        fill="#9aa1ad"
        font-size="10"
      >
        {{ xl.label }}
      </text>
    </svg>
    <div
      v-if="tip.show && tipBucket"
      data-testid="token-trend-tooltip"
      class="pointer-events-none absolute z-10 min-w-[140px] rounded-lg bg-[#1a1d23] px-2.5 py-2 text-[11px] text-white shadow-lg"
      :style="{ left: tip.x + 'px', top: tip.y + 'px', transform: 'translate(-50%, -100%)' }"
    >
      <div class="mb-1 font-semibold">
        {{ formatBucketLabel(tipBucket.bucket, bucketWidth) }}
        · {{ fmtTokenCount(tipBucket.total) }}
      </div>
      <div
        v-for="key in TOKEN_PART_KEYS"
        :key="key"
        class="flex justify-between gap-3 text-[#c7cbd4]"
      >
        <span class="inline-flex items-center gap-1.5">
          <i class="inline-block h-2 w-2 rounded-sm" :style="{ background: TOKEN_PART_COLORS[key] }" />
          {{ key }}
        </span>
        <b class="font-normal tabular-nums text-white">{{
          fmtTokenCount(
            key === 'input'
              ? tipBucket.inputTokens
              : key === 'output'
                ? tipBucket.outputTokens
                : key === 'cacheRead'
                  ? tipBucket.cacheReadTokens
                  : tipBucket.cacheWriteTokens,
          )
        }}</b>
      </div>
    </div>
  </div>
</template>
