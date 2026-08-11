<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import Icon from '../ui/Icon.vue'
import LangSelect from '../ui/LangSelect.vue'
import RunOutputPptModal from './RunOutputPptModal.vue'
import { theme, toggleTheme } from '@/lib/shared/theme'
import { isDraining } from '@/lib/composables/useShutdownState'
import { locale, setLocale, type AppLocale } from '@/lib/shared/locale'
import { relTime } from '@/lib/shared/format'
import { useRunTerminalNotifications } from '@/lib/run/useRunTerminalNotifications'

const emit = defineEmits<{ (e: 'toggle-menu'): void }>()

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const menuOpen = ref(false)
const panelOpen = ref(false)
const bellWrapEl = ref<HTMLElement | null>(null)

const {
  previewItems,
  remainingCount,
  unreadCount,
  hasUnreadFailed,
  badgeLabel,
  markRead,
  markAllRead,
  startPolling,
  stopPolling,
} = useRunTerminalNotifications()

const outputOpen = ref(false)
const outputRunId = ref<string | null>(null)
const outputContext = ref('')

const themeTitle = computed(() =>
  t(theme.value === 'dark' ? 'shell.theme.toLight' : 'shell.theme.toDark'),
)

watch(
  () => route.path,
  () => {
    menuOpen.value = false
    panelOpen.value = false
  },
)

function toggleMenu() {
  menuOpen.value = !menuOpen.value
  emit('toggle-menu')
}

function onLocaleSelect(v: AppLocale) {
  void setLocale(v)
}

function togglePanel() {
  panelOpen.value = !panelOpen.value
}

function closePanel() {
  panelOpen.value = false
}

function onDocClick(ev: MouseEvent) {
  if (!panelOpen.value) return
  const root = bellWrapEl.value
  if (root && ev.target instanceof Node && root.contains(ev.target)) return
  closePanel()
}

function onDocKeydown(ev: KeyboardEvent) {
  if (ev.key === 'Escape' && panelOpen.value) {
    closePanel()
  }
}

function statusLabel(status: string) {
  return status === 'failed'
    ? t('common.status.failed')
    : t('common.status.completed')
}

function itemTitle(item: {
  title: string
  titleNeutral?: boolean
  workflowName: string
  runId: string
  status: 'completed' | 'failed'
}) {
  if (item.titleNeutral) {
    return t('shell.runNotifications.neutralTitle', {
      name: item.workflowName || item.runId,
      status: statusLabel(item.status),
    })
  }
  return item.title
}

function itemContext(item: { workflowName: string; runId: string; title: string }) {
  const wf = item.workflowName || item.title
  return wf ? `${wf} · ${item.runId}` : item.runId
}

async function onItemClick(item: {
  runId: string
  status: 'completed' | 'failed'
  title: string
  workflowName: string
}) {
  markRead(item.runId)
  closePanel()
  if (item.status === 'failed') {
    await router.push(`/runs/${item.runId}`)
    return
  }
  outputContext.value = itemContext(item)
  outputRunId.value = item.runId
  outputOpen.value = true
}

function closeOutputModal() {
  outputOpen.value = false
  outputRunId.value = null
  outputContext.value = ''
}

function onMarkAllRead() {
  markAllRead()
}

function viewAll() {
  // Opening "view all" must NOT batch-mark read; goes to independent notifications page.
  closePanel()
  void router.push({ path: '/notifications' })
}

onMounted(() => {
  startPolling()
  document.addEventListener('click', onDocClick)
  document.addEventListener('keydown', onDocKeydown)
})

onBeforeUnmount(() => {
  stopPolling()
  document.removeEventListener('click', onDocClick)
  document.removeEventListener('keydown', onDocKeydown)
})

defineExpose({
  menuOpen,
  toggleMenu,
  panelOpen,
  togglePanel,
  outputOpen,
  outputRunId,
})
</script>

