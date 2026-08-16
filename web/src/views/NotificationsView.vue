<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import EmptyState from '@/components/ui/EmptyState.vue'
import Icon from '@/components/ui/Icon.vue'
import Pagination from '@/components/ui/Pagination.vue'
import RunOutputPptModal from '@/components/shell/RunOutputPptModal.vue'
import { useNotificationsPageEntry } from '@/lib/composables/useNotificationsPageEntry'
import { relTime } from '@/lib/shared/format'
import { useRunTerminalNotifications } from '@/lib/run/useRunTerminalNotifications'

type ReadFilter = 'all' | 'unread' | 'read'

const PAGE_SIZE = 20

const { t } = useI18n()
const router = useRouter()
const { enterNonce } = useNotificationsPageEntry()

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
const page = ref(1)
const outputOpen = ref(false)
const outputRunId = ref<string | null>(null)
const outputContext = ref('')

const filteredItems = computed(() => {
  const items = listItems.value
  if (filter.value === 'unread') return items.filter((n) => n.unread)
  if (filter.value === 'read') return items.filter((n) => !n.unread)
  return items
})

const filteredTotal = computed(() => filteredItems.value.length)

const pagedItems = computed(() => {
  const start = (page.value - 1) * PAGE_SIZE
  return filteredItems.value.slice(start, start + PAGE_SIZE)
})

const pageRangeText = computed(() => {
  const n = filteredTotal.value
  if (n <= 0) return ''
  const k = page.value
  const from = (k - 1) * PAGE_SIZE + 1
  const to = Math.min(k * PAGE_SIZE, n)
  return t('pages.notifications.pageRange', { page: k, from, to, total: n })
})

const pagerSummary = computed(() =>
  t('pages.notifications.pagerSummary', { total: filteredTotal.value, pageSize: PAGE_SIZE }),
)

function setFilter(next: ReadFilter) {
  filter.value = next
  page.value = 1
}

function resetToAllFirstPage() {
  filter.value = 'all'
  page.value = 1
}

function clampPage() {
  const n = filteredTotal.value
  if (n <= 0) {
    page.value = 1
    return
  }
  const last = Math.ceil(n / PAGE_SIZE)
  if (page.value > last || pagedItems.value.length === 0) {
    page.value = last
  }
}

watch(enterNonce, () => {
  resetToAllFirstPage()
})

