<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/ui/Icon.vue'
import RunBoardColumn from '@/components/board/RunBoardColumn.vue'
import RunBoardPreviewDrawer from '@/components/board/RunBoardPreviewDrawer.vue'
import { api, type DashboardStats } from '@/lib/api'
import { readStoredProjectId } from '@/lib/useProjectContext'
import { useRunBoard } from '@/lib/useRunBoard'
import type { Run } from '@/lib/types'

const router = useRouter()
const { t } = useI18n()
const stats = ref<DashboardStats | null>(null)
const statsError = ref<string | null>(null)
const storedProjectId = ref(readStoredProjectId())

const hasProject = computed(() => !!storedProjectId.value)

const { load, column, loading, hasLoaded, error: boardError } = useRunBoard({
  mode: 'dashboard',
  projectId: () => storedProjectId.value,
})

const selected = ref<Run | null>(null)
const drawerOpen = ref(false)

const kpis = computed(() => [
  { label: t('pages.dashboard.kpi.running'), value: stats.value?.running ?? 0, icon: 'runs', cls: 'text-info' },
  { label: t('pages.dashboard.kpi.waitingHuman'), value: stats.value?.waitingHuman ?? 0, icon: 'gate', cls: 'text-warn' },
  { label: t('pages.dashboard.kpi.failed'), value: stats.value?.failed ?? 0, icon: 'alert', cls: 'text-err' },
  { label: t('pages.dashboard.kpi.completed'), value: stats.value?.completed ?? 0, icon: 'check', cls: 'text-ok' },
])

const showInitialLoading = computed(() => hasProject.value && loading.value && !hasLoaded.value)
const loadError = computed(() => {
  if (statsError.value) return statsError.value
  if (!hasProject.value) return null
  if (boardError.value === 'missing_project') return null
  return boardError.value
})

function openPreview(run: Run) {
  selected.value = run
  drawerOpen.value = true
}

function closePreview() {
  drawerOpen.value = false
  selected.value = null
}

function goFullBoard() {
  const id = storedProjectId.value
  if (id) {
    void router.push({ path: `/projects/${id}`, query: { tab: 'board' } })
    return
  }
  void router.push('/projects')
}

function goSelectProject() {
  void router.push('/projects')
}

async function refreshStats() {
  try {
    stats.value = await api.dashboard()
    statsError.value = null
  } catch (err) {
    console.warn('[DashboardView] dashboard stats failed', err)
    statsError.value = err instanceof Error ? err.message : String(err || 'stats failed')
  }
}

async function refreshBoard() {
  storedProjectId.value = readStoredProjectId()
  if (!storedProjectId.value) return
  await load()
}

async function refreshAll() {
  await Promise.all([refreshStats(), refreshBoard()])
}

let timer: number | undefined
function onVisible() {
  if (document.visibilityState === 'visible') refreshAll()
}

onMounted(() => {
  refreshAll()
  timer = window.setInterval(refreshAll, 8000)
  document.addEventListener('visibilitychange', onVisible)
})

onUnmounted(() => {
  if (timer) window.clearInterval(timer)
  document.removeEventListener('visibilitychange', onVisible)
})
</script>

<template>
  <div data-testid="dashboard-view">
    <div class="mb-5 flex items-center justify-between">
      <div>
        <h2 class="text-lg font-semibold text-txt">{{ t('pages.dashboard.title') }}</h2>
        <p class="text-sm text-txt3">{{ t('pages.dashboard.subtitle') }}</p>
      </div>
    </div>

    <div
      v-if="loadError"
      class="mb-4 flex flex-wrap items-center justify-between gap-2 border border-err/40 bg-err/10 px-3 py-2 text-[13px] text-err"
      data-testid="dashboard-load-error"
    >
      <span>{{ t('pages.board.loadFailed') }}</span>
      <button
        type="button"
        class="border border-err/40 px-2.5 py-1 text-xs text-err hover:bg-err/10"
        data-testid="dashboard-retry"
        @click="refreshAll()"
      >
        {{ t('pages.board.retry') }}
      </button>
    </div>

    <div class="mb-6 grid grid-cols-2 gap-4 md:grid-cols-4">
      <div v-for="k in kpis" :key="k.label" class="card p-4">
        <div class="flex items-center justify-between">
          <span class="text-[13px] text-txt2">{{ k.label }}</span>
          <Icon :name="k.icon" :size="18" :class="k.cls" />
        </div>
        <div class="mt-2 text-3xl font-semibold text-txt">{{ k.value }}</div>
      </div>
    </div>

    <div class="card">
      <div class="flex items-center justify-between border-b border-line px-5 py-3">
        <h3 class="text-sm font-semibold text-txt">{{ t('pages.dashboard.boardTitle') }}</h3>
        <button
          v-if="hasProject"
          class="text-xs text-txt3 hover:text-txt"
          data-testid="dashboard-view-full-board"
          @click="goFullBoard"
        >
          {{ t('pages.dashboard.viewFullBoard') }}
        </button>
      </div>
      <div class="p-3.5">
        <div
          v-if="!hasProject"
          class="flex flex-col items-center justify-center gap-3 px-4 py-10 text-center"
          data-testid="dashboard-board-empty"
        >
          <p class="text-sm text-txt3">{{ t('pages.dashboard.noProjectBoard') }}</p>
          <button
            type="button"
            class="border border-line bg-surface px-3 py-1.5 text-xs text-txt2 hover:border-line-strong hover:text-txt"
            data-testid="dashboard-select-project"
            @click="goSelectProject"
          >
            {{ t('pages.dashboard.selectProject') }}
          </button>
        </div>
        <div v-else class="grid grid-cols-1 items-start gap-3.5 md:grid-cols-2">
          <RunBoardColumn
            :title="t('pages.board.columns.active')"
            :hint="t('pages.board.hints.active')"
            accent="active"
            :items="column('active').items"
            :total="column('active').total"
            :loading="showInitialLoading"
            :loading-text="t('pages.board.loading')"
            :empty-text="t('pages.board.empty.active')"
            @select="openPreview"
          />
          <RunBoardColumn
            :title="t('pages.board.columns.completed')"
            :hint="t('pages.board.hints.completedOnly')"
            accent="done"
            :items="column('completed').items"
            :total="column('completed').total"
            :loading="showInitialLoading"
            :loading-text="t('pages.board.loading')"
            :empty-text="t('pages.board.empty.completed')"
            @select="openPreview"
          />
        </div>
      </div>
    </div>

    <RunBoardPreviewDrawer :open="drawerOpen" :run="selected" @close="closePreview" />
  </div>
</template>
