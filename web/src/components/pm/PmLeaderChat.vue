<script setup lang="ts">
import { ref, computed, onMounted, onBeforeUnmount, watch, nextTick } from 'vue'
import { useI18n } from 'vue-i18n'
import AppButton from '@/components/ui/AppButton.vue'
import Icon from '@/components/ui/Icon.vue'
import CitationCard from '@/components/pm/CitationCard.vue'
import ArtifactLoadingPane from '@/components/run/ArtifactLoadingPane.vue'
import EmptyState from '@/components/ui/EmptyState.vue'
import { relTime } from '@/lib/format'
import { renderMarkdown } from '@/lib/markdown'
import { api } from '@/lib/api'
import { imgSrc } from '@/lib/compositeText'
import { useImageAttachments } from '@/lib/useImageAttachments'
import { useToast } from '@/lib/useToast'
import {
  classifyPmTurnError,
  findConvergableOrphanIds,
  isPmFailKind,
  pmActiveThreadStorageKey,
  shouldApplyEventSeq,
  type PmFailKind,
} from '@/lib/pmTurnState'
import { extractAgentMessageDelta } from '@/lib/acpUnpack'
import { useBreakpoint } from '@/lib/useBreakpoint'
import type { ChatMessage, ChatThread, ClarifyImage, PmLeaderBinding } from '@/lib/types'

type FailKind = PmFailKind

const props = defineProps<{
  projectId: string
  binding: PmLeaderBinding | null
  restoreMobileChat?: boolean
}>()
const emit = defineEmits<{ openSettings: []; restoredMobileChat: [] }>()

const { t } = useI18n()
const toast = useToast()
const { isMobile } = useBreakpoint()
const mobileView = ref<'threads' | 'chat'>('threads')

const threads = ref<ChatThread[]>([])
const activeId = ref('')
const messages = ref<ChatMessage[]>([])
const input = ref('')
const loading = ref(true)
const messagesLoading = ref(false)
const messagesLoadFailed = ref(false)
const finalizingRefetchFailed = ref(false)
const finalizing = ref(false)
const sending = ref(false)
const streaming = ref(false)
const streamText = ref('')
const resuming = ref(false)
const lastEventSeq = ref(-1)
const scroller = ref<HTMLElement | null>(null)

const STICK_THRESHOLD = 48
/** Align with approved Demo: near-top auto lazyload threshold (px). */
const TOP_THRESHOLD = 56
/** Fixed page size for non-Channel PM sessions (message rows). */
const PAGE_SIZE = 20
const stickToBottom = ref(true)
/** True when older messages remain beyond the loaded window (non-Channel only). */
const hasMoreEarlier = ref(false)
const historyLoading = ref(false)
const historyLoadFailed = ref(false)

function isNearBottom(el: HTMLElement) {
  return el.scrollHeight - el.scrollTop - el.clientHeight <= STICK_THRESHOLD
}

function onScrollerScroll() {
  const el = scroller.value
  if (!el) return
  stickToBottom.value = isNearBottom(el)
  if (el.scrollTop <= TOP_THRESHOLD) {
    void loadEarlier()
  }
}

function scrollBottom(force = false) {
  const el = scroller.value
  if (el && (force || stickToBottom.value)) {
    el.scrollTop = el.scrollHeight
  }
}

/** Keep already-loaded prefix; update/append by message id (never shrink to latest PAGE). */
function mergeMessagesKeepPrefix(existing: ChatMessage[], incoming: ChatMessage[]): ChatMessage[] {
  if (!incoming.length) return existing
  if (!existing.length) return incoming.slice()
  const byId = new Map(existing.map((m) => [m.id, m]))
  for (const m of incoming) {
    byId.set(m.id, m)
  }
  const seen = new Set(existing.map((m) => m.id))
  const out = existing.map((m) => byId.get(m.id)!)
  for (const m of incoming) {
    if (!seen.has(m.id)) {
      out.push(m)
      seen.add(m.id)
    }
  }
  return out
}

function resetHistoryWindowState() {
  hasMoreEarlier.value = false
  historyLoading.value = false
  historyLoadFailed.value = false
}

/** Session-only failed partial bubbles (S2); not persisted as assistant rows. */
const failedPartialByUserMsgId = ref<Record<string, string>>({})

/** Current turn's user message id (for fail/retry persistence). */
const activeUserMessageId = ref('')

const {
  attachments,
  fileInput,
  onPickFiles,
  onPaste,
  removeAttachment,
  takeAttachments,
} = useImageAttachments()

let ws: WebSocket | null = null
let sandboxId = 0
/** When true, ignore a late turn_done after user cancelled / failTurn. */
let streamCancelled = false
/** Generation token so late async work from a previous turn is ignored. */
let turnGen = 0
/** Prevents duplicate failTurn / late WS handlers after a turn already finished. */
let turnClosed = false
let turnDeadlineTimer: ReturnType<typeof setTimeout> | null = null
let readyAbort: AbortController | null = null
/** Discard stale listPmMessages responses after thread switch or re-load. */
let threadLoadGen = 0

const TURN_DEADLINE_MS = 90_000
const WS_OPEN_TIMEOUT_MS = 10_000

const enabled = computed(() => !!props.binding?.enabled && props.binding.agentAvailable)
const turnBusy = computed(
  () => sending.value || streaming.value || finalizing.value || resuming.value,
)
const busy = computed(() => turnBusy.value || messagesLoading.value)
const canSend = computed(
  () => !busy.value && (!!input.value.trim() || attachments.value.length > 0),
)
const showStreamBubble = computed(
  () => sending.value || streaming.value || finalizing.value || resuming.value,
)
/** Main pane priority: finalizing > resuming > messagesLoading > errorEmpty > content */
const mainViewState = computed(() => {
  if (finalizing.value) return 'finalizing'
  if (resuming.value) return 'resuming'
  if (messagesLoading.value) return 'messagesLoading'
  if (messagesLoadFailed.value) return 'errorEmpty'
  return 'content'
})
const busyHint = computed(() => {
  if (finalizing.value) return t('pages.projectDetail.pm.busyFinalizing')
  if (resuming.value) return t('pages.projectDetail.pm.busyResuming')
  if (streaming.value && streamText.value) return t('pages.projectDetail.pm.busyStreaming')
  return t('pages.projectDetail.pm.busyWaiting')
})
const suggestions = computed(() => [
  t('pages.projectDetail.pm.suggestProgress'),
  t('pages.projectDetail.pm.suggestBlockers'),
  t('pages.projectDetail.pm.suggestRisk'),
])
const showStreamTypingDots = computed(
  () => showStreamBubble.value && !streamText.value && !finalizing.value && !resuming.value,
)

async function copyAssistantText(ev: Event) {
  const root = (ev.currentTarget as HTMLElement | null)?.closest('[data-assistant-bubble]')
  const md = root?.querySelector('.md') as HTMLElement | null
  if (!md) return
  const text = md.innerText.trim()
  if (!text) return
  try {
    await navigator.clipboard.writeText(text)
    toast.success(t('pages.projectDetail.pm.copySuccess'))
  } catch {
    toast.error(t('pages.projectDetail.pm.copyFailed'))
  }
}
const showThreadsAside = computed(() => !isMobile.value || mobileView.value === 'threads')
const showChatSection = computed(() => !isMobile.value || mobileView.value === 'chat')

/** QQ Channel synthetic user id, e.g. qq:guild:… / qq:group:… / qq:c2c:… */
function isChannelThread(th: ChatThread | undefined | null): boolean {
  return !!th?.userId?.startsWith('qq:')
}

function threadDisplayTitle(th: ChatThread | undefined | null): string {
  const title = (th?.title || '').trim()
  if (title) return title
  if (isChannelThread(th)) return t('pages.projectDetail.pm.channelUntitled')
  return t('pages.projectDetail.pm.untitled')
}