watch(filteredTotal, () => {
  clampPage()
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
  if (item.status === 'failed') {
    markRead(item.runId)
    await router.push(`/runs/${item.runId}`)
    return
  }
  // completed: defer markRead until user clicks「标记已读」in the output modal
  outputContext.value = itemContext(item)
  outputRunId.value = item.runId
  outputOpen.value = true
}

function closeOutputModal() {
  outputOpen.value = false
  outputRunId.value = null
  outputContext.value = ''
}

function onOutputMarkRead() {
  if (outputRunId.value) markRead(outputRunId.value)
  closeOutputModal()
}

onMounted(() => {
  ensureUsername()
  void refresh({ source: 'manual' })
})

defineExpose({
  page,
  filter,
  PAGE_SIZE,
})
</script>

<template>
  <div class="flex h-full min-h-0 flex-col" data-testid="notifications-page">
    <div class="min-h-0 flex-1 overflow-auto">
      <div class="mx-auto w-full max-w-[880px] px-4 py-4 md:px-6 md:py-6">
        <header class="flex flex-col items-stretch gap-3 border-b border-line pb-4 md:flex-row md:flex-wrap md:items-start md:justify-between">
          <div>
            <h1 class="m-0 text-lg font-semibold tracking-tight text-txt">{{ t('pages.notifications.title') }}</h1>
            <p class="mt-1 text-xs text-txt3">{{ t('pages.notifications.subtitle') }}</p>
            <p
              v-if="pageRangeText"
              class="mt-1 text-xs text-txt3"
              data-testid="notifications-page-range"
            >
              {{ pageRangeText }}
            </p>
          </div>
          <div class="flex flex-wrap items-center gap-2">
            <span class="text-xs text-txt3" data-testid="notifications-unread-count">
              {{ t('shell.runNotifications.unreadCount', { n: unreadCount }) }}
            </span>
            <button
              type="button"
              class="min-h-11 border border-line bg-transparent px-3 text-[12px] text-txt2 hover:border-accent hover:text-accent disabled:opacity-40"
              data-testid="notifications-mark-all"
              :disabled="unreadCount === 0"
              @click="markAllRead()"
            >
              {{ t('shell.runNotifications.markAllRead') }}
            </button>
          </div>
        </header>

        <div class="flex flex-wrap items-center gap-2 py-3">
          <button
            v-for="opt in (['all', 'unread', 'read'] as const)"
            :key="opt"
            type="button"
            class="min-h-11 border px-2.5 py-1 text-[12px]"
            :class="
              filter === opt
                ? 'border-accent/45 bg-accent-dim text-txt'
                : 'border-line bg-transparent text-txt2 hover:border-line-strong hover:text-txt'
            "
            :data-testid="`notifications-filter-${opt}`"
            @click="setFilter(opt)"
          >
            {{ t(`pages.notifications.filter.${opt}`) }}
          </button>
        </div>

        <div
          v-if="loading && !listItems.length"
          class="border border-line bg-surface px-6 py-14 text-center text-sm text-txt3 shadow-card"
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
        <div v-else class="border border-line bg-surface shadow-card">
          <button
            v-for="item in pagedItems"
            :key="item.runId"
            type="button"
            class="group relative flex w-full items-start gap-3 border-b border-line px-4 py-3 text-left last:border-b-0 hover:bg-elevated md:px-4"
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
            <span
              class="mt-0.5 inline-flex h-[26px] w-[26px] shrink-0 items-center justify-center border"
              :class="
                item.status === 'failed'
                  ? 'border-err/45 bg-err/10 text-err'
                  : 'border-ok/40 bg-ok/10 text-ok'
              "
              aria-hidden="true"
            >
              <Icon :name="item.status === 'failed' ? 'close' : 'check'" :size="13" />
            </span>
            <span class="min-w-0 flex-1">
              <span class="flex items-center gap-2">
                <span class="truncate text-[13px] font-medium text-txt">{{ itemTitle(item) }}</span>
                <span
                  class="shrink-0 border px-1.5 py-0.5 text-[10px] font-semibold"
                  :class="
                    item.status === 'failed' ? 'border-err/45 text-err' : 'border-ok/40 text-ok'
                  "
                >{{ statusLabel(item.status) }}</span>
              </span>
              <span class="mt-1 flex min-w-0 flex-wrap items-center gap-x-2 gap-y-0.5 text-xs text-txt3">
                <span class="truncate text-txt2">{{ item.workflowName || item.title }}</span>
                <span aria-hidden="true">·</span>
                <span class="font-mono text-[11px]">{{ item.runId }}</span>
                <template v-if="item.beforeBaseline">
                  <span aria-hidden="true">·</span>
                  <span data-testid="notifications-before-baseline">{{
                    t('shell.runNotifications.beforeBaseline')
                  }}</span>
                </template>
              </span>
            </span>
            <span class="hidden w-32 shrink-0 flex-col items-end gap-1 text-right sm:flex">
              <span class="text-xs tabular-nums text-txt3">{{ relTime(item.finishedApprox || item.startedAt) }}</span>
              <span class="text-[11px] text-accent-2 opacity-0 transition-opacity group-hover:opacity-100">
                {{
                  item.status === 'completed'
                    ? t('shell.runNotifications.clickForOutput')
                    : t('shell.runNotifications.clickForDetail')
                }}
              </span>
            </span>
          </button>
        </div>

        <Pagination
          v-if="filteredTotal > PAGE_SIZE"
          v-model:page="page"
          class="shrink-0"
          data-testid="notifications-pagination"
          :page-size="PAGE_SIZE"
          :total="filteredTotal"
          :summary-override="pagerSummary"
          summary-test-id="notifications-pager-summary"
        />
      </div>
    </div>

    <RunOutputPptModal
      :open="outputOpen"
      :run-id="outputRunId"
      :context-label="outputContext"
      @close="closeOutputModal"
      @mark-read="onOutputMarkRead"
    />
  </div>
</template>
