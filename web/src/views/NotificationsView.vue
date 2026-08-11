<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import EmptyState from '@/components/ui/EmptyState.vue'
import RunOutputPptModal from '@/components/shell/RunOutputPptModal.vue'
import { relTime } from '@/lib/format'
import { useRunTerminalNotifications } from '@/lib/useRunTerminalNotifications'

type ReadFilter = 'all' | 'unread' | 'read'

const { t } = useI18n()
const router = useRouter()

const {
  listItems,
  unreadCount,
  markRead,
  markAllRead,
  refresh,
  ensureUsername,
  loading,
} = useRunTerminalNotifications()

const filter = ref<ReadFilter>('all')
const outputOpen = ref(false)
const outputRunId = ref<string | null>(null)
const outputContext = ref('')

const filteredItems = computed(() => {
  const items = listItems.value
  if (filter.value === 'unread') return items.filter((n) => n.unread)
  if (filter.value === 'read') return items.filter((n) => !n.unread)
  return items
})

function statusLabel(status: string) {
  return status === 'failed' ? t('common.status.failed') : t('common.status.completed')
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

onMounted(() => {
  ensureUsername()
  void refresh({ source: 'manual' })
})
</script>

<template>
  <div class="flex h-full min-h-0 flex-col" data-testid="notifications-page">
    <div class="flex flex-wrap items-start justify-between gap-3 border-b border-line px-4 py-4 md:px-6">
      <div>
        <h1 class="m-0 text-base font-semibold text-txt">{{ t('pages.notifications.title') }}</h1>
        <p class="mt-1 text-xs text-txt3">{{ t('pages.notifications.subtitle') }}</p>
      </div>
      <div class="flex flex-wrap items-center gap-2">
        <span class="text-xs text-txt3">
          {{ t('shell.runNotifications.unreadCount', { n: unreadCount }) }}
        </span>
        <button
          type="button"
          class="border border-line bg-transparent px-3 py-1.5 text-[12px] text-txt2 hover:border-accent hover:text-accent disabled:opacity-40"
          data-testid="notifications-mark-all"
          :disabled="unreadCount === 0"
          @click="markAllRead()"
        >
          {{ t('shell.runNotifications.markAllRead') }}
        </button>
      </div>
    </div>

    <div class="flex flex-wrap items-center gap-2 border-b border-line px-4 py-2.5 md:px-6">
      <button
        v-for="opt in (['all', 'unread', 'read'] as const)"
        :key="opt"
        type="button"
        class="border px-2.5 py-1 text-[12px]"
        :class="
          filter === opt
            ? 'border-accent/45 bg-accent-dim text-txt'
            : 'border-line bg-transparent text-txt2 hover:border-line-strong hover:text-txt'
        "
        :data-testid="`notifications-filter-${opt}`"
        @click="filter = opt"
      >
        {{ t(`pages.notifications.filter.${opt}`) }}
      </button>
    </div>

    <div class="min-h-0 flex-1 overflow-auto">
      <div
        v-if="loading && !listItems.length"
        class="px-6 py-14 text-center text-sm text-txt3"
        data-testid="notifications-loading"
      >
        {{ t('pages.notifications.loading') }}
      </div>
      <EmptyState
        v-else-if="!filteredItems.length && filter === 'all'"
        icon="bell"
        :title="t('shell.runNotifications.empty')"
        :desc="`${t('shell.runNotifications.emptyHint')} ${t('shell.runNotifications.emptyRunsHint')}`"
      />
      <EmptyState
        v-else-if="!filteredItems.length"
        icon="bell"
        :title="t('pages.notifications.filterEmpty')"
        :desc="t('pages.notifications.filterEmptyHint')"
      />
      <button
        v-for="item in filteredItems"
        :key="item.runId"
        type="button"
        class="relative block w-full border-b border-line px-4 py-3.5 text-left hover:bg-elevated md:px-6"
        :class="item.unread ? 'bg-accent/5' : ''"
        data-testid="notifications-item"
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
              item.status === 'failed' ? 'border-err/45 text-err' : 'border-ok/40 text-ok'
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
            <span data-testid="notifications-before-baseline">{{
              t('shell.runNotifications.beforeBaseline')
            }}</span>
          </template>
        </div>
      </button>
    </div>

    <RunOutputPptModal
      :open="outputOpen"
      :run-id="outputRunId"
      :context-label="outputContext"
      @close="closeOutputModal"
    />
  </div>
</template>
