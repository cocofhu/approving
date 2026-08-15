<script setup lang="ts">
import { ref, computed, watch, nextTick, onMounted, onBeforeUnmount } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/ui/Icon.vue'
import AppButton from '@/components/ui/AppButton.vue'
import AppModal from '@/components/ui/AppModal.vue'
import AgentOrgSidebar from '@/components/agent/AgentOrgSidebar.vue'
import AgentChatTester from '@/components/agent/AgentChatTester.vue'
import AgentDataPanel, { type DataSubTab } from '@/components/agent/AgentDataPanel.vue'
import AgentFilesPanel from '@/components/agent/AgentFilesPanel.vue'
import AgentMcpPanel from '@/components/agent/AgentMcpPanel.vue'
import AgentEnvPanel from '@/components/agent/AgentEnvPanel.vue'
import AgentPromptsPanel from '@/components/agent/AgentPromptsPanel.vue'
import AgentPlatformRulesPanel from '@/components/agent/AgentPlatformRulesPanel.vue'
import AgentMetaPanel from '@/components/agent/AgentMetaPanel.vue'
import AgentCreateWizard from '@/components/agent/AgentCreateWizard.vue'
import CreateAgentTeamWizard from '@/components/agent/CreateAgentTeamWizard.vue'
import TeamBootstrapPanel from '@/components/agent/TeamBootstrapPanel.vue'
import { api, type Agent, type AgentOrg, type TeamBootstrapSession } from '@/lib/api/api'
import { createListRequestSeq, httpStatusOf } from '@/lib/shared/listRequestSeq'
import { useBreakpoint } from '@/lib/composables/useBreakpoint'
import {
  emptyOrg,
  groupPath,
  newGroupId,
  applyDeleteGroup,
  applyMoveAgent,
  applyRemoveAgentFromGroup,
  wouldCreateGroupCycle,
  buildOrgTreeRows,
  recursiveMemberNames,
  classifyAssignTargets,
  assignNeedsDraftConfirm,
  shouldSyncDraftAfterAssign,
  isAgentInGroupSubtree,
  UNGROUPED_ID,
} from '@/lib/agent/agentOrg'
import { downloadZip, validateAgentName, normalizeAgentName } from '@/lib/agent/agentIO'
import { useAgentImport } from '@/lib/agent/useAgentImport'
import { isManagedRegionKey } from '@/lib/shared/regionPolicy'
import {
  PROMPT_KEYS,
  toDraft,
  fromDraft,
  fromDraftRaw,
  normalizeDraftRegions,
  type AgentStudioDraft as Draft,
} from '@/lib/agent/agentStudioDraft'

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

const AGENT_LIST_COLLAPSED_KEY = 'agent-studio-agent-list-collapsed'
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
function toggleAgentListCollapsed() {
  agentListCollapsed.value = !agentListCollapsed.value
  writeCollapsedState(AGENT_LIST_COLLAPSED_KEY, agentListCollapsed.value)
}

const cardGridStyle = computed(() => {
  if (isMobile.value) return { gridTemplateColumns: '1fr' }
  return {
    gridTemplateColumns: `${agentListCollapsed.value ? SIDEBAR_COLLAPSED_W : ORG_SIDEBAR_EXPANDED_W} 1fr`,
  }
})

const filesStep = ref<'list' | 'edit'>('list')
const justSaved = ref(false)
const filesPanelRef = ref<InstanceType<typeof AgentFilesPanel> | null>(null)

const TAB_FADE_THRESHOLD = 4
const agentNameEl = ref<HTMLElement | null>(null)
const tabStripEl = ref<HTMLElement | null>(null)
const agentNameTruncated = ref(false)
const showFullNameTip = ref(false)
const fullNameTipStyle = ref<Record<string, string>>({})
const tabFadeLeft = ref(false)
const tabFadeRight = ref(false)

let tabStripObserver: ResizeObserver | null = null

function closeFullNameTip() {
  showFullNameTip.value = false
}

function closeMobileChromeOverlays() {
  closeFullNameTip()
  filesPanelRef.value?.closeExplorerMore()
}

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

const agents = ref<Agent[]>([])
const projects = ref<{ id: string; name: string }[]>([])
const org = ref<AgentOrg>(emptyOrg())
const orgBaseline = ref('')
const activeName = ref('')
const draft = ref<Draft | null>(null)
const originalJson = ref('')
const tab = ref<StudioTab>('files')
const dataSubTab = ref<DataSubTab>('memory')
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
const loading = ref(true)
const hasInitialLoaded = ref(false)
const loadFailed = ref(false)
const loadDenied = ref(false)
const studioSeq = createListRequestSeq()
const error = ref('')
const saving = ref(false)
const initialLoading = computed(() => loading.value && !hasInitialLoaded.value)
const showRefreshProgress = computed(
  () => loading.value && hasInitialLoaded.value && agents.value.length > 0,
)

const toastMsg = ref('')
let toastTimer: ReturnType<typeof setTimeout> | null = null

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
  if (!mobile) {
    showOrgSheet.value = false
    closeMobileChromeOverlays()
  }
  nextTick(() => {
    measureAgentNameTruncation()
    syncTabFade()
  })
})