const activeThread = computed(() => threads.value.find((x) => x.id === activeId.value))
const activeIsChannel = computed(() => isChannelThread(activeThread.value))
const activeThreadTitle = computed(() => threadDisplayTitle(activeThread.value))
const showEmptyHint = computed(
  () =>
    mainViewState.value === 'content' &&
    !activeIsChannel.value &&
    !messages.value.length &&
    !showStreamBubble.value,
)
/** Top non-button history tip (Demo four-state); Channel never shows it. */
const showHistoryTip = computed(
  () =>
    !activeIsChannel.value &&
    mainViewState.value === 'content' &&
    !showEmptyHint.value &&
    (messages.value.length > 0 || historyLoading.value || historyLoadFailed.value),
)
const historyTipText = computed(() => {
  if (historyLoading.value) return t('pages.projectDetail.pm.historyLoading')
  if (historyLoadFailed.value) return t('pages.projectDetail.pm.historyLoadFailed')
  if (!hasMoreEarlier.value) return t('pages.projectDetail.pm.historyReachedStart')
  return t('pages.projectDetail.pm.historyScrollUp')
})
const historyTipClass = computed(() => {
  if (historyLoading.value) return 'text-accent'
  if (historyLoadFailed.value) return 'text-err'
  return 'text-txt3'
})
const showIdleSuggestions = computed(() => {
  if (activeIsChannel.value) return false
  const msgs = messages.value
  if (!msgs.length || busy.value || showStreamBubble.value) return false
  return msgs[msgs.length - 1]?.role === 'assistant'
})

type ChannelCtx = { open: true; x: number; y: number; threadId: string }
const channelCtx = ref<ChannelCtx | null>(null)
const channelDetailOpen = ref(false)
const channelDetailTitle = ref('')

function closeChannelCtx() {
  channelCtx.value = null
}

function openChannelCtx(e: MouseEvent, th: ChatThread) {
  if (!isChannelThread(th)) return
  e.preventDefault()
  e.stopPropagation()
  channelCtx.value = { open: true, x: e.clientX, y: e.clientY, threadId: th.id }
}

function openChannelDetail() {
  if (!channelCtx.value) return
  const th = threads.value.find((x) => x.id === channelCtx.value!.threadId)
  channelDetailTitle.value = threadDisplayTitle(th)
  channelDetailOpen.value = true
  closeChannelCtx()
}

function closeChannelDetail() {
  channelDetailOpen.value = false
}

function onChannelCtxAction() {
  openChannelDetail()
}

const FAIL_KIND_KEYS: Record<FailKind, { title: string; desc: string }> = {
  connection: { title: 'failConnectionTitle', desc: 'failConnectionDesc' },
  sandbox: { title: 'failSandboxTitle', desc: 'failSandboxDesc' },
  empty: { title: 'failEmptyTitle', desc: 'failEmptyDesc' },
  unknown: { title: 'failUnknownTitle', desc: 'failUnknownDesc' },
  stopped: { title: 'failStoppedTitle', desc: 'failStoppedDesc' },
}

function failMeta(kind: string) {
  const k = (isPmFailKind(kind) ? kind : 'unknown') as FailKind
  const keys = FAIL_KIND_KEYS[k]
  return {
    kind: k,
    title: t(`pages.projectDetail.pm.${keys.title}`),
    desc: t(`pages.projectDetail.pm.${keys.desc}`),
  }
}

function applyRestoreMobileChat() {
  if (props.restoreMobileChat && isMobile.value && activeId.value) {
    mobileView.value = 'chat'
    emit('restoredMobileChat')
  }
}

function isFailedUser(m: ChatMessage) {
  return m.role === 'user' && m.status === 'failed'
}

async function loadThreads() {
  loading.value = true
  try {
    const res = await api.listPmThreads(props.projectId)
    threads.value = res.items || []
    const stored =
      typeof localStorage !== 'undefined'
        ? localStorage.getItem(pmActiveThreadStorageKey(props.projectId)) || ''
        : ''
    const preferred =
      (stored && threads.value.some((t) => t.id === stored) && stored) ||
      activeId.value ||
      (threads.value[0]?.id ?? '')
    if (!activeId.value && preferred) {
      await activateThread(preferred)
    } else if (activeId.value) {
      await loadMessages(activeId.value)
    }
  } catch (e: any) {
    toast.error(String(e?.message || e))
  } finally {
    loading.value = false
  }
}

/**
 * Bind UI to a thread. When preserveTurn is true (send-path ensure), do NOT bump
 * turnGen / turnClosed — otherwise a mid-send newThread would silently abort the turn.
 */
async function activateThread(id: string, opts?: { preserveTurn?: boolean }) {
  threadLoadGen += 1
  if (!opts?.preserveTurn) {
    resetTurnLocal()
  }
  activeId.value = id
  messages.value = []
  messagesLoadFailed.value = false
  resetHistoryWindowState()
  try {
    if (typeof localStorage !== 'undefined') {
      localStorage.setItem(pmActiveThreadStorageKey(props.projectId), id)
    }
  } catch {
    /* ignore quota */
  }
  closeWs()
  sandboxId = 0
  await loadMessages(id)
}

async function selectThread(id: string) {
  if (turnBusy.value) return
  if (id === activeId.value) {
    if (isMobile.value) mobileView.value = 'chat'
    return
  }
  await activateThread(id)
  if (isMobile.value) mobileView.value = 'chat'
}

function backToThreads() {
  if (turnBusy.value) return
  mobileView.value = 'threads'
}

/** Create + activate a thread without resetting an in-flight send/retry generation. */
async function ensureActiveThread() {
  if (activeId.value) return
  const thr = await api.createPmThread(props.projectId)
  threads.value = [thr, ...threads.value]
  await activateThread(thr.id, { preserveTurn: true })
}

async function loadMessages(tid: string) {
  const gen = ++threadLoadGen
  messagesLoadFailed.value = false
  messagesLoading.value = true
  historyLoadFailed.value = false
  historyLoading.value = false
  try {
    const th = threads.value.find((x) => x.id === tid)
    const channel = isChannelThread(th)
    // Channel: keep full-list strategy. Non-Channel: newest-tail window of PAGE_SIZE.
    const res = channel
      ? await api.listPmMessages(props.projectId, tid)
      : await api.listPmMessages(props.projectId, tid, { limit: PAGE_SIZE })
    if (gen !== threadLoadGen || tid !== activeId.value) return
    messages.value = res.items || []
    hasMoreEarlier.value = channel ? false : !!res.hasMore
    await hydrateDraftAndMaybeResume(tid)
    if (gen !== threadLoadGen || tid !== activeId.value) return
  } catch (e: any) {
    if (gen !== threadLoadGen || tid !== activeId.value) return
    messagesLoadFailed.value = true
    toast.error(t('pages.projectDetail.pm.loadFailed'))
  } finally {
    if (gen === threadLoadGen && tid === activeId.value) {
      // Clear loading before scrollBottom: while messagesLoading, template shows
      // ArtifactLoadingPane and scrollHeight is not the message list.
      messagesLoading.value = false
    }
  }
  // Force stick-to-bottom only after content replaces the loading pane.
  if (gen !== threadLoadGen || tid !== activeId.value || messagesLoadFailed.value) return
  stickToBottom.value = true
  await nextTick()
  if (gen !== threadLoadGen || tid !== activeId.value) return
  scrollBottom(true)
  // Browser layout can lag one frame behind the v-if swap; re-pin after paint.
  requestAnimationFrame(() => {
    if (gen !== threadLoadGen || tid !== activeId.value) return
    scrollBottom(true)
  })
}

/**
 * Near-top lazyload: prepend up to PAGE_SIZE older messages.
 * Uses threadLoadGen so a late response cannot write into another session.
 */
