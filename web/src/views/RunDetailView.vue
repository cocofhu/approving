<script setup lang="ts">
import { ref, reactive, computed, watch, nextTick, onMounted, onUnmounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/ui/Icon.vue'
import AppButton from '@/components/ui/AppButton.vue'
import StatusPill from '@/components/ui/StatusPill.vue'
import PriorityBadge from '@/components/ui/PriorityBadge.vue'
import PrioritySegmented, { type RunPriority } from '@/components/ui/PrioritySegmented.vue'
import TruncatedTextTooltip from '@/components/ui/TruncatedTextTooltip.vue'
import WorkflowCanvas from '@/components/canvas/WorkflowCanvas.vue'
import NodeOutputPanel from '@/components/run/NodeOutputPanel.vue'
import LiveLogPanel from '@/components/run/LiveLogPanel.vue'
import ArtifactPanel from '@/components/run/ArtifactPanel.vue'
import StateTracePanel from '@/components/run/StateTracePanel.vue'
import VariablesPanel from '@/components/run/VariablesPanel.vue'
import RunSandboxEnvPanel from '@/components/run/RunSandboxEnvPanel.vue'
import ClarifyChat from '@/components/run/ClarifyChat.vue'
import ClarifyBootLoader from '@/components/run/ClarifyBootLoader.vue'
import GateApproval from '@/components/run/GateApproval.vue'
import AppPreviewPanel from '@/components/run/AppPreviewPanel.vue'
import StructuredProductPanel from '@/components/run/StructuredProductPanel.vue'
import ReviewShell from '@/components/run/ReviewShell.vue'
import ReviewComposer from '@/components/run/ReviewComposer.vue'
import ExecutionTimeline from '@/components/run/ExecutionTimeline.vue'
import ExecutionStatsPanel from '@/components/run/ExecutionStatsPanel.vue'
import EmptyState from '@/components/ui/EmptyState.vue'
import ArtifactLoadingPane from '@/components/run/ArtifactLoadingPane.vue'
import RefreshStrip from '@/components/run/RefreshStrip.vue'
import HardLoadLayer from '@/components/run/HardLoadLayer.vue'
import AppTabs from '@/components/ui/AppTabs.vue'
import AppDrawer from '@/components/ui/AppDrawer.vue'
import AppModal from '@/components/ui/AppModal.vue'
import { api } from '@/lib/api/api'
import { useToast } from '@/lib/composables/useToast'
import {
  REVIEW_CANVAS_MIN,
  REVIEW_SIDEBAR,
  REVIEW_SHELL_WIDTH_KEY_REVIEW,
  reviewRightPanelCssWidth,
} from '@/lib/inbox/reviewLayoutBudget'
import { useBreakpoint } from '@/lib/composables/useBreakpoint'
import { addClarifyAnnotation, useClarifyDraft } from '@/lib/inbox/useClarifyDraft'
import { previewPickLabel, type AppPreviewPickPayload } from '@/lib/shared/previewPickUrl'
import { NODE_DEFS } from '@/data/nodeRegistry'
import { resolveNodeDisplayLabelFromNode } from '@/lib/run/resolveNodeDisplayLabel'
import { fmtTime, fmtDuration, formatTrigger } from '@/lib/shared/format'
import { resolveOutputFocusNodeId } from '@/lib/run/runOutputSelection'
import { pickDefaultTimelineNodeId } from '@/lib/run/runStats'
import { PRODUCT_NODE_TYPES } from '@/lib/run/productNodeArtifacts'
import { applyLiveWsAcpPage } from '@/lib/run/applyLiveWsAcpPage'
import { mergeAcpEvents, type MergedAcpEvent } from '@/lib/run/mergeAcpEvents'
import { createPendingAcpBuffer, pickAcpRails } from '@/lib/run/pendingAcpBuffer'
import { deliverOrBufferDialogueAcp } from '@/lib/run/dialogueAcpDelivery'
import {
  createBusySeedRetryController,
  runBusySeedRetry,
} from '@/lib/run/busySeedRetry'
import { createWsReconnectController } from '@/lib/run/wsReconnect'
import type { LiveLogBootSession } from '@/lib/run/liveLogBootPhase'
import {
  isAbortError,
  RehydrateOrchestrator,
  type RehydrateStatus,
} from '@/lib/run/liveLogRehydrate'
import {
  clearLiveLogSnapshotsExceptRun,
  cloneEventPageSnapshot,
  getLiveLogSnapshot,
  listLiveLogSnapshotsForRun,
  putLiveLogEventPage,
  putLiveLogMcpCalls,
} from '@/lib/run/liveLogSnapshotCache'
import type { AcpEvent, McpCall, NodeRun, NodeRunStatus, Run, Workflow } from '@/lib/shared/types'
import type { SandboxView } from '@/lib/api/api'
import { isClearlyInvalidRunRouteId } from '@/lib/pm/pmCitationShape'

type RunLoadErrorKind = 'not_found' | 'network_or_server'

const route = useRoute()
const router = useRouter()
const { t } = useI18n()
const toast = useToast()
const { isMobile } = useBreakpoint()
const runId = computed(() => route.params.id as string)

function emptyRun(id: string): Run {
  return {
    id, workflowId: '', workflowName: t('pages.runDetail.loadingWorkflow'), status: 'running', trigger: '',
    startedAt: new Date().toISOString(), durationSec: 0, progress: 0,
    nodeRuns: {}, artifacts: [], trace: [], vars: [],
  }
}

function emptyWorkflow(): Workflow {
  return { id: '', name: '', description: '', status: 'draft', version: 1, updatedAt: '', needsRepo: false, nodes: [], edges: [] }
}

const run = ref<Run>(emptyRun(runId.value))
const wf = ref<Workflow>(emptyWorkflow())
/** Project alias for 「未知/未分桶」display on Run token table. */
const unknownModelDisplayName = ref('')

async function loadUnknownModelDisplayName(projectId?: string | null) {
  unknownModelDisplayName.value = ''
  if (!projectId) return
  try {
    const p = await api.getProject(projectId)
    unknownModelDisplayName.value = p.unknownModelDisplayName || ''
  } catch {
    // Keep default label when project cannot be loaded.
  }
}
const runLoading = ref(false)
const loadError = ref(false)
const loadErrorKind = ref<RunLoadErrorKind | null>(null)
const refreshing = ref(false)

// Event log read on demand with cursor/limit pagination; older history is
// prepended via "load earlier". Live WS events append to the loaded window.
type EventPageState = {
  events: MergedAcpEvent[]
  nextCursor: string
  hasMore: boolean
  live: boolean
}
const eventPages = reactive<Record<string, EventPageState>>({})
const liveEvents = reactive<Record<string, AcpEvent[]>>({})
// Authoritative queue_state.busy per node (true while the agent is actively
// processing the turn), pushed alongside the acp events over the WebSocket.
const liveBusy = reactive<Record<string, boolean>>({})
const liveNode = ref<string | null>(null)
/** ClarifyChat / ReviewComposer surface for review WS frames (queue/stream/Cancel). */
const reviewChatRef = ref<{
  applyReviewFrame?: (frame: any) => boolean | void
  applyAcpEvents?: (events: AcpEvent[] | undefined, nodeId?: string) => boolean | void
  discardLastQueued?: () => void
  isSessionBusy?: () => boolean
  isChatReady?: () => boolean
} | null>(null)

/** Clarify/review session in-flight: skip full-page loadRun (g3.2 / review v1). */
function isClarifySessionBusy(): boolean {
  const nodeId = selClarify.value?.nodeId
  if (!nodeId) return false
  if (liveBusy[nodeId] === true) return true
  if (!!(run.value as any)?.reactSessions?.[nodeId]?.busy) return true
  if (reviewChatRef.value?.isSessionBusy?.()) return true
  return false
}
const gateApprovalRef = ref<{
  applyReviewFrame?: (frame: any) => void
  applyAcpEvents?: (events: AcpEvent[] | undefined) => boolean | void
} | null>(null)
/**
 * ACP frames that arrived while ClarifyChat / GateApproval was unmounted
 * during hard load. Flushed after queue_state rebuilds the streaming slot.
 */
const pendingDialogueAcp = createPendingAcpBuffer()
/** Rails applied via seed or live — stops busy seed retry (g3). */
let dialogueRailsFilled = false
let dialogueLiveIncremental = false
const busySeedRetry = createBusySeedRetryController()
let wsReconnectRunId = ''
const wsReconnect = createWsReconnectController({
  connect: () => connectWs({ fromReconnect: true }),
  shouldReconnect: () =>
    !!runId.value &&
    (run.value.status === 'running' || run.value.status === 'waiting_human'),
})
const manual = ref(false)
// Per-node fetch generation: discard stale REST responses so a slow empty
// reply cannot overwrite a newer non-empty write-back.
const eventFetchGen = reactive<Record<string, number>>({})

// Per-node REST rehydrate status (idle|loading|ready|error). Independent of
// Boot's 120s stage timeout — ~10s hang becomes a visible failure + retry.
// Orchestrator owns generation + AbortController so an older in-flight attempt
// cannot flip a newer loading→error (or discard a current-gen success).
const rehydrateByNode = reactive<Record<string, RehydrateStatus>>({})
const rehydrateOrchs: Record<string, RehydrateOrchestrator> = {}

function disposeAllRehydrateOrchs() {
  for (const id of Object.keys(rehydrateOrchs)) {
    rehydrateOrchs[id]?.dispose()
    delete rehydrateOrchs[id]
  }
}

function nodeNeedsRehydrate(nodeId: string): boolean {
  return run.value.nodeRuns[nodeId]?.status === 'running'
}

// Event log read on demand straight from the node's sandbox (or its persisted
// snapshot). Returns false on failure so the rehydrate state machine can surface
// errors instead of silently treating them as an empty timeline. Abort → stale
// (caller must not treat as hard failure).
async function fetchNodeEvents(
  nodeId: string | null,
  opts?: { signal?: AbortSignal },
): Promise<boolean> {
  if (!nodeId) return false
  const gen = (eventFetchGen[nodeId] = (eventFetchGen[nodeId] || 0) + 1)
  try {
    const r = await api.nodeEvents(runId.value, nodeId, {
      limit: 20,
      signal: opts?.signal,
    })
    if (opts?.signal?.aborted) return false
    if (eventFetchGen[nodeId] !== gen) return false
    const prev = eventPages[nodeId]?.events || []
    const merged = mergeAcpEvents(prev, r.events || [], { live: !!r.live })
    if ('hasMore' in r) {
      eventPages[nodeId] = {
        events: merged,
        nextCursor: r.nextCursor || '',
        hasMore: r.hasMore,
        live: !!r.live,
      }
    } else {
      eventPages[nodeId] = {
        events: merged,
        nextCursor: '',
        hasMore: false,
        live: r.live,
      }
    }
    syncEventPageToCache(nodeId)
    return true
  } catch (err) {
    if (isAbortError(err) || opts?.signal?.aborted) return false
    // Keep whatever we already have; caller decides error UI.
    return false
  }
}

function syncEventPageToCache(nodeId: string) {
  const page = eventPages[nodeId]
  if (!page) return
  putLiveLogEventPage(runId.value, nodeId, page)
}

function syncMcpCallsToCache(nodeId: string, mcpCalls: McpCall[] | undefined) {
  if (!mcpCalls?.length) return
  putLiveLogMcpCalls(runId.value, nodeId, mcpCalls)
}

/** Restore displayable event pages after hard remount within the same SPA visit. */
function restoreEventPagesFromCache(id: string) {
  for (const { nodeId, snapshot } of listLiveLogSnapshotsForRun(id)) {
    if (!snapshot.eventPage?.events.length) continue
    eventPages[nodeId] = cloneEventPageSnapshot(snapshot.eventPage)
  }
}

function ensureRehydrateOrch(nodeId: string): RehydrateOrchestrator {
  let orch = rehydrateOrchs[nodeId]
  if (!orch) {
    orch = new RehydrateOrchestrator({
      fetch: async (signal) => {
        const ok = await fetchNodeEvents(nodeId, { signal })
        if (signal.aborted) throw new DOMException('Aborted', 'AbortError')
        return ok ? 'ok' : 'error'
      },
      onStatus: (s) => {
        rehydrateByNode[nodeId] = s
      },
    })
    rehydrateOrchs[nodeId] = orch
  }
  return orch
}

/** Entering a running node: loading → ready|error; ~10s hang → error. */
async function rehydrateNodeEvents(nodeId: string | null, opts?: { force?: boolean }) {
  if (!nodeId) return
  const running = nodeNeedsRehydrate(nodeId)
  if (!running) {
    // Completed / non-running: clear rehydrate UI and still load persisted snapshot.
    if (rehydrateOrchs[nodeId]) {
      await rehydrateOrchs[nodeId].run({ running: false })
    } else {
      rehydrateByNode[nodeId] = 'idle'
    }
    await fetchNodeEvents(nodeId)
    return
  }
  await ensureRehydrateOrch(nodeId).run({ running: true, force: !!opts?.force })
}

function retryRehydrate() {
  const id = selected.value
  if (!id) return
  void rehydrateNodeEvents(id, { force: true })
}

const selRehydrateStatus = computed<RehydrateStatus>(() => {
  const id = selected.value
  if (!id) return 'idle'
  return rehydrateByNode[id] || 'idle'
})

async function loadEarlierEvents(nodeId: string) {
  const st = eventPages[nodeId]
  if (!st?.hasMore) return
  try {
    const r = await api.nodeEvents(runId.value, nodeId, { cursor: st.nextCursor, limit: 20 })
    if ('hasMore' in r && r.events?.length) {
      const older = mergeAcpEvents([], r.events, { live: false })
      st.events = [...older, ...st.events]
      st.nextCursor = r.nextCursor || ''
      st.hasMore = r.hasMore
      syncEventPageToCache(nodeId)
    }
  } catch {
    /* keep current window on failure */
  }
}

// Raw sandbox container logs (docker logs stdout/stderr): live while the node's
// container runs, then the archived snapshot captured at teardown. Kept for
// post-mortem troubleshooting (e.g. a failed git clone in startup.sh).
type SbxLogState = { content: string; live: boolean; found: boolean; error?: string }
const sbxLogs = reactive<Record<string, SbxLogState>>({})
const sandboxLookup = ref<SandboxView | null>(null)
const sbxLogLoading = ref(false)
let sandboxLogGen = 0
let sandboxLogAbort: AbortController | null = null
// Boot dwell/timeout must survive LiveLogPanel remounts (log ↔ sandbox / other tabs).
const liveLogBootSessions = reactive<Record<string, LiveLogBootSession>>({})
async function fetchSandboxLog(nodeId: string | null) {
  if (!nodeId) {
    sandboxLogAbort?.abort()
    sandboxLogAbort = null
    sbxLogLoading.value = false
    return
  }
  const attemptGen = ++sandboxLogGen
  sandboxLogAbort?.abort()
  sandboxLogAbort = new AbortController()
  sbxLogLoading.value = true
  try {
    const r = await api.nodeSandboxLog(runId.value, nodeId, { signal: sandboxLogAbort.signal })
    if (attemptGen !== sandboxLogGen) return
    // Always write the response so empty live / found=false / error map correctly
    // (do not treat empty content as "no update").
    sbxLogs[nodeId] = {
      content: r.content ?? '',
      live: !!r.live,
      found: !!r.found,
      error: r.error || undefined,
    }
  } catch (e) {
    if (attemptGen !== sandboxLogGen) return
    if (isAbortError(e)) return
    const prev = sbxLogs[nodeId]
    sbxLogs[nodeId] = {
      content: prev?.content ?? '',
      live: false,
      found: !!prev?.found,
      error: t('pages.runDetail.sandboxLog.loadFailed'),
    }
  } finally {
    if (attemptGen === sandboxLogGen) sbxLogLoading.value = false
  }
}

let timer: number | undefined
let clock: number | undefined
let ws: WebSocket | undefined
let wsConnected = false

function clearReactiveRecord(rec: Record<string, unknown>) {
  for (const k of Object.keys(rec)) delete rec[k]
}

function resetRunState(id: string) {
  // Drop other runs' snapshots; keep this run's session cache across remount.
  clearLiveLogSnapshotsExceptRun(id)
  run.value = emptyRun(id)
  wf.value = emptyWorkflow()
  selected.value = null
  manual.value = false
  liveNode.value = null
  gateError.value = null
  clarifyConfirmError.value = null
  resumeError.value = null
  disposeAllRehydrateOrchs()
  clearReactiveRecord(eventPages)
  clearReactiveRecord(liveEvents)
  clearReactiveRecord(liveBusy)
  clearReactiveRecord(eventFetchGen)
  clearReactiveRecord(sbxLogs)
  clearReactiveRecord(liveLogBootSessions as Record<string, unknown>)
  clearReactiveRecord(rehydrateByNode as Record<string, unknown>)
  sandboxLookup.value = null
  pendingDialogueAcp.clear()
  dialogueRailsFilled = false
  dialogueLiveIncremental = false
  busySeedRetry.stop()
  // Prefer cached timeline so re-entry is not blanked by loading/error UI.
  restoreEventPagesFromCache(id)
}

function teardownRealtime() {
  busySeedRetry.stop()
  wsReconnect.markIntentionalClose()
  ws?.close()
  ws = undefined
  wsConnected = false
  if (timer) {
    window.clearInterval(timer)
    timer = undefined
  }
}

function classifyRunLoadError(err: unknown): RunLoadErrorKind {
  const msg = err instanceof Error ? err.message : String(err ?? '')
  // Prefer explicit not-found over apiState.online (404 also flips online=false).
  if (/not found/i.test(msg) || /^404\b/.test(msg) || msg.includes(' 404 ')) {
    return 'not_found'
  }
  if (err instanceof TypeError || /failed to fetch/i.test(msg) || /network/i.test(msg)) {
    return 'network_or_server'
  }
  // Other non-ok HTTP / server errors remain retryable.
  return 'network_or_server'
}

async function fetchRunData(): Promise<true | RunLoadErrorKind> {
  const id = runId.value
  if (isClearlyInvalidRunRouteId(id)) {
    return 'not_found'
  }
  try {
    const r = await api.getRun(id)
    run.value = r
    // Prefer the run's own pinned graph so the canvas reflects exactly what
    // executed, even if the live workflow was since edited or deleted. Fall
    // back to fetching the workflow definition only if the run lacks a graph.
    if (r.nodes?.length) {
      wf.value = {
        id: r.workflowId, name: r.workflowName, description: '', status: 'published',
        version: r.workflowVersion || 1, updatedAt: '', needsRepo: false, nodes: r.nodes, edges: r.edges || [],
      }
      // Still resolve projectId for unknown-model display alias when graph is pinned.
      if (r.workflowId) {
        try {
          const live = await api.getWorkflow(r.workflowId)
          if (live.projectId) wf.value.projectId = live.projectId
        } catch {
          // Workflow may have been deleted; alias stays unset → default label.
        }
      }
    } else if (r.workflowId) {
      wf.value = await api.getWorkflow(r.workflowId)
    }
    await loadUnknownModelDisplayName(wf.value.projectId)
    // Refresh-resume: wait for dialogue surfaces to mount, then project busy/queue.
    void projectDialogueAfterLoad(r)
    return true
  } catch (err) {
    return classifyRunLoadError(err)
  }
}

/**
 * Align Inbox hard-load: multi nextTick so ReviewComposer/GateApproval mount
 * before queue_state → seed → flush → live (g2.1).
 */
async function projectDialogueAfterLoad(r: { reactSessions?: Record<string, any> }) {
  dialogueRailsFilled = false
  dialogueLiveIncremental = false
  await nextTick()
  await nextTick()
  restoreReactSessions(r)
}

function restoreReactSessions(r: { reactSessions?: Record<string, any> }) {
  const sessions = r.reactSessions
  if (!sessions) return
  const busyNodes: string[] = []
  for (const [nodeId, snap] of Object.entries(sessions)) {
    if (!snap || typeof snap !== 'object') continue
    liveBusy[nodeId] = !!snap.busy
    if (snap.busy) busyNodes.push(nodeId)
    const frame = {
      event: 'queue_state',
      nodeId,
      waiting: snap.waiting ?? 0,
      items: snap.items ?? [],
      busy: !!snap.busy,
      activeItem: snap.activeItem,
    }
    if (selClarify.value?.nodeId === nodeId) {
      const ok = reviewChatRef.value?.applyReviewFrame?.(frame)
      if (ok === false) {
        /* inner ClarifyChat not ready — seed path still buffers ACP */
      }
    }
    // Gate hot-revise shares the producer reactSessions key.
    if (run.value.gate?.reactUpstreamNodeId === nodeId) {
      gateApprovalRef.value?.applyReviewFrame?.(frame)
    }
  }
  if (busyNodes.length) {
    void seedDialogueAcpAfterRestore(busyNodes)
  } else {
    // Authority not busy (f3): surfaces already tore down empty placeholders.
    busySeedRetry.stop()
    void nextTick(() => flushPendingDialogueAcp())
  }
}

/**
 * Seed-then-live: after busy slot rebuild, replay eventPages / nodeEvents into
 * the dialogue surface, then flush any ACP buffered during hard-load remount.
 * Busy + empty rails → periodic retry (g3).
 */
async function seedDialogueAcpAfterRestore(nodeIds: string[]) {
  await nextTick()
  for (const nodeId of nodeIds) {
    await seedDialogueNodeOnce(nodeId)
  }
  await nextTick()
  flushPendingDialogueAcp()
  const stillEmptyBusy = nodeIds.filter(
    (nid) => liveBusy[nid] && !dialogueRailsFilled && !dialogueLiveIncremental,
  )
  if (stillEmptyBusy.length) {
    startDialogueBusySeedRetry(stillEmptyBusy)
  }
}

async function seedDialogueNodeOnce(nodeId: string): Promise<boolean> {
  let events = eventPages[nodeId]?.events as AcpEvent[] | undefined
  if (!events?.length) {
    try {
      await fetchNodeEvents(nodeId)
    } catch {
      return false
    }
    events = eventPages[nodeId]?.events as AcpEvent[] | undefined
  }
  if (!events?.length) return false
  const rails = pickAcpRails(events)
  if (!rails.thought && !rails.message) return false
  const forClarify = selClarify.value?.nodeId === nodeId
  const forGate = run.value.gate?.reactUpstreamNodeId === nodeId
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
  if (result === 'buffer') {
    pendingDialogueAcp.push({ nodeId, events, busy: true })
    return false
  }
  dialogueRailsFilled = true
  return true
}

function startDialogueBusySeedRetry(nodeIds: string[]) {
  busySeedRetry.start(async (signal) => {
    await runBusySeedRetry({
      signal,
      isBusy: () => nodeIds.some((nid) => !!liveBusy[nid]),
      hasContent: () => dialogueRailsFilled,
      liveIncrementalReceived: () => dialogueLiveIncremental,
      seed: async () => {
        let any = false
        for (const nid of nodeIds) {
          if (!liveBusy[nid]) continue
          if (await seedDialogueNodeOnce(nid)) any = true
        }
        flushPendingDialogueAcp()
        return any || dialogueRailsFilled
      },
    })
  })
}

function flushPendingDialogueAcp() {
  if (!pendingDialogueAcp.size) return
  const leftover: { nodeId?: string; events: AcpEvent[]; busy?: boolean }[] = []
  for (const frame of pendingDialogueAcp.takeAll()) {
    const nodeId = frame.nodeId || ''
    const forClarify = !!selClarify.value?.nodeId && (selClarify.value.nodeId === nodeId || !nodeId)
    const forGate =
      !!run.value.gate?.reactUpstreamNodeId &&
      (run.value.gate.reactUpstreamNodeId === nodeId || !nodeId)
    const result = deliverOrBufferDialogueAcp({
      forClarify,
      forGate,
      events: frame.events,
      nodeId: nodeId || selClarify.value?.nodeId || run.value.gate?.reactUpstreamNodeId || '',
      applyClarify: (events, nid) => {
        if (!reviewChatRef.value?.applyAcpEvents) return false
        return reviewChatRef.value.applyAcpEvents(events, nid)
      },
      applyGate: (events) => {
        if (!gateApprovalRef.value?.applyAcpEvents) return false
        return gateApprovalRef.value.applyAcpEvents(events) !== false
      },
    })
    if (result === 'applied') {
      const rails = pickAcpRails(frame.events)
      if (rails.thought || rails.message) dialogueRailsFilled = true
    }
    if (result === 'buffer') leftover.push(frame)
  }
  for (const frame of leftover) pendingDialogueAcp.push(frame)
}

/** Deliver ACP to dialogue surfaces or buffer until they remount. */
function applyOrBufferDialogueAcp(
  nodeId: string,
  events: AcpEvent[],
  busy?: boolean,
) {
  const forClarify = selClarify.value?.nodeId === nodeId
  const forGate = run.value.gate?.reactUpstreamNodeId === nodeId
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
  if (result === 'buffer') {
    pendingDialogueAcp.push({ nodeId, events, busy })
  }
}

function syncAllMcpCallsFromRun() {
  const runs = run.value.nodeRuns || {}
  for (const [nodeId, nr] of Object.entries(runs)) {
    if (nr?.mcpCalls?.length) syncMcpCallsToCache(nodeId, nr.mcpCalls)
  }
}

async function initAfterLoadSuccess() {
  applyDetailArtifactsDeepLink()
  if (!applyOutputDeepLinkFocus() && !selected.value) selected.value = defaultNode.value || null
  syncAllMcpCallsFromRun()
  // Rehydrate regardless of which tab is active (default tab stays unchanged).
  void rehydrateNodeEvents(selected.value)
  fetchSandboxLog(selected.value)
  connectWs()
  if (!timer) {
    timer = window.setInterval(() => {
      if (run.value.status === 'running' || run.value.status === 'waiting_human') {
        // Clarify/review session busy: skip full loadRun so we do not wipe
        // live stream / send lock / input focus (narrow updates via WS frames).
        if (!isClarifySessionBusy()) {
          loadRun(false)
        }
        const sel = selected.value
        // Only skip REST once we already have displayable live events;
        // busy-only / empty live frames must keep the 2s poll fallback.
        const skipPoll =
          wsConnected &&
          !!sel &&
          liveNode.value === sel &&
          !!eventPages[sel]?.live &&
          (eventPages[sel]?.events.length || 0) > 0
        if (!skipPoll && sel) {
          const rh = rehydrateByNode[sel] || 'idle'
          // Do not auto-recover from error; keep polling only after ready.
          if (rh === 'error' || rh === 'loading') {
            /* stay put */
          } else if (rh === 'ready') {
            void fetchNodeEvents(sel)
          } else {
            void rehydrateNodeEvents(sel)
          }
        }
        if (nodeTab.value === 'sandbox') fetchSandboxLog(selected.value)
        // Boot-stage empty state needs fresh sandbox row status/containerStatus.
        maybePollSandboxForBoot()
      }
    }, 2000)
  }
}

async function loadRun(hard = false) {
  const id = runId.value
  if (hard) {
    runLoading.value = true
    loadError.value = false
    loadErrorKind.value = null
    resetRunState(id)
    teardownRealtime()
  } else {
    refreshing.value = true
  }

  try {
    const result = await fetchRunData()
    if (hard) {
      if (result === true) {
        loadError.value = false
        loadErrorKind.value = null
        await initAfterLoadSuccess()
      } else {
        loadError.value = true
        loadErrorKind.value = result
      }
    }
  } finally {
    if (hard) runLoading.value = false
    else refreshing.value = false
  }
}

function connectWs(opts?: { fromReconnect?: boolean }) {
  const id = runId.value
  if (!id) return
  wsReconnectRunId = id
  if (opts?.fromReconnect) {
    wsReconnect.markIntentionalClose()
    ws?.close()
    ws = undefined
    wsConnected = false
  }
  let socket: WebSocket
  try {
    socket = new WebSocket(api.runEventsWsUrl(id))
    ws = socket
  } catch {
    wsReconnect.onClose()
    return
  }
  socket.onopen = () => {
    if (ws !== socket) return
    wsConnected = true
    wsReconnect.markOpened()
    // Reconnect: force same-depth re-seed as hard refresh (g4.2).
    if (opts?.fromReconnect && run.value.reactSessions) {
      void projectDialogueAfterLoad(run.value)
    }
  }
  socket.onclose = () => {
    if (ws !== socket) return
    wsConnected = false
    ws = undefined
    wsReconnect.onClose()
  }
  socket.onmessage = (ev) => {
    if (ws !== socket) return
    let m: any
    try {
      m = JSON.parse(ev.data)
    } catch {
      return
    }
    if (m.type === 'snapshot' && m.run?.reactSessions) {
      run.value = { ...run.value, reactSessions: m.run.reactSessions }
      // queue_state → seed → flush → live (parity with Inbox g4.2).
      void projectDialogueAfterLoad(run.value)
      return
    }
    if (m.type === 'acp' && m.nodeId) {
      const wsEvents: AcpEvent[] = m.events || []
      liveEvents[m.nodeId] = wsEvents
      // Empty busy-only frames: update busy/liveNode only — never wipe timeline
      // or sync an empty page into the session cache (f3 / soft-warn regression).
      const mergedPage = applyLiveWsAcpPage(eventPages[m.nodeId], wsEvents)
      if (mergedPage) {
        eventPages[m.nodeId] = mergedPage
        // WS only merges into the snapshot — never clears rehydrate error / soft warn.
        syncEventPageToCache(m.nodeId)
      }
      if (typeof m.busy === 'boolean') {
        liveBusy[m.nodeId] = m.busy
        if (!m.busy) busySeedRetry.stop()
      }
      liveNode.value = m.nodeId
      if (!manual.value) selected.value = m.nodeId
      // Dialogue-surface stream (ClarifyChat / GateApproval), not only LiveLog.
      applyOrBufferDialogueAcp(m.nodeId, wsEvents, m.busy)
    } else if (m.type === 'review' && m.nodeId) {
      // Prefer matching producer; components also filter by nodeId defensively.
      if (m.event === 'turn_begin') liveBusy[m.nodeId] = true
      if (m.event === 'queue_state' && typeof m.busy === 'boolean') {
        liveBusy[m.nodeId] = !!m.busy
        if (!m.busy) busySeedRetry.stop()
      }
      if (!selClarify.value || selClarify.value.nodeId === m.nodeId) {
        reviewChatRef.value?.applyReviewFrame?.(m)
      }
      gateApprovalRef.value?.applyReviewFrame?.(m)
      if (m.event === 'turn_done' || m.event === 'error') {
        liveBusy[m.nodeId] = false
        busySeedRetry.stop()
        loadRun(false)
      }
    } else if (
      m.type === 'trace' ||
      m.type === 'status' ||
      m.type === 'react' ||
      m.type === 'artifact_edit'
    ) {
      // Node finished / transitioned / human artifact edit: pull authoritative snapshot.
      // Mid-clarify: review/acp frames project the session — skip full-page rebind for
      // react/status/trace/artifact_edit alike (g3.2 / review v3).
      // Soft-refresh path unchanged (g2.3).
      if (isClarifySessionBusy()) return
      if (m.type === 'status') liveNode.value = null
      loadRun(false)
    }
  }
}

onMounted(async () => {
  await loadRun(true)
  clock = window.setInterval(() => {
    if (ACTIVE.includes(run.value.status)) nowMs.value = Date.now()
  }, 1000)
  document.addEventListener('visibilitychange', onVisible)
  window.addEventListener('focus', onFocusRefresh)
})
function onFocusRefresh() {
  // Clarify/review mid-turn: skip focus/visibility full reload (keeps stream + input focus).
  if (isClarifySessionBusy()) return
  if (!runLoading.value && !loadError.value) loadRun(false)
}
function onVisible() {
  if (document.visibilityState === 'visible') onFocusRefresh()
}
watch(
  () => route.params.id,
  (newId, oldId) => {
    if (!newId || newId === oldId) return
    loadRun(true)
  },
)
onUnmounted(() => {
  if (timer) window.clearInterval(timer)
  if (clock) window.clearInterval(clock)
  sandboxLogAbort?.abort()
  sandboxLogAbort = null
  sandboxLogGen++
  disposeAllRehydrateOrchs()
  document.removeEventListener('visibilitychange', onVisible)
  window.removeEventListener('focus', onFocusRefresh)
  teardownRealtime()
})

function onArtifactDeleted(id: string) {
  run.value.artifacts = run.value.artifacts.filter((a) => a.id !== id)
}

const gateError = ref<string | null>(null)
const gateSubmitting = ref(false)
const clarifyConfirmError = ref<string | null>(null)
async function onGateResolve(action: string, form: Record<string, any> = {}) {
  if (!run.value.gate || gateSubmitting.value) return
  gateSubmitting.value = true
  gateError.value = null
  try {
    await api.resumeGate(runId.value, run.value.gate.nodeId, action, form)
    const positive = action === 'pass' || action === 'approve'
    toast.success(positive ? t('pages.gateApproval.approveSuccess') : t('pages.gateApproval.rejectSuccess'))
  } catch (e: any) {
    // Surface the backend rejection (e.g. a required form field, or the run
    // having ended while paused) instead of silently swallowing it and leaving
    // the gate showing an optimistic "已提交".
    gateError.value = e?.message || t('pages.runDetail.gateError')
  } finally {
    gateSubmitting.value = false
  }
  await loadRun(false)
}
async function onClarifySend(
  text: string,
  images: import('@/lib/shared/types').ClarifyImage[] = [],
  annotations: import('@/lib/shared/types').ReactAnnotation[] = [],
  force = false,
) {
  const conv = selClarify.value
  if (!conv || conv.done) return
  const nodeId = conv.nodeId
  clarifyConfirmError.value = null
  // Demo: send may attach the latest staged pick even if user skipped「添加到聊天」.
  const anns =
    hasAppPreview.value && !force ? mergeStagedAppPreviewPick(annotations) : annotations
  if (anns !== annotations) lastStagedAppPreviewPick.value = null
  try {
    await api.reactReply(runId.value, nodeId, text, images, force, anns)
  } catch (e: any) {
    // Re-sync below so the UI reflects the real state (e.g. the dialogue has
    // already completed) instead of leaving the input enabled to re-click.
    console.warn('reactReply failed', e?.message || e)
    const msg = e?.message || t('pages.runDetail.gateError')
    // Non-force: roll back optimistic pending-send row so FR4 / send lock is not stuck.
    if (!force) reviewChatRef.value?.discardLastQueued?.()
    clarifyConfirmError.value = msg
  }
  // Enqueue returns before the turn finishes — avoid wiping live bubbles.
  // Force finish still needs a snapshot refresh.
  if (force) {
    lastStagedAppPreviewPick.value = null
    await loadRun(false)
  }
}
async function onClarifyCancel() {
  const conv = selClarify.value
  if (!conv || conv.done) return
  try {
    await api.reactCancel(runId.value, conv.nodeId)
  } catch (e: any) {
    console.warn('reactCancel failed', e?.message || e)
  }
}
// Clarify: finish early. Review: confirm product & advance (different prompt).
function onClarifyFinish() {
  const prompt = reviewActive.value
    ? t('pages.clarify.confirmFlowPrompt')
    : t('pages.runDetail.clarifyFinishPrompt')
  onClarifySend(prompt, [], [], true)
}
const canCancelRun = computed(() => {
  const s = run.value?.status
  return s === 'queued' || s === 'running' || s === 'waiting_human'
})

const showCancelConfirm = ref(false)
const cancellingRun = ref(false)
const cancelRunError = ref('')

function openCancelConfirm() {
  if (!canCancelRun.value || cancellingRun.value) return
  cancelRunError.value = ''
  showCancelConfirm.value = true
}

function closeCancelConfirm() {
  if (cancellingRun.value) return
  showCancelConfirm.value = false
  cancelRunError.value = ''
}

function mapCancelRunError(e: unknown): string {
  const status = (e as { status?: number })?.status
  const msg = e instanceof Error ? e.message : String(e || '')
  if (status === 404 || /not found/i.test(msg)) return t('pages.runDetail.cancelErrorNotFound')
  if (status === 400 || /already finished|cannot cancel/i.test(msg)) {
    return t('pages.runDetail.cancelErrorNotCancellable')
  }
  return msg || t('pages.runDetail.cancelErrorGeneric')
}

async function confirmCancelRun() {
  if (!canCancelRun.value || cancellingRun.value) return
  cancellingRun.value = true
  cancelRunError.value = ''
  try {
    await api.cancelRun(runId.value)
    showCancelConfirm.value = false
    toast.success(t('pages.runDetail.cancelSuccess'))
    await loadRun(false)
  } catch (e) {
    cancelRunError.value = mapCancelRunError(e)
  } finally {
    cancellingRun.value = false
  }
}

const canDeleteRun = computed(() => {
  const s = run.value?.status
  return s === 'completed' || s === 'failed' || s === 'cancelled'
})

const deleteRunHint = computed(() => {
  if (canDeleteRun.value) return ''
  const s = run.value?.status
  if (s === 'queued' || s === 'running' || s === 'waiting_human') {
    return t('pages.runDetail.deleteHintActive')
  }
  return ''
})

const showDeleteConfirm = ref(false)
const deletingRun = ref(false)
const deleteRunError = ref('')

function openDeleteConfirm() {
  if (!canDeleteRun.value || deletingRun.value) return
  deleteRunError.value = ''
  showDeleteConfirm.value = true
}

function closeDeleteConfirm() {
  if (deletingRun.value) return
  showDeleteConfirm.value = false
  deleteRunError.value = ''
}

function mapDeleteRunError(e: unknown): string {
  const status = (e as { status?: number })?.status
  const msg = e instanceof Error ? e.message : String(e || '')
  if (status === 404 || /not found/i.test(msg)) return t('pages.runDetail.deleteErrorNotFound')
  if (status === 409 || /cannot delete run/i.test(msg)) return t('pages.runDetail.deleteErrorNotDeletable')
  return msg || t('pages.runDetail.deleteErrorGeneric')
}

async function confirmDeleteRun() {
  if (!canDeleteRun.value || deletingRun.value) return
  const wfId = run.value?.workflowId || ''
  deletingRun.value = true
  deleteRunError.value = ''
  try {
    await api.deleteRun(runId.value)
    showDeleteConfirm.value = false
    toast.success(t('pages.runDetail.deleteSuccess'))
    const qs = wfId ? `?wf=${encodeURIComponent(wfId)}` : ''
    await router.push('/runs' + qs)
  } catch (e) {
    deleteRunError.value = mapDeleteRunError(e)
  } finally {
    deletingRun.value = false
  }
}

// Download the full logs of this run as a single text file for offline error
// diagnosis (FSM trace + every node/iteration's agent events, MCP calls,
// errors, output + sandbox docker logs), assembled server-side.
function exportLogs() {
  const a = document.createElement('a')
  a.href = api.exportRunLogsUrl(runId.value)
  a.download = `${runId.value}-logs.txt`
  a.click()
}

// Continue a failed/cancelled run from a node (default: the node that failed),
// reusing everything the original run already produced. Used after a transient
// fault (e.g. a sandbox/ACP hiccup) so automation recovers without re-running
// the whole workflow.
const resuming = ref(false)
const resumeError = ref<string | null>(null)
async function onResume(nodeId = '') {
  if (resuming.value) return
  resuming.value = true
  resumeError.value = null
  try {
    await api.resumeRun(runId.value, nodeId)
  } catch (e: any) {
    resumeError.value = e?.message || t('pages.runDetail.resumeError')
  } finally {
    resuming.value = false
  }
  await loadRun(false)
}

const priorityDraft = ref<RunPriority>('normal')
const prioritySaving = ref(false)
const priorityError = ref<string | null>(null)
const priorityOk = ref(false)
const priorityPopoverOpen = ref(false)
const priorityBadgeRef = ref<HTMLElement | null>(null)
const priorityEditorRef = ref<HTMLElement | null>(null)
const priorityPopoverStyle = ref<Record<string, string>>({
  top: '0px',
  left: '0px',
  width: '320px',
})
/** Success tip (+ auto-close) is cleared on its own timer — never by draft sync after save. */
let priorityOkTimer: ReturnType<typeof setTimeout> | null = null

const priorityEditable = computed(() =>
  ['queued', 'running', 'waiting_human'].includes(run.value.status),
)

/** Weak discoverability cue: chevron only while queued. */
const showPriorityChevron = computed(
  () => priorityEditable.value && run.value.status === 'queued',
)

const committedPriority = computed<RunPriority>(() => {
  const p = run.value.priority
  return p === 'high' || p === 'low' || p === 'normal' ? p : 'normal'
})

/** Match badge semantic color so the chevron reads as part of the trigger. */
const priorityChevronClass = computed(() => {
  const p = committedPriority.value
  if (p === 'high') return 'text-[#f87171]'
  if (p === 'low') return 'text-txt3'
  return 'text-accent-2'
})

const priorityTriggerTitle = computed(() =>
  priorityEditable.value
    ? t('pages.runDetail.priorityEditTrigger')
    : t('common.priority.label'),
)

const priorityTriggerAria = computed(() => {
  const priority = t(`common.priority.${committedPriority.value}`)
  if (priorityEditable.value) {
    return t('pages.runDetail.priorityEditAria', { priority })
  }
  return `${t('common.priority.label')} ${priority}`
})

const priorityHint = computed(() => {
  switch (run.value.status) {
    case 'running':
      return t('pages.runDetail.priorityHintRunning')
    case 'waiting_human':
      return t('pages.runDetail.priorityHintWaiting')
    case 'queued':
      return t('pages.runDetail.priorityHintQueued')
    default:
      return t('pages.runDetail.priorityHintTerminal')
  }
})

function clearPriorityOkTimer() {
  if (priorityOkTimer) {
    clearTimeout(priorityOkTimer)
    priorityOkTimer = null
  }
}

function placePriorityPopover() {
  const anchor = priorityBadgeRef.value
  if (!anchor || !priorityPopoverOpen.value) return
  const margin = 16
  const gap = 8
  const width = Math.min(320, window.innerWidth - 32)
  const rect = anchor.getBoundingClientRect()
  let left = rect.left
  if (left + width > window.innerWidth - margin) {
    left = window.innerWidth - margin - width
  }
  left = Math.max(margin, left)
  let top = rect.bottom + gap
  // Keep panel in viewport when near the bottom edge.
  const estimatedHeight = 220
  if (top + estimatedHeight > window.innerHeight - margin) {
    top = Math.max(margin, rect.top - estimatedHeight - gap)
  }
  priorityPopoverStyle.value = {
    top: `${Math.round(top)}px`,
    left: `${Math.round(left)}px`,
    width: `${Math.round(width)}px`,
  }
}

function openPriorityPopover() {
  if (!priorityEditable.value) return
  syncPriorityDraft()
  priorityOk.value = false
  priorityError.value = null
  clearPriorityOkTimer()
  priorityPopoverOpen.value = true
  void nextTick(() => {
    placePriorityPopover()
    priorityEditorRef.value?.focus()
  })
}

function closePriorityPopover(discard = true) {
  if (!priorityPopoverOpen.value) return
  priorityPopoverOpen.value = false
  clearPriorityOkTimer()
  priorityOk.value = false
  if (discard) syncPriorityDraft()
  void nextTick(() => priorityBadgeRef.value?.focus())
}

function togglePriorityPopover() {
  if (!priorityEditable.value) return
  if (priorityPopoverOpen.value) closePriorityPopover(true)
  else openPriorityPopover()
}

/** Brief in-popover tip, then auto-close the editor layer. */
function showPrioritySaved() {
  clearPriorityOkTimer()
  priorityOk.value = true
  priorityOkTimer = setTimeout(() => {
    priorityOk.value = false
    priorityOkTimer = null
    closePriorityPopover(false)
  }, 1000)
}

function syncPriorityDraft() {
  const p = run.value.priority
  priorityDraft.value = p === 'high' || p === 'low' || p === 'normal' ? p : 'normal'
  priorityError.value = null
  // Do not clear priorityOk here: save writes run.priority and would flash the tip away.
}

function onPriorityKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape' && priorityPopoverOpen.value) {
    e.preventDefault()
    closePriorityPopover(true)
  }
}

