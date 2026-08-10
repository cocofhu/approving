import { computed, ref } from 'vue'
import { api, isPaginated } from '@/lib/api'
import type { Run } from '@/lib/types'
import { createTimeoutController, isAbortError } from '@/lib/loadingRequest'
import { DEFAULT_LOADING_TIMEOUT_MS } from '@/lib/loadingTypes'
import { useAuth } from '@/lib/useAuth'

/** Recent completed/failed runs that form the notification pool (plan: ~50). */
export const RUN_TERMINAL_POOL_SIZE = 50
/** Dropdown preview hard limit (~10 per clarified requirement). */
export const RUN_TERMINAL_PANEL_LIMIT = 10
/** Align with AppSidebarNav pending-gates peek interval. */
export const RUN_TERMINAL_POLL_MS = 15_000

export type RunTerminalStatus = 'completed' | 'failed'

/**
 * Client-side Notification projection of a terminal Run.
 * Product object is Notification (inbox event), not Run list row.
 */
export interface RunTerminalNotification {
  runId: string
  status: RunTerminalStatus
  /** Cleaned display title (never progress-noise). */
  title: string
  /** True when title was replaced by the neutral template. */
  titleNeutral: boolean
  workflowName: string
  startedAt: string
  /** Approx terminal time: startedAt + durationSec (ISO). */
  finishedApprox: string
}

export type RunTerminalRefreshSource =
  | 'mount'
  | 'sidebar-poll'
  | 'visibility'
  | 'focus'
  | 'manual'

interface NotificationPrefs {
  /** Feature-enable baseline; history before this is always treated as read. */
  enabledAt: string
  readIds: string[]
}

export function formatUnreadBadge(n: number): string {
  if (n <= 0) return ''
  if (n >= 99) return '99+'
  return String(n)
}

/** @deprecated Prefer prefsKeyForUser; kept for migration of legacy read sets. */
export function storageKeyForUser(username: string): string {
  return `approving.runTerminalNotifications.readIds.${username || 'anonymous'}`
}

export function prefsKeyForUser(username: string): string {
  return `approving.notifications.prefs.${username || 'anonymous'}`
}

/** Progress / in-progress wording that must not appear as notification titles. */
export function isNoisyNotificationTitle(title: string): boolean {
  const s = title.trim()
  if (!s) return false
  return /运行中|排队中?|等待人工|等待中|(?:^|[\s·\-_/，,])(queued|running|waiting)(?:[\s·\-_/，,]|$)|waiting[_ ]human|in\s*progress/i.test(
    s,
  )
}

/** Approx enter-terminal timestamp for sort + unread baseline. */
export function finishedApproxIso(run: Run): string {
  const start = (run.startedAt || run.createdAt || '').trim()
  if (!start) return ''
  const dur = typeof run.durationSec === 'number' && run.durationSec > 0 ? run.durationSec : 0
  if (!dur) return start
  const t = Date.parse(start)
  if (Number.isNaN(t)) return start
  return new Date(t + dur * 1000).toISOString()
}

export function mapRunToNotification(run: Run): RunTerminalNotification | null {
  if (run.status !== 'completed' && run.status !== 'failed') return null
  const raw = (run.title || '').trim()
  const workflowName = (run.workflowName || '').trim()
  const finishedApprox = finishedApproxIso(run)
  const startedAt = (run.startedAt || run.createdAt || '').trim()

  let title = raw || workflowName || run.id
  let titleNeutral = false
  if (isNoisyNotificationTitle(title)) {
    const name = workflowName || run.id
    // Neutral template tokens; UI may re-localize via titleNeutral + status.
    title = `${name} · ${run.status === 'failed' ? 'failed' : 'completed'}`
    titleNeutral = true
  }

  return {
    runId: run.id,
    status: run.status,
    title,
    titleNeutral,
    workflowName,
    startedAt,
    finishedApprox: finishedApprox || startedAt,
  }
}

function compareFinishedDesc(a: RunTerminalNotification, b: RunTerminalNotification): number {
  const ta = Date.parse(a.finishedApprox || a.startedAt || '') || 0
  const tb = Date.parse(b.finishedApprox || b.startedAt || '') || 0
  if (tb !== ta) return tb - ta
  return b.runId.localeCompare(a.runId)
}