const agentNames = computed(() => agents.value.map((a) => a.name))
const manageSearch = ref('')
const manageSearchQuery = computed(() => manageSearch.value.trim().toLowerCase())
const filteredManageNames = computed(() => {
  const query = manageSearchQuery.value
  return query ? agentNames.value.filter((name) => name.toLowerCase().includes(query)) : agentNames.value
})
const manageSearchActive = computed(() => !!manageSearchQuery.value)

type ManageNameHighlight = { before: string; hit: string; after: string }
function manageNameHighlight(name: string): ManageNameHighlight {
  const query = manageSearchQuery.value
  const index = query ? name.toLowerCase().indexOf(query) : -1
  if (index < 0) return { before: name, hit: '', after: '' }
  return {
    before: name.slice(0, index),
    hit: name.slice(index, index + query.length),
    after: name.slice(index + query.length),
  }
}

function clearManageSearch() {
  manageSearch.value = ''
}

const orgSheetRows = computed(() =>
  buildOrgTreeRows(org.value, agentNames.value, orgSheetCollapsed.value, agents.value, projects.value),
)

type AssignFailItem = { name: string; reason: string }
const showAssignPick = ref(false)
const showAssignCover = ref(false)
const showAssignDraft = ref(false)
const assignApplying = ref(false)
const assignGroupName = ref('')
const assignMembers = ref<string[]>([])
const assignTargetId = ref('')
const assignDiffBound = ref<{ name: string; oldProjectId: string }[]>([])
const assignFail = ref<AssignFailItem[]>([])
const assignOkCount = ref(0)

const assignTargetLabel = computed(() =>
  assignTargetId.value ? projectNameById(assignTargetId.value) : '',
)
const assignMemberList = computed(() => assignMembers.value.join('、'))
const assignAffectedList = computed(() =>
  assignDiffBound.value
    .map((item) => `${item.name}（${projectNameById(item.oldProjectId)}）`)
    .join('、'),
)

function closeAssignModals() {
  showAssignPick.value = false
  showAssignCover.value = false
  showAssignDraft.value = false
  assignApplying.value = false
}

function onAssignProject(groupId: string) {
  const group = (org.value.groups || []).find((g) => g.id === groupId)
  if (!group) return
  assignFail.value = []
  assignOkCount.value = 0
  const members = recursiveMemberNames(org.value, groupId, agentNames.value)
  if (!members.length) {
    showToast(t('pages.agentStudio.org.assignEmpty'))
    return
  }
  if (!projects.value.length) {
    showToast(t('pages.agentStudio.org.assignNoProjects'))
    return
  }
  assignGroupName.value = group.name
  assignMembers.value = members
  assignTargetId.value = projects.value[0]?.id || ''
  assignDiffBound.value = []
  showAssignCover.value = false
  showAssignDraft.value = false
  showAssignPick.value = true
}

function onAssignPickNext() {
  const target = assignTargetId.value.trim()
  if (!target) {
    showToast(t('pages.agentStudio.org.assignNoProjects'))
    return
  }
  const classified = classifyAssignTargets(assignMembers.value, agents.value, target)
  if (classified.already.length === assignMembers.value.length) {
    closeAssignModals()
    showToast(t('pages.agentStudio.org.assignAlready'))
    return
  }
  assignDiffBound.value = classified.diffBound
  if (classified.diffBound.length) {
    showAssignPick.value = false
    showAssignCover.value = true
    return
  }
  maybeAssignDraftThenApply()
}

function cancelAssignCover() {
  closeAssignModals()
  showToast(t('pages.agentStudio.org.assignCancelled'))
}

function maybeAssignDraftThenApply() {
  const needsDraft = assignNeedsDraftConfirm({
    activeName: activeName.value,
    memberNames: assignMembers.value,
    draftBindingDirty: draftBindingDirty.value,
  })
  if (needsDraft) {
    showAssignPick.value = false
    showAssignCover.value = false
    showAssignDraft.value = true
    return
  }
  void applyAssign(assignMembers.value.includes(activeName.value))
}

function keepAssignDraft() {
  closeAssignModals()
  showToast(t('pages.agentStudio.org.assignDraftKept'))
}

function syncDraftProjectId(target: string) {
  if (!draft.value) return
  draft.value.projectId = target
  try {
    const snap = JSON.parse(originalJson.value || '{}') as Agent
    snap.projectId = target
    originalJson.value = JSON.stringify(snap)
  } catch {
    /* ignore malformed snapshot */
  }
}

async function applyAssign(syncDraftRequested: boolean) {
  const target = assignTargetId.value.trim()
  if (!target || assignApplying.value) return
  assignApplying.value = true
  assignFail.value = []
  assignOkCount.value = 0
  const ok: string[] = []
  const fail: AssignFailItem[] = []
  for (const name of assignMembers.value) {
    try {
      await api.patchAgentProject(name, target)
      ok.push(name)
    } catch (e: any) {
      fail.push({ name, reason: String(e?.message || e) })
    }
  }
  assignOkCount.value = ok.length
  assignFail.value = fail
  try {
    const list = await api.listAgents()
    agents.value = list || []
  } catch {
    /* keep local list; brackets refresh on next load */
  }
  if (
    shouldSyncDraftAfterAssign({
      activeName: activeName.value,
      memberNames: assignMembers.value,
      failNames: fail.map((f) => f.name),
      syncDraftRequested,
    })
  ) {
    syncDraftProjectId(target)
  }
  closeAssignModals()
  if (fail.length) {
    showToast(t('pages.agentStudio.org.assignPartialToast', { ok: ok.length, fail: fail.length }))
  } else {
    showToast(
      t('pages.agentStudio.org.assignOkToast', { n: ok.length, project: projectNameById(target) }),
    )
  }
}

