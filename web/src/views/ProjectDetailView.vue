<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/ui/Icon.vue'
import AppButton from '@/components/ui/AppButton.vue'
import AppModal from '@/components/ui/AppModal.vue'
import EmptyState from '@/components/ui/EmptyState.vue'
import StatusPill from '@/components/ui/StatusPill.vue'
import RunLaunchModal, { type InputField } from '@/components/workflow/RunLaunchModal.vue'
import CopyWorkflowModal from '@/components/workflow/CopyWorkflowModal.vue'
import ExportVersionModal from '@/components/workflow/ExportVersionModal.vue'
import BoardView from '@/views/BoardView.vue'
import { api } from '@/lib/api'
import { writeStoredProjectId } from '@/lib/useProjectContext'
import { useToast } from '@/lib/useToast'
import { fmtTime } from '@/lib/format'
import { clearRunDraft, mergeRunDraft, saveRunDraft } from '@/lib/runDraft'
import { useBreakpoint } from '@/lib/useBreakpoint'
import { useWorkflowImport } from '@/lib/useWorkflowImport'
import PmLeaderChat from '@/components/pm/PmLeaderChat.vue'
import PmCronJobsPanel from '@/components/pm/PmCronJobsPanel.vue'
import PmSettingsPanel from '@/components/pm/PmSettingsPanel.vue'
import type { ClarifyImage, PmLeaderBinding, Project, ProjectEnvEntry, ProjectVariable, Workflow } from '@/lib/types'

const PROJECT_TABS = [
  'board',
  'workflows',
  'pmLeader',
  'cronJobs',
  'sandboxEnv',
  'variables',
  'meta',
] as const
type Tab = (typeof PROJECT_TABS)[number]
/** Legacy deep-link id; no longer a visible top-bar tab. */
const LEGACY_PM_SETTINGS_TAB = 'pmSettings'
/** Legacy project-memory deep-link; removed tab — fall back to board + migration banner. */
const LEGACY_PM_MEMORY_TAB = 'pmMemory'
type PmView = 'chat' | 'settings'

function isProjectTab(q: unknown): q is Tab {
  return typeof q === 'string' && (PROJECT_TABS as readonly string[]).includes(q)
}

function parseProjectTab(q: unknown): Tab {
  if (q === LEGACY_PM_SETTINGS_TAB) return 'pmLeader'
  if (q === LEGACY_PM_MEMORY_TAB) return 'board'
  if (isProjectTab(q)) return q
  return 'board'
}

const route = useRoute()
const router = useRouter()
const { t } = useI18n()
const toast = useToast()
const { isMobile } = useBreakpoint()
const projectId = computed(() => route.params.id as string)

const project = ref<Project | null>(null)
const workflows = ref<Workflow[]>([])
const loading = ref(true)
const initialLegacyPmSettings = route.query.tab === LEGACY_PM_SETTINGS_TAB
const initialLegacyPmMemory = route.query.tab === LEGACY_PM_MEMORY_TAB
const tab = ref<Tab>(parseProjectTab(route.query.tab))
/** Inline sub-view inside PM Leader; page-local, not a shareable URL param. */
const pmView = ref<PmView>(initialLegacyPmSettings ? 'settings' : 'chat')
/** Show once when landing via legacy ?tab=pmMemory. */
const showPmMemoryMigration = ref(initialLegacyPmMemory)

function setTab(id: Tab) {
  if (id !== 'pmLeader') {
    pmView.value = 'chat'
  }
  tab.value = id
  const nextQuery = { ...route.query, tab: id }
  if (route.query.tab === id) return
  void router.replace({ query: nextQuery })
}

/** After settings, remount chat on mobile in conversation view (not thread list). */
const pmRestoreMobileChat = ref(false)

function openPmSettings() {
  pmView.value = 'settings'
}

function backToPmChat() {
  pmView.value = 'chat'
  pmRestoreMobileChat.value = true
}

/** Reset inline PM view when project context changes; legacy deep-link still opens settings. */
function resetPmViewForProjectContext() {
  pmView.value = route.query.tab === LEGACY_PM_SETTINGS_TAB ? 'settings' : 'chat'
  pmRestoreMobileChat.value = false
  if (route.query.tab === LEGACY_PM_MEMORY_TAB) {
    showPmMemoryMigration.value = true
  }
}

function rewriteLegacyPmSettingsQuery() {
  if (route.query.tab !== LEGACY_PM_SETTINGS_TAB) return
  void router.replace({ query: { ...route.query, tab: 'pmLeader' } })
}

function rewriteLegacyPmMemoryQuery() {
  if (route.query.tab !== LEGACY_PM_MEMORY_TAB) return
  showPmMemoryMigration.value = true
  void router.replace({ query: { ...route.query, tab: 'board' } })
}

function syncTabFromRoute() {
  const q = route.query.tab
  if (q === LEGACY_PM_SETTINGS_TAB) {
    tab.value = 'pmLeader'
    pmView.value = 'settings'
    rewriteLegacyPmSettingsQuery()
    return
  }
  if (q === LEGACY_PM_MEMORY_TAB) {
    tab.value = 'board'
    pmView.value = 'chat'
    rewriteLegacyPmMemoryQuery()
    return
  }
  const next = parseProjectTab(q)
  if (tab.value !== next) {
    if (next !== 'pmLeader') {
      pmView.value = 'chat'
    }
    tab.value = next
  }
}

function ensureTabQuery() {
  if (route.query.tab === LEGACY_PM_SETTINGS_TAB) {
    tab.value = 'pmLeader'
    pmView.value = 'settings'
    rewriteLegacyPmSettingsQuery()
    return
  }
  if (route.query.tab === LEGACY_PM_MEMORY_TAB) {
    tab.value = 'board'
    rewriteLegacyPmMemoryQuery()
    return
  }
  if (isProjectTab(route.query.tab)) {
    return
  }
  void router.replace({ query: { ...route.query, tab: tab.value } })
}

function dismissPmMemoryMigration() {
  showPmMemoryMigration.value = false
}

function goStudioMemory(agent?: string) {
  const name = (agent || pmBinding.value?.agentConfigRef || '').trim()
  if (name) {
    void router.push({ path: '/agents', query: { agent: name, tab: 'data', sub: 'memory' } })
    return
  }
  void router.push({ path: '/agents' })
}
const savingMeta = ref(false)
const savingEnv = ref(false)
const savingVars = ref(false)
const editName = ref('')
const editDesc = ref('')
const envRows = ref<ProjectEnvEntry[]>([])
const varRows = ref<ProjectVariable[]>([])
const showDelete = ref(false)
const deleting = ref(false)
const deleteError = ref('')
const helpOpen = ref(false)