function onPriorityReposition() {
  if (priorityPopoverOpen.value) placePriorityPopover()
}

watch(
  () => run.value.id,
  () => {
    closePriorityPopover(true)
    clearPriorityOkTimer()
    priorityOk.value = false
    syncPriorityDraft()
  },
)

// Watch scalar sources (not a fresh array each tick): poll/WS that replaces
// run.value with the same priority+status must not re-fire and wipe a dirty draft.
watch(
  [() => run.value.priority, () => run.value.status],
  () => {
    const saved =
      run.value.priority === 'high' || run.value.priority === 'low' || run.value.priority === 'normal'
        ? run.value.priority
        : 'normal'
    // Keep unsaved draft while still editable (running/waiting_human poll) —
    // including while the popover is open with a dirty draft.
    if (priorityEditable.value && priorityDraft.value !== saved) return
    syncPriorityDraft()
  },
)

watch(priorityEditable, (editable) => {
  if (!editable) closePriorityPopover(true)
})

async function savePriority() {
  if (!priorityEditable.value || prioritySaving.value) return
  if (priorityDraft.value === committedPriority.value) return
  prioritySaving.value = true
  priorityError.value = null
  priorityOk.value = false
  clearPriorityOkTimer()
  try {
    const res = await api.updateRunPriority(runId.value, priorityDraft.value)
    run.value = { ...run.value, priority: (res.priority as RunPriority) || priorityDraft.value }
    showPrioritySaved()
  } catch (e: any) {
    priorityError.value = e?.message || t('pages.runDetail.prioritySaveFailed')
  } finally {
    prioritySaving.value = false
  }
}

