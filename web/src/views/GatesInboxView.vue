<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, watch, nextTick } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/ui/Icon.vue'
import EmptyState from '@/components/ui/EmptyState.vue'
import PipelineFilter from '@/components/ui/PipelineFilter.vue'
import ProjectFilter from '@/components/ui/ProjectFilter.vue'
import TagFilter from '@/components/ui/TagFilter.vue'
import GateApproval from '@/components/run/GateApproval.vue'
import GateShareLinkPanel from '@/components/run/GateShareLinkPanel.vue'
import InboxPendingCard from '@/components/inbox/InboxPendingCard.vue'
import ReviewShell from '@/components/run/ReviewShell.vue'
import { REVIEW_SHELL_WIDTH_KEY_APPROVAL } from '@/lib/inbox/reviewLayoutBudget'
import ReviewComposer from '@/components/run/ReviewComposer.vue'
import ArtifactLoadingPane from '@/components/run/ArtifactLoadingPane.vue'
import ClarifyProductStage from '@/components/run/ClarifyProductStage.vue'
import ReactArtifactStage from '@/components/run/ReactArtifactStage.vue'
import RefreshStrip from '@/components/run/RefreshStrip.vue'
import AppInlineError from '@/components/ui/AppInlineError.vue'
import Pagination from '@/components/ui/Pagination.vue'
import { api, isPaginated } from '@/lib/api/api'
import { adaptInboxContextToRun } from '@/lib/inbox/inboxContext'
import { usePipelineFilter } from '@/lib/composables/usePipelineFilter'
import { useTagFilter } from '@/lib/composables/useTagFilter'
import { useProjectContext } from '@/lib/composables/useProjectContext'
import { usePendingGates } from '@/lib/inbox/usePendingGates'
import { addClarifyAnnotation, useClarifyDraft } from '@/lib/inbox/useClarifyDraft'
import { previewPickLabel, type AppPreviewPickPayload } from '@/lib/shared/previewPickUrl'
import { useBreakpoint } from '@/lib/composables/useBreakpoint'
import { inboxSecondaryLine } from '@/lib/inbox/inboxDisplay'
import {
  inboxComposerMode,
  pickInboxClarifySession,
  resolveInboxReviewState,
} from '@/lib/inbox/inboxReviewMode'
import {
  inboxTripleKey,
  isInboxLeftPendingError,
  pickNextActiveAfterRemove,
} from '@/lib/inbox/inboxActiveSelection'
import { isAbortError } from '@/lib/run/liveLogRehydrate'
import { applyPreviewArtifactName, inboxStageRemoteKind } from '@/lib/run/reactArtifactPreview'
import { createPendingAcpBuffer, pickAcpRails } from '@/lib/run/pendingAcpBuffer'
import { deliverOrBufferDialogueAcp } from '@/lib/run/dialogueAcpDelivery'
import {
  createBusySeedRetryController,
  runBusySeedRetry,
} from '@/lib/run/busySeedRetry'
import { createWsReconnectController } from '@/lib/run/wsReconnect'
import { useToast } from '@/lib/composables/useToast'
import { inboxShareKind, isHumanGateInboxItem, isShareableInboxItem } from '@/lib/inbox/gateShareLink'
import type { AcpEvent, Gate, GateInboxItem, GateShareInboxStatus, InboxItem, Run } from '@/lib/shared/types'

const router = useRouter()
const route = useRoute()
const { t } = useI18n()
const toast = useToast()
const {
  displayedItems,
  remoteItems,
  totalCount,
  refresh,
  peek,
  applyPending,
  removeItemLocally,
  hasPendingUpdate,
  pendingMeta,
  lastPeekAt,
  itemKey,
  ariaBusy,
} = usePendingGates()
const PAGE_SIZE = 20
const SKELETON_CARDS = 5
const listItems = ref<InboxItem[]>([])
/** Snapshot before the latest list mutation — used for neighbor active selection. */
let listSnapshotForNeighbor: InboxItem[] = []
const listTotal = ref(0)
const listPage = ref(1)
/** true on first paint so EmptyState cannot flash before onMounted loadList (plan g2.1). */
const listLoading = ref(true)
const listLoadError = ref<string | null>(null)
/** Monotonic generation: stale listGates responses must not overwrite local converge. */
let listLoadGeneration = 0
const { isMobile } = useBreakpoint()
const active = ref<InboxItem | null>(null)
const mobileView = ref<'list' | 'detail'>('list')
const listScrollTop = ref(0)
const listEl = ref<HTMLElement | null>(null)
const gateApprovalRef = ref<InstanceType<typeof GateApproval> | null>(null)
const reviewChatRef = ref<{
  applyReviewFrame?: (frame: any) => boolean | void
  applyAcpEvents?: (events: any[] | undefined, nodeId?: string) => boolean | void
  discardLastQueued?: () => void
  isSessionBusy?: () => boolean
} | null>(null)
/**
 * Review/clarify WS frames that arrived while ClarifyChat was unmounted
 * (hard loadActiveRun nulls activeRun → ReviewComposer gone). Flushed after mount.
 */
let pendingReviewFrames: Record<string, unknown>[] = []
/**
 * ACP frames during the same hard-load window (parity with review buffer).
 * Latest cumulative events per nodeId; flushed after busy slot rebuild.
 */
const pendingAcpFrames = createPendingAcpBuffer()
/** Snapshot reactSessions stashed when activeRun is still null during hard load. */
let pendingSnapshotSessions: Run['reactSessions'] | null = null
const { selected } = usePipelineFilter()
const { selected: selectedProject, ensureHydrated: hydrateProject } = useProjectContext()
const { selectedTags } = useTagFilter()
const projectFilterOpen = ref(false)
const pipelineFilterOpen = ref(false)
const tagFilterOpen = ref(false)

watch(projectFilterOpen, (v) => {
  if (v) {
    pipelineFilterOpen.value = false
    tagFilterOpen.value = false
  }
})
watch(pipelineFilterOpen, (v) => {
  if (v) {
    projectFilterOpen.value = false
    tagFilterOpen.value = false
  }
})
watch(tagFilterOpen, (v) => {
  if (v) {
    projectFilterOpen.value = false
    pipelineFilterOpen.value = false
  }
})

const showUpdateBanner = ref(false)
const showProcessedBanner = ref(false)
const manualRefreshing = ref(false)
/** First-screen skeleton when loading with no rows yet (plan g2.1). */
const showListSkeleton = computed(
  () => (listLoading.value || manualRefreshing.value) && !listItems.value.length && !listLoadError.value,
)
/** First-screen error when load failed and nothing to show. */
const showListError = computed(() => !!listLoadError.value && !listItems.value.length)
/** Keep old rows + RefreshStrip/fade on user refresh or filter reload (plan g2.2). */
const showListRefresh = computed(
  () => (listLoading.value || manualRefreshing.value) && listItems.value.length > 0,
)
/** Includes silent peek ariaBusy — no RefreshStrip for silent poll (plan g2.3). */
const listPanelBusy = computed(
  () => listLoading.value || manualRefreshing.value || ariaBusy.value,
)
/** Review confirm validation failure (bottom status bar; replaces force-failure toast). */
const clarifyConfirmError = ref<string | null>(null)

/** Triples that left pending — never softRefresh/load inbox-context for these. */
const processedTriples = new Set<string>()
/** Processed triples observed absent from a subsequent list load (safe to unmark if they reappear). */
const confirmedAbsentTriples = new Set<string>()
/** Per-triple AbortController for in-flight inbox-context fetches. */
const inboxContextAborts = new Map<string, AbortController>()
/** Single-flight: one softRefresh/loadActiveRun at a time for the selected triple. */
let inboxContextInFlight: string | null = null
/** Processing intent lock — blocks list reselection until converge finishes. */
const processingLock = ref(false)
/** Snapshot used to reopen Active WS if resume/finish rolls back. */
let processingIntentTriple: string | null = null

function markProcessed(it: Pick<InboxItem, 'runId' | 'nodeId' | 'iteration'>) {
  processedTriples.add(inboxTripleKey(it))
}

function unmarkProcessed(it: Pick<InboxItem, 'runId' | 'nodeId' | 'iteration'>) {
  const key = inboxTripleKey(it)
  processedTriples.delete(key)
  confirmedAbsentTriples.delete(key)
}

function isProcessedTriple(it: Pick<InboxItem, 'runId' | 'nodeId' | 'iteration'> | null | undefined) {
  if (!it) return false
  return processedTriples.has(inboxTripleKey(it))
}

