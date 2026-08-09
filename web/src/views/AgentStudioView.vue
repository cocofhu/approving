<script setup lang="ts">
import { ref, computed, watch, nextTick, onMounted, onBeforeUnmount } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/ui/Icon.vue'
import AppButton from '@/components/ui/AppButton.vue'
import AppModal from '@/components/ui/AppModal.vue'
import CodeEditor from '@/components/ui/CodeEditor.vue'
import MarkdownSplitEditor from '@/components/agent/MarkdownSplitEditor.vue'
import ExplorerContextMenu, { type CtxTarget } from '@/components/agent/ExplorerContextMenu.vue'
import AgentOrgSidebar from '@/components/agent/AgentOrgSidebar.vue'
import AgentChatTester from '@/components/agent/AgentChatTester.vue'
import AgentDataPanel, { type DataSubTab } from '@/components/agent/AgentDataPanel.vue'
import AgentGitGuide from '@/components/agent/AgentGitGuide.vue'
import EnvCredentialHelpModal, {
  type EnvCredentialHelpSection,
} from '@/components/agent/EnvCredentialHelpModal.vue'
import AgentCreateWizard from '@/components/agent/AgentCreateWizard.vue'
import { api, type Agent, type AgentOrg, type MCPServer, type AgentPrompts, type PlatformRuleMeta } from '@/lib/api'
import { useBreakpoint } from '@/lib/useBreakpoint'
import {
  emptyOrg,
  groupPath,
  newGroupId,
  applyDeleteGroup,
  applyMoveAgent,
  applyRemoveAgentFromGroup,
  setAgentMembership,
  groupIdsOf,
  parentOf,
  wouldCreateGroupCycle,
  wouldCreateReportingCycle,
  buildOrgTreeRows,
  isAgentInGroupSubtree,
  UNGROUPED_ID,
} from '@/lib/agentOrg'
import type { GitCredentialType } from '@/lib/gitCredentialAnalysis'
import { downloadZip, validateAgentName, normalizeAgentName } from '@/lib/agentIO'
import { useAgentImport } from '@/lib/useAgentImport'
import {
  ACP_BACKENDS,
  getRegionPolicy,
  isManagedRegionKey,
  normalizeRegions,
  setRegion,
  switchBackendRegions,
  type BackendId,
} from '@/lib/regionPolicy'
import { BACKEND_AUTH_HINTS } from '@/lib/backendAuthGuide'

const { t } = useI18n()
const { isMobile } = useBreakpoint()
const route = useRoute()
const router = useRouter()

type StudioTab = 'files' | 'mcp' | 'env' | 'prompts' | 'platform-rules' | 'test' | 'meta' | 'data'
const STUDIO_TABS: StudioTab[] = ['files', 'mcp', 'env', 'prompts', 'platform-rules', 'test', 'meta', 'data']

function isStudioTab(q: unknown): q is StudioTab {
  return typeof q === 'string' && (STUDIO_TABS as readonly string[]).includes(q)
}

function parseDataSub(q: unknown): DataSubTab {
  if (q === 'context' || q === 'jobs' || q === 'memory') return q
  return 'memory'
}

type KV = { k: string; v: string }
type PromptKey = keyof AgentPrompts
type PromptDraft = Record<PromptKey, string>
type DraftMCP = {
  name: string
  transport: 'url' | 'command'
  url: string
  headers: KV[]
  command: string
  args: string
  env: KV[]
}
type DraftFile = { path: string; content: string }
type Draft = {
  name: string
  projectId: string
  acpBackend: BackendId
  gitCredentialType?: GitCredentialType
  files: DraftFile[]
  mcp: DraftMCP[]
  env: KV[]
  layout: { configRoot: string; workspaceDir: string }
  prompts: PromptDraft
}

const ARTIFACT_STORE = 'artifact-store'
const LEGACY_PM_LEADER = 'pm-leader'
const MEMORY_STORE = 'memory-store'
const CONTEXT_STORE = 'context-store'
const TASK_SCHEDULER = 'task-scheduler'
const AGENT_PLATFORM_MCPS = [
  {
    name: MEMORY_STORE,
    url: '${APPROVING_MEMORY_URL}',
    token: '${APPROVING_MEMORY_TOKEN}',
  },
  {
    name: CONTEXT_STORE,
    url: '${APPROVING_CONTEXT_URL}',
    token: '${APPROVING_CONTEXT_TOKEN}',
  },
  {
    name: TASK_SCHEDULER,
    url: '${APPROVING_SCHEDULER_URL}',
    token: '${APPROVING_SCHEDULER_TOKEN}',
  },
] as const
const AGENT_LIST_COLLAPSED_KEY = 'agent-studio-agent-list-collapsed'
const EXPLORER_COLLAPSED_KEY = 'agent-studio-explorer-collapsed'
const SIDEBAR_EXPANDED_W = '240px'
const ORG_SIDEBAR_EXPANDED_W = '280px'
const SIDEBAR_COLLAPSED_W = '28px'

function readCollapsedState(key: string): boolean {
  try {
    const v = localStorage.getItem(key)
    if (v === null) return false
    return v === 'true'
  } catch {
    return false
  }
}

function writeCollapsedState(key: string, collapsed: boolean) {
  try {
    localStorage.setItem(key, String(collapsed))
  } catch {
    /* ignore quota / private mode */
  }
}

const agentListCollapsed = ref(false)
const explorerCollapsed = ref(false)

function toggleAgentListCollapsed() {
  agentListCollapsed.value = !agentListCollapsed.value
  writeCollapsedState(AGENT_LIST_COLLAPSED_KEY, agentListCollapsed.value)
}

function toggleExplorerCollapsed() {
  explorerCollapsed.value = !explorerCollapsed.value
  writeCollapsedState(EXPLORER_COLLAPSED_KEY, explorerCollapsed.value)
}

const cardGridStyle = computed(() => {
  if (isMobile.value) return { gridTemplateColumns: '1fr' }
  return {
    gridTemplateColumns: `${agentListCollapsed.value ? SIDEBAR_COLLAPSED_W : ORG_SIDEBAR_EXPANDED_W} 1fr`,
  }
})

const workspaceGridStyle = computed(() => {
  if (isMobile.value) return { gridTemplateColumns: '1fr' }
  return {
    gridTemplateColumns: `${explorerCollapsed.value ? SIDEBAR_COLLAPSED_W : SIDEBAR_EXPANDED_W} 1fr`,
  }
})

/** Narrow-screen workspace step: resource list → full-screen edit. */
type FilesStep = 'list' | 'edit'
const filesStep = ref<FilesStep>('list')
const justSaved = ref(false)

/** Narrow-screen agent org tree bottom sheet (≈70vh). */
const showOrgSheet = ref(false)
const orgSheetCollapsed = ref<Set<string>>(new Set())

type LeaveConfirmCfg = {
  title: string
  message: string
  saveText: string
  discardText: string
  /** Return true only when leave may proceed (e.g. save succeeded). */
  onSave: () => boolean | Promise<boolean>
  onDiscard: () => void
}
const leaveConfirmCfg = ref<LeaveConfirmCfg | null>(null)

const collapseBtnClass =
  'flex shrink-0 items-center justify-center rounded w-[22px] h-[22px] text-txt3 transition hover:bg-elevated hover:text-accent-2 focus-visible:shadow-[inset_0_0_0_2px_rgba(99,102,241,0.35)] outline-none'

// Ordered prompt-override fields. Empty value = platform default; the field
// placeholder shows the actual default. Rule files (rules/*.md) are NOT here —
// they live in the Agent working directory (files tab).
const PROMPT_KEYS: PromptKey[] = [
  'upstreamArtifactsHeader', 'producesContract', 'reactOpenSuffix', 'producesRetry',
]
const PROMPT_FRAGMENTS = computed(() => [
  {
    key: 'upstreamArtifactsHeader' as PromptKey,
    label: t('pages.agentStudio.promptFields.upstreamHeader.label'),
    hint: t('pages.agentStudio.promptFields.upstreamHeader.hint'),
    placeholder: t('pages.agentStudio.promptFields.upstreamHeader.placeholder'),
  },
  {
    key: 'producesContract' as PromptKey,
    label: t('pages.agentStudio.promptFields.producesContract.label'),
    hint: t('pages.agentStudio.promptFields.producesContract.hint'),
    placeholder: t('pages.agentStudio.promptFields.producesContract.placeholder'),
  },
  {
    key: 'reactOpenSuffix' as PromptKey,
    label: t('pages.agentStudio.promptFields.reactOpening.label'),
    hint: t('pages.agentStudio.promptFields.reactOpening.hint'),
    placeholder: t('pages.agentStudio.promptFields.reactOpening.placeholder'),
  },
  {
    key: 'producesRetry' as PromptKey,
    label: t('pages.agentStudio.promptFields.reactMissingArtifact.label'),
    hint: t('pages.agentStudio.promptFields.reactMissingArtifact.hint'),
    placeholder: t('pages.agentStudio.promptFields.reactMissingArtifact.placeholder'),
  },
])
function emptyPrompts(): PromptDraft {
  return { upstreamArtifactsHeader: '', producesContract: '', reactOpenSuffix: '', producesRetry: '' }
}

const agents = ref<Agent[]>([])
const projects = ref<{ id: string; name: string }[]>([])
const org = ref<AgentOrg>(emptyOrg())
const orgBaseline = ref('')
const activeName = ref('')
const draft = ref<Draft | null>(null)
const originalJson = ref('')
const tab = ref<StudioTab>('files')
const dataSubTab = ref<DataSubTab>('memory')
const parentDropdownOpen = ref(false)
const orgSaving = ref(false)
/** Skip one URL write while applying deep-link query on load. */
let applyingStudioQuery = false

function syncStudioQuery() {
  if (applyingStudioQuery) return
  const next: Record<string, string> = {}
  if (activeName.value) next.agent = activeName.value
  if (tab.value !== 'files') next.tab = tab.value
  if (tab.value === 'data') next.sub = dataSubTab.value
  const curAgent = typeof route.query.agent === 'string' ? route.query.agent : ''
  const curTab = typeof route.query.tab === 'string' ? route.query.tab : ''
  const curSub = typeof route.query.sub === 'string' ? route.query.sub : ''
  if (curAgent === (next.agent || '') && curTab === (next.tab || '') && curSub === (next.sub || '')) {
    return
  }
  void router.replace({ query: next })
}

// Data / chat tester follow the last-saved binding so unsaved draft edits
// cannot hit APIs under the wrong (or unbound) project.
const savedProjectId = computed(() => {
  const a = agents.value.find((x) => x.name === activeName.value)
  return a?.projectId?.trim() || ''
})
const isProjectBound = computed(() => !!savedProjectId.value)
const draftBindingDirty = computed(() => {
  const draftPid = draft.value?.projectId?.trim() || ''
  return draftPid !== savedProjectId.value
})
function projectNameById(id: string): string {
  return projects.value.find((p) => p.id === id)?.name || id
}
const pendingProjectId = ref<string | null>(null)
const showProjectSwitch = ref(false)
const projectSelectValue = computed<string>({
  get: () => draft.value?.projectId || '',
  set: (val) => {
    if (!draft.value) return
    const old = draft.value.projectId || ''
    if (val === old) return
    if (old) {
      pendingProjectId.value = val
      showProjectSwitch.value = true
      return
    }
    draft.value.projectId = val
  },
})
const pendingProjectLabel = computed(() =>
  pendingProjectId.value ? projectNameById(pendingProjectId.value) : '',
)
function confirmProjectChange() {
  if (draft.value && pendingProjectId.value !== null) {
    draft.value.projectId = pendingProjectId.value
  }
  pendingProjectId.value = null
  showProjectSwitch.value = false
}
function cancelProjectChange() {
  pendingProjectId.value = null
  showProjectSwitch.value = false
}

// Sandbox-protocol layout defaults (WSP/1). configRoot / workspaceDir are
// per-Agent, persisted (agent.json) and consumed at sandbox creation to drive
// the bind-mount target + WORKSPACE_DIR. mcp.json / rules/ / skills/ are
// protocol-fixed sub-paths under configRoot, shown derived (read-only).
const DEFAULT_CONFIG_ROOT = '/root/.cursor'
const DEFAULT_WORKSPACE_DIR = '/root/workspace'

let configRootTouched = false

function defaultConfigRootFor(backend: BackendId): string {
  return ACP_BACKENDS.find((b) => b.id === backend)?.configRoot || DEFAULT_CONFIG_ROOT
}

function selectAcpBackend(id: BackendId) {
  if (!draft.value) return
  const prev = draft.value.acpBackend
  draft.value.acpBackend = id
  if (!configRootTouched) {
    draft.value.layout.configRoot = defaultConfigRootFor(id)
  }
  if (prev !== id) draft.value.env = recToKV(switchBackendRegions(kvToRec(draft.value.env), id))
}

const currentAuthHint = computed(() => BACKEND_AUTH_HINTS[draft.value?.acpBackend || 'cursor'])

const envHelpOpen = ref(false)
const envHelpSection = ref<EnvCredentialHelpSection>('inject')

function openEnvHelp(section: EnvCredentialHelpSection) {
  envHelpSection.value = section
  envHelpOpen.value = true
}

function upsertEnv(key: string, value: string) {
  if (!draft.value) return
  const row = draft.value.env.find((e) => e.k === key)
  if (row) row.v = value
  else draft.value.env.push({ k: key, v: value })
}

function storedRegion(): string {
  if (!draft.value) return ''
  const policy = getRegionPolicy(draft.value.acpBackend)
  if (!policy) return ''
  return draft.value.env.find((e) => e.k === policy.regionEnvKey)?.v || ''
}

const currentRegionPolicy = computed(() =>
  draft.value ? getRegionPolicy(draft.value.acpBackend) : undefined,
)
const showMetaRegionBlock = computed(() => !!currentRegionPolicy.value)

const metaRegionOptions = computed(() => currentRegionPolicy.value?.options || [])

const displayRegion = computed(() => {
  const backend = draft.value?.acpBackend
  if (!backend) return ''
  const normalized = normalizeRegions(kvToRec(draft.value!.env), backend, 'preserve-special')
  return normalized.special ? '' : normalized.region
})
const specialRegion = computed(() => {
  const backend = draft.value?.acpBackend
  if (!backend) return ''
  const normalized = normalizeRegions(kvToRec(draft.value!.env), backend, 'preserve-special')
  return normalized.special ? normalized.region : ''
})

function selectRegion(id: string) {
  if (!draft.value) return
  draft.value.env = recToKV(setRegion(kvToRec(draft.value.env), draft.value.acpBackend, id))
}

function joinConfigPath(root: string, sub: string): string {
  return (root || DEFAULT_CONFIG_ROOT).replace(/\/+$/, '') + '/' + sub
}

// Derived (read-only) sub-paths under the current configRoot, for the meta tab.
const derivedPaths = computed(() => {
  const root = draft.value?.layout.configRoot || DEFAULT_CONFIG_ROOT
  return [
    { label: t('pages.agentStudio.configPaths.mcp'), path: joinConfigPath(root, 'mcp.json'), note: t('pages.agentStudio.configPaths.mcpNote') },
    { label: t('pages.agentStudio.configPaths.rules'), path: joinConfigPath(root, 'rules/'), note: t('pages.agentStudio.configPaths.rulesNote') },
    { label: t('pages.agentStudio.configPaths.skills'), path: joinConfigPath(root, 'skills/'), note: t('pages.agentStudio.configPaths.skillsNote') },
    { label: t('pages.agentStudio.configPaths.commands'), path: joinConfigPath(root, 'commands/'), note: t('pages.agentStudio.configPaths.commandsNote') },
    { label: t('pages.agentStudio.configPaths.env'), path: 'container-env', note: t('pages.agentStudio.configPaths.envNote') },
  ]
})
const loading = ref(true)
const error = ref('')
const saving = ref(false)

// working-dir file tree state. The active/open files are tracked by object
// reference (not path) so renaming a file's path never drops the selection.
const activeFile = ref<DraftFile | null>(null)
const openTabs = ref<DraftFile[]>([])
const expanded = ref<Set<string>>(new Set())
const emptyDirs = ref<Set<string>>(new Set())
const renamingPath = ref('')
const renameInput = ref('')
// VSCode-style inline new entry: an input row rendered in the tree under the
// target directory (dir==='' is root) instead of a modal prompt.
const creating = ref<{ dir: string; kind: 'file' | 'folder' } | null>(null)
const createInput = ref('')
const mcpRaw = ref(false)
const envRaw = ref(false)
const mcpRawText = ref('')
const envRawText = ref('')
const rawError = ref('')

const folderInput = ref<HTMLInputElement | null>(null)
const uploadTargetDir = ref('')
const selectedTreeRow = ref<TreeRow | null>(null)
const ctxMenu = ref<{ open: boolean; x: number; y: number; target: CtxTarget | null }>({
  open: false,
  x: 0,
  y: 0,
  target: null,
})
const toastMsg = ref('')
let toastTimer: ReturnType<typeof setTimeout> | null = null

const platformRuleItems = ref<PlatformRuleMeta[]>([])
const platformRuleFile = ref('')
const platformRuleContent = ref('')
const platformRuleLoading = ref(false)
const platformRuleSaving = ref(false)
const platformRuleError = ref('')

// generic dialogs
type PromptCfg = {
  title: string
  label: string
  placeholder?: string
  /** Soft info line (e.g. rename cascade hint); not a warning. */
  hint?: string
  /** When true, show green "valid" feedback after write-rule checks pass. */
  showValidHint?: boolean
  validate: (v: string) => string
  submit: (v: string) => void | Promise<void>
}
const promptCfg = ref<PromptCfg | null>(null)
const promptValue = ref('')
const promptError = ref('')
const promptOkMsg = ref('')
const promptCanSubmit = computed(() => {
  if (!promptCfg.value) return false
  const v = promptValue.value
  if (!v.trim()) return false
  return !promptCfg.value.validate(v)
})

