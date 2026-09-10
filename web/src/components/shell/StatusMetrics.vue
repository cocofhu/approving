<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
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
const router = useRouter()
const { isMobile } = useBreakpoint()
const { metrics, stale } = usePlatformStatusMetrics()

const useCompact = computed(() => {
  if (props.variant === 'compact') return true
  if (props.variant === 'full') return false
  return isMobile.value
})

/** Sidebar/drawer compact tips Teleport above the trigger to escape overflow-hidden. */
const usePortaledCompactTip = computed(() => props.variant === 'compact')

const compactTrigger = ref<HTMLElement | null>(null)
const compactTip = ref<HTMLElement | null>(null)
const compactTipHovered = ref(false)
const compactTipFocused = ref(false)
/** Hide Teleport tip after click until pointer leaves (plan g1.1). */
const suppressCompactTip = ref(false)
const compactTipStyle = ref<FixedOverlayAboveStyle | null>(null)

const compactTipVisible = computed(
  () =>
    usePortaledCompactTip.value &&
    !suppressCompactTip.value &&
    (compactTipHovered.value || compactTipFocused.value),
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

const cumulative = computed(() => metrics.value?.cumulativeTokens ?? null)
const today = computed(() => metrics.value?.todayTokens ?? null)
const running = computed(() => metrics.value?.runningCount ?? 0)
const queued = computed(() => metrics.value?.queuedCount ?? 0)

function todayAria(): string {
  return `${t('shell.statusMetrics.today')}: ${fmtFull(today.value)} · ${t('shell.statusMetrics.openStats')}`
}

function statsAria(label: string): string {
  return `${label} · ${t('shell.statusMetrics.openStats')}`
}

function goToStats() {
  suppressCompactTip.value = true
  compactTipHovered.value = false
  compactTipFocused.value = false
  void router.push({ name: 'stats' })
}

function onActivateKey(ev: KeyboardEvent) {
  if (ev.key !== 'Enter' && ev.key !== ' ') return
  ev.preventDefault()
  goToStats()
}
</script>

<template>
  <div
    class="status-metrics flex select-none items-center font-mono text-txt2 tabular-nums"
    data-testid="status-metrics"
    :aria-label="t('shell.statusMetrics.aria')"
    :data-stale="stale ? 'true' : 'false'"
  >
    <!-- Desktop ≥md (or variant=full): four icon+value items -->
    <template v-if="!useCompact">
      <button
        type="button"
        class="sm-item relative inline-flex items-center gap-1.5 border-0 bg-transparent px-1.5 py-1 text-inherit hover:bg-elevated hover:text-txt focus-visible:bg-elevated focus-visible:text-txt focus-visible:outline-none"
        data-testid="status-metrics-tokens"
        :aria-label="statsAria(t('shell.statusMetrics.tokens'))"
        @click="goToStats"
        @keydown="onActivateKey"
      >
        <svg class="sm-ico block h-3.5 w-3.5 shrink-0 text-txt3" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
          <ellipse cx="12" cy="6.6" rx="7.2" ry="3.1" />
          <path d="M4.8 6.6v4.7c0 1.7 3.2 3.1 7.2 3.1s7.2-1.4 7.2-3.1V6.6" />
          <path d="M4.8 11.5v4.7c0 1.7 3.2 3.1 7.2 3.1s7.2-1.4 7.2-3.1v-4.7" />
        </svg>
        <span class="sm-val text-xs leading-none text-txt">{{ fmtCompactTokenCount(cumulative) }}</span>
        <span
          class="rounded-md sm-tip pointer-events-none absolute left-1/2 top-[calc(100%+6px)] z-40 hidden -translate-x-1/2 whitespace-nowrap border border-line-strong bg-overlay px-2.5 py-1.5 text-left font-sans text-xs leading-snug text-txt2 shadow-card"
          role="tooltip"
        >
          {{ t('shell.statusMetrics.tokens') }}: <span class="font-mono">{{ fmtFull(cumulative) }}</span>
        </span>
      </button>
      <span class="mx-0.5 h-3.5 w-px shrink-0 bg-line-strong" aria-hidden="true" />

      <button
        type="button"
        class="sm-item relative inline-flex items-center gap-1.5 border-0 bg-transparent px-1.5 py-1 text-inherit hover:bg-elevated hover:text-txt focus-visible:bg-elevated focus-visible:text-txt focus-visible:outline-none"
        data-testid="status-metrics-today"
        :aria-label="todayAria()"
        @click="goToStats"
        @keydown="onActivateKey"
      >
        <svg class="sm-ico block h-3.5 w-3.5 shrink-0 text-txt3" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
          <rect x="3.4" y="5.2" width="17.2" height="15.2" rx="2.6" />
          <path d="M8.2 3v4.2M15.8 3v4.2M3.4 10.2h17.2" />
          <circle cx="12" cy="15.2" r="2.6" />
        </svg>
        <span class="sm-val text-xs leading-none text-txt">{{ fmtCompactTokenCount(today) }}</span>
        <span
          class="rounded-md sm-tip pointer-events-none absolute left-1/2 top-[calc(100%+6px)] z-40 hidden -translate-x-1/2 whitespace-nowrap border border-line-strong bg-overlay px-2.5 py-1.5 text-left font-sans text-xs leading-snug text-txt2 shadow-card"
          role="tooltip"
        >
          {{ t('shell.statusMetrics.today') }}: <span class="font-mono">{{ fmtFull(today) }}</span>
        </span>
      </button>
      <span class="mx-0.5 h-3.5 w-px shrink-0 bg-line-strong" aria-hidden="true" />

      <button
        type="button"
        class="sm-item relative inline-flex items-center gap-1.5 border-0 bg-transparent px-1.5 py-1 text-inherit hover:bg-elevated hover:text-txt focus-visible:bg-elevated focus-visible:text-txt focus-visible:outline-none"
        data-testid="status-metrics-running"
        :aria-label="statsAria(t('shell.statusMetrics.running'))"
        @click="goToStats"
        @keydown="onActivateKey"
      >
        <svg class="sm-ico block h-3.5 w-3.5 shrink-0 text-txt3" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
          <circle cx="12" cy="12" r="8.2" />
          <path d="M10.3 8.7l5.4 3.3-5.4 3.3z" />
        </svg>
        <span class="sm-val text-xs leading-none text-txt">{{ running }}</span>
        <span
          class="rounded-md sm-tip pointer-events-none absolute left-1/2 top-[calc(100%+6px)] z-40 hidden -translate-x-1/2 whitespace-nowrap border border-line-strong bg-overlay px-2.5 py-1.5 text-left font-sans text-xs leading-snug text-txt2 shadow-card"
          role="tooltip"
        >
          {{ t('shell.statusMetrics.running') }}: <span class="font-mono">{{ running }}</span>
        </span>
      </button>
      <span class="mx-0.5 h-3.5 w-px shrink-0 bg-line-strong" aria-hidden="true" />

      <button
        type="button"
        class="sm-item relative inline-flex items-center gap-1.5 border-0 bg-transparent px-1.5 py-1 text-inherit hover:bg-elevated hover:text-txt focus-visible:bg-elevated focus-visible:text-txt focus-visible:outline-none"
        data-testid="status-metrics-queued"
        :aria-label="statsAria(t('shell.statusMetrics.queued'))"
        @click="goToStats"
        @keydown="onActivateKey"
      >
        <svg class="sm-ico block h-3.5 w-3.5 shrink-0 text-txt3" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
          <path d="M4 7.2h16M4 12h11.5M4 16.8h7" />
        </svg>
        <span class="sm-val text-xs leading-none text-txt">{{ queued }}</span>
        <span
          class="rounded-md sm-tip pointer-events-none absolute left-1/2 top-[calc(100%+6px)] z-40 hidden -translate-x-1/2 whitespace-nowrap border border-line-strong bg-overlay px-2.5 py-1.5 text-left font-sans text-xs leading-snug text-txt2 shadow-card"
          role="tooltip"
        >
          {{ t('shell.statusMetrics.queued') }}: <span class="font-mono">{{ queued }}</span>
        </span>
      </button>
    </template>

    <!-- Narrow &lt;md: Token · RUN/Q; today only in tip -->
    <button
      v-else
      ref="compactTrigger"
      type="button"
      class="sm-item sm-compact relative inline-flex w-full items-center gap-2 rounded-md border-0 bg-elevated px-2 py-1.5 text-[11px] text-inherit hover:bg-elevated hover:text-txt focus-visible:bg-elevated focus-visible:text-txt focus-visible:outline-none"
      :class="suppressCompactTip ? 'tip-suppressed' : ''"
      data-testid="status-metrics-compact"
      :aria-label="t('shell.statusMetrics.compactAria')"
      @click="goToStats"
      @keydown="onActivateKey"
      @blur="compactTipFocused = false"
      @focus="compactTipFocused = true"
      @mouseenter="suppressCompactTip = false; compactTipHovered = true"
      @mouseleave="compactTipHovered = false; suppressCompactTip = false"
    >
      <span class="inline-flex items-center gap-1.5">
        <svg class="sm-ico block h-[13px] w-[13px] shrink-0 text-txt3" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
          <ellipse cx="12" cy="6.6" rx="7.2" ry="3.1" />
          <path d="M4.8 6.6v4.7c0 1.7 3.2 3.1 7.2 3.1s7.2-1.4 7.2-3.1V6.6" />
          <path d="M4.8 11.5v4.7c0 1.7 3.2 3.1 7.2 3.1s7.2-1.4 7.2-3.1v-4.7" />
        </svg>
        <span class="sm-val text-[11px] font-semibold leading-none text-txt">{{ fmtCompactTokenCount(cumulative) }}</span>
      </span>
      <span class="h-3 w-px shrink-0 bg-line-strong opacity-90" aria-hidden="true" />
      <span class="inline-flex items-center gap-1.5">
        <svg class="sm-ico block h-[13px] w-[13px] shrink-0 text-txt3" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
          <circle cx="12" cy="12" r="8.2" />
          <path d="M10.3 8.7l5.4 3.3-5.4 3.3z" />
        </svg>
        <span class="sm-val text-[11px] font-semibold leading-none text-txt">{{ running }}</span>
        <span class="text-txt3">/</span>
        <svg class="sm-ico block h-[13px] w-[13px] shrink-0 text-txt3" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
          <path d="M4 7.2h16M4 12h11.5M4 16.8h7" />
        </svg>
        <span class="sm-val text-[11px] font-semibold leading-none text-txt">{{ queued }}</span>
      </span>
      <span
        v-if="!usePortaledCompactTip"
        class="rounded-md sm-tip pointer-events-none absolute left-1/2 top-[calc(100%+6px)] z-40 hidden min-w-[180px] -translate-x-1/2 border border-line-strong bg-overlay px-2.5 py-2 text-left font-sans text-xs leading-snug text-txt2 shadow-card"
        role="tooltip"
      >
        <div>{{ t('shell.statusMetrics.tokens') }}: <span class="font-mono">{{ fmtFull(cumulative) }}</span></div>
        <div>{{ t('shell.statusMetrics.today') }}: <span class="font-mono">{{ fmtFull(today) }}</span></div>
        <div>{{ t('shell.statusMetrics.running') }}: <span class="font-mono">{{ running }}</span></div>
        <div>{{ t('shell.statusMetrics.queued') }}: <span class="font-mono">{{ queued }}</span></div>
      </span>
    </button>

    <Teleport v-if="usePortaledCompactTip" to="body">
      <div
        v-show="compactTipVisible"
        ref="compactTip"
        class="rounded-md sm-tip pointer-events-none z-[60] min-w-[180px] border border-line-strong bg-overlay px-2.5 py-2 text-left font-sans text-xs leading-snug text-txt2 shadow-card"
        role="tooltip"
        data-testid="status-metrics-compact-tip"
        data-placement="above"
        :style="compactTipStyle ?? undefined"
      >
        <div>{{ t('shell.statusMetrics.tokens') }}: <span class="font-mono">{{ fmtFull(cumulative) }}</span></div>
        <div>{{ t('shell.statusMetrics.today') }}: <span class="font-mono">{{ fmtFull(today) }}</span></div>
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
.sm-item.tip-suppressed .sm-tip,
.sm-item.tip-suppressed:hover .sm-tip,
.sm-item.tip-suppressed:focus-visible .sm-tip {
  display: none;
}
</style>