onMounted(() => {
  document.addEventListener('keydown', onPriorityKeydown)
  window.addEventListener('resize', onPriorityReposition)
  window.addEventListener('scroll', onPriorityReposition, true)
})

onUnmounted(() => {
  clearPriorityOkTimer()
  document.removeEventListener('keydown', onPriorityKeydown)
  window.removeEventListener('resize', onPriorityReposition)
  window.removeEventListener('scroll', onPriorityReposition, true)
})

// A terminated run can be resumed; the header button auto-picks the failed node.
const canResume = computed(() => run.value.status === 'failed' || run.value.status === 'cancelled')
// The selected node can be individually retried when the run is terminated
// (any graph node) or still running with a failed node (react sandbox setup).
const canResumeSelected = computed(() => {
  if (!selected.value || !selNode.value) return false
  if (run.value.status === 'failed' || run.value.status === 'cancelled') return true
  return run.value.status === 'running' && selStatus.value === 'failed'
})

const statusMap = computed<Record<string, NodeRunStatus>>(() => {
  const m: Record<string, NodeRunStatus> = {}
  for (const [k, v] of Object.entries(run.value.nodeRuns)) m[k] = v.status
  // Surface the in-flight node (no StateRun yet) as running on the canvas.
  if (liveNode.value && !m[liveNode.value]) m[liveNode.value] = 'running'
  return m
})

