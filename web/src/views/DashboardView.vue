<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/ui/Icon.vue'
import TokenUsageHoverTip from '@/components/ui/TokenUsageHoverTip.vue'
import RunBoardColumn from '@/components/board/RunBoardColumn.vue'
import RunBoardPreviewDrawer from '@/components/board/RunBoardPreviewDrawer.vue'
import { api, type DashboardStats } from '@/lib/api/api'
import { fmtCompactTokenCount } from '@/lib/run/tokenUsage'
import { readStoredProjectId } from '@/lib/composables/useProjectContext'
import { useRunBoard } from '@/lib/run/useRunBoard'
import { serializeStatusQuery } from '@/lib/composables/useStatusFilter'
import type { Run } from '@/lib/shared/types'

type TokenSnapshot = {
  totalTokens: number | null
  workflowTokens: number | null
  pmTokens: number | null
}

const router = useRouter()
const { t } = useI18n()
const stats = ref<DashboardStats | null>(null)
const statsError = ref<string | null>(null)
const storedProjectId = ref(readStoredProjectId())

/** Last successful Token snapshot for stale-while-revalidate (never flash to 0). */
const lastSuccessTokens = ref<TokenSnapshot | null>(null)
const tokensRefreshing = ref(false)

const hasProject = computed(() => !!storedProjectId.value)

const { load, column, loading, hasLoaded, error: boardError } = useRunBoard({
  mode: 'dashboard',
  projectId: () => storedProjectId.value,
})

const selected = ref<Run | null>(null)
const drawerOpen = ref(false)

const kpis = computed(() => [
  {
    status: 'running',
    label: t('pages.dashboard.kpi.running'),
    value: stats.value?.running ?? 0,
    icon: 'runs',
    cls: 'text-info',
  },
  {
    status: 'waiting_human',
    label: t('pages.dashboard.kpi.waitingHuman'),
    value: stats.value?.waitingHuman ?? 0,
    icon: 'gate',
    cls: 'text-warn',
  },
  {
    status: 'failed',
    label: t('pages.dashboard.kpi.failed'),
    value: stats.value?.failed ?? 0,
    icon: 'alert',
    cls: 'text-err',
  },
  {
    status: 'completed',
    label: t('pages.dashboard.kpi.completed'),
    value: stats.value?.completed ?? 0,
    icon: 'check',
    cls: 'text-ok',
  },
])

const displayTokens = computed<TokenSnapshot | null>(() => lastSuccessTokens.value)

const tokenDisplayValue = computed(() =>
  displayTokens.value ? displayTokens.value.totalTokens : undefined,
)

const tokenFoot = computed(() => {
  if (tokensRefreshing.value && lastSuccessTokens.value) {
    return t('pages.dashboard.kpi.totalTokensUpdating')
  }
  if (!lastSuccessTokens.value) {
    return tokensRefreshing.value
      ? t('pages.dashboard.kpi.totalTokensUpdating')
      : t('pages.dashboard.kpi.totalTokensScope')
  }
  const total = lastSuccessTokens.value.totalTokens
  if (total == null) return t('pages.dashboard.kpi.totalTokensUnreported')
  if (total === 0) return t('pages.dashboard.kpi.totalTokensZero')
  return t('pages.dashboard.kpi.totalTokensScope')
})

const tokenAria = computed(() => {
  const total = tokenDisplayValue.value
  if (total == null) return t('pages.dashboard.kpi.totalTokensAriaUnreported')
  return t('pages.dashboard.kpi.totalTokensAria', {
    count: fmtCompactTokenCount(total),
  })
})

const showTokenTip = computed(() => tokenDisplayValue.value != null)

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

