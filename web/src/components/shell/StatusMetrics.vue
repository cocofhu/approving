<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useBreakpoint } from '@/lib/composables/useBreakpoint'
import {
  placeFixedOverlayAbove,
  useFixedOverlayAboveListeners,
  type FixedOverlayAboveStyle,
} from '@/lib/composables/useFixedOverlayAbove'
import { usePlatformStatusMetrics } from '@/lib/composables/usePlatformStatusMetrics'
import { fmtCompactTokenCount } from '@/lib/run/tokenUsage'

const props = withDefaults(
  defineProps<{
    /** auto = breakpoint; compact forces narrow strip (sidebar/drawer). */
    variant?: 'auto' | 'full' | 'compact'
  }>(),
  { variant: 'auto' },
)

const { t } = useI18n()
const { isMobile } = useBreakpoint()
const { metrics, stale } = usePlatformStatusMetrics()

const useCompact = computed(() => {
  if (props.variant === 'compact') return true
  if (props.variant === 'full') return false
  return isMobile.value
})

/** Sidebar/drawer compact tips Teleport above the trigger to escape overflow-hidden. */
const usePortaledCompactTip = computed(() => props.variant === 'compact')

/** Which tip is pinned via click/touch (Demo tip-open). */
const tipOpen = ref<string | null>(null)
const compactTrigger = ref<HTMLElement | null>(null)
const compactTip = ref<HTMLElement | null>(null)
const compactTipHovered = ref(false)
const compactTipFocused = ref(false)
const compactTipStyle = ref<FixedOverlayAboveStyle | null>(null)

const compactTipVisible = computed(
  () =>
  usePortaledCompactTip.value &&
    (tipOpen.value === 'compact' || compactTipHovered.value || compactTipFocused.value),
)

async function repositionCompactTip() {
  if (!compactTipVisible.value) return
  await nextTick()
  compactTipStyle.value = await placeFixedOverlayAbove(compactTrigger.value, compactTip.value, {
    align: 'center',
    gap: 8,
  })
}

const { start: startCompactTipListeners, stop: stopCompactTipListeners } =
  useFixedOverlayAboveListeners(compactTipVisible, repositionCompactTip)

watch(compactTipVisible, async (visible) => {
  if (visible) {
    startCompactTipListeners()
    await repositionCompactTip()
  } else {
    stopCompactTipListeners()
    compactTipStyle.value = null
  }
})

function fmtFull(n: number | null | undefined): string {
  if (n === null || n === undefined) return '—'
  return n.toLocaleString('en-US')
}

/** Bucket cumulative for StatusMetrics; not Run-level token/s. */
function fmtFiveMinuteRate(n: number | null | undefined): string {
  if (n === null || n === undefined) return '—'
  return `${fmtCompactTokenCount(n)}/5m`
}