function abortInboxContext(triple: string) {
  const ctrl = inboxContextAborts.get(triple)
  if (!ctrl) return
  ctrl.abort()
  inboxContextAborts.delete(triple)
  if (inboxContextInFlight === triple) inboxContextInFlight = null
}

function acquireInboxContextSignal(triple: string): AbortSignal | null {
  // Single-flight: drop duplicate softRefresh/load while one request is in flight.
  if (inboxContextInFlight === triple) return null
  // Switching selection: abort the previous triple's in-flight request.
  if (inboxContextInFlight && inboxContextInFlight !== triple) {
    abortInboxContext(inboxContextInFlight)
  }
  abortInboxContext(triple)
  const ctrl = new AbortController()
  inboxContextAborts.set(triple, ctrl)
  inboxContextInFlight = triple
  return ctrl.signal
}

function releaseInboxContextSignal(triple: string, signal: AbortSignal) {
  const ctrl = inboxContextAborts.get(triple)
  if (ctrl?.signal === signal) inboxContextAborts.delete(triple)
  if (inboxContextInFlight === triple) inboxContextInFlight = null
}

/**
 * User clicked approve / force-finish clarify: short-circuit immediately so the
 * resume/finish window cannot emit more inbox-context traffic.
 */
function beginProcessingIntent(it: Pick<InboxItem, 'runId' | 'nodeId' | 'iteration'>) {
  const triple = inboxTripleKey(it)
  processingIntentTriple = triple
  processingLock.value = true
  markProcessed(it)
  abortInboxContext(triple)
  closeActiveRunWs()
}

/** resumeGate / reactReply(force) failed — restore fetchability and unlock. */
function rollbackProcessingIntent(it: Pick<InboxItem, 'runId' | 'nodeId' | 'iteration'>) {
  const triple = inboxTripleKey(it)
  if (processingIntentTriple === triple) processingIntentTriple = null
  unmarkProcessed(it)
  processingLock.value = false
  // Reopen Active WS only if this item is still the selected pending row.
  if (
    active.value &&
    inboxTripleKey(active.value) === triple &&
    isActiveStillInList(active.value) &&
    !isProcessedTriple(active.value)
  ) {
    connectActiveRunWs(active.value.runId)
  }
}

function endProcessingIntent() {
  processingIntentTriple = null
  processingLock.value = false
}

function isActiveStillInList(it: InboxItem | null | undefined = active.value) {
  if (!it) return false
  const key = itemKey(it)
  return listItems.value.some((row) => itemKey(row) === key)
}

function shouldFetchActiveInboxContext() {
  const it = active.value
  if (!it) return false
  if (isProcessedTriple(it)) return false
  if (!isActiveStillInList(it)) return false
  return true
}

/** Invalidate in-flight loadList writebacks (e.g. after local approve converge). */
function invalidateListLoads() {
  listLoadGeneration++
}

/** Drop a lagging list row so users cannot re-select a processed ghost. */
function removeListItemLocally(removedKey: string) {
  // Any older listGates still in flight must not restore this row after unlock.
  invalidateListLoads()
  const before = listItems.value.length
  listItems.value = listItems.value.filter((row) => itemKey(row) !== removedKey)
  const removed = before - listItems.value.length
  if (removed > 0) listTotal.value = Math.max(0, listTotal.value - removed)
  // Keep sidebar badge / singleton in lockstep with the page list.
  removeItemLocally(removedKey)
}

/**
 * List non-empty + invalid/missing active → pick first valid item.
 * Empty list → clear active (full empty inbox). Never leave a Run # - shell.
 */
function ensureValidActive() {
  if (processingLock.value) return
  const list = listItems.value
  if (!list.length) {
    if (active.value) active.value = null
    return
  }
  if (active.value && list.some((it) => itemKey(it) === itemKey(active.value!))) {
    return
  }
  active.value = list[0]
}

/**
 * Allow re-fetch only after a processed triple was confirmed gone, then reappears as pending.
 * Avoids clearing the mark on the immediate lagging loadList that still contains the item.
 */
function reconcileProcessedWithList(list: InboxItem[]) {
  const pending = new Set(list.map((it) => inboxTripleKey(it)))
  for (const key of [...processedTriples]) {
    if (!pending.has(key)) {
      confirmedAbsentTriples.add(key)
      continue
    }
    if (confirmedAbsentTriples.has(key)) {
      processedTriples.delete(key)
      confirmedAbsentTriples.delete(key)
    }
  }
}

/** After an item leaves the list: neighbor active (or null). Never keep stale active. */
function selectActiveAfterRemove(prevList: InboxItem[], removedKey: string) {
  active.value = pickNextActiveAfterRemove(prevList, removedKey, itemKey)
  showProcessedBanner.value = false
  if (isMobile.value && !listItems.value.length) mobileView.value = 'list'
}

/** Quietly converge when inbox-context 404 means the item left pending. */
function handleLeftInboxContext(it: InboxItem) {
  markProcessed(it)
  const key = itemKey(it)
  const prevList = listItems.value.some((row) => itemKey(row) === key)
    ? listItems.value.slice()
    : [...listItems.value, it]
  // Sync UI: lagging list must not keep a clickable ghost with shouldFetch=false.
  removeListItemLocally(key)
  const stillActive = active.value !== null && itemKey(active.value) === key
  if (!stillActive) return
  closeActiveRunWs()
  activeRun.value = null
  activeRunLoadError.value = false
  selectActiveAfterRemove(prevList, key)
}

async function loadList({ showLoading = false }: { showLoading?: boolean } = {}) {
  const gen = ++listLoadGeneration
  listLoadError.value = null
  if (showLoading) listLoading.value = true
  try {
    const data = await api.listGates({
      page: listPage.value,
      pageSize: PAGE_SIZE,
      wf: selected.value || undefined,
      projectId: selectedProject.value || undefined,
      tag: selectedTags.value.join(',') || undefined,
    })
    // Discard overdue snapshots so they cannot resurrect a just-removed gate.
    if (gen !== listLoadGeneration) return
    listSnapshotForNeighbor = listItems.value.slice()
    if (isPaginated(data)) {
      listItems.value = data.items
      listTotal.value = data.total
    } else {
      listItems.value = data
      listTotal.value = data.length
    }
    listLoadError.value = null
    reconcileProcessedWithList(listItems.value)
    // Independent loadList success path: repair invalid selection (no Run # - shell).
    if (!processingLock.value) ensureValidActive()
  } catch (err) {
    if (gen !== listLoadGeneration) return
    /* keep previous list on transient failure; surface error when nothing to show */
    listLoadError.value =
      err instanceof Error && err.message
        ? err.message
        : String(t('common.asyncState.loadFailedDesc'))
  } finally {
    if (gen === listLoadGeneration) listLoading.value = false
  }
}

function retryListLoad() {
  void loadList({ showLoading: true })
}

watch(selected, () => {
  listPage.value = 1
  loadList({ showLoading: true })
})

watch(selectedProject, () => {
  listPage.value = 1
  loadList({ showLoading: true })
})

watch(listPage, () => {
  loadList({ showLoading: true })
})
watch(() => route.query.tag, () => {
  listPage.value = 1
  loadList({ showLoading: true })
})

const isGateEditing = computed(() => gateApprovalRef.value?.isEditing ?? false)

const {
  draft: clarifyDraft,
  attachments: clarifyAttachments,
  annotations: clarifyAnnotations,
} = useClarifyDraft(
  () => (active.value?.type === 'clarify' ? active.value.runId : null),
  () => (active.value?.type === 'clarify' ? active.value.nodeId : null),
)

/** VNC pick on app_preview inbox stage → same ReAct annotation chips as Run review. */
const lastStagedAppPreviewPick = ref<AppPreviewPickPayload | null>(null)

watch(
  () => (active.value ? itemKey(active.value) : ''),
  () => {
    lastStagedAppPreviewPick.value = null
  },
)

function onAppPreviewStagedPick(payload: AppPreviewPickPayload | null) {
  lastStagedAppPreviewPick.value = payload
}

function onAppPreviewReviewPick(payload: AppPreviewPickPayload) {
  const rid = active.value?.type === 'clarify' ? active.value.runId : ''
  const nid = active.value?.type === 'clarify' ? active.value.nodeId : ''
  if (!rid || !nid) return
  const url = (payload.url || '').trim()
  const result = addClarifyAnnotation(rid, nid, {
    selector: payload.selector,
    url: url || undefined,
    label: previewPickLabel(url, payload.selector, payload.tagName),
  })
  if (result === 'duplicate') toast.warn(t('pages.reviewComposer.alreadyAdded'))
  lastStagedAppPreviewPick.value = null
}