async function loadEarlier() {
  if (
    historyLoading.value ||
    !hasMoreEarlier.value ||
    activeIsChannel.value ||
    !activeId.value ||
    messagesLoading.value ||
    !messages.value.length
  ) {
    return
  }
  const gen = threadLoadGen
  const tid = activeId.value
  const before = messages.value[0]?.id
  if (!before) return

  historyLoading.value = true
  historyLoadFailed.value = false
  const el = scroller.value
  const prevTop = el?.scrollTop ?? 0
  const prevHeight = el?.scrollHeight ?? 0

  try {
    const res = await api.listPmMessages(props.projectId, tid, { limit: PAGE_SIZE, before })
    if (gen !== threadLoadGen || tid !== activeId.value) return
    const older = res.items || []
    if (older.length) {
      const existing = new Set(messages.value.map((m) => m.id))
      const prepend = older.filter((m) => !existing.has(m.id))
      messages.value = [...prepend, ...messages.value]
    }
    hasMoreEarlier.value = !!res.hasMore
    stickToBottom.value = false
    await nextTick()
    if (el) {
      el.scrollTop = prevTop + (el.scrollHeight - prevHeight)
    }
  } catch {
    if (gen !== threadLoadGen || tid !== activeId.value) return
    historyLoadFailed.value = true
  } finally {
    if (gen === threadLoadGen && tid === activeId.value) {
      historyLoading.value = false
    }
  }
}

async function retryLoadMessages() {
  if (!activeId.value || messagesLoading.value) return
  await loadMessages(activeId.value)
}

/**
 * Hydrate priority: hasFinal > streaming+live resume > streaming+!live|failed converge > orphan.
 * Never call beginResume when live=false.
 */
async function hydrateDraftAndMaybeResume(tid: string) {
  if (tid !== activeId.value) return
  // Channel threads are Web-readonly: never resume turns or persist fail metadata.
  const th = threads.value.find((x) => x.id === tid)
  if (isChannelThread(th)) return
  let draftUserMsgId = ''
  let skipOrphans = false
  try {
    const info = await api.getPmDraft(props.projectId, tid)
    const draft = info.draft
    // hasFinal alone skips resume/orphan for this hydrate (draft may already be cleared).
    if (info.hasFinal) {
      skipOrphans = true
      return
    }
    if (draft && draft.status === 'streaming' && draft.userMsgId) {
      draftUserMsgId = draft.userMsgId
      if (info.live) {
        skipOrphans = true
        // Already busy with a live send in this session — don't double-resume.
        if (busy.value && activeUserMessageId.value === draft.userMsgId) return
        await beginResume(tid, draft.partialText || '', draft.eventSeq ?? -1, draft.userMsgId)
        return
      }
      // Dead streaming draft (older server without reconcile): connection + optional partial.
      await convergeFailedDraft(draft.userMsgId, 'connection', draft.partialText || '')
      return
    }
    if (draft && draft.status === 'failed' && draft.userMsgId) {
      // Exclude this turn from orphan converge; still ensure fail card + Retry.
      draftUserMsgId = draft.userMsgId
      const raw = draft.failKind || ''
      const kind = (isPmFailKind(raw) ? raw : 'connection') as FailKind
      // Refresh/interrupt path must not stay on unknown when draft is already failed.
      const resolved: FailKind = kind === 'unknown' ? 'connection' : kind
      await convergeFailedDraft(draft.userMsgId, resolved, draft.partialText || '')
    }
  } catch {
    /* draft fetch failed → orphan path uses connection (not unknown) */
  } finally {
    await convergeOrphanTurns(tid, { draftUserMsgId, skipAll: skipOrphans })
  }
}

function setFailedPartial(userMsgId: string, partial: string) {
  const text = partial.trim()
  if (!userMsgId || !text) return
  failedPartialByUserMsgId.value = { ...failedPartialByUserMsgId.value, [userMsgId]: partial }
}

function clearFailedPartial(userMsgId: string) {
  if (!(userMsgId in failedPartialByUserMsgId.value)) return
  const next = { ...failedPartialByUserMsgId.value }
  delete next[userMsgId]
  failedPartialByUserMsgId.value = next
}

async function convergeFailedDraft(userMsgId: string, kind: FailKind, partial: string) {
  if (partial.trim()) setFailedPartial(userMsgId, partial)
  else clearFailedPartial(userMsgId)
  const existing = messages.value.find((m) => m.id === userMsgId)
  if (existing && existing.status !== 'failed') {
    await persistFailure(userMsgId, kind)
  } else if (existing && existing.status === 'failed' && existing.failKind !== kind) {
    await persistFailure(userMsgId, kind)
  }
}

async function beginResume(tid: string, partial: string, afterSeq: number, userMsgId: string) {
  if (tid !== activeId.value) return
  const gen = ++turnGen
  streamCancelled = false
  turnClosed = false
  activeUserMessageId.value = userMsgId
  sending.value = true
  streaming.value = true
  resuming.value = true
  streamText.value = partial
  lastEventSeq.value = afterSeq
  startTurnDeadline(gen)
  readyAbort = new AbortController()
  const signal = readyAbort.signal
  try {
    await ensureSandbox(false, signal)
    if (gen !== turnGen || streamCancelled || turnClosed) return
    startTurnDeadline(gen)
    if (!ws || ws.readyState !== WebSocket.OPEN) {
      throw Object.assign(new Error('ws not open'), { failKind: 'connection' as FailKind })
    }
    ws.send(JSON.stringify({ type: 'resume', afterSeq }))
  } catch (e: any) {
    resuming.value = false
    if (gen !== turnGen || turnClosed) return
    if (e?.name === 'AbortError' || streamCancelled) {
      if (!turnClosed) await failTurn('stopped')
      return
    }
    await failTurn(classifyPmTurnError(e))
    const detail = String(e?.message || e)
    if (detail) toast.error(detail)
  } finally {
    if (readyAbort?.signal === signal) readyAbort = null
  }
}

/**
 * Only converge truly unrecoverable orphans (no draft / no live turn).
 * Default failKind is connection (refresh/interrupt), never unknown.
 */
async function convergeOrphanTurns(
  tid: string,
  opts?: { draftUserMsgId?: string; skipAll?: boolean },
) {
  if (tid !== activeId.value) return
  const orphans = findConvergableOrphanIds(messages.value, opts).filter((id) => {
    if (busy.value && id === activeUserMessageId.value) return false
    return true
  })
  for (const mid of orphans) {
    if (tid !== activeId.value) return
    try {
      const msg = await api.patchPmMessage(props.projectId, tid, mid, {
        status: 'failed',
        failKind: 'connection',
      })
      patchLocalMessage(mid, {
        status: msg.status || 'failed',
        failKind: msg.failKind || 'connection',
      })
    } catch (e: any) {
      patchLocalMessage(mid, { status: 'failed', failKind: 'connection' })
      toast.error(String(e?.message || e))
    }
  }
}

async function newThread() {
  if (!enabled.value) {
    emit('openSettings')
    return
  }
  if (turnBusy.value) return
  try {
    const thr = await api.createPmThread(props.projectId)
    threads.value = [thr, ...threads.value]
    await activateThread(thr.id)
    if (isMobile.value) mobileView.value = 'chat'
  } catch (e: any) {
    toast.error(String(e?.message || e))
  }
}

async function removeThread(id: string) {
  if (turnBusy.value) return
  const th = threads.value.find((x) => x.id === id)
  if (isChannelThread(th)) return
  try {
    await api.deletePmThread(props.projectId, id)
    threads.value = threads.value.filter((x) => x.id !== id)
    if (activeId.value === id) {
      activeId.value = ''
      messages.value = []
      resetTurnLocal()
      closeWs()
      if (threads.value[0]) await activateThread(threads.value[0].id)
    }
  } catch (e: any) {
    toast.error(String(e?.message || e))
  }
}

function closeWs() {
  if (ws) {
    try {
      ws.close()
    } catch {
      /* ignore */
    }
    ws = null
  }
}

function clearTurnDeadline() {
  if (turnDeadlineTimer) {
    clearTimeout(turnDeadlineTimer)
    turnDeadlineTimer = null
  }
}

