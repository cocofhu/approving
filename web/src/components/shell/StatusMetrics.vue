<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useBreakpoint } from '@/lib/composables/useBreakpoint'
import { usePlatformStatusMetrics } from '@/lib/composables/usePlatformStatusMetrics'
import { fmtCompactTokenCount } from '@/lib/run/tokenUsage'

const { t } = useI18n()
const { isMobile } = useBreakpoint()
const { metrics, stale } = usePlatformStatusMetrics()

/** Which tip is pinned via click/touch (Demo tip-open). */
const tipOpen = ref<string | null>(null)

function fmtFull(n: number | null | undefined): string {
  if (n === null || n === undefined) return '—'
  return n.toLocaleString('en-US')
}

/** Bucket cumulative for StatusMetrics; not Run-level token/s. */
function fmtFiveMinuteRate(n: number | null | undefined): string {
  if (n === null || n === undefined) return '—'
  return `${fmtCompactTokenCount(n)}/5m`
}

function fmtBucketRange(start?: string | null, end?: string | null): string {
  if (!start || !end) return ''
  const a = new Date(start)
  const b = new Date(end)
  if (Number.isNaN(a.getTime()) || Number.isNaN(b.getTime())) return ''
  const p = (d: Date) =>
    `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`
  return `${p(a)}–${p(b)}`
}

const cumulative = computed(() => metrics.value?.cumulativeTokens ?? null)
const rate = computed(() => metrics.value?.current5mBucketTokens ?? null)
const peak = computed(() => metrics.value?.todayMaxCompleted5mTokens ?? null)
const running = computed(() => metrics.value?.runningCount ?? 0)
const queued = computed(() => metrics.value?.queuedCount ?? 0)

const rateBucketLabel = computed(() =>
  fmtBucketRange(metrics.value?.currentBucketStart, metrics.value?.currentBucketEnd),
)
const peakBucketLabel = computed(() =>
  fmtBucketRange(metrics.value?.peakBucketStart, metrics.value?.peakBucketEnd),
)

function toggleTip(id: string, ev: Event) {
  ev.preventDefault()
  tipOpen.value = tipOpen.value === id ? null : id
}

function onBlurTip(id: string) {
  // Delay so focus can move within the same control without flicker.
  requestAnimationFrame(() => {
    if (tipOpen.value === id) tipOpen.value = null
  })
}
</script>