function openOrgSheet() {
  closeMobileChromeOverlays()
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
function showToast(msg: string) {
  toastMsg.value = msg
  if (toastTimer) clearTimeout(toastTimer)
  toastTimer = setTimeout(() => {
    toastMsg.value = ''
  }, 2800)
}

function discardUnsavedChanges() {
  const snap = filesPanelRef.value?.snapshot() || { path: '', openPaths: [] as string[] }
  resetOrgFromBaseline()
  const a = agents.value.find((x) => x.name === activeName.value)
  if (!a) return
  const loaded = toDraft(a)
  originalJson.value = JSON.stringify(fromDraftRaw(loaded))
  normalizeDraftRegions(loaded)
  draft.value = loaded
  nextTick(() => filesPanelRef.value?.restoreAfterDiscard(snap))
  justSaved.value = false
  error.value = ''
}

function requestStudioTab(next: StudioTab) {
  if (next === tab.value) return
  const exposedStep = filesPanelRef.value?.filesStep as unknown
  const panelStep =
    exposedStep && typeof exposedStep === 'object' && exposedStep !== null && 'value' in exposedStep
      ? (exposedStep as { value: 'list' | 'edit' }).value
      : ((exposedStep as 'list' | 'edit' | undefined) ?? filesStep.value)
  const leavingDirtyEdit =
    isMobile.value && tab.value === 'files' && panelStep === 'edit' && dirty.value
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
        syncStudioQuery()
        return true
      },
      onDiscard: () => {
        discardUnsavedChanges()
        tab.value = next
        syncStudioQuery()
      },
    }
    return
  }
  tab.value = next
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
async function load() {
  const localSeq = studioSeq.beginListRequest()
  const keepStale = agents.value.length > 0
  loading.value = true
  error.value = ''
  loadFailed.value = false
  loadDenied.value = false
  try {
    const [list, o, projList] = await Promise.all([api.listAgents(), api.getAgentsOrg(), api.listProjects()])
    if (!studioSeq.isCurrentSeq(localSeq)) return
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
    if (!studioSeq.isCurrentSeq(localSeq)) return
    if (keepStale) return
    error.value = String(e?.message || e)
    agents.value = []
    const status = httpStatusOf(e)
    loadDenied.value = status === 403
    loadFailed.value = status !== 403
  } finally {
    if (!studioSeq.isCurrentSeq(localSeq)) return
    loading.value = false
    hasInitialLoaded.value = true
  }
}