const activePath = computed(() => {
  const ids: string[] = []
  for (const e of wf.value.edges) {
    const s = run.value.nodeRuns[e.source]?.status
    const t = run.value.nodeRuns[e.target]?.status
    if (s === 'completed' && (t === 'running' || t === 'waiting_human')) ids.push(e.id)
  }
  return ids
})

const ACTIVE = ['queued', 'running', 'waiting_human']

// Live wall-clock tick (1s) so the elapsed timer advances without waiting for
// the 2s state poll. Backend never fills Run.durationSec mid-run, so we derive
// it here from startedAt instead of showing a frozen 00:00.
const nowMs = ref(Date.now())
const elapsedSec = computed(() => {
  const start = Date.parse(run.value.startedAt)
  if (isNaN(start)) return run.value.durationSec || 0
  if (ACTIVE.includes(run.value.status)) return Math.max(0, Math.floor((nowMs.value - start) / 1000))
  if (run.value.durationSec > 0) return run.value.durationSec
  // Terminal run without a backend duration: derive the end from the latest
  // node finish (startedAt + its own duration).
  let end = start
  for (const nr of Object.values(run.value.nodeRuns)) {
    if (!nr.startedAt) continue
    const t = Date.parse(nr.startedAt) + (nr.durationSec || 0) * 1000
    if (!isNaN(t) && t > end) end = t
  }
  return Math.max(0, Math.floor((end - start) / 1000))
})

// Progress that counts the in-flight node as half-done, so a run sitting on its
// first (long) node reads as "in progress" instead of a flat 0%. Falls back to
// the backend fraction when the pinned graph isn't available yet.
const progressFrac = computed(() => {
  const nodes = wf.value.nodes.length ? wf.value.nodes : run.value.nodes || []
  const total = nodes.length
  if (!total) return run.value.progress || 0
  if (run.value.status === 'completed') return 1
  let done = 0
  let active = 0
  for (const n of nodes) {
    const st = statusMap.value[n.id]
    if (st === 'completed' || st === 'skipped') done++
    else if (st === 'running' || st === 'waiting_human') active++
  }
  return Math.min(1, (done + active * 0.5) / total)
})

// default selection: last running/waiting_human in timeline order, else most recent execution
const defaultNode = computed(() => pickDefaultTimelineNodeId(run.value))

const selected = ref<string | null>(defaultNode.value || null)
const selNode = computed(() => wf.value.nodes.find((n) => n.id === selected.value) || null)
const selNodeDisplayLabel = computed(() =>
  selNode.value ? resolveNodeDisplayLabelFromNode(selNode.value, t) : '',
)

// The selected node's execution history (oldest→newest). A node that ran
// several times (loop-back / gate revise / rollback retry) has one entry per
// execution. Falls back to the single latest run for older runs / live nodes.
const selExecutions = computed<NodeRun[]>(() => {
  const id = selected.value
  if (!id) return []
  const list = run.value.nodeExecutions?.[id]
  if (list && list.length) return list
  const single = run.value.nodeRuns[id]
  return single ? [single] : []
})
// Which execution the user is viewing; null = the latest. Reset on re-selection.
const selIterIdx = ref<number | null>(null)
// When the timeline selects a specific execution it also changes `selected`;
// this carries the desired iteration index across that change so the reset
// below lands on the picked execution instead of the latest.
const pendingIter = ref<number | null>(null)
watch(selected, () => {
  selIterIdx.value = pendingIter.value
  pendingIter.value = null
})
// Clamp/refresh the pointer when the history grows (e.g. a new iteration starts
// while viewing the latest): stay pinned to latest unless the user picked one.
const selExecIdx = computed(() => {
  const n = selExecutions.value.length
  if (!n) return -1
  if (selIterIdx.value == null) return n - 1
  return Math.min(selIterIdx.value, n - 1)
})
const viewingLatest = computed(() => selExecIdx.value === selExecutions.value.length - 1)
const selRun = computed<NodeRun | null>(() => {
  const idx = selExecIdx.value
  return idx >= 0 ? selExecutions.value[idx] : selected.value ? run.value.nodeRuns[selected.value] : null
})

// Effective status of the selected node, falling back to the canvas status map
// (which surfaces the in-flight node as running even before a StateRun exists).
const selStatus = computed<NodeRunStatus>(() => selRun.value?.status || statusMap.value[selected.value || ''] || 'pending')
// Always give NodeOutputPanel a node-run to render, even before the node has
// produced one — so the panel shows "waiting / running" instead of vanishing.
const selRunView = computed<NodeRun | null>(() =>
  selNode.value ? selRun.value || { nodeId: selNode.value.id, status: selStatus.value, outputs: {} } : null,
)

// Events for the selected node: paginated fetch with optional prepended history,
// merged with live WS updates for the running node. Treat empty [] as a miss so
// it cannot block fallback to selRun.events.
const logEvents = computed<AcpEvent[]>(() => {
  const sel = selected.value
  if (!sel) return []
  if (!viewingLatest.value) return selRun.value?.events || []
  const pageEvents = eventPages[sel]?.events
  if (pageEvents && pageEvents.length > 0) return pageEvents
  return selRun.value?.events || []
})
const logHasMore = computed(() => {
  const sel = selected.value
  return sel ? (eventPages[sel]?.hasMore ?? false) : false
})
const logLive = computed(() => viewingLatest.value && !!selected.value && liveNode.value === selected.value)
// Authoritative busy for the selected node while it is live; undefined when the
// node isn't the one currently streaming (LiveLogPanel then falls back).
const logBusy = computed<boolean | undefined>(() =>
  logLive.value && selected.value ? liveBusy[selected.value] : undefined,
)

// Prefer authoritative run mcpCalls; fall back to session cache so re-entry
// still has displayable content when REST rehydrate fails.
const selMcpCalls = computed<McpCall[] | undefined>(() => {
  const fromRun = selRun.value?.mcpCalls
  if (fromRun?.length) return fromRun
  const sel = selected.value
  if (!sel) return undefined
  const cached = getLiveLogSnapshot(runId.value, sel)?.mcpCalls
  return cached?.length ? cached : undefined
})

