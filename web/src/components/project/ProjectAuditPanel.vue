<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import AuditFilterDropdown, { type AuditDdOption } from '@/components/project/AuditFilterDropdown.vue'
import EmptyState from '@/components/ui/EmptyState.vue'
import { api, isPaginated, type PaginatedResponse } from '@/lib/api'
import { prettyAuditPayload } from '@/lib/auditPayload'
import { useBreakpoint } from '@/lib/useBreakpoint'
import { useToast } from '@/lib/useToast'
import { fmtTime } from '@/lib/format'
import type {
  ProjectAuditEvent,
  ProjectAuditFacetResource,
  ProjectAuditFacetRun,
  ProjectAuditStats,
} from '@/lib/types'

const props = defineProps<{
  projectId: string
  /** Test/demo hook: force the Demo「无权查看」state without a read-only role. */
  forceDenied?: boolean
}>()

const { t } = useI18n()
const toast = useToast()
const { isMobile } = useBreakpoint()

type Mode = 'run' | 'all'

const loading = ref(false)
const denied = ref(false)
const events = ref<ProjectAuditEvent[]>([])
const total = ref(0)
const stats = ref<ProjectAuditStats>({ total: 0, mcp: 0, fail: 0 })
const page = ref(1)
const pageSize = ref(10)
const openId = ref<string | null>(null)
const mode = ref<Mode>('run')
const initialized = ref(false)
/** Mobile filter editor: default collapsed (plan g2 / Demo). */
const filtersExpanded = ref(false)

const timeWindow = ref<'24h' | '7d' | '30d'>('24h')
const runId = ref('')
const nodeId = ref('')
const callerKind = ref('')
const resource = ref('')
const search = ref('')

const runOptions = ref<ProjectAuditFacetRun[]>([])
const nodeOptions = ref<{ nodeId: string; label: string }[]>([])
const resourceOptions = ref<ProjectAuditFacetResource[]>([])

const ACTION_CLASS: Record<string, string> = {
  'mcp.call': 'mcp',
  'run.start': 'run',
  'run.cancel': 'run',
  'run.completed': 'run',
  'run.failed': 'run',
  'run.cancelled': 'run',
  'gate.decide': 'gate',
  'workflow.create': 'cfg',
  'workflow.update': 'cfg',
  'workflow.delete': 'cfg',
  'workflow.publish': 'cfg',
  'project.config': 'cfg',
  'audit.export': 'exp',
}

const NODE_LABEL: Record<string, string> = {
  research: '代码调研',
  react: '需求澄清',
  visual: '视觉网页',
  gate: '门禁',
  plan: '计划',
  proposal: '方案',
  implement: '实现',
  test: '测试',
  review: '评审',
  app_preview: '预览',
}

const CALLER_LABEL_KEYS: Record<string, string> = {
  pm: 'pages.projectDetail.audit.callerPm',
  apikey: 'pages.projectDetail.audit.callerApiKey',
  system: 'pages.projectDetail.audit.callerSystem',
}

const hasMore = computed(() => page.value * pageSize.value < total.value)
const pageCount = computed(() => Math.max(1, Math.ceil(total.value / pageSize.value) || 1))

const runDdOptions = computed<AuditDdOption[]>(() =>
  runOptions.value.map((r) => ({
    value: r.runId,
    label: r.label,
    sub: r.sub,
    short: r.runId.replace(/^run-/, '').slice(0, 8),
    dot: 'run',
  })),
)

const nodeDdOptions = computed<AuditDdOption[]>(() => [
  { value: '', label: t('pages.projectDetail.audit.filterAll') },
  ...nodeOptions.value.map((n) => ({
    value: n.nodeId,
    label: NODE_LABEL[n.nodeId] || n.label || n.nodeId,
    sub: n.nodeId,
  })),
])

const callerDdOptions = computed<AuditDdOption[]>(() => [
  { value: '', label: t('pages.projectDetail.audit.filterAll') },
  { value: 'pm', label: t('pages.projectDetail.audit.callerPm'), sub: t('pages.projectDetail.audit.callerPmSub') },
  { value: 'apikey', label: t('pages.projectDetail.audit.callerApiKey'), sub: t('pages.projectDetail.audit.callerApiKeySub') },
  { value: 'system', label: t('pages.projectDetail.audit.callerSystem'), sub: t('pages.projectDetail.audit.callerSystemSub') },
])

const resourceDdOptions = computed<AuditDdOption[]>(() => [
  { value: '', label: t('pages.projectDetail.audit.filterAll'), dot: '' },
  ...resourceOptions.value.map((r) => {
    const value = r.resource || [r.resourceType, r.resourceId].filter(Boolean).join('/')
    return {
      value,
      label: value,
      sub: r.resourceType || '',
      short: value,
      dot: resDot(value),
    }
  }),
])

const timeDdOptions = computed<AuditDdOption[]>(() => [
  { value: '24h', label: t('pages.projectDetail.audit.time24h') },
  { value: '7d', label: t('pages.projectDetail.audit.time7d') },
  { value: '30d', label: t('pages.projectDetail.audit.time30d') },
])

const pageSizeOptions = computed<AuditDdOption[]>(() => [
  { value: '5', label: '5' },
  { value: '10', label: '10' },
  { value: '20', label: '20' },
])

type Chip = { key: string; label: string; value: string; clearable: boolean }