function mergeStagedAppPreviewPick(
  annotations: import('@/lib/shared/types').ReactAnnotation[],
): import('@/lib/shared/types').ReactAnnotation[] {
  const staged = lastStagedAppPreviewPick.value
  if (!staged?.selector) return annotations
  const url = (staged.url || '').trim()
  if (
    annotations.some(
      (a) => a.selector === staged.selector && (a.url || '').trim() === url,
    )
  ) {
    return annotations
  }
  return [
    ...annotations,
    {
      selector: staged.selector,
      url: url || undefined,
      label: previewPickLabel(url, staged.selector, staged.tagName),
    },
  ]
}

const isClarifyEditing = computed(
  () =>
    clarifyDraft.value.trim().length > 0 ||
    clarifyAttachments.value.length > 0 ||
    clarifyAnnotations.value.length > 0,
)

const isEditing = computed(() => isGateEditing.value || isClarifyEditing.value)

const statusPillClass = computed(() => {
  if (hasPendingUpdate.value) return 'pending'
  if (isEditing.value) return 'editing'
  return 'idle'
})

const statusPillText = computed(() => {
  if (hasPendingUpdate.value) return t('pages.gatesInbox.statusPending')
  if (isEditing.value) return t('pages.gatesInbox.statusEditing')
  return t('pages.gatesInbox.statusIdle')
})

const updateBannerDetail = computed(() => {
  const meta = pendingMeta.value
  if (!meta) return t('pages.gatesInbox.updateBannerDetailEmpty')
  const parts: string[] = []
  if (meta.added) parts.push(t('pages.gatesInbox.updateAdded', { n: meta.added }))
  if (meta.removed) parts.push(t('pages.gatesInbox.updateRemoved', { n: meta.removed }))
  const summary = parts.length ? `（${parts.join('，')}）` : ''
  return t('pages.gatesInbox.updateBannerDetail', { summary })
})

function syncActiveAfterApply(list: InboxItem[], prevKey: string | null) {
  // Processing intent owns selection until converge/unlock — ignore list-apply swaps.
  if (processingLock.value) return
  if (!active.value && list.length) {
    active.value = list[0]
    return
  }
  if (!active.value) return

  const key = prevKey ?? itemKey(active.value)
  const stillExists = list.some((it) => itemKey(it) === key)

  if (stillExists) {
    showProcessedBanner.value = false
    return
  }

  if (isEditing.value) {
    showProcessedBanner.value = true
    return
  }

  const prevList =
    listSnapshotForNeighbor.some((it) => itemKey(it) === key)
      ? listSnapshotForNeighbor
      : active.value
        ? [active.value, ...list.filter((it) => itemKey(it) !== key)]
        : list
  active.value = pickNextActiveAfterRemove(prevList, key, itemKey)
  showProcessedBanner.value = false
  if (isMobile.value && !list.length) mobileView.value = 'list'
}

function checkProcessedWhileEditing() {
  if (!active.value || !isEditing.value) {
    if (!isEditing.value) showProcessedBanner.value = false
    return
  }
  const key = itemKey(active.value)
  const inRemote = remoteItems.value.some((it) => itemKey(it) === key)
  if (!inRemote) showProcessedBanner.value = true
}

watch(hasPendingUpdate, (pending) => {
  if (pending) showUpdateBanner.value = true
})

watch(lastPeekAt, () => {
  if (hasPendingUpdate.value) showUpdateBanner.value = true
  checkProcessedWhileEditing()
})

watch(isEditing, (editing) => {
  if (!editing) showProcessedBanner.value = false
  else checkProcessedWhileEditing()
})

watch(isMobile, (mobile) => {
  if (!mobile) mobileView.value = 'list'
})

function isActive(it: InboxItem) {
  return active.value !== null && itemKey(active.value) === itemKey(it)
}

const activeRun = ref<Run | null>(null)
const activeRunLoading = ref(false)
const activeRunLoadError = ref(false)

/** Active-run event socket — mirrors RunDetailView artifact_edit/react → reload. */
let activeRunWs: WebSocket | undefined
let activeRunWsRunId = ''
/**
 * Live busy from review/acp WS frames (not stale activeRun.reactSessions snapshot).
 * Used to gate softRefresh while clarify session is mid-turn (g3.2 / review v3).
 */
const clarifyLiveBusy = ref(false)
/** True once thought/message rails were applied to a dialogue surface (seed or live). */
let dialogueRailsFilled = false
/** True once a live WS ACP frame applied content (stops seed retry per f2). */
let dialogueLiveIncremental = false
const busySeedRetry = createBusySeedRetryController()
const activeRunWsReconnect = createWsReconnectController({
  connect: () => {
    const id = active.value?.runId
    if (id) connectActiveRunWs(id, { fromReconnect: true })
  },
  shouldReconnect: () =>
    !!active.value?.runId &&
    (active.value.type === 'clarify' || active.value.type === 'gate') &&
    !isProcessedTriple(active.value) &&
    isActiveStillInList(active.value),
})

function isClarifySoftRefreshBlocked(): boolean {
  if (active.value?.type !== 'clarify') return false
  if (clarifyLiveBusy.value) return true
  if (reviewChatRef.value?.isSessionBusy?.()) return true
  return false
}

function closeActiveRunWs() {
  busySeedRetry.stop()
  activeRunWsReconnect.markIntentionalClose()
  activeRunWs?.close()
  activeRunWs = undefined
  activeRunWsRunId = ''
  clarifyLiveBusy.value = false
  dialogueRailsFilled = false
  dialogueLiveIncremental = false
  pendingReviewFrames = []
  pendingAcpFrames.clear()
  pendingSnapshotSessions = null
}

/** Producer nodeId for busy session restore (clarify node or gate react upstream). */
function activeDialogueNodeId(r?: Run | null): string | null {
  if (active.value?.type === 'clarify') return active.value.nodeId
  if (active.value?.type === 'gate') {
    return (
      r?.gate?.reactUpstreamNodeId ||
      activeRun.value?.gate?.reactUpstreamNodeId ||
      (active.value as Gate).reactUpstreamNodeId ||
      null
    )
  }
  return null
}

/** Deliver ACP to chat/gate, or buffer until surface remounts after hard load. */
function applyOrBufferAcpFrame(m: {
  nodeId?: string
  events?: AcpEvent[]
  busy?: boolean
}) {
  const events = Array.isArray(m.events) ? m.events : []
  const nodeId = m.nodeId || ''
  const forClarify = active.value?.type === 'clarify'
  const forGate = active.value?.type === 'gate'
  const result = deliverOrBufferDialogueAcp({
    forClarify,
    forGate,
    events,
    nodeId,
    applyClarify: (evs, nid) => {
      if (!reviewChatRef.value?.applyAcpEvents) return false
      return reviewChatRef.value.applyAcpEvents(evs, nid)
    },
    applyGate: (evs) => {
      if (!gateApprovalRef.value?.applyAcpEvents) return false
      return gateApprovalRef.value.applyAcpEvents(evs) !== false
    },
  })
  if (result === 'applied') {
    const rails = pickAcpRails(events)
    if (rails.thought || rails.message) {
      dialogueRailsFilled = true
      dialogueLiveIncremental = true
    }
  }
  // Hard-load / slot-not-ready: keep latest cumulative ACP for seed-then-live.
  if (result === 'buffer' && (forClarify || forGate)) {
    pendingAcpFrames.push({
      nodeId: m.nodeId,
      events,
      busy: m.busy,
    })
  }
}

function flushPendingAcpFrames() {
  if (!pendingAcpFrames.size) return
  const frames = pendingAcpFrames.takeAll()
  for (const frame of frames) {
    applyOrBufferAcpFrame(frame)
  }
}

/**
 * One-shot REST seed from LiveNodeEvents / persisted snapshot.
 * Returns true only when non-empty rails were actually applied (not merely buffered).
 */
