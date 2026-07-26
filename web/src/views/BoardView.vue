<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref, toRef, watch } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import RunBoardColumn from '@/components/board/RunBoardColumn.vue'
import RunBoardPreviewDrawer from '@/components/board/RunBoardPreviewDrawer.vue'
import TokenStatsPanel from '@/components/board/token-stats/TokenStatsPanel.vue'
import { serializeStatusQuery } from '@/lib/useStatusFilter'
import { useRunBoard, type BoardColumnKey } from '@/lib/useRunBoard'
import type { Run } from '@/lib/types'

const props = defineProps<{
  /** Required project boundary — board never loads unfiltered platform runs. */
  projectId: string
  /** When embedded in project detail, hide the standalone page heading. */
  embedded?: boolean
}>()

const router = useRouter()
const { t } = useI18n()

const extraEnabled = reactive({
  queued: false,
  failed: false,
  cancelled: false,
})

const extraStatuses = computed(() => {
  const set = new Set<string>()
  if (extraEnabled.queued) set.add('queued')
  if (extraEnabled.failed) set.add('failed')
  if (extraEnabled.cancelled) set.add('cancelled')
  return set
})

const projectIdRef = toRef(props, 'projectId')

const { load, column, loading, hasLoaded, error } = useRunBoard({
  mode: 'full',
  projectId: projectIdRef,
  extraStatuses: () => extraStatuses.value,
})

watch(projectIdRef, () => {
  void load()
})
const selected = ref<Run | null>(null)
const drawerOpen = ref(false)

const mainCols: { key: BoardColumnKey; accent: 'running' | 'waiting' | 'done'; titleKey: string; hintKey: string; emptyKey: string }[] = [
  { key: 'running', accent: 'running', titleKey: 'pages.board.columns.running', hintKey: 'pages.board.hints.inProgress', emptyKey: 'pages.board.empty.running' },
  { key: 'waiting_human', accent: 'waiting', titleKey: 'pages.board.columns.waitingHuman', hintKey: 'pages.board.hints.inReview', emptyKey: 'pages.board.empty.waitingHuman' },
  { key: 'completed', accent: 'done', titleKey: 'pages.board.columns.completed', hintKey: 'pages.board.hints.doneRecent', emptyKey: 'pages.board.empty.completed' },
]

const extraCols: { key: BoardColumnKey; titleKey: string; emptyKey: string }[] = [
  { key: 'queued', titleKey: 'common.status.queued', emptyKey: 'pages.board.empty.extra' },
  { key: 'failed', titleKey: 'common.status.failed', emptyKey: 'pages.board.empty.extra' },
  { key: 'cancelled', titleKey: 'common.status.cancelled', emptyKey: 'pages.board.empty.extra' },
]

const visibleExtras = computed(() => extraCols.filter((c) => extraEnabled[c.key as keyof typeof extraEnabled]))

const showInitialLoading = computed(() => loading.value && !hasLoaded.value)

function truncatedHint(key: BoardColumnKey): string {
  const col = column(key)
  if (!col.truncated) return ''
  if (key === 'running' || key === 'waiting_human' || key === 'queued' || key === 'failed' || key === 'cancelled') {
    return t('pages.board.truncated.showingFirst100')
  }
  return ''
}

function openPreview(run: Run) {
  selected.value = run
  drawerOpen.value = true
}

function closePreview() {
  drawerOpen.value = false
  selected.value = null
}

function goMoreCompleted() {
  router.push({
    path: '/runs',
    query: {
      status: serializeStatusQuery(['completed']),
      projectId: props.projectId,
    },
  })
}

function toggleExtra(key: 'queued' | 'failed' | 'cancelled') {
  extraEnabled[key] = !extraEnabled[key]
  load()
}

let timer: number | undefined
function onVisible() {
  if (document.visibilityState === 'visible') load()
}
function onFocus() {
  load()
}

onMounted(() => {
  load()
  timer = window.setInterval(load, 3000)
  document.addEventListener('visibilitychange', onVisible)
  window.addEventListener('focus', onFocus)
})