function refreshPromptFeedback() {
  const c = promptCfg.value
  if (!c) {
    promptError.value = ''
    promptOkMsg.value = ''
    return
  }
  const v = promptValue.value
  if (!v.trim()) {
    promptError.value = ''
    promptOkMsg.value = ''
    return
  }
  const err = c.validate(v)
  if (err) {
    promptError.value = err
    promptOkMsg.value = ''
  } else {
    promptError.value = ''
    promptOkMsg.value = c.showValidHint ? t('pages.agentStudio.dialogs.nameValid') : ''
  }
}
type ConfirmCfg = { title: string; message: string; confirmText?: string; danger?: boolean; ok: () => void | Promise<void> }
const confirmCfg = ref<ConfirmCfg | null>(null)
const showAgentManage = ref(false)
/** Agent management 打开时高亮/滚动到的目标行；关闭时清除 */
const manageFocusAgent = ref('')
/** 侧栏铅笔阻断弹窗 */
const showRenameBlocked = ref(false)
const renameBlockedTarget = ref('')

const showUnsavedExport = ref(false)
const exporting = ref(false)
const showFolderSecrets = ref(false)
const pendingFolderExportGroupId = ref('')

const agentImport = useAgentImport({
  dirty: () => agentDirty.value,
  agentDirty: () => agentDirty.value,
  orgDirty: () => orgDirty.value,
  persistOrg: () => persistOrg(org.value),
  agentNames: () => agents.value.map((a) => a.name),
  onImported: async (agent) => {
    const i = agents.value.findIndex((a) => a.name === agent.name)
    if (i >= 0) agents.value[i] = agent
    else {
      agents.value.push(agent)
      agents.value.sort((a, b) => a.name.localeCompare(b.name))
    }
    // ZIP import carries no org → agent stays ungrouped.
    await reloadOrg()
    select(agent.name)
  },
  onFolderImported: async (importedOrg) => {
    org.value = importedOrg
    orgBaseline.value = orgSnapshot(importedOrg)
    try {
      const list = await api.listAgents()
      agents.value = list || []
      if (activeName.value && agents.value.some((a) => a.name === activeName.value)) {
        const a = agents.value.find((x) => x.name === activeName.value)!
        const loaded = toDraft(a)
        originalJson.value = JSON.stringify(fromDraftRaw(loaded))
        normalizeDraftRegions(loaded)
        draft.value = loaded
      } else if (agents.value.length) {
        select(agents.value[0].name)
      }
    } catch (e: any) {
      error.value = String(e?.message || e)
    }
  },
})
const {
  fileInput: importFileInput,
  showDiscardConfirm: showImportDiscardConfirm,
  showConflict: showImportConflict,
  showImportError: showImportErrorModal,
  importError: importErrorMessage,
  conflictName: importConflictName,
  conflictAction: importConflictAction,
  renameValue: importRenameValue,
  renameError: importRenameError,
  showBatchConflict,
  batchConflictNames,
  triggerImport,
  triggerGroupImport,
  onDiscardCancel: onImportDiscardCancel,
  onDiscardConfirm: onImportDiscardConfirm,
  handleFileChange: onImportFileChange,
  selectConflict: selectImportConflict,
  closeConflict: closeImportConflict,
  confirmConflict: confirmImportConflict,
  closeBatchConflict,
  confirmBatchRename,
  confirmBatchOverwrite,
} = agentImport

function recToKV(rec?: Record<string, string>): KV[] {
  return Object.entries(rec || {}).map(([k, v]) => ({ k, v }))
}
function kvToRec(kvs: KV[]): Record<string, string> {
  const out: Record<string, string> = {}
  for (const { k, v } of kvs) if (k.trim()) out[k.trim()] = v
  return out
}
function normalizeDraftRegions(d: Draft): void {
  d.env = recToKV(normalizeRegions(kvToRec(d.env), d.acpBackend, 'preserve-special').env)
}
function apiMcpToDraft(m: MCPServer): DraftMCP {
  return {
    name: m.name,
    transport: m.command ? 'command' : 'url',
    url: m.url ?? '',
    headers: recToKV(m.headers),
    command: m.command ?? '',
    args: (m.args || []).join('\n'),
    env: recToKV(m.env),
  }
}
function draftMcpToApi(m: DraftMCP): MCPServer {
  if (m.transport === 'command') {
    return { name: m.name.trim(), command: m.command.trim(), args: m.args.split('\n').map((s) => s.trim()).filter(Boolean), env: kvToRec(m.env) }
  }
  return { name: m.name.trim(), url: m.url.trim(), headers: kvToRec(m.headers) }
}

function toDraft(a: Agent): Draft {
  const prompts = emptyPrompts()
  for (const k of PROMPT_KEYS) prompts[k] = a.prompts?.[k] ?? ''
  return {
    name: a.name,
    projectId: a.projectId || '',
    acpBackend: (a.acpBackend as BackendId) || 'cursor',
    gitCredentialType: a.gitCredentialType,
    files: (a.files || []).map((f) => ({ path: f.path, content: f.content })),
    mcp: (a.mcp || []).map(apiMcpToDraft),
    env: recToKV(a.env),
    layout: {
      configRoot: a.layout?.configRoot || DEFAULT_CONFIG_ROOT,
      workspaceDir: a.layout?.workspaceDir || DEFAULT_WORKSPACE_DIR,
    },
    prompts,
  }
}
// Collapse the prompt draft to only non-empty fields; return undefined when the
// Agent uses platform defaults for everything (keeps agent.json clean).
function draftPromptsToApi(p: PromptDraft): AgentPrompts | undefined {
  const out: AgentPrompts = {}
  let any = false
  for (const k of PROMPT_KEYS) {
    if (p[k].trim()) {
      out[k] = p[k]
      any = true
    }
  }
  return any ? out : undefined
}
function fromDraftRaw(d: Draft): Agent {
  return {
    name: d.name,
    // Always send projectId (including "") so unbind is explicit; omitted field
    // is preserved server-side and must not be used for intentional clear.
    projectId: d.projectId?.trim() || '',
    acpBackend: d.acpBackend || 'cursor',
    ...(d.gitCredentialType ? { gitCredentialType: d.gitCredentialType } : {}),
    files: d.files.filter((f) => f.path.trim()).map((f) => ({ path: f.path.trim(), content: f.content })),
    mcp: d.mcp.filter((m) => m.name.trim()).map(draftMcpToApi),
    env: kvToRec(d.env),
    layout: {
      configRoot: d.layout.configRoot.trim() || DEFAULT_CONFIG_ROOT,
      workspaceDir: d.layout.workspaceDir.trim() || DEFAULT_WORKSPACE_DIR,
    },
    prompts: draftPromptsToApi(d.prompts),
  }
}
function fromDraft(d: Draft): Agent {
  const payload = fromDraftRaw(d)
  payload.env = normalizeRegions(payload.env || {}, d.acpBackend, 'preserve-special').env
  return payload
}

function orgSnapshot(o: AgentOrg): string {
  return JSON.stringify({
    revision: o.revision,
    groups: o.groups || [],
    agents: o.agents || {},
  })
}

const agentDirty = computed(
  () => !!draft.value && JSON.stringify(fromDraft(draft.value)) !== originalJson.value,
)
const orgDirty = computed(() => orgSnapshot(org.value) !== orgBaseline.value)
const dirty = computed(() => agentDirty.value || orgDirty.value)

watch(dirty, (d) => {
  if (d) justSaved.value = false
})

watch(isMobile, (mobile) => {
  if (mobile) {
    // Keep file context when crossing the breakpoint; default to list if none open.
    filesStep.value = activeFile.value ? 'edit' : 'list'
  } else {
    // Desktop sidebar owns org navigation — close sheet to avoid dual entry.
    showOrgSheet.value = false
  }
})

const agentNames = computed(() => agents.value.map((a) => a.name))

const orgSheetRows = computed(() =>
  buildOrgTreeRows(org.value, agentNames.value, orgSheetCollapsed.value),
)

function openOrgSheet() {
  showOrgSheet.value = true
}

function closeOrgSheet() {
  showOrgSheet.value = false
}

function toggleOrgSheetNode(id: string) {
  const next = new Set(orgSheetCollapsed.value)
  if (next.has(id)) next.delete(id)
  else next.add(id)
  orgSheetCollapsed.value = next
}

function orgSheetPadStyle(depth: number) {
  return { paddingLeft: `${6 + depth * 14}px` }
}

const metaGroupTiles = computed(() => {
  const list = [...(org.value.groups || [])]
  list.sort((a, b) => groupPath(org.value, a.id).localeCompare(groupPath(org.value, b.id)))
  return list.map((g) => ({
    id: g.id,
    name: g.name,
    path: groupPath(org.value, g.id),
    selected: activeName.value ? groupIdsOf(org.value, activeName.value).includes(g.id) : false,
  }))
})

const metaParentOptions = computed(() =>
  agentNames.value.filter((n) => n !== activeName.value),
)

const metaParentAgent = computed(() => (activeName.value ? parentOf(org.value, activeName.value) : ''))

function toggleMetaGroup(groupId: string) {
  if (!activeName.value) return
  const cur = groupIdsOf(org.value, activeName.value)
  const next = cur.includes(groupId) ? cur.filter((id) => id !== groupId) : [...cur, groupId]
  org.value = setAgentMembership(org.value, activeName.value, next, parentOf(org.value, activeName.value))
}

function setMetaParent(parent: string) {
  if (!activeName.value) return
  if (parent && wouldCreateReportingCycle(org.value, activeName.value, parent)) {
    error.value = t('pages.agentStudio.org.reportingCycle')
    parentDropdownOpen.value = false
    return
  }
  error.value = ''
  org.value = setAgentMembership(
    org.value,
    activeName.value,
    groupIdsOf(org.value, activeName.value),
    parent,
  )
  parentDropdownOpen.value = false
}

async function persistOrg(next: AgentOrg): Promise<boolean> {
  orgSaving.value = true
  error.value = ''
  try {
    const saved = await api.saveAgentsOrg({
      revision: org.value.revision,
      groups: next.groups || [],
      agents: next.agents || {},
    })
    org.value = saved
    orgBaseline.value = orgSnapshot(saved)
    return true
  } catch (e: any) {
    error.value = String(e?.message || e)
    // Reload authoritative org on conflict / validation failure.
    try {
      const fresh = await api.getAgentsOrg()
      org.value = fresh
      orgBaseline.value = orgSnapshot(fresh)
    } catch {
      /* ignore */
    }
    return false
  } finally {
    orgSaving.value = false
  }
}

async function reloadOrg() {
  try {
    const o = (await api.getAgentsOrg()) || emptyOrg()
    if (!o.agents) o.agents = {}
    if (!o.groups) o.groups = []
    org.value = o
    orgBaseline.value = orgSnapshot(o)
  } catch (e: any) {
    error.value = String(e?.message || e)
  }
}

function openCreateRootGroup() {
  promptValue.value = ''
  promptError.value = ''
  promptOkMsg.value = ''
  promptCfg.value = {
    title: t('pages.agentStudio.org.newRootGroup'),
    label: t('pages.agentStudio.org.groupNameLabel'),
    placeholder: t('pages.agentStudio.org.groupNamePlaceholder'),
    validate: (v) => (!v ? t('pages.agentStudio.dialogs.nameRequired') : ''),
    submit: async (v) => {
      const next = {
        ...org.value,
        groups: [...(org.value.groups || []), { id: newGroupId(), name: v }],
      }
      if (!(await persistOrg(next))) throw new Error(error.value || 'org save failed')
    },
  }
}

function openCreateChildGroup(parentId: string) {
  promptValue.value = ''
  promptError.value = ''
  promptOkMsg.value = ''
  promptCfg.value = {
    title: t('pages.agentStudio.org.newChildGroup'),
    label: t('pages.agentStudio.org.groupNameLabel'),
    placeholder: t('pages.agentStudio.org.groupNamePlaceholder'),
    validate: (v) => (!v ? t('pages.agentStudio.dialogs.nameRequired') : ''),
    submit: async (v) => {
      const next = {
        ...org.value,
        groups: [...(org.value.groups || []), { id: newGroupId(), name: v, parentGroupId: parentId }],
      }
      if (!(await persistOrg(next))) throw new Error(error.value || 'org save failed')
    },
  }
}

function openRenameGroup(groupId: string) {
  const g = (org.value.groups || []).find((x) => x.id === groupId)
  if (!g) return
  promptValue.value = g.name
  promptError.value = ''
  promptOkMsg.value = ''
  promptCfg.value = {
    title: t('pages.agentStudio.org.renameGroup'),
    label: t('pages.agentStudio.org.groupNameLabel'),
    validate: (v) => (!v ? t('pages.agentStudio.dialogs.nameRequired') : ''),
    submit: async (v) => {
      const next = {
        ...org.value,
        groups: (org.value.groups || []).map((x) => (x.id === groupId ? { ...x, name: v } : x)),
      }
      if (!(await persistOrg(next))) throw new Error(error.value || 'org save failed')
    },
  }
}

function confirmDeleteGroup(groupId: string) {
  const g = (org.value.groups || []).find((x) => x.id === groupId)
  if (!g) return
  confirmCfg.value = {
    title: t('pages.agentStudio.org.deleteGroupTitle'),
    message: t('pages.agentStudio.org.deleteGroupMessage', { name: g.name }),
    confirmText: t('pages.agentStudio.dialogs.delete'),
    danger: true,
    ok: async () => {
      const next = applyDeleteGroup(org.value, groupId)
      await persistOrg(next)
    },
  }
}

async function onMoveGroup(groupId: string, newParentId: string) {
  if (wouldCreateGroupCycle(org.value, groupId, newParentId)) {
    error.value = t('pages.agentStudio.org.groupCycle')
    return
  }
  const next = {
    ...org.value,
    groups: (org.value.groups || []).map((g) =>
      g.id === groupId ? { ...g, parentGroupId: newParentId || undefined } : g,
    ),
  }
  await persistOrg(next)
}

async function onMoveAgent(agentName: string, sourceGroupId: string, targetGroupId: string) {
  const next = applyMoveAgent(org.value, agentName, sourceGroupId, targetGroupId)
  await persistOrg(next)
}

async function onRemoveFromGroup(agentName: string, groupId: string) {
  const next = applyRemoveAgentFromGroup(org.value, agentName, groupId)
  if (await persistOrg(next)) {
    showToast(t('pages.agentStudio.org.removeFromGroupToast'))
  }
}
const promptCount = computed(() => (draft.value ? PROMPT_KEYS.filter((k) => draft.value!.prompts[k].trim()).length : 0))
const studioTabs = computed(() => {
  if (!draft.value) return []
  const d = draft.value
  const pc = promptCount.value
  return [
    { k: 'files' as const, l: t('pages.agentStudio.tabs.files', { n: d.files.length }) },
    { k: 'mcp' as const, l: t('pages.agentStudio.tabs.mcp', { n: d.mcp.length }) },
    { k: 'env' as const, l: t('pages.agentStudio.tabs.env', { n: d.env.filter((e) => !isManagedRegionKey(e.k)).length }) },
    { k: 'prompts' as const, l: pc ? t('pages.agentStudio.tabs.promptsCount', { n: pc }) : t('pages.agentStudio.tabs.prompts') },
    { k: 'platform-rules' as const, l: t('pages.agentStudio.tabs.platformRules') },
    { k: 'test' as const, l: t('pages.agentStudio.tabs.test') },
    { k: 'data' as const, l: t('pages.agentStudio.tabs.data') },
    { k: 'meta' as const, l: t('pages.agentStudio.tabs.meta') },
  ]
})
const studioTabLabel = computed(() => {
  const item = studioTabs.value.find((x) => x.k === tab.value)
  return item?.l || tab.value
})
const hasArtifactStore = computed(() => !!draft.value?.mcp.some((m) => m.name.trim() === ARTIFACT_STORE))
const hasLegacyPmLeader = computed(() => !!draft.value?.mcp.some((m) => m.name.trim() === LEGACY_PM_LEADER))
function isLegacyPmLeaderName(name: string) {
  return name.trim() === LEGACY_PM_LEADER
}
function isAgentPlatformMcpName(name: string) {
  const n = name.trim()
  return n === MEMORY_STORE || n === CONTEXT_STORE || n === TASK_SCHEDULER
}
function hasAgentPlatformMcp(name: string) {
  return !!draft.value?.mcp.some((m) => m.name.trim() === name)
}
const currentFile = computed(() => activeFile.value)
const activePath = computed(() => activeFile.value?.path || '')
const breadcrumb = computed(() => activePath.value.split('/').filter(Boolean))

function langForPath(p: string): string {
  const ext = p.split('.').pop()?.toLowerCase() || ''
  return ({ md: 'markdown', markdown: 'markdown', json: 'json', sh: 'shell', bash: 'shell', zsh: 'shell', js: 'javascript', mjs: 'javascript', ts: 'typescript', py: 'python', yml: 'yaml', yaml: 'yaml', toml: 'ini', txt: 'plaintext' } as Record<string, string>)[ext] || 'plaintext'
}

function isMdPath(p: string): boolean {
  return p.toLowerCase().endsWith('.md')
}

const activePlatformRule = computed(() => platformRuleItems.value.find((x) => x.file === platformRuleFile.value))
const platformRuleOverridden = computed(() => activePlatformRule.value?.source === 'override')
const platformRuleOverrideCount = computed(() => platformRuleItems.value.filter((x) => x.source === 'override').length)

async function loadPlatformRules() {
  if (!activeName.value) return
  platformRuleLoading.value = true
  platformRuleError.value = ''
  try {
    const res = await api.listAgentPlatformRules(activeName.value)
    platformRuleItems.value = res.items
    if (!platformRuleFile.value && res.items.length) {
      platformRuleFile.value = res.items[0].file
    }
    if (platformRuleFile.value) {
      const item = await api.getAgentPlatformRule(activeName.value, platformRuleFile.value)
      platformRuleContent.value = item.content
    }
  } catch (e: any) {
    platformRuleError.value = e?.message || t('pages.agentStudio.platformRules.loadFailed')
  } finally {
    platformRuleLoading.value = false
  }
}

async function selectPlatformRuleFile(file: string) {
  if (!activeName.value) return
  platformRuleFile.value = file
  platformRuleError.value = ''
  try {
    const item = await api.getAgentPlatformRule(activeName.value, file)
    platformRuleContent.value = item.content
  } catch (e: any) {
    platformRuleError.value = e?.message || t('pages.agentStudio.platformRules.loadFailed')
  }
}

async function createPlatformRuleOverride() {
  if (!activeName.value || !platformRuleFile.value) return
  platformRuleSaving.value = true
  platformRuleError.value = ''
  try {
    const item = await api.saveAgentPlatformRule(activeName.value, platformRuleFile.value, platformRuleContent.value)
    platformRuleContent.value = item.content
    await loadPlatformRules()
    showToast(t('pages.agentStudio.platformRules.overrideCreated'))
  } catch (e: any) {
    platformRuleError.value = e?.message || t('pages.agentStudio.platformRules.saveFailed')
  } finally {
    platformRuleSaving.value = false
  }
}