async function seedClarifyAcpFromNodeEventsOnce(
  runId: string,
  nodeId: string,
): Promise<boolean> {
  try {
    const r = await api.nodeEvents(runId, nodeId, { limit: 50 })
    if (!r.events?.length) return false
    const rails = pickAcpRails(r.events)
    if (!rails.thought && !rails.message) return false
    const forClarify = active.value?.type === 'clarify'
    const forGate = active.value?.type === 'gate'
    const result = deliverOrBufferDialogueAcp({
      forClarify,
      forGate,
      events: r.events,
      nodeId,
      applyClarify: (evs, nid) => {
        if (!reviewChatRef.value?.applyAcpEvents) return false
        return reviewChatRef.value.applyAcpEvents(evs, nid)
      },
      applyGate: (evs) => {
        if (!gateApprovalRef.value?.applyAcpEvents) return false
        return gateApprovalRef.value.applyAcpEvents(evs) !== false
      },
    })
    if (result === 'buffer') {
      pendingAcpFrames.push({ nodeId, events: r.events, busy: true })
      return false
    }
    dialogueRailsFilled = true
    return true
  } catch {
    /* 502 / empty — caller retries while busy */
    return false
  }
}

/** Busy-guarded seed with periodic retry until content / live / idle (f2). */
function startBusySeedRetry(runId: string, nodeId: string) {
  busySeedRetry.start(async (signal) => {
    await runBusySeedRetry({
      signal,
      isBusy: () => {
        if (clarifyLiveBusy.value) return true
        const snap = activeRun.value?.reactSessions?.[nodeId]
        return !!snap?.busy
      },
      hasContent: () => dialogueRailsFilled,
      liveIncrementalReceived: () => dialogueLiveIncremental,
      seed: async () => seedClarifyAcpFromNodeEventsOnce(runId, nodeId),
    })
  })
}

/** Project authoritative busy/queue onto ClarifyChat / GateApproval. */
function restoreReactSessions(r: { reactSessions?: Run['reactSessions']; gate?: Run['gate'] } | null) {
  const nodeId = activeDialogueNodeId(r as Run | null)
  if (!nodeId) return
  const sessions = r?.reactSessions ?? pendingSnapshotSessions
  if (!sessions) return
  const snap = sessions[nodeId]
  if (!snap || typeof snap !== 'object') return
  clarifyLiveBusy.value = !!snap.busy
  const frame = {
    event: 'queue_state',
    nodeId,
    waiting: snap.waiting ?? 0,
    items: snap.items ?? [],
    busy: !!snap.busy,
    activeItem: snap.activeItem,
  }
  let delivered = false
  if (active.value?.type === 'gate' && gateApprovalRef.value?.applyReviewFrame) {
    gateApprovalRef.value.applyReviewFrame(frame)
    delivered = true
  }
  if (reviewChatRef.value?.applyReviewFrame) {
    const ok = reviewChatRef.value.applyReviewFrame(frame)
    if (ok !== false) delivered = true
  }
  if (!delivered) pendingReviewFrames.push(frame)
  pendingSnapshotSessions = null
}

function flushPendingReviewFrames() {
  if (!pendingReviewFrames.length) return
  const frames = pendingReviewFrames
  pendingReviewFrames = []
  for (const frame of frames) {
    gateApprovalRef.value?.applyReviewFrame?.(frame)
    reviewChatRef.value?.applyReviewFrame?.(frame)
  }
}

/** Deliver review frame to chat/gate, or buffer until surface remounts after hard load. */
function applyOrBufferReviewFrame(m: Record<string, unknown>) {
  let applied = false
  if (gateApprovalRef.value?.applyReviewFrame) {
    gateApprovalRef.value.applyReviewFrame(m)
    applied = true
  }
  if (reviewChatRef.value?.applyReviewFrame) {
    const ok = reviewChatRef.value.applyReviewFrame(m)
    if (ok !== false) applied = true
  }
  if (!applied && (active.value?.type === 'clarify' || active.value?.type === 'gate')) {
    pendingReviewFrames.push(m)
  }
}

/**
 * After hard load / WS reconnect: project reactSessions + flush frames buffered
 * while ReviewComposer / GateApproval was unmounted (WS Broadcast race).
 * Order: queue_state → seed nodeEvents → flush pendingAcpBuffer → live (g2.1).
 * Covers clarify and gate.
 */
async function projectClarifySessionAfterLoad(r: Run) {
  dialogueRailsFilled = false
  dialogueLiveIncremental = false
  await nextTick()
  restoreReactSessions(r)
  await nextTick()
  flushPendingReviewFrames()
  await nextTick()
  const nodeId = activeDialogueNodeId(r)
  const snap = nodeId ? r.reactSessions?.[nodeId] : null
  if (nodeId && snap?.busy && active.value?.runId) {
    // First attempt immediately, then busy-guarded retry (g3).
    await seedClarifyAcpFromNodeEventsOnce(active.value.runId, nodeId)
    await nextTick()
    flushPendingAcpFrames()
    if (!dialogueRailsFilled && !dialogueLiveIncremental) {
      startBusySeedRetry(active.value.runId, nodeId)
    }
  } else {
    // Authority not busy (f3): queue_state already tore down empty placeholder.
    busySeedRetry.stop()
    flushPendingAcpFrames()
  }
}

/** Re-seed after WS reconnect / snapshot — same depth as hard refresh (g4.2). */
async function reseedAfterWsReconnect(runId: string) {
  if (!active.value || active.value.runId !== runId) return
  if (!activeRun.value) return
  await projectClarifySessionAfterLoad(activeRun.value)
}

function connectActiveRunWs(runId: string, opts?: { fromReconnect?: boolean }) {
  if (!runId) {
    closeActiveRunWs()
    return
  }
  if (activeRunWs && activeRunWsRunId === runId && !opts?.fromReconnect) return
  // Soft close for reconnect: keep buffers; only mark intentional when switching away.
  if (opts?.fromReconnect) {
    activeRunWsReconnect.markIntentionalClose()
    activeRunWs?.close()
    activeRunWs = undefined
    activeRunWsRunId = ''
  } else {
    closeActiveRunWs()
  }
  activeRunWsRunId = runId
  let socket: WebSocket
  try {
    socket = new WebSocket(api.runEventsWsUrl(runId))
    activeRunWs = socket
  } catch {
    activeRunWs = undefined
    activeRunWsRunId = ''
    activeRunWsReconnect.onClose()
    return
  }
  socket.onopen = () => {
    if (activeRunWs !== socket) return
    activeRunWsReconnect.markOpened()
    if (opts?.fromReconnect) {
      void reseedAfterWsReconnect(runId)
    }
  }
  socket.onmessage = (ev) => {
    if (activeRunWs !== socket) return
    let m: {
      type?: string
      event?: string
      nodeId?: string
      events?: any[]
      busy?: boolean
      run?: { reactSessions?: Run['reactSessions'] }
      previewArtifact?: string
      name?: string
    }
    try {
      m = JSON.parse(String(ev.data))
    } catch {
      return
    }
    // WS connect snapshot may carry reactSessions before BroadcastReviewSessions.
    if (m.type === 'snapshot') {
      const sessions = m.run?.reactSessions
      const dialogueActive =
        active.value &&
        (active.value.type === 'clarify' || active.value.type === 'gate') &&
        active.value.runId === runId
      if (sessions && dialogueActive) {
        if (activeRun.value) {
          activeRun.value = { ...activeRun.value, reactSessions: sessions }
          // Same depth as hard refresh: queue_state → seed → flush → live (g4.2).
          void projectClarifySessionAfterLoad(activeRun.value)
        } else {
          pendingSnapshotSessions = sessions
        }
      }
      return
    }
    if (m.type === 'review') {
      if (m.event === 'turn_begin') clarifyLiveBusy.value = true
      if (m.event === 'queue_state' && typeof m.busy === 'boolean') {
        clarifyLiveBusy.value = !!m.busy
        if (!m.busy) busySeedRetry.stop()
      }
      applyOrBufferReviewFrame(m as Record<string, unknown>)
      if (m.event === 'turn_done' || m.event === 'error') {
        clarifyLiveBusy.value = false
        busySeedRetry.stop()
        if (active.value && active.value.runId === runId) void softRefreshActiveRun()
      }
      return
    }
    if (m.type === 'acp') {
      if (typeof m.busy === 'boolean') {
        clarifyLiveBusy.value = !!m.busy
        if (!m.busy) busySeedRetry.stop()
      }
      applyOrBufferAcpFrame(m as { nodeId?: string; events?: AcpEvent[]; busy?: boolean })
      return
    }
    if (m.type === 'artifact_edit' || m.type === 'react' || m.type === 'trace' || m.type === 'status') {
      // Ignore events for runs that no longer match the current pending active item.
      if (!active.value || active.value.runId !== runId) return
      if (isProcessedTriple(active.value) || !isActiveStillInList()) return
      // status/trace are noisy (run transitions / appendTrace) and must not softRefresh
      // while reviewing — peek/list cover "left pending" awareness instead.
      if (m.type === 'status' || m.type === 'trace') return
      // Visual gates: only artifact_edit here; review turn_done/error already refresh above.
      // Ignore react (e.g. turn_begin) — no product change yet.
      if (active.value.type === 'gate') {
        if (m.type === 'artifact_edit') void softRefreshActiveRun()
        return
      }
      // Clarify: react/artifact_edit mid-turn projected via review/acp — skip
      // softRefresh while busy so we do not wipe busy/queue/stream (g3.2 / review v4).
      // Soft-refresh semantics unchanged (g2.3).
      if (active.value.type === 'clarify') {
        if (m.type === 'artifact_edit' || m.type === 'react') {
          // Gate on live WS busy / sessionBusy — not stale reactSessions snapshot.
          if (!isClarifySoftRefreshBlocked()) void softRefreshActiveRun()
          else if (m.type === 'artifact_edit') void patchActiveRunArtifacts(m)
        }
      }
    }
  }
  socket.onclose = () => {
    // Ignore stale close from a superseded socket (reconnect race).
    if (activeRunWs !== socket) return
    activeRunWs = undefined
    activeRunWsRunId = ''
    activeRunWsReconnect.onClose()
  }
}