const chips = computed<Chip[]>(() => {
  const list: Chip[] = []
  if (mode.value === 'run') {
    if (runId.value) {
      const hit = runOptions.value.find((r) => r.runId === runId.value)
      list.push({
        key: 'run',
        label: 'Run',
        value: hit?.label || shortRun(runId.value),
        clearable: false,
      })
    }
    if (nodeId.value) {
      list.push({
        key: 'node',
        label: t('pages.projectDetail.audit.colNode'),
        value: NODE_LABEL[nodeId.value] || nodeId.value,
        clearable: true,
      })
    }
  } else if (callerKind.value) {
    list.push({
      key: 'caller',
      label: t('pages.projectDetail.audit.colCaller'),
      value: t(CALLER_LABEL_KEYS[callerKind.value] || '') || callerKind.value,
      clearable: true,
    })
  }
  if (resource.value) {
    list.push({
      key: 'resource',
      label: t('pages.projectDetail.audit.colResource'),
      value: resource.value,
      clearable: true,
    })
  }
  if (search.value.trim()) {
    list.push({
      key: 'search',
      label: t('pages.projectDetail.audit.filterSearch'),
      value: search.value.trim(),
      clearable: true,
    })
  }
  if (timeWindow.value !== '24h') {
    const lab =
      timeWindow.value === '7d'
        ? t('pages.projectDetail.audit.time7d')
        : t('pages.projectDetail.audit.time30d')
    list.push({
      key: 'time',
      label: t('pages.projectDetail.audit.filterTime'),
      value: lab,
      clearable: true,
    })
  }
  return list
})

const clearableChips = computed(() => chips.value.filter((c) => c.clearable))
const noRuns = computed(() => mode.value === 'run' && initialized.value && runOptions.value.length === 0)

const timeWindowLabel = computed(() => {
  if (timeWindow.value === '7d') return t('pages.projectDetail.audit.time7d')
  if (timeWindow.value === '30d') return t('pages.projectDetail.audit.time30d')
  return t('pages.projectDetail.audit.time24h')
})

/** One-line mobile filter summary (plan g2.1/g2.2); not the chips row. */
const filterSummaryText = computed(() => {
  const all = t('pages.projectDetail.audit.filterAll')
  const resLab = resource.value || all
  const timeLab = timeWindowLabel.value
  if (mode.value === 'run') {
    const runLab = runId.value
      ? runOptions.value.find((r) => r.runId === runId.value)?.label || shortRun(runId.value)
      : t('pages.projectDetail.audit.noRun')
    const nodeLab = nodeId.value ? NODE_LABEL[nodeId.value] || nodeId.value : all
    return `Run · ${runLab} · ${timeLab} · ${t('pages.projectDetail.audit.colNode')} ${nodeLab} / ${t('pages.projectDetail.audit.colResource')} ${resLab}`
  }
  const callerLab = callerKind.value
    ? t(CALLER_LABEL_KEYS[callerKind.value] || '') || callerKind.value
    : all
  return `${t('pages.projectDetail.audit.colCaller')} · ${callerLab} · ${timeLab} · ${t('pages.projectDetail.audit.colResource')} ${resLab}`
})

function toggleFiltersExpanded() {
  filtersExpanded.value = !filtersExpanded.value
}

function resDot(v: string) {
  if (!v) return ''
  if (v.startsWith('mcp/') || v === 'mcp') return 'mcp'
  if (v.startsWith('run/')) return 'run'
  if (v.startsWith('gate/')) return 'gate'
  if (v.startsWith('workflow/')) return 'wf'
  if (v.startsWith('project/')) return 'prj'
  if (v.startsWith('audit/')) return 'aud'
  return ''
}

function resGroup(o: AuditDdOption) {
  if (!o.value) return t('pages.projectDetail.audit.filterAll')
  const i = o.value.indexOf('/')
  return i > 0 ? o.value.slice(0, i) : 'other'
}

function shortRun(id: string) {
  const s = id.replace(/^run-/, '')
  return s.length > 8 ? s.slice(0, 8) : s
}

function actionLabel(action: string) {
  const known: Record<string, string> = {
    'mcp.call': t('pages.projectDetail.audit.actionMcp'),
    'run.start': t('pages.projectDetail.audit.actionRun'),
    'run.cancel': t('pages.projectDetail.audit.actionRun'),
    'run.completed': t('pages.projectDetail.audit.actionRun'),
    'run.failed': t('pages.projectDetail.audit.actionRun'),
    'run.cancelled': t('pages.projectDetail.audit.actionRun'),
    'gate.decide': t('pages.projectDetail.audit.actionGate'),
    'workflow.create': t('pages.projectDetail.audit.actionWorkflow'),
    'workflow.update': t('pages.projectDetail.audit.actionWorkflow'),
    'workflow.delete': t('pages.projectDetail.audit.actionWorkflow'),
    'workflow.publish': t('pages.projectDetail.audit.actionWorkflow'),
    'project.config': t('pages.projectDetail.audit.actionProjectConfig'),
    'audit.export': t('pages.projectDetail.audit.actionExport'),
  }
  if (known[action]) return known[action]
  const prefix = action.split('.')[0]
  const byPrefix: Record<string, string> = {
    mcp: t('pages.projectDetail.audit.actionMcp'),
    run: t('pages.projectDetail.audit.actionRun'),
    gate: t('pages.projectDetail.audit.actionGate'),
    workflow: t('pages.projectDetail.audit.actionWorkflow'),
    project: t('pages.projectDetail.audit.actionProjectConfig'),
    audit: t('pages.projectDetail.audit.actionExport'),
  }
  return byPrefix[prefix || ''] || action
}

function actionClass(action: string) {
  return ACTION_CLASS[action] || 'run'
}

function callerLabel(ev: ProjectAuditEvent) {
  const kind = ev.callerKind || (ev.unattributable || ev.actor === 'system' ? 'system' : 'pm')
  const key = CALLER_LABEL_KEYS[kind]
  return key ? t(key) : kind
}

function nodeLabel(id?: string) {
  if (!id) return '—'
  return NODE_LABEL[id] || id
}

function outcomeLabel(ev: ProjectAuditEvent) {
  return ev.outcome === 'fail'
    ? t('pages.projectDetail.audit.outcomeFail')
    : t('pages.projectDetail.audit.outcomeOk')
}