const runTarget = ref<Workflow | null>(null)
const runFields = ref<InputField[]>([])
const runInputs = ref<Record<string, string>>({})
const runImages = ref<Record<string, ClarifyImage[]>>({})
const draftRestored = ref(false)
const openMenuId = ref<string | null>(null)

const deleteWfTarget = ref<Workflow | null>(null)
const deletingWf = ref(false)
const deleteWfError = ref('')

const copyPreviewLoading = ref<string | null>(null)
const copyModal = ref<{ sourceId: string; sourceName: string; suggestedName: string } | null>(null)
const exportTarget = ref<Workflow | null>(null)

const { fileInput, triggerImport, handleFileChange } = useWorkflowImport({
  projectId: () => projectId.value,
  onImported: async (wf) => {
    workflows.value = [wf, ...workflows.value.filter((x) => x.id !== wf.id)]
  },
})

const tabs: { id: Tab; labelKey: string }[] = [
  { id: 'board', labelKey: 'pages.projectDetail.tabBoard' },
  { id: 'workflows', labelKey: 'pages.projectDetail.tabWorkflows' },
  { id: 'pmLeader', labelKey: 'pages.projectDetail.tabPmLeader' },
  { id: 'cronJobs', labelKey: 'pages.projectDetail.tabCronJobs' },
  { id: 'sandboxEnv', labelKey: 'pages.projectDetail.tabSandboxEnv' },
  { id: 'variables', labelKey: 'pages.projectDetail.tabVariables' },
  { id: 'meta', labelKey: 'pages.projectDetail.tabMeta' },
]

const pmBinding = ref<PmLeaderBinding | null>(null)
const pmStudioAgent = computed(() => (pmBinding.value?.agentConfigRef || '').trim())
const canOpenStudioMemory = computed(() => !!pmStudioAgent.value)

async function loadPmBinding() {
  try {
    pmBinding.value = await api.getPmLeader(projectId.value)
  } catch {
    pmBinding.value = null
  }
}

function onPmBindingChanged(b: PmLeaderBinding) {
  pmBinding.value = b
  // Save succeeded → return to chat / empty state with refreshed binding.
  pmView.value = 'chat'
}

const VAR_TYPES = computed(() => [
  { value: 'string', label: t('common.varTypes.string') },
  { value: 'paragraph', label: t('common.varTypes.paragraph') },
  { value: 'number', label: t('common.varTypes.number') },
  { value: 'bool', label: t('common.varTypes.bool') },
  { value: 'select', label: t('common.varTypes.select') },
])

const existingNames = () => workflows.value.map((w) => w.name)

function askFields(w: Workflow): InputField[] {
  const input = (w.nodes || []).find((n) => n.type === 'input')
  const vars = ((input?.config?.variables as any[]) || []).filter((v) => v && v.name && v.ask)
  return vars.map((v) => ({
    key: v.name,
    desc: v.desc,
    type: v.type === 'string' ? 'text' : v.type,
    required: v.required,
    default:
      v.type === 'repos'
        ? JSON.stringify(Array.isArray(v.value) ? v.value : [])
        : v.value == null
          ? ''
          : String(v.value),
    editable: v.editable,
    options: v.options,
  }))
}

function fieldOptions(f: InputField): string[] {
  return String(f.options || '')
    .split(/[,，]/)
    .map((s) => s.trim())
    .filter(Boolean)
}

function closeMenu() {
  openMenuId.value = null
}

function toggleMenu(id: string) {
  openMenuId.value = openMenuId.value === id ? null : id
}

function menuIdFor(id: string) {
  return 'pd-wf-more-menu-' + id
}

function onDocClick(e: MouseEvent) {
  const el = e.target as Element | null
  if (!el?.closest?.('[data-wf-menu]')) closeMenu()
}

function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape') closeMenu()
}

function onScrollClose() {
  if (openMenuId.value) closeMenu()
}

async function load() {
  resetPmViewForProjectContext()
  loading.value = true
  try {
    const [p, wfs] = await Promise.all([
      api.getProject(projectId.value),
      api.listWorkflows({ projectId: projectId.value }),
    ])
    project.value = p
    void loadPmBinding()
    writeStoredProjectId(p.id)
    editName.value = p.name
    editDesc.value = p.description || ''
    envRows.value = (p.sandboxEnv || []).map((e) => ({ ...e }))
    // Spread-copy preserves server-side ask/required/editable (and options/desc).
    varRows.value = (p.variables || []).map((v) => ({ ...v }))
    workflows.value = wfs
  } catch {
    project.value = null
    workflows.value = []
  } finally {
    loading.value = false
  }
}

async function reloadWorkflows() {
  try {
    workflows.value = await api.listWorkflows({ projectId: projectId.value })
  } catch {
    // keep current list on refresh failure
  }
}

async function saveMeta() {
  if (!project.value) return
  savingMeta.value = true
  try {
    project.value = await api.updateProject(project.value.id, {
      name: editName.value.trim(),
      description: editDesc.value,
    })
    toast.success(t('pages.projectDetail.saved'))
  } catch (e: any) {
    toast.error(String(e?.message || e))
  } finally {
    savingMeta.value = false
  }
}

async function saveEnv() {
  if (!project.value) return
  savingEnv.value = true
  try {
    const sandboxEnv = envRows.value
      .filter((e) => e.key.trim())
      .map((e) => ({
        ...e,
        secret: e.secret || isPlatformAuthEnvKey(e.key),
      }))
    project.value = await api.updateProject(project.value.id, { sandboxEnv })
    envRows.value = (project.value.sandboxEnv || []).map((e) => ({
      ...e,
      secret: e.secret || isPlatformAuthEnvKey(e.key),
    }))
    toast.success(t('pages.projectDetail.saved'))
  } catch (e: any) {
    toast.error(String(e?.message || e))
  } finally {
    savingEnv.value = false
  }
}

async function saveVars() {
  if (!project.value) return
  savingVars.value = true
  try {
    // Submit full row objects (incl. hidden ask/required/editable) so mergeProjectVars
    // whole-table replace does not wipe server-side values.
    project.value = await api.updateProject(project.value.id, {
      variables: varRows.value.filter((v) => v.name.trim()),
    })
    varRows.value = (project.value.variables || []).map((v) => ({ ...v }))
    toast.success(t('pages.projectDetail.saved'))
  } catch (e: any) {
    toast.error(String(e?.message || e))
  } finally {
    savingVars.value = false
  }
}