async function patchActiveRunArtifacts(frame?: { previewArtifact?: string; name?: string }) {
  if (!active.value || !activeRun.value) return
  const name = String(frame?.previewArtifact || '').trim()
  if (name) {
    activeRun.value = applyPreviewArtifactName(activeRun.value, active.value.nodeId, name)
  }
  try {
    const arts = await api.runArtifacts(active.value.runId)
    if (!active.value || !activeRun.value) return
    activeRun.value = { ...activeRun.value, artifacts: Array.isArray(arts) ? arts : activeRun.value.artifacts }
  } catch {
    /* keep last known artifacts */
  }
}

async function softRefreshActiveRun() {
  if (!shouldFetchActiveInboxContext()) return
  const target = active.value!
  const triple = inboxTripleKey(target)
  const signal = acquireInboxContextSignal(triple)
  if (!signal) return
  try {
    const ctx = await api.inboxContext(target.runId, target.nodeId, target.iteration ?? 1, { signal })
    // Drop stale responses if active moved while in flight.
    if (!active.value || inboxTripleKey(active.value) !== triple) return
    if (isProcessedTriple(target)) return
    activeRun.value = adaptInboxContextToRun(ctx, target.runId)
    activeRunLoadError.value = false
    // Soft refresh keeps chat mounted — only merge reactSessions onto run; do not
    // re-project live bubbles (would reset stream). Hard-load path restores below.
  } catch (e) {
    if (isAbortError(e) || signal.aborted) return
    if (
      isInboxLeftPendingError(e) ||
      isProcessedTriple(target) ||
      !listItems.value.some((it) => itemKey(it) === itemKey(target))
    ) {
      handleLeftInboxContext(target)
      return
    }
    /* keep last known run on transient errors */
  } finally {
    releaseInboxContextSignal(triple, signal)
  }
}

async function loadActiveRun(hard = true) {
  if (!active.value) {
    activeRun.value = null
    activeRunLoadError.value = false
    closeActiveRunWs()
    return
  }
  if (!shouldFetchActiveInboxContext()) {
    if (hard) {
      activeRun.value = null
      activeRunLoadError.value = false
      activeRunLoading.value = false
    }
    return
  }
  const target = active.value
  const triple = inboxTripleKey(target)
  const signal = acquireInboxContextSignal(triple)
  if (!signal) return
  if (hard) {
    activeRun.value = null
    activeRunLoadError.value = false
    activeRunLoading.value = true
    // Chat unmounts — clear stale buffer for this load cycle (WS may refill).
    pendingReviewFrames = []
    pendingAcpFrames.clear()
  }
  let projectAfterLoad = false
  try {
    const ctx = await api.inboxContext(target.runId, target.nodeId, target.iteration ?? 1, { signal })
    if (!active.value || inboxTripleKey(active.value) !== triple) return
    if (isProcessedTriple(target)) return
    activeRun.value = adaptInboxContextToRun(ctx, target.runId)
    activeRunLoadError.value = false
    // Must project AFTER loading flag clears so ReviewComposer / GateApproval can mount.
    projectAfterLoad = hard && (target.type === 'clarify' || target.type === 'gate')
  } catch (e) {
    if (isAbortError(e) || signal.aborted) return
    if (
      isInboxLeftPendingError(e) ||
      isProcessedTriple(target) ||
      !listItems.value.some((it) => itemKey(it) === itemKey(target))
    ) {
      handleLeftInboxContext(target)
      return
    }
    if (hard) {
      activeRun.value = null
      activeRunLoadError.value = true
    }
  } finally {
    releaseInboxContextSignal(triple, signal)
    if (hard) activeRunLoading.value = false
  }
  if (projectAfterLoad && activeRun.value) {
    await projectClarifySessionAfterLoad(activeRun.value)
  }
}
watch(
  active,
  (item) => {
    if (!item) {
      closeActiveRunWs()
      activeRun.value = null
      activeRunLoadError.value = false
      return
    }
    if (isProcessedTriple(item) || !isActiveStillInList(item)) {
      closeActiveRunWs()
      return
    }
    void loadActiveRun(true)
    connectActiveRunWs(item.runId)
  },
  { immediate: true },
)

function retryActiveRun() {
  return loadActiveRun(true)
}

const activeClarify = computed(() => {
  if (active.value?.type !== 'clarify' || !activeRun.value) return null
  return pickInboxClarifySession(activeRun.value, active.value.nodeId)
})

/** Inbox app_preview waiting: prefer API kind, fall back to loaded graph node type. */
const inboxAppPreviewActive = computed(() => {
  if (active.value?.type !== 'clarify') return false
  if (active.value.kind === 'app_preview') return true
  const n = activeRun.value?.nodes?.find((node) => node.id === active.value!.nodeId)
  return n?.type === 'app_preview'
})

const inboxRemoteKind = computed(() =>
  inboxStageRemoteKind({
    appPreview: inboxAppPreviewActive.value,
    run: activeRun.value,
    nodeId: active.value?.type === 'clarify' ? active.value.nodeId : '',
  }),
)

const inboxClarifyStageKind = computed(() => (activeRunLoadError.value ? 'loadFailed' : 'pending'))

// Mirror RunDetailView.reviewActive: post-run product review on a non-react
// producer (backend only seeds clarify sessions for ReviewCapable nodes).
// Inbox API type stays "clarify"; mode is decided from the loaded graph.
const inboxReviewState = computed(() =>
  resolveInboxReviewState(active.value, activeRun.value, activeClarify.value),
)
const reviewActive = computed(() => inboxReviewState.value.reviewActive)
const composerMode = computed<'clarify' | 'review'>(() => inboxComposerMode(reviewActive.value))

watch(
  () => ({
    missing: inboxReviewState.value.nodeMissing,
    nodeId: active.value?.type === 'clarify' ? active.value.nodeId : '',
  }),
  ({ missing, nodeId }) => {
    if (missing && nodeId) {
      console.warn(
        '[GatesInboxView] inbox review node missing from run graph; falling back to clarify mode',
        nodeId,
      )
    }
  },
)

// The inbox-list gate item lacks the engine-derived ReAct fields (built in the
// store layer). Overlay them from the freshly loaded run's gate DTO so the
// ReAct reject entry appears when the upstream session is alive.
const activeGate = computed<Gate | null>(() => {
  if (active.value?.type !== 'gate') return null
  const g = active.value
  const rg = activeRun.value?.gate
  if (rg && rg.nodeId === g.nodeId) {
    return { ...g, reactUpstreamNodeId: rg.reactUpstreamNodeId, reactSessionAlive: rg.reactSessionAlive }
  }
  return g
})

const clarifyInputActive = computed(() =>
  activeRun.value ? ['queued', 'running', 'waiting_human'].includes(activeRun.value.status) : false,
)