onUnmounted(() => {
  if (timer) window.clearInterval(timer)
  document.removeEventListener('visibilitychange', onVisible)
  window.removeEventListener('focus', onFocus)
})
</script>

<template>
  <div data-testid="board-view" class="min-w-0">
    <div v-if="!embedded" class="mb-5 flex flex-wrap items-start justify-between gap-4">
      <div>
        <h2 class="text-lg font-semibold text-txt">{{ t('pages.board.title') }}</h2>
        <p class="text-sm text-txt3">{{ t('pages.board.subtitle') }}</p>
      </div>
    </div>

    <div
      v-if="error && error !== 'missing_project'"
      class="mb-4 flex flex-wrap items-center justify-between gap-2 border border-err/40 bg-err/10 px-3 py-2 text-[13px] text-err"
      data-testid="board-load-error"
    >
      <span>{{ t('pages.board.loadFailed') }}</span>
      <button
        type="button"
        class="border border-err/40 px-2.5 py-1 text-xs text-err hover:bg-err/10"
        data-testid="board-retry"
        @click="load()"
      >
        {{ t('pages.board.retry') }}
      </button>
    </div>

    <TokenStatsPanel :project-id="projectId" />

    <div class="mb-4 flex flex-wrap items-center justify-between gap-3">
      <div class="flex flex-wrap items-center gap-2">
        <span class="mr-1 text-xs text-txt3">{{ t('pages.board.filters.label') }}</span>
        <button
          v-for="key in (['queued', 'failed', 'cancelled'] as const)"
          :key="key"
          type="button"
          class="inline-flex items-center gap-1.5 border px-2.5 py-1 text-xs transition"
          :class="
            extraEnabled[key]
              ? 'border-accent-2/45 bg-accent-dim text-txt'
              : 'border-line bg-surface text-txt2 hover:border-line-strong'
          "
          :data-testid="`board-filter-${key}`"
          @click="toggleExtra(key)"
        >
          <span
            class="inline-block h-2 w-2 border"
            :class="extraEnabled[key] ? 'border-accent-2 bg-accent-2' : 'border-line-strong bg-transparent'"
          />
          {{ t(`common.status.${key}`) }}
        </button>
      </div>
      <span class="text-[11px] text-txt3">{{ t('pages.board.toolbarHint') }}</span>
    </div>

    <div class="grid grid-cols-1 items-start gap-3.5 md:grid-cols-3">
      <RunBoardColumn
        v-for="col in mainCols"
        :key="col.key"
        :title="t(col.titleKey)"
        :hint="t(col.hintKey)"
        :accent="col.accent"
        :items="column(col.key).items"
        :total="column(col.key).total"
        :loading="showInitialLoading"
        :loading-text="t('pages.board.loading')"
        :empty-text="t(col.emptyKey)"
        :truncated-hint="truncatedHint(col.key)"
        @select="openPreview"
      >
        <template v-if="col.key === 'completed'" #footer>
          <button
            type="button"
            class="text-xs text-txt3 hover:text-txt"
            data-testid="board-view-more-completed"
            @click="goMoreCompleted"
          >
            {{ t('pages.board.viewMoreCompleted') }}
          </button>
        </template>
      </RunBoardColumn>
    </div>

    <div
      v-if="visibleExtras.length"
      class="mt-3.5 grid grid-cols-1 items-start gap-3.5 sm:grid-cols-2 lg:grid-cols-3"
      data-testid="board-extra-columns"
    >
      <RunBoardColumn
        v-for="col in visibleExtras"
        :key="col.key"
        :title="t(col.titleKey)"
        :hint="t('pages.board.hints.extraFilter')"
        accent="extra"
        :items="column(col.key).items"
        :total="column(col.key).total"
        :loading="loading && !column(col.key).items.length && !column(col.key).error"
        :loading-text="t('pages.board.loading')"
        :empty-text="t(col.emptyKey)"
        :truncated-hint="truncatedHint(col.key)"
        @select="openPreview"
      />
    </div>

    <RunBoardPreviewDrawer :open="drawerOpen" :run="selected" @close="closePreview" />
  </div>
</template>