function goKpiRuns(status: string) {
  void router.push({
    path: '/runs',
    query: { status: serializeStatusQuery([status]) },
  })
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

function snapshotTokens(s: DashboardStats): TokenSnapshot {
  return {
    totalTokens: s.totalTokens ?? null,
    workflowTokens: s.workflowTokens ?? null,
    pmTokens: s.pmTokens ?? null,
  }
}

async function refreshStats() {
  const hadSuccess = lastSuccessTokens.value != null
  tokensRefreshing.value = true
  try {
    const next = await api.dashboard()
    stats.value = next
    statsError.value = null
    lastSuccessTokens.value = snapshotTokens(next)
  } catch (err) {
    console.warn('[DashboardView] dashboard stats failed', err)
    statsError.value = err instanceof Error ? err.message : String(err || 'stats failed')
    // Keep lastSuccessTokens on failure so we never flash a fake 0.
    if (!hadSuccess) {
      // First load failed: still do not invent 0; leave snapshot null.
    }
  } finally {
    tokensRefreshing.value = false
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
  <div data-testid="dashboard-view" class="flex flex-col md:h-full md:min-h-0">
    <div class="mb-5 flex shrink-0 items-center justify-between">
      <div>
        <h2 class="text-lg font-semibold text-txt">{{ t('pages.dashboard.title') }}</h2>
        <p class="text-sm text-txt3">{{ t('pages.dashboard.subtitle') }}</p>
      </div>
    </div>

    <div
      v-if="loadError"
      class="mb-4 flex shrink-0 flex-wrap items-center justify-between gap-2 border border-err/40 bg-err/10 px-3 py-2 text-[13px] text-err"
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

    <!-- Desktop 5 / mid 3 / narrow 2; Token after status KPIs (plan g2.1) -->
    <div
      class="mb-6 grid shrink-0 grid-cols-2 gap-4 md:grid-cols-3 lg:grid-cols-5"
      data-testid="dashboard-kpi-grid"
    >
      <button
        v-for="k in kpis"
        :key="k.status"
        type="button"
        class="card w-full cursor-pointer p-4 text-left transition hover:border-line-strong hover:bg-elevated focus-visible:border-accent focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent/35"
        :data-testid="`dashboard-kpi-${k.status}`"
        :aria-label="t('pages.dashboard.kpi.ariaNav', { label: k.label, count: k.value })"
        @click="goKpiRuns(k.status)"
      >
        <div class="flex items-center justify-between">
          <span class="text-[13px] text-txt2">{{ k.label }}</span>
          <Icon :name="k.icon" :size="18" :class="k.cls" />
        </div>
        <div class="mt-2 text-3xl font-semibold text-txt">{{ k.value }}</div>
      </button>

      <div
        class="group relative card w-full p-4 text-left transition"
        :class="[
          showTokenTip
            ? 'cursor-help border-accent/45 bg-gradient-to-b from-accent-dim/80 to-surface hover:border-accent'
            : 'border-accent/35 bg-gradient-to-b from-accent-dim/50 to-surface',
          tokensRefreshing && lastSuccessTokens ? 'opacity-70' : '',
        ]"
        data-testid="dashboard-kpi-total-tokens"
        role="group"
        :aria-label="tokenAria"
        :aria-describedby="showTokenTip ? 'dashboard-token-detail-tip' : undefined"
        :tabindex="showTokenTip ? 0 : undefined"
        @click.stop
      >
        <div class="flex items-center justify-between gap-2">
          <span class="text-[13px] text-txt2">{{ t('pages.dashboard.kpi.totalTokens') }}</span>
          <Icon name="sparkles" :size="18" class="text-accent-2" />
        </div>
        <div
          class="mt-2 font-mono text-3xl font-semibold tabular-nums tracking-tight"
          :class="tokenDisplayValue == null ? 'text-txt3' : 'text-txt'"
          data-testid="dashboard-kpi-total-tokens-value"
        >
          {{ fmtCompactTokenCount(tokenDisplayValue) }}
        </div>
        <div
          class="mt-1.5 text-[11px] text-txt3"
          data-testid="dashboard-kpi-total-tokens-foot"
        >
          {{ tokenFoot }}
        </div>
        <TokenUsageHoverTip
          v-if="showTokenTip && displayTokens"
          tip-id="dashboard-token-detail-tip"
          :total-tokens="displayTokens.totalTokens!"
          :workflow-tokens="displayTokens.workflowTokens"
          :pm-tokens="displayTokens.pmTokens"
        />
      </div>
    </div>

    <div
      class="card"
      :class="hasProject ? 'md:flex md:min-h-0 md:flex-1 md:flex-col md:overflow-hidden' : ''"
    >
      <div class="flex shrink-0 items-center justify-between border-b border-line px-5 py-3">
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
      <div
        class="p-3.5"
        :class="hasProject ? 'md:flex md:min-h-0 md:flex-1 md:flex-col' : ''"
      >
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
        <div
          v-else
          class="grid grid-cols-1 items-start gap-3.5 md:grid-cols-2 md:min-h-0 md:flex-1 md:items-stretch"
        >
          <RunBoardColumn
            :title="t('pages.board.columns.active')"
            :hint="t('pages.board.hints.active')"
            accent="active"
            :items="column('active').items"
            :total="column('active').total"
            :loading="showInitialLoading"
            :loading-text="t('pages.board.loading')"
            :empty-text="t('pages.board.empty.active')"
            :fill="true"
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
            :fill="true"
            @select="openPreview"
          />
        </div>
      </div>
    </div>

    <RunBoardPreviewDrawer :open="drawerOpen" :run="selected" @close="closePreview" />
  </div>
</template>