function loadPrefs(username: string): NotificationPrefs {
  const now = new Date().toISOString()
  if (typeof localStorage === 'undefined') {
    return { enabledAt: now, readIds: [] }
  }
  try {
    const raw = localStorage.getItem(prefsKeyForUser(username))
    if (raw) {
      const parsed = JSON.parse(raw) as Partial<NotificationPrefs>
      const enabledAt =
        typeof parsed.enabledAt === 'string' && parsed.enabledAt.trim()
          ? parsed.enabledAt
          : now
      const readIds = Array.isArray(parsed.readIds)
        ? parsed.readIds.filter((x): x is string => typeof x === 'string')
        : []
      return { enabledAt, readIds }
    }
  } catch {
    // fall through to migrate / create
  }

  // Migrate legacy readIds-only storage once.
  let legacyIds: string[] = []
  try {
    const legacy = localStorage.getItem(storageKeyForUser(username))
    if (legacy) {
      const parsed = JSON.parse(legacy) as unknown
      if (Array.isArray(parsed)) {
        legacyIds = parsed.filter((x): x is string => typeof x === 'string')
      }
    }
  } catch {
    legacyIds = []
  }

  const prefs: NotificationPrefs = { enabledAt: now, readIds: legacyIds }
  persistPrefs(username, prefs)
  return prefs
}

function persistPrefs(username: string, prefs: NotificationPrefs) {
  if (typeof localStorage === 'undefined') return
  try {
    localStorage.setItem(prefsKeyForUser(username), JSON.stringify(prefs))
  } catch {
    // Quota / private mode — ignore; unread may reset next session.
  }
}

const pool = ref<RunTerminalNotification[]>([])
const readIds = ref<Set<string>>(new Set())
const enabledAt = ref<string>('')
const usernameKey = ref('anonymous')
const error = ref<string | null>(null)
const lastRefreshSource = ref<RunTerminalRefreshSource | null>(null)
const lastPeekAt = ref(0)
const loading = ref(false)

let refreshPromise: Promise<void> | null = null
let refreshGeneration = 0
let abortCtrl: AbortController | null = null
let pollTimer: number | undefined
let listenersAttached = false
let prefsHydrated = false

function isUnread(n: RunTerminalNotification): boolean {
  if (readIds.value.has(n.runId)) return false
  const baseline = enabledAt.value
  if (!baseline) return false
  const finished = n.finishedApprox || n.startedAt
  if (!finished) return false
  const ft = Date.parse(finished)
  const bt = Date.parse(baseline)
  if (Number.isNaN(ft) || Number.isNaN(bt)) return false
  // Only events that entered terminal after the enable baseline count as unread.
  return ft > bt
}

const unreadCount = computed(
  () => pool.value.filter((n) => isUnread(n)).length,
)

const hasUnreadFailed = computed(() =>
  pool.value.some((n) => isUnread(n) && n.status === 'failed'),
)

const badgeLabel = computed(() => formatUnreadBadge(unreadCount.value))

function withUnread(n: RunTerminalNotification) {
  return { ...n, unread: isUnread(n) }
}

const previewItems = computed(() =>
  pool.value.slice(0, RUN_TERMINAL_PANEL_LIMIT).map(withUnread),
)

/** Full pool with unread flags for the independent notifications page. */
const listItems = computed(() => pool.value.map(withUnread))

const remainingCount = computed(() => Math.max(pool.value.length - RUN_TERMINAL_PANEL_LIMIT, 0))

const poolTotal = computed(() => pool.value.length)

function ensureUsername() {
  const { user } = useAuth()
  const next = user.value?.username?.trim() || 'anonymous'
  if (next !== usernameKey.value) {
    usernameKey.value = next
    const prefs = loadPrefs(next)
    enabledAt.value = prefs.enabledAt
    readIds.value = new Set(prefs.readIds)
    prefsHydrated = true
    return
  }
  if (!prefsHydrated) {
    const prefs = loadPrefs(next)
    enabledAt.value = prefs.enabledAt
    readIds.value = new Set(prefs.readIds)
    prefsHydrated = true
  }
}

function persistCurrent() {
  persistPrefs(usernameKey.value, {
    enabledAt: enabledAt.value || new Date().toISOString(),
    readIds: [...readIds.value],
  })
}

function markRead(runId: string) {
  if (!runId || readIds.value.has(runId)) return
  const next = new Set(readIds.value)
  next.add(runId)
  readIds.value = next
  persistCurrent()
}