async function savePlatformRuleOverride() {
  if (!activeName.value || !platformRuleFile.value || !platformRuleOverridden.value) return
  platformRuleSaving.value = true
  platformRuleError.value = ''
  try {
    const item = await api.saveAgentPlatformRule(activeName.value, platformRuleFile.value, platformRuleContent.value)
    platformRuleContent.value = item.content
    await loadPlatformRules()
    showToast(t('pages.agentStudio.platformRules.saved'))
  } catch (e: any) {
    platformRuleError.value = e?.message || t('pages.agentStudio.platformRules.saveFailed')
  } finally {
    platformRuleSaving.value = false
  }
}

async function deletePlatformRuleOverride() {
  if (!activeName.value || !platformRuleFile.value || !platformRuleOverridden.value) return
  platformRuleSaving.value = true
  platformRuleError.value = ''
  try {
    await api.deleteAgentPlatformRule(activeName.value, platformRuleFile.value)
    await loadPlatformRules()
    if (platformRuleFile.value) {
      const item = await api.getAgentPlatformRule(activeName.value, platformRuleFile.value)
      platformRuleContent.value = item.content
    }
    showToast(t('pages.agentStudio.platformRules.overrideDeleted'))
  } catch (e: any) {
    platformRuleError.value = e?.message || t('pages.agentStudio.platformRules.deleteFailed')
  } finally {
    platformRuleSaving.value = false
  }
}

function onPlatformRulesTab() {
  if (tab.value === 'platform-rules') loadPlatformRules()
}

function showToast(msg: string) {
  toastMsg.value = msg
  if (toastTimer) clearTimeout(toastTimer)
  toastTimer = setTimeout(() => {
    toastMsg.value = ''
  }, 2800)
}

function hideCtxMenu() {
  ctxMenu.value.open = false
  ctxMenu.value.target = null
}

function openCtxMenu(e: MouseEvent, target: CtxTarget) {
  e.preventDefault()
  e.stopPropagation()
  selectedTreeRow.value = target.blank || target.dir
    ? null
    : { name: target.name, path: target.path, dir: false, depth: 0 }
  ctxMenu.value = { open: true, x: e.clientX, y: e.clientY, target }
  nextTick(() => {
    const menu = document.querySelector('.explorer-ctx-menu') as HTMLElement | null
    if (!menu) return
    const rect = menu.getBoundingClientRect()
    ctxMenu.value.x = Math.min(e.clientX, window.innerWidth - rect.width - 8)
    ctxMenu.value.y = Math.min(e.clientY, window.innerHeight - rect.height - 8)
  })
}

function onExplorerBlankCtx(e: MouseEvent) {
  openCtxMenu(e, { dir: true, path: '', name: t('pages.agentStudio.configPaths.root'), blank: true })
}

function onCtxAction(action: string) {
  const target = ctxMenu.value.target
  if (!target) return
  const parentDir = target.path.includes('/') ? target.path.slice(0, target.path.lastIndexOf('/')) : ''
  const row: TreeRow = { name: target.name, path: target.path, dir: target.dir, depth: 0 }

  switch (action) {
    case 'newFile':
      newFile(target.blank ? '' : target.dir ? target.path : parentDir)
      break
    case 'newFolder':
      newFolder(target.blank ? '' : target.dir ? target.path : parentDir)
      break
    case 'uploadFolder':
      if (!target.dir || target.blank) return
      uploadTargetDir.value = target.path
      folderInput.value?.click()
      break
    case 'rename':
      if (target.blank) return
      startRename(row)
      break
    case 'copyPath':
      if (target.dir) return
      navigator.clipboard?.writeText(target.path).then(
        () => showToast(t('common.toast.pathCopied', { path: target.path })),
        () => showToast(t('common.toast.copyPathFailed')),
      )
      break
    case 'delete':
      if (target.path === 'rules' || target.path === 'skills') return
      deleteEntry(row)
      break
  }
  hideCtxMenu()
}

function onExplorerKeydown(e: KeyboardEvent) {
  if (e.key === 'F2' && selectedTreeRow.value && !selectedTreeRow.value.dir) {
    e.preventDefault()
    startRename(selectedTreeRow.value)
  }
  if (e.key === 'Escape') hideCtxMenu()
}

// --- working-dir file tree -------------------------------------------------
type TreeNode = { name: string; path: string; dir: boolean; children: Record<string, TreeNode> }
type TreeRow = { name: string; path: string; dir: boolean; depth: number }

function joinPath(dir: string, name: string): string {
  return [dir, name].filter(Boolean).join('/').split('/').map((s) => s.trim()).filter(Boolean).join('/')
}
function expandParents(p: string) {
  const segs = p.split('/').filter(Boolean)
  let acc = ''
  for (let i = 0; i < segs.length - 1; i++) {
    acc = acc ? `${acc}/${segs[i]}` : segs[i]
    expanded.value.add(acc)
  }
}

const tree = computed<TreeNode>(() => {
  const root: TreeNode = { name: '', path: '', dir: true, children: {} }
  const add = (path: string, isFile: boolean) => {
    const segs = path.split('/').filter(Boolean)
    let cur = root
    let acc = ''
    segs.forEach((seg, i) => {
      acc = acc ? `${acc}/${seg}` : seg
      const leaf = isFile && i === segs.length - 1
      if (!cur.children[seg]) cur.children[seg] = { name: seg, path: acc, dir: !leaf, children: {} }
      cur = cur.children[seg]
    })
  }
  for (const f of draft.value?.files || []) add(f.path, true)
  for (const d of emptyDirs.value) add(d, false)
  return root
})

const rows = computed<TreeRow[]>(() => {
  const out: TreeRow[] = []
  const walk = (node: TreeNode, depth: number) => {
    const kids = Object.values(node.children).sort((a, b) =>
      a.dir === b.dir ? a.name.localeCompare(b.name) : a.dir ? -1 : 1,
    )
    for (const k of kids) {
      out.push({ name: k.name, path: k.path, dir: k.dir, depth })
      if (k.dir && expanded.value.has(k.path)) walk(k, depth + 1)
    }
  }
  walk(tree.value, 0)
  return out
})

// open a file in the editor (and as a tab), tracking it by reference.
function openFile(f: DraftFile) {
  activeFile.value = f
  if (!openTabs.value.includes(f)) openTabs.value.push(f)
  expandParents(f.path)
  selectedTreeRow.value = { name: f.path.split('/').pop() || f.path, path: f.path, dir: false, depth: 0 }
  if (isMobile.value) filesStep.value = 'edit'
}
function openPath(path: string) {
  const f = draft.value?.files.find((x) => x.path === path)
  if (f) openFile(f)
}

function discardUnsavedChanges() {
  const path = activeFile.value?.path || ''
  const openPaths = openTabs.value.map((f) => f.path)
  resetOrgFromBaseline()
  const a = agents.value.find((x) => x.name === activeName.value)
  if (!a) return
  const loaded = toDraft(a)
  originalJson.value = JSON.stringify(fromDraftRaw(loaded))
  normalizeDraftRegions(loaded)
  draft.value = loaded
  openTabs.value = openPaths
    .map((p) => loaded.files.find((f) => f.path === p))
    .filter((f): f is DraftFile => !!f)
  activeFile.value = path ? loaded.files.find((f) => f.path === path) || null : null
  justSaved.value = false
  error.value = ''
}

function goFilesList() {
  filesStep.value = 'list'
}

function tryBackToList() {
  if (!dirty.value) {
    goFilesList()
    return
  }
  leaveConfirmCfg.value = {
    title: t('pages.agentStudio.dialogs.leaveUnsavedTitle'),
    message: t('pages.agentStudio.dialogs.leaveUnsavedBackMessage'),
    saveText: t('pages.agentStudio.dialogs.saveAndBack'),
    discardText: t('pages.agentStudio.dialogs.discardChanges'),
    onSave: async () => {
      const ok = await save()
      if (!ok) return false
      justSaved.value = true
      goFilesList()
      return true
    },
    onDiscard: () => {
      discardUnsavedChanges()
      goFilesList()
    },
  }
}

function requestStudioTab(next: StudioTab) {
  if (next === tab.value) {
    if (next === 'platform-rules') onPlatformRulesTab()
    return
  }
  const leavingDirtyEdit =
    isMobile.value && tab.value === 'files' && filesStep.value === 'edit' && dirty.value
  if (leavingDirtyEdit) {
    leaveConfirmCfg.value = {
      title: t('pages.agentStudio.dialogs.leaveUnsavedTitle'),
      message: t('pages.agentStudio.dialogs.leaveUnsavedTabMessage'),
      saveText: t('pages.agentStudio.dialogs.saveAndContinue'),
      discardText: t('pages.agentStudio.dialogs.discardChanges'),
      onSave: async () => {
        const ok = await save()
        if (!ok) return false
        justSaved.value = true
        tab.value = next
        if (next === 'platform-rules') onPlatformRulesTab()
        syncStudioQuery()
        return true
      },
      onDiscard: () => {
        discardUnsavedChanges()
        tab.value = next
        if (next === 'platform-rules') onPlatformRulesTab()
        syncStudioQuery()
      },
    }
    return
  }
  tab.value = next
  if (next === 'platform-rules') onPlatformRulesTab()
  syncStudioQuery()
}

function onDataSubTab(next: DataSubTab) {
  dataSubTab.value = next
  syncStudioQuery()
}

async function leaveConfirmSave() {
  const c = leaveConfirmCfg.value
  if (!c) return
  try {
    const ok = await c.onSave()
    if (!ok) return
  } catch (e: any) {
    error.value = String(e?.message || e)
    return
  }
  leaveConfirmCfg.value = null
}

function leaveConfirmDiscard() {
  const c = leaveConfirmCfg.value
  if (!c) return
  c.onDiscard()
  leaveConfirmCfg.value = null
}

function leaveConfirmCancel() {
  leaveConfirmCfg.value = null
}
function closeTab(f: DraftFile) {
  const i = openTabs.value.indexOf(f)
  if (i < 0) return
  openTabs.value.splice(i, 1)
  if (activeFile.value === f) activeFile.value = openTabs.value[i] || openTabs.value[i - 1] || openTabs.value[openTabs.value.length - 1] || null
}

// inline rename (VSCode F2): edits the leaf name in place; folders rewrite the
// prefix of every contained file.
function startRename(row: TreeRow) {
  renamingPath.value = row.path
  renameInput.value = row.name
  nextTick(() => {
    const el = document.querySelector<HTMLInputElement>('input[data-rename]')
    el?.focus()
    el?.select()
  })
}
function cancelRename() {
  renamingPath.value = ''
}
function commitRename(row: TreeRow) {
  if (renamingPath.value !== row.path) return
  const leaf = renameInput.value.trim().replace(/\//g, '')
  renamingPath.value = ''
  if (!leaf || leaf === row.name) return
  const parent = row.path.includes('/') ? row.path.slice(0, row.path.lastIndexOf('/')) : ''
  const np = joinPath(parent, leaf)
  if (np === row.path) return
  if (draft.value!.files.some((f) => f.path === np)) {
    error.value = t('pages.agentStudio.dialogs.pathExists', { path: np })
    return
  }
  if (row.dir) {
    const pref = row.path + '/'
    draft.value!.files.forEach((f) => {
      if (f.path === row.path || f.path.startsWith(pref)) f.path = np + f.path.slice(row.path.length)
    })
    const ed = new Set<string>()
    expanded.value.forEach((d) => ed.add(d === row.path || d.startsWith(pref) ? np + d.slice(row.path.length) : d))
    expanded.value = ed
    const ned = new Set<string>()
    emptyDirs.value.forEach((d) => ned.add(d === row.path || d.startsWith(pref) ? np + d.slice(row.path.length) : d))
    emptyDirs.value = ned
  } else {
    const f = draft.value!.files.find((x) => x.path === row.path)
    if (f) f.path = np
  }
  expandParents(np)
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    const [list, o, projList] = await Promise.all([api.listAgents(), api.getAgentsOrg(), api.listProjects()])
    agents.value = list || []
    projects.value = (projList || []).map((p) => ({ id: p.id, name: p.name }))
    org.value = o?.groups ? o : { revision: o?.revision || 0, groups: o?.groups || [], agents: o?.agents || {} }
    if (!org.value.agents) org.value.agents = {}
    if (!org.value.groups) org.value.groups = []
    orgBaseline.value = orgSnapshot(org.value)
    applyingStudioQuery = true
    try {
      const qAgent = typeof route.query.agent === 'string' ? route.query.agent.trim() : ''
      const qTab = isStudioTab(route.query.tab) ? route.query.tab : null
      const qSub = parseDataSub(route.query.sub)
      if (qAgent && agents.value.some((a) => a.name === qAgent)) {
        select(qAgent, { tab: qTab || 'files', dataSub: qSub, skipQuerySync: true })
      } else if (agents.value.length && !activeName.value) {
        select(agents.value[0].name, { skipQuerySync: true })
      }
    } finally {
      applyingStudioQuery = false
    }
    syncStudioQuery()
  } catch (e: any) {
    error.value = String(e?.message || e)
    agents.value = []
  } finally {
    loading.value = false
  }
}

function select(
  name: string,
  opts?: { tab?: StudioTab; dataSub?: DataSubTab; skipQuerySync?: boolean },
) {
  configRootTouched = false
  const a = agents.value.find((x) => x.name === name)
  if (!a) return
  activeName.value = name
  const loaded = toDraft(a)
  originalJson.value = JSON.stringify(fromDraftRaw(loaded))
  normalizeDraftRegions(loaded)
  draft.value = loaded
  tab.value = opts?.tab || 'files'
  if (opts?.dataSub) dataSubTab.value = opts.dataSub
  else if (!opts?.tab || opts.tab !== 'data') dataSubTab.value = 'memory'
  filesStep.value = 'list'
  justSaved.value = false
  leaveConfirmCfg.value = null
  expanded.value = new Set()
  emptyDirs.value = new Set()
  openTabs.value = []
  activeFile.value = null
  renamingPath.value = ''
  platformRuleFile.value = ''
  platformRuleItems.value = []
  platformRuleContent.value = ''
  platformRuleError.value = ''
  creating.value = null
  selectDefaultFile()
  mcpRaw.value = envRaw.value = false
  if (!opts?.skipQuerySync) syncStudioQuery()
}

function selectDefaultFile() {
  // Narrow screen starts on the resource list step; do not auto-enter edit.
  if (isMobile.value) {
    activeFile.value = null
    openTabs.value = []
    filesStep.value = 'list'
    return
  }
  const files = [...(draft.value?.files || [])].sort((a, b) => a.path.localeCompare(b.path))
  const target = files.find((f) => f.path.toLowerCase().endsWith('.md')) || files[0]
  if (target) openFile(target)
  else activeFile.value = null
}

function resetOrgFromBaseline() {
  try {
    org.value = JSON.parse(orgBaseline.value || orgSnapshot(emptyOrg()))
  } catch {
    /* keep current */
  }
}

// guard agent switches when there are unsaved changes
function chooseAgent(name: string) {
  if (name === activeName.value) return
  if (dirty.value) {
    confirmCfg.value = {
      title: t('pages.agentStudio.dialogs.discardTitle'),
      message: t('pages.agentStudio.dialogs.discardMessage', { name: activeName.value }),
      confirmText: t('pages.agentStudio.dialogs.discardConfirm'),
      danger: true,
      ok: () => {
        resetOrgFromBaseline()
        select(name)
      },
    }
    return
  }
  select(name)
}

/** Narrow-screen sheet switch: leaveConfirm-style save / discard / cancel. */
function chooseAgentFromSheet(name: string) {
  if (name === activeName.value) {
    closeOrgSheet()
    return
  }
  if (!dirty.value) {
    select(name)
    closeOrgSheet()
    return
  }
  leaveConfirmCfg.value = {
    title: t('pages.agentStudio.dialogs.leaveUnsavedTitle'),
    message: t('pages.agentStudio.dialogs.leaveUnsavedSwitchMessage', { name: activeName.value }),
    saveText: t('pages.agentStudio.dialogs.saveAndSwitch'),
    discardText: t('pages.agentStudio.dialogs.discardAndSwitch'),
    onSave: async () => {
      const ok = await save()
      if (!ok) return false
      justSaved.value = true
      select(name)
      closeOrgSheet()
      return true
    },
    onDiscard: () => {
      resetOrgFromBaseline()
      select(name)
      closeOrgSheet()
    },
  }
}

function openManageFromSheet() {
  openAgentManage()
}

async function save() {
  if (!dirty.value) return false
  saving.value = true
  error.value = ''
  try {
    if (draft.value && JSON.stringify(fromDraft(draft.value)) !== originalJson.value) {
      const payload = fromDraft(draft.value)
      await api.saveAgent(payload)
      const i = agents.value.findIndex((x) => x.name === payload.name)
      if (i >= 0) agents.value[i] = payload
      originalJson.value = JSON.stringify(payload)
    }
    if (orgSnapshot(org.value) !== orgBaseline.value) {
      const ok = await persistOrg(org.value)
      if (!ok) return false
    }
    justSaved.value = true
    return true
  } catch (e: any) {
    error.value = String(e?.message || e)
    return false
  } finally {
    saving.value = false
  }
}

async function doExport(name: string) {
  exporting.value = true
  error.value = ''
  try {
    const blob = await api.exportAgent(name)
    downloadZip(blob, `${name}.zip`)
    showToast(t('pages.agentStudio.exportImport.exportSuccess', { name }))
  } catch (e: any) {
    error.value = String(e?.message || e)
  } finally {
    exporting.value = false
  }
}

function triggerExport() {
  if (!activeName.value || exporting.value) return
  pendingFolderExportGroupId.value = ''
  if (dirty.value) {
    showUnsavedExport.value = true
    return
  }
  void doExport(activeName.value)
}

function cancelUnsavedExport() {
  showUnsavedExport.value = false
  pendingFolderExportGroupId.value = ''
}

async function discardAndExport() {
  showUnsavedExport.value = false
  if (pendingFolderExportGroupId.value) {
    showFolderSecrets.value = true
    return
  }
  if (!activeName.value) return
  await doExport(activeName.value)
}