const SECRET_MASK = '****'

/** Official ACP CLI auth keys — project env baseline; always forced Secret (matches server). */
const PLATFORM_AUTH_ENV_KEYS = new Set([
  'CURSOR_API_KEY',
  'ANTHROPIC_API_KEY',
  'CODEBUDDY_API_KEY',
  'TRAE_API_KEY',
  'TRAECLI_PERSONAL_ACCESS_TOKEN',
])

function isPlatformAuthEnvKey(key: string): boolean {
  return PLATFORM_AUTH_ENV_KEYS.has(key.trim())
}

function addEnvRow() {
  envRows.value.push({ key: '', value: '', secret: false })
}
function removeEnvRow(i: number) {
  envRows.value.splice(i, 1)
}
function addVarRow() {
  varRows.value.push({
    name: '',
    type: 'string',
    value: '',
    secret: false,
    desc: '',
    options: '',
    ask: false,
    required: false,
    editable: false,
  })
}
function removeVarRow(i: number) {
  varRows.value.splice(i, 1)
}

/** Clear mask when un-secreting or renaming so **** is never treated as plaintext. */
function onEnvSecretChange(row: ProjectEnvEntry, secret: boolean) {
  if (isPlatformAuthEnvKey(row.key)) {
    row.secret = true
    return
  }
  row.secret = secret
  if (!secret && row.value === SECRET_MASK) {
    row.value = ''
  }
}
function onEnvKeyChange(row: ProjectEnvEntry, key: string) {
  if (row.key !== key && row.value === SECRET_MASK) {
    row.value = ''
  }
  row.key = key
  if (isPlatformAuthEnvKey(key)) {
    row.secret = true
  }
}
function onVarSecretChange(row: ProjectVariable, secret: boolean) {
  row.secret = secret
  if (!secret && row.value === SECRET_MASK) {
    row.value = ''
  }
}
function onVarNameChange(row: ProjectVariable, name: string) {
  if (row.name !== name && row.value === SECRET_MASK) {
    row.value = ''
  }
  row.name = name
}

/** In-place type switch — never rebuild the row object (preserves ask/required/editable). */
function onVarTypeChange(row: ProjectVariable, type: string) {
  row.type = type
  if (type === 'bool') {
    row.value = row.value === true || row.value === 'true'
  } else if (type === 'number') {
    const n = Number(row.value)
    row.value = Number.isFinite(n) ? n : 0
  } else if (type === 'select') {
    if (row.options == null) row.options = ''
    row.value = row.value == null ? '' : String(row.value)
  } else {
    row.value = row.value == null ? '' : String(row.value)
  }
}

function selectOptions(row: ProjectVariable): string[] {
  return String(row.options || '')
    .split(',')
    .map((s) => s.trim())
    .filter(Boolean)
}

function isBoolTrue(row: ProjectVariable): boolean {
  return row.value === true || row.value === 'true'
}

function setBoolValue(row: ProjectVariable, v: boolean) {
  row.value = v
}

function onVarValueInput(row: ProjectVariable, raw: string, asNumber = false) {
  row.value = asNumber ? Number(raw) : raw
}

function newWorkflow() {
  router.push({ path: '/workflows/new/edit', query: { projectId: projectId.value } })
}

function openWorkflow(w: Workflow) {
  router.push('/workflows/' + w.id + '/edit')
}

function openEdit(w: Workflow) {
  closeMenu()
  openWorkflow(w)
}

function openRun(w: Workflow) {
  closeMenu()
  runTarget.value = w
  draftRestored.value = false
  runFields.value = askFields(w)
  const seed: Record<string, string> = {}
  const imgSeed: Record<string, ClarifyImage[]> = {}
  for (const f of runFields.value) {
    seed[f.key] = f.default || (f.type === 'select' ? fieldOptions(f)[0] || '' : '')
    imgSeed[f.key] = []
  }
  const keys = runFields.value.map((f) => f.key)
  const merged = mergeRunDraft(w.id, seed, imgSeed, keys)
  runInputs.value = merged.inputs
  runImages.value = merged.images
  draftRestored.value = merged.restored
}

function saveRunDraftClick() {
  const target = runTarget.value
  if (!target) return
  const images: Record<string, ClarifyImage[]> = {}
  for (const [k, v] of Object.entries(runImages.value)) {
    images[k] = v ? [...v] : []
  }
  const result = saveRunDraft(target.id, { ...runInputs.value }, images)
  if (result === 'ok') toast.success(t('common.toast.draftSaved'))
  else if (result === 'quota_exceeded') toast.error(t('common.toast.draftTooLarge'))
  else toast.error(t('common.toast.draftSaveFailed'))
}

function closeRunModal() {
  runTarget.value = null
}

function onRunStayed() {
  void reloadWorkflows()
}

function onRunStarted() {
  if (runTarget.value) clearRunDraft(runTarget.value.id)
}

function onViewRun(runId: string) {
  router.push('/runs/' + runId)
}

async function openCopy(w: Workflow) {
  closeMenu()
  copyPreviewLoading.value = w.id
  try {
    const preview = await api.copyPreviewWorkflow(w.id)
    copyModal.value = {
      sourceId: preview.sourceId,
      sourceName: preview.sourceName,
      suggestedName: preview.suggestedName,
    }
  } catch {
    toast.error(t('common.toast.copyNameFailed'))
  } finally {
    copyPreviewLoading.value = null
  }
}

function closeCopyModal() {
  copyModal.value = null
}

function onCopied(wf: Workflow) {
  workflows.value = [wf, ...workflows.value.filter((x) => x.id !== wf.id)]
  copyModal.value = null
  toast.success(t('common.toast.copied', { name: wf.name }))
}

function openExport(w: Workflow) {
  closeMenu()
  exportTarget.value = w
}

function openDeleteWf(w: Workflow) {
  closeMenu()
  deleteWfTarget.value = w
  deleteWfError.value = ''
}

async function confirmDeleteWf() {
  const target = deleteWfTarget.value
  if (!target) return
  deletingWf.value = true
  deleteWfError.value = ''
  try {
    await api.deleteWorkflow(target.id)
    workflows.value = workflows.value.filter((w) => w.id !== target.id)
    deleteWfTarget.value = null
  } catch (e: any) {
    deleteWfError.value = String(e?.message || e)
  } finally {
    deletingWf.value = false
  }
}