function startTurnDeadline(gen: number) {
  clearTurnDeadline()
  turnDeadlineTimer = setTimeout(() => {
    if (gen !== turnGen) return
    if (!busy.value) return
    void failTurn('sandbox')
  }, TURN_DEADLINE_MS)
}

function resetTurnLocal() {
  turnGen += 1
  streamCancelled = true
  turnClosed = true
  clearTurnDeadline()
  if (readyAbort) {
    readyAbort.abort()
    readyAbort = null
  }
  streamText.value = ''
  streaming.value = false
  sending.value = false
  resuming.value = false
  finalizing.value = false
  lastEventSeq.value = -1
  activeUserMessageId.value = ''
  failedPartialByUserMsgId.value = {}
}

function patchLocalMessage(mid: string, patch: Partial<ChatMessage>) {
  messages.value = messages.value.map((m) => (m.id === mid ? { ...m, ...patch } : m))
}

async function persistFailure(mid: string, kind: FailKind) {
  if (!activeId.value || !mid) return
  try {
    const msg = await api.patchPmMessage(props.projectId, activeId.value, mid, {
      status: 'failed',
      failKind: kind,
    })
    patchLocalMessage(mid, { status: msg.status || 'failed', failKind: msg.failKind || kind })
  } catch (e: any) {
    // Still show session-local failure even if persistence fails.
    patchLocalMessage(mid, { status: 'failed', failKind: kind })
    toast.error(String(e?.message || e))
  }
}

async function clearFailure(mid: string) {
  if (!activeId.value || !mid) return
  try {
    const msg = await api.patchPmMessage(props.projectId, activeId.value, mid, { status: 'ok' })
    patchLocalMessage(mid, { status: msg.status || 'ok', failKind: '' })
  } catch (e: any) {
    patchLocalMessage(mid, { status: 'ok', failKind: '' })
    toast.error(String(e?.message || e))
  }
}

/**
 * Unified turn failure finish: discard stream half-product, persist failKind on user turn.
 */
async function failTurn(kind: FailKind) {
  if (turnClosed) return
  turnClosed = true
  const mid = activeUserMessageId.value
  streamCancelled = true
  clearTurnDeadline()
  if (readyAbort) {
    readyAbort.abort()
    readyAbort = null
  }
  streamText.value = ''
  streaming.value = false
  sending.value = false
  resuming.value = false
  finalizing.value = false
  if (mid) {
    await persistFailure(mid, kind)
  }
  await nextTick()
  scrollBottom()
}

async function ensureSandbox(injectHistory: boolean, signal?: AbortSignal) {
  if (!activeId.value) {
    // Must not reset turnGen — caller (runTurn) already owns the busy lock.
    await ensureActiveThread()
  }
  if (!activeId.value) throw new Error('no thread')
  if (signal?.aborted) throw new DOMException('Aborted', 'AbortError')
  const res = await api.ensurePmSandbox(props.projectId, activeId.value, {
    injectHistory,
  })
  if (signal?.aborted) throw new DOMException('Aborted', 'AbortError')
  sandboxId = res.sandbox.id
  await waitReady(sandboxId, signal)
  await openWs(sandboxId)
  return res.preamble || ''
}

const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms))

async function waitReady(id: number, signal?: AbortSignal) {
  for (let i = 0; i < 90; i++) {
    if (signal?.aborted) throw new DOMException('Aborted', 'AbortError')
    const s = await api.getSandbox(id)
    if (signal?.aborted) throw new DOMException('Aborted', 'AbortError')
    if (s.status === 'running') return
    if (s.status === 'error') throw Object.assign(new Error(s.error || 'sandbox error'), { failKind: 'unknown' as FailKind })
    await sleep(1000)
  }
  throw Object.assign(new Error('sandbox timeout'), { failKind: 'sandbox' as FailKind })
}

function waitWsOpen(socket: WebSocket, timeoutMs: number): Promise<void> {
  if (socket.readyState === WebSocket.OPEN) return Promise.resolve()
  if (socket.readyState === WebSocket.CLOSING || socket.readyState === WebSocket.CLOSED) {
    return Promise.reject(Object.assign(new Error('ws closed'), { failKind: 'connection' as FailKind }))
  }
  return new Promise((resolve, reject) => {
    const timer = setTimeout(() => {
      cleanup()
      reject(Object.assign(new Error('ws open timeout'), { failKind: 'connection' as FailKind }))
    }, timeoutMs)
    const onOpen = () => {
      cleanup()
      resolve()
    }
    const onErr = () => {
      cleanup()
      reject(Object.assign(new Error('ws error'), { failKind: 'connection' as FailKind }))
    }
    const onClose = () => {
      cleanup()
      reject(Object.assign(new Error('ws closed'), { failKind: 'connection' as FailKind }))
    }
    const cleanup = () => {
      clearTimeout(timer)
      socket.removeEventListener('open', onOpen)
      socket.removeEventListener('error', onErr)
      socket.removeEventListener('close', onClose)
    }
    socket.addEventListener('open', onOpen)
    socket.addEventListener('error', onErr)
    socket.addEventListener('close', onClose)
  })
}

async function openWs(_id: number) {
  closeWs()
  if (!activeId.value) throw new Error('no thread')
  const socket = new WebSocket(api.pmThreadChatWsUrl(props.projectId, activeId.value))
  ws = socket
  socket.onmessage = (ev) => {
    let msg: any
    try {
      msg = JSON.parse(ev.data)
    } catch {
      return
    }
    // Dedup overlapping fan-out / resume catch-up by seq (exclusive watermark).
    if (typeof msg.seq === 'number') {
      if (!shouldApplyEventSeq(msg.seq, lastEventSeq.value)) return
      lastEventSeq.value = msg.seq
    }
    if (msg.type === 'acp' && msg.data) {
      handleAcp(msg.data)
    } else if (msg.type === 'resume_hint') {
      // Absolute snapshot from server draft — never append onto stale streamText.
      if (typeof msg.partialText === 'string') {
        streamText.value = msg.partialText
      }
      if (typeof msg.eventSeq === 'number') {
        lastEventSeq.value = msg.eventSeq
      }
      if (msg.userMsgId) activeUserMessageId.value = msg.userMsgId
      resuming.value = true
    } else if (msg.type === 'turn_done') {
      void onTurnDone()
    } else if (msg.type === 'error') {
      const kind = (isPmFailKind(msg.failKind) ? msg.failKind : 'unknown') as FailKind
      void onTurnError(kind, String(msg.error || msg.message || 'error'))
    }
  }
  socket.onerror = () => {
    if (!turnClosed && busy.value && !streamCancelled) {
      void failTurn('connection')
    }
  }
  socket.onclose = () => {
    if (ws === socket) ws = null
    // WS disconnect must NOT fail the turn — server turn runner continues.
  }
  await waitWsOpen(socket, WS_OPEN_TIMEOUT_MS)
}

function handleAcp(raw: any) {
  if (streamCancelled) return
  const delta = extractAgentMessageDelta(raw)
  if (!delta?.text) return
  streamText.value += delta.text
  void nextTick().then(() => scrollBottom())
}

function clearFinalizingStream() {
  finalizing.value = false
  finalizingRefetchFailed.value = false
  streamText.value = ''
  activeUserMessageId.value = ''
  resuming.value = false
}