function resourceText(ev: ProjectAuditEvent) {
  return ev.resource || [ev.resourceType, ev.resourceId].filter(Boolean).join('/') || '—'
}

function prettyPayload(payload: Record<string, unknown> | undefined) {
  return prettyAuditPayload(payload)
}

function buildParams(extra?: { page?: number }) {
  const params: Record<string, string | number | undefined> = {
    time: timeWindow.value,
    resource: resource.value || undefined,
    search: search.value.trim() || undefined,
    page: extra?.page ?? page.value,
    pageSize: pageSize.value,
  }
  if (mode.value === 'run') {
    params.runId = runId.value || undefined
    params.nodeId = nodeId.value || undefined
  } else {
    params.callerKind = callerKind.value || undefined
  }
  return params
}

async function loadFacets(selectedRun?: string) {
  if (props.forceDenied || denied.value) {
    runOptions.value = []
    nodeOptions.value = []
    resourceOptions.value = []
    return
  }
  try {
    const res = await api.listProjectAuditFacets(props.projectId, {
      time: timeWindow.value,
      runId: selectedRun || (mode.value === 'run' ? runId.value || undefined : undefined),
    })
    runOptions.value = Array.isArray(res.runs) ? res.runs : []
    nodeOptions.value = Array.isArray(res.nodes) ? res.nodes : []
    resourceOptions.value = Array.isArray(res.resources) ? res.resources : []
  } catch (e: any) {
    if (e?.status === 403) {
      denied.value = true
      runOptions.value = []
      nodeOptions.value = []
      resourceOptions.value = []
      return
    }
  }
}

async function load(resetPage = false) {
  if (props.forceDenied) {
    denied.value = true
    events.value = []
    total.value = 0
    stats.value = { total: 0, mcp: 0, fail: 0 }
    return
  }
  if (mode.value === 'run' && !runId.value) {
    events.value = []
    total.value = 0
    stats.value = { total: 0, mcp: 0, fail: 0 }
    loading.value = false
    return
  }
  if (resetPage) page.value = 1
  loading.value = true
  denied.value = false
  try {
    const res = await api.listProjectAudit(props.projectId, buildParams())
    const pageData: PaginatedResponse<ProjectAuditEvent> & { stats?: ProjectAuditStats } = isPaginated(res)
      ? res
      : {
          items: res as ProjectAuditEvent[],
          total: (res as ProjectAuditEvent[]).length,
          page: 1,
          pageSize: pageSize.value,
          hasMore: false,
        }
    events.value = pageData.items || []
    total.value = pageData.total
    page.value = pageData.page
    stats.value = pageData.stats || {
      total: pageData.total,
      mcp: events.value.filter((e) => e.action.startsWith('mcp')).length,
      fail: events.value.filter((e) => e.outcome === 'fail').length,
    }
  } catch (e: any) {
    if (e?.status === 403) {
      denied.value = true
      events.value = []
      total.value = 0
      return
    }
    toast.error(e?.message || t('pages.projectDetail.audit.loadFailed'))
  } finally {
    loading.value = false
  }
}

async function bootstrap() {
  if (props.forceDenied) {
    denied.value = true
    initialized.value = true
    return
  }
  loading.value = true
  await loadFacets()
  if (runOptions.value.length > 0) {
    mode.value = 'run'
    runId.value = runOptions.value[0]!.runId
    await loadFacets(runId.value)
  } else {
    // No runs in window — stay in run mode with empty state; user can switch to all.
    mode.value = 'run'
    runId.value = ''
  }
  initialized.value = true
  await load(true)
}

async function setMode(next: Mode) {
  if (mode.value === next) return
  mode.value = next
  openId.value = null
  page.value = 1
  nodeId.value = ''
  callerKind.value = ''
  resource.value = ''
  if (next === 'run') {
    await loadFacets()
    if (!runId.value && runOptions.value.length) {
      runId.value = runOptions.value[0]!.runId
    }
    if (runId.value) await loadFacets(runId.value)
  } else {
    await loadFacets()
  }
  await load(true)
}

async function onRunChange(v: string) {
  runId.value = v
  nodeId.value = ''
  resource.value = ''
  openId.value = null
  await loadFacets(v)
  await load(true)
}

function onFilterChange() {
  openId.value = null
  void load(true)
}

let searchTimer: ReturnType<typeof setTimeout> | null = null
function onSearchInput() {
  if (searchTimer) clearTimeout(searchTimer)
  searchTimer = setTimeout(() => {
    openId.value = null
    void load(true)
  }, 200)
}

function clearChip(key: string) {
  if (key === 'node') nodeId.value = ''
  if (key === 'caller') callerKind.value = ''
  if (key === 'resource') resource.value = ''
  if (key === 'search') search.value = ''
  if (key === 'time') timeWindow.value = '24h'
  void (async () => {
    await loadFacets(mode.value === 'run' ? runId.value : undefined)
    await load(true)
  })()
}

function clearOptional() {
  nodeId.value = ''
  callerKind.value = ''
  resource.value = ''
  search.value = ''
  timeWindow.value = '24h'
  void (async () => {
    await loadFacets(mode.value === 'run' ? runId.value : undefined)
    await load(true)
  })()
}

function toggleOpen(id: string) {
  openId.value = openId.value === id ? null : id
}

async function exportAudit() {
  if (denied.value || props.forceDenied) {
    toast.error(t('pages.projectDetail.audit.exportDenied'))
    return
  }
  try {
    const url = api.exportProjectAuditUrl(props.projectId, {
      format: 'json',
      time: timeWindow.value,
      callerKind: mode.value === 'all' ? callerKind.value || undefined : undefined,
      resource: resource.value || undefined,
      runId: mode.value === 'run' ? runId.value || undefined : undefined,
      nodeId: mode.value === 'run' ? nodeId.value || undefined : undefined,
      search: search.value.trim() || undefined,
    })
    const a = document.createElement('a')
    a.href = url
    a.download = ''
    a.rel = 'noopener'
    document.body.appendChild(a)
    a.click()
    a.remove()
    toast.success(t('pages.projectDetail.audit.exportStarted', { format: 'JSON' }))
    setTimeout(() => void load(true), 400)
  } catch (e: any) {
    toast.error(e?.message || t('pages.projectDetail.audit.exportFailed'))
  }
}