async function applyListUpdate() {
  if (processingLock.value) return
  const prevKey = active.value ? itemKey(active.value) : null
  applyPending()
  await loadList()
  syncActiveAfterApply(listItems.value, prevKey)
  await loadActiveRun(false)
  await softRefreshActiveRun()
  showUpdateBanner.value = false
}

async function onManualRefresh() {
  if (processingLock.value) return
  manualRefreshing.value = true
  try {
    if (hasPendingUpdate.value) {
      await applyListUpdate()
    } else {
      const prevKey = active.value ? itemKey(active.value) : null
      await refresh({ source: 'manual', mode: 'force' })
      await loadList({ showLoading: listItems.value.length === 0 })
      syncActiveAfterApply(listItems.value, prevKey)
      await loadActiveRun(isEditing.value ? false : true)
      checkProcessedWhileEditing()
    }
  } finally {
    manualRefreshing.value = false
  }
}

function dismissUpdateBanner() {
  showUpdateBanner.value = false
}

// ReAct reject-and-revise edited the upstream producer in place; the gate stays
// pending, so just reload the active run to show the refreshed gate body.
async function onReactRevised() {
  await loadActiveRun(false)
}
async function onResolve(action: string, form: Record<string, any> = {}) {
  const g = active.value
  if (!g || g.type !== 'gate') return
  if (processingLock.value) return
  // Intent moment: short-circuit before await resume so WS cannot race.
  beginProcessingIntent(g)
  showProcessedBanner.value = false
  try {
    await api.resumeGate(g.runId, g.nodeId, action, form)
  } catch {
    rollbackProcessingIntent(g)
    return
  }
  const submittedKey = itemKey(g)
  const prevList = listItems.value.slice()
  try {
    await refresh({ source: 'submit', mode: 'force' })
    await loadList()
  } catch {
    /* refresh/loadList already soft-fail in most paths; still converge below */
  } finally {
    // Gate left pending even if list lag still returns the row — drop ghost and pick neighbor.
    // Always unlock even when refresh/loadList throws so the inbox cannot soft-lock.
    removeListItemLocally(submittedKey)
    selectActiveAfterRemove(prevList, submittedKey)
    // watch(active) owns the single loadActiveRun entry; avoid a second fetch here.
    if (!active.value) {
      activeRun.value = null
      activeRunLoadError.value = false
    }
    endProcessingIntent()
  }
}

const sharePanelOpen = ref(false)
const shareTarget = ref<InboxItem | null>(null)

function patchShareStatus(it: InboxItem, status: GateShareInboxStatus) {
  const next = { ...it, shareLink: status } as InboxItem
  listItems.value = listItems.value.map((row) => (itemKey(row) === itemKey(it) ? next : row))
  if (active.value && itemKey(active.value) === itemKey(it)) {
    active.value = next
  }
  shareTarget.value = next
}

function openSharePanel(it: InboxItem, alsoOpenDetail = false) {
  if (processingLock.value || !isShareableInboxItem(it)) return
  selectItem(it)
  shareTarget.value = it
  sharePanelOpen.value = true
  if (alsoOpenDetail) openDetail(it)
}

function selectItem(it: InboxItem) {
  if (processingLock.value) return
  showProcessedBanner.value = false
  clarifyConfirmError.value = null
  active.value = it
}

function openDetail(it: InboxItem) {
  if (processingLock.value) return
  if (listEl.value) listScrollTop.value = listEl.value.scrollTop
  selectItem(it)
  mobileView.value = 'detail'
}

function backToList() {
  mobileView.value = 'list'
  nextTick(() => {
    if (listEl.value) listEl.value.scrollTop = listScrollTop.value
  })
}

async function onClarifySend(
  text: string,
  images: import('@/lib/shared/types').ClarifyImage[] = [],
  annotations: import('@/lib/shared/types').ReactAnnotation[] = [],
  force = false,
) {
  const it = active.value
  if (!it || it.type !== 'clarify' || it.done) return
  if (force && processingLock.value) return
  // Force-finish leaves pending — short-circuit at intent moment (symmetric with gate).
  // Ordinary turn replies stay pending and must not markProcessed.
  if (force) {
    beginProcessingIntent(it)
  }
  const mergedAnnotations =
    inboxAppPreviewActive.value && !force ? mergeStagedAppPreviewPick(annotations) : annotations
  clarifyConfirmError.value = null
  let ok = true
  try {
    await api.reactReply(it.runId, it.nodeId, text, images, force, mergedAnnotations)
    if (!force) lastStagedAppPreviewPick.value = null
  } catch (e: any) {
    ok = false
    /* refresh below to reflect real state */
    console.warn('reactReply failed', e?.message || e)
    const msg = e?.message || t('pages.gatesInbox.reviewFinishFailed')
    if (force) {
      // Align with RunDetail: bottom status bar, not toast.
      clarifyConfirmError.value = msg
      rollbackProcessingIntent(it)
      return
    }
    // Non-force enqueue failed: roll back optimistic queue + surface error.
    reviewChatRef.value?.discardLastQueued?.()
    clarifyConfirmError.value = msg
  }
  const finished = force && ok

  // Force-finish: mirror onResolve — local converge + unlock in finally even if refresh throws.
  if (force) {
    const submittedKey = itemKey(it)
    const prevList = listItems.value.slice()
    let stillThere = true
    try {
      showProcessedBanner.value = false
      await refresh({ source: 'submit', mode: 'force' })
      await loadList()
      stillThere = listItems.value.some((k) => itemKey(k) === submittedKey)
    } catch {
      /* refresh/loadList already soft-fail in most paths; still converge below */
    } finally {
      removeListItemLocally(submittedKey)
      selectActiveAfterRemove(prevList, submittedKey)
      if (!active.value) {
        activeRun.value = null
        activeRunLoadError.value = false
      }
      endProcessingIntent()
      if (finished) {
        clarifyConfirmError.value = null
        toast.success(
          stillThere
            ? t('pages.gatesInbox.reviewFinishedPending')
            : t('pages.gatesInbox.reviewFinished'),
        )
      }
    }
    return
  }

  // Ordinary turn enqueue returns before the turn finishes — keep live
  // queue/stream bubbles (clarify + review shared session UX). No force refresh.
}

// Review: confirm product & advance (same contract as RunDetailView.onClarifyFinish).
// Classic clarify hides finish via ReviewComposer, so this only fires in review mode.
function onClarifyFinish() {
  const prompt = reviewActive.value
    ? t('pages.clarify.confirmFlowPrompt')
    : t('pages.runDetail.clarifyFinishPrompt')
  onClarifySend(prompt, [], [], true)
}

async function onClarifyCancel() {
  const it = active.value
  if (!it || it.type !== 'clarify' || it.done) return
  try {
    await api.reactCancel(it.runId, it.nodeId)
  } catch (e: any) {
    console.warn('reactCancel failed', e?.message || e)
  }
}


function onFocus() {
  peek({ source: 'focus' })
}

function onVisible() {
  if (document.visibilityState === 'visible') peek({ source: 'visibility' })
}

onMounted(() => {
  hydrateProject()
  loadList({ showLoading: true }).then(() => {
    if (!active.value) active.value = listItems.value[0] || null
  })
  peek({ source: 'mount' })
  document.addEventListener('visibilitychange', onVisible)
  window.addEventListener('focus', onFocus)
})
onUnmounted(() => {
  document.removeEventListener('visibilitychange', onVisible)
  window.removeEventListener('focus', onFocus)
  closeActiveRunWs()
  for (const triple of [...inboxContextAborts.keys()]) abortInboxContext(triple)
})

function itemTitle(it: InboxItem) {
  return it.type === 'gate' ? it.title : it.label
}

function itemSecondary(it: InboxItem) {
  return inboxSecondaryLine(it)
}
</script>