async function refetchAfterTurnDone() {
  if (!activeId.value) return
  finalizingRefetchFailed.value = false
  const tid = activeId.value
  const gen = threadLoadGen
  try {
    const channel = isChannelThread(threads.value.find((x) => x.id === tid))
    // Channel: full list replace. Non-Channel: tail refetch + merge keep prefix.
    const res = channel
      ? await api.listPmMessages(props.projectId, tid)
      : await api.listPmMessages(props.projectId, tid, { limit: PAGE_SIZE })
    if (gen !== threadLoadGen || tid !== activeId.value) return
    const incoming = res.items || []
    messages.value = channel
      ? incoming
      : mergeMessagesKeepPrefix(messages.value, incoming)
    if (!channel && typeof res.hasMore === 'boolean' && messages.value.length <= PAGE_SIZE) {
      // Only trust hasMore when we have not prepended beyond the tail window.
      hasMoreEarlier.value = res.hasMore
    }
    const thr = await api.listPmThreads(props.projectId)
    if (gen !== threadLoadGen || tid !== activeId.value) return
    threads.value = thr.items || []
    await nextTick()
    scrollBottom()
    clearFinalizingStream()
  } catch {
    if (gen !== threadLoadGen || tid !== activeId.value) return
    finalizingRefetchFailed.value = true
    toast.error(t('pages.projectDetail.pm.loadFailed'))
  }
}

/**
 * Server already finalized (assistant appended or failure persisted). Refresh messages.
 */
async function onTurnDone() {
  if (streamCancelled || turnClosed) {
    streamText.value = ''
    streaming.value = false
    sending.value = false
    resuming.value = false
    finalizingRefetchFailed.value = false
    finalizing.value = false
    return
  }
  turnClosed = true
  streamCancelled = true
  clearTurnDeadline()
  streaming.value = false
  sending.value = false
  resuming.value = false
  finalizing.value = true
  if (!activeId.value) {
    clearFinalizingStream()
    return
  }
  await refetchAfterTurnDone()
}

async function onTurnError(kind: FailKind, detail: string) {
  if (turnClosed) return
  // Server already persisted failure on user message + draft; refresh then show card.
  turnClosed = true
  streamCancelled = true
  clearTurnDeadline()
  streamText.value = ''
  streaming.value = false
  sending.value = false
  resuming.value = false
  finalizing.value = false
  const mid = activeUserMessageId.value
  if (activeId.value) {
    const tid = activeId.value
    const gen = threadLoadGen
    try {
      const channel = isChannelThread(threads.value.find((x) => x.id === tid))
      const res = channel
        ? await api.listPmMessages(props.projectId, tid)
        : await api.listPmMessages(props.projectId, tid, { limit: PAGE_SIZE })
      if (gen === threadLoadGen && tid === activeId.value) {
        const incoming = res.items || []
        messages.value = channel
          ? incoming
          : mergeMessagesKeepPrefix(messages.value, incoming)
        if (!channel && typeof res.hasMore === 'boolean' && messages.value.length <= PAGE_SIZE) {
          hasMoreEarlier.value = res.hasMore
        }
      }
      // If server did not mark failure (edge), persist locally.
      if (mid) {
        const user = messages.value.find((m) => m.id === mid)
        if (user && user.status !== 'failed') {
          await persistFailure(mid, kind)
        }
      }
    } catch {
      if (mid) await persistFailure(mid, kind)
    }
  } else if (mid) {
    await persistFailure(mid, kind)
  }
  activeUserMessageId.value = ''
  if (detail && kind === 'unknown') toast.error(detail)
  await nextTick()
  scrollBottom()
}

/**
 * Run one consult turn against an already-persisted user message (send or retry).
 * @param existingGen - when send() already claimed a generation for the busy lock, reuse it.
 */
async function runTurn(
  userMsg: ChatMessage,
  content: string,
  imgs: ClarifyImage[],
  existingGen?: number,
) {
  const gen = existingGen ?? ++turnGen
  streamCancelled = false
  turnClosed = false
  activeUserMessageId.value = userMsg.id
  sending.value = true
  streaming.value = true
  streamText.value = ''
  // Ready-phase deadline (~90s); refreshed after sandbox is ready for stream phase.
  startTurnDeadline(gen)
  readyAbort = new AbortController()
  const signal = readyAbort.signal

  try {
    const preamble = await ensureSandbox(true, signal)
    if (gen !== turnGen || streamCancelled || turnClosed) return
    // Fresh generation window after ready so slow sandbox does not starve streaming.
    startTurnDeadline(gen)
    const prompt = preamble
      ? `${preamble}\n\n用户问题：${content || t('pages.projectDetail.pm.imagesOnly')}`
      : content || t('pages.projectDetail.pm.imagesOnly')
    if (!ws || ws.readyState !== WebSocket.OPEN) {
      throw Object.assign(new Error('ws not open'), { failKind: 'connection' as FailKind })
    }
    ws.send(
      JSON.stringify({
        type: 'chat',
        content: prompt,
        images: imgs.map((a) => ({ data: a.data, mimeType: a.mimeType })),
        userMsgId: userMsg.id,
        sandboxId,
      }),
    )
  } catch (e: any) {
    if (gen !== turnGen || turnClosed) return
    if (e?.name === 'AbortError' || streamCancelled) {
      if (!turnClosed) await failTurn('stopped')
      return
    }
    await failTurn(classifyPmTurnError(e))
    const detail = String(e?.message || e)
    if (detail && detail !== 'sandbox timeout') toast.error(detail)
  } finally {
    if (readyAbort?.signal === signal) readyAbort = null
  }
}

/**
 * @param text - when set, use as message body (suggestions) instead of input
 * @param explicitImages - when provided, use these images and do not take pending
 */
async function send(text?: string, explicitImages?: ClarifyImage[]) {
  const fromInput = text == null
  const content = (text ?? input.value).trim()
  const imgs =
    explicitImages !== undefined
      ? explicitImages.slice()
      : takeAttachments()
  if ((!content && imgs.length === 0) || busy.value) return
  if (activeIsChannel.value) return
  if (!enabled.value) {
    emit('openSettings')
    return
  }
  // f7: lock busy BEFORE any await so double-click / suggestion cannot append another user turn.
  const gen = ++turnGen
  streamCancelled = false
  turnClosed = false
  sending.value = true
  streaming.value = true
  streamText.value = ''
  startTurnDeadline(gen)
  if (fromInput) input.value = ''
  try {
    // Create thread without resetTurnLocal so this generation stays valid (review v1).
    if (!activeId.value) await ensureActiveThread()
    if (gen !== turnGen || streamCancelled || turnClosed) {
      // Stopped while creating thread / before append.
      sending.value = false
      streaming.value = false
      clearTurnDeadline()
      return
    }
    if (!activeId.value) throw new Error('no thread')
    const userMsg = await api.appendPmMessage(props.projectId, activeId.value, {
      role: 'user',
      content,
      images: imgs.length ? imgs : undefined,
    })
    messages.value = [...messages.value, userMsg]
    await nextTick()
    stickToBottom.value = true
    scrollBottom(true)
    if (gen !== turnGen || streamCancelled || turnClosed) {
      // Stopped during append: persist stopped on the message we just created.
      activeUserMessageId.value = userMsg.id
      if (turnClosed) {
        await persistFailure(userMsg.id, 'stopped')
        sending.value = false
        streaming.value = false
      } else {
        await failTurn('stopped')
      }
      return
    }
    await runTurn(userMsg, content, imgs, gen)
  } catch (e: any) {
    sending.value = false
    streaming.value = false
    clearTurnDeadline()
    if (fromInput) input.value = content
    if (imgs.length) attachments.value = [...imgs, ...attachments.value]
    toast.error(String(e?.message || e))
  }
}

function stop() {
  if (!busy.value) return
  streamCancelled = true
  clearTurnDeadline()
  if (readyAbort) {
    readyAbort.abort()
    readyAbort = null
  }
  if (ws && ws.readyState === WebSocket.OPEN) {
    try {
      ws.send(JSON.stringify({ type: 'cancel' }))
    } catch {
      /* ignore */
    }
  }
  streamText.value = ''
  resuming.value = false
  void failTurn('stopped')
}