watch(
  () => {
    const id = selected.value
    if (!id) return null
    const calls = run.value.nodeRuns[id]?.mcpCalls
    return calls?.length ? { id, calls } : null
  },
  (v) => {
    if (v) syncMcpCallsToCache(v.id, v.calls)
  },
  { deep: true },
)

const sbxLog = computed(() => (selected.value ? sbxLogs[selected.value] : null) || null)

// Paint-then-work: commit selection + brief right-panel loading first, then
// rehydrate/sandbox after a frame so pointer INP is not blocked by heavy work.
const panelSwitching = ref(false)
let selectionWorkGen = 0

function afterNextPaint(): Promise<void> {
  return new Promise((resolve) => {
    requestAnimationFrame(() => resolve())
  })
}

async function runSelectionSideEffects(id: string | null) {
  const gen = ++selectionWorkGen
  if (!id) {
    panelSwitching.value = false
    void rehydrateNodeEvents(null)
    fetchSandboxLog(null)
    return
  }
  panelSwitching.value = true
  await nextTick()
  await afterNextPaint()
  if (gen !== selectionWorkGen) return
  void rehydrateNodeEvents(id)
  fetchSandboxLog(id)
  // Yield once more so heavy tab mounts land after the loading frame paints.
  await afterNextPaint()
  if (gen !== selectionWorkGen) return
  panelSwitching.value = false
}

// Pull the node's event + container logs whenever the selection changes (covers
// refresh / re-entry: the running node's logs are read back from its container).
// Deferred via paint-then-work so selected highlight paints before rehydrate.
watch(selected, (id) => {
  void runSelectionSideEffects(id)
})

// The right panel is scoped to the selected node with node-relevant tabs.
// Gate/clarify nodes add their interaction tab; agent-like nodes (sandbox
// execution) add the ACP execution-log tab.
const gateActive = computed(() => !!run.value.gate && selected.value === run.value.gate.nodeId)
// A react node IS the clarify node — surface its tab as soon as the node is
// selected, even before the first turn has finished generating (the conversation
// row is only created after ReactOpen returns). The panel then shows a loading
// state until the dialogue is available.
const clarifyActive = computed(() => selNode.value?.type === 'react')
// The selected react node's own conversation (per-node), falling back to the
// run's current clarify when it matches this node.
const selClarify = computed(() => {
  const id = selected.value
  if (!id) return null
  return run.value.clarifyByNode?.[id] || (run.value.clarify?.nodeId === id ? run.value.clarify : null) || null
})
// The clarify chat only accepts replies while the run is live AND this node is
// genuinely waiting for human input (not a sandbox infrastructure failure).
const clarifyInputActive = computed(
  () => ['queued', 'running', 'waiting_human'].includes(run.value.status) && selStatus.value === 'waiting_human',
)
// React node failed during sandbox setup — show error-box instead of chat/loader.
const clarifySandboxFailed = computed(
  () => selNode.value?.type === 'react' && selStatus.value === 'failed' && !!selRun.value?.error,
)
// Run-level failure banner for any failed run (research/agent early fails included).
const runFailureReason = computed(() => {
  if (run.value.status !== 'failed') return ''
  return (run.value.error || run.value.failedReason || '').trim()
})
const showRunFailureBanner = computed(() => !!runFailureReason.value)
const { draft: clarifyDraft, attachments: clarifyAttachments, annotations: clarifyAnnotations } = useClarifyDraft(() => runId.value, () => selected.value)

/** VNC pick on app_preview review stage → same ReAct annotation chips as structured ⤴. */
const lastStagedAppPreviewPick = ref<AppPreviewPickPayload | null>(null)

function onAppPreviewStagedPick(payload: AppPreviewPickPayload | null) {
  lastStagedAppPreviewPick.value = payload
}