function goPage(p: number) {
  const max = pageCount.value
  page.value = Math.max(1, Math.min(max, p))
  openId.value = null
  void load()
}

function onPageSizeChange(v: string) {
  pageSize.value = Number(v) || 10
  openId.value = null
  void load(true)
}

async function onTimeChange(v: string) {
  timeWindow.value = v as '24h' | '7d' | '30d'
  openId.value = null
  await loadFacets(mode.value === 'run' ? runId.value : undefined)
  if (mode.value === 'run' && runId.value && !runOptions.value.some((r) => r.runId === runId.value)) {
    runId.value = runOptions.value[0]?.runId || ''
    if (runId.value) await loadFacets(runId.value)
  }
  await load(true)
}

watch(
  () => [props.projectId, props.forceDenied],
  () => {
    initialized.value = false
    void bootstrap()
  },
)

onMounted(() => {
  void bootstrap()
})
</script>

<template>
  <div class="audit-panel" data-testid="project-audit-panel">
    <div
      v-if="denied || forceDenied"
      class="flex flex-1 flex-col items-center justify-center gap-2 border border-dashed border-line bg-surface px-6 py-16 text-center"
      data-testid="project-audit-denied"
    >
      <div class="text-sm font-semibold text-txt">{{ t('pages.projectDetail.audit.deniedTitle') }}</div>
      <p class="max-w-md text-[13px] text-txt3">{{ t('pages.projectDetail.audit.deniedDesc') }}</p>
    </div>

    <template v-else>
      <div class="panel-hd">
        <h4>{{ t('pages.projectDetail.audit.title') }}</h4>
        <div class="seg" role="tablist" data-testid="project-audit-mode">
          <button
            type="button"
            :class="{ on: mode === 'run' }"
            data-testid="project-audit-mode-run"
            @click="setMode('run')"
          >
            {{ t('pages.projectDetail.audit.modeRun') }}
          </button>
          <button
            type="button"
            :class="{ on: mode === 'all' }"
            data-testid="project-audit-mode-all"
            @click="setMode('all')"
          >
            {{ t('pages.projectDetail.audit.modeAll') }}
          </button>
        </div>
      </div>

      <div class="filters" :class="{ 'filters-mobile': isMobile }" data-testid="project-audit-filters">
        <!-- Mobile: collapsible filter summary (plan g2); search/export stay outside fold -->
        <template v-if="isMobile">
          <button
            type="button"
            class="filter-summary"
            data-testid="project-audit-filter-summary"
            :aria-expanded="filtersExpanded ? 'true' : 'false'"
            @click="toggleFiltersExpanded"
          >
            <span class="sum-text">{{ filterSummaryText }}</span>
            <span class="sum-action">
              {{
                filtersExpanded
                  ? t('pages.projectDetail.audit.filterCollapse')
                  : t('pages.projectDetail.audit.filterExpand')
              }}
            </span>
          </button>

          <div
            v-show="filtersExpanded"
            class="filters-editor"
            data-testid="project-audit-filters-editor"
          >
            <AuditFilterDropdown
              v-if="mode === 'run'"
              :model-value="runId"
              :label-key="'Run'"
              :options="runDdOptions"
              :searchable="true"
              :block="true"
              :empty-label="t('pages.projectDetail.audit.noRun')"
              test-id="project-audit-run"
              @update:model-value="onRunChange"
            />
            <AuditFilterDropdown
              v-if="mode === 'run'"
              :model-value="nodeId"
              :label-key="t('pages.projectDetail.audit.colNode')"
              :options="nodeDdOptions"
              :searchable="true"
              :block="true"
              :empty-label="t('pages.projectDetail.audit.filterAll')"
              test-id="project-audit-node"
              @update:model-value="(v) => { nodeId = v; onFilterChange() }"
            />
            <AuditFilterDropdown
              v-if="mode === 'all'"
              :model-value="callerKind"
              :label-key="t('pages.projectDetail.audit.colCaller')"
              :options="callerDdOptions"
              :block="true"
              :empty-label="t('pages.projectDetail.audit.filterAll')"
              test-id="project-audit-caller"
              @update:model-value="(v) => { callerKind = v; onFilterChange() }"
            />
            <AuditFilterDropdown
              :model-value="resource"
              :label-key="t('pages.projectDetail.audit.colResource')"
              :options="resourceDdOptions"
              :searchable="true"
              :block="true"
              :empty-label="t('pages.projectDetail.audit.filterAll')"
              :group-by="resGroup"
              test-id="project-audit-resource"
              @update:model-value="(v) => { resource = v; onFilterChange() }"
            />
            <AuditFilterDropdown
              :model-value="timeWindow"
              :label-key="t('pages.projectDetail.audit.filterTime')"
              :options="timeDdOptions"
              :block="true"
              test-id="project-audit-time"
              @update:model-value="onTimeChange"
            />
          </div>

          <div class="search">
            <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <circle cx="11" cy="11" r="7" />
              <path d="M20 20l-3-3" />
            </svg>
            <input
              v-model="search"
              type="search"
              :placeholder="t('pages.projectDetail.audit.searchPlaceholder')"
              autocomplete="off"
              data-testid="project-audit-search"
              @input="onSearchInput"
            />
          </div>
          <div class="filters-actions">
            <button type="button" class="btn" data-testid="project-audit-export" @click="exportAudit">
              {{ t('pages.projectDetail.audit.export') }}
            </button>
          </div>
        </template>

        <!-- Desktop: horizontal filters + wide table (plan g4.1) -->
        <template v-else>
          <div class="search">
            <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <circle cx="11" cy="11" r="7" />
              <path d="M20 20l-3-3" />
            </svg>
            <input
              v-model="search"
              type="search"
              :placeholder="t('pages.projectDetail.audit.searchPlaceholder')"
              autocomplete="off"
              data-testid="project-audit-search"
              @input="onSearchInput"
            />
          </div>

          <AuditFilterDropdown
            v-if="mode === 'run'"
            :model-value="runId"
            :label-key="'Run'"
            :options="runDdOptions"
            :searchable="true"
            :width="280"
            :empty-label="t('pages.projectDetail.audit.noRun')"
            test-id="project-audit-run"
            @update:model-value="onRunChange"
          />
          <AuditFilterDropdown
            v-if="mode === 'run'"
            :model-value="nodeId"
            :label-key="t('pages.projectDetail.audit.colNode')"
            :options="nodeDdOptions"
            :searchable="true"
            :empty-label="t('pages.projectDetail.audit.filterAll')"
            test-id="project-audit-node"
            @update:model-value="(v) => { nodeId = v; onFilterChange() }"
          />
          <AuditFilterDropdown
            v-if="mode === 'all'"
            :model-value="callerKind"
            :label-key="t('pages.projectDetail.audit.colCaller')"
            :options="callerDdOptions"
            :empty-label="t('pages.projectDetail.audit.filterAll')"
            test-id="project-audit-caller"
            @update:model-value="(v) => { callerKind = v; onFilterChange() }"
          />
          <AuditFilterDropdown
            :model-value="resource"
            :label-key="t('pages.projectDetail.audit.colResource')"
            :options="resourceDdOptions"
            :searchable="true"
            :width="280"
            :empty-label="t('pages.projectDetail.audit.filterAll')"
            :group-by="resGroup"
            test-id="project-audit-resource"
            @update:model-value="(v) => { resource = v; onFilterChange() }"
          />
          <AuditFilterDropdown
            :model-value="timeWindow"
            :label-key="t('pages.projectDetail.audit.filterTime')"
            :options="timeDdOptions"
            test-id="project-audit-time"
            @update:model-value="onTimeChange"
          />

          <div class="filters-actions">
            <button type="button" class="btn" data-testid="project-audit-export" @click="exportAudit">
              {{ t('pages.projectDetail.audit.export') }}
            </button>
          </div>
        </template>
      </div>

      <div v-if="!isMobile && chips.length" class="chips" data-testid="project-audit-chips">
        <span v-for="ch in chips" :key="ch.key" class="chip">
          <em>{{ ch.label }}</em>
          <b>{{ ch.value }}</b>
          <button
            v-if="ch.clearable"
            type="button"
            class="x"
            :data-testid="`project-audit-chip-clear-${ch.key}`"
            @click="clearChip(ch.key)"
          >
            ×
          </button>
        </span>
        <button
          v-if="clearableChips.length"
          type="button"
          class="linkish"
          data-testid="project-audit-clear"
          @click="clearOptional"
        >
          {{ t('pages.projectDetail.audit.clearFilters') }}
        </button>
      </div>

      <div v-if="!isMobile" class="meta">
        <div class="meta-l" data-testid="project-audit-stats">
          <span>{{ t('pages.projectDetail.audit.statTotal') }} <b>{{ stats.total }}</b></span>
          <span>MCP <b>{{ stats.mcp }}</b></span>
          <span>{{ t('pages.projectDetail.audit.statFail') }} <b>{{ stats.fail }}</b></span>
          <span v-if="mode === 'run' && runId">Run <b>{{ shortRun(runId) }}</b></span>
        </div>
        <div>{{ t('pages.projectDetail.audit.expandHint') }}</div>
      </div>
      <div v-else class="meta meta-mobile" data-testid="project-audit-stats">
        <span>{{ t('pages.projectDetail.audit.statTotal') }} <b>{{ stats.total }}</b></span>
        <span>MCP <b>{{ stats.mcp }}</b></span>
        <span>{{ t('pages.projectDetail.audit.statFail') }} <b>{{ stats.fail }}</b></span>
        <span v-if="mode === 'run' && runId">Run <b>{{ shortRun(runId) }}</b></span>
      </div>

      <div v-if="loading" class="py-10 text-center text-[13px] text-txt3">
        {{ t('pages.projectDetail.audit.loading') }}
      </div>
      <div
        v-else-if="noRuns"
        class="empty"
        data-testid="project-audit-empty-runs"
      >
        <div class="big">{{ t('pages.projectDetail.audit.emptyRunsTitle') }}</div>
        <div>
          {{ t('pages.projectDetail.audit.emptyRunsDesc') }}
          <button type="button" class="linkish" @click="setMode('all')">
            {{ t('pages.projectDetail.audit.modeAll') }}
          </button>
        </div>
      </div>
      <EmptyState
        v-else-if="!events.length"
        data-testid="project-audit-empty"
        :title="t('pages.projectDetail.audit.emptyTitle')"
        :desc="t('pages.projectDetail.audit.emptyDesc')"
      />
      <!-- Mobile event cards (plan g3); shared by run + all modes -->
      <div
        v-else-if="isMobile"
        class="event-cards"
        data-testid="project-audit-list"
        data-layout="cards"
      >
        <button
          v-for="ev in events"
          :key="ev.id"
          type="button"
          class="event-card"
          :class="{ open: openId === ev.id, fail: ev.outcome === 'fail' }"
          :data-testid="`project-audit-event-${ev.id}`"
          @click="toggleOpen(ev.id)"
        >
          <div class="ec-top">
            <span class="ec-time mono">{{ fmtTime(ev.occurredAt) }}</span>
            <span :class="ev.outcome === 'fail' ? 'bad' : 'ok'">{{ outcomeLabel(ev) }}</span>
          </div>
          <div class="ec-row">
            <span class="k">{{ t('pages.projectDetail.audit.colNode') }}</span>
            {{ nodeLabel(ev.nodeId) }}
            ·
            <span class="k">{{ t('pages.projectDetail.audit.colAction') }}</span>
            <span class="act" :class="actionClass(ev.action)">{{ actionLabel(ev.action) }}</span>
          </div>
          <div class="ec-sum" :class="{ open: openId === ev.id }">{{ ev.summary }}</div>
          <div v-if="openId === ev.id" class="ec-detail" @click.stop>
            <div>
              <span class="k">{{ t('pages.projectDetail.audit.colCaller') }}</span>
              {{ callerLabel(ev) }}
            </div>
            <div>
              <span class="k">{{ t('pages.projectDetail.audit.colResource') }}</span>
              {{ resourceText(ev) }}
            </div>
            <div v-if="mode === 'all' && ev.runId">
              <span class="k">{{ t('pages.projectDetail.audit.colRun') }}</span>
              <code>{{ ev.runId }}</code>
            </div>
            <div>
              <span class="k">action</span>
              <code>{{ ev.action }}</code>
            </div>
            <pre
              class="payload"
              data-testid="project-audit-payload"
              v-html="prettyPayload(ev.payload)"
            />
          </div>
          <div class="ec-hint">
            {{
              openId === ev.id
                ? t('pages.projectDetail.audit.cardCollapseHint')
                : t('pages.projectDetail.audit.cardExpandHint')
            }}
          </div>
        </button>
      </div>
      <!-- Desktop wide table (plan g4.1) -->
      <div
        v-else
        class="table-wrap"
        :class="{ 'col-run-hide': mode === 'run', 'col-node-hide': mode !== 'run' }"
        data-testid="project-audit-list"
        data-layout="table"
      >
        <table>
          <thead>
            <tr>
              <th style="width: 148px">{{ t('pages.projectDetail.audit.colTime') }}</th>
              <th class="col-node" style="width: 88px">{{ t('pages.projectDetail.audit.colNode') }}</th>
              <th style="width: 100px">{{ t('pages.projectDetail.audit.colCaller') }}</th>
              <th style="width: 88px">{{ t('pages.projectDetail.audit.colAction') }}</th>
              <th style="width: 180px">{{ t('pages.projectDetail.audit.colResource') }}</th>
              <th class="col-run" style="width: 88px">{{ t('pages.projectDetail.audit.colRun') }}</th>
              <th style="width: 56px">{{ t('pages.projectDetail.audit.colOutcome') }}</th>
              <th>{{ t('pages.projectDetail.audit.colSummary') }}</th>
            </tr>
          </thead>
          <tbody>
            <template v-for="ev in events" :key="ev.id">
              <tr
                class="row"
                :class="{ open: openId === ev.id, fail: ev.outcome === 'fail' }"
                :data-testid="`project-audit-event-${ev.id}`"
                @click="toggleOpen(ev.id)"
              >
                <td>
                  <div class="time-main mono">{{ fmtTime(ev.occurredAt) }}</div>
                </td>
                <td class="col-node">
                  <span class="node">{{ nodeLabel(ev.nodeId) }}</span>
                </td>
                <td><span class="who">{{ callerLabel(ev) }}</span></td>
                <td>
                  <span class="act" :class="actionClass(ev.action)">{{ actionLabel(ev.action) }}</span>
                </td>
                <td><span class="res">{{ resourceText(ev) }}</span></td>
                <td class="col-run mono">{{ ev.runId ? shortRun(ev.runId) : '—' }}</td>
                <td>
                  <span :class="ev.outcome === 'fail' ? 'bad' : 'ok'">{{ outcomeLabel(ev) }}</span>
                </td>
                <td class="summary">{{ ev.summary }}</td>
              </tr>
              <tr v-if="openId === ev.id" class="detail">
                <td colspan="8">
                  <div class="detail-inner" @click.stop>
                    <div class="detail-meta">
                      <span>action <code>{{ ev.action }}</code></span>
                      <span v-if="ev.nodeId">{{ t('pages.projectDetail.audit.colNode') }} <code>{{ ev.nodeId }}</code></span>
                      <span>{{ t('pages.projectDetail.audit.colResource') }} <code>{{ resourceText(ev) }}</code></span>
                      <span v-if="ev.runId">Run <code>{{ ev.runId }}</code></span>
                    </div>
                    <pre
                      class="payload"
                      data-testid="project-audit-payload"
                      v-html="prettyPayload(ev.payload)"
                    />
                  </div>
                </td>
              </tr>
            </template>
          </tbody>
        </table>
      </div>

      <div v-if="!noRuns" class="pager">
        <span data-testid="project-audit-pager-info">
          <template v-if="total">
            <b>{{ (page - 1) * pageSize + 1 }}-{{ Math.min(page * pageSize, total) }}</b>
            /
            <b>{{ total }}</b>
          </template>
          <template v-else>{{ t('pages.projectDetail.audit.statTotal') }} <b>0</b></template>
        </span>
        <div class="pager-btns">
          <button type="button" class="btn" :disabled="page <= 1 || loading" @click="goPage(page - 1)">
            {{ t('common.pagination.prev') }}
          </button>
          <button type="button" class="btn" :disabled="!hasMore || loading" @click="goPage(page + 1)">
            {{ t('common.pagination.next') }}
          </button>
        </div>
        <div class="pager-size">
          <span>{{ t('pages.projectDetail.audit.perPage') }}</span>
          <AuditFilterDropdown
            :model-value="String(pageSize)"
            :options="pageSizeOptions"
            :width="80"
            :right="true"
            test-id="project-audit-page-size"
            @update:model-value="onPageSizeChange"
          />
        </div>
      </div>
    </template>
  </div>