const cumulative = computed(() => metrics.value?.cumulativeTokens ?? null)
const rate = computed(() => metrics.value?.current5mBucketTokens ?? null)
const peak = computed(() => metrics.value?.todayMaxCompleted5mTokens ?? null)
const running = computed(() => metrics.value?.runningCount ?? 0)
const queued = computed(() => metrics.value?.queuedCount ?? 0)

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
    <!-- Desktop ≥md (or variant=full): five icon+value items -->
    <template v-if="!useCompact">
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
          class="sm-tip pointer-events-none absolute left-1/2 top-[calc(100%+6px)] z-40 hidden -translate-x-1/2 whitespace-nowrap border border-line-strong bg-overlay px-2.5 py-1.5 text-left font-sans text-xs leading-snug text-txt2 shadow-card"
          role="tooltip"
        >
          {{ t('shell.statusMetrics.tokens') }}: <span class="font-mono">{{ fmtFull(cumulative) }}</span>
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
          class="sm-tip pointer-events-none absolute left-1/2 top-[calc(100%+6px)] z-40 hidden -translate-x-1/2 whitespace-nowrap border border-line-strong bg-overlay px-2.5 py-1.5 text-left font-sans text-xs leading-snug text-txt2 shadow-card"
          role="tooltip"
        >
          {{ t('shell.statusMetrics.rate') }}: <span class="font-mono">{{ fmtFull(rate) }}</span>
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
          class="sm-tip pointer-events-none absolute left-1/2 top-[calc(100%+6px)] z-40 hidden -translate-x-1/2 whitespace-nowrap border border-line-strong bg-overlay px-2.5 py-1.5 text-left font-sans text-xs leading-snug text-txt2 shadow-card"
          role="tooltip"
        >
          {{ t('shell.statusMetrics.peak') }}: <span class="font-mono">{{ fmtFull(peak) }}</span>
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
          class="sm-tip pointer-events-none absolute left-1/2 top-[calc(100%+6px)] z-40 hidden -translate-x-1/2 whitespace-nowrap border border-line-strong bg-overlay px-2.5 py-1.5 text-left font-sans text-xs leading-snug text-txt2 shadow-card"
          role="tooltip"
        >
          {{ t('shell.statusMetrics.running') }}: <span class="font-mono">{{ running }}</span>
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
          class="sm-tip pointer-events-none absolute left-1/2 top-[calc(100%+6px)] z-40 hidden -translate-x-1/2 whitespace-nowrap border border-line-strong bg-overlay px-2.5 py-1.5 text-left font-sans text-xs leading-snug text-txt2 shadow-card"
          role="tooltip"
        >
          {{ t('shell.statusMetrics.queued') }}: <span class="font-mono">{{ queued }}</span>
        </span>
      </button>
    </template>

    <!-- Narrow &lt;md: Token · RUN/Q; rate/peak only in tip -->
    <button
      v-else
      ref="compactTrigger"
      type="button"
      class="sm-item sm-compact relative inline-flex w-full items-center gap-2 border-0 bg-elevated px-2 py-1.5 text-[11px] text-inherit hover:bg-elevated hover:text-txt focus-visible:bg-elevated focus-visible:text-txt focus-visible:outline-none"
      :class="tipOpen === 'compact' ? 'text-txt tip-open' : ''"
      data-testid="status-metrics-compact"
      :aria-label="t('shell.statusMetrics.compactAria')"
      @click="toggleTip('compact', $event)"
      @blur="onBlurTip('compact'); compactTipFocused = false"
      @focus="compactTipFocused = true"
      @mouseenter="compactTipHovered = true"
      @mouseleave="compactTipHovered = false"
    >
      <span class="inline-flex items-center gap-1.5">
        <svg class="sm-ico block h-[13px] w-[13px] shrink-0 text-txt3" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" aria-hidden="true">
          <circle cx="12" cy="12" r="8" />
          <path d="M12 8v8M9.5 10.2c.6-.7 1.5-1.1 2.5-1.1 1.7 0 3 1 3 2.4s-1.3 2.4-3 2.4h-1.2c-1.7 0-3 1-3 2.4 0 1.4 1.4 2.3 3.2 2.3 1.1 0 2-.4 2.6-1.1" />
        </svg>
        <span class="sm-val text-[11px] font-semibold leading-none text-txt">{{ fmtCompactTokenCount(cumulative) }}</span>
      </span>
      <span class="h-3 w-px shrink-0 bg-line-strong opacity-90" aria-hidden="true" />
      <span class="inline-flex items-center gap-1.5">
        <svg class="sm-ico block h-[13px] w-[13px] shrink-0 text-txt3" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" aria-hidden="true">
          <circle cx="12" cy="12" r="8" />
          <path d="M10 9.2l5.2 2.8L10 14.8V9.2z" fill="currentColor" stroke="none" />
        </svg>
        <span class="sm-val text-[11px] font-semibold leading-none text-txt">{{ running }}</span>
        <span class="text-txt3">/</span>
        <svg class="sm-ico block h-[13px] w-[13px] shrink-0 text-txt3" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" aria-hidden="true">
          <path d="M5 7h14M5 12h14M5 17h10" />
        </svg>
        <span class="sm-val text-[11px] font-semibold leading-none text-txt">{{ queued }}</span>
      </span>
      <span
        v-if="!usePortaledCompactTip"
        class="sm-tip pointer-events-none absolute left-1/2 top-[calc(100%+6px)] z-40 hidden min-w-[180px] -translate-x-1/2 border border-line-strong bg-overlay px-2.5 py-2 text-left font-sans text-xs leading-snug text-txt2 shadow-card"
        role="tooltip"
      >
        <div>{{ t('shell.statusMetrics.tokens') }}: <span class="font-mono">{{ fmtFull(cumulative) }}</span></div>
        <div>{{ t('shell.statusMetrics.rate') }}: <span class="font-mono">{{ fmtFull(rate) }}</span></div>
        <div>{{ t('shell.statusMetrics.peak') }}: <span class="font-mono">{{ fmtFull(peak) }}</span></div>
        <div>{{ t('shell.statusMetrics.running') }}: <span class="font-mono">{{ running }}</span></div>
        <div>{{ t('shell.statusMetrics.queued') }}: <span class="font-mono">{{ queued }}</span></div>
      </span>
    </button>

    <Teleport v-if="usePortaledCompactTip" to="body">
      <div
        v-show="compactTipVisible"
        ref="compactTip"
        class="sm-tip pointer-events-none z-[60] min-w-[180px] border border-line-strong bg-overlay px-2.5 py-2 text-left font-sans text-xs leading-snug text-txt2 shadow-card"
        role="tooltip"
        data-testid="status-metrics-compact-tip"
        data-placement="above"
        :style="compactTipStyle ?? undefined"
      >
        <div>{{ t('shell.statusMetrics.tokens') }}: <span class="font-mono">{{ fmtFull(cumulative) }}</span></div>
        <div>{{ t('shell.statusMetrics.rate') }}: <span class="font-mono">{{ fmtFull(rate) }}</span></div>
        <div>{{ t('shell.statusMetrics.peak') }}: <span class="font-mono">{{ fmtFull(peak) }}</span></div>
        <div>{{ t('shell.statusMetrics.running') }}: <span class="font-mono">{{ running }}</span></div>
        <div>{{ t('shell.statusMetrics.queued') }}: <span class="font-mono">{{ queued }}</span></div>
      </div>
    </Teleport>
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