async function saveThenExport() {
  const ok = await save()
  if (!ok) return
  showUnsavedExport.value = false
  if (pendingFolderExportGroupId.value) {
    showFolderSecrets.value = true
    return
  }
  if (activeName.value) await doExport(activeName.value)
}

async function onExportGroup(groupId: string) {
  if (exporting.value) return
  if (orgDirty.value) {
    if (!(await persistOrg(org.value))) return
  }
  const inSubtree =
    !!activeName.value && isAgentInGroupSubtree(org.value, activeName.value, groupId)
  if (inSubtree && agentDirty.value) {
    pendingFolderExportGroupId.value = groupId
    showUnsavedExport.value = true
    return
  }
  pendingFolderExportGroupId.value = groupId
  showFolderSecrets.value = true
}

function cancelFolderSecrets() {
  showFolderSecrets.value = false
  pendingFolderExportGroupId.value = ''
}

async function confirmFolderSecrets() {
  const groupId = pendingFolderExportGroupId.value
  showFolderSecrets.value = false
  pendingFolderExportGroupId.value = ''
  if (!groupId || exporting.value) return
  exporting.value = true
  error.value = ''
  try {
    const { blob, filename } = await api.exportOrgFolder(groupId)
    downloadZip(blob, filename)
    const base = filename.replace(/\.zip$/i, '')
    showToast(t('pages.agentStudio.exportImport.folderExportSuccess', { name: base }))
  } catch (e: any) {
    error.value = String(e?.message || e)
  } finally {
    exporting.value = false
  }
}

function onImportGroup(groupId: string) {
  triggerGroupImport(groupId)
}

const showCreateWizard = ref(false)

function openCreateAgent() {
  showCreateWizard.value = true
}

function onWizardCreated(created: Agent) {
  agents.value.push(created)
  agents.value.sort((a, b) => a.name.localeCompare(b.name))
  // New agents default to ungrouped / no parent (no org entry).
  select(created.name)
  showToast(t('pages.agentStudio.wizard.createdToast', { name: created.name }))
}

function openAgentManage(focusAgentName?: string) {
  manageFocusAgent.value = focusAgentName || ''
  showAgentManage.value = true
  if (focusAgentName) {
    nextTick(() => {
      const el = Array.from(document.querySelectorAll('[data-manage-agent]')).find(
        (n) => n.getAttribute('data-manage-agent') === focusAgentName,
      )
      el?.scrollIntoView({ block: 'nearest', behavior: 'smooth' })
    })
  }
}

function closeAgentManage() {
  showAgentManage.value = false
  manageFocusAgent.value = ''
}

/** 侧栏铅笔：阻断改名并引导至 Agent 管理（不调用 api.renameAgent） */
function onSidebarRenameBlocked(name: string) {
  renameBlockedTarget.value = name
  showRenameBlocked.value = true
}

function closeRenameBlocked() {
  showRenameBlocked.value = false
  renameBlockedTarget.value = ''
}

function gotoManageFromBlocked() {
  const target = renameBlockedTarget.value
  closeRenameBlocked()
  openAgentManage(target)
}

function openRenameAgent(name: string) {
  promptValue.value = name
  promptError.value = ''
  promptOkMsg.value = ''
  promptCfg.value = {
    title: t('pages.agentStudio.dialogs.renameTitle'),
    label: t('pages.agentStudio.dialogs.renameLabel'),
    hint: t('pages.agentStudio.dialogs.renameCascadeHint'),
    showValidHint: true,
    validate: (v) => {
      const code = validateAgentName(v)
      if (code === 'required') return t('pages.agentStudio.dialogs.nameRequired')
      if (code === 'invalid') return t('pages.agentStudio.dialogs.nameInvalid')
      const normalized = normalizeAgentName(v)
      if (normalized !== name && agents.value.some((a) => a.name === normalized)) {
        return t('pages.agentStudio.dialogs.nameExists')
      }
      return ''
    },
    submit: async (v) => {
      const normalized = normalizeAgentName(v)
      if (normalized === name) return
      const updated = await api.renameAgent(name, normalized)
      const n = updated.updatedWorkflowCount ?? 0
      const { updatedWorkflowCount: _count, ...agent } = updated
      agents.value = agents.value.filter((a) => a.name !== name)
      agents.value.push(agent)
      agents.value.sort((a, b) => a.name.localeCompare(b.name))
      await reloadOrg()
      // 管理弹窗内改名后同步 focus 到新名（若仍打开）
      if (manageFocusAgent.value === name) manageFocusAgent.value = agent.name
      select(agent.name)
      if (n > 0) showToast(t('pages.agentStudio.dialogs.renameSuccessWithWorkflows', { n }))
      else showToast(t('pages.agentStudio.dialogs.renameSuccess'))
    },
  }
  refreshPromptFeedback()
}

function confirmDeleteAgent(name: string) {
  confirmCfg.value = {
    title: t('pages.agentStudio.dialogs.deleteAgentTitle'),
    message: t('pages.agentStudio.dialogs.deleteAgentMessage', { name }),
    confirmText: t('pages.agentStudio.dialogs.delete'),
    danger: true,
    ok: async () => {
      await api.deleteAgent(name)
      agents.value = agents.value.filter((a) => a.name !== name)
      await reloadOrg()
      closeAgentManage()
      // FR-f8: clear selection; do not auto-select the next agent
      if (activeName.value === name) {
        activeName.value = ''
        draft.value = null
      }
    },
  }
}

async function promptOk() {
  const c = promptCfg.value
  if (!c) return
  const v = normalizeAgentName(promptValue.value)
  const err = c.validate(promptValue.value)
  if (err) {
    promptError.value = err
    promptOkMsg.value = ''
    return
  }
  try {
    await c.submit(v)
    promptCfg.value = null
  } catch (e: any) {
    promptError.value = String(e?.message || e)
    promptOkMsg.value = ''
  }
}

async function confirmOk() {
  const c = confirmCfg.value
  if (!c) return
  try {
    await c.ok()
  } catch (e: any) {
    error.value = String(e?.message || e)
  }
  confirmCfg.value = null
}

// --- working-dir file operations ------------------------------------------
function toggleDir(path: string) {
  if (expanded.value.has(path)) expanded.value.delete(path)
  else expanded.value.add(path)
}
function newFile(dir = '') { startCreate('file', dir) }
function newFolder(dir = '') { startCreate('folder', dir) }

// open an inline input row in the tree (VSCode New File / New Folder).
function startCreate(kind: 'file' | 'folder', dir = '') {
  renamingPath.value = ''
  error.value = ''
  createInput.value = ''
  creating.value = { dir, kind }
  if (dir) {
    expandParents(dir)
    expanded.value.add(dir)
  }
  nextTick(() => {
    const el = document.querySelector<HTMLInputElement>('input[data-create]')
    el?.focus()
  })
}
function cancelCreate() {
  creating.value = null
  createInput.value = ''
}
function commitCreate() {
  const c = creating.value
  if (!c) return
  const leaf = createInput.value.trim().replace(/^\/+|\/+$/g, '')
  if (!leaf) { cancelCreate(); return }
  const full = joinPath(c.dir, leaf)
  if (!full) { cancelCreate(); return }
  if (c.kind === 'file') {
    if (draft.value!.files.some((f) => f.path === full)) { error.value = t('pages.agentStudio.dialogs.pathExists', { path: full }); return }
    const f = { path: full, content: '' }
    draft.value!.files.push(f)
    emptyDirs.value.delete(c.dir)
    expandParents(full)
    tab.value = 'files'
    openFile(f)
  } else {
    emptyDirs.value.add(full)
    expandParents(full)
    expanded.value.add(full)
  }
  creating.value = null
  createInput.value = ''
}
function isProtectedDir(path: string) {
  return path === 'rules' || path === 'skills'
}

function deleteEntry(row: TreeRow) {
  if (row.dir && isProtectedDir(row.path)) return
  confirmCfg.value = {
    title: row.dir ? t('pages.agentStudio.dialogs.deleteFolderTitle') : t('pages.agentStudio.dialogs.deleteFileTitle'),
    message: row.dir ? t('pages.agentStudio.dialogs.deleteFolderMessage', { path: row.path }) : t('pages.agentStudio.dialogs.deleteFileMessage', { path: row.path }),
    confirmText: t('pages.agentStudio.dialogs.delete'),
    danger: true,
    ok: () => {
      const gone = row.dir
        ? draft.value!.files.filter((f) => f.path === row.path || f.path.startsWith(row.path + '/'))
        : draft.value!.files.filter((f) => f.path === row.path)
      draft.value!.files = draft.value!.files.filter((f) => !gone.includes(f))
      openTabs.value = openTabs.value.filter((f) => !gone.includes(f))
      if (activeFile.value && gone.includes(activeFile.value)) activeFile.value = openTabs.value[openTabs.value.length - 1] || null
      if (row.dir) {
        const pref = row.path + '/'
        const ed = new Set<string>()
        emptyDirs.value.forEach((d) => { if (!(d === row.path || d.startsWith(pref))) ed.add(d) })
        emptyDirs.value = ed
      }
      if (!activeFile.value) selectDefaultFile()
    },
  }
}

async function onFolderPick(e: Event) {
  const input = e.target as HTMLInputElement
  const files = Array.from(input.files || [])
  input.value = ''
  if (!files.length || !draft.value) return
  const targetDir = uploadTargetDir.value
  uploadTargetDir.value = ''
  let added = 0
  let first = ''
  for (const f of files) {
    if (f.size > 512 * 1024) continue
    const parts = (f.webkitRelativePath || f.name).split('/').map((s) => s.trim()).filter(Boolean)
    const rel = parts.slice(1).join('/') || parts.join('/')
    if (!rel) continue
    const path = targetDir ? joinPath(targetDir, rel) : rel
    try {
      const content = await f.text()
      const at = draft.value.files.findIndex((x) => x.path === path)
      if (at >= 0) draft.value.files[at].content = content
      else draft.value.files.push({ path, content })
      if (!first) first = path
      added++
    } catch {
      /* skip unreadable/binary */
    }
  }
  if (!added) {
    error.value = t('pages.agentStudio.dialogs.importNoText')
    return
  }
  draft.value.files.sort((a, b) => a.path.localeCompare(b.path))
  tab.value = 'files'
  if (first) openPath(first)
  showToast(t('common.toast.importedFiles', { count: added, dir: targetDir ? targetDir + '/' : t('pages.agentStudio.configPaths.root') }))
}

// --- mcp / env ------------------------------------------------------------
function addMcp() {
  draft.value?.mcp.push({ name: '', transport: 'url', url: '', headers: [], command: '', args: '', env: [] })
}
function updateEnvKey(i: number, value: string) {
  if (!draft.value) return
  if (isManagedRegionKey(value)) {
    draft.value.env[i].k = ''
    showToast(t('pages.agentStudio.region.managedConflict'))
    return
  }
  draft.value.env[i].k = value
}
function removeMcp(i: number) {
  draft.value?.mcp.splice(i, 1)
}
function addArtifactStore() {
  if (!draft.value || hasArtifactStore.value) return
  draft.value.mcp.unshift({
    name: ARTIFACT_STORE,
    transport: 'url',
    url: '${APPROVING_ARTIFACT_URL}',
    headers: [{ k: 'Authorization', v: 'Bearer ${APPROVING_ARTIFACT_TOKEN}' }],
    command: '',
    args: '',
    env: [],
  })
}
function addAgentPlatformMcp(name: (typeof AGENT_PLATFORM_MCPS)[number]['name']) {
  if (!draft.value) return
  if (!isProjectBound.value) {
    showToast(t('pages.agentStudio.mcp.projectRequiredForPlatformMcp'))
    return
  }
  const spec = AGENT_PLATFORM_MCPS.find((m) => m.name === name)
  if (!spec) return
  if (hasAgentPlatformMcp(name)) {
    showToast(t('pages.agentStudio.mcp.agentMcpAlreadyExists', { name }))
    return
  }
  draft.value.mcp.push({
    name: spec.name,
    transport: 'url',
    url: spec.url,
    headers: [{ k: 'Authorization', v: `Bearer ${spec.token}` }],
    command: '',
    args: '',
    env: [],
  })
}
function upgradeLegacyPmLeader() {
  if (!draft.value) return
  draft.value.mcp = draft.value.mcp.filter((m) => m.name.trim() !== LEGACY_PM_LEADER)
  for (const spec of AGENT_PLATFORM_MCPS) {
    if (!hasAgentPlatformMcp(spec.name)) {
      addAgentPlatformMcp(spec.name)
    }
  }
  showToast(t('pages.agentStudio.mcp.legacyPmUpgraded'))
}
function toggleMcpRaw() {
  if (!draft.value) return
  mcpRaw.value = !mcpRaw.value
  rawError.value = ''
  if (mcpRaw.value) mcpRawText.value = JSON.stringify(draft.value.mcp.map(draftMcpToApi), null, 2)
}
function onMcpRaw(text: string) {
  mcpRawText.value = text
  if (!draft.value) return
  try {
    const arr = JSON.parse(text)
    if (!Array.isArray(arr)) throw new Error(t('pages.agentStudio.dialogs.jsonArray'))
    draft.value.mcp = (arr as MCPServer[]).map(apiMcpToDraft)
    rawError.value = ''
  } catch (e: any) {
    rawError.value = t('pages.agentStudio.mcp.parseError') + (e?.message || e)
  }
}
function toggleEnvRaw() {
  if (!draft.value) return
  envRaw.value = !envRaw.value
  rawError.value = ''
  if (envRaw.value) envRawText.value = JSON.stringify(kvToRec(draft.value.env), null, 2)
}
function onEnvRaw(text: string) {
  envRawText.value = text
  if (!draft.value) return
  try {
    const obj = JSON.parse(text)
    if (typeof obj !== 'object' || Array.isArray(obj)) throw new Error(t('pages.agentStudio.dialogs.jsonObject'))
    const incoming = obj as Record<string, string>
    const policy = getRegionPolicy(draft.value.acpBackend)
    if (policy) incoming[policy.regionEnvKey] = storedRegion()
    draft.value.env = recToKV(
      normalizeRegions(
        incoming,
        draft.value.acpBackend,
        'preserve-special',
      ).env,
    )
    rawError.value = ''
  } catch (e: any) {
    rawError.value = t('pages.agentStudio.env.parseError') + (e?.message || e)
  }
}

function onDocumentClick() {
  hideCtxMenu()
}

onMounted(() => {
  agentListCollapsed.value = readCollapsedState(AGENT_LIST_COLLAPSED_KEY)
  explorerCollapsed.value = readCollapsedState(EXPLORER_COLLAPSED_KEY)
  load()
  document.addEventListener('click', onDocumentClick)
  document.addEventListener('keydown', onExplorerKeydown)
})

onBeforeUnmount(() => {
  document.removeEventListener('click', onDocumentClick)
  document.removeEventListener('keydown', onExplorerKeydown)
  if (toastTimer) clearTimeout(toastTimer)
})
</script>

