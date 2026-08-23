<script setup lang="ts">
import { RouterLink, useRoute } from 'vue-router'
import { computed, ref, watch, onMounted, onBeforeUnmount } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '../ui/Icon.vue'
import { sidebarNavGroups } from '@/data/sidebarNav'
import { usePendingGates } from '@/lib/inbox/usePendingGates'
import { useRunTerminalNotifications } from '@/lib/run/useRunTerminalNotifications'
import { useWorkflowFavorites } from '@/lib/run/useWorkflowFavorites'
import { useWorkflowRunLaunch } from '@/lib/run/useWorkflowRunLaunch'
import { useToast } from '@/lib/composables/useToast'
import { useBreakpoint } from '@/lib/composables/useBreakpoint'

defineProps<{ drawer?: boolean }>()
const emit = defineEmits<{ (e: 'navigate'): void }>()

const { t } = useI18n()
const route = useRoute()
const toast = useToast()

// Shared singleton source so approving a gate elsewhere updates the badge immediately.
const { count: gateCount, peek, refresh } = usePendingGates()
        // Same unreadCount singleton as shell chrome bell — keep sidebar /notifications badge in sync.
const { unreadCount } = useRunTerminalNotifications()
const { displayItems, hydrateDisplay, unfavorite, getFavoriteWorkflow, reorderFavorites } = useWorkflowFavorites()
const { openLaunch } = useWorkflowRunLaunch()
const { isMobile } = useBreakpoint()

const DRAG_THRESHOLD_PX = 4
const quickList = ref<HTMLElement>()
const suppressQuickItemClick = ref(false)
const dragState = ref<{
  workflowId: string
  from: number
  placeholder: number
  x: number
  y: number
  offsetX: number
  offsetY: number
  width: number
}>()
const dragItems = computed(() =>
  dragState.value
    ? displayItems.value.filter((item) => item.workflowId !== dragState.value?.workflowId)
    : displayItems.value,
)

let timer: number | undefined
function badgeFor(to: string): number {
  if (to === '/gates') return gateCount.value
  if (to === '/notifications') return unreadCount.value
  return 0
}
function pollRefresh() {
  return peek({ source: 'sidebar-poll' })
}
onMounted(() => {
  refresh({ source: 'mount' })
  void hydrateDisplay()
  timer = window.setInterval(pollRefresh, 15000)
})
onBeforeUnmount(() => {
  if (timer) clearInterval(timer)
})
// Refresh promptly when navigating (e.g. right after approving a gate).
watch(
  () => route.path,
  () => {
    refresh({ source: 'navigate' })
    void hydrateDisplay()
  },
)

function isActive(to: string) {
  return route.path === to || route.path.startsWith(to + '/')
}

function onNavigate() {
  emit('navigate')
}

function onUnfavorite(workflowId: string, name: string, ev: Event) {
  ev.stopPropagation()
  ev.preventDefault()
  if (suppressQuickItemClick.value) return
  unfavorite(workflowId, { name })
}

async function onLaunch(workflowId: string) {
  if (suppressQuickItemClick.value) return
  try {
    const wf = await getFavoriteWorkflow(workflowId)
    if (!wf) return
    openLaunch(wf)
    onNavigate()
  } catch {
    toast.error(t('common.toast.favoriteLaunchFailed'))
  }
}

function updatePlaceholder(clientY: number) {
  if (!dragState.value || !quickList.value) return
  const rows = Array.from(quickList.value.querySelectorAll<HTMLElement>('[data-sortable-row]'))
  const slot = rows.findIndex((row) => clientY < row.getBoundingClientRect().top + row.offsetHeight / 2)
  dragState.value.placeholder = slot === -1 ? rows.length : slot
}