</template>

<style scoped>
.audit-panel {
  display: flex;
  flex-direction: column;
  min-height: 420px;
  border: 1px solid var(--line, #ececef);
  background: var(--card, #fff);
  overflow: hidden;
}
.panel-hd {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  flex-wrap: wrap;
  padding: 14px 16px 12px;
  border-bottom: 1px solid var(--line, #ececef);
}
.panel-hd h4 {
  margin: 0;
  font-size: 15px;
  font-weight: 650;
  letter-spacing: -0.02em;
  color: var(--txt, #18181b);
}
.seg {
  display: inline-flex;
  background: #f4f4f5;
  padding: 3px;
  gap: 2px;
}
.seg button {
  border: 0;
  background: transparent;
  height: 28px;
  padding: 0 12px;
  font: inherit;
  font-size: 12px;
  color: var(--txt3, #71717a);
  cursor: pointer;
  font-weight: 500;
}
.seg button:hover {
  color: var(--txt, #18181b);
}
.seg button.on {
  background: #fff;
  color: var(--txt, #18181b);
  font-weight: 600;
  box-shadow: 0 1px 2px rgba(0, 0, 0, 0.06);
}
.filters {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
  padding: 12px 16px;
  border-bottom: 1px solid var(--line, #ececef);
}
.search {
  flex: 1 1 180px;
  min-width: 140px;
  max-width: 280px;
  height: 32px;
  display: flex;
  align-items: center;
  gap: 8px;
  border: 1px solid #e4e4e7;
  background: #fff;
  padding: 0 10px;
}
.search:focus-within {
  border-color: #c4b5fd;
  box-shadow: 0 0 0 3px rgba(124, 58, 237, 0.1);
}
.search svg {
  opacity: 0.4;
  flex: 0 0 auto;
}
.search input {
  flex: 1;
  border: 0;
  outline: none;
  font: inherit;
  background: transparent;
  min-width: 0;
  height: 30px;
  font-size: 12px;
}
.search input::placeholder {
  color: #a1a1aa;
}
.filters-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-left: auto;
}
.btn {
  height: 32px;
  padding: 0 12px;
  border: 1px solid #e4e4e7;
  background: #fff;
  color: var(--txt, #18181b);
  font: inherit;
  font-size: 12px;
  font-weight: 500;
  cursor: pointer;
  white-space: nowrap;
}
.btn:hover:not(:disabled) {
  background: #fafafa;
  border-color: #d4d4d8;
}
.btn:disabled {
  opacity: 0.35;
  cursor: not-allowed;
}
.chips {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  align-items: center;
  padding: 0 16px 12px;
  border-bottom: 1px solid var(--line, #ececef);
}
.chip {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  height: 24px;
  padding: 0 4px 0 8px;
  background: #f4f4f5;
  border: 1px solid #e4e4e7;
  font-size: 11px;
  color: var(--txt, #18181b);
}
.chip em {
  font-style: normal;
  color: var(--txt3, #71717a);
  font-weight: 500;
}
.chip b {
  font-weight: 600;
}
.chip .x {
  width: 18px;
  height: 18px;
  border: 0;
  background: transparent;
  color: var(--txt3, #71717a);
  cursor: pointer;
  font-size: 13px;
  line-height: 1;
}
.chip .x:hover {
  color: var(--txt, #18181b);
  background: #e4e4e7;
}
.linkish {
  border: 0;
  background: transparent;
  color: var(--txt3, #71717a);
  font: inherit;
  font-size: 11px;
  cursor: pointer;
  padding: 0 4px;
}
.linkish:hover {
  color: var(--accent, #7c3aed);
}
.meta {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
  padding: 8px 16px;
  font-size: 12px;
  color: var(--txt3, #71717a);
  border-bottom: 1px solid var(--line, #ececef);
}
.meta b {
  color: var(--txt, #18181b);
  font-weight: 600;
  font-variant-numeric: tabular-nums;
}
.meta-l {
  display: flex;
  gap: 12px;
  flex-wrap: wrap;
  align-items: center;
}
.meta-mobile {
  gap: 10px;
  padding: 6px 16px;
}
.meta-mobile b {
  color: var(--txt, #18181b);
  font-weight: 600;
  font-variant-numeric: tabular-nums;
}
.table-wrap {
  overflow: auto;
  max-height: 520px;
}
table {
  width: 100%;
  border-collapse: collapse;
  min-width: 820px;
}
thead th {
  position: sticky;
  top: 0;
  z-index: 1;
  text-align: left;
  padding: 9px 14px;
  background: #fafafa;
  color: var(--txt3, #71717a);
  font-size: 11px;
  font-weight: 600;
  border-bottom: 1px solid var(--line, #ececef);
}
td {
  padding: 11px 14px;
  border-bottom: 1px solid var(--line, #ececef);
  vertical-align: middle;
  font-size: 13px;
}
tr.row {
  cursor: pointer;
}
tr.row:hover td {
  background: #fafafa;
}
tr.row.open td {
  background: #f5f3ff;
}
tr.row.fail td:first-child {
  box-shadow: inset 2px 0 0 #dc2626;
}
tr.detail td {
  padding: 0;
  background: #fafafa;
  border-bottom: 1px solid var(--line, #ececef);
}
.detail-inner {
  padding: 12px 14px 14px;
  margin-left: 2px;
  border-left: 2px solid var(--accent, #7c3aed);
}
.detail-meta {
  font-size: 12px;
  color: var(--txt3, #71717a);
  margin-bottom: 8px;
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
}
.detail-meta code {
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font-size: 11px;
  color: var(--txt, #18181b);
  background: #fff;
  border: 1px solid var(--line, #ececef);
  padding: 1px 5px;
}
.payload {
  margin: 0;
  padding: 10px 12px;
  border: 1px solid var(--line, #ececef);
  background: #fff;
  font: 11px/1.5 ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  white-space: pre-wrap;
  word-break: break-word;
  color: #3f3f46;
  overflow-x: auto;
}
.mono {
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font-size: 12px;
  color: #52525b;
}
.time-main {
  font-variant-numeric: tabular-nums;
  color: #3f3f46;
  font-size: 12px;
}
.who {
  color: var(--txt, #18181b);
  font-weight: 500;
}
.act {
  font-size: 12px;
  font-weight: 500;
}
.act.mcp {
  color: #7c3aed;
}
.act.run {
  color: #52525b;
}
.act.gate {
  color: #b45309;
}
.act.cfg {
  color: #2563eb;
}
.act.exp {
  color: #db2777;
}
.res {
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font-size: 12px;
  color: #71717a;
}
.node {
  color: #52525b;
  font-size: 12px;
}
.ok {
  color: #16a34a;
  font-size: 12px;
  font-weight: 500;
}
.bad {
  color: #dc2626;
  font-size: 12px;
  font-weight: 500;
}
.summary {
  color: #3f3f46;
}
.empty {
  padding: 48px 16px;
  text-align: center;
  color: var(--txt3, #71717a);
}
.empty .big {
  font-size: 14px;
  font-weight: 600;
  color: var(--txt, #18181b);
  margin-bottom: 4px;
}
.col-run-hide :deep(.col-run),
.col-node-hide :deep(.col-node) {
  display: none;
}
.col-run-hide .col-run,
.col-node-hide .col-node {
  display: none;
}
.pager {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  flex-wrap: wrap;
  padding: 10px 16px;
  border-top: 1px solid var(--line, #ececef);
  font-size: 12px;
  color: var(--txt3, #71717a);
}
.pager b {
  color: var(--txt, #18181b);
  font-weight: 600;
  font-variant-numeric: tabular-nums;
}
.pager-btns {
  display: flex;
  align-items: center;
  gap: 2px;
}
.pager-btns .btn {
  height: 28px;
  min-width: 28px;
  padding: 0 9px;
  border-color: transparent;
  background: transparent;
}
.pager-btns .btn:hover:not(:disabled) {
  background: #f4f4f5;
}
.pager-size {
  display: flex;
  align-items: center;
  gap: 6px;
}
.filter-summary {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
  min-height: 44px;
  border: 1px solid #e4e4e7;
  background: #fafafa;
  padding: 10px 12px;
  font: inherit;
  text-align: left;
  cursor: pointer;
  user-select: none;
}
.filter-summary .sum-text {
  flex: 1;
  min-width: 0;
  font-size: 12px;
  color: var(--txt, #18181b);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.filter-summary .sum-action {
  flex: 0 0 auto;
  font-size: 11px;
  font-weight: 600;
  color: var(--accent, #7c3aed);
  white-space: nowrap;
}
.filters-editor {
  display: flex;
  flex-direction: column;
  gap: 8px;
  width: 100%;
  /* 与搜索同宽（f1/g2.3）：仅纵向内边距，避免左右 inset 缩窄触发器 */
  padding: 10px 0;
  border-top: 1px solid #eee;
  border-bottom: 1px solid #eee;
  background: #fcfcfc;
}
.filters-mobile {
  flex-direction: column;
  align-items: stretch;
}
.filters-mobile .search {
  max-width: none;
  width: 100%;
  flex: 0 0 auto;
  height: 44px;
  min-height: 44px;
}
.filters-mobile .search input {
  height: 42px;
}
.filters-mobile .filters-actions {
  margin-left: 0;
  width: 100%;
  flex: 0 0 auto;
}
.filters-mobile .filters-actions .btn {
  flex: 1;
  width: 100%;
  min-height: 44px;
  height: auto;
}
.filters-mobile .filter-summary {
  flex: 0 0 auto;
}
.event-cards {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 12px 16px;
}
.event-card {
  display: block;
  width: 100%;
  border: 1px solid #e5e5e5;
  background: #fff;
  padding: 10px;
  text-align: left;
  font: inherit;
  color: inherit;
  cursor: pointer;
  transition: border-color 0.15s ease, background 0.15s ease;
}
.event-card:hover {
  border-color: #c4b5fd;
}
.event-card.open {
  border-color: var(--accent, #7c3aed);
  background: #faf8ff;
}
.event-card.fail {
  box-shadow: inset 2px 0 0 #dc2626;
}
.ec-top {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  margin-bottom: 4px;
}
.ec-time {
  font-size: 11px;
  color: #888;
  font-variant-numeric: tabular-nums;
}
.ec-row {
  font-size: 12px;
  color: #333;
  margin-top: 2px;
}
.ec-row .k,
.ec-detail .k {
  color: #888;
}
.ec-sum {
  margin-top: 6px;
  font-size: 12px;
  color: #444;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}
.ec-sum.open {
  display: block;
  -webkit-line-clamp: unset;
}
.ec-detail {
  margin-top: 8px;
  padding-top: 8px;
  border-top: 1px dashed #e5e5e5;
  font-size: 12px;
  color: #444;
}
.ec-detail div {
  margin-top: 3px;
}
.ec-detail code {
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font-size: 11px;
  color: var(--txt, #18181b);
  background: #fff;
  border: 1px solid var(--line, #ececef);
  padding: 1px 5px;
}
.ec-detail .payload {
  margin-top: 8px;
}
.ec-hint {
  margin-top: 6px;
  font-size: 10px;
  color: var(--accent, #7c3aed);
}
@media (max-width: 768px) {
  .filters:not(.filters-mobile) {
    flex-direction: column;
    align-items: stretch;
  }
  .filters:not(.filters-mobile) .search {
    max-width: none;
    width: 100%;
  }
  .filters:not(.filters-mobile) .filters-actions {
    margin-left: 0;
    width: 100%;
  }
  .filters:not(.filters-mobile) .filters-actions .btn {
    flex: 1;
    width: 100%;
  }
}
:deep(.tok-key) {
  color: #7dd3c7;
}
:deep(.audit-mask) {
  color: #b45309;
}
</style>