<template>
  <div class="flex h-full min-h-0 flex-col overflow-hidden">
    <div
      class="mb-5 flex shrink-0 gap-4"
      :class="isMobile ? 'flex-col items-stretch' : 'justify-end'"
    >
      <div class="flex shrink-0 gap-2" :class="isMobile ? 'flex-col' : 'items-center'">
        <AppButton
          variant="outline"
          icon="input"
          :class="isMobile ? 'min-h-11 w-full justify-center' : ''"
          @click="triggerImport"
        >{{ t('pages.agentStudio.exportImport.import') }}</AppButton>
        <AppButton
          variant="primary"
          icon="plus"
          :class="isMobile ? 'min-h-11 w-full justify-center' : ''"
          @click="openCreateAgent"
        >{{ t('common.buttons.newAgent') }}</AppButton>
      </div>
    </div>

    <div v-if="error" class="card mb-3 shrink-0 border-err/40 p-3 text-[13px] text-err">{{ t('pages.agentStudio.errorPrefix') }}{{ error }}</div>

    <div class="flex min-h-0 flex-1 flex-col">
      <div v-if="loading" class="card flex flex-1 items-center justify-center text-sm text-txt3">{{ t('common.buttons.loading') }}</div>

      <div v-else-if="!agents.length" class="card flex flex-1 items-center justify-center text-sm text-txt3">
        {{ t('pages.agentStudio.empty') }}
      </div>

      <div
        v-else
        class="card grid min-h-0 flex-1 overflow-hidden transition-[grid-template-columns] duration-[220ms] ease-in-out"
        :style="cardGridStyle"
      >
      <!-- agent org tree (hidden on narrow screens; agent name bar remains) -->
      <AgentOrgSidebar
        v-if="!isMobile"
        :org="org"
        :agent-names="agentNames"
        :active-name="activeName"
        :collapsed="agentListCollapsed"
        @select-agent="chooseAgent"
        @rename-agent="onSidebarRenameBlocked"
        @remove-from-group="onRemoveFromGroup"
        @open-manage="openAgentManage"
        @create-root-group="openCreateRootGroup"
        @create-child-group="openCreateChildGroup"
        @rename-group="openRenameGroup"
        @delete-group="confirmDeleteGroup"
        @export-group="onExportGroup"
        @import-group="onImportGroup"
        @move-group="onMoveGroup"
        @move-agent="onMoveAgent"
        @toggle-collapsed="toggleAgentListCollapsed"
      />

      <!-- editor -->
      <div v-if="draft" class="flex min-h-0 min-w-0 flex-col overflow-hidden">
        <div
          class="flex items-center gap-2 border-b border-line px-4"
          :class="isMobile ? 'min-h-12 py-2.5' : 'py-2'"
        >
          <Icon name="robot" :size="15" class="shrink-0 text-accent-2" />
          <span class="min-w-0 truncate text-[13px] font-medium text-txt">{{ activeName }}</span>
          <button
            v-if="isMobile"
            type="button"
            data-test="org-switch"
            class="inline-flex shrink-0 items-center gap-1 rounded border border-line bg-elevated px-2 py-1 text-[12px] text-txt2 transition hover:border-line-strong hover:text-txt"
            :title="t('pages.agentStudio.mobile.switchTitle')"
            :aria-label="t('pages.agentStudio.mobile.switchAria')"
            @click="openOrgSheet"
          >
            <Icon name="menu" :size="14" />
            <span>{{ t('pages.agentStudio.mobile.switch') }}</span>
          </button>
          <span v-if="dirty" class="chip shrink-0 border-warn/30 text-warn">{{ t('pages.agentStudio.unsaved') }}</span>
          <span
            v-else-if="justSaved"
            class="chip shrink-0 border-ok/30 bg-ok/10 text-ok"
          >{{ t('pages.agentStudio.saved') }}</span>
          <div class="ml-auto flex shrink-0 items-center gap-2">
            <AppButton size="sm" variant="outline" icon="download" :disabled="exporting" @click="triggerExport">
              {{ t('pages.agentStudio.exportImport.export') }}
            </AppButton>
            <AppButton
              size="sm"
              variant="primary"
              :class="isMobile ? 'min-h-11' : ''"
              :disabled="!dirty || saving"
              @click="save"
            >
              {{ saving ? t('common.buttons.saving') : dirty ? t('common.buttons.save') : t('pages.agentStudio.saved') }}
            </AppButton>
          </div>
        </div>

        <div class="scroll-area flex gap-1 overflow-x-auto border-b border-line px-3 pt-2 [-webkit-overflow-scrolling:touch]">
          <button
            v-for="tabItem in studioTabs"
            :key="tabItem.k"
            class="shrink-0 whitespace-nowrap px-3 text-[12px] transition"
            :class="[
              isMobile ? 'min-h-11 py-2.5' : 'rounded-t py-1.5',
              tab === tabItem.k ? 'border-b-2 border-accent text-txt' : 'text-txt3 hover:text-txt2',
            ]"
            @click="requestStudioTab(tabItem.k)"
          >{{ tabItem.l }}</button>
        </div>

        <!-- agent working directory (VSCode-style explorer + tabs + editor) -->
        <div
          v-if="tab === 'files'"
          class="min-h-0 flex-1 overflow-hidden transition-[grid-template-columns] duration-[220ms] ease-in-out"
          :class="isMobile ? 'flex flex-col' : 'grid'"
          :style="isMobile ? undefined : workspaceGridStyle"
        >
          <!-- explorer (list step on mobile) -->
          <div
            v-if="!isMobile || filesStep === 'list'"
            class="flex min-h-0 min-w-0 flex-col bg-base/30"
            :class="isMobile ? 'flex-1' : 'border-r border-line'"
          >
            <div
              class="flex shrink-0 items-center gap-0.5 border-b border-line px-2 py-1.5"
              :class="[
                isMobile ? 'min-h-11' : 'min-h-8',
                explorerCollapsed && !isMobile ? 'justify-center px-[3px]' : '',
              ]"
            >
              <span
                v-if="!explorerCollapsed || isMobile"
                class="flex-1 truncate px-1 text-[10.5px] font-semibold uppercase tracking-wider text-txt3"
              >{{ t('pages.agentStudio.explorer.title') }}</span>
              <template v-if="!explorerCollapsed || isMobile">
                <button
                  class="rounded p-1 text-txt3 hover:bg-elevated hover:text-accent-2 focus-visible:shadow-[inset_0_0_0_2px_rgba(99,102,241,0.35)] outline-none"
                  :class="isMobile ? 'min-h-11 min-w-11' : ''"
                  :title="t('pages.agentStudio.explorer.newFile')"
                  @click="newFile('')"
                ><Icon name="doc" :size="13" /></button>
                <button
                  class="rounded p-1 text-txt3 hover:bg-elevated hover:text-accent-2 focus-visible:shadow-[inset_0_0_0_2px_rgba(99,102,241,0.35)] outline-none"
                  :class="isMobile ? 'min-h-11 min-w-11' : ''"
                  :title="t('pages.agentStudio.explorer.newFolder')"
                  @click="newFolder('')"
                ><Icon name="folder" :size="13" /></button>
                <input ref="folderInput" type="file" webkitdirectory directory multiple class="hidden" @change="onFolderPick" />
                <button
                  v-if="!isMobile"
                  type="button"
                  :class="collapseBtnClass"
                  :title="t('pages.agentStudio.explorer.collapse')"
                  :aria-label="t('pages.agentStudio.explorer.collapse')"
                  @click="toggleExplorerCollapsed"
                >
                  <Icon name="chevron-right" :size="14" class="rotate-180" />
                </button>
              </template>
              <button
                v-if="explorerCollapsed && !isMobile"
                type="button"
                :class="collapseBtnClass"
                :title="t('pages.agentStudio.explorer.expand')"
                :aria-label="t('pages.agentStudio.explorer.expand')"
                @click="toggleExplorerCollapsed"
              >
                <Icon name="chevron-right" :size="14" />
              </button>
            </div>
            <div
              v-if="!explorerCollapsed || isMobile"
              class="scroll-area min-h-0 flex-1 overflow-y-auto py-1"
              @contextmenu="onExplorerBlankCtx"
            >
              <div v-if="!rows.length && !creating" class="px-3 py-6 text-center text-[11px] leading-5 text-txt3">{{ t('pages.agentStudio.explorer.empty') }}</div>

              <!-- inline new entry at root (VSCode-style) -->
              <div v-if="creating && creating.dir === ''" class="flex w-full items-center gap-1 py-[3px] pr-1.5">
                <span class="flex shrink-0" :style="{ width: '8px' }" />
                <Icon :name="creating.kind === 'folder' ? 'folder' : 'doc'" :size="13" class="shrink-0" :class="creating.kind === 'folder' ? 'text-accent-2' : 'text-txt3'" />
                <input
                  data-create
                  v-model="createInput"
                  class="min-w-0 flex-1 rounded border border-accent bg-surface px-1 py-0 font-mono text-[12px] text-txt outline-none"
                  :placeholder="creating.kind === 'folder' ? t('pages.agentStudio.explorer.folderPlaceholder') : t('pages.agentStudio.explorer.filePlaceholder')"
                  @keyup.enter="commitCreate"
                  @keyup.esc="cancelCreate"
                  @blur="commitCreate"
                  @click.stop
                />
              </div>

              <template v-for="row in rows" :key="(row.dir ? 'd:' : 'f:') + row.path">
              <div
                class="group relative flex w-full items-center gap-1 pr-1.5 text-left text-[12px] transition"
                :class="[
                  isMobile ? 'min-h-11 py-2' : 'py-[3px]',
                  !row.dir && activePath === row.path ? 'bg-accent-dim text-txt' : 'text-txt2 hover:bg-elevated',
                  ctxMenu.target?.path === row.path ? 'bg-overlay outline outline-1 outline-accent/35' : '',
                ]"
                @dblclick="startRename(row)"
                @contextmenu="openCtxMenu($event, { dir: row.dir, path: row.path, name: row.name })"
              >
                <span
                  v-if="!row.dir && activePath === row.path"
                  class="absolute inset-y-0 left-0 w-0.5 bg-accent"
                />
                <!-- indent guides -->
                <span class="flex shrink-0" :style="{ width: 8 + row.depth * 12 + 'px' }">
                  <span v-for="d in row.depth" :key="d" class="ml-[5px] w-[7px] border-l border-line/60" />
                </span>

                <template v-if="renamingPath === row.path">
                  <Icon :name="row.dir ? 'folder' : 'doc'" :size="13" class="shrink-0" :class="row.dir ? 'text-accent-2' : 'text-txt3'" />
                  <input
                    data-rename
                    v-model="renameInput"
                    class="min-w-0 flex-1 rounded border border-accent bg-surface px-1 py-0 font-mono text-[12px] text-txt outline-none"
                    @keyup.enter="commitRename(row)"
                    @keyup.esc="cancelRename"
                    @blur="commitRename(row)"
                    @click.stop
                  />
                </template>

                <template v-else>
                  <button v-if="row.dir" class="flex min-w-0 flex-1 items-center gap-1" @click="toggleDir(row.path)">
                    <Icon :name="expanded.has(row.path) ? 'chevron-down' : 'chevron-right'" :size="12" class="shrink-0 text-txt3" />
                    <Icon name="folder" :size="13" class="shrink-0 text-accent-2" />
                    <span class="truncate font-mono">{{ row.name }}</span>
                  </button>
                  <button v-else class="flex min-w-0 flex-1 items-center gap-1 pl-[15px]" @click="openPath(row.path); selectedTreeRow = row">
                    <Icon name="doc" :size="13" class="shrink-0 text-txt3" />
                    <span class="truncate font-mono">{{ row.name }}</span>
                  </button>
                  <button
                    v-if="row.dir"
                    data-test="file-row-action"
                    class="shrink-0 text-txt3 hover:text-accent-2"
                    :class="isMobile ? 'flex min-h-9 min-w-9 items-center justify-center opacity-100' : 'opacity-0 group-hover:opacity-100'"
                    :title="t('pages.agentStudio.explorer.newFileInFolder')"
                    @click.stop="newFile(row.path)"
                  ><Icon name="doc" :size="12" /></button>
                  <button
                    v-if="row.dir"
                    data-test="file-row-action"
                    class="shrink-0 text-txt3 hover:text-accent-2"
                    :class="isMobile ? 'flex min-h-9 min-w-9 items-center justify-center opacity-100' : 'opacity-0 group-hover:opacity-100'"
                    :title="t('pages.agentStudio.explorer.newFolderInFolder')"
                    @click.stop="newFolder(row.path)"
                  ><Icon name="folder" :size="12" /></button>
                  <button
                    data-test="file-row-action"
                    class="shrink-0 text-txt3 hover:text-accent-2"
                    :class="isMobile ? 'flex min-h-9 min-w-9 items-center justify-center opacity-100' : 'opacity-0 group-hover:opacity-100'"
                    :title="t('pages.agentStudio.explorer.rename')"
                    @click.stop="startRename(row)"
                  ><Icon name="edit" :size="12" /></button>
                  <button
                    v-if="!row.dir || !isProtectedDir(row.path)"
                    data-test="file-row-action"
                    class="shrink-0 text-txt3 hover:text-err"
                    :class="isMobile ? 'flex min-h-9 min-w-9 items-center justify-center opacity-100' : 'opacity-0 group-hover:opacity-100'"
                    :title="t('pages.agentStudio.explorer.delete')"
                    @click.stop="deleteEntry(row)"
                  ><Icon name="close" :size="12" /></button>
                </template>
              </div>

              <!-- inline new entry under this folder (VSCode-style) -->
              <div v-if="creating && creating.dir === row.path" class="flex w-full items-center gap-1 py-[3px] pr-1.5">
                <span class="flex shrink-0" :style="{ width: 8 + (row.depth + 1) * 12 + 'px' }">
                  <span v-for="d in (row.depth + 1)" :key="d" class="ml-[5px] w-[7px] border-l border-line/60" />
                </span>
                <Icon :name="creating.kind === 'folder' ? 'folder' : 'doc'" :size="13" class="shrink-0" :class="creating.kind === 'folder' ? 'text-accent-2' : 'text-txt3'" />
                <input
                  data-create
                  v-model="createInput"
                  class="min-w-0 flex-1 rounded border border-accent bg-surface px-1 py-0 font-mono text-[12px] text-txt outline-none"
                  :placeholder="creating.kind === 'folder' ? t('pages.agentStudio.explorer.folderName') : t('pages.agentStudio.explorer.fileName')"
                  @keyup.enter="commitCreate"
                  @keyup.esc="cancelCreate"
                  @blur="commitCreate"
                  @click.stop
                />
              </div>
              </template>
            </div>
          </div>

          <!-- editor pane (edit step on mobile) -->
          <div
            v-if="!isMobile || filesStep === 'edit'"
            class="flex min-h-0 min-w-0 flex-col overflow-hidden"
            :class="isMobile ? 'flex-1' : ''"
          >
            <!-- mobile edit chrome: back + path -->
            <div
              v-if="isMobile"
              class="flex shrink-0 items-center gap-2 border-b border-line bg-base/25 px-2.5 py-2 min-h-12"
            >
              <button
                type="button"
                class="inline-flex min-h-11 items-center gap-1 px-2 text-[12px] text-txt2 hover:text-txt"
                @click="tryBackToList"
              >
                <Icon name="chevron-right" :size="14" class="rotate-180" />
                {{ t('pages.agentStudio.mobile.back') }}
              </button>
              <span class="min-w-0 flex-1 truncate font-mono text-[12px] text-txt">{{ activePath }}</span>
            </div>

            <!-- open-file tabs (desktop only; mobile uses single-file full-screen edit) -->
            <div
              v-if="!isMobile && openTabs.length"
              class="scroll-area flex shrink-0 items-stretch overflow-x-auto border-b border-line bg-base/40"
            >
              <div
                v-for="tabFile in openTabs"
                :key="tabFile.path"
                class="group flex shrink-0 cursor-pointer items-center gap-1.5 border-r border-line px-3 py-1.5 text-[12px] transition"
                :class="activeFile === tabFile ? 'bg-surface text-txt' : 'text-txt3 hover:bg-elevated'"
                @click="activeFile = tabFile"
              >
                <Icon name="doc" :size="12" class="shrink-0" />
                <span class="max-w-[160px] truncate font-mono">{{ tabFile.path.split('/').pop() }}</span>
                <button class="shrink-0 rounded text-txt3 opacity-0 hover:bg-overlay hover:text-txt group-hover:opacity-100" :class="activeFile === tabFile ? 'opacity-60' : ''" :title="t('pages.agentStudio.explorer.close')" @click.stop="closeTab(tabFile)"><Icon name="close" :size="12" /></button>
              </div>
            </div>

            <template v-if="currentFile">
              <!-- breadcrumb (desktop) -->
              <div
                v-if="!isMobile"
                class="flex shrink-0 items-center gap-1 border-b border-line px-3 py-1 text-[11px] text-txt3"
              >
                <Icon name="folder" :size="11" class="text-accent-2/70" />
                <template v-for="(seg, i) in breadcrumb" :key="i">
                  <Icon v-if="i > 0" name="chevron-right" :size="10" class="text-txt3/60" />
                  <span :class="i === breadcrumb.length - 1 ? 'font-mono text-txt2' : 'font-mono'">{{ seg }}</span>
                </template>
              </div>
              <div class="flex min-h-0 flex-1 flex-col overflow-hidden">
                <MarkdownSplitEditor
                  v-if="isMdPath(currentFile.path)"
                  v-model="currentFile.content"
                  :file-path="currentFile.path"
                  :variant="isMobile ? 'stack' : 'split'"
                />
                <CodeEditor v-else v-model="currentFile.content" :language="langForPath(currentFile.path)" />
              </div>
              <!-- status bar -->
              <div
                class="flex shrink-0 items-center gap-3 border-t border-line bg-base/40 px-3 py-1 text-[10.5px] text-txt3"
              >
                <span class="uppercase">{{ langForPath(currentFile.path) }}</span>
                <span>{{ t('common.format.lines', { n: currentFile.content.split('\n').length }) }}</span>
                <span>{{ t('common.format.chars', { n: currentFile.content.length }) }}</span>
                <span
                  v-if="!isMobile && isMdPath(currentFile.path)"
                  class="border border-line bg-elevated px-1.5 text-[10px] text-txt2"
                >{{ t('pages.agentStudio.explorer.markdownBadge') }}</span>
                <span class="ml-auto truncate font-mono">{{ currentFile.path }}</span>
              </div>
            </template>
            <div v-else class="flex flex-1 flex-col items-center justify-center gap-2 text-sm text-txt3">
              <Icon name="doc" :size="28" class="text-line-strong" />
              {{ t('pages.agentStudio.explorer.selectOrCreate') }}
            </div>
          </div>
        </div>

        <!-- narrow-screen: non-whitelist tabs show desktop-only tip (files+data allowed) -->
        <div
          v-else-if="isMobile && tab !== 'data'"
          class="flex min-h-0 flex-1 flex-col items-center justify-center gap-3 px-5 py-8 text-center"
        >
          <div class="flex h-10 w-10 items-center justify-center border border-info/35 bg-info/10 text-info">
            <Icon name="alert" :size="20" />
          </div>
          <h3 class="text-[14px] font-semibold text-txt">{{ t('pages.agentStudio.mobile.desktopOnlyTitle') }}</h3>
          <p class="max-w-[28ch] text-[12.5px] leading-relaxed text-txt2">
            {{ t('pages.agentStudio.mobile.desktopOnlyDesc', { tab: studioTabLabel }) }}
          </p>
        </div>

        <!-- mcp -->
        <div v-else-if="tab === 'mcp'" class="flex min-h-0 flex-1 flex-col">
          <div class="flex items-center gap-2 border-b border-line px-4 py-2">
            <p class="flex-1 text-[11px] text-txt3">{{ t('pages.agentStudio.mcp.hint', { configRoot: draft?.layout?.configRoot || DEFAULT_CONFIG_ROOT }) }}</p>
            <button class="rounded border border-line px-2 py-1 text-[11px] text-txt2 hover:border-line-strong" @click="toggleMcpRaw">{{ mcpRaw ? t('pages.agentStudio.mcp.formEdit') : t('pages.agentStudio.mcp.rawJson') }}</button>
          </div>

          <div v-if="mcpRaw" class="flex min-h-0 flex-1 flex-col">
            <div v-if="rawError" class="border-b border-err/30 bg-err/10 px-4 py-1.5 text-[11px] text-err">{{ rawError }}</div>
            <div class="min-h-0 flex-1"><CodeEditor :model-value="mcpRawText" language="json" @update:model-value="onMcpRaw" /></div>
          </div>

          <div v-else class="scroll-area flex-1 space-y-3 overflow-y-auto p-4">
            <div class="rounded-md border border-accent/30 bg-accent-dim/40 p-2.5 text-[11px] leading-5 text-txt2">
              <div class="mb-1 flex items-center gap-1.5 text-txt">
                <Icon name="sparkles" :size="13" class="text-accent-2" />{{ t('pages.agentStudio.mcp.runVarsTitle') }}
                <span class="rounded border border-info/35 px-1.5 py-px text-[10px] font-medium text-info">{{ t('pages.agentStudio.mcp.runScopeTag') }}</span>
              </div>
              <div class="grid gap-0.5 font-mono text-[10.5px] text-txt3">
                <div><code class="text-accent-2">${APPROVING_ARTIFACT_URL}</code> — {{ t('pages.agentStudio.mcp.artifactUrl') }}</div>
                <div><code class="text-accent-2">${APPROVING_ARTIFACT_TOKEN}</code> — {{ t('pages.agentStudio.mcp.artifactToken') }}</div>
                <div><code class="text-accent-2">${APPROVING_RUN_ID}</code> · <code class="text-accent-2">${APPROVING_NODE_ID}</code></div>
                <div><code class="text-accent-2">${vars.&lt;name&gt;}</code> — {{ t('pages.agentStudio.mcp.globalVar') }}</div>
              </div>
              <button v-if="!hasArtifactStore" class="mt-2 rounded border border-accent/40 px-2 py-1 text-accent-2 hover:bg-accent-dim" @click="addArtifactStore">{{ t('pages.agentStudio.mcp.addArtifactStore') }}</button>
            </div>

            <div class="rounded-md border border-ok/30 bg-ok/5 p-2.5 text-[11px] leading-5 text-txt2">
              <div class="mb-1 flex items-center gap-1.5 text-txt">
                <Icon name="clock" :size="13" class="text-ok" />{{ t('pages.agentStudio.mcp.agentVarsTitle') }}
                <span class="rounded border border-ok/35 px-1.5 py-px text-[10px] font-medium text-ok">{{ t('pages.agentStudio.mcp.agentScopeTag') }}</span>
              </div>
              <div class="grid gap-0.5 font-mono text-[10.5px] text-txt3">
                <div><code class="text-accent-2">${APPROVING_MEMORY_URL}</code> / <code class="text-accent-2">${APPROVING_MEMORY_TOKEN}</code> — {{ t('pages.agentStudio.mcp.memoryVars') }}</div>
                <div><code class="text-accent-2">${APPROVING_CONTEXT_URL}</code> / <code class="text-accent-2">${APPROVING_CONTEXT_TOKEN}</code> — {{ t('pages.agentStudio.mcp.contextVars') }}</div>
                <div><code class="text-accent-2">${APPROVING_SCHEDULER_URL}</code> / <code class="text-accent-2">${APPROVING_SCHEDULER_TOKEN}</code> — {{ t('pages.agentStudio.mcp.schedulerVars') }}</div>
              </div>
              <div class="mt-2 border-t border-line-strong/80 pt-2 text-[10.5px] leading-5 text-txt3">
                <span class="font-medium text-ok">{{ t('pages.agentStudio.mcp.agentScopeNote') }}</span>
              </div>
              <div v-if="!isProjectBound" class="mt-2 rounded border border-dashed border-warn/40 bg-warn/10 p-2 text-[10.5px] leading-5 text-warn">
                {{ t('pages.agentStudio.mcp.projectRequiredForPlatformMcp') }}
              </div>
              <div class="mt-2 flex flex-wrap gap-1.5">
                <button class="rounded border border-accent/40 px-2 py-1 text-accent-2 hover:bg-accent-dim disabled:cursor-not-allowed disabled:opacity-40" type="button" :disabled="!isProjectBound" @click="addAgentPlatformMcp('memory-store')">{{ t('pages.agentStudio.mcp.addMemoryStore') }}</button>
                <button class="rounded border border-accent/40 px-2 py-1 text-accent-2 hover:bg-accent-dim disabled:cursor-not-allowed disabled:opacity-40" type="button" :disabled="!isProjectBound" @click="addAgentPlatformMcp('context-store')">{{ t('pages.agentStudio.mcp.addContextStore') }}</button>
                <button class="rounded border border-accent/40 px-2 py-1 text-accent-2 hover:bg-accent-dim disabled:cursor-not-allowed disabled:opacity-40" type="button" :disabled="!isProjectBound" @click="addAgentPlatformMcp('task-scheduler')">{{ t('pages.agentStudio.mcp.addTaskScheduler') }}</button>
              </div>
              <div v-if="hasLegacyPmLeader" class="mt-2 rounded border border-dashed border-warn/40 bg-warn/10 p-2 text-[10.5px] leading-5 text-warn">
                <div>{{ t('pages.agentStudio.mcp.legacyPmHint') }}</div>
                <button class="mt-1.5 rounded border border-warn/40 px-2 py-1 text-warn hover:bg-warn/15" type="button" @click="upgradeLegacyPmLeader">{{ t('pages.agentStudio.mcp.upgradeLegacyPm') }}</button>
              </div>
            </div>

            <div v-for="(m, i) in draft.mcp" :key="i" class="rounded-md border bg-base p-3" :class="isAgentPlatformMcpName(m.name) || isLegacyPmLeaderName(m.name) ? 'border-ok/30' : 'border-line'">
              <div class="mb-2 flex items-center gap-2">
                <input v-model="m.name" :placeholder="t('pages.agentStudio.mcp.serviceName')" class="flex-1 rounded border border-line bg-surface px-2 py-1 text-[12px] text-txt outline-none focus:border-accent" />
                <select v-model="m.transport" class="rounded border border-line bg-surface px-2 py-1 text-[12px] text-txt2 outline-none">
                  <option value="url">HTTP (url)</option>
                  <option value="command">{{ t('pages.agentStudio.mcp.transportCommand') }}</option>
                </select>
                <button class="text-txt3 hover:text-err" :title="t('pages.agentStudio.mcp.remove')" @click="removeMcp(i)"><Icon name="close" :size="14" /></button>
              </div>

              <template v-if="m.transport === 'url'">
                <input v-model="m.url" placeholder="https://mcp.example.com/sse" class="mb-2 w-full rounded border border-line bg-surface px-2 py-1 font-mono text-[11px] text-txt outline-none focus:border-accent" />
                <div class="text-[11px] text-txt3">{{ t('pages.agentStudio.mcp.headers') }}</div>
                <div v-for="(h, hi) in m.headers" :key="hi" class="mt-1 flex items-center gap-1.5">
                  <input v-model="h.k" placeholder="Authorization" class="w-1/3 rounded border border-line bg-surface px-2 py-1 font-mono text-[11px] text-txt2 outline-none" />
                  <input v-model="h.v" placeholder="Bearer …" class="flex-1 rounded border border-line bg-surface px-2 py-1 font-mono text-[11px] text-txt2 outline-none" />
                  <button class="text-txt3 hover:text-err" @click="m.headers.splice(hi, 1)"><Icon name="close" :size="12" /></button>
                </div>
                <button class="mt-1.5 text-[11px] text-accent-2 hover:underline" @click="m.headers.push({ k: '', v: '' })">{{ t('pages.agentStudio.mcp.addHeader') }}</button>
              </template>

              <template v-else>
                <input v-model="m.command" placeholder="npx" class="mb-2 w-full rounded border border-line bg-surface px-2 py-1 font-mono text-[11px] text-txt outline-none focus:border-accent" />
                <div class="text-[11px] text-txt3">{{ t('pages.agentStudio.mcp.args') }}</div>
                <textarea v-model="m.args" rows="2" placeholder="-y&#10;@upstash/context7-mcp" class="mt-1 w-full resize-y rounded border border-line bg-surface px-2 py-1 font-mono text-[11px] text-txt2 outline-none" />
                <div class="mt-2 text-[11px] text-txt3">{{ t('pages.agentStudio.mcp.env') }}</div>
                <div v-for="(e, ei) in m.env" :key="ei" class="mt-1 flex items-center gap-1.5">
                  <input v-model="e.k" placeholder="KEY" class="w-1/3 rounded border border-line bg-surface px-2 py-1 font-mono text-[11px] text-txt2 outline-none" />
                  <input v-model="e.v" placeholder="value" class="flex-1 rounded border border-line bg-surface px-2 py-1 font-mono text-[11px] text-txt2 outline-none" />
                  <button class="text-txt3 hover:text-err" @click="m.env.splice(ei, 1)"><Icon name="close" :size="12" /></button>
                </div>
                <button class="mt-1.5 text-[11px] text-accent-2 hover:underline" @click="m.env.push({ k: '', v: '' })">{{ t('pages.agentStudio.mcp.addEnv') }}</button>
              </template>

              <div v-if="isAgentPlatformMcpName(m.name)" class="mt-2.5 flex items-start gap-2 border border-dashed border-ok/35 bg-ok/5 p-2 text-[10.5px] leading-5 text-txt2">
                <Icon name="alert" :size="14" class="mt-0.5 shrink-0 text-ok" />
                <div>{{ t('pages.agentStudio.mcp.agentScopeBadge') }}</div>
              </div>
              <div v-else-if="isLegacyPmLeaderName(m.name)" class="mt-2.5 flex items-start gap-2 border border-dashed border-warn/40 bg-warn/10 p-2 text-[10.5px] leading-5 text-warn">
                <Icon name="alert" :size="14" class="mt-0.5 shrink-0 text-warn" />
                <div>{{ t('pages.agentStudio.mcp.legacyPmEntryBadge') }}</div>
              </div>
            </div>
            <AppButton size="sm" variant="outline" icon="plus" @click="addMcp">{{ t('pages.agentStudio.mcp.addService') }}</AppButton>
          </div>
        </div>

        <!-- env -->
        <div v-else-if="tab === 'env'" class="flex min-h-0 flex-1 flex-col">
          <div class="flex items-center gap-2 border-b border-line px-4 py-2">
            <button
              type="button"
              class="bg-transparent p-0 text-[12px] text-accent-2 underline underline-offset-[3px] hover:text-[#c4b5fd] focus-visible:outline focus-visible:outline-1 focus-visible:outline-offset-2 focus-visible:outline-accent"
              data-test="env-help-inject"
              @click="openEnvHelp('inject')"
            >
              {{ t('pages.agentStudio.envHelp.link') }}
            </button>
            <span class="flex-1" />
            <button class="rounded border border-line px-2 py-1 text-[11px] text-txt2 hover:border-line-strong" @click="toggleEnvRaw">{{ envRaw ? t('pages.agentStudio.mcp.formEdit') : t('pages.agentStudio.mcp.rawJson') }}</button>
          </div>

          <div v-if="envRaw" class="flex min-h-0 flex-1 flex-col">
            <div v-if="rawError" class="border-b border-err/30 bg-err/10 px-4 py-1.5 text-[11px] text-err">{{ rawError }}</div>
            <div class="min-h-0 flex-1"><CodeEditor :model-value="envRawText" language="json" @update:model-value="onEnvRaw" /></div>
          </div>
          <div v-else class="scroll-area flex-1 space-y-2 overflow-y-auto p-4">
            <AgentGitGuide
              :env="draft.env"
              :upsert-env="upsertEnv"
              :credential-type="draft.gitCredentialType"
              @update:credential-type="draft.gitCredentialType = $event"
              @help="openEnvHelp('git')"
            />
            <div class="mb-3 rounded-lg border border-line bg-base/50 p-3 text-[11px] leading-6 text-txt3">
              <div class="mb-1 flex items-center justify-between gap-2">
                <div class="font-medium text-txt2">{{ t('pages.agentStudio.env.backendAuthTitle') }}</div>
                <button
                  type="button"
                  class="bg-transparent p-0 text-[12px] text-accent-2 underline underline-offset-[3px] hover:text-[#c4b5fd] focus-visible:outline focus-visible:outline-1 focus-visible:outline-offset-2 focus-visible:outline-accent"
                  data-test="env-help-acp"
                  @click="openEnvHelp('acp')"
                >
                  {{ t('pages.agentStudio.envHelp.link') }}
                </button>
              </div>
              <p class="font-mono text-accent-2">{{ currentAuthHint.key }}<span v-if="currentAuthHint.alt"> / {{ currentAuthHint.alt }}</span></p>
            </div>
            <template v-for="(e, i) in draft.env" :key="i">
              <div v-if="!isManagedRegionKey(e.k)" class="flex items-center gap-1.5">
                <input :value="e.k" placeholder="KEY" class="w-1/3 rounded border border-line bg-surface px-2 py-1.5 font-mono text-[12px] text-txt outline-none focus:border-accent" @input="updateEnvKey(i, ($event.target as HTMLInputElement).value)" />
                <input v-model="e.v" placeholder="value" class="flex-1 rounded border border-line bg-surface px-2 py-1.5 font-mono text-[12px] text-txt2 outline-none focus:border-accent" />
                <button class="text-txt3 hover:text-err" @click="draft.env.splice(i, 1)"><Icon name="close" :size="14" /></button>
              </div>
            </template>
            <div v-if="currentRegionPolicy" class="border border-accent/30 bg-accent-dim/40 p-3">
              <div class="mb-2 text-[11px] text-txt3">{{ t('pages.agentStudio.region.managedByAcp') }}</div>
              <div class="flex items-center gap-1.5">
                <input :value="currentRegionPolicy.regionEnvKey" readonly :aria-label="t('pages.agentStudio.region.managedKey')" class="w-1/3 rounded border border-line bg-elevated px-2 py-1.5 font-mono text-[12px] text-txt3" />
                <input :value="storedRegion()" readonly :aria-label="t('pages.agentStudio.region.managedValue')" class="flex-1 rounded border border-line bg-elevated px-2 py-1.5 font-mono text-[12px] text-txt3" />
              </div>
            </div>
            <AppButton size="sm" variant="outline" icon="plus" @click="draft.env.push({ k: '', v: '' })">{{ t('pages.agentStudio.env.add') }}</AppButton>
          </div>
        </div>

        <!-- prompts: per-Agent overrides of platform-injected prompt text + rule files -->
        <div v-else-if="tab === 'prompts' && draft" class="scroll-area min-h-0 flex-1 overflow-y-auto p-4">
          <div class="mb-4 max-w-3xl">
            <h3 class="text-sm font-semibold text-txt">{{ t('pages.agentStudio.prompts.title') }}</h3>
            <p class="mt-1 text-[12px] leading-6 text-txt3" v-html="t('pages.agentStudio.prompts.intro')" />
          </div>

          <div class="max-w-3xl space-y-4">
            <label v-for="f in PROMPT_FRAGMENTS" :key="f.key" class="block">
              <span class="text-[12px] font-medium text-txt2">{{ f.label }}</span>
              <p class="mb-1.5 text-[11px] text-txt3">{{ f.hint }}</p>
              <textarea
                v-model="draft.prompts[f.key]"
                rows="3"
                spellcheck="false"
                :placeholder="t('pages.agentStudio.prompts.defaultPrefix') + f.placeholder"
                class="w-full resize-y rounded-md border border-line bg-base px-3 py-2 font-mono text-[12px] leading-6 text-txt outline-none focus:border-accent"
              />
            </label>
          </div>

          <p class="mt-4 max-w-3xl text-[11px] leading-5 text-txt3">
            {{ t('pages.agentStudio.prompts.rulesNote') }}
          </p>
        </div>

        <!-- platform rules override -->
        <div v-if="tab === 'platform-rules' && !isMobile" class="grid min-h-0 flex-1 grid-cols-[240px_1fr_260px] overflow-hidden">
          <aside class="flex min-h-0 flex-col border-r border-line bg-base/30">
            <div class="border-b border-line px-3 py-3">
              <h3 class="text-[13px] font-semibold text-txt">{{ t('pages.agentStudio.platformRules.title') }}</h3>
              <p class="mt-1 text-[11px] leading-relaxed text-txt3">{{ t('pages.agentStudio.platformRules.subtitle', { agent: activeName }) }}</p>
            </div>
            <div v-if="platformRuleLoading" class="p-4 text-xs text-txt3">{{ t('common.buttons.loading') }}</div>
            <div v-else class="scroll-area flex-1 overflow-y-auto p-2">
              <button
                v-for="item in platformRuleItems"
                :key="item.file"
                class="mb-0.5 flex w-full items-center gap-2 rounded px-2 py-1.5 text-left text-[12px] transition"
                :class="platformRuleFile === item.file ? 'bg-accent-dim text-txt' : 'text-txt3 hover:bg-elevated hover:text-txt2'"
                @click="selectPlatformRuleFile(item.file)"
              >
                <Icon name="file" :size="14" class="shrink-0 opacity-70" />
                <span class="min-w-0 flex-1 truncate font-mono text-[11px]">{{ item.file }}</span>
                <span
                  class="shrink-0 rounded border px-1.5 py-0.5 text-[10px]"
                  :class="item.source === 'override' ? 'border-warn/30 bg-warn/10 text-warn' : 'border-info/30 bg-info/10 text-info'"
                >
                  {{ item.source === 'override' ? t('pages.agentStudio.platformRules.overridden') : t('pages.agentStudio.platformRules.inherited') }}
                </span>
              </button>
            </div>
          </aside>

          <section class="flex min-h-0 min-w-0 flex-col">
            <div class="flex items-center justify-between gap-2 border-b border-line px-4 py-2">
              <div class="flex min-w-0 items-center gap-2">
                <span class="truncate font-mono text-[12px] text-txt2">
                  {{ platformRuleOverridden ? `profiles/${activeName}/platform-rules/${platformRuleFile}` : t('pages.agentStudio.platformRules.inheritPath', { file: platformRuleFile }) }}
                </span>
                <span
                  class="shrink-0 rounded border px-2 py-0.5 text-[10px]"
                  :class="platformRuleOverridden ? 'border-warn/30 bg-warn/10 text-warn' : 'border-info/30 bg-info/10 text-info'"
                >
                  {{ platformRuleOverridden ? t('pages.agentStudio.platformRules.overridden') : t('pages.agentStudio.platformRules.inherited') }}
                </span>
              </div>
              <div class="flex shrink-0 items-center gap-2">
                <AppButton
                  v-if="!platformRuleOverridden"
                  variant="primary"
                  size="sm"
                  icon="plus"
                  :disabled="platformRuleSaving || !platformRuleFile"
                  @click="createPlatformRuleOverride"
                >
                  {{ t('pages.agentStudio.platformRules.createOverride') }}
                </AppButton>
                <template v-else>
                  <AppButton variant="ghost" size="sm" icon="trash" :disabled="platformRuleSaving" @click="deletePlatformRuleOverride">
                    {{ t('pages.agentStudio.platformRules.deleteOverride') }}
                  </AppButton>
                  <AppButton variant="primary" size="sm" icon="check" :disabled="platformRuleSaving" @click="savePlatformRuleOverride">
                    {{ platformRuleSaving ? t('common.buttons.saving') : t('common.buttons.save') }}
                  </AppButton>
                </template>
              </div>
            </div>
            <div v-if="platformRuleError" class="border-b border-err/30 bg-err/10 px-4 py-2 text-[12px] text-err">{{ platformRuleError }}</div>
            <div class="min-h-0 flex-1">
              <MarkdownSplitEditor
                v-if="platformRuleFile"
                v-model="platformRuleContent"
                :file-path="`rules/${platformRuleFile}`"
                :readonly="!platformRuleOverridden"
              />
            </div>
          </section>

          <aside class="scroll-area min-h-0 overflow-y-auto border-l border-line bg-base/20 p-3">
            <h4 class="text-[11px] font-semibold uppercase tracking-wider text-txt3">{{ t('pages.agentStudio.platformRules.statusTitle') }}</h4>
            <div class="mt-2 flex flex-wrap gap-2 text-[10px]">
              <span class="rounded border border-warn/30 bg-warn/10 px-2 py-1 text-warn">
                {{ t('pages.agentStudio.platformRules.overriddenCount', { n: platformRuleOverrideCount }) }}
              </span>
              <span class="rounded border border-info/30 bg-info/10 px-2 py-1 text-info">
                {{ t('pages.agentStudio.platformRules.inheritedCount', { n: platformRuleItems.length - platformRuleOverrideCount }) }}
              </span>
            </div>
            <p v-if="platformRuleOverridden" class="mt-3 text-[11px] leading-relaxed text-warn">
              {{ t('pages.agentStudio.platformRules.diffHint') }}
            </p>
            <p class="mt-3 text-[11px] leading-relaxed text-txt3">{{ t('pages.agentStudio.platformRules.promptsNote') }}</p>
          </aside>
        </div>

        <!-- data: Agent-scoped memory / context / cron-job management (whitelist on mobile) -->
        <div v-if="tab === 'data' && draft" class="min-h-0 flex-1 overflow-hidden">
          <div v-if="draftBindingDirty" class="border-b border-warn/30 bg-warn/10 px-4 py-2 text-[12px] text-warn">
            {{ t('pages.agentStudio.data.unsavedBinding') }}
          </div>
          <AgentDataPanel
            v-if="isProjectBound"
            :agent-name="activeName"
            :project-name="projectNameById(savedProjectId)"
            :sub-tab="dataSubTab"
            @update:sub-tab="onDataSubTab"
          />
          <div v-else class="scroll-area min-h-0 flex-1 overflow-auto p-4">
            <div class="max-w-lg rounded border border-dashed border-line bg-base p-6 text-center">
              <p class="text-[13px] font-medium text-txt">{{ t('pages.agentStudio.data.emptyTitle') }}</p>
              <p class="mt-1.5 text-[12px] leading-6 text-txt3">{{ t('pages.agentStudio.data.emptyDesc') }}</p>
              <!-- Mobile: meta is desktop-only — avoid dead-link to bind -->
              <button
                v-if="!isMobile"
                class="mt-3 rounded border border-accent/40 px-3 py-1.5 text-[12px] text-accent-2 hover:bg-accent-dim"
                type="button"
                @click="tab = 'meta'"
              >
                {{ t('pages.agentStudio.data.goBind') }}
              </button>
            </div>
          </div>
        </div>

        <!-- chat test (kept mounted via v-show so the session survives tab switches) -->
        <div v-show="tab === 'test' && !isMobile" class="flex min-h-0 flex-1 flex-col">
          <AgentChatTester :profile="activeName" :home-project-id="savedProjectId" />
        </div>

        <!-- meta: per-Agent sandbox-injection layout (persisted; drives creation) -->
        <div v-if="tab === 'meta' && draft && !isMobile" class="scroll-area min-h-0 flex-1 overflow-auto p-4">
          <div class="mb-4 max-w-3xl">
            <h3 class="text-sm font-semibold text-txt">{{ t('pages.agentStudio.org.metaTitle') }}</h3>
            <p class="mt-1 text-[12px] leading-6 text-txt3">{{ t('pages.agentStudio.org.metaIntro') }}</p>
            <p class="mt-2 border-l-2 border-accent-2 bg-accent-dim px-2.5 py-1.5 text-[11px] leading-5 text-txt2">
              {{ t('pages.agentStudio.org.metaRenameHint') }}
            </p>
          </div>

          <div class="mb-8 max-w-3xl">
            <div class="mb-1 text-[12px] font-medium text-txt2">{{ t('pages.agentStudio.project.label') }}</div>
            <p class="mb-2.5 text-[11px] leading-5 text-txt3">{{ t('pages.agentStudio.project.hint') }}</p>
            <select
              v-model="projectSelectValue"
              class="max-w-sm w-full rounded border border-line bg-surface px-2 py-1.5 text-[12px] text-txt outline-none focus:border-accent"
            >
              <option value="">{{ t('pages.agentStudio.project.unbound') }}</option>
              <option v-for="p in projects" :key="p.id" :value="p.id">{{ p.name }}</option>
            </select>
            <p v-if="!isProjectBound" class="mt-2 rounded border border-dashed border-warn/40 bg-warn/10 px-2.5 py-1.5 text-[11px] leading-5 text-warn">
              {{ t('pages.agentStudio.project.unboundWarn') }}
            </p>
          </div>

          <div class="mb-8 max-w-3xl space-y-5">
            <div>
              <div class="mb-1 flex items-center justify-between gap-2 text-[12px] font-medium text-txt2">
                <span>{{ t('pages.agentStudio.org.groupsLabel') }}</span>
                <span class="text-[11px] font-normal text-txt3">{{ t('pages.agentStudio.org.groupsMeta') }}</span>
              </div>
              <p class="mb-2.5 text-[11px] leading-5 text-txt3">{{ t('pages.agentStudio.org.groupsHint') }}</p>
              <div v-if="metaGroupTiles.length" class="grid grid-cols-1 gap-2 sm:grid-cols-2">
                <button
                  v-for="tile in metaGroupTiles"
                  :key="tile.id"
                  type="button"
                  class="flex min-h-[56px] items-start gap-2.5 border px-3 py-2.5 text-left transition"
                  :class="tile.selected
                    ? 'border-accent bg-accent-dim text-txt shadow-[inset_0_0_0_1px_rgba(99,102,241,0.25)]'
                    : 'border-line bg-base text-txt2 hover:border-line-strong hover:bg-elevated hover:text-txt'"
                  @click="toggleMetaGroup(tile.id)"
                >
                  <span
                    class="mt-0.5 flex h-4 w-4 shrink-0 items-center justify-center border"
                    :class="tile.selected ? 'border-accent bg-accent text-white' : 'border-line-strong bg-surface'"
                  >
                    <Icon v-if="tile.selected" name="check" :size="11" />
                  </span>
                  <span class="min-w-0 flex-1">
                    <span class="block truncate text-[12.5px] font-semibold">{{ tile.name }}</span>
                    <span
                      class="mt-0.5 block truncate text-[10.5px]"
                      :class="tile.selected ? 'text-accent-2/80' : 'text-txt3'"
                    >{{ tile.path }}</span>
                  </span>
                </button>
              </div>
              <p v-else class="border border-dashed border-line px-3 py-3 text-[12px] text-txt3">
                {{ t('pages.agentStudio.org.noGroups') }}
              </p>
            </div>

            <div>
              <div class="mb-1 flex items-center justify-between gap-2 text-[12px] font-medium text-txt2">
                <span>{{ t('pages.agentStudio.org.parentLabel') }}</span>
                <span class="text-[11px] font-normal text-txt3">{{ t('pages.agentStudio.org.parentMeta') }}</span>
              </div>
              <p class="mb-2.5 text-[11px] leading-5 text-txt3">{{ t('pages.agentStudio.org.parentHint') }}</p>
              <div class="relative max-w-sm">
                <button
                  type="button"
                  class="flex w-full items-center gap-2 border border-line bg-base px-3 py-2.5 text-left text-[12.5px] text-txt transition hover:border-line-strong"
                  :class="parentDropdownOpen ? 'border-accent shadow-[0_0_0_1px_rgba(99,102,241,0.25)]' : ''"
                  @click="parentDropdownOpen = !parentDropdownOpen"
                >
                  <span class="min-w-0 flex-1 truncate" :class="metaParentAgent ? '' : 'text-txt3'">
                    {{ metaParentAgent || t('pages.agentStudio.org.parentNone') }}
                  </span>
                  <Icon name="chevron-right" :size="14" class="shrink-0 text-txt3 transition" :class="parentDropdownOpen ? 'rotate-90' : 'rotate-90'" />
                </button>
                <div
                  v-if="parentDropdownOpen"
                  class="absolute left-0 right-0 top-[calc(100%+4px)] z-20 max-h-56 overflow-auto border border-line-strong bg-elevated p-1 shadow-card"
                >
                  <button
                    type="button"
                    class="flex w-full items-center gap-2 px-2.5 py-2 text-left text-[12.5px] transition"
                    :class="!metaParentAgent ? 'bg-accent-dim text-txt' : 'text-txt2 hover:bg-overlay hover:text-txt'"
                    @click="setMetaParent('')"
                  >
                    {{ t('pages.agentStudio.org.parentNone') }}
                  </button>
                  <button
                    v-for="opt in metaParentOptions"
                    :key="opt"
                    type="button"
                    class="flex w-full items-center gap-2 px-2.5 py-2 text-left text-[12.5px] transition"
                    :class="metaParentAgent === opt ? 'bg-accent-dim text-txt' : 'text-txt2 hover:bg-overlay hover:text-txt'"
                    @click="setMetaParent(opt)"
                  >
                    <Icon name="robot" :size="13" class="shrink-0 text-accent-2" />
                    <span class="truncate">{{ opt }}</span>
                  </button>
                </div>
              </div>
            </div>
          </div>

          <div class="mb-4 max-w-3xl border-t border-line pt-5">
            <h3 class="text-sm font-semibold text-txt">{{ t('pages.agentStudio.meta.layoutTitle') }}</h3>
            <p class="mt-1 text-[12px] leading-6 text-txt3" v-html="t('pages.agentStudio.meta.layoutIntro')" />
          </div>

          <div class="max-w-3xl space-y-4">
            <div>
              <div class="text-[12px] font-medium text-txt2">{{ t('pages.agentStudio.meta.acpBackend') }}</div>
              <p class="mb-2 text-[11px] text-txt3">{{ t('pages.agentStudio.meta.acpBackendDesc') }}</p>
              <div class="grid grid-cols-2 gap-2 sm:grid-cols-4">
                <button
                  v-for="b in ACP_BACKENDS"
                  :key="b.id"
                  type="button"
                  class="border px-2 py-3 text-center transition"
                  :class="draft.acpBackend === b.id ? 'border-accent bg-accent-dim text-txt' : 'border-line bg-base text-txt2 hover:border-line-strong'"
                  @click="selectAcpBackend(b.id)"
                >
                  <div class="text-[12px] font-semibold">{{ b.label }}</div>
                  <div class="mt-0.5 font-mono text-[10px] text-txt3">{{ b.id }}</div>
                </button>
              </div>
            </div>
            <div v-if="showMetaRegionBlock" class="border-t border-dashed border-line pt-4">
              <div class="text-[12px] font-medium text-txt2">
                {{ t('pages.agentStudio.meta.regionTitle') }}
                <span class="ml-1.5 inline-block border border-accent/30 bg-accent-dim px-1.5 py-0.5 text-[9px] font-bold uppercase tracking-wider text-accent-2">{{ t('pages.agentStudio.meta.regionNewBadge') }}</span>
              </div>
              <p class="mb-2 text-[11px] text-txt3">{{ t('pages.agentStudio.meta.regionDesc') }}</p>
              <p v-if="specialRegion" class="mb-2 border border-warn/35 bg-warn/10 px-2.5 py-2 font-mono text-[11px] text-warn">
                {{ t('pages.agentStudio.region.special', { value: specialRegion }) }}
              </p>
              <div class="grid max-w-md grid-cols-2 gap-2" role="radiogroup" :aria-label="t('pages.agentStudio.region.title')">
                <button
                  v-for="r in metaRegionOptions"
                  :key="r.id"
                  type="button"
                  class="border px-2 py-3 text-center transition"
                  :class="displayRegion === r.id ? 'border-accent bg-accent-dim text-txt' : 'border-line bg-base text-txt2 hover:border-line-strong'"
                  role="radio"
                  :aria-checked="displayRegion === r.id"
                  :aria-label="`${t(r.labelKey)} (${r.id})`"
                  @click="selectRegion(r.id)"
                >
                  <div class="text-[12px] font-semibold">{{ t(r.labelKey) }}</div>
                  <div class="mt-0.5 font-mono text-[10px] text-txt3">{{ r.id }}</div>
                  <div class="mt-1.5 text-[10px] leading-snug" :class="displayRegion === r.id ? 'text-accent-2' : 'text-txt3'">{{ t(r.hintKey) }}</div>
                </button>
              </div>
            </div>
            <label class="block">
              <span class="text-[12px] font-medium text-txt2">{{ t('pages.agentStudio.meta.configRoot') }}</span>
              <p class="mb-1.5 text-[11px] text-txt3">{{ t('pages.agentStudio.meta.configRootDesc') }}</p>
              <input
                v-model="draft.layout.configRoot"
                :placeholder="defaultConfigRootFor(draft.acpBackend)"
                spellcheck="false"
                class="w-full rounded-md border border-line bg-base px-3 py-2 font-mono text-[12px] text-txt outline-none focus:border-accent"
                @input="configRootTouched = true"
              />
            </label>
            <label class="block">
              <span class="text-[12px] font-medium text-txt2">{{ t('pages.agentStudio.meta.workspaceDir') }}</span>
              <p class="mb-1.5 text-[11px] text-txt3">{{ t('pages.agentStudio.meta.workspaceDirDesc') }}</p>
              <input
                v-model="draft.layout.workspaceDir"
                :placeholder="DEFAULT_WORKSPACE_DIR"
                spellcheck="false"
                class="w-full rounded-md border border-line bg-base px-3 py-2 font-mono text-[12px] text-txt outline-none focus:border-accent"
              />
            </label>
          </div>

          <div class="mt-5 max-w-3xl">
            <div class="mb-1.5 text-[11px] uppercase tracking-wider text-txt3">{{ t('pages.agentStudio.meta.derivedPaths') }}</div>
            <div class="overflow-hidden rounded-lg border border-line">
              <table class="w-full text-left text-[12px]">
                <tbody>
                  <tr v-for="(e, i) in derivedPaths" :key="e.label" :class="i % 2 ? 'bg-base/40' : ''">
                    <td class="px-3 py-2 text-txt2">{{ e.label }}</td>
                    <td class="px-3 py-2"><code class="rounded bg-base px-1.5 py-0.5 font-mono text-accent-2">{{ e.path }}</code></td>
                    <td class="px-3 py-2 text-txt3">{{ e.note }}</td>
                  </tr>
                </tbody>
              </table>
            </div>
          </div>

          <p class="mt-3 max-w-3xl text-[11px] leading-5 text-txt3">
            {{ t('pages.agentStudio.meta.capabilitiesNote') }}
          </p>
        </div>
      </div>
      <div
        v-else
        class="flex min-h-0 min-w-0 flex-col items-center justify-center gap-2 text-[13px] text-txt3"
      >
        <Icon name="robot" :size="24" class="opacity-50" />
        <span>{{ t('pages.agentStudio.org.noSelection') }}</span>
      </div>
    </div>
    </div>

    <!-- Agent management (rename + hard delete) -->
    <AppModal
      :open="showAgentManage"
      :title="t('pages.agentStudio.org.manageTitle')"
      :width="520"
      @close="closeAgentManage"
    >
      <p class="mb-3 text-[12.5px] leading-relaxed text-txt2">{{ t('pages.agentStudio.org.manageIntro') }}</p>
      <div v-if="!agentNames.length" class="text-[13px] text-txt3">{{ t('pages.agentStudio.org.manageEmpty') }}</div>
      <div v-else class="flex flex-col gap-0.5">
        <div
          v-for="name in agentNames"
          :key="name"
          :data-manage-agent="name"
          class="flex items-center gap-2 border px-2.5 py-2 transition"
          :class="
            manageFocusAgent === name
              ? 'border-accent/40 bg-accent-dim shadow-[inset_0_0_0_1px_rgba(99,102,241,0.35)]'
              : 'border-transparent hover:bg-elevated'
          "
        >
          <Icon name="robot" :size="14" class="shrink-0 text-accent-2" />
          <span class="min-w-0 flex-1 truncate text-[13px] text-txt">{{ name }}</span>
          <div class="flex shrink-0 gap-1.5">
            <AppButton size="sm" variant="outline" icon="edit" @click="openRenameAgent(name)">
              {{ t('pages.agentStudio.org.manageRename') }}
            </AppButton>
            <AppButton size="sm" variant="danger" icon="trash" @click="confirmDeleteAgent(name)">
              {{ t('pages.agentStudio.dialogs.delete') }}
            </AppButton>
          </div>
        </div>
      </div>
      <template #footer>
        <AppButton size="sm" variant="ghost" @click="closeAgentManage">{{ t('common.buttons.close') }}</AppButton>
      </template>
    </AppModal>

    <!-- sidebar pencil: rename blocked → guide to Agent management -->
    <AppModal
      :open="showRenameBlocked"
      :title="t('pages.agentStudio.org.renameBlockedTitle')"
      :width="420"
      @close="closeRenameBlocked"
    >
      <div class="border border-warn/40 bg-warn/10 px-3.5 py-3 text-[13px] leading-6 text-warn">
        <div class="mb-1.5 text-[14px] font-semibold text-txt">{{ t('pages.agentStudio.org.renameBlockedMessage') }}</div>
        <p>{{ t('pages.agentStudio.org.renameBlockedBody') }}</p>
        <p class="mt-2 text-[12px] text-txt2">{{ t('pages.agentStudio.org.renameBlockedHint') }}</p>
      </div>
      <template #footer>
        <AppButton size="sm" variant="ghost" @click="closeRenameBlocked">{{ t('common.buttons.close') }}</AppButton>
        <AppButton size="sm" variant="primary" @click="gotoManageFromBlocked">
          {{ t('pages.agentStudio.org.gotoManage') }}
        </AppButton>
      </template>
    </AppModal>

    <!-- prompt modal (create / rename) -->
    <AppModal :open="!!promptCfg" :title="promptCfg?.title" :width="420" @close="promptCfg = null">
      <label class="mb-1 block text-[12px] text-txt2">{{ promptCfg?.label }}</label>
      <input
        v-model="promptValue"
        :placeholder="promptCfg?.placeholder"
        class="w-full rounded-md border border-line bg-base px-3 py-2 text-[13px] text-txt outline-none focus:border-accent"
        :class="{
          'border-err': !!promptError,
          'border-ok/55': !!promptOkMsg && !promptError,
        }"
        @input="refreshPromptFeedback"
        @keyup.enter="promptOk"
      />
      <p v-if="promptError" class="mt-2 text-[12px] text-err">{{ promptError }}</p>
      <p v-else-if="promptOkMsg" class="mt-2 text-[12px] text-ok">{{ promptOkMsg }}</p>
      <p v-if="promptCfg?.hint" class="mt-3 text-[12px] leading-relaxed text-txt2">{{ promptCfg.hint }}</p>
      <template #footer>
        <AppButton size="sm" variant="ghost" @click="promptCfg = null">{{ t('common.buttons.cancel') }}</AppButton>
        <AppButton size="sm" variant="primary" :disabled="!promptCanSubmit" @click="promptOk">{{ t('pages.agentStudio.dialogs.confirm') }}</AppButton>
      </template>
    </AppModal>

    <!-- confirm modal (delete / discard) -->
    <AppModal :open="!!confirmCfg" :title="confirmCfg?.title" :width="420" @close="confirmCfg = null">
      <p class="text-[13px] leading-6 text-txt2">{{ confirmCfg?.message }}</p>
      <template #footer>
        <AppButton size="sm" variant="ghost" @click="confirmCfg = null">{{ t('common.buttons.cancel') }}</AppButton>
        <AppButton size="sm" :variant="confirmCfg?.danger ? 'danger' : 'primary'" @click="confirmOk">{{ confirmCfg?.confirmText || t('pages.agentStudio.dialogs.confirm') }}</AppButton>
      </template>
    </AppModal>

    <!-- narrow-screen agent org tree sheet -->
    <Teleport to="body">
      <div
        v-if="showOrgSheet"
        data-test="org-sheet"
        class="fixed inset-0 z-40"
        role="dialog"
        aria-modal="true"
        :aria-label="t('pages.agentStudio.mobile.orgSheetTitle')"
      >
        <div
          data-test="org-sheet-backdrop"
          class="absolute inset-0 bg-black/50"
          @click="closeOrgSheet"
        />
        <div
          class="absolute inset-x-0 bottom-0 flex h-[70vh] max-h-[70vh] flex-col overflow-hidden rounded-t-xl border-t border-line bg-elevated shadow-card"
        >
          <div class="mx-auto mt-2 h-1 w-10 shrink-0 rounded-full bg-line-strong/70" aria-hidden="true" />
          <div class="flex shrink-0 items-center gap-2 border-b border-line px-3 py-2.5">
            <h3 class="min-w-0 flex-1 truncate text-[14px] font-semibold text-txt">
              {{ t('pages.agentStudio.mobile.orgSheetTitle') }}
            </h3>
            <AppButton
              size="sm"
              variant="outline"
              data-test="org-sheet-manage"
              class="min-h-9 shrink-0"
              @click="openManageFromSheet"
            >{{ t('pages.agentStudio.org.manage') }}</AppButton>
            <button
              type="button"
              data-test="org-sheet-close"
              class="flex min-h-9 min-w-9 shrink-0 items-center justify-center rounded text-txt3 hover:bg-overlay hover:text-txt"
              :aria-label="t('common.buttons.close')"
              @click="closeOrgSheet"
            >
              <Icon name="close" :size="14" />
            </button>
          </div>
          <div class="scroll-area min-h-0 flex-1 overflow-y-auto p-1.5 [-webkit-overflow-scrolling:touch]">
            <div
              v-for="row in orgSheetRows"
              :key="row.key"
              class="relative flex w-full items-center gap-0.5 py-0.5 pr-1 text-left text-[12px] transition"
              :class="
                row.kind === 'agent'
                  ? activeName === row.name
                    ? 'bg-accent-dim'
                    : ''
                  : 'text-txt3'
              "
              :data-org-kind="row.kind"
              :data-org-name="row.kind === 'agent' ? row.name : row.kind === 'group' ? row.name : 'ungrouped'"
            >
              <template v-if="row.kind === 'group'">
                <button
                  type="button"
                  class="flex min-h-11 min-w-0 flex-1 items-center gap-0.5 py-1 text-left"
                  :style="orgSheetPadStyle(row.depth)"
                  @click="toggleOrgSheetNode(row.id)"
                >
                  <Icon name="chevron-right" :size="12" class="shrink-0 text-txt3" :class="row.collapsed ? '' : 'rotate-90'" />
                  <Icon name="folder" :size="14" class="shrink-0 text-warn" />
                  <span class="truncate font-medium text-txt2">{{ row.name }}</span>
                  <span class="ml-auto inline-flex h-4 min-w-[18px] shrink-0 items-center justify-end border border-line bg-base px-1 text-[10px] font-semibold tabular-nums text-txt3">{{ row.count }}</span>
                </button>
              </template>
              <template v-else-if="row.kind === 'ungrouped-header'">
                <button
                  type="button"
                  class="flex min-h-11 min-w-0 flex-1 items-center gap-0.5 py-1 text-left"
                  :style="orgSheetPadStyle(row.depth)"
                  @click="toggleOrgSheetNode(UNGROUPED_ID)"
                >
                  <Icon name="chevron-right" :size="12" class="shrink-0 text-txt3" :class="row.collapsed ? '' : 'rotate-90'" />
                  <Icon name="folder" :size="14" class="shrink-0 text-txt3" />
                  <span class="truncate font-medium text-txt2">{{ t('pages.agentStudio.org.ungrouped') }}</span>
                  <span class="ml-auto inline-flex h-4 min-w-[18px] shrink-0 items-center justify-end border border-line bg-base px-1 text-[10px] font-semibold tabular-nums text-txt3">{{ row.count }}</span>
                </button>
              </template>
              <template v-else>
                <button
                  type="button"
                  data-test="org-sheet-agent"
                  class="flex min-h-11 min-w-0 flex-1 items-center gap-1.5 py-1 text-left hover:bg-elevated"
                  :style="orgSheetPadStyle(row.depth)"
                  @click="chooseAgentFromSheet(row.name)"
                >
                  <span class="inline-block h-4 w-4 shrink-0" />
                  <Icon name="robot" :size="14" class="shrink-0 text-accent-2" />
                  <span class="min-w-0 flex-1">
                    <span class="flex items-center gap-1">
                      <span class="truncate text-txt">{{ row.name }}</span>
                      <span
                        v-if="row.multi"
                        class="shrink-0 border border-accent-2/35 bg-accent/15 px-1 text-[9px] font-bold uppercase tracking-wide text-accent-2"
                      >{{ t('pages.agentStudio.org.multiGroup') }}</span>
                    </span>
                  </span>
                </button>
              </template>
            </div>
            <p class="mx-1 mt-2 border border-dashed border-line-strong/60 bg-white/[0.015] px-2.5 py-2 text-[11px] leading-relaxed text-txt3">
              {{ t('pages.agentStudio.mobile.orgSheetHint') }}
            </p>
          </div>
        </div>
      </div>
    </Teleport>

    <!-- leave edit: save / discard / cancel -->
    <AppModal
      :open="!!leaveConfirmCfg"
      :title="leaveConfirmCfg?.title"
      :width="420"
      @close="leaveConfirmCancel"
    >
      <p class="text-[13px] leading-6 text-txt2">{{ leaveConfirmCfg?.message }}</p>
      <template #footer>
        <div class="flex w-full flex-col gap-2 sm:flex-row sm:justify-end">
          <AppButton
            size="sm"
            variant="primary"
            class="min-h-11 sm:min-h-0"
            :disabled="saving"
            @click="leaveConfirmSave"
          >{{ leaveConfirmCfg?.saveText }}</AppButton>
          <AppButton
            size="sm"
            variant="outline"
            class="min-h-11 border-warn/40 text-warn sm:min-h-0"
            @click="leaveConfirmDiscard"
          >{{ leaveConfirmCfg?.discardText }}</AppButton>
          <AppButton
            size="sm"
            variant="ghost"
            class="min-h-11 sm:min-h-0"
            @click="leaveConfirmCancel"
          >{{ t('common.buttons.cancel') }}</AppButton>
        </div>
      </template>
    </AppModal>

    <!-- folder export secrets warning (Demo) -->
    <AppModal
      :open="showFolderSecrets"
      :title="t('pages.agentStudio.exportImport.folderSecrets.title')"
      :width="460"
      close-on-esc
      @close="cancelFolderSecrets"
    >
      <p class="text-[13px] leading-6 text-txt2">{{ t('pages.agentStudio.exportImport.folderSecrets.body') }}</p>
      <template #footer>
        <AppButton size="sm" variant="ghost" @click="cancelFolderSecrets">{{ t('common.buttons.cancel') }}</AppButton>
        <AppButton size="sm" variant="primary" :disabled="exporting" @click="confirmFolderSecrets">
          {{ t('pages.agentStudio.exportImport.folderSecrets.confirm') }}
        </AppButton>
      </template>
    </AppModal>

    <!-- folder batch name conflict (Demo) -->
    <AppModal
      :open="showBatchConflict"
      :title="t('pages.agentStudio.exportImport.batchConflict.title')"
      :width="480"
      close-on-esc
      @close="closeBatchConflict"
    >
      <p class="text-[13px] leading-6 text-txt2">{{ t('pages.agentStudio.exportImport.batchConflict.intro') }}</p>
      <ul class="mt-2 max-h-32 overflow-auto border border-line bg-base px-3 py-2 text-[12px] text-txt">
        <li v-for="n in batchConflictNames" :key="n" class="font-mono">{{ n }}</li>
      </ul>
      <div class="mt-3 flex flex-col gap-2">
        <button
          type="button"
          class="w-full border border-accent/50 bg-accent-dim px-3 py-2.5 text-left"
          @click="confirmBatchRename"
        >
          <div class="text-[13px] font-medium text-txt">{{ t('pages.agentStudio.exportImport.batchConflict.rename') }}</div>
          <div class="text-[11.5px] text-txt3">{{ t('pages.agentStudio.exportImport.batchConflict.renameDesc') }}</div>
        </button>
        <button
          type="button"
          class="w-full border border-err/40 bg-base px-3 py-2.5 text-left hover:bg-err/10"
          @click="confirmBatchOverwrite"
        >
          <div class="text-[13px] font-medium text-err">{{ t('pages.agentStudio.exportImport.batchConflict.overwrite') }}</div>
          <div class="text-[11.5px] text-txt3">{{ t('pages.agentStudio.exportImport.batchConflict.overwriteDesc') }}</div>
        </button>
        <button
          type="button"
          class="w-full border border-line bg-base px-3 py-2.5 text-left hover:bg-elevated"
          @click="closeBatchConflict"
        >
          <div class="text-[13px] font-medium text-txt">{{ t('pages.agentStudio.exportImport.batchConflict.cancel') }}</div>
          <div class="text-[11.5px] text-txt3">{{ t('pages.agentStudio.exportImport.batchConflict.cancelDesc') }}</div>
        </button>
      </div>
    </AppModal>

    <!-- unsaved export guide -->
    <AppModal :open="showUnsavedExport" :title="t('pages.agentStudio.exportImport.unsavedExport.title')" :width="420" @close="cancelUnsavedExport">
      <p class="text-[13px] leading-6 text-txt2">{{ t('pages.agentStudio.exportImport.unsavedExport.message', { name: activeName }) }}</p>
      <template #footer>
        <AppButton size="sm" variant="ghost" @click="cancelUnsavedExport">{{ t('common.buttons.cancel') }}</AppButton>
        <AppButton size="sm" variant="outline" :disabled="exporting" @click="discardAndExport">{{ t('pages.agentStudio.exportImport.unsavedExport.discard') }}</AppButton>
        <AppButton size="sm" variant="primary" :disabled="exporting || saving" @click="saveThenExport">{{ t('pages.agentStudio.exportImport.unsavedExport.saveThenExport') }}</AppButton>
      </template>
    </AppModal>

    <!-- import discard confirm -->
    <AppModal :open="showImportDiscardConfirm" :title="t('pages.agentStudio.exportImport.discardImport.title')" :width="420" @close="onImportDiscardCancel">
      <p class="text-[13px] leading-6 text-txt2">{{ t('pages.agentStudio.exportImport.discardImport.message', { name: activeName }) }}</p>
      <template #footer>
        <AppButton size="sm" variant="ghost" @click="onImportDiscardCancel">{{ t('common.buttons.cancel') }}</AppButton>
        <AppButton size="sm" variant="danger" @click="onImportDiscardConfirm">{{ t('pages.agentStudio.exportImport.discardImport.confirm') }}</AppButton>
      </template>
    </AppModal>

    <!-- import name conflict -->
    <AppModal :open="showImportConflict" :title="t('pages.agentStudio.exportImport.conflict.title')" :width="460" @close="closeImportConflict">
      <p class="text-[13px] leading-6 text-txt2">{{ t('pages.agentStudio.exportImport.conflict.intro', { name: importConflictName }) }}</p>
      <div class="mt-3 flex flex-col gap-2">
        <button
          type="button"
          class="w-full border px-3 py-2.5 text-left transition"
          :class="importConflictAction === 'overwrite' ? 'border-accent/50 bg-accent-dim' : 'border-line bg-base hover:bg-elevated'"
          @click="selectImportConflict('overwrite')"
        >
          <div class="text-[13px] font-medium text-txt">{{ t('pages.agentStudio.exportImport.conflict.overwrite') }}</div>
          <div class="text-[11.5px] text-txt3">{{ t('pages.agentStudio.exportImport.conflict.overwriteDesc') }}</div>
        </button>
        <button
          type="button"
          class="w-full border px-3 py-2.5 text-left transition"
          :class="importConflictAction === 'rename' ? 'border-accent/50 bg-accent-dim' : 'border-line bg-base hover:bg-elevated'"
          @click="selectImportConflict('rename')"
        >
          <div class="text-[13px] font-medium text-txt">{{ t('pages.agentStudio.exportImport.conflict.rename') }}</div>
          <div class="text-[11.5px] text-txt3">{{ t('pages.agentStudio.exportImport.conflict.renameDesc') }}</div>
        </button>
        <button
          type="button"
          class="w-full border px-3 py-2.5 text-left transition"
          :class="importConflictAction === 'cancel' ? 'border-accent/50 bg-accent-dim' : 'border-line bg-base hover:bg-elevated'"
          @click="selectImportConflict('cancel')"
        >
          <div class="text-[13px] font-medium text-txt">{{ t('pages.agentStudio.exportImport.conflict.cancel') }}</div>
          <div class="text-[11.5px] text-txt3">{{ t('pages.agentStudio.exportImport.conflict.cancelDesc') }}</div>
        </button>
      </div>
      <div v-if="importConflictAction === 'rename'" class="mt-3.5">
        <label class="mb-1.5 block text-[12px] text-txt2">{{ t('pages.agentStudio.exportImport.conflict.newName') }}</label>
        <input
          v-model="importRenameValue"
          class="w-full rounded-md border border-line bg-base px-3 py-2 font-mono text-[13px] text-txt outline-none focus:border-accent"
          @keyup.enter="confirmImportConflict"
        />
        <p v-if="importRenameError" class="mt-2 text-[12px] text-err">{{ importRenameError }}</p>
      </div>
      <template #footer>
        <AppButton size="sm" variant="ghost" @click="closeImportConflict">{{ t('common.buttons.cancel') }}</AppButton>
        <AppButton size="sm" variant="primary" @click="confirmImportConflict">{{ t('pages.agentStudio.exportImport.conflict.confirm') }}</AppButton>
      </template>
    </AppModal>

    <!-- import error -->
    <AppModal :open="showImportErrorModal" :title="t('pages.agentStudio.exportImport.importError.title')" :width="420" @close="showImportErrorModal = false">
      <p class="text-[13px] leading-6 text-txt2">{{ importErrorMessage || t('pages.agentStudio.exportImport.importError.invalidZip') }}</p>
      <template #footer>
        <AppButton size="sm" variant="primary" @click="showImportErrorModal = false">{{ t('pages.agentStudio.exportImport.importError.ok') }}</AppButton>
      </template>
    </AppModal>

    <input ref="importFileInput" type="file" accept=".zip" class="hidden" @change="onImportFileChange" />

    <ExplorerContextMenu
      :open="ctxMenu.open"
      :x="ctxMenu.x"
      :y="ctxMenu.y"
      :target="ctxMenu.target"
      @close="hideCtxMenu"
      @action="onCtxAction"
    />

    <AgentCreateWizard
      :open="showCreateWizard"
      :existing-names="agents.map((a) => a.name)"
      @close="showCreateWizard = false"
      @created="onWizardCreated"
    />

    <EnvCredentialHelpModal
      :open="envHelpOpen"
      :section="envHelpSection"
      :backend="draft?.acpBackend || 'cursor'"
      @close="envHelpOpen = false"
    />

    <AppModal :open="showProjectSwitch" :title="t('pages.agentStudio.project.switchTitle')" :width="460" @close="cancelProjectChange">
      <div class="space-y-2 text-[13px] leading-6 text-txt2">
        <p>{{ t('pages.agentStudio.project.switchWarn') }}</p>
        <ul class="list-disc space-y-1 pl-5 text-[12px]">
          <li>{{ t('pages.agentStudio.project.switchItemMemory') }}</li>
          <li>{{ t('pages.agentStudio.project.switchItemContext') }}</li>
          <li>{{ t('pages.agentStudio.project.switchItemJobs') }}</li>
          <li>{{ t('pages.agentStudio.project.switchItemPm') }}</li>
        </ul>
        <p class="mt-2 text-[11.5px] text-txt3">{{ t('pages.agentStudio.project.switchApplyHint') }}</p>
      </div>
      <template #footer>
        <AppButton size="sm" variant="ghost" @click="cancelProjectChange">{{ t('common.buttons.cancel') }}</AppButton>
        <AppButton size="sm" variant="danger" @click="confirmProjectChange">
          {{ pendingProjectLabel ? t('pages.agentStudio.project.switchConfirm', { name: pendingProjectLabel }) : t('pages.agentStudio.project.unbindConfirm') }}
        </AppButton>
      </template>
    </AppModal>

    <Teleport to="body">
      <div
        v-if="toastMsg"
        class="fixed bottom-5 right-5 z-[10000] border border-line bg-elevated px-3.5 py-2 text-[12px] text-txt2 shadow-card"
      >
        {{ toastMsg }}
      </div>
    </Teleport>
  </div>
</template>