<template>
  <div
    class="flex h-full min-h-0 flex-col"
    data-testid="gates-inbox-view"
    :aria-busy="listPanelBusy ? 'true' : 'false'"
  >
    <!-- Mobile detail header -->
    <div v-if="isMobile && mobileView === 'detail'" class="mb-3 flex shrink-0 items-center gap-2">
      <button
        class="flex h-11 w-11 shrink-0 items-center justify-center rounded-md text-txt2 hover:bg-elevated hover:text-txt"
        :aria-label="t('shell.aria.backToList')"
        @click="backToList"
      >
        <Icon name="arrow-left" :size="18" />
      </button>
      <div class="min-w-0 flex-1">
        <h2 class="truncate text-base font-semibold text-txt">{{ active ? itemTitle(active) : '' }}</h2>
        <p class="truncate text-[11px] text-txt3" :title="active ? itemSecondary(active) : undefined">
          {{ active ? itemSecondary(active) : '' }}
        </p>
      </div>
    </div>

    <!-- Desktop / mobile list header -->
    <div v-else class="mb-5 flex shrink-0 flex-col gap-2.5 md:flex-row md:items-start md:justify-between">
      <div class="min-w-0">
        <h2 class="text-lg font-semibold text-txt">{{ t('pages.gatesInbox.title') }}</h2>
        <p class="text-sm text-txt3" v-html="t('pages.gatesInbox.subtitleHtml')" />
      </div>
      <div class="flex w-full shrink-0 flex-col gap-2 sm:flex-row sm:flex-wrap sm:items-center md:w-auto">
        <TagFilter
          v-model="selectedTags"
          v-model:open="tagFilterOpen"
          :project-id="selectedProject"
        />
        <div class="flex flex-wrap items-center gap-2">
          <span
            class="inline-flex items-center gap-1.5 rounded border px-2.5 py-1 text-[11px] font-medium"
            :class="{
              'border-info/40 bg-info/10 text-info': statusPillClass === 'pending',
              'border-accent/40 bg-accent-dim/50 text-accent-2': statusPillClass === 'editing',
              'border-ok/35 bg-ok/10 text-ok': statusPillClass === 'idle',
            }"
          >
            <span
              class="h-1.5 w-1.5 rounded-full"
              :class="{
                'bg-info animate-pulse': statusPillClass === 'pending',
                'bg-accent': statusPillClass === 'editing',
                'bg-ok': statusPillClass === 'idle',
              }"
            />
            {{ statusPillText }}
          </span>
          <button
            class="inline-flex items-center gap-1.5 rounded-md border border-line bg-surface px-2.5 py-1.5 text-xs font-medium text-txt transition hover:border-line-strong hover:bg-elevated disabled:opacity-45"
            :disabled="manualRefreshing || processingLock"
            :aria-busy="processingLock || undefined"
            @click="onManualRefresh"
          >
            <Icon name="refresh" :size="14" :class="{ 'animate-spin': manualRefreshing }" />
            {{ t('common.buttons.refresh') }}
          </button>
          <ProjectFilter
            v-model="selectedProject"
            v-model:open="projectFilterOpen"
            :count="listTotal"
          />
          <PipelineFilter
            v-model="selected"
            v-model:open="pipelineFilterOpen"
            :count="listTotal"
          />
        </div>
      </div>
    </div>

    <!-- f6: processed while editing -->
    <div
      v-if="showProcessedBanner && isEditing"
      class="mb-3 flex shrink-0 items-center gap-2.5 rounded-md border border-warn/35 bg-warn/10 px-3.5 py-2.5 text-sm text-warn"
    >
      <Icon name="alert" :size="18" class="shrink-0" />
      <span v-html="t('pages.gatesInbox.processedBanner')" />
    </div>

    <!-- f2: pending update banner -->
    <div
      v-if="showUpdateBanner && hasPendingUpdate"
      class="mb-3 flex shrink-0 flex-wrap items-center justify-between gap-3 rounded-md border border-info/35 bg-info/10 px-3.5 py-2.5 text-sm animate-[slideDown_0.25s_ease]"
    >
      <div class="flex min-w-0 flex-1 items-center gap-2.5 text-info">
        <Icon name="bell" :size="18" class="shrink-0" />
        <div class="min-w-0">
          <strong class="font-semibold text-txt">{{ t('pages.gatesInbox.updateBannerTitle') }}</strong>
          <span class="text-txt2">{{ updateBannerDetail }}</span>
        </div>
      </div>
      <div class="flex shrink-0 gap-2">
        <button
          class="rounded-md bg-accent px-3 py-1.5 text-xs font-medium text-white transition hover:bg-accent-2 disabled:opacity-45"
          :disabled="processingLock"
          @click="applyListUpdate()"
        >
          {{ t('common.buttons.applyRefresh') }}
        </button>
        <button
          class="rounded-md border border-line bg-surface px-3 py-1.5 text-xs font-medium text-txt transition hover:bg-elevated"
          @click="dismissUpdateBanner"
        >
          {{ t('common.buttons.later') }}
        </button>
      </div>
    </div>

    <!-- Mobile list view -->
    <template v-if="isMobile && mobileView === 'list'">
      <RefreshStrip v-if="showListRefresh" data-testid="gates-inbox-refresh-strip" />
      <div
        v-if="showListSkeleton"
        class="flex min-h-0 flex-1 flex-col gap-2"
        data-testid="gates-inbox-list-skeleton"
        aria-hidden="true"
      >
        <div
          v-for="n in SKELETON_CARDS"
          :key="'skel-m-' + n"
          class="flex w-full shrink-0 flex-col gap-2 border border-line bg-surface p-3"
        >
          <div class="flex items-start gap-3">
            <div class="h-9 w-9 shrink-0 bg-elevated animate-pulse" />
            <div class="min-w-0 flex-1 space-y-2">
              <div class="h-3.5 w-2/3 bg-elevated animate-pulse" />
              <div class="h-2.5 w-full bg-elevated animate-pulse" />
            </div>
          </div>
        </div>
      </div>
      <div
        v-else-if="showListError"
        class="card flex min-h-0 flex-1 flex-col items-stretch justify-center overflow-auto p-4"
        data-testid="gates-inbox-list-error"
      >
        <AppInlineError
          :title="t('common.asyncState.loadFailedTitle')"
          :message="listLoadError ?? undefined"
          @retry="retryListLoad"
        />
      </div>
      <div
        v-else-if="listItems.length"
        class="flex min-h-0 flex-1 flex-col"
        :class="showListRefresh ? 'opacity-[0.55]' : ''"
      >
        <div ref="listEl" class="scroll-area flex min-h-0 flex-1 flex-col gap-2 overflow-y-auto">
        <InboxPendingCard
          v-for="it in listItems"
          :key="itemKey(it)"
          :item="it"
          :active="isActive(it)"
          :disabled="processingLock"
          show-chevron
          @select="openDetail(it)"
          @open-share="openSharePanel(it, true)"
        />
        </div>
        <Pagination v-if="listTotal > PAGE_SIZE" v-model:page="listPage" :page-size="PAGE_SIZE" :total="listTotal" />
      </div>
      <div v-else class="card flex min-h-0 flex-1 flex-col items-center justify-center overflow-auto">
        <EmptyState
          icon="gate"
          :title="listTotal ? t('common.empty.noPendingGatesForPipeline') : t('common.empty.noPendingGates')"
          :desc="
            listTotal
              ? t('common.empty.noPendingGatesPipelineDesc')
              : t('common.empty.noPendingGatesDesc')
          "
        />
      </div>
    </template>

    <!-- Mobile detail view -->
    <div v-else-if="isMobile && mobileView === 'detail' && active" class="card flex min-h-0 flex-1 flex-col overflow-hidden">
      <div class="flex shrink-0 items-center justify-end gap-3 border-b border-line px-4 py-2">
        <button
          v-if="isShareableInboxItem(active)"
          type="button"
          class="inline-flex min-h-11 items-center text-xs text-accent-2 hover:underline"
          data-testid="gate-share-copy-btn-detail"
          :aria-label="t('pages.gatesInbox.share.copyLinkAria')"
          @click="openSharePanel(active)"
        >
          {{ t('pages.gatesInbox.share.copyLink') }}
        </button>
        <button class="text-xs text-accent-2 hover:underline" @click="router.push('/runs/' + active.runId)">
          {{ t('common.buttons.openRunDetail') }}
        </button>
      </div>
      <div class="flex min-h-0 flex-1 flex-col">
        <ArtifactLoadingPane v-if="activeRunLoading" message-key="pages.gatesInbox.loadingRun" />
            <GateApproval
          v-else-if="active.type === 'gate'"
          ref="gateApprovalRef"
          :key="active.runId + active.nodeId"
          :gate="activeGate!"
          :run="activeRun || undefined"
          :fill-preview="true"
          :share-link="isHumanGateInboxItem(active) ? active.shareLink ?? { state: 'none' } : null"
          @resolve="onResolve"
          @react-revised="onReactRevised"
          @open-share="openSharePanel(active)"
        />
        <ReviewShell
          v-else-if="active.type === 'clarify' && activeClarify"
          :key="active.runId + active.nodeId"
          class="min-h-0 flex-1"
          mobile
          :sidebar-width="400"
          :storage-key="REVIEW_SHELL_WIDTH_KEY_APPROVAL"
        >
          <template #stage>
            <ReactArtifactStage
              :artifacts="activeRun?.artifacts || []"
              :preview-artifact="activeClarify?.previewArtifact"
              :run-id="active.runId"
              :run="activeRun || undefined"
              :node-id="active.nodeId"
              :annotatable="clarifyInputActive"
              :remote-kind="inboxRemoteKind"
              :share-enabled="inboxAppPreviewActive"
              @pick="onAppPreviewReviewPick"
              @staged-pick="onAppPreviewStagedPick"
              @open-share="openSharePanel(active)"
            />
          </template>
          <template #sidebar>
            <ReviewComposer
              ref="reviewChatRef"
              :mode="composerMode"
              :run-id="active.runId"
              :node-id="activeClarify.nodeId"
              :iteration="activeClarify.iteration ?? 1"
              v-model:draft="clarifyDraft"
              v-model:attachments="clarifyAttachments"
              v-model:annotations="clarifyAnnotations"
              :turns="activeClarify.turns"
              :done="activeClarify.done"
              :active="clarifyInputActive"
              :confirm-error="clarifyConfirmError"
              @send="onClarifySend"
              @finish="onClarifyFinish"
              @cancel="onClarifyCancel"
            />
          </template>
        </ReviewShell>
        <ClarifyProductStage
          v-else-if="active.type === 'clarify'"
          :product-nodes="[]"
          :selected-product-id="null"
          :stage-kind="inboxClarifyStageKind"
          :selected-node="null"
          :selected-node-run="null"
          :run="null"
          :loading="activeRunLoading"
          @retry="retryActiveRun"
        />
      </div>
    </div>

    <!-- Desktop three-zone: list | product stage + review sidebar (via GateApproval/ReviewShell).
         items-stretch so detail card + review sidebar fill remaining viewport height (no page void under card). -->
    <div
      v-else-if="!isMobile && listItems.length"
      class="grid min-h-0 flex-1 grid-cols-[320px_1fr] items-stretch gap-4"
      :class="showListRefresh ? 'opacity-[0.55]' : ''"
    >
      <div class="flex h-full min-h-0 flex-col overflow-hidden">
        <RefreshStrip v-if="showListRefresh" data-testid="gates-inbox-refresh-strip" />
        <div class="scroll-area flex min-h-0 flex-1 flex-col gap-2 overflow-y-auto">
          <InboxPendingCard
            v-for="it in listItems"
            :key="itemKey(it)"
            :item="it"
            :active="isActive(it)"
            :disabled="processingLock"
            @select="selectItem(it)"
            @open-share="openSharePanel(it)"
          />
        </div>
        <Pagination v-if="listTotal > PAGE_SIZE" v-model:page="listPage" :page-size="PAGE_SIZE" :total="listTotal" />
      </div>

      <div v-if="active" class="flex h-full min-h-0 min-w-0 flex-col">
        <div class="card flex h-full min-h-0 w-full flex-col overflow-hidden">
          <div class="flex shrink-0 items-center justify-between border-b border-line px-4 py-2.5">
            <span class="text-xs text-txt3">Run #{{ active.runId.replace('run-', '') }} · {{ active.nodeId }}</span>
            <button class="text-xs text-accent-2 hover:underline" @click="router.push('/runs/' + active.runId)">
              {{ t('common.buttons.openRunDetail') }}
            </button>
          </div>
          <div class="flex min-h-0 flex-1 flex-col overflow-hidden">
            <ArtifactLoadingPane v-if="activeRunLoading" message-key="pages.gatesInbox.loadingRun" />
            <GateApproval
              v-else-if="active.type === 'gate'"
              ref="gateApprovalRef"
              :key="active.runId + active.nodeId"
              :gate="activeGate!"
              :run="activeRun || undefined"
              :fill-preview="true"
              :unified-preview-budget="true"
              class="min-h-0 flex-1"
              :share-link="isHumanGateInboxItem(active) ? active.shareLink ?? { state: 'none' } : null"
              @resolve="onResolve"
              @react-revised="onReactRevised"
              @open-share="openSharePanel(active)"
            />
            <ReviewShell
              v-else-if="active.type === 'clarify' && activeClarify"
              :key="active.runId + active.nodeId"
              class="min-h-0 flex-1"
              :sidebar-width="400"
              :storage-key="REVIEW_SHELL_WIDTH_KEY_APPROVAL"
            >
              <template #stage>
                <ReactArtifactStage
                  :artifacts="activeRun?.artifacts || []"
                  :preview-artifact="activeClarify?.previewArtifact"
                  :run-id="active.runId"
                  :run="activeRun || undefined"
                  :node-id="active.nodeId"
                  :annotatable="clarifyInputActive"
                  :remote-kind="inboxRemoteKind"
                  :share-enabled="inboxAppPreviewActive"
                  @pick="onAppPreviewReviewPick"
                  @staged-pick="onAppPreviewStagedPick"
                  @open-share="openSharePanel(active)"
                />
              </template>
              <template #sidebar>
                <ReviewComposer
                  ref="reviewChatRef"
                  :mode="composerMode"
                  :run-id="active.runId"
                  :node-id="activeClarify.nodeId"
                  :iteration="activeClarify.iteration ?? 1"
                  v-model:draft="clarifyDraft"
                  v-model:attachments="clarifyAttachments"
                  v-model:annotations="clarifyAnnotations"
                  :turns="activeClarify.turns"
                  :done="activeClarify.done"
                  :active="clarifyInputActive"
                  :confirm-error="clarifyConfirmError"
                  @send="onClarifySend"
                  @finish="onClarifyFinish"
                  @cancel="onClarifyCancel"
                />
              </template>
            </ReviewShell>
            <ClarifyProductStage
              v-else-if="active.type === 'clarify'"
              :product-nodes="[]"
              :selected-product-id="null"
              :stage-kind="inboxClarifyStageKind"
              :selected-node="null"
              :selected-node-run="null"
              :run="null"
              :loading="activeRunLoading"
              @retry="retryActiveRun"
            />
          </div>
        </div>
      </div>
    </div>

    <div
      v-else-if="!isMobile && showListSkeleton"
      class="flex min-h-0 flex-1 flex-col gap-2"
      data-testid="gates-inbox-list-skeleton"
      aria-hidden="true"
    >
      <div
        v-for="n in SKELETON_CARDS"
        :key="'skel-d-' + n"
        class="flex w-full max-w-[320px] shrink-0 flex-col gap-2 border border-line bg-surface p-3"
      >
        <div class="flex items-start gap-3">
          <div class="h-9 w-9 shrink-0 bg-elevated animate-pulse" />
          <div class="min-w-0 flex-1 space-y-2">
            <div class="h-3.5 w-2/3 bg-elevated animate-pulse" />
            <div class="h-2.5 w-full bg-elevated animate-pulse" />
          </div>
        </div>
      </div>
    </div>

    <div
      v-else-if="!isMobile && showListError"
      class="card flex min-h-0 flex-1 flex-col items-stretch justify-center overflow-auto p-4"
      data-testid="gates-inbox-list-error"
    >
      <AppInlineError
        :title="t('common.asyncState.loadFailedTitle')"
        :message="listLoadError ?? undefined"
        @retry="retryListLoad"
      />
    </div>

    <div v-else-if="!isMobile" class="card flex min-h-0 flex-1 flex-col items-center justify-center overflow-auto">
      <EmptyState
        icon="gate"
        :title="listTotal ? t('common.empty.noPendingGatesForPipeline') : t('common.empty.noPendingGates')"
        :desc="
          listTotal
            ? t('common.empty.noPendingGatesPipelineDesc')
            : t('common.empty.noPendingGatesDesc')
        "
      />
    </div>

    <GateShareLinkPanel
      :open="sharePanelOpen"
      :target="shareTarget"
      :kind="shareTarget ? inboxShareKind(shareTarget) : 'human_gate'"
      @close="sharePanelOpen = false"
      @updated="(st) => shareTarget && patchShareStatus(shareTarget, st)"
      @revoked="
        (st) => {
          if (shareTarget) patchShareStatus(shareTarget, st)
          sharePanelOpen = false
        }
      "
    />
  </div>
</template>

<style scoped>
@keyframes slideDown {
  from {
    opacity: 0;
    transform: translateY(-6px);
  }
}
</style>
