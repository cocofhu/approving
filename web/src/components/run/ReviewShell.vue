<script setup lang="ts">
import { ref, onMounted, onBeforeUnmount, watch } from 'vue'
import { useI18n } from 'vue-i18n'

/**
 * Shared review layout: annotatable product stage + review sidebar.
 * Desktop: horizontal (stage flex-1 | sash | sidebar); sidebar width is
 * internal state (default via sidebarWidth; Run Detail review passes 300),
 * draggable via sash, clamped to [240, min(480, shell−stageMin−sash)],
 * optionally persisted per storageKey.
 * Narrow: vertical with a bottom drawer (draggable height); no horizontal sash.
 */

const SIDEBAR_MIN = 240
const SIDEBAR_MAX = 480
const STAGE_MIN = 160
const SASH_WIDTH = 4
const DRAG_THRESHOLD_PX = 3

const props = withDefaults(
  defineProps<{
    /** Narrow / mobile: stage on top, sidebar as bottom drawer. */
    mobile?: boolean
    /** Desktop sidebar default width in px (also used for double-click reset). */
    sidebarWidth?: number
    /** Initial drawer height on mobile. */
    drawerHeight?: number
    /** localStorage key for scene-isolated width persistence. */
    storageKey?: string
  }>(),
  {
    mobile: false,
    sidebarWidth: 400,
    drawerHeight: 280,
    storageKey: '',
  },
)

const { t } = useI18n()

const shellRef = ref<HTMLElement | null>(null)
const sashDragging = ref(false)

const height = ref(props.drawerHeight)
let drawerDragging = false
let startY = 0
let startH = 0

let sashStartX = 0
let sashStartW = 0
let sashDidDrag = false

function effectiveMax(): number {
  const shellW = shellRef.value?.getBoundingClientRect().width
  const available =
    shellW && shellW > 0
      ? shellW
      : typeof window !== 'undefined'
        ? window.innerWidth
        : SIDEBAR_MAX + STAGE_MIN + SASH_WIDTH
  const room = Math.floor(available - STAGE_MIN - SASH_WIDTH)
  // When the shell is too narrow to fit SIDEBAR_MIN + STAGE_MIN + sash,
  // prefer protecting the stage: allow the sidebar below SIDEBAR_MIN.
  if (room < SIDEBAR_MIN) return Math.max(0, room)
  return Math.min(SIDEBAR_MAX, room)
}

function clampSidebar(px: number): number {
  const max = effectiveMax()
  const min = Math.min(SIDEBAR_MIN, max)
  return Math.max(min, Math.min(max, Math.round(px)))
}

/** Read stored width; returns null for missing/illegal/out-of-[240,480] values. */
function readStored(): number | null {
  if (!props.storageKey) return null
  try {
    const raw = localStorage.getItem(props.storageKey)
    if (raw == null || raw === '') return null
    const n = Number(raw)
    if (!Number.isFinite(n)) return null
    const rounded = Math.round(n)
    // Out-of-range or illegal → ignore and fall back to props default.
    if (rounded < SIDEBAR_MIN || rounded > SIDEBAR_MAX) return null
    return rounded
  } catch {
    return null
  }
}

function writeStored(px: number) {
  if (!props.storageKey) return
  try {
    localStorage.setItem(props.storageKey, String(px))
  } catch {
    /* ignore quota / private mode */
  }
}

function initWidth() {
  width.value = clampSidebar(readStored() ?? props.sidebarWidth)
}

const width = ref(clampSidebar(readStored() ?? props.sidebarWidth))

function onWindowResize() {
  if (props.mobile) return
  width.value = clampSidebar(width.value)
}

function onPointerDown(e: PointerEvent) {
  if (!props.mobile) return
  drawerDragging = true
  startY = e.clientY
  startH = height.value
  ;(e.currentTarget as HTMLElement).setPointerCapture(e.pointerId)
}

function onPointerMove(e: PointerEvent) {
  if (!drawerDragging) return
  const dy = startY - e.clientY
  const max = Math.floor(window.innerHeight * 0.75)
  height.value = Math.max(180, Math.min(max, startH + dy))
}

function onPointerUp() {
  drawerDragging = false
}

function setSashDraggingUi(on: boolean) {
  if (typeof document === 'undefined') return
  document.body.classList.toggle('review-shell-sash-dragging', on)
}