<template>
  <header class="safe-area-top relative z-20 flex h-14 shrink-0 items-center gap-3 border-b border-line bg-surface/80 px-4 backdrop-blur md:px-6">
    <button
      class="flex h-11 w-11 shrink-0 items-center justify-center rounded-md text-txt2 hover:bg-elevated hover:text-txt md:hidden"
      :aria-label="t('shell.aria.openNav')"
      @click="toggleMenu"
    >
      <Icon name="menu" :size="20" />
    </button>
    <div class="flex-1" />
    <span
      v-if="isDraining()"
      class="inline-flex items-center gap-1.5 rounded-md border border-warn/45 bg-warn/10 px-2.5 py-1 text-xs font-medium text-warn"
      :title="t('shell.shutdown.drainingTitle')"
    >
      <span class="inline-flex h-1.5 w-1.5 animate-pulse rounded-full bg-warn" />
      {{ t('shell.shutdown.draining') }}
    </span>
    <LangSelect :model-value="locale" @update:model-value="onLocaleSelect" />
    <button
      class="flex h-9 w-9 items-center justify-center rounded-md text-txt2 hover:bg-elevated hover:text-txt"
      :title="themeTitle"
      @click="toggleTheme"
    >
      <Icon :name="theme === 'dark' ? 'sun' : 'moon'" :size="18" />
    </button>
    <div ref="bellWrapEl" class="relative">
      <button
        type="button"
        class="relative flex h-9 w-9 items-center justify-center rounded-md text-txt2 hover:bg-elevated hover:text-txt"
        :class="panelOpen ? 'bg-elevated text-txt' : ''"
        data-testid="run-notifications-bell"
        :aria-label="t('shell.runNotifications.title')"
        aria-haspopup="true"
        :aria-expanded="panelOpen ? 'true' : 'false'"
        @click.stop="togglePanel"
      >
        <Icon name="bell" :size="18" />
        <span
          v-if="badgeLabel"
          class="absolute right-0.5 top-0.5 inline-flex h-4 min-w-4 items-center justify-center px-1 text-[10px] font-bold leading-none text-white"
          :class="hasUnreadFailed ? 'bg-err' : 'bg-accent'"
          data-testid="run-notifications-badge"
        >{{ badgeLabel }}</span>
      </button>

      <div
        v-if="panelOpen"
        class="absolute right-0 top-[calc(100%+6px)] z-40 flex w-[min(380px,calc(100vw-2rem))] flex-col border border-line-strong bg-elevated shadow-card"
        role="menu"
        :aria-label="t('shell.runNotifications.title')"
        data-testid="run-notifications-panel"
        @click.stop
      >
        <div class="flex items-center justify-between gap-2 border-b border-line px-3.5 py-3">
          <h3 class="m-0 text-[13px] font-semibold text-txt">{{ t('shell.runNotifications.title') }}</h3>
          <div class="flex items-center gap-2">
            <span class="text-xs text-txt3">
              {{ t('shell.runNotifications.unreadCount', { n: unreadCount }) }}
            </span>
            <button
              v-if="unreadCount > 0"
              type="button"
              class="border border-line bg-transparent px-2 py-0.5 text-[11px] text-txt2 hover:border-accent hover:text-accent"
              data-testid="run-notifications-mark-all"
              @click="onMarkAllRead"
            >
              {{ t('shell.runNotifications.markAllRead') }}
            </button>
          </div>
        </div>

        <div class="max-h-[360px] overflow-auto">
          <div
            v-if="!previewItems.length"
            class="px-5 py-9 text-center"
            data-testid="run-notifications-empty"
          >
            <p class="m-0 text-[13px] text-txt2">{{ t('shell.runNotifications.empty') }}</p>
            <p class="mt-1.5 text-xs text-txt3">{{ t('shell.runNotifications.emptyHint') }}</p>
            <p class="mt-1.5 text-xs text-txt3">{{ t('shell.runNotifications.emptyRunsHint') }}</p>
          </div>
          <button
            v-for="item in previewItems"
            :key="item.runId"
            type="button"
            class="relative block w-full border-b border-line px-3.5 py-3 text-left hover:bg-overlay"
            :class="item.unread ? 'bg-accent/5' : ''"
            data-testid="run-notifications-item"
            :data-run-id="item.runId"
            :data-status="item.status"
            :data-unread="item.unread ? 'true' : 'false'"
            :data-before-baseline="item.beforeBaseline ? 'true' : 'false'"
            @click="onItemClick(item)"
          >
            <span
              v-if="item.unread"
              class="absolute bottom-0 left-0 top-0 w-0.5 bg-accent"
              aria-hidden="true"
            />
            <div class="mb-1 flex items-center gap-2">
              <span
                class="shrink-0 border px-1.5 py-0.5 text-[10px] font-semibold"
                :class="
                  item.status === 'failed'
                    ? 'border-err/45 text-err'
                    : 'border-ok/40 text-ok'
                "
              >{{ statusLabel(item.status) }}</span>
              <span class="truncate text-[13px] font-medium text-txt">{{ itemTitle(item) }}</span>
            </div>
            <div class="truncate text-xs text-txt3">
              {{ itemContext(item) }}
              <template v-if="item.status === 'completed'">
                · {{ t('shell.runNotifications.clickForOutput') }}
              </template>
              <template v-else>
                · {{ t('shell.runNotifications.clickForDetail') }}
              </template>
              · {{ relTime(item.finishedApprox || item.startedAt) }}
              <template v-if="item.beforeBaseline">
                ·
                <span data-testid="run-notifications-before-baseline">{{
                  t('shell.runNotifications.beforeBaseline')
                }}</span>
              </template>
            </div>
          </button>
        </div>

        <div class="flex flex-col gap-2 border-t border-line px-3.5 py-2.5">
          <div
            v-if="remainingCount > 0"
            class="text-xs text-txt3"
            data-testid="run-notifications-more"
          >
            {{ t('shell.runNotifications.moreHint', { n: remainingCount }) }}
          </div>
          <button
            type="button"
            class="w-full border border-line bg-transparent px-3 py-2 text-[13px] text-accent-2 hover:border-accent hover:bg-accent-dim hover:text-accent"
            data-testid="run-notifications-view-all"
            @click="viewAll"
          >
            {{ t('shell.runNotifications.viewAll') }}
          </button>
        </div>
      </div>
    </div>

    <RunOutputPptModal
      :open="outputOpen"
      :run-id="outputRunId"
      :context-label="outputContext"
      @close="closeOutputModal"
    />
  </header>
</template>