function select(
  name: string,
  opts?: { tab?: StudioTab; dataSub?: DataSubTab; skipQuerySync?: boolean },
) {
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
  nextTick(() => filesPanelRef.value?.resetForSelect())
  if (!opts?.skipQuerySync) syncStudioQuery()
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
const showTeamWizard = ref(false)
const teamBootstrapSessionId = ref('')

function openCreateAgent() {
  showCreateWizard.value = true
}

function openCreateTeam() {
  showTeamWizard.value = true
}

function onWizardCreated(created: Agent) {
  agents.value.push(created)
  agents.value.sort((a, b) => a.name.localeCompare(b.name))
  // New agents default to ungrouped / no parent (no org entry).
  select(created.name)
  showToast(t('pages.agentStudio.wizard.createdToast', { name: created.name }))
}

function onTeamBootstrapStarted(session: TeamBootstrapSession) {
  teamBootstrapSessionId.value = session.id
  void reloadOrg()
  void refreshAgentsList()
}

async function refreshAgentsList() {
  try {
    const list = await api.listAgents()
    agents.value = list || []
    agents.value.sort((a, b) => a.name.localeCompare(b.name))
  } catch {
    /* ignore */
  }
}

async function onTeamBootstrapRefresh() {
  await Promise.all([reloadOrg(), refreshAgentsList()])
}

function onTeamBootstrapSelectPm(name: string) {
  void select(name)
}

function onTeamBootstrapOpenPm(name: string) {
  teamBootstrapSessionId.value = ''
  void select(name)
}

function onTeamBootstrapDone() {
  const pm = agents.value.find((a) => a.name.includes('项目经理'))?.name || agents.value[0]?.name
  teamBootstrapSessionId.value = ''
  void onTeamBootstrapRefresh().then(() => {
    if (pm) void select(pm)
  })
}

function openAgentManage(focusAgentName?: string) {
  clearManageSearch()
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
  clearManageSearch()
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
function measureAgentNameTruncation() {
  const el = agentNameEl.value
  if (!el) {
    agentNameTruncated.value = false
    return
  }
  agentNameTruncated.value = el.scrollWidth > el.clientWidth + 1
}

function placeFullNameTip() {
  const el = agentNameEl.value
  if (!el || !showFullNameTip.value) return
  const rect = el.getBoundingClientRect()
  const margin = 12
  fullNameTipStyle.value = {
    top: `${Math.round(rect.bottom + 8)}px`,
    left: `${margin}px`,
    right: `${margin}px`,
  }
}

function onAgentNameClick() {
  measureAgentNameTruncation()
  if (!agentNameTruncated.value) return
  filesPanelRef.value?.closeExplorerMore()
  showFullNameTip.value = !showFullNameTip.value
  if (showFullNameTip.value) nextTick(placeFullNameTip)
}

function syncTabFade() {
  const el = tabStripEl.value
  if (!el || !isMobile.value) {
    tabFadeLeft.value = false
    tabFadeRight.value = false
    return
  }
  const max = el.scrollWidth - el.clientWidth
  tabFadeLeft.value = el.scrollLeft > TAB_FADE_THRESHOLD
  tabFadeRight.value = el.scrollLeft < max - TAB_FADE_THRESHOLD
}

function bindTabStripObserver() {
  if (typeof ResizeObserver === 'undefined') return
  tabStripObserver?.disconnect()
  tabStripObserver = null
  if (!tabStripEl.value) return
  tabStripObserver = new ResizeObserver(() => syncTabFade())
  tabStripObserver.observe(tabStripEl.value)
}

function onChromeReposition() {
  if (showFullNameTip.value) placeFullNameTip()
  syncTabFade()
}

function onChromeKeydown(e: KeyboardEvent) {
  if (e.key !== 'Escape') return
  if (showFullNameTip.value) closeMobileChromeOverlays()
}

watch(activeName, () => {
  closeFullNameTip()
  nextTick(measureAgentNameTruncation)
})

watch(tabStripEl, () => {
  bindTabStripObserver()
  nextTick(syncTabFade)
})

onMounted(() => {
  agentListCollapsed.value = readCollapsedState(AGENT_LIST_COLLAPSED_KEY)
  load()
  document.addEventListener('keydown', onChromeKeydown)
  window.addEventListener('resize', onChromeReposition)
  window.addEventListener('scroll', onChromeReposition, true)
  nextTick(() => {
    measureAgentNameTruncation()
    bindTabStripObserver()
    syncTabFade()
  })
})

onBeforeUnmount(() => {
  document.removeEventListener('keydown', onChromeKeydown)
  window.removeEventListener('resize', onChromeReposition)
  window.removeEventListener('scroll', onChromeReposition, true)
  tabStripObserver?.disconnect()
  tabStripObserver = null
  if (toastTimer) clearTimeout(toastTimer)
})
</script>
<template>
  <div
    class="flex h-full min-h-0 flex-col overflow-hidden"
    data-testid="agent-studio-panel"
    :aria-busy="loading ? 'true' : 'false'"
  >
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
          variant="outline"
          icon="skills"
          :class="isMobile ? 'min-h-11 w-full justify-center' : ''"
          @click="openCreateTeam"
        >{{ t('pages.agentStudio.createTeam') }}</AppButton>
        <AppButton
          variant="primary"
          icon="plus"
          :class="isMobile ? 'min-h-11 w-full justify-center' : ''"
          @click="openCreateAgent"
        >{{ t('common.buttons.newAgent') }}</AppButton>
      </div>
    </div>

    <div
      v-if="error && !loadFailed && !loadDenied && agents.length"
      class="card mb-3 shrink-0 border-err/40 p-3 text-[13px] text-err"
    >{{ t('pages.agentStudio.errorPrefix') }}{{ error }}</div>
    <div
      v-if="assignFail.length"
      data-test="org-assign-fail"
      class="card mb-3 shrink-0 border-err/40 bg-err/10 p-3 text-[13px] text-txt2"
    >
      <div class="font-medium text-err">{{ t('pages.agentStudio.project.assignFailTitle') }}</div>
      <div class="mt-1 text-[12px]">
        {{ t('pages.agentStudio.project.assignFailSummary', { ok: assignOkCount, fail: assignFail.length }) }}
      </div>
      <ul class="mt-2 list-disc space-y-1 pl-5 text-[12px]">
        <li v-for="item in assignFail" :key="item.name">{{ item.name }}：{{ item.reason }}</li>
      </ul>
      <div class="mt-2 text-[11px] text-txt3">{{ t('pages.agentStudio.project.assignFailHint') }}</div>
    </div>

    <div class="flex min-h-0 flex-1 flex-col">
      <div
        v-if="showRefreshProgress"
        class="mb-2 h-[2px] overflow-hidden bg-line"
        data-testid="agent-studio-thin-progress"
        aria-hidden="true"
      >
        <i class="admin-list-thin-bar bg-accent" />
      </div>

      <div
        v-if="initialLoading"
        class="card grid min-h-0 flex-1 overflow-hidden"
        data-testid="agent-studio-skeleton"
        aria-hidden="true"
        :style="isMobile ? { gridTemplateColumns: '1fr' } : { gridTemplateColumns: '280px 1fr' }"
      >
        <div v-if="!isMobile" class="border-r border-line bg-base p-3">
          <div class="mb-3 h-3 w-16 bg-elevated animate-pulse" />
          <div v-for="n in 6" :key="'org-skel-' + n" class="mb-1.5 h-8 bg-elevated animate-pulse" />
        </div>
        <div class="space-y-3 p-4">
          <div class="h-8 w-48 bg-elevated animate-pulse" />
          <div class="h-10 w-full bg-elevated animate-pulse" />
          <div class="h-40 w-full bg-elevated animate-pulse" />
        </div>
      </div>

      <div
        v-else-if="loadDenied"
        role="status"
        data-testid="agent-studio-denied"
        class="card flex flex-1 flex-col items-center justify-center border-warn/40 bg-warn/10 px-6 text-center"
      >
        <Icon name="lock" :size="22" class="mb-3 text-warn" />
        <h3 class="text-sm font-semibold text-txt">{{ t('common.asyncState.permissionDeniedTitle') }}</h3>
        <p class="mt-1 max-w-md text-xs text-txt2">{{ t('common.asyncState.permissionDeniedDesc') }}</p>
        <AppButton class="mt-4" variant="outline" data-testid="agent-studio-retry" @click="load">
          {{ t('common.buttons.retry') }}
        </AppButton>
      </div>

      <div
        v-else-if="loadFailed"
        role="status"
        data-testid="agent-studio-failed"
        class="card flex flex-1 flex-col items-center justify-center border-err/40 bg-err/10 px-6 text-center"
      >
        <h3 class="text-sm font-semibold text-txt">{{ t('common.asyncState.loadFailedTitle') }}</h3>
        <p class="mt-1 max-w-md text-xs text-txt2">{{ t('common.asyncState.loadFailedDesc') }}</p>
        <AppButton class="mt-4" variant="outline" data-testid="agent-studio-retry" @click="load">
          {{ t('common.buttons.retry') }}
        </AppButton>
      </div>

      <div
        v-else-if="!agents.length && !teamBootstrapSessionId"
        class="card flex flex-1 flex-col items-center justify-center gap-3 px-6 text-center"
        data-testid="agent-studio-empty-team"
      >
        <h2 class="m-0 text-[18px] font-semibold text-txt">{{ t('pages.agentStudio.emptyTeamTitle') }}</h2>
        <p class="m-0 max-w-md text-[13px] leading-6 text-txt3">{{ t('pages.agentStudio.emptyTeamDesc') }}</p>
        <AppButton variant="primary" icon="skills" @click="openCreateTeam">
          {{ t('pages.agentStudio.emptyTeamCta') }}
        </AppButton>
        <button type="button" class="text-[12px] text-accent-2 hover:underline" @click="openCreateAgent">
          {{ t('pages.agentStudio.emptyTeamOrSingle') }}
        </button>
      </div>

      <div
        v-else
        class="card grid min-h-0 flex-1 overflow-hidden transition-[grid-template-columns] duration-[220ms] ease-in-out"
        :class="showRefreshProgress ? 'opacity-[0.55]' : ''"
        :style="cardGridStyle"
      >
      <!-- agent org tree (hidden on narrow screens; agent name bar remains) -->
      <AgentOrgSidebar
        v-if="!isMobile"
        :org="org"
        :agent-names="agentNames"
        :active-name="activeName"
        :collapsed="agentListCollapsed"
        :agents="agents"
        :projects="projects"
        @select-agent="chooseAgent"
        @rename-agent="onSidebarRenameBlocked"
        @remove-from-group="onRemoveFromGroup"
        @open-manage="openAgentManage"
        @create-root-group="openCreateRootGroup"
        @create-team="openCreateTeam"
        @create-child-group="openCreateChildGroup"
        @rename-group="openRenameGroup"
        @delete-group="confirmDeleteGroup"
        @assign-project="onAssignProject"
        @export-group="onExportGroup"
        @import-group="onImportGroup"
        @move-group="onMoveGroup"
        @move-agent="onMoveAgent"
        @toggle-collapsed="toggleAgentListCollapsed"
      />

      <TeamBootstrapPanel
        v-if="teamBootstrapSessionId"
        class="min-h-0 min-w-0"
        :session-id="teamBootstrapSessionId"
        @open-pm="onTeamBootstrapOpenPm"
        @select-pm="onTeamBootstrapSelectPm"
        @done="onTeamBootstrapDone"
        @refresh-org="onTeamBootstrapRefresh"
      />

      <!-- editor -->
      <div v-else-if="draft" class="flex min-h-0 min-w-0 flex-col overflow-hidden">
        <div
          v-if="isMobile"
          data-test="studio-name-bar"
          class="flex flex-col gap-2 border-b border-line px-4 py-2.5"
        >
          <div data-test="studio-name-row-top" class="flex min-w-0 items-center gap-2">
            <Icon name="robot" :size="15" class="shrink-0 text-accent-2" />
            <button
              ref="agentNameEl"
              type="button"
              data-test="agent-name"
              class="min-w-0 flex-1 truncate text-left text-[13px] font-medium text-txt"
              :class="agentNameTruncated ? 'cursor-pointer' : 'cursor-default'"
              :title="activeName"
              :aria-label="agentNameTruncated ? t('pages.agentStudio.mobile.fullNameAria') : undefined"
              @click="onAgentNameClick"
            >{{ activeName }}</button>
            <button
              type="button"
              data-test="org-switch"
              class="inline-flex min-h-11 shrink-0 items-center gap-1 rounded border border-line bg-elevated px-2.5 text-[12px] text-txt2 transition hover:border-line-strong hover:text-txt"
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
          </div>
          <div data-test="studio-name-row-bottom" class="flex items-center gap-2">
            <AppButton
              data-test="studio-export"
              variant="outline"
              icon="download"
              class="min-h-11"
              :disabled="exporting"
              @click="triggerExport"
            >
              {{ t('pages.agentStudio.exportImport.export') }}
            </AppButton>
            <AppButton
              v-if="dirty"
              data-test="studio-save"
              variant="primary"
              class="min-h-11"
              :disabled="saving"
              @click="save"
            >
              {{ saving ? t('common.buttons.saving') : t('common.buttons.save') }}
            </AppButton>
          </div>
        </div>
        <div
          v-else
          data-test="studio-name-bar"
          class="flex items-center gap-2 border-b border-line px-4 py-2"
        >
          <Icon name="robot" :size="15" class="shrink-0 text-accent-2" />
          <span class="min-w-0 truncate text-[13px] font-medium text-txt" :title="activeName">{{ activeName }}</span>
          <span v-if="dirty" class="chip shrink-0 border-warn/30 text-warn">{{ t('pages.agentStudio.unsaved') }}</span>
          <span
            v-else-if="justSaved"
            class="chip shrink-0 border-ok/30 bg-ok/10 text-ok"
          >{{ t('pages.agentStudio.saved') }}</span>
          <div class="ml-auto flex shrink-0 items-center gap-2">
            <AppButton
              data-test="studio-export"
              size="sm"
              variant="outline"
              icon="download"
              :disabled="exporting"
              @click="triggerExport"
            >
              {{ t('pages.agentStudio.exportImport.export') }}
            </AppButton>
            <AppButton
              data-test="studio-save"
              size="sm"
              variant="primary"
              :disabled="!dirty || saving"
              @click="save"
            >
              {{ saving ? t('common.buttons.saving') : dirty ? t('common.buttons.save') : t('pages.agentStudio.saved') }}
            </AppButton>
          </div>
        </div>

        <div data-test="studio-tabs" class="relative min-w-0 border-b border-line">
          <div
            ref="tabStripEl"
            data-test="studio-tab-strip"
            class="scroll-area flex min-w-0 gap-1 overflow-x-auto px-3 pt-2 [-webkit-overflow-scrolling:touch]"
            @scroll="syncTabFade"
          >
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
          <div
            v-if="isMobile && tabFadeLeft"
            data-test="tab-fade-left"
            class="pointer-events-none absolute inset-y-0 left-0 flex w-10 items-center justify-center bg-gradient-to-r from-surface to-transparent text-txt2"
            aria-hidden="true"
          >
            <Icon name="chevron-left" :size="14" />
          </div>
          <div
            v-if="isMobile && tabFadeRight"
            data-test="tab-fade-right"
            class="pointer-events-none absolute inset-y-0 right-0 flex w-10 items-center justify-center bg-gradient-to-l from-surface to-transparent text-txt2"
            aria-hidden="true"
          >
            <Icon name="chevron-right" :size="14" />
          </div>
        </div>

        <AgentFilesPanel
          v-if="draft"
          v-show="tab === 'files'"
          ref="filesPanelRef"
          :draft="draft"
          :dirty="dirty"
          :is-mobile="isMobile"
          :save="save"
          @error="error = $event"
          @toast="showToast"
          @update:just-saved="justSaved = $event"
          @discard="discardUnsavedChanges"
        />

        <!-- narrow-screen: non-whitelist tabs show desktop-only tip (files+data allowed) -->
        <div
          v-if="tab !== 'files' && isMobile && tab !== 'data'"
          class="flex min-h-0 flex-1 flex-col items-center justify-center gap-3 px-5 py-8 text-center"
        >
          <div class="flex h-10 w-10 items-center justify-center border border-info/35 bg-info/10 text-info">
            <Icon name="alert" :size="20" />
          </div>
          <h3 class="text-[14px] font-semibold text-txt">{{ t('pages.agentStudio.mobile.desktopOnlyTitle') }}</h3>
          <p class="max-w-[28ch] text-[12.5px] leading-relaxed text-txt2">
            {{ t('pages.agentStudio.mobile.desktopOnlyDesc', { tab: studioTabLabel }) }}
          </p>
          <button
            type="button"
            class="min-h-11 border border-line bg-transparent px-4 text-[13px] text-txt2 hover:border-accent hover:text-txt"
            data-testid="studio-mobile-back-files"
            @click="requestStudioTab('files')"
          >
            {{ t('pages.agentStudio.mobile.backToFiles') }}
          </button>
        </div>

        <AgentMcpPanel
          v-if="tab === 'mcp' && draft && !isMobile"
          :draft="draft"
          :is-project-bound="isProjectBound"
          @toast="showToast"
        />

        <AgentEnvPanel
          v-if="tab === 'env' && draft && !isMobile"
          :draft="draft"
          @toast="showToast"
        />

        <AgentPromptsPanel
          v-if="tab === 'prompts' && draft && !isMobile"
          :draft="draft"
        />

        <AgentPlatformRulesPanel
          v-if="tab === 'platform-rules' && !isMobile"
          :agent-name="activeName"
          :active="tab === 'platform-rules'"
          @toast="showToast"
        />

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

        <AgentMetaPanel
          v-if="tab === 'meta' && draft && !isMobile"
          :draft="draft"
          :org="org"
          :agent-name="activeName"
          :agent-names="agentNames"
          :projects="projects"
          :is-project-bound="isProjectBound"
          @update:org="org = $event"
          @error="(msg) => (error = msg)"
        />

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
      <template v-else>
        <div class="relative mb-2">
          <Icon name="search" :size="15" class="pointer-events-none absolute left-2.5 top-1/2 -translate-y-1/2 text-txt3" />
          <input
            v-model="manageSearch"
            type="text"
            autocomplete="off"
            :placeholder="t('pages.agentStudio.org.manageSearchPlaceholder')"
            class="w-full border border-line bg-base py-2 pl-8 pr-8 text-[13px] text-txt outline-none transition focus:border-accent"
            data-test="manage-search"
          />
          <button
            v-if="manageSearch"
            type="button"
            class="absolute right-1.5 top-1/2 flex h-6 w-6 -translate-y-1/2 items-center justify-center text-txt3 hover:bg-elevated hover:text-txt"
            :aria-label="t('pages.agentStudio.org.manageClearSearch')"
            data-test="manage-search-clear"
            @click="clearManageSearch"
          >
            <Icon name="close" :size="14" />
          </button>
        </div>
        <p class="mb-2 text-[12px] tabular-nums text-txt3" data-test="manage-search-count">
          {{
            manageSearchActive
              ? t('pages.agentStudio.org.manageSearchCount', { matched: filteredManageNames.length, total: agentNames.length })
              : t('pages.agentStudio.org.manageTotalCount', { total: agentNames.length })
          }}
        </p>
        <div v-if="manageSearchActive && !filteredManageNames.length" class="border border-dashed border-line px-4 py-8 text-center">
          <Icon name="search" :size="20" class="mx-auto mb-2 text-txt3" />
          <p class="text-[13px] font-medium text-txt">{{ t('pages.agentStudio.org.manageNoMatchTitle') }}</p>
          <p class="mt-1 text-[12px] text-txt3">{{ t('pages.agentStudio.org.manageNoMatchDesc') }}</p>
          <AppButton class="mt-3" size="sm" variant="outline" @click="clearManageSearch">
            {{ t('pages.agentStudio.org.manageClearSearch') }}
          </AppButton>
        </div>
        <div v-else class="flex flex-col gap-0.5">
        <div
          v-for="name in filteredManageNames"
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
          <span class="min-w-0 flex-1 truncate text-[13px] text-txt">
            <template v-if="manageNameHighlight(name).hit">
              {{ manageNameHighlight(name).before }}<mark class="bg-warn/25 px-0 text-inherit">{{ manageNameHighlight(name).hit }}</mark>{{ manageNameHighlight(name).after }}
            </template>
            <template v-else>{{ name }}</template>
          </span>
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
      </template>
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
                  <span class="flex min-w-0 flex-1 items-baseline overflow-hidden" data-org-gname>
                    <span class="min-w-0 truncate font-medium text-txt2">{{ row.name }}</span><span
                      v-if="row.projectLabel"
                      class="shrink-0 font-normal text-txt3"
                      data-org-project
                    >({{ row.projectLabel }})</span>
                  </span>
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

    <Teleport to="body">
      <div
        v-if="isMobile && showFullNameTip"
        data-test="agent-name-tip-backdrop"
        class="fixed inset-0 z-[9998]"
        @click="closeFullNameTip"
      />
      <div
        v-if="isMobile && showFullNameTip"
        data-test="agent-name-tip"
        class="fixed z-[9999] border border-line bg-elevated px-3 py-2.5 text-[12.5px] text-txt shadow-card"
        :style="fullNameTipStyle"
        @click.stop
      >
        <small class="mb-1 block text-[11px] text-txt3">{{ t('pages.agentStudio.mobile.fullNameLabel') }}</small>
        <b class="break-all font-semibold">{{ activeName }}</b>
      </div>
    </Teleport>

    <AgentCreateWizard
      :open="showCreateWizard"
      :existing-names="agents.map((a) => a.name)"
      @close="showCreateWizard = false"
      @created="onWizardCreated"
    />

    <CreateAgentTeamWizard
      :open="showTeamWizard"
      :existing-names="agents.map((a) => a.name)"
      @close="showTeamWizard = false"
      @started="onTeamBootstrapStarted"
    />

    <AppModal
      :open="showAssignPick"
      :title="t('pages.agentStudio.org.assignTitle', { name: assignGroupName })"
      :width="460"
      data-test="org-assign-pick"
      :close-on-backdrop="!assignApplying"
      @close="closeAssignModals"
    >
      <div class="space-y-3 text-[13px] text-txt2">
        <p>{{ t('pages.agentStudio.org.assignIntro', { n: assignMembers.length }) }}</p>
        <div class="flex max-h-48 flex-col gap-1.5 overflow-y-auto">
          <label
            v-for="p in projects"
            :key="p.id"
            class="flex cursor-pointer items-center gap-2.5 border border-line bg-base px-2.5 py-2"
            :class="assignTargetId === p.id ? 'border-accent bg-accent-dim' : ''"
          >
            <input v-model="assignTargetId" type="radio" class="accent-accent" :value="p.id" />
            <span class="min-w-0 flex-1 text-txt">{{ p.name }}</span>
            <span class="font-mono text-[11px] text-txt3">{{ p.id }}</span>
          </label>
        </div>
        <p v-if="!projects.length" class="text-[12px] text-warn">{{ t('pages.agentStudio.org.assignNoProjects') }}</p>
        <div class="max-h-28 overflow-y-auto text-[11px] leading-relaxed text-txt3">
          {{ t('pages.agentStudio.org.assignMembers', { list: assignMemberList }) }}
        </div>
      </div>
      <template #footer>
        <AppButton size="sm" variant="ghost" :disabled="assignApplying" @click="closeAssignModals">{{ t('common.buttons.cancel') }}</AppButton>
        <AppButton
          size="sm"
          variant="primary"
          data-test="org-assign-submit"
          :disabled="assignApplying || !projects.length || !assignTargetId"
          @click="onAssignPickNext"
        >{{ assignApplying ? t('pages.agentStudio.org.assignApplying') : t('pages.agentStudio.org.assignSubmit') }}</AppButton>
      </template>
    </AppModal>

    <AppModal
      :open="showAssignCover"
      :title="t('pages.agentStudio.project.assignCoverTitle')"
      :width="460"
      data-test="org-assign-cover"
      :close-on-backdrop="!assignApplying"
      @close="cancelAssignCover"
    >
      <div class="space-y-2 text-[13px] leading-6 text-txt2">
        <p>{{ t('pages.agentStudio.project.assignCoverLead', { n: assignDiffBound.length }) }}</p>
        <div class="border border-warn/35 bg-warn/10 px-3 py-2.5 text-[12px]">
          <b class="text-warn">{{ t('pages.agentStudio.project.assignCoverWarn') }}</b>
          <ul class="mt-1.5 list-disc space-y-1 pl-5">
            <li>{{ t('pages.agentStudio.project.switchItemMemory') }}</li>
            <li>{{ t('pages.agentStudio.project.switchItemContext') }}</li>
            <li>{{ t('pages.agentStudio.project.switchItemJobs') }}</li>
            <li>{{ t('pages.agentStudio.project.switchItemPm') }}</li>
          </ul>
        </div>
        <p class="text-[12px]">
          {{ t('pages.agentStudio.project.assignCoverCross') }}
          {{ t('pages.agentStudio.project.assignCoverAffected', { list: assignAffectedList }) }}
        </p>
        <p class="text-[11.5px] text-txt3">{{ t('pages.agentStudio.project.assignImmediateHint') }}</p>
      </div>
      <template #footer>
        <AppButton size="sm" variant="ghost" :disabled="assignApplying" @click="cancelAssignCover">{{ t('common.buttons.cancel') }}</AppButton>
        <AppButton
          size="sm"
          variant="primary"
          data-test="org-assign-cover-ok"
          :disabled="assignApplying"
          @click="maybeAssignDraftThenApply"
        >{{ t('pages.agentStudio.project.switchConfirm', { name: assignTargetLabel }) }}</AppButton>
      </template>
    </AppModal>

    <AppModal
      :open="showAssignDraft"
      :title="t('pages.agentStudio.project.assignDraftTitle')"
      :width="460"
      data-test="org-assign-draft"
      :close-on-backdrop="!assignApplying"
      @close="keepAssignDraft"
    >
      <p class="text-[13px] leading-6 text-txt2">
        {{
          t('pages.agentStudio.project.assignDraftBody', {
            name: activeName,
            draft: projectNameById(draft?.projectId || '') || t('pages.agentStudio.project.unbound'),
            target: assignTargetLabel,
          })
        }}
      </p>
      <template #footer>
        <AppButton size="sm" variant="ghost" data-test="org-assign-draft-keep" :disabled="assignApplying" @click="keepAssignDraft">
          {{ t('pages.agentStudio.project.assignDraftKeep') }}
        </AppButton>
        <AppButton
          size="sm"
          variant="primary"
          data-test="org-assign-draft-ok"
          :disabled="assignApplying"
          @click="applyAssign(true)"
        >{{ t('pages.agentStudio.project.assignDraftOverwrite') }}</AppButton>
      </template>
    </AppModal>


    <Teleport to="body">
      <div
        v-if="toastMsg"
        data-test="studio-toast"
        class="fixed bottom-5 right-5 z-[10000] border border-line bg-elevated px-3.5 py-2 text-[12px] text-txt2 shadow-card"
      >
        {{ toastMsg }}
      </div>
    </Teleport>
  </div>
</template>
