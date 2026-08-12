<script setup lang="ts">
import { RouterLink, useRoute } from 'vue-router'
import { watch, onMounted, onBeforeUnmount } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '../ui/Icon.vue'
import { sidebarNavGroups } from '@/data/sidebarNav'
import { usePendingGates } from '@/lib/inbox/usePendingGates'
import { useRunTerminalNotifications } from '@/lib/run/useRunTerminalNotifications'
import { useWorkflowFavorites } from '@/lib/run/useWorkflowFavorites'
import { useWorkflowRunLaunch } from '@/lib/run/useWorkflowRunLaunch'
import { useToast } from '@/lib/composables/useToast'

defineProps<{ drawer?: boolean }>()
const emit = defineEmits<{ (e: 'navigate'): void }>()

const { t } = useI18n()
const route = useRoute()
const toast = useToast()

// Shared singleton source so approving a gate elsewhere updates the badge immediately.
const { count: gateCount, peek, refresh } = usePendingGates()
// Same unreadCount singleton as topbar bell — keep sidebar /notifications badge in sync.
const { unreadCount } = useRunTerminalNotifications()
const { displayItems, hydrateDisplay, unfavorite, getFavoriteWorkflow } = useWorkflowFavorites()
const { openLaunch } = useWorkflowRunLaunch()

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
  unfavorite(workflowId, { name })
}

async function onLaunch(workflowId: string) {
  try {
    const wf = await getFavoriteWorkflow(workflowId)
    if (!wf) return
    openLaunch(wf)
    onNavigate()
  } catch {
    toast.error(t('common.toast.favoriteLaunchFailed'))
  }
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

    <!-- Quick pipelines: independent of sidebarNavGroups (between notifications & config) -->
    <div class="mb-3" data-testid="nav-quick-pipelines">
      <div class="px-3 pb-1 pt-2 text-[10px] font-semibold uppercase tracking-wider text-txt3">
        {{ t('nav.quickPipelines') }}
      </div>
      <p
        v-if="!displayItems.length"
        class="px-3 text-[12px] leading-snug text-txt3"
        data-testid="nav-quick-pipelines-empty"
      >
        {{ t('nav.quickPipelinesEmpty') }}
      </p>
      <div
        v-for="item in displayItems"
        :key="item.workflowId"
        class="mb-0.5 grid w-full grid-cols-[1fr_28px] items-center gap-0.5 rounded-md px-2 py-1.5 text-left text-txt2 transition hover:bg-elevated hover:text-txt"
        data-testid="nav-quick-pipeline-item"
        role="button"
        tabindex="0"
        @click="onLaunch(item.workflowId)"
        @keydown.enter.prevent="onLaunch(item.workflowId)"
        @keydown.space.prevent="onLaunch(item.workflowId)"
      >
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
</template>