<template>
  <div
    class="status-metrics flex select-none items-center font-mono text-txt2 tabular-nums"
    data-testid="status-metrics"
    :aria-label="t('shell.statusMetrics.aria')"
    :data-stale="stale ? 'true' : 'false'"
  >
    <!-- Desktop ≥md: five icon+value items -->
    <template v-if="!isMobile">
      <button
        type="button"
        class="sm-item relative inline-flex items-center gap-1.5 border-0 bg-transparent px-1.5 py-1 text-inherit hover:bg-elevated hover:text-txt focus-visible:bg-elevated focus-visible:text-txt focus-visible:outline-none"
        :class="tipOpen === 'tokens' ? 'bg-elevated text-txt tip-open' : ''"
        data-testid="status-metrics-tokens"
        :aria-label="t('shell.statusMetrics.tokens')"
        @click="toggleTip('tokens', $event)"
        @blur="onBlurTip('tokens')"
      >
        <svg class="sm-ico block h-3.5 w-3.5 shrink-0 text-txt3" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" aria-hidden="true">
          <circle cx="12" cy="12" r="8" />
          <path d="M12 8v8M9.5 10.2c.6-.7 1.5-1.1 2.5-1.1 1.7 0 3 1 3 2.4s-1.3 2.4-3 2.4h-1.2c-1.7 0-3 1-3 2.4 0 1.4 1.4 2.3 3.2 2.3 1.1 0 2-.4 2.6-1.1" />
        </svg>
        <span class="sm-val text-xs leading-none text-txt">{{ fmtCompactTokenCount(cumulative) }}</span>
        <span
          class="sm-tip pointer-events-none absolute left-1/2 top-[calc(100%+6px)] z-40 hidden min-w-[210px] max-w-[290px] -translate-x-1/2 border border-line-strong bg-overlay px-2.5 py-2 text-left font-sans text-xs leading-snug text-txt2 shadow-card"
          role="tooltip"
        >
          <strong class="mb-1 block font-semibold text-txt">{{ t('shell.statusMetrics.tokens') }}</strong>
          {{ t('shell.statusMetrics.tokensTip') }}
          {{ t('shell.statusMetrics.fullValue') }}
          <span class="font-mono">{{ fmtFull(cumulative) }}</span>
        </span>
      </button>
      <span class="mx-0.5 h-3.5 w-px shrink-0 bg-line-strong" aria-hidden="true" />

      <button
        type="button"
        class="sm-item relative inline-flex items-center gap-1.5 border-0 bg-transparent px-1.5 py-1 text-inherit hover:bg-elevated hover:text-txt focus-visible:bg-elevated focus-visible:text-txt focus-visible:outline-none"
        :class="tipOpen === 'rate' ? 'bg-elevated text-txt tip-open' : ''"
        data-testid="status-metrics-rate"
        :aria-label="t('shell.statusMetrics.rate')"
        @click="toggleTip('rate', $event)"
        @blur="onBlurTip('rate')"
      >
        <svg class="sm-ico block h-3.5 w-3.5 shrink-0 text-txt3" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" aria-hidden="true">
          <path d="M13 3L5 14h7l-1 7 8-11h-7l1-7z" />
        </svg>
        <span class="sm-val text-xs leading-none text-txt">{{ fmtFiveMinuteRate(rate) }}</span>
        <span
          class="sm-tip pointer-events-none absolute left-1/2 top-[calc(100%+6px)] z-40 hidden min-w-[210px] max-w-[290px] -translate-x-1/2 border border-line-strong bg-overlay px-2.5 py-2 text-left font-sans text-xs leading-snug text-txt2 shadow-card"
          role="tooltip"
        >
          <strong class="mb-1 block font-semibold text-txt">{{ t('shell.statusMetrics.rate') }}</strong>
          {{ t('shell.statusMetrics.rateTip') }}
          {{ t('shell.statusMetrics.fullValue') }}
          <span class="font-mono">{{ fmtFull(rate) }}</span>
          <template v-if="rateBucketLabel"> · {{ t('shell.statusMetrics.bucket') }} {{ rateBucketLabel }}</template>
        </span>
      </button>
      <span class="mx-0.5 h-3.5 w-px shrink-0 bg-line-strong" aria-hidden="true" />

      <button
        type="button"
        class="sm-item relative inline-flex items-center gap-1.5 border-0 bg-transparent px-1.5 py-1 text-inherit hover:bg-elevated hover:text-txt focus-visible:bg-elevated focus-visible:text-txt focus-visible:outline-none"
        :class="tipOpen === 'peak' ? 'bg-elevated text-txt tip-open' : ''"
        data-testid="status-metrics-peak"
        :aria-label="t('shell.statusMetrics.peak')"
        @click="toggleTip('peak', $event)"
        @blur="onBlurTip('peak')"
      >
        <svg class="sm-ico block h-3.5 w-3.5 shrink-0 text-txt3" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" aria-hidden="true">
          <path d="M3 18h18" />
          <path d="M5 18V13l4-4 3 3 5-6 2 2v10" />
        </svg>
        <span class="sm-val text-xs leading-none text-txt">{{ fmtCompactTokenCount(peak) }}</span>
        <span
          class="sm-tip pointer-events-none absolute left-1/2 top-[calc(100%+6px)] z-40 hidden min-w-[210px] max-w-[290px] -translate-x-1/2 border border-line-strong bg-overlay px-2.5 py-2 text-left font-sans text-xs leading-snug text-txt2 shadow-card"
          role="tooltip"
        >
          <strong class="mb-1 block font-semibold text-txt">{{ t('shell.statusMetrics.peak') }}</strong>
          {{ t('shell.statusMetrics.peakTip') }}
          {{ t('shell.statusMetrics.fullValue') }}
          <span class="font-mono">{{ fmtFull(peak) }}</span>
          <template v-if="peakBucketLabel"> · {{ peakBucketLabel }}</template>
        </span>
      </button>
      <span class="mx-0.5 h-3.5 w-px shrink-0 bg-line-strong" aria-hidden="true" />

      <button
        type="button"
        class="sm-item relative inline-flex items-center gap-1.5 border-0 bg-transparent px-1.5 py-1 text-inherit hover:bg-elevated hover:text-txt focus-visible:bg-elevated focus-visible:text-txt focus-visible:outline-none"
        :class="tipOpen === 'running' ? 'bg-elevated text-txt tip-open' : ''"
        data-testid="status-metrics-running"
        :aria-label="t('shell.statusMetrics.running')"
        @click="toggleTip('running', $event)"
        @blur="onBlurTip('running')"
      >
        <svg class="sm-ico block h-3.5 w-3.5 shrink-0 text-txt3" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" aria-hidden="true">
          <circle cx="12" cy="12" r="8" />
          <path d="M10 9.2l5.2 2.8L10 14.8V9.2z" fill="currentColor" stroke="none" />
        </svg>
        <span class="sm-val text-xs leading-none text-txt">{{ running }}</span>
        <span
          class="sm-tip pointer-events-none absolute left-1/2 top-[calc(100%+6px)] z-40 hidden min-w-[210px] max-w-[290px] -translate-x-1/2 border border-line-strong bg-overlay px-2.5 py-2 text-left font-sans text-xs leading-snug text-txt2 shadow-card"
          role="tooltip"
        >
          <strong class="mb-1 block font-semibold text-txt">{{ t('shell.statusMetrics.running') }}</strong>
          {{ t('shell.statusMetrics.runningTip') }}
        </span>
      </button>
      <span class="mx-0.5 h-3.5 w-px shrink-0 bg-line-strong" aria-hidden="true" />

      <button
        type="button"
        class="sm-item relative inline-flex items-center gap-1.5 border-0 bg-transparent px-1.5 py-1 text-inherit hover:bg-elevated hover:text-txt focus-visible:bg-elevated focus-visible:text-txt focus-visible:outline-none"
        :class="tipOpen === 'queued' ? 'bg-elevated text-txt tip-open' : ''"
        data-testid="status-metrics-queued"
        :aria-label="t('shell.statusMetrics.queued')"
        @click="toggleTip('queued', $event)"
        @blur="onBlurTip('queued')"
      >
        <svg class="sm-ico block h-3.5 w-3.5 shrink-0 text-txt3" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" aria-hidden="true">
          <path d="M5 7h14M5 12h14M5 17h10" />
        </svg>
        <span class="sm-val text-xs leading-none text-txt">{{ queued }}</span>
        <span
          class="sm-tip pointer-events-none absolute left-1/2 top-[calc(100%+6px)] z-40 hidden min-w-[210px] max-w-[290px] -translate-x-1/2 border border-line-strong bg-overlay px-2.5 py-2 text-left font-sans text-xs leading-snug text-txt2 shadow-card"
          role="tooltip"
        >
          <strong class="mb-1 block font-semibold text-txt">{{ t('shell.statusMetrics.queued') }}</strong>
          {{ t('shell.statusMetrics.queuedTip') }}
        </span>
      </button>
    </template>

    <!-- Narrow &lt;md: Token · RUN/Q; rate/peak only in tip -->
    <button
      v-else
      type="button"
      class="sm-item relative inline-flex items-center gap-1 border-0 bg-transparent px-1.5 py-1 text-inherit hover:bg-elevated hover:text-txt focus-visible:bg-elevated focus-visible:text-txt focus-visible:outline-none"
      :class="tipOpen === 'compact' ? 'bg-elevated text-txt tip-open' : ''"
      data-testid="status-metrics-compact"
      :aria-label="t('shell.statusMetrics.compactAria')"
      @click="toggleTip('compact', $event)"
      @blur="onBlurTip('compact')"
    >
      <span class="inline-flex items-center gap-1">
        <svg class="sm-ico block h-3.5 w-3.5 shrink-0 text-txt3" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" aria-hidden="true">
          <circle cx="12" cy="12" r="8" />
          <path d="M12 8v8M9.5 10.2c.6-.7 1.5-1.1 2.5-1.1 1.7 0 3 1 3 2.4s-1.3 2.4-3 2.4h-1.2c-1.7 0-3 1-3 2.4 0 1.4 1.4 2.3 3.2 2.3 1.1 0 2-.4 2.6-1.1" />
        </svg>
        <span class="sm-val text-xs leading-none text-txt">{{ fmtCompactTokenCount(cumulative) }}</span>
      </span>
      <span class="mx-1.5 h-3.5 w-px shrink-0 bg-line-strong" aria-hidden="true" />
      <span class="inline-flex items-center gap-1">
        <svg class="sm-ico block h-3.5 w-3.5 shrink-0 text-txt3" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" aria-hidden="true">
          <circle cx="12" cy="12" r="8" />
          <path d="M10 9.2l5.2 2.8L10 14.8V9.2z" fill="currentColor" stroke="none" />
        </svg>
        <span class="sm-val text-xs leading-none text-txt">{{ running }}</span>
        <span class="text-txt3">/</span>
        <svg class="sm-ico block h-3.5 w-3.5 shrink-0 text-txt3" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" aria-hidden="true">
          <path d="M5 7h14M5 12h14M5 17h10" />
        </svg>
        <span class="sm-val text-xs leading-none text-txt">{{ queued }}</span>
      </span>
      <span
        class="sm-tip pointer-events-none absolute left-1/2 top-[calc(100%+6px)] z-40 hidden min-w-[210px] max-w-[290px] -translate-x-1/2 border border-line-strong bg-overlay px-2.5 py-2 text-left font-sans text-xs leading-snug text-txt2 shadow-card"
        role="tooltip"
      >
        <strong class="mb-1 block font-semibold text-txt">{{ t('shell.statusMetrics.compactTitle') }}</strong>
        {{ t('shell.statusMetrics.compactTip') }}
        <br />
        {{ t('shell.statusMetrics.rate') }}
        <span class="font-mono">{{ fmtFiveMinuteRate(rate) }}</span>
        · {{ t('shell.statusMetrics.peak') }}
        <span class="font-mono">{{ fmtCompactTokenCount(peak) }}</span>
      </span>
    </button>
  </div>
</template>

<style scoped>
.sm-item:hover .sm-ico,
.sm-item:focus-visible .sm-ico,
.sm-item.tip-open .sm-ico {
  color: rgb(var(--c-txt2));
}
.sm-item:hover .sm-tip,
.sm-item:focus-visible .sm-tip,
.sm-item.tip-open .sm-tip {
  display: block;
}
</style>