function onAppPreviewReviewPick(payload: AppPreviewPickPayload) {
  const rid = run.value?.id
  const nid = selNode.value?.id
  if (!rid || !nid) return
  const url = (payload.url || '').trim()
  const result = addClarifyAnnotation(rid, nid, {
    selector: payload.selector,
    url: url || undefined,
    label: previewPickLabel(url, payload.selector, payload.tagName),
  })
  if (result === 'duplicate') toast.warn(t('pages.reviewComposer.alreadyAdded'))
  // Added to pending chips — clear staged so send won't double-attach.
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
// Every sandbox-backed node (all "Agent" category types: agent/react/plan/
// implement/research/test/review/proposal/submit_mr/visual) runs the in-container
// cursor-agent, so it gets both the ACP 执行日志 and 沙箱日志 tabs. Derive this
// from the node registry so new Agent node types are covered automatically.
const hasLog = computed(() => !!selNode.value && NODE_DEFS[selNode.value.type]?.category === 'nodes.categories.agent')

// Non-generic Agent cards each expose a structured product; surface it in a
// dedicated "产物" tab. The generic `agent` node is intentionally excluded.
const hasProduct = computed(() => !!selNode.value && PRODUCT_NODE_TYPES.includes(selNode.value.type))
const nodeCompleted = computed(() => selStatus.value === 'completed')

const hasAppPreview = computed(() => selNode.value?.type === 'app_preview')

// Post-run ReAct review: a non-react producer node that has an open review
// conversation (the backend only seeds one for review-capable producers). The
// combined review tab shows the product view (annotatable) + the ReAct chat.
const reviewActive = computed(() => {
  const n = selNode.value
  if (!n || n.type === 'react') return false
  const conv = selClarify.value
  return !!conv && !conv.done
})

const nodeTabs = computed(() => {
  const tabs: { id: string; label: string; ghosted?: boolean; disabled?: boolean }[] = []
  if (gateActive.value) tabs.push({ id: 'gate', label: t('pages.runDetail.tabs.gate') })
  // app_preview: Gate shell removed — keep a ghosted Gate tab (Demo) that cannot enter.
  else if (hasAppPreview.value && (reviewActive.value || selStatus.value === 'waiting_human')) {
    tabs.push({ id: 'gate', label: t('pages.runDetail.tabs.gate'), ghosted: true, disabled: true })
  }
  if (clarifyActive.value) tabs.push({ id: 'clarify', label: t('pages.runDetail.tabs.clarify') })
  if (reviewActive.value) tabs.push({ id: 'review', label: t('pages.runDetail.tabs.review') })
  if (hasAppPreview.value) tabs.push({ id: 'preview', label: t('pages.runDetail.tabs.appPreview') })
  if (hasProduct.value) tabs.push({ id: 'product', label: t('pages.runDetail.tabs.product') })
  tabs.push({ id: 'output', label: t('pages.runDetail.tabs.output') })
  if (hasLog.value) tabs.push({ id: 'log', label: t('pages.runDetail.tabs.log') })
  if (hasLog.value) tabs.push({ id: 'sandbox', label: t('pages.runDetail.tabs.sandbox') })
  return tabs
})

function onNodeTabDisabledClick(id: string) {
  if (id === 'gate') toast.warn(t('pages.runDetail.gateRemoved'))
}
const nodeTab = ref('output')
/** When set, watch(selected) must not steal tab=output (QQ deep link / live complete). */
const outputFocusLock = ref(false)

function graphNodesForFocus() {
  return wf.value.nodes.length ? wf.value.nodes : run.value.nodes || []
}

function queryParam(key: string): string {
  const raw = route.query[key]
  if (typeof raw === 'string') return raw.trim()
  if (Array.isArray(raw) && typeof raw[0] === 'string') return String(raw[0]).trim()
  return ''
}

/** Parse ?node=&tab=output (completed QQ deep link). Returns true when applied. */
function applyOutputDeepLinkFocus(): boolean {
  const qNode = queryParam('node')
  const qTab = queryParam('tab')
  if (qTab !== 'output' && !qNode) return false

  const nodes = graphNodesForFocus()
  const focusId =
    (qNode && nodes.some((n) => n.id === qNode) ? qNode : null) ||
    resolveOutputFocusNodeId(run.value, nodes)

  outputFocusLock.value = true
  if (focusId) {
    manual.value = false
    selected.value = focusId
  }
  nodeTab.value = 'output'
  if (isMobile.value) mobileMainPanel.value = 'detail'
  return true
}

/**
 * Mobile (≤767) list-detail: mutually exclusive timeline vs node detail.
 * Desktop keeps side-by-side panes; this state is ignored when !isMobile.
 * Defaults: waiting_human or deep-link tab=output → detail; else timeline.
 * Live running→completed also switches to detail (see status watch).
 */
const mobileMainPanel = ref<'timeline' | 'detail'>(
  isMobile.value && (run.value.status === 'waiting_human' || queryParam('tab') === 'output')
    ? 'detail'
    : 'timeline',
)
/** Bumped to re-scroll selected timeline item (e.g. back from detail). */
const timelineScrollToken = ref(0)

const mobileDetailPanelLabel = computed(() => {
  const tab = nodeTabs.value.find((t) => t.id === nodeTab.value)
  return tab?.label || t('pages.runDetail.tabs.output')
})

function showMobileTimelinePanel() {
  mobileMainPanel.value = 'timeline'
  timelineScrollToken.value += 1
}

function showMobileDetailPanel() {
  mobileMainPanel.value = 'detail'
}

function backToMobileTimeline() {
  showMobileTimelinePanel()
}

/** Desktop「复审」Tab: widen right panel + canvas floor (see reviewLayoutBudget). */
const desktopReviewLayout = computed(() => !isMobile.value && nodeTab.value === 'review')
const reviewRightPanelStyle = computed(() =>
  desktopReviewLayout.value ? { width: reviewRightPanelCssWidth() } : undefined,
)
const canvasPaneStyle = computed(() =>
  desktopReviewLayout.value ? { minWidth: `${REVIEW_CANVAS_MIN}px` } : undefined,
)

// Pick a sensible default tab when the SELECTION changes (not on every poll, or
// it would fight the user's manual tab choice): a pending human interaction
// first, then the live log for agent-like nodes, else the overview.
watch(
  selected,
  () => {
    if (outputFocusLock.value) {
      nodeTab.value = 'output'
      return
    }
    // app_preview: Gate 仅壳，主交互为复审对话 + VNC
    if (hasAppPreview.value && reviewActive.value) nodeTab.value = 'review'
    else if (gateActive.value) nodeTab.value = 'gate'
    else if (clarifyActive.value && !run.value.clarify?.done) nodeTab.value = 'clarify'
    else if (reviewActive.value) nodeTab.value = 'review'
    else if (hasProduct.value && nodeCompleted.value) nodeTab.value = 'product'
    else if (hasLog.value) nodeTab.value = 'log'
    else nodeTab.value = 'output'
  },
  { immediate: true },
)
watch(nodeTab, (tab) => {
  if (outputFocusLock.value && tab !== 'output') outputFocusLock.value = false
})
// Live running/waiting_human→completed: select last output node, open output view,
// mobile detail. Skip hard-load hydration (emptyRun dummy running → completed).
watch(
  () => run.value.status,
  (st, prev) => {
    if (st !== 'completed') return
    if (runLoading.value) return
    if (prev !== 'running' && prev !== 'waiting_human') return
    const id = resolveOutputFocusNodeId(run.value, graphNodesForFocus())
    if (id) {
      manual.value = false
      selected.value = id
    }
    outputFocusLock.value = true
    nodeTab.value = 'output'
    if (isMobile.value) mobileMainPanel.value = 'detail'
  },
)

watch(
  () => run.value.gate?.nodeId,
  (gateNodeId) => {
    if (isMobile.value && gateNodeId && run.value.status === 'waiting_human') {
      manual.value = false
      selected.value = gateNodeId
      nodeTab.value = 'gate'
      mobileMainPanel.value = 'detail'
    }
  },
  { immediate: true },
)

// waiting_human without a gate (e.g. review/clarify): still prefer detail panel once.
watch(
  () => run.value.status,
  (st, prev) => {
    if (!isMobile.value) return
    if (st === 'waiting_human' && prev !== 'waiting_human' && !run.value.gate?.nodeId) {
      mobileMainPanel.value = 'detail'
    }
  },
)
// If the current tab disappears (e.g. clarify resolved), fall back gracefully.
// Ghosted/disabled tabs (app_preview Gate) are never a valid active selection.
watch(nodeTabs, (tabs) => {
  const cur = tabs.find((t) => t.id === nodeTab.value)
  if (!cur || cur.ghosted || cur.disabled) {
    nodeTab.value = tabs.find((t) => !t.ghosted && !t.disabled)?.id || 'output'
  }
})

// Lazy lookup of the run/node sandbox for the console button. Only fetched when
// viewing the latest execution on an agent node's log/sandbox tab.
async function fetchRunNodeSandbox() {
  const id = selected.value
  if (
    !id ||
    !hasLog.value ||
    !viewingLatest.value ||
    (nodeTab.value !== 'log' && nodeTab.value !== 'sandbox')
  ) {
    sandboxLookup.value = null
    return
  }
  try {
    sandboxLookup.value = await api.getRunNodeSandbox(runId.value, id)
  } catch {
    sandboxLookup.value = null
  }
}

function openSandboxConsole() {
  const id = sandboxLookup.value?.id
  if (!id) return
  router.push({ path: `/sandboxes/${id}/console`, query: { tab: 'terminal' } })
}

watch([selected, selIterIdx, nodeTab, viewingLatest, hasLog], fetchRunNodeSandbox)

// While execution log is in boot empty state, keep refreshing sandbox signals
// so creating → container ready → running can advance the three stages.
// Also poll on sandbox tab so stages can still advance after timeout CTA.
function maybePollSandboxForBoot() {
  if (!hasLog.value || !viewingLatest.value) return
  if (nodeTab.value !== 'log' && nodeTab.value !== 'sandbox') return
  if (selStatus.value !== 'running') return
  if ((logEvents.value?.length || 0) > 0) return
  if (selRun.value?.mcpCalls?.length) return
  void fetchRunNodeSandbox()
}

function goSandboxLogTab() {
  nodeTab.value = 'sandbox'
}

function liveLogBootKey(nodeId: string | null | undefined, iterIdx: number): string {
  if (!nodeId) return ''
  return `${nodeId}:${iterIdx}`
}

const currentLiveLogBootSession = computed<LiveLogBootSession | null>(() => {
  const key = liveLogBootKey(selected.value, selExecIdx.value)
  return key ? liveLogBootSessions[key] ?? null : null
})

function onLiveLogBootSession(session: LiveLogBootSession) {
  const key = liveLogBootKey(selected.value, selExecIdx.value)
  if (!key) return
  if (session.confirmedPhase == null && session.stageEnteredAt == null && !session.timedOut) {
    delete liveLogBootSessions[key]
    return
  }
  liveLogBootSessions[key] = { ...session }
}

// Overall run detail drawer: cross-node info not tied to a single node.
const showDetail = ref(false)
const detailTab = ref('trace')
const detailTabs = computed(() => {
  const tabs = [{ id: 'trace', label: t('pages.runDetail.tabs.trace') }]
  if (run.value.vars?.length) tabs.push({ id: 'vars', label: t('pages.runDetail.tabs.vars') })
  // Always offer the run-level env tab so empty snapshot shows an empty state (no error).
  tabs.push({ id: 'sandboxEnv', label: t('pages.runDetail.tabs.sandboxEnv') })
  tabs.push({ id: 'artifacts', label: t('pages.runDetail.tabs.artifacts') })
  return tabs
})

/** Parse ?detail=artifacts (run-output empty-state / triage deep link). */
function applyDetailArtifactsDeepLink(): boolean {
  if (queryParam('detail') !== 'artifacts') return false
  showDetail.value = true
  detailTab.value = 'artifacts'
  return true
}

function selectNode(id: string) {
  outputFocusLock.value = false
  manual.value = true
  selected.value = id
  if (isMobile.value) mobileMainPanel.value = 'detail'
}

// Main-area view: canvas / timeline (+ node detail) or execution-stats split.
// Narrow screens default to timeline (no persistence); desktop keeps canvas.
const viewMode = ref<'canvas' | 'timeline' | 'stats'>(isMobile.value ? 'timeline' : 'canvas')
/** Stats sub-tab: single-run (timeline + panel) vs multi-run aggregate (full width). */
const statsTab = ref<'single' | 'multi'>('single')

// Narrow: canvas is not in the allowed set — silently normalize without Toast.
watch(
  isMobile,
  (mobile) => {
    if (mobile && viewMode.value === 'canvas') viewMode.value = 'timeline'
  },
  { immediate: true },
)

// Entering mobile timeline should already have a meaningful selection when possible.
watch([viewMode, isMobile, defaultNode], () => {
  if (isMobile.value && viewMode.value === 'timeline' && !selected.value && defaultNode.value) {
    selected.value = defaultNode.value
  }
})

// Timeline click: focus a node AND a specific past execution so the right panel
// renders that exact iteration's product/log/overview. Setting selected resets
// the iteration pointer (watch above), so route the desired index through
// pendingIter when the node actually changes.
function selectExecution(nodeId: string, idx: number) {
  outputFocusLock.value = false
  manual.value = true
  if (selected.value === nodeId) {
    selIterIdx.value = idx
  } else {
    pendingIter.value = idx
    selected.value = nodeId
  }
  if (isMobile.value) mobileMainPanel.value = 'detail'
}
</script>

<template>
  <div data-testid="run-detail-root" class="flex h-full min-w-0 flex-col overflow-x-hidden bg-base">
    <!-- top bar: ≤767 two rows (status full-text priority); md+ single row -->
    <header class="shrink-0 overflow-x-hidden border-b border-line bg-surface px-5 py-3">
      <div v-if="runLoading || loadError" class="flex min-w-0 items-center gap-3">
        <button class="flex h-8 w-8 shrink-0 items-center justify-center rounded-md text-txt2 hover:bg-elevated hover:text-txt" @click="router.push('/runs')">
          <Icon name="arrow-left" :size="18" />
        </button>
        <h1 class="min-w-0 truncate text-[17px] font-semibold text-txt">Run #{{ runId.replace('run-', '') }}</h1>
        <span v-if="runLoading" class="chip shrink-0 text-txt3">{{ t('pages.runDetail.loadingChip') }}</span>
        <span
          v-else
          class="chip shrink-0 border-err/35 bg-err/8 text-err"
          data-testid="run-load-error-chip"
        >{{
          loadErrorKind === 'not_found'
            ? t('pages.runDetail.notFoundChip')
            : t('pages.runDetail.loadFailedChip')
        }}</span>
      </div>
      <template v-else>
        <div class="flex min-w-0 flex-col gap-2 md:flex-row md:items-center md:gap-3">
          <div data-testid="run-header-row1" class="flex min-w-0 flex-1 items-center gap-2 md:gap-3">
            <button class="flex h-8 w-8 shrink-0 items-center justify-center rounded-md text-txt2 hover:bg-elevated hover:text-txt" @click="router.push('/runs')">
              <Icon name="arrow-left" :size="18" />
            </button>
            <h1 class="min-w-0 truncate text-[17px] font-semibold text-txt">Run #{{ run.id.replace('run-', '') }}</h1>
            <TruncatedTextTooltip
              :text="run.workflowName"
              data-testid="workflow-chip"
              class="chip hidden max-w-[9rem] truncate md:inline-flex"
            />
            <span
              v-if="run.workflowVersion"
              data-testid="version-chip"
              class="chip shrink-0"
              :title="t('common.format.pinnedVersionTitle')"
            >v{{ run.workflowVersion }}</span>
            <StatusPill data-testid="status-pill" :status="run.status" class="shrink-0" />
            <button
              v-if="priorityEditable"
              ref="priorityBadgeRef"
              type="button"
              data-testid="priority-badge"
              class="inline-flex shrink-0 items-center gap-1 border-0 bg-transparent p-0 text-left transition hover:opacity-90 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent-2"
              :title="priorityTriggerTitle"
              :aria-label="priorityTriggerAria"
              aria-haspopup="dialog"
              :aria-expanded="priorityPopoverOpen"
              aria-controls="run-priority-editor"
              @click="togglePriorityPopover"
            >
              <PriorityBadge :priority="run.priority" hide-title />
              <Icon
                v-if="showPriorityChevron"
                name="chevron-down"
                :size="10"
                class="shrink-0 opacity-75 transition-transform"
                :class="[priorityChevronClass, { 'rotate-180': priorityPopoverOpen }]"
                aria-hidden="true"
              />
            </button>
            <PriorityBadge
              v-else
              data-testid="priority-badge"
              :priority="run.priority"
              class="shrink-0"
            />
          </div>
          <div data-testid="run-header-actions" class="flex flex-wrap items-center gap-2 pl-10 md:ml-auto md:shrink-0 md:pl-0">
            <AppButton variant="ghost" size="sm" icon="edit" @click="router.push('/workflows/' + run.workflowId + '/edit')">{{ t('common.buttons.edit') }}</AppButton>
            <AppButton variant="ghost" size="sm" icon="doc" @click="showDetail = true">{{ t('common.buttons.details') }}</AppButton>
            <AppButton
              data-testid="export-logs-btn"
              variant="ghost"
              size="sm"
              icon="download"
              :disabled="runLoading"
              @click="exportLogs"
            >{{ t('pages.runDetail.exportLogs') }}</AppButton>
            <button
              class="inline-flex items-center gap-1.5 rounded-md border border-line bg-surface px-2.5 py-1.5 text-xs font-medium text-txt transition hover:border-line-strong hover:bg-elevated disabled:opacity-45"
              :disabled="refreshing || runLoading"
              @click="loadRun(false)"
            >
              <Icon name="refresh" :size="14" :class="{ 'animate-spin': refreshing }" />
              {{ t('common.buttons.refresh') }}
            </button>
            <AppButton
              v-if="canCancelRun"
              data-testid="cancel-run-btn"
              variant="danger"
              size="sm"
              :icon="cancellingRun ? 'spinner' : 'close'"
              :disabled="cancellingRun"
              :aria-busy="cancellingRun ? 'true' : 'false'"
              @click="openCancelConfirm"
            >{{ cancellingRun ? t('common.buttons.cancelling') : t('common.buttons.cancelRun') }}</AppButton>
            <AppButton
              data-testid="delete-run-btn"
              variant="danger"
              size="sm"
              icon="trash"
              :disabled="!canDeleteRun || deletingRun"
              :title="deleteRunHint || t('common.buttons.deleteRun')"
              @click="openDeleteConfirm"
            >{{ t('common.buttons.deleteRun') }}</AppButton>
            <span
              v-if="deleteRunHint"
              data-testid="delete-run-hint"
              class="text-[11px] text-txt3"
            >{{ deleteRunHint }}</span>
            <AppButton v-if="canResume" variant="primary" size="sm" icon="refresh" :disabled="resuming" @click="onResume('')">
              {{ resuming ? t('common.buttons.resuming') : t('common.buttons.resumeFromFail') }}
            </AppButton>
          </div>
        </div>
        <div v-if="resumeError" class="mt-1.5 pl-11 text-[12px] text-err">{{ resumeError }}</div>
        <div class="mt-2 flex min-w-0 max-w-full flex-wrap items-center gap-x-5 gap-y-1 pl-11 text-[12px] text-txt3">
          <span><Icon name="trigger" :size="12" class="mr-1 inline" />{{ formatTrigger(run.trigger) }}</span>
          <span><Icon name="clock" :size="12" class="mr-1 inline" />{{ fmtTime(run.startedAt) }}</span>
          <span>{{ t('pages.runDetail.duration') }} {{ fmtDuration(elapsedSec) }}</span>
          <span v-if="run.branch" class="min-w-0 max-w-full">{{ t('pages.runDetail.branch') }} <code class="inline-block max-w-full overflow-x-auto whitespace-nowrap align-bottom font-mono text-accent-2">{{ run.branch }}</code></span>
          <span v-if="run.git?.pushedSha" class="min-w-0 max-w-full">{{ t('pages.runDetail.sha') }} <code class="inline-block max-w-full overflow-x-auto whitespace-nowrap align-bottom font-mono text-accent-2">{{ run.git.pushedSha }}</code></span>
          <span v-else class="text-txt3">{{ t('pages.runDetail.noRepo') }}</span>
        </div>
        <div v-if="run.tags?.length" class="mt-2 flex flex-wrap items-center gap-1.5 pl-11">
          <span class="text-[12px] text-txt3">{{ t('pages.runDetail.tagsLabel') }}</span>
          <span v-for="tag in run.tags" :key="tag" class="chip text-txt2">{{ tag }}</span>
        </div>
        <div class="mt-2.5 flex items-center gap-3 pl-11">
          <div class="h-1.5 flex-1 overflow-hidden rounded-full bg-elevated">
            <div class="h-full rounded-full bg-gradient-to-r from-accent to-accent-2 transition-all" :style="{ width: progressFrac * 100 + '%' }" />
          </div>
          <span class="text-[11px] text-txt3">{{ Math.round(progressFrac * 100) }}%</span>
        </div>
        <div
          v-if="showRunFailureBanner"
          data-testid="run-failure-banner"
          class="mt-3 ml-11 mr-0 rounded-md border border-err/35 bg-err/8 px-3.5 py-2.5 text-[13px] leading-relaxed text-err"
          role="alert"
        >
          <strong class="mb-0.5 block text-[11px] font-semibold uppercase tracking-wide text-err/90">
            {{ t('pages.runDetail.failureBanner.title') }}
          </strong>
          <p class="m-0 whitespace-pre-wrap break-words text-txt">{{ runFailureReason }}</p>
        </div>
      </template>
    </header>

    <!-- Teleport avoids header overflow-x-hidden clipping; anchored to badge rect. -->
    <Teleport to="body">
      <div
        v-if="priorityPopoverOpen"
        data-testid="priority-popover-backdrop"
        class="fixed inset-0 z-30 bg-transparent"
        aria-hidden="true"
        @click="closePriorityPopover(true)"
      />
      <div
        v-if="priorityPopoverOpen"
        id="run-priority-editor"
        ref="priorityEditorRef"
        data-testid="run-priority-editor"
        role="dialog"
        tabindex="-1"
        :aria-label="t('pages.runDetail.priorityTitle')"
        class="fixed z-40 border border-line-strong bg-surface p-3.5 shadow-card outline-none"
        :style="priorityPopoverStyle"
      >
        <h3 class="mb-2.5 text-[13px] font-semibold text-txt">{{ t('pages.runDetail.priorityTitle') }}</h3>
        <PrioritySegmented v-model="priorityDraft" />
        <p class="mt-2.5 text-[12px] leading-relaxed text-txt3">{{ priorityHint }}</p>
        <div class="mt-3 flex flex-wrap items-center gap-2">
          <AppButton
            variant="primary"
            size="sm"
            :disabled="prioritySaving || priorityDraft === committedPriority"
            @click="savePriority"
          >
            {{ prioritySaving ? t('common.buttons.saving') : t('common.buttons.save') }}
          </AppButton>
        </div>
        <div
          v-if="priorityError"
          class="mt-2.5 flex items-start gap-1.5 border border-err/30 bg-err/10 px-2.5 py-2 text-[12px] text-err"
        >
          <Icon name="alert" :size="14" class="mt-0.5 shrink-0" />
          <span>{{ priorityError }}</span>
        </div>
        <div
          v-else-if="priorityOk"
          class="mt-2.5 border border-ok/30 bg-ok/10 px-2.5 py-2 text-[12px] text-ok"
        >
          {{ t('pages.runDetail.prioritySaved') }}
        </div>
      </div>
    </Teleport>

    <div data-testid="run-detail-main" class="relative flex min-h-0 min-w-0 w-full max-w-full flex-1">
      <div
        v-show="!runLoading && !loadError"
        data-testid="run-detail-content"
        class="flex min-h-0 min-w-0 w-full max-w-full flex-1 flex-col md:flex-row"
      >
      <!-- View mode switcher: always visible so mobile can open stats. -->
      <div
        class="flex shrink-0 items-center gap-2 border-b border-line bg-surface px-3 py-2 md:absolute md:left-3 md:top-3 md:z-10 md:border-0 md:bg-transparent md:p-0"
      >
        <div class="inline-flex rounded-lg border border-line bg-surface/90 p-0.5 text-[12px] backdrop-blur">
          <button
            v-if="!isMobile"
            data-testid="view-mode-canvas"
            class="rounded-md px-2.5 py-1 font-medium transition-colors"
            :class="viewMode === 'canvas' ? 'bg-accent-dim text-accent' : 'text-txt3 hover:text-txt2'"
            @click="viewMode = 'canvas'"
          >
            {{ t('pages.runDetail.canvas') }}
          </button>
          <button
            data-testid="view-mode-timeline"
            class="rounded-md px-2.5 py-1 font-medium transition-colors"
            :class="viewMode === 'timeline' ? 'bg-accent-dim text-accent' : 'text-txt3 hover:text-txt2'"
            @click="viewMode = 'timeline'"
          >
            {{ t('pages.runDetail.timeline') }}
          </button>
          <button
            data-testid="view-mode-stats"
            class="rounded-md px-2.5 py-1 font-medium transition-colors"
            :class="viewMode === 'stats' ? 'bg-accent-dim text-accent' : 'text-txt3 hover:text-txt2'"
            @click="viewMode = 'stats'"
          >
            {{ t('pages.runDetail.stats') }}
          </button>
        </div>
      </div>

      <!-- Stats mode: full-width single/multi tabs; single = timeline+panel (no click link); multi = full panel. -->
      <template v-if="viewMode === 'stats'">
        <div class="relative flex min-h-0 min-w-0 w-full max-w-full flex-1 flex-col md:pt-12">
          <div class="flex shrink-0 border-b border-line bg-surface px-3 sm:px-4">
            <button
              type="button"
              class="border-b-2 px-3 py-2.5 text-[13px] font-medium transition-colors"
              :class="
                statsTab === 'single'
                  ? 'border-accent-2 text-accent-2'
                  : 'border-transparent text-txt3 hover:text-txt2'
              "
              @click="statsTab = 'single'"
            >
              {{ t('pages.executionStats.tabSingle') }}
            </button>
            <button
              type="button"
              class="border-b-2 px-3 py-2.5 text-[13px] font-medium transition-colors"
              :class="
                statsTab === 'multi'
                  ? 'border-accent-2 text-accent-2'
                  : 'border-transparent text-txt3 hover:text-txt2'
              "
              @click="statsTab = 'multi'"
            >
              {{ t('pages.executionStats.tabMulti') }}
            </button>
          </div>
          <div
            data-testid="run-stats-split"
            class="relative flex min-h-0 min-w-0 w-full max-w-full flex-1"
            :class="statsTab === 'single' ? 'flex-col md:flex-row' : 'flex-col'"
          >
            <div
              v-if="statsTab === 'single'"
              class="relative min-h-[240px] min-w-0 flex-1 border-b border-line md:min-h-0 md:border-b-0 md:border-r"
            >
              <ExecutionTimeline
                :run="run"
                :nodes="wf.nodes"
                :selected-node-id="null"
                :selected-exec-idx="-1"
                :interactive="false"
                :now-ms="nowMs"
              />
            </div>
            <div
              data-testid="run-stats-panel-wrap"
              class="flex min-h-[320px] min-w-0 w-full max-w-full shrink-0 flex-col bg-surface md:min-h-0"
              :class="statsTab === 'single' ? 'md:w-[min(520px,46%)]' : 'min-w-0 flex-1'"
            >
              <ExecutionStatsPanel
                :run="run"
                :nodes="wf.nodes"
                :wall-sec="elapsedSec"
                :now-ms="nowMs"
                :stats-tab="statsTab"
                :unknown-model-display-name="unknownModelDisplayName"
                @update:stats-tab="statsTab = $event"
              />
            </div>
          </div>
        </div>
      </template>

      <template v-else>
      <!-- Canvas: desktop only (narrow viewMode is normalized away from canvas). -->
      <div
        v-if="viewMode === 'canvas'"
        class="relative hidden min-w-0 flex-1 border-r border-line md:block"
        :style="canvasPaneStyle"
      >
        <WorkflowCanvas
          :nodes="wf.nodes"
          :edges="wf.edges"
          mode="run"
          :status-map="statusMap"
          :selected-node="selected"
          :active-path="activePath"
          @select-node="selectNode"
        />
        <div class="pointer-events-none absolute right-3 top-3 rounded-md border border-line bg-surface/90 px-2.5 py-1 text-[11px] text-txt3 backdrop-blur">
          {{ t('pages.runDetail.canvasHint') }}
        </div>
      </div>

      <!-- Mobile ≤767: page-level timeline / detail tabs (Demo single-panel). -->
      <div
        v-if="isMobile && viewMode === 'timeline'"
        data-testid="mobile-main-panel-tabs"
        class="flex shrink-0 border-b border-line bg-surface"
      >
        <button
          type="button"
          data-testid="mobile-panel-timeline"
          class="flex-1 border-b-2 px-3 py-2.5 text-[12px] font-semibold transition-colors"
          :class="
            mobileMainPanel === 'timeline'
              ? 'border-accent text-accent'
              : 'border-transparent text-txt3'
          "
          @click="showMobileTimelinePanel"
        >
          {{ t('pages.runDetail.timeline') }}
        </button>
        <button
          type="button"
          data-testid="mobile-panel-detail"
          class="flex-1 border-b-2 px-3 py-2.5 text-[12px] font-semibold transition-colors"
          :class="
            mobileMainPanel === 'detail'
              ? 'border-accent text-accent'
              : 'border-transparent text-txt3'
          "
          @click="showMobileDetailPanel"
        >
          {{ mobileDetailPanelLabel }}
        </button>
      </div>

      <!-- Timeline: mobile single-panel (min-h-0 flex-1); desktop side pane. -->
      <div
        v-if="viewMode === 'timeline'"
        v-show="!isMobile || mobileMainPanel === 'timeline'"
        data-testid="run-timeline-pane"
        class="relative min-h-0 min-w-0 flex-1 border-b border-line md:border-b-0 md:border-r md:pt-12"
        :class="isMobile ? 'border-b-0' : ''"
      >
        <ExecutionTimeline
          :run="run"
          :nodes="wf.nodes"
          :selected-node-id="selected"
          :selected-exec-idx="selExecIdx"
          :now-ms="nowMs"
          :ensure-visible-token="timelineScrollToken"
          @select="selectExecution"
        />
      </div>

      <!-- right panel: scoped to the selected node; mobile fills main area when active -->
      <div
        v-show="!isMobile || mobileMainPanel === 'detail' || viewMode !== 'timeline'"
        data-testid="run-detail-right-panel"
        class="flex min-h-0 min-w-0 w-full max-w-full flex-col bg-surface"
        :class="[
          desktopReviewLayout ? '' : 'md:w-[520px]',
          isMobile && viewMode === 'timeline' ? 'flex-1' : 'shrink-0 md:shrink-0',
        ]"
        :style="reviewRightPanelStyle"
      >
        <!-- Mobile detail chrome: back to timeline -->
        <div
          v-if="isMobile && viewMode === 'timeline'"
          data-testid="mobile-detail-back-bar"
          class="flex shrink-0 items-center gap-2 border-b border-line bg-surface px-3 py-2"
        >
          <button
            type="button"
            data-testid="mobile-back-to-timeline"
            class="inline-flex items-center gap-1 rounded-md border border-line bg-elevated px-2 py-1 text-[11px] font-semibold text-txt2 hover:bg-surface"
            @click="backToMobileTimeline"
          >
            <Icon name="arrow-left" :size="12" />
            {{ t('pages.runDetail.backToTimeline') }}
          </button>
          <StatusPill v-if="selStatus" :status="selStatus" size="sm" class="shrink-0" />
          <span v-if="selNode" class="min-w-0 truncate text-[11px] text-txt3">{{ selNodeDisplayLabel }}</span>
        </div>
        <template v-if="selNode && selRunView">
          <!-- Per-node execution history: a node re-run by a loop-back / gate
               revise / rollback keeps every past execution. Switch between them
               to trace each run's own output, log, and duration. -->
          <div v-if="selExecutions.length > 1" class="flex shrink-0 flex-wrap items-center gap-1.5 border-b border-line px-3 py-2">
            <span class="mr-0.5 text-[11px] text-txt3">{{ t('pages.runDetail.executionHistory') }}</span>
            <button
              v-for="(ex, i) in selExecutions"
              :key="i"
              class="rounded-md border px-2 py-0.5 text-[11px] transition-colors"
              :class="i === selExecIdx ? 'border-accent/50 bg-accent-dim text-accent' : 'border-line text-txt2 hover:bg-elevated'"
              :title="ex.durationSec != null ? t('pages.runDetail.duration') + ' ' + fmtDuration(ex.durationSec) : ''"
              @click="selIterIdx = i"
            >
              {{ t('pages.runDetail.executionN', { n: ex.iteration || i + 1 }) }}
            </button>
            <span v-if="!viewingLatest" class="ml-1 text-[11px] text-warn">{{ t('pages.runDetail.historicalReadonly') }}</span>
          </div>
          <div v-if="canResumeSelected" class="flex shrink-0 items-center gap-2 border-b border-line bg-err/5 px-3 py-2">
            <Icon name="alert" :size="13" class="text-err" />
            <span class="text-[12px] text-txt2">{{ t('pages.runDetail.nodeFailed') }}</span>
            <div class="flex-1" />
            <AppButton variant="primary" size="sm" icon="refresh" :disabled="resuming" @click="onResume(selected!)">
              {{ resuming ? t('common.buttons.resuming') : t('common.buttons.resumeFromNode') }}
            </AppButton>
          </div>
          <div class="shrink-0 px-3 pt-2">
            <AppTabs :tabs="nodeTabs" v-model="nodeTab" @disabled-click="onNodeTabDisabledClick" />
          </div>
          <div
            class="relative min-h-0 flex-1"
            data-testid="run-detail-node-panel"
            :aria-busy="panelSwitching || sbxLogLoading ? 'true' : 'false'"
          >
            <RefreshStrip
              v-if="panelSwitching && !(nodeTab === 'gate' && run.gate)"
              data-testid="run-detail-panel-switching"
              :message="t('common.buttons.refreshing')"
            />
            <GateApproval
              v-if="nodeTab === 'gate' && run.gate"
              ref="gateApprovalRef"
              :gate="run.gate"
              :run="run"
              :fill-preview="true"
              :mobile-fill-remaining="true"
              :submit-error="gateError"
              @resolve="onGateResolve"
              @react-revised="loadRun(false)"
            />
            <template v-else-if="nodeTab === 'clarify'">
              <div v-if="clarifySandboxFailed" class="scroll-area flex h-full flex-col overflow-y-auto p-4">
                <div class="mb-3 flex items-center gap-2.5">
                  <div class="flex h-8 w-8 shrink-0 items-center justify-center rounded-md bg-n-clarify/15 text-n-clarify">
                    <Icon name="chat" :size="16" />
                  </div>
                  <div class="min-w-0 flex-1">
                    <div class="text-[14px] font-semibold text-txt [overflow-wrap:anywhere]">{{ selNodeDisplayLabel }}</div>
                    <div class="text-[11px] text-txt3 [overflow-wrap:anywhere]">{{ t('pages.runDetail.clarifyFailed.subtitle', { id: selNode!.id }) }}</div>
                  </div>
                  <StatusPill status="failed" />
                </div>
                <div class="mb-3 border border-err/40 bg-err/5 p-3.5">
                  <div class="mb-2 flex items-center gap-1.5 text-[11px] font-semibold uppercase tracking-wide text-err">
                    <Icon name="alert" :size="14" />
                    {{ t('pages.runDetail.clarifyFailed.errorTitle') }}
                  </div>
                  <pre class="min-w-0 max-w-full overflow-x-auto whitespace-pre font-mono text-[11px] leading-relaxed text-txt2">{{ selRun!.error }}</pre>
                </div>
                <p class="text-[11px] text-txt3">{{ t('pages.runDetail.clarifyFailed.hint') }}</p>
              </div>
              <ClarifyChat
                v-else-if="selClarify"
                ref="reviewChatRef"
                :run-id="run.id"
                :node-id="selClarify.nodeId"
                :iteration="selClarify.iteration ?? 1"
                v-model:draft="clarifyDraft"
                v-model:attachments="clarifyAttachments"
                :turns="selClarify.turns"
                :done="selClarify.done"
                :active="clarifyInputActive"
                @send="onClarifySend"
                @finish="onClarifyFinish"
                @cancel="onClarifyCancel"
              />
              <ClarifyBootLoader v-else :phase="selStatus === 'pending' ? 'pending' : 'starting'" />
            </template>
            <template v-else-if="nodeTab === 'review' && selNode && selRunView">
              <!-- Left product stage + right review sidebar (page.html RUN 复审 / app_preview VNC) -->
              <ReviewShell
                class="h-full min-h-0"
                :mobile="isMobile"
                :sidebar-width="REVIEW_SIDEBAR"
                :storage-key="REVIEW_SHELL_WIDTH_KEY_REVIEW"
              >
                <template #stage>
                  <div v-if="selNode.type === 'app_preview'" class="flex h-full min-h-0 flex-col p-3">
                    <AppPreviewPanel
                      :run-id="run.id"
                      :node-id="selNode.id"
                      fill
                      :show-feedback="false"
                      @pick="onAppPreviewReviewPick"
                      @staged-pick="onAppPreviewStagedPick"
                    />
                  </div>
                  <StructuredProductPanel v-else :node="selNode" :node-run="selRunView" :run="run" annotatable />
                </template>
                <template #sidebar>
                  <ReviewComposer
                    v-if="selClarify"
                    ref="reviewChatRef"
                    mode="review"
                    :run-id="run.id"
                    :node-id="selClarify.nodeId"
                    :iteration="selClarify.iteration ?? 1"
                    v-model:draft="clarifyDraft"
                    v-model:attachments="clarifyAttachments"
                    v-model:annotations="clarifyAnnotations"
                    :turns="selClarify.turns"
                    :done="selClarify.done"
                    :active="clarifyInputActive"
                    :confirm-error="clarifyConfirmError"
                    @send="onClarifySend"
                    @finish="onClarifyFinish"
                    @cancel="onClarifyCancel"
                  />
                  <ClarifyBootLoader v-else :phase="selStatus === 'pending' ? 'pending' : 'starting'" />
                </template>
              </ReviewShell>
            </template>
            <div v-else-if="nodeTab === 'preview' && selNode" class="flex h-full min-h-0 flex-col p-4">
              <AppPreviewPanel :run-id="runId" :node-id="selNode.id" fill />
            </div>
            <StructuredProductPanel v-else-if="nodeTab === 'product'" :node="selNode" :node-run="selRunView" :run="run" />
            <!-- Keep both panels mounted across log ↔ sandbox so boot timeout dwell survives CTA. -->
            <div v-else-if="nodeTab === 'log' || nodeTab === 'sandbox'" class="flex h-full min-h-0 flex-col">
              <LiveLogPanel
                :key="`${selected}:${selExecIdx}`"
                v-show="nodeTab === 'log'"
                class="min-h-0 flex-1"
                :events="logEvents"
                :live="logLive"
                :busy="logBusy"
                :status="selStatus"
                :mcp-calls="selMcpCalls"
                :has-more="logHasMore"
                :show-console="!!sandboxLookup"
                :sandbox-status="sandboxLookup?.status"
                :sandbox-container-status="sandboxLookup?.containerStatus"
                :boot-session="currentLiveLogBootSession"
                :rehydrate-status="selRehydrateStatus"
                @load-earlier="selected && loadEarlierEvents(selected)"
                @console-click="openSandboxConsole"
                @go-sandbox-log="goSandboxLogTab"
                @boot-session="onLiveLogBootSession"
                @retry-rehydrate="retryRehydrate"
              />
              <div v-show="nodeTab === 'sandbox'" class="relative flex h-full min-h-0 flex-col">
                <RefreshStrip v-if="sbxLogLoading && sbxLog?.content" :message="t('pages.sandboxConsole.logRefreshing')" />
                <HardLoadLayer
                  v-else-if="sbxLogLoading && !sbxLog?.content && !sbxLog?.error"
                  :overlay="true"
                  :stuck-after-ms="10_000"
                  :stage="t('common.loading.label')"
                  @retry="fetchSandboxLog(selected)"
                />
                <div class="flex items-center gap-2 border-b border-line px-3 py-1.5 text-[11px] text-txt3">
                  <Icon name="terminal" :size="12" />
                  <span>{{ t('pages.runDetail.sandboxLog.title') }}</span>
                  <span
                    v-if="sbxLog?.error"
                    class="inline-flex items-center rounded-full border border-err/40 bg-err/10 px-2 py-0.5 text-[10px] text-err"
                  >{{ t('pages.runDetail.sandboxLog.errorBadge') }}</span>
                  <span
                    v-else-if="sbxLog?.live"
                    class="inline-flex items-center rounded-full border border-accent/40 bg-accent-dim px-2 py-0.5 text-[10px] text-accent"
                  >{{ t('pages.runDetail.sandboxLog.live') }}</span>
                  <span v-else-if="sbxLog?.found" class="chip">{{ t('pages.runDetail.sandboxLog.archived') }}</span>
                  <div class="flex-1" />
                  <button
                    v-if="sandboxLookup"
                    type="button"
                    class="rounded border border-line px-2 py-1 text-[11px] text-txt2 hover:border-line-strong"
                    @click="openSandboxConsole"
                  >
                    <Icon name="terminal" :size="12" class="-mt-0.5 mr-0.5 inline" />{{ t('common.buttons.console') }}
                  </button>
                  <button class="text-txt3 hover:text-txt" :title="t('common.buttons.refresh')" @click="fetchSandboxLog(selected)"><Icon name="refresh" :size="12" /></button>
                </div>
                <div class="scroll-area min-h-0 flex-1 overflow-auto bg-base p-3">
                  <div
                    v-if="sbxLog?.error"
                    class="mb-2 rounded-lg border border-err/30 bg-err/10 px-3 py-2.5 text-[12px] text-err"
                    data-testid="sandbox-log-error"
                    role="alert"
                  >
                    <strong class="mb-1 block">{{ t('pages.runDetail.sandboxLog.errorTitle') }}</strong>
                    <span>{{ sbxLog.error }}</span>
                    <button
                      type="button"
                      class="mt-2 inline-flex min-h-11 items-center border border-line px-3 text-[12px] text-txt"
                      @click="fetchSandboxLog(selected)"
                    >
                      {{ t('common.chatImage.retry') }}
                    </button>
                  </div>
                  <pre
                    v-if="sbxLog?.found && sbxLog.content"
                    class="min-w-max whitespace-pre font-mono text-[11px] leading-relaxed text-txt2"
                  >{{ sbxLog.content }}</pre>
                  <pre
                    v-else-if="sbxLog?.found && sbxLog.live"
                    class="whitespace-pre-wrap font-mono text-[11px] leading-relaxed text-txt3"
                    data-testid="sandbox-log-live-empty"
                  >{{ t('pages.runDetail.sandboxLog.liveEmpty') }}</pre>
                  <div v-else class="flex h-full items-center justify-center text-center text-[12px] text-txt3">
                    <div>
                      <Icon name="terminal" :size="24" class="mx-auto mb-2 opacity-40" />
                      <p>{{ selStatus === 'pending' ? t('pages.runDetail.sandboxLog.pending') : t('pages.runDetail.sandboxLog.empty') }}</p>
                    </div>
                  </div>
                </div>
              </div>
            </div>
            <NodeOutputPanel v-else :node="selNode" :node-run="selRunView" :run="run" />
          </div>
        </template>
        <EmptyState v-else :title="t('common.empty.selectNode')" :desc="t('common.empty.selectNodeDesc')" />
      </div>
      </template>
      </div>

      <div v-if="runLoading || loadError" class="absolute inset-0 z-10 bg-surface">
        <HardLoadLayer
          v-if="runLoading"
          :overlay="false"
          :stuck-after-ms="10_000"
          :stage="t('pages.runDetail.loadingRun')"
          @retry="loadRun(true)"
        />
        <EmptyState
          v-else
          icon="alert"
          :title="loadErrorKind === 'not_found' ? t('pages.runDetail.notFoundTitle') : t('pages.runDetail.loadFailedTitle')"
          :desc="loadErrorKind === 'not_found' ? t('pages.runDetail.notFoundDesc') : t('pages.runDetail.loadFailedDesc')"
          data-testid="run-load-error"
        >
          <AppButton
            v-if="loadErrorKind === 'not_found'"
            variant="outline"
            size="sm"
            disabled
            data-testid="run-retry-unavailable"
          >
            {{ t('pages.runDetail.retryUnavailable') }}
          </AppButton>
          <AppButton
            v-else
            variant="primary"
            size="sm"
            icon="refresh"
            data-testid="run-retry"
            @click="loadRun(true)"
          >
            {{ t('pages.runDetail.retry') }}
          </AppButton>
        </EmptyState>
      </div>
    </div>

    <AppDrawer :open="showDetail" :title="t('pages.runDetail.detailTitle')" :width="480" @close="showDetail = false">
      <div class="flex h-full flex-col">
        <div class="px-3 pt-2"><AppTabs :tabs="detailTabs" v-model="detailTab" /></div>
        <div class="min-h-0 flex-1">
          <StateTracePanel v-if="detailTab === 'trace'" :trace="run.trace || []" />
          <VariablesPanel v-else-if="detailTab === 'vars'" :vars="run.vars || []" />
          <RunSandboxEnvPanel
            v-else-if="detailTab === 'sandboxEnv'"
            :entries="run.sandboxEnv || []"
          />
          <ArtifactPanel
            v-else-if="detailTab === 'artifacts'"
            :artifacts="run.artifacts"
            @deleted="onArtifactDeleted"
          />
        </div>
      </div>
    </AppDrawer>

    <AppModal
      :open="showCancelConfirm"
      :title="t('pages.runDetail.cancelTitle')"
      :width="440"
      @close="closeCancelConfirm"
    >
      <div class="space-y-3 text-sm text-txt2">
        <div class="flex items-start gap-2 rounded-md border border-err/30 bg-err/10 px-3 py-2 text-[12px] text-err">
          <Icon name="alert" :size="14" class="mt-0.5 shrink-0" />
          {{ t('pages.runDetail.cancelWarning') }}
        </div>
        <p>{{ t('pages.runDetail.cancelConfirm') }}</p>
        <div
          v-if="cancelRunError"
          class="flex items-start gap-2 rounded-md border border-err/30 bg-err/10 px-3 py-2 text-[12px] text-err"
          role="alert"
          data-testid="cancel-run-error"
        >
          <Icon name="alert" :size="14" class="mt-0.5" />{{ cancelRunError }}
        </div>
      </div>
      <template #footer>
        <AppButton variant="ghost" :disabled="cancellingRun" @click="closeCancelConfirm">
          {{ t('common.buttons.cancel') }}
        </AppButton>
        <AppButton
          data-testid="confirm-cancel-run-btn"
          variant="danger"
          :icon="cancellingRun ? 'spinner' : 'close'"
          :disabled="cancellingRun"
          :aria-busy="cancellingRun ? 'true' : 'false'"
          @click="confirmCancelRun"
        >
          {{ cancellingRun ? t('common.buttons.cancelling') : t('common.buttons.confirmCancelRun') }}
        </AppButton>
      </template>
    </AppModal>

    <AppModal
      :open="showDeleteConfirm"
      :title="t('pages.runDetail.deleteTitle')"
      :width="440"
      @close="closeDeleteConfirm"
    >
      <div class="space-y-3 text-sm text-txt2">
        <div class="flex items-start gap-2 rounded-md border border-err/30 bg-err/10 px-3 py-2 text-[12px] text-err">
          <Icon name="alert" :size="14" class="mt-0.5 shrink-0" />
          {{ t('pages.runDetail.deleteWarning') }}
        </div>
        <p>{{ t('pages.runDetail.deleteConfirm') }}</p>
        <div
          v-if="deleteRunError"
          class="flex items-start gap-2 rounded-md border border-err/30 bg-err/10 px-3 py-2 text-[12px] text-err"
        >
          <Icon name="alert" :size="14" class="mt-0.5" />{{ deleteRunError }}
        </div>
      </div>
      <template #footer>
        <AppButton variant="ghost" :disabled="deletingRun" @click="closeDeleteConfirm">
          {{ t('common.buttons.cancel') }}
        </AppButton>
        <AppButton
          data-testid="confirm-delete-run-btn"
          variant="danger"
          icon="trash"
          :disabled="deletingRun"
          @click="confirmDeleteRun"
        >
          {{ deletingRun ? t('common.buttons.deleting') : t('common.buttons.confirmDelete') }}
        </AppButton>
      </template>
    </AppModal>
  </div>
</template>