function markAllRead() {
  ensureUsername()
  if (!pool.value.length) {
    // Still persist baseline so empty sessions don't re-seed oddly.
    persistCurrent()
    return
  }
  const next = new Set(readIds.value)
  for (const n of pool.value) next.add(n.runId)
  readIds.value = next
  persistCurrent()
}

async function fetchPool(signal?: AbortSignal): Promise<RunTerminalNotification[]> {
  const data = await api.listRuns({
    status: 'completed,failed',
    page: 1,
    pageSize: RUN_TERMINAL_POOL_SIZE,
    sort: 'started_at',
    order: 'desc',
    signal,
  })
  const items = isPaginated(data) ? data.items : data
  const mapped: RunTerminalNotification[] = []
  for (const run of items) {
    const n = mapRunToNotification(run)
    if (n) mapped.push(n)
  }
  mapped.sort(compareFinishedDesc)
  return mapped
}

async function refresh(opts?: { source?: RunTerminalRefreshSource }): Promise<void> {
  ensureUsername()
  if (refreshPromise) return refreshPromise

  const gen = ++refreshGeneration
  abortCtrl?.abort()
  abortCtrl = new AbortController()
  const tc = createTimeoutController(DEFAULT_LOADING_TIMEOUT_MS, abortCtrl.signal)

  const holder: { flight: Promise<void> | null } = { flight: null }
  holder.flight = (async () => {
    loading.value = true
    try {
      const next = await fetchPool(tc.signal)
      if (gen !== refreshGeneration) return
      pool.value = next
      // Keep read set bounded to current pool (+ already-persisted ids intersecting pool).
      const poolIds = new Set(next.map((n) => n.runId))
      const pruned = new Set([...readIds.value].filter((id) => poolIds.has(id)))
      if (pruned.size !== readIds.value.size) {
        readIds.value = pruned
        persistCurrent()
      }
      error.value = null
      lastRefreshSource.value = opts?.source ?? null
      lastPeekAt.value = Date.now()
    } catch (err) {
      if (gen !== refreshGeneration) return
      if (isAbortError(err) && !tc.timedOut) return
      error.value = err instanceof Error && err.message ? err.message : String(err || 'load failed')
    } finally {
      tc.clear()
      if (refreshPromise === holder.flight) refreshPromise = null
      loading.value = false
    }
  })()
  refreshPromise = holder.flight
  return holder.flight
}

function onVisibility() {
  if (typeof document !== 'undefined' && document.visibilityState === 'visible') {
    void refresh({ source: 'visibility' })
  }
}

function onFocus() {
  void refresh({ source: 'focus' })
}

function startPolling() {
  ensureUsername()
  stopPolling()
  void refresh({ source: 'mount' })
  if (typeof window === 'undefined') return
  pollTimer = window.setInterval(() => {
    void refresh({ source: 'sidebar-poll' })
  }, RUN_TERMINAL_POLL_MS)
  if (!listenersAttached) {
    document.addEventListener('visibilitychange', onVisibility)
    window.addEventListener('focus', onFocus)
    listenersAttached = true
  }
}

function stopPolling() {
  if (pollTimer != null) {
    clearInterval(pollTimer)
    pollTimer = undefined
  }
  if (listenersAttached && typeof window !== 'undefined') {
    document.removeEventListener('visibilitychange', onVisibility)
    window.removeEventListener('focus', onFocus)
    listenersAttached = false
  }
  abortCtrl?.abort()
  abortCtrl = null
}

/** Test helper: reset module singleton state. */
export function __resetRunTerminalNotificationsForTests() {
  stopPolling()
  pool.value = []
  readIds.value = new Set()
  enabledAt.value = ''
  usernameKey.value = 'anonymous'
  error.value = null
  lastRefreshSource.value = null
  lastPeekAt.value = 0
  loading.value = false
  refreshPromise = null
  refreshGeneration = 0
  prefsHydrated = false
}

export function useRunTerminalNotifications() {
  return {
    pool,
    poolTotal,
    previewItems,
    listItems,
    remainingCount,
    unreadCount,
    hasUnreadFailed,
    badgeLabel,
    enabledAt,
    error,
    loading,
    lastRefreshSource,
    lastPeekAt,
    markRead,
    markAllRead,
    refresh,
    startPolling,
    stopPolling,
    ensureUsername,
    isUnread,
  }
}