function onSashPointerDown(e: PointerEvent) {
  if (props.mobile) return
  sashDragging.value = true
  sashDidDrag = false
  sashStartX = e.clientX
  sashStartW = width.value
  setSashDraggingUi(true)
  ;(e.currentTarget as HTMLElement).setPointerCapture(e.pointerId)
  e.preventDefault()
}

function onSashPointerMove(e: PointerEvent) {
  if (!sashDragging.value) return
  // Dragging sash left grows the sidebar (sidebar is on the right).
  const dx = sashStartX - e.clientX
  if (Math.abs(dx) > DRAG_THRESHOLD_PX) sashDidDrag = true
  width.value = clampSidebar(sashStartW + dx)
  e.preventDefault()
}

function onSashPointerUp() {
  if (!sashDragging.value) return
  sashDragging.value = false
  setSashDraggingUi(false)
  if (sashDidDrag) writeStored(width.value)
}

function onSashDblClick() {
  if (props.mobile || sashDidDrag) return
  width.value = clampSidebar(props.sidebarWidth)
  writeStored(width.value)
}

watch(
  () => [props.storageKey, props.sidebarWidth, props.mobile] as const,
  () => {
    if (props.mobile || sashDragging.value) return
    initWidth()
  },
)

onMounted(() => {
  if (!props.mobile) initWidth()
  window.addEventListener('resize', onWindowResize)
})

onBeforeUnmount(() => {
  drawerDragging = false
  sashDragging.value = false
  setSashDraggingUi(false)
  window.removeEventListener('resize', onWindowResize)
})
</script>

<template>
  <div
    ref="shellRef"
    class="flex h-full min-h-0"
    :class="[
      mobile ? 'flex-col' : 'flex-row',
      sashDragging ? 'select-none' : '',
    ]"
    data-testid="review-shell"
  >
    <section
      class="flex min-h-0 flex-1 flex-col overflow-hidden"
      :class="mobile ? 'min-w-0 border-b border-line' : 'review-shell-stage'"
      data-testid="review-shell-stage"
    >
      <slot name="stage" />
    </section>

    <div
      v-if="!mobile"
      class="review-shell-sash relative shrink-0 cursor-col-resize bg-line transition-colors hover:bg-accent"
      :class="sashDragging ? 'bg-accent' : ''"
      role="separator"
      aria-orientation="vertical"
      :aria-valuemin="Math.min(SIDEBAR_MIN, effectiveMax())"
      :aria-valuemax="effectiveMax()"
      :aria-valuenow="width"
      :aria-label="t('pages.reviewShell.resizeSash')"
      :title="t('pages.reviewShell.resizeSash')"
      data-testid="review-shell-sash"
      @pointerdown="onSashPointerDown"
      @pointermove="onSashPointerMove"
      @pointerup="onSashPointerUp"
      @pointercancel="onSashPointerUp"
      @dblclick="onSashDblClick"
    />

    <aside
      class="flex min-h-0 flex-col bg-surface"
      :class="mobile ? 'w-full' : 'shrink-0'"
      :style="
        mobile
          ? { height: `${height}px`, maxHeight: '75vh', minHeight: '180px' }
          : { width: `${width}px` }
      "
      data-testid="review-shell-sidebar"
    >
      <div
        v-if="mobile"
        class="flex h-[22px] shrink-0 cursor-ns-resize items-center justify-center gap-2 border-b border-line text-[11px] text-txt3"
        data-testid="review-shell-drawer-handle"
        @pointerdown="onPointerDown"
        @pointermove="onPointerMove"
        @pointerup="onPointerUp"
        @pointercancel="onPointerUp"
      >
        <span class="inline-block h-[3px] w-9 bg-line-strong" />
        {{ t('pages.reviewShell.drawerHandle') }}
      </div>
      <div class="flex min-h-0 flex-1 flex-col overflow-hidden">
        <slot name="sidebar" />
      </div>
    </aside>
  </div>
</template>

<style scoped>
.review-shell-stage {
  /* Second line of defense: keep stage usable when sidebar hits its floor. */
  min-width: 160px;
}
.review-shell-sash {
  width: 4px;
  touch-action: none;
  z-index: 2;
}
/* Expanded hit target (~12px) without widening the visual bar. */
.review-shell-sash::before {
  content: '';
  position: absolute;
  inset: 0 -4px;
}
</style>

<style>
/* Unscoped: applied on document.body while sash is dragged. */
body.review-shell-sash-dragging {
  cursor: col-resize;
  user-select: none;
}
</style>