/** Cover-this-turn retry: reuse userMessageId, do not append a new user bubble. */
async function retryTurn(userMessageId: string) {
  if (busy.value) return
  const userMsg = messages.value.find((m) => m.id === userMessageId && m.role === 'user')
  if (!userMsg) return
  // Lock busy before clearFailure await (same f7 race as send).
  const gen = ++turnGen
  streamCancelled = false
  turnClosed = false
  sending.value = true
  streaming.value = true
  streamText.value = ''
  clearFailedPartial(userMessageId)
  startTurnDeadline(gen)
  try {
    await clearFailure(userMessageId)
    if (gen !== turnGen || streamCancelled || turnClosed) {
      sending.value = false
      streaming.value = false
      clearTurnDeadline()
      return
    }
    const content = (userMsg.content || '').trim()
    const imgs = userMsg.images?.slice() ?? []
    await runTurn(userMsg, content, imgs, gen)
  } catch (e: any) {
    sending.value = false
    streaming.value = false
    clearTurnDeadline()
    toast.error(String(e?.message || e))
  }
}

watch(isMobile, () => {
  mobileView.value = 'threads'
})

watch(
  () => props.projectId,
  () => {
    activeId.value = ''
    messages.value = []
    messagesLoadFailed.value = false
    messagesLoading.value = false
    mobileView.value = 'threads'
    resetTurnLocal()
    void loadThreads()
  },
)

onMounted(() => {
  void loadThreads().then(() => applyRestoreMobileChat())
})
onBeforeUnmount(() => {
  resetTurnLocal()
  closeWs()
})
</script>