async function confirmDelete() {
  if (!project.value) return
  deleting.value = true
  deleteError.value = ''
  try {
    await api.deleteProject(project.value.id)
    writeStoredProjectId('')
    toast.success(t('pages.projectDetail.deleted'))
    router.push('/projects')
  } catch (e: any) {
    deleteError.value = String(e?.message || e)
  } finally {
    deleting.value = false
  }
}

watch(projectId, () => {
  resetPmViewForProjectContext()
  void load()
})
watch(
  () => route.query.tab,
  () => {
    syncTabFromRoute()
  },
)
onMounted(() => {
  ensureTabQuery()
  void load()
  document.addEventListener('click', onDocClick)
  document.addEventListener('keydown', onKeydown)
  document.addEventListener('scroll', onScrollClose, true)
})
onUnmounted(() => {
  document.removeEventListener('click', onDocClick)
  document.removeEventListener('keydown', onKeydown)
  document.removeEventListener('scroll', onScrollClose, true)
})
</script>

<template>
  <div class="flex min-h-0 flex-1 flex-col">
    <div class="mb-4 shrink-0">
      <button
        type="button"
        class="mb-2 inline-flex items-center gap-1 text-xs text-txt3 hover:text-txt2"
        @click="router.push('/projects')"
      >
        <Icon name="chevron-right" :size="12" class="rotate-180" />
        {{ t('pages.projectDetail.back') }}
      </button>
      <div v-if="loading" class="h-8 w-48 animate-pulse rounded bg-elevated" />
      <template v-else-if="project">
        <div class="flex flex-col gap-3 md:flex-row md:items-start md:justify-between">
          <div class="min-w-0">
            <h2 class="text-lg font-semibold text-txt">{{ project.name }}</h2>
            <p v-if="project.description" class="mt-0.5 text-sm text-txt3">{{ project.description }}</p>
          </div>
          <div class="flex flex-wrap gap-2">
            <AppButton variant="outline" size="sm" class="text-err" @click="showDelete = true">
              {{ t('common.buttons.delete') }}
            </AppButton>
          </div>
        </div>
      </template>
      <div v-else class="text-sm text-err">{{ t('pages.projectDetail.notFound') }}</div>
    </div>

    <template v-if="project">
      <div class="mb-4 flex shrink-0 flex-wrap gap-1 border-b border-line" data-testid="project-detail-tabs">
        <button
          v-for="tb in tabs"
          :key="tb.id"
          type="button"
          class="border-b-2 px-3 py-2 text-sm transition"
          :class="
            tab === tb.id
              ? 'border-accent text-accent-2'
              : 'border-transparent text-txt3 hover:text-txt2'
          "
          :data-testid="`project-tab-${tb.id}`"
          @click="setTab(tb.id)"
        >
          {{ t(tb.labelKey) }}
        </button>
      </div>

      <div
        v-if="showPmMemoryMigration"
        data-testid="pm-memory-migration-banner"
        class="mb-3 flex flex-col gap-2 border border-warn/35 bg-warn/10 px-3 py-2.5 sm:flex-row sm:items-center sm:justify-between"
      >
        <div class="min-w-0">
          <div class="text-[12px] font-medium text-txt">{{ t('pages.projectDetail.pm.memoryMigratedTitle') }}</div>
          <p class="mt-0.5 text-[11px] text-txt2">{{ t('pages.projectDetail.pm.memoryMigratedDesc') }}</p>
        </div>
        <div class="flex shrink-0 flex-wrap gap-2">
          <AppButton
            size="sm"
            variant="primary"
            data-testid="pm-memory-go-studio"
            @click="goStudioMemory()"
          >
            {{ t('pages.projectDetail.pm.goStudioMemory') }}
          </AppButton>
          <AppButton size="sm" variant="ghost" data-testid="pm-memory-migration-dismiss" @click="dismissPmMemoryMigration">
            {{ t('common.buttons.close') }}
          </AppButton>
        </div>
      </div>

      <div v-if="tab === 'board'" data-testid="project-board-panel">
        <BoardView :project-id="projectId" embedded />
      </div>

      <div
        v-else-if="tab === 'pmLeader'"
        class="flex min-h-0 flex-1 flex-col"
        data-testid="project-pm-leader-panel"
      >
        <div
          class="mb-2 flex flex-col gap-2 border border-line bg-surface px-3 py-2 sm:flex-row sm:items-center sm:justify-between"
          data-testid="pm-studio-memory-guide"
        >
          <div class="min-w-0">
            <div class="text-[12px] font-medium text-txt">{{ t('pages.projectDetail.pm.openStudioMemoryTitle') }}</div>
            <p class="mt-0.5 text-[11px] text-txt3">
              {{
                canOpenStudioMemory
                  ? t('pages.projectDetail.pm.openStudioMemoryHint', { agent: pmStudioAgent })
                  : t('pages.projectDetail.pm.openStudioMemoryDisabledHint')
              }}
            </p>
          </div>
          <AppButton
            size="sm"
            variant="primary"
            data-testid="pm-open-studio-memory"
            :disabled="!canOpenStudioMemory"
            :title="canOpenStudioMemory ? undefined : t('pages.projectDetail.pm.openStudioMemoryDisabledHint')"
            @click="goStudioMemory(pmStudioAgent)"
          >
            {{ t('pages.projectDetail.pm.openStudioMemory') }}
          </AppButton>
        </div>
        <PmLeaderChat
          v-if="pmView === 'chat'"
          :project-id="projectId"
          :binding="pmBinding"
          :restore-mobile-chat="pmRestoreMobileChat"
          @open-settings="openPmSettings"
          @restored-mobile-chat="pmRestoreMobileChat = false"
        />
        <div
          v-else
          class="flex min-h-0 flex-1 flex-col gap-3 overflow-hidden"
          data-testid="project-pm-settings-view"
        >
          <div class="flex shrink-0 items-center border-b border-line px-3 py-2" :class="isMobile ? 'min-h-[44px]' : ''">
            <button
              v-if="isMobile"
              type="button"
              class="flex h-11 w-11 shrink-0 items-center justify-center rounded-md text-txt2 hover:bg-elevated hover:text-txt"
              data-testid="pm-settings-back"
              :aria-label="t('shell.aria.backToList')"
              @click="backToPmChat"
            >
              <Icon name="arrow-left" :size="18" />
            </button>
            <AppButton
              v-else
              variant="ghost"
              size="sm"
              data-testid="pm-settings-back"
              @click="backToPmChat"
            >
              {{ t('pages.projectDetail.pm.backToChat') }}
            </AppButton>
          </div>
          <PmSettingsPanel :project-id="projectId" @changed="onPmBindingChanged" />
        </div>
      </div>

      <div v-else-if="tab === 'cronJobs'" class="flex min-h-[420px] flex-col">
        <PmCronJobsPanel :project-id="projectId" />
      </div>

      <!-- Workflows tab -->
      <div v-else-if="tab === 'workflows'">
        <div class="mb-3 flex justify-end gap-2">
          <AppButton variant="outline" icon="input" @click="triggerImport">
            {{ t('common.buttons.import') }}
          </AppButton>
          <AppButton variant="primary" icon="plus" @click="newWorkflow">
            {{ t('common.buttons.newWorkflow') }}
          </AppButton>
        </div>
        <div v-if="!workflows.length" class="card px-5 py-10 text-center text-[13px] text-txt3">
          {{ t('common.empty.noWorkflows') }}
        </div>
        <!-- Mobile card list: avoids table overflow clipping the more menu -->
        <div v-else-if="isMobile" class="flex flex-col gap-2">
          <article
            v-for="w in workflows"
            :key="w.id"
            class="flex flex-col gap-3 rounded-lg border border-line bg-surface p-3 transition hover:border-line-strong hover:bg-elevated"
          >
            <button
              type="button"
              class="flex w-full items-start justify-between gap-3 text-left"
              @click="openWorkflow(w)"
            >
              <div class="min-w-0 flex-1">
                <div class="truncate text-sm font-semibold text-txt">{{ w.name }}</div>
                <div
                  v-if="w.description"
                  class="mt-0.5 truncate text-[11px] text-txt3"
                >{{ w.description }}</div>
              </div>
              <StatusPill :status="w.status" size="sm" />
            </button>

            <div class="relative flex items-center gap-2" data-wf-menu @click.stop>
              <button
                type="button"
                class="inline-flex min-h-[44px] flex-1 items-center justify-center gap-1.5 rounded-md bg-accent-dim px-3.5 text-sm font-medium text-accent-2 transition hover:brightness-110"
                @click="openRun(w)"
              >
                <Icon name="play" :size="14" />
                {{ t('common.buttons.run') }}
              </button>
              <button
                type="button"
                class="inline-flex h-11 w-11 shrink-0 items-center justify-center rounded-md border border-line bg-surface text-txt2 transition hover:border-line-strong hover:bg-elevated hover:text-txt"
                :class="{ 'border-line-strong bg-elevated text-txt': openMenuId === w.id }"
                :aria-label="t('pages.workflowList.moreActions')"
                :aria-expanded="openMenuId === w.id"
                aria-haspopup="menu"
                :aria-controls="openMenuId === w.id ? menuIdFor(w.id) : undefined"
                @click="toggleMenu(w.id)"
              >
                <Icon name="more" :size="18" />
              </button>
              <div
                v-if="openMenuId === w.id"
                :id="menuIdFor(w.id)"
                class="absolute bottom-[calc(100%+6px)] right-0 z-30 min-w-[148px] rounded-md border border-line-strong bg-surface p-1 shadow-lg"
                role="menu"
              >
                <button
                  type="button"
                  role="menuitem"
                  class="flex min-h-[44px] w-full items-center gap-2 rounded-md px-3 text-left text-[13px] text-txt2 transition hover:bg-elevated hover:text-txt"
                  @click="openEdit(w)"
                >
                  <Icon name="edit" :size="14" />{{ t('common.buttons.edit') }}
                </button>
                <button
                  type="button"
                  role="menuitem"
                  class="flex min-h-[44px] w-full items-center gap-2 rounded-md px-3 text-left text-[13px] text-txt2 transition hover:bg-elevated hover:text-txt disabled:opacity-50"
                  :disabled="copyPreviewLoading === w.id"
                  @click="openCopy(w)"
                >
                  <Icon name="copy" :size="14" />{{
                    copyPreviewLoading === w.id ? t('common.buttons.loading') : t('common.buttons.copy')
                  }}
                </button>
                <button
                  type="button"
                  role="menuitem"
                  class="flex min-h-[44px] w-full items-center gap-2 rounded-md px-3 text-left text-[13px] text-txt2 transition hover:bg-elevated hover:text-txt"
                  @click="openExport(w)"
                >
                  <Icon name="download" :size="14" />{{ t('common.buttons.export') }}
                </button>
                <button
                  type="button"
                  role="menuitem"
                  class="flex min-h-[44px] w-full items-center gap-2 rounded-md px-3 text-left text-[13px] text-err transition hover:bg-err/10"
                  @click="openDeleteWf(w)"
                >
                  <Icon name="trash" :size="14" />{{ t('common.buttons.delete') }}
                </button>
              </div>
            </div>
          </article>
        </div>
        <!-- Desktop table -->
        <div v-else class="overflow-hidden rounded-lg border border-line">
          <div class="scroll-area overflow-x-auto">
            <table class="w-full text-left text-sm">
              <thead class="bg-elevated text-xs text-txt3">
                <tr>
                  <th class="px-3 py-2 font-medium">{{ t('pages.projectDetail.colName') }}</th>
                  <th class="px-3 py-2 font-medium">{{ t('pages.projectDetail.colStatus') }}</th>
                  <th class="px-3 py-2 font-medium">{{ t('pages.projectDetail.colUpdated') }}</th>
                  <th class="px-3 py-2 text-right font-medium whitespace-nowrap">{{ t('common.table.actions') }}</th>
                </tr>
              </thead>
              <tbody>
                <tr
                  v-for="w in workflows"
                  :key="w.id"
                  class="cursor-pointer border-t border-line hover:bg-elevated"
                  @click="openWorkflow(w)"
                >
                  <td class="px-3 py-2.5">
                    <div class="font-medium text-txt">{{ w.name }}</div>
                    <div v-if="w.description" class="truncate text-xs text-txt3">{{ w.description }}</div>
                  </td>
                  <td class="px-3 py-2.5"><StatusPill :status="w.status" size="sm" /></td>
                  <td class="px-3 py-2.5 text-txt3">{{ fmtTime(w.updatedAt) }}</td>
                  <td class="px-3 py-2.5" @click.stop>
                    <div class="flex flex-wrap items-center justify-end gap-1">
                      <button
                        type="button"
                        class="whitespace-nowrap rounded-md px-2 py-1 text-xs text-txt2 hover:bg-overlay hover:text-txt"
                        @click="openEdit(w)"
                      >
                        <Icon name="edit" :size="13" class="mr-1 inline" />{{ t('common.buttons.edit') }}
                      </button>
                      <button
                        type="button"
                        class="whitespace-nowrap rounded-md px-2 py-1 text-xs text-accent-2 hover:bg-accent-dim"
                        @click="openRun(w)"
                      >
                        <Icon name="play" :size="13" class="mr-1 inline" />{{ t('common.buttons.run') }}
                      </button>
                      <button
                        type="button"
                        class="whitespace-nowrap rounded-md px-2 py-1 text-xs text-accent-2 hover:bg-accent-dim disabled:opacity-50"
                        :disabled="copyPreviewLoading === w.id"
                        @click="openCopy(w)"
                      >
                        <Icon name="copy" :size="13" class="mr-1 inline" />{{
                          copyPreviewLoading === w.id ? t('common.buttons.loading') : t('common.buttons.copy')
                        }}
                      </button>
                      <button
                        type="button"
                        class="whitespace-nowrap rounded-md px-2 py-1 text-xs text-accent-2 hover:bg-accent-dim"
                        @click="openExport(w)"
                      >
                        <Icon name="download" :size="13" class="mr-1 inline" />{{ t('common.buttons.export') }}
                      </button>
                      <button
                        type="button"
                        class="whitespace-nowrap rounded-md px-2 py-1 text-xs text-txt2 hover:bg-err/10 hover:text-err"
                        @click="openDeleteWf(w)"
                      >
                        <Icon name="trash" :size="13" class="mr-1 inline" />{{ t('common.buttons.delete') }}
                      </button>
                    </div>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
      </div>

      <!-- Sandbox env tab -->
      <div v-else-if="tab === 'sandboxEnv'" class="flex min-h-[420px] flex-col">
        <div class="mb-3 flex flex-wrap items-baseline gap-x-3 gap-y-1.5">
          <p class="m-0 text-[13px] text-txt3">{{ t('pages.projectDetail.envHint') }}</p>
          <button
            type="button"
            class="p-0 text-[13px] text-accent-2 underline underline-offset-2 hover:text-txt"
            @click="helpOpen = true"
          >
            {{ t('pages.projectDetail.viewMergeRules') }}
          </button>
        </div>

        <!-- Empty: same shell as data panel, min-h centered, primary CTA -->
        <div
          v-if="!envRows.length"
          class="flex min-h-[360px] flex-1 flex-col border border-line bg-surface shadow-[var(--shadow-card)]"
        >
          <div class="flex flex-1 flex-col justify-center">
            <EmptyState
              icon="variable"
              :title="t('pages.projectDetail.envEmptyTitle')"
              :desc="t('pages.projectDetail.envEmptyDesc')"
            >
              <AppButton variant="primary" icon="plus" @click="addEnvRow">
                {{ t('pages.projectDetail.addRow') }}
              </AppButton>
            </EmptyState>
          </div>
        </div>

        <!-- Data: unified panel with head / rows / foot -->
        <div
          v-else
          class="flex flex-1 flex-col overflow-hidden border border-line bg-surface shadow-[var(--shadow-card)]"
        >
          <div
            class="hidden gap-2 border-b border-line bg-elevated/55 px-3 py-2.5 text-[11px] font-semibold uppercase tracking-wider text-txt3 sm:grid sm:grid-cols-[minmax(0,1.1fr)_minmax(0,1.4fr)_88px_40px]"
          >
            <span>{{ t('pages.projectDetail.envKey') }}</span>
            <span>{{ t('pages.projectDetail.colValue') }}</span>
            <span>{{ t('pages.projectDetail.colType') }}</span>
            <span>{{ t('common.table.actions') }}</span>
          </div>

          <div class="flex flex-col">
            <div
              v-for="(row, i) in envRows"
              :key="i"
              class="grid grid-cols-1 gap-2 border-b border-line bg-base/40 px-3 py-2 last:border-b-0 hover:bg-elevated/35 sm:grid-cols-[minmax(0,1.1fr)_minmax(0,1.4fr)_88px_40px] sm:items-center"
            >
              <input
                :value="row.key"
                class="input px-2.5 py-1.5 font-mono text-xs"
                :placeholder="t('pages.projectDetail.envKey')"
                @input="onEnvKeyChange(row, ($event.target as HTMLInputElement).value)"
              />
              <input
                v-model="row.value"
                class="input min-w-0 px-2.5 py-1.5 text-xs"
                :placeholder="t('pages.projectDetail.envValue')"
                :type="row.secret ? 'password' : 'text'"
              />
              <div class="flex w-full flex-col items-stretch gap-1">
                <button
                  type="button"
                  class="chip w-full justify-center"
                  :class="row.secret ? 'border-accent/50 text-accent-2' : 'text-txt3'"
                  :disabled="isPlatformAuthEnvKey(row.key)"
                  :title="
                    isPlatformAuthEnvKey(row.key)
                      ? t('pages.projectDetail.secretForcedHint')
                      : row.secret
                        ? t('pages.projectDetail.secret')
                        : t('pages.projectDetail.plain')
                  "
                  @click="onEnvSecretChange(row, !row.secret)"
                >
                  {{ row.secret ? t('pages.projectDetail.secret') : t('pages.projectDetail.plain') }}
                </button>
                <span
                  v-if="isPlatformAuthEnvKey(row.key)"
                  class="text-center text-[10px] font-semibold tracking-wide text-accent-2"
                >
                  {{ t('pages.projectDetail.secretForced') }}
                </span>
              </div>
              <button
                type="button"
                class="inline-flex h-7 w-7 shrink-0 items-center justify-center text-txt3 hover:text-err sm:justify-self-center"
                :title="t('common.buttons.delete')"
                :aria-label="t('common.buttons.delete')"
                @click="removeEnvRow(i)"
              >
                <Icon name="close" :size="14" />
              </button>
            </div>
          </div>

          <div class="flex flex-wrap gap-2 border-t border-line bg-surface p-3">
            <AppButton variant="outline" icon="plus" @click="addEnvRow">
              {{ t('pages.projectDetail.addRow') }}
            </AppButton>
            <AppButton variant="primary" :disabled="savingEnv" @click="saveEnv">
              {{ t('common.buttons.save') }}
            </AppButton>
          </div>
        </div>
      </div>

      <!-- Variables tab -->
      <div v-else-if="tab === 'variables'" class="flex min-h-[420px] flex-col">
        <div class="mb-3 flex flex-wrap items-baseline gap-x-3 gap-y-1.5">
          <p class="m-0 text-[13px] text-txt3">
            {{ t('pages.projectDetail.varsHint') }}
          </p>
          <button
            type="button"
            class="p-0 text-[13px] text-accent-2 underline underline-offset-2 hover:text-txt"
            @click="helpOpen = true"
          >
            {{ t('pages.projectDetail.viewMergeRules') }}
          </button>
        </div>

        <!-- Empty: same shell as sandbox tab -->
        <div
          v-if="!varRows.length"
          class="flex min-h-[360px] flex-1 flex-col border border-line bg-surface shadow-[var(--shadow-card)]"
        >
          <div class="flex flex-1 flex-col justify-center">
            <EmptyState
              icon="doc"
              :title="t('pages.projectDetail.varsEmptyTitle')"
              :desc="t('pages.projectDetail.varsEmptyDesc')"
            >
              <AppButton variant="primary" icon="plus" @click="addVarRow">
                {{ t('pages.projectDetail.addRow') }}
              </AppButton>
            </EmptyState>
          </div>
        </div>

        <!-- Data: unified panel; main row 4-col + secondary desc/options -->
        <div
          v-else
          class="flex flex-1 flex-col overflow-hidden border border-line bg-surface shadow-[var(--shadow-card)]"
        >
          <div
            class="hidden gap-2 border-b border-line bg-elevated/55 px-3 py-2.5 text-[11px] font-semibold uppercase tracking-wider text-txt3 sm:grid sm:grid-cols-[minmax(0,1fr)_minmax(0,1.2fr)_minmax(120px,auto)_40px]"
          >
            <span>{{ t('pages.projectDetail.colVarName') }}</span>
            <span>{{ t('pages.projectDetail.colDefault') }}</span>
            <span>{{ t('pages.projectDetail.colType') }}</span>
            <span>{{ t('common.table.actions') }}</span>
          </div>

          <div class="flex flex-col">
            <div
              v-for="(row, i) in varRows"
              :key="i"
              class="border-b border-line bg-base/40 px-3 py-2 last:border-b-0 hover:bg-elevated/35"
            >
              <div
                class="grid grid-cols-1 gap-2 sm:grid-cols-[minmax(0,1fr)_minmax(0,1.2fr)_minmax(120px,auto)_40px] sm:items-center"
              >
                <input
                  :value="row.name"
                  class="input min-w-0 px-2.5 py-1.5 font-mono text-xs"
                  :placeholder="t('pages.projectDetail.varName')"
                  @input="onVarNameChange(row, ($event.target as HTMLInputElement).value)"
                />

                <!-- Value control matrix by type -->
                <div
                  v-if="row.type === 'bool'"
                  class="flex min-w-0 overflow-hidden border border-line"
                >
                  <button
                    type="button"
                    class="flex-1 bg-base px-2.5 py-1.5 text-xs transition"
                    :class="isBoolTrue(row) ? 'bg-accent-dim text-accent-2' : 'text-txt3'"
                    @click="setBoolValue(row, true)"
                  >
                    true
                  </button>
                  <button
                    type="button"
                    class="flex-1 bg-base px-2.5 py-1.5 text-xs transition"
                    :class="!isBoolTrue(row) ? 'bg-accent-dim text-accent-2' : 'text-txt3'"
                    @click="setBoolValue(row, false)"
                  >
                    false
                  </button>
                </div>
                <input
                  v-else-if="row.type === 'number'"
                  :value="row.value == null ? '' : String(row.value)"
                  type="number"
                  class="input min-w-0 px-2.5 py-1.5 text-xs"
                  :placeholder="t('pages.projectDetail.varValue')"
                  @input="onVarValueInput(row, ($event.target as HTMLInputElement).value, true)"
                />
                <textarea
                  v-else-if="row.type === 'paragraph'"
                  :value="row.value == null ? '' : String(row.value)"
                  rows="2"
                  class="input min-h-[40px] min-w-0 px-2.5 py-1.5 text-xs"
                  :placeholder="t('pages.projectDetail.varValue')"
                  @input="onVarValueInput(row, ($event.target as HTMLTextAreaElement).value)"
                />
                <select
                  v-else-if="row.type === 'select'"
                  :value="row.value == null ? '' : String(row.value)"
                  class="input min-w-0 px-2.5 py-1.5 text-xs"
                  @change="onVarValueInput(row, ($event.target as HTMLSelectElement).value)"
                >
                  <option value="">{{ t('pages.projectDetail.selectPlaceholder') }}</option>
                  <option v-for="opt in selectOptions(row)" :key="opt" :value="opt">{{ opt }}</option>
                </select>
                <input
                  v-else
                  :value="row.value == null ? '' : String(row.value)"
                  class="input min-w-0 px-2.5 py-1.5 text-xs"
                  :placeholder="t('pages.projectDetail.varValue')"
                  :type="row.secret ? 'password' : 'text'"
                  @input="onVarValueInput(row, ($event.target as HTMLInputElement).value)"
                />

                <div class="flex min-w-0 flex-wrap items-center gap-1.5">
                  <select
                    :value="row.type"
                    class="input min-w-0 flex-1 px-2.5 py-1.5 text-xs sm:w-[100px] sm:flex-none"
                    @change="onVarTypeChange(row, ($event.target as HTMLSelectElement).value)"
                  >
                    <option v-for="vt in VAR_TYPES" :key="vt.value" :value="vt.value">
                      {{ vt.label }}
                    </option>
                  </select>
                  <button
                    type="button"
                    class="chip shrink-0"
                    :class="row.secret ? 'border-accent/50 text-accent-2' : 'text-txt3'"
                    :title="row.secret ? t('pages.projectDetail.secret') : t('pages.projectDetail.plain')"
                    @click="onVarSecretChange(row, !row.secret)"
                  >
                    {{ row.secret ? t('pages.projectDetail.secret') : t('pages.projectDetail.plain') }}
                  </button>
                </div>

                <button
                  type="button"
                  class="inline-flex h-7 w-7 shrink-0 items-center justify-center text-txt3 hover:text-err sm:justify-self-center"
                  :title="t('common.buttons.delete')"
                  :aria-label="t('common.buttons.delete')"
                  @click="removeVarRow(i)"
                >
                  <Icon name="close" :size="14" />
                </button>
              </div>

              <!-- Secondary row: desc / select options -->
              <div class="mt-2 flex flex-col gap-1.5">
                <input
                  v-model="row.desc"
                  class="input px-2.5 py-1.5 text-xs"
                  :placeholder="t('pages.projectDetail.varDescPlaceholder')"
                />
                <input
                  v-if="row.type === 'select'"
                  v-model="row.options"
                  class="input px-2.5 py-1.5 font-mono text-xs"
                  :placeholder="t('pages.projectDetail.varOptionsPlaceholder')"
                />
              </div>
            </div>
          </div>

          <div class="flex flex-wrap gap-2 border-t border-line bg-surface p-3">
            <AppButton variant="outline" icon="plus" @click="addVarRow">
              {{ t('pages.projectDetail.addRow') }}
            </AppButton>
            <AppButton variant="primary" :disabled="savingVars" @click="saveVars">
              {{ t('common.buttons.save') }}
            </AppButton>
          </div>
        </div>
      </div>

      <!-- Project info (meta) tab: near-full-width panel (shell / head / fields / primary save) -->
      <div v-else-if="tab === 'meta'" class="flex min-h-[420px] flex-col">
        <div
          class="flex flex-1 flex-col overflow-hidden border border-line bg-surface shadow-[var(--shadow-card)]"
        >
          <div class="border-b border-line bg-elevated/55 px-4 py-3.5">
            <h2 class="m-0 text-sm font-semibold text-txt">
              {{ t('pages.projectDetail.metaTitle') }}
            </h2>
            <p class="m-0 mt-1 text-[13px] leading-snug text-txt3">
              {{ t('pages.projectDetail.metaHint') }}
            </p>
          </div>

          <div class="flex flex-1 flex-col gap-3.5 p-4">
            <div>
              <label class="label" for="project-meta-name">{{ t('pages.projectList.nameLabel') }}</label>
              <input
                id="project-meta-name"
                v-model="editName"
                class="input"
                autocomplete="off"
              />
            </div>
            <div>
              <label class="label" for="project-meta-desc">{{ t('pages.projectList.descLabel') }}</label>
              <textarea
                id="project-meta-desc"
                v-model="editDesc"
                rows="5"
                class="input min-h-[120px]"
                :placeholder="t('pages.projectDetail.metaDescPlaceholder')"
              />
            </div>
          </div>

          <div class="flex flex-wrap justify-end gap-2 border-t border-line bg-surface p-3">
            <AppButton variant="primary" :disabled="savingMeta" @click="saveMeta">
              {{ t('common.buttons.save') }}
            </AppButton>
          </div>
        </div>
      </div>
    </template>

    <input ref="fileInput" type="file" accept=".json" class="hidden" @change="handleFileChange" />

    <RunLaunchModal
      v-if="runTarget"
      :open="!!runTarget"
      :workflow-id="runTarget.id"
      :workflow-name="runTarget.name"
      :fields="runFields"
      :run-inputs="runInputs"
      :run-images="runImages"
      :draft-restored="draftRestored"
      @close="closeRunModal"
      @stayed="onRunStayed"
      @view-run="onViewRun"
      @save-draft="saveRunDraftClick"
      @started="onRunStarted"
    />

    <CopyWorkflowModal
      v-if="copyModal"
      :open="!!copyModal"
      :source-id="copyModal.sourceId"
      :source-name="copyModal.sourceName"
      :suggested-name="copyModal.suggestedName"
      :existing-names="existingNames()"
      @close="closeCopyModal"
      @copied="onCopied"
    />

    <ExportVersionModal
      v-if="exportTarget"
      :open="!!exportTarget"
      :workflow-id="exportTarget.id"
      :workflow-name="exportTarget.name"
      :description="exportTarget.description"
      :needs-repo="exportTarget.needsRepo"
      :status="exportTarget.status"
      @close="exportTarget = null"
    />

    <AppModal
      :open="!!deleteWfTarget"
      :title="t('pages.workflowList.deleteTitle', { name: deleteWfTarget?.name || '' })"
      @close="deleteWfTarget = null"
    >
      <div class="space-y-3 text-sm text-txt2">
        <div class="flex items-start gap-2 rounded-md border border-err/30 bg-err/10 px-3 py-2 text-[12px] text-err">
          <Icon name="alert" :size="14" class="mt-0.5 shrink-0" />
          {{ t('pages.workflowList.deleteWarning') }}
        </div>
        <p>{{ t('pages.workflowList.deleteConfirm', { name: deleteWfTarget?.name }) }}</p>
        <div
          v-if="deleteWfError"
          class="flex items-start gap-2 rounded-md border border-err/30 bg-err/10 px-3 py-2 text-[12px] text-err"
        >
          <Icon name="alert" :size="14" class="mt-0.5" />{{ deleteWfError }}
        </div>
      </div>
      <template #footer>
        <AppButton variant="ghost" @click="deleteWfTarget = null">{{ t('common.buttons.cancel') }}</AppButton>
        <AppButton variant="danger" icon="trash" :disabled="deletingWf" @click="confirmDeleteWf">
          {{ deletingWf ? t('common.buttons.deleting') : t('common.buttons.confirmDelete') }}
        </AppButton>
      </template>
    </AppModal>

    <AppModal
      :open="showDelete"
      :title="t('pages.projectDetail.deleteTitle', { name: project?.name || '' })"
      :width="420"
      @close="!deleting && (showDelete = false)"
    >
      <p class="mb-3 text-sm text-txt2">{{ t('pages.projectDetail.deleteWarning') }}</p>
      <p v-if="deleteError" class="mb-2 text-sm text-err">{{ deleteError }}</p>
      <div class="flex justify-end gap-2">
        <AppButton variant="outline" :disabled="deleting" @click="showDelete = false">
          {{ t('common.buttons.cancel') }}
        </AppButton>
        <AppButton variant="danger" :disabled="deleting" @click="confirmDelete">
          {{ t('common.buttons.delete') }}
        </AppButton>
      </div>
    </AppModal>

    <AppModal
      :open="helpOpen"
      :title="t('pages.projectDetail.helpTitle')"
      :width="480"
      @close="helpOpen = false"
    >
      <div class="space-y-2 text-sm text-txt2">
        <p>{{ t('pages.projectDetail.helpMerge') }}</p>
        <p>{{ t('pages.projectDetail.helpNamespaces') }}</p>
        <p>{{ t('pages.projectDetail.helpSecret') }}</p>
      </div>
    </AppModal>
  </div>
</template>