function onHandlePointerDown(workflowId: string, event: PointerEvent) {
  if (isMobile.value || event.button !== 0) return
  const source = (event.currentTarget as HTMLElement).closest<HTMLElement>('[data-sortable-row]')
  const from = displayItems.value.findIndex((item) => item.workflowId === workflowId)
  if (!source || from < 0) return

  event.preventDefault()
  const handle = event.currentTarget as HTMLElement
  const startX = event.clientX
  const startY = event.clientY
  let activated = false
  let offsetX = 0
  let offsetY = 0

  try {
    handle.setPointerCapture(event.pointerId)
  } catch {
    // Pointer capture is a progressive enhancement for the drag session.
  }

  const activate = (moveEvent: PointerEvent) => {
    const rect = source.getBoundingClientRect()
    offsetX = moveEvent.clientX - rect.left
    offsetY = moveEvent.clientY - rect.top
    activated = true
    suppressQuickItemClick.value = true
    dragState.value = {
      workflowId,
      from,
      placeholder: from,
      x: moveEvent.clientX,
      y: moveEvent.clientY,
      offsetX,
      offsetY,
      width: rect.width,
    }
    document.body.classList.add('quick-pipeline-dragging')
    updatePlaceholder(moveEvent.clientY)
  }

  const onMove = (moveEvent: PointerEvent) => {
    if (!activated) {
      const dx = moveEvent.clientX - startX
      const dy = moveEvent.clientY - startY
      if (Math.hypot(dx, dy) < DRAG_THRESHOLD_PX) return
      activate(moveEvent)
    }
    if (!dragState.value) return
    dragState.value.x = moveEvent.clientX
    dragState.value.y = moveEvent.clientY
    updatePlaceholder(moveEvent.clientY)
    moveEvent.preventDefault()
  }

  const finish = (cancelled: boolean) => {
    handle.removeEventListener('pointermove', onMove)
    handle.removeEventListener('pointerup', onUp)
    handle.removeEventListener('pointercancel', onCancel)
    window.removeEventListener('pointermove', onMove)
    window.removeEventListener('pointerup', onUp)
    window.removeEventListener('pointercancel', onCancel)
    try {
      handle.releasePointerCapture(event.pointerId)
    } catch {
      // Capture may already be released by the browser.
    }
    if (!activated || !dragState.value) return

    const { from: dragFrom, placeholder } = dragState.value
    dragState.value = undefined
    document.body.classList.remove('quick-pipeline-dragging')
    if (!cancelled) reorderFavorites(dragFrom, placeholder)
    window.setTimeout(() => {
      suppressQuickItemClick.value = false
    }, 0)
  }
  const onUp = () => finish(false)
  const onCancel = () => finish(true)

  handle.addEventListener('pointermove', onMove)
  handle.addEventListener('pointerup', onUp)
  handle.addEventListener('pointercancel', onCancel)
  // Once activation removes the source row from the rendered list, some browsers
  // release element-level pointer capture. Window listeners keep the session intact.
  window.addEventListener('pointermove', onMove)
  window.addEventListener('pointerup', onUp)
  window.addEventListener('pointercancel', onCancel)
}

const primaryGroup = sidebarNavGroups[0]
const configGroups = sidebarNavGroups.slice(1)
</script>