<template>
  <div v-if="!enabled" class="flex min-h-[420px] flex-col items-center justify-center gap-3 text-center">
    <h3 class="text-lg font-semibold text-txt">{{ t('pages.projectDetail.pm.disabledTitle') }}</h3>
    <p class="max-w-md text-sm text-txt3">{{ t('pages.projectDetail.pm.disabledHint') }}</p>
    <p v-if="binding?.agentError" class="text-sm text-err">{{ binding.agentError }}</p>
    <AppButton @click="emit('openSettings')">{{ t('pages.projectDetail.pm.goSettings') }}</AppButton>
  </div>

  <div v-else class="flex min-h-0 flex-1 overflow-hidden border border-line bg-base">
    <!-- left rail -->
    <aside
      v-if="showThreadsAside"
      data-testid="pm-threads-aside"
      class="flex min-h-0 shrink-0 flex-col border-r border-line bg-surface"
      :class="isMobile ? 'w-full border-r-0' : 'w-56'"
    >
      <div class="flex items-center justify-between border-b border-line px-2 py-2">
        <span class="text-xs font-medium text-txt3">{{ t('pages.projectDetail.pm.threads') }}</span>
        <AppButton
          size="sm"
          variant="ghost"
          :disabled="turnBusy"
          :class="isMobile ? 'min-h-[44px] min-w-[44px]' : ''"
          @click="newThread"
        >{{ t('pages.projectDetail.pm.newThread') }}</AppButton>
      </div>
      <div class="scroll-area flex-1 overflow-y-auto">
        <button
          v-for="th in threads"
          :key="th.id"
          type="button"
          class="group flex w-full items-center gap-1 border-b border-line px-2 text-left text-sm text-txt2 hover:bg-elevated hover:text-txt disabled:cursor-not-allowed disabled:opacity-50"
          :class="[
            th.id === activeId ? 'bg-elevated font-medium text-txt' : '',
            isMobile ? 'min-h-[44px] py-3' : 'py-2',
          ]"
          :data-channel="isChannelThread(th) ? '1' : '0'"
          :disabled="turnBusy && th.id !== activeId"
          @click="selectThread(th.id)"
          @contextmenu="openChannelCtx($event, th)"
        >
          <span class="flex min-w-0 flex-1 items-center gap-1.5 overflow-hidden">
            <span class="min-w-0 truncate">{{ threadDisplayTitle(th) }}</span>
            <span
              v-if="isChannelThread(th)"
              class="inline-flex shrink-0 items-center border border-accent-2/35 bg-accent/15 px-1 text-[9px] font-bold uppercase tracking-wide leading-4 text-accent-2"
              data-testid="pm-qq-tag"
              :title="t('pages.projectDetail.pm.channelSource')"
            >QQ</span>
          </span>
          <span
            v-if="!turnBusy && !isChannelThread(th)"
            class="hidden text-xs text-txt3 group-hover:inline"
            data-testid="pm-thread-delete"
            @click.stop="removeThread(th.id)"
          >×</span>
        </button>
        <p v-if="!threads.length && !loading" class="px-2 py-4 text-xs text-txt3">
          {{ t('pages.projectDetail.pm.noThreads') }}
        </p>
      </div>
    </aside>

    <!-- main chat -->
    <section
      v-if="showChatSection"
      data-testid="pm-chat-section"
      class="flex min-h-0 min-w-0 flex-col bg-base"
      :class="isMobile ? 'w-full flex-1' : 'flex-1'"
    >
      <div
        class="flex shrink-0 items-center gap-2 border-b border-line px-3 py-2"
        :class="isMobile ? 'min-h-[44px]' : ''"
      >
        <button
          v-if="isMobile"
          type="button"
          class="flex h-11 w-11 shrink-0 items-center justify-center rounded-md text-txt2 hover:bg-elevated hover:text-txt disabled:cursor-not-allowed disabled:opacity-50"
          data-testid="pm-mobile-back-to-threads"
          :aria-label="t('shell.aria.backToList')"
          :disabled="turnBusy"
          @click="backToThreads"
        >
          <Icon name="arrow-left" :size="18" />
        </button>
        <div
          v-if="activeId"
          class="flex min-w-0 flex-1 items-center gap-2 overflow-hidden"
          data-testid="pm-chat-title"
        >
          <span class="min-w-0 truncate text-sm font-medium text-txt">{{ activeThreadTitle }}</span>
          <span
            v-if="activeIsChannel"
            class="inline-flex shrink-0 items-center border border-accent-2/35 bg-accent/15 px-1 text-[9px] font-bold uppercase tracking-wide leading-4 text-accent-2"
            data-testid="pm-qq-tag-header"
            :title="t('pages.projectDetail.pm.channelSource')"
          >QQ</span>
        </div>
        <span v-else class="min-w-0 flex-1" />
        <AppButton
          size="sm"
          variant="ghost"
          icon="settings"
          data-testid="pm-chat-open-settings"
          :class="isMobile ? 'min-h-[44px] shrink-0' : ''"
          @click="emit('openSettings')"
        >
          {{ t('pages.projectDetail.pm.settings') }}
        </AppButton>
      </div>
      <div
        ref="scroller"
        data-testid="pm-message-scroller"
        class="scroll-area min-h-0 flex-1 space-y-3 overflow-y-auto p-4"
        @scroll="onScrollerScroll"
      >
        <ArtifactLoadingPane
          v-if="mainViewState === 'messagesLoading'"
          message-key="pages.projectDetail.pm.loadingHistory"
          data-testid="pm-messages-loading"
        />

        <EmptyState
          v-else-if="mainViewState === 'errorEmpty'"
          icon="alert"
          :title="t('pages.projectDetail.pm.loadFailed')"
          :desc="t('pages.projectDetail.pm.loadFailedDesc')"
          data-testid="pm-messages-load-failed"
        >
          <AppButton variant="primary" size="sm" icon="refresh" @click="retryLoadMessages">
            {{ t('pages.projectDetail.pm.retry') }}
          </AppButton>
        </EmptyState>

        <template v-else>
        <div class="mx-auto flex max-w-3xl flex-col gap-3.5">
        <div v-if="showEmptyHint" class="space-y-3 py-8 text-center">
          <p class="text-sm text-txt3">{{ t('pages.projectDetail.pm.emptyHint') }}</p>
          <div class="flex flex-wrap justify-center gap-2">
            <button
              v-for="s in suggestions"
              :key="s"
              type="button"
              class="border border-line bg-surface px-3 py-1 text-sm text-txt2 hover:bg-elevated hover:text-txt disabled:opacity-50"
              :disabled="busy"
              @click="send(s)"
            >
              {{ s }}
            </button>
          </div>
        </div>

        <div
          v-if="showHistoryTip"
          data-testid="pm-history-tip"
          class="min-h-[28px] py-1.5 text-center text-[11.5px]"
          :class="historyTipClass"
        >
          {{ historyTipText }}
        </div>

        <template v-for="m in messages" :key="m.id">
          <div v-if="m.role === 'user'" class="flex gap-2.5 flex-row-reverse" :data-msg-id="m.id">
            <div
              class="flex h-7 w-7 shrink-0 items-center justify-center border border-accent/20 bg-accent-dim text-[11px] font-semibold text-accent-2"
            >
              {{ t('pages.projectDetail.pm.me') }}
            </div>
            <div class="min-w-0 max-w-[85%]">
              <div v-if="m.images?.length" class="mb-1.5 flex flex-wrap justify-end gap-1.5">
                <img
                  v-for="(im, ii) in m.images"
                  :key="ii"
                  :src="imgSrc(im)"
                  class="h-20 w-20 border border-line object-cover"
                  alt=""
                />
              </div>
              <div
                v-if="m.content"
                class="border border-accent/35 bg-accent px-3 py-2 text-sm leading-6 text-white whitespace-pre-wrap"
              >
                {{ m.content }}
              </div>
              <div
                v-else-if="m.images?.length"
                class="border border-accent/35 bg-accent px-3 py-2 text-sm leading-6 text-white/80"
              >
                {{ t('pages.projectDetail.pm.imagesOnly') }}
              </div>
              <div class="msg-time mt-1 text-right text-[10px] text-txt3">{{ relTime(m.createdAt) }}</div>
            </div>
          </div>
          <div v-else-if="m.role === 'assistant'" class="flex gap-2.5" :data-msg-id="m.id">
            <div
              class="flex h-7 w-7 shrink-0 items-center justify-center border border-accent/25 bg-accent/10 text-accent-2"
            >
              <Icon name="robot" :size="15" />
            </div>
            <div class="min-w-0 max-w-[85%]">
              <div
                data-assistant-bubble
                class="border border-line bg-elevated px-3 py-2 text-sm leading-6 text-txt"
              >
                <div class="md" v-html="renderMarkdown(m.content)" />
                <div
                  v-if="m.content?.trim()"
                  class="msg-actions mt-1.5 flex justify-end gap-1 border-t border-line pt-1"
                >
                  <button
                    type="button"
                    class="inline-flex items-center gap-1 px-2 py-0.5 text-[11px] text-txt3 hover:bg-surface hover:text-txt2"
                    @click="copyAssistantText"
                  >
                    <Icon name="copy" :size="12" />
                    {{ t('pages.projectDetail.pm.copyFull') }}
                  </button>
                </div>
              </div>
              <div class="msg-time mt-1 text-right text-[10px] text-txt3">{{ relTime(m.createdAt) }}</div>
              <div v-if="m.citations?.length" class="mt-1.5 space-y-1">
                <CitationCard v-for="(c, i) in m.citations" :key="i" :citation="c" />
              </div>
            </div>
          </div>

          <!-- S2: failed partial bubble (session-only), then independent fail card. -->
          <div
            v-if="isFailedUser(m) && failedPartialByUserMsgId[m.id]"
            class="flex gap-2.5"
            data-testid="pm-failed-partial"
          >
            <div
              class="flex h-7 w-7 shrink-0 items-center justify-center border border-accent/25 bg-accent/10 text-accent-2"
            >
              <Icon name="robot" :size="15" />
            </div>
            <div class="min-w-0 max-w-[85%]">
              <div class="border border-err/35 bg-err/[0.06] px-3 py-2 text-sm leading-6 text-txt">
                <div class="mb-1.5 flex items-center gap-1.5 text-[11px] font-semibold text-red-400">
                  <Icon name="alert" :size="14" class="shrink-0" />
                  {{ t('pages.projectDetail.pm.failPartialKeptMeta') }}
                </div>
                <div class="md" v-html="renderMarkdown(failedPartialByUserMsgId[m.id])" />
              </div>
            </div>
          </div>

          <!-- Failure card hangs beside the user turn (after the bubble / partial). -->
          <div v-if="isFailedUser(m)" class="flex justify-start">
            <div
              class="fail-card ml-[38px] max-w-[85%] flex flex-col gap-2 border border-err/35 bg-err/10 px-3 py-2.5"
              role="alert"
            >
              <div class="flex items-start gap-2">
                <Icon name="alert" :size="16" class="mt-0.5 shrink-0 text-err" />
                <div>
                  <div class="text-[13px] font-semibold text-err">
                    {{ failMeta(m.failKind || 'connection').title }}
                  </div>
                  <div class="mt-0.5 text-xs text-txt2">
                    {{ failMeta(m.failKind || 'connection').desc }}
                  </div>
                </div>
              </div>
              <div>
                <button
                  type="button"
                  class="border border-err/40 bg-transparent px-2.5 py-1 text-xs text-err hover:bg-err/15 disabled:opacity-50"
                  :disabled="busy"
                  data-testid="pm-fail-retry"
                  @click="retryTurn(m.id)"
                >
                  {{ t('pages.projectDetail.pm.retry') }}
                </button>
              </div>
            </div>
          </div>
        </template>

        <div v-if="showStreamBubble" class="flex gap-2.5" data-testid="pm-stream-bubble">
          <div
            class="flex h-7 w-7 shrink-0 items-center justify-center border border-accent/25 bg-accent/10 text-accent-2"
          >
            <Icon name="robot" :size="15" />
          </div>
          <div class="min-w-0 max-w-[85%]">
            <div
              class="border bg-elevated px-3 py-2 text-sm text-txt"
              :class="
                finalizing
                  ? 'border-warn/35 shadow-[inset_0_0_0_1px_rgb(var(--color-warn)/0.08)]'
                  : resuming
                    ? 'border-accent-2/40 shadow-[inset_0_0_0_1px_rgb(var(--color-accent-2)/0.08)]'
                    : 'border-line'
              "
            >
              <div v-if="resuming && !streamText && !finalizing" class="flex items-center gap-1.5 text-txt3">
                <Icon name="spinner" :size="13" class="animate-spin text-accent-2" />
                {{ t('pages.projectDetail.pm.resuming') }}
              </div>
              <div v-else-if="streamText" class="md" v-html="renderMarkdown(streamText)" />
              <div v-else-if="showStreamTypingDots" class="typing-dots py-1">
                <i /><i /><i />
              </div>
              <div v-else class="flex items-center gap-1.5 text-txt3">
                <Icon name="spinner" :size="13" class="animate-spin text-accent-2" />
                {{ t('pages.projectDetail.pm.generating') }}
              </div>
              <div
                v-if="finalizing"
                class="mt-2 flex flex-wrap items-center gap-1.5 border-t border-line pt-2 text-[11px] text-warn"
                data-testid="pm-stream-finalizing-meta"
              >
                <template v-if="finalizingRefetchFailed">
                  <Icon name="alert" :size="13" class="text-warn" />
                  <span>{{ t('pages.projectDetail.pm.loadFailed') }}</span>
                  <button
                    type="button"
                    class="ml-1 text-accent underline-offset-2 hover:underline"
                    data-testid="pm-finalizing-retry"
                    @click="refetchAfterTurnDone"
                  >
                    {{ t('pages.projectDetail.pm.retry') }}
                  </button>
                </template>
                <template v-else>
                  <Icon name="spinner" :size="13" class="animate-spin text-warn" />
                  {{ t('pages.projectDetail.pm.finalizing') }}
                </template>
              </div>
              <div
                v-else-if="resuming"
                class="mt-2 flex items-center gap-1.5 border-t border-line pt-2 text-[11px] text-accent-2"
                data-testid="pm-stream-resuming-meta"
              >
                <Icon name="spinner" :size="13" class="animate-spin text-accent-2" />
                {{ t('pages.projectDetail.pm.resuming') }}
              </div>
            </div>
            <div
              class="msg-time mt-1 text-right text-[10px] text-txt3"
              :class="streamText || finalizing ? '' : 'invisible'"
            >
              —
            </div>
          </div>
        </div>

        <div
          v-if="showIdleSuggestions"
          class="mt-1 flex flex-wrap gap-1.5 pl-[38px]"
          data-testid="pm-idle-suggestions"
        >
          <button
            v-for="s in suggestions"
            :key="s"
            type="button"
            class="border border-line bg-surface px-2.5 py-1 text-xs text-txt2 hover:bg-elevated hover:text-txt disabled:opacity-50"
            :disabled="busy"
            @click="send(s)"
          >
            {{ s }}
          </button>
        </div>
        </div>
        </template>
      </div>

      <div v-if="activeIsChannel" class="shrink-0 border-t border-line p-3" data-testid="pm-channel-readonly">
        <div class="flex items-center gap-2.5 border border-accent-2/30 bg-accent/10 px-3.5 py-3">
          <div
            class="flex h-7 w-7 shrink-0 items-center justify-center border border-accent-2/35 bg-accent/15 text-accent-2"
            aria-hidden="true"
          >
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <rect x="3" y="11" width="18" height="11" />
              <path d="M7 11V7a5 5 0 0 1 10 0v4" />
            </svg>
          </div>
          <div class="min-w-0">
            <strong class="block text-[13px] font-semibold text-txt">
              {{ t('pages.projectDetail.pm.channelReadonlyTitle') }}
            </strong>
            <p class="mt-0.5 text-xs text-txt3">
              {{ t('pages.projectDetail.pm.channelReadonlyHint') }}
            </p>
          </div>
        </div>
      </div>
      <div v-else class="shrink-0 border-t border-line p-3">
        <div v-if="attachments.length" class="mb-2 flex flex-wrap gap-1.5">
          <div v-for="(im, ii) in attachments" :key="ii" class="relative">
            <img :src="imgSrc(im)" class="h-14 w-14 border border-line object-cover" alt="" />
            <button
              type="button"
              class="absolute -right-1.5 -top-1.5 flex h-4 w-4 items-center justify-center bg-err text-white"
              :disabled="busy"
              @click="removeAttachment(ii)"
            >
              <Icon name="close" :size="9" />
            </button>
          </div>
        </div>
        <div class="flex flex-wrap items-end gap-2">
          <input
            ref="fileInput"
            type="file"
            accept="image/*"
            multiple
            class="hidden"
            @change="onPickFiles"
          />
          <AppButton
            size="sm"
            variant="outline"
            icon="paperclip"
            :disabled="busy"
            data-testid="pm-chat-attach"
            :class="isMobile ? 'min-h-[44px] min-w-[44px]' : ''"
            @click="fileInput?.click()"
          >
            {{ t('pages.projectDetail.pm.images') }}
          </AppButton>
          <textarea
            v-model="input"
            rows="2"
            class="scroll-area max-h-32 min-h-[40px] min-w-0 flex-1 resize-none border border-line bg-base px-3 py-2 text-[13px] text-txt outline-none focus:border-accent disabled:opacity-50"
            :placeholder="t('pages.projectDetail.pm.inputPh')"
            :disabled="busy"
            @keydown.enter.exact.prevent="send()"
            @paste="onPaste"
          />
          <AppButton
            v-if="turnBusy"
            size="sm"
            variant="outline"
            icon="close"
            :class="isMobile ? 'min-h-[44px] min-w-[44px]' : ''"
            @click="stop"
          >
            {{ t('pages.projectDetail.pm.stop') }}
          </AppButton>
          <AppButton
            v-else
            size="sm"
            variant="primary"
            icon="send"
            :disabled="!canSend"
            data-testid="pm-chat-send"
            :class="isMobile ? 'min-h-[44px] min-w-[44px]' : ''"
            @click="send()"
          >
            {{ t('pages.projectDetail.pm.send') }}
          </AppButton>
        </div>
        <p v-if="turnBusy" class="mt-2 text-[11px] text-warn">
          {{ busyHint }}
        </p>
      </div>
    </section>
  </div>

  <!-- Channel context menu -->
  <div
    v-if="channelCtx"
    class="fixed inset-0 z-40"
    data-testid="pm-channel-ctx-backdrop"
    @click="closeChannelCtx"
    @contextmenu.prevent="closeChannelCtx"
  />
  <div
    v-if="channelCtx"
    class="fixed z-50 min-w-[160px] border border-line bg-elevated py-1 shadow-card"
    data-testid="pm-channel-ctx-menu"
    role="menu"
    :style="{ left: `${channelCtx.x}px`, top: `${channelCtx.y}px` }"
  >
    <button
      type="button"
      role="menuitem"
      class="flex w-full items-center gap-2 px-3 py-1.5 text-left text-xs text-txt2 hover:bg-overlay hover:text-txt"
      data-testid="pm-channel-ctx-detail"
      @click="onChannelCtxAction"
    >
      <Icon name="doc" :size="13" />
      {{ t('pages.projectDetail.pm.channelViewDetail') }}
    </button>
  </div>

  <!-- Channel detail modal -->
  <div
    v-if="channelDetailOpen"
    class="fixed inset-0 z-[60] flex items-center justify-center bg-black/60 p-4"
    data-testid="pm-channel-detail-backdrop"
    role="dialog"
    aria-modal="true"
    @click.self="closeChannelDetail"
  >
    <div class="w-full max-w-[360px] border border-line bg-surface shadow-card">
      <div class="flex items-center justify-between border-b border-line px-3.5 py-3 text-[13px] font-semibold text-txt">
        <span>{{ t('pages.projectDetail.pm.channelDetailTitle') }}</span>
        <button
          type="button"
          class="px-2 text-txt2 hover:text-txt"
          :aria-label="t('pages.projectDetail.pm.channelDetailClose')"
          data-testid="pm-channel-detail-close"
          @click="closeChannelDetail"
        >×</button>
      </div>
      <div class="flex flex-col gap-3 p-3.5">
        <div>
          <div class="mb-1 text-[11px] text-txt3">{{ t('pages.projectDetail.pm.channelDetailLabelTitle') }}</div>
          <div class="border border-line bg-base px-2.5 py-2 text-[13px] text-txt" data-testid="pm-channel-detail-title">
            {{ channelDetailTitle }}
          </div>
        </div>
        <div>
          <div class="mb-1 text-[11px] text-txt3">{{ t('pages.projectDetail.pm.channelDetailLabelSource') }}</div>
          <div class="border border-line bg-base px-2.5 py-2 text-[13px] text-txt" data-testid="pm-channel-detail-source">
            {{ t('pages.projectDetail.pm.channelSource') }}
          </div>
        </div>
      </div>
      <div class="flex justify-end border-t border-line px-3.5 py-2.5">
        <AppButton size="sm" variant="outline" data-testid="pm-channel-detail-ok" @click="closeChannelDetail">
          {{ t('pages.projectDetail.pm.channelDetailClose') }}
        </AppButton>
      </div>
    </div>
  </div>
</template>

<style scoped>
.typing-dots {
  display: inline-flex;
  align-items: center;
  gap: 4px;
}
.typing-dots i {
  width: 5px;
  height: 5px;
  border-radius: 9999px;
  background: rgb(var(--c-accent-2));
  animation: pm-typing-bounce 1.2s infinite ease-in-out both;
}
.typing-dots i:nth-child(2) {
  animation-delay: 0.16s;
}
.typing-dots i:nth-child(3) {
  animation-delay: 0.32s;
}
@keyframes pm-typing-bounce {
  0%,
  70%,
  100% {
    transform: translateY(0);
    opacity: 0.35;
  }
  35% {
    transform: translateY(-4px);
    opacity: 1;
  }
}
</style>