<template>
  <nav class="scroll-area flex-1 overflow-y-auto px-3 py-2" data-testid="app-sidebar-nav">
    <!-- Primary static nav (dashboard → notifications) -->
    <div v-if="primaryGroup" class="mb-3">
      <div
        v-if="primaryGroup.titleKey"
        class="px-3 pb-1 pt-2 text-[10px] font-semibold uppercase tracking-wider text-txt3"
      >
        {{ t(primaryGroup.titleKey) }}
      </div>
      <RouterLink
        v-for="item in primaryGroup.items"
        :key="item.to"
        :to="item.to"
        class="nav-item mb-0.5"
        :class="{ active: isActive(item.to) }"
        @click="onNavigate"
      >
        <Icon :name="item.icon" :size="17" />
        <span class="flex-1">{{ t(item.labelKey) }}</span>
        <span
          v-if="badgeFor(item.to)"
          class="flex h-5 min-w-5 items-center justify-center rounded-full bg-accent px-1.5 text-[11px] font-semibold text-white"
          :data-testid="item.to === '/notifications' ? 'nav-notifications-badge' : item.to === '/gates' ? 'nav-gates-badge' : undefined"
        >{{ badgeFor(item.to) }}</span>
      </RouterLink>
    </div>

    <!-- Quick pipelines: only when there are favorites to show -->
    <div v-if="displayItems.length" class="mb-3" data-testid="nav-quick-pipelines">
      <div class="px-3 pb-1 pt-2 text-[10px] font-semibold uppercase tracking-wider text-txt3">
        {{ t('nav.quickPipelines') }}
      </div>
      <div ref="quickList" class="quick-pipelines-list">
        <template v-for="(item, index) in dragItems" :key="item.workflowId">
          <div
            v-if="dragState && dragState.placeholder === index"
            class="quick-pipeline-placeholder"
            data-testid="nav-quick-pipeline-placeholder"
          />
          <div
            class="mb-0.5 grid w-full grid-cols-[28px_1fr_28px] items-center gap-0.5 px-2 py-1.5 text-left text-txt2 transition hover:bg-elevated hover:text-txt"
            data-sortable-row
            data-testid="nav-quick-pipeline-item"
            role="button"
            tabindex="0"
            @click="onLaunch(item.workflowId)"
            @keydown.enter.prevent="onLaunch(item.workflowId)"
            @keydown.space.prevent="onLaunch(item.workflowId)"
          >
            <button
              v-if="!isMobile"
              type="button"
              class="quick-pipeline-handle flex h-7 w-7 items-center justify-center text-txt3 hover:bg-overlay hover:text-txt"
              data-testid="nav-quick-pipeline-drag-handle"
              aria-label="拖动调整顺序"
              title="拖动调整顺序"
              @pointerdown="onHandlePointerDown(item.workflowId, $event)"
            ><span aria-hidden="true">⠿</span></button>
            <div v-else aria-hidden="true" />
            <div class="min-w-0">
              <div class="truncate text-[13px] text-txt" :title="item.name">{{ item.name }}</div>
              <div class="mt-0.5 flex min-w-0 items-center gap-1.5">
                <span class="truncate text-[11px] text-txt3" :title="item.projectName">{{ item.projectName }}</span>
                <span
                  v-if="item.status === 'draft'"
                  class="shrink-0 border border-warn/35 bg-warn/10 px-1.5 py-px text-[10px] text-warn"
                >{{ t('common.status.draft') }}</span>
              </div>
            </div>
            <button
              type="button"
              class="flex h-7 w-7 items-center justify-center text-warn hover:text-warn"
              data-testid="nav-quick-pipeline-unfavorite"
              :aria-label="t('common.buttons.unfavorite')"
              :title="t('common.buttons.unfavorite')"
              @click="onUnfavorite(item.workflowId, item.name, $event)"
            >
              <Icon name="star-filled" :size="14" />
            </button>
          </div>
        </template>
        <div
          v-if="dragState && dragState.placeholder === dragItems.length"
          class="quick-pipeline-placeholder"
          data-testid="nav-quick-pipeline-placeholder"
        />
      </div>
    </div>

    <!-- Config group(s) -->
    <div v-for="(g, gi) in configGroups" :key="'cfg-' + gi" class="mb-3">
      <div v-if="g.titleKey" class="px-3 pb-1 pt-2 text-[10px] font-semibold uppercase tracking-wider text-txt3">
        {{ t(g.titleKey) }}
      </div>
      <RouterLink
        v-for="item in g.items"
        :key="item.to"
        :to="item.to"
        class="nav-item mb-0.5"
        :class="{ active: isActive(item.to) }"
        @click="onNavigate"
      >
        <Icon :name="item.icon" :size="17" />
        <span class="flex-1">{{ t(item.labelKey) }}</span>
        <span
          v-if="badgeFor(item.to)"
          class="flex h-5 min-w-5 items-center justify-center rounded-full bg-accent px-1.5 text-[11px] font-semibold text-white"
        >{{ badgeFor(item.to) }}</span>
      </RouterLink>
    </div>
  </nav>
  <div
    v-if="dragState"
    class="quick-pipeline-drag-float grid grid-cols-[28px_1fr_28px] items-center gap-0.5 px-2 py-1.5"
    :style="{
      width: `${dragState.width}px`,
      left: `${dragState.x - dragState.offsetX}px`,
      top: `${dragState.y - dragState.offsetY}px`,
    }"
    aria-hidden="true"
  >
    <span class="flex h-7 w-7 items-center justify-center text-txt">⠿</span>
    <div class="min-w-0">
      <div class="truncate text-[13px] text-txt">{{ displayItems.find((item) => item.workflowId === dragState?.workflowId)?.name }}</div>
      <div class="truncate text-[11px] text-txt3">{{ displayItems.find((item) => item.workflowId === dragState?.workflowId)?.projectName }}</div>
    </div>
    <span class="flex h-7 w-7 items-center justify-center text-warn"><Icon name="star-filled" :size="14" /></span>
  </div>
</template>

<style scoped>
.quick-pipeline-handle {
  cursor: grab;
  touch-action: none;
}

.quick-pipeline-handle:active {
  cursor: grabbing;
}

.quick-pipeline-placeholder {
  height: 46px;
  margin-bottom: 2px;
  border: 1px dashed rgb(var(--c-accent) / 70%);
  background: rgb(var(--c-accent) / 12%);
}

.quick-pipeline-drag-float {
  position: fixed;
  z-index: 9999;
  pointer-events: none;
  border: 1px solid rgb(var(--c-accent));
  background: rgb(var(--c-elevated));
  box-shadow: 0 12px 28px rgb(0 0 0 / 55%), 0 0 0 1px rgb(var(--c-accent) / 35%);
  opacity: 0.96;
  transform: scale(1.02);
}

:global(body.quick-pipeline-dragging),
:global(body.quick-pipeline-dragging *) {
  cursor: grabbing !important;
  user-select: none;
}
</style>
