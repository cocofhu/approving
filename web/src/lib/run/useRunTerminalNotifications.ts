import { computed, ref, watch } from 'vue'
import { api, isPaginated } from '@/lib/api/api'
import type { Run } from '@/lib/shared/types'
import { createTimeoutController, isAbortError } from '@/lib/shared/loadingRequest'
import { DEFAULT_LOADING_TIMEOUT_MS } from '@/lib/shared/loadingTypes'
import { useAuth } from '@/lib/composables/useAuth'

/** Recent completed/failed runs that form the notification pool (plan: ~50). */
export const RUN_TERMINAL_POOL_SIZE = 50
/** Dropdown preview hard limit (5 per clarified requirement / Demo). */
export const RUN_TERMINAL_PANEL_LIMIT = 5
/** Align with AppSidebarNav pending-gates peek interval. */
const RUN_TERMINAL_POLL_MS = 15_000

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

/** Pool item enriched with unread + baseline-before flags for UI. */
export type RunTerminalNotificationItem = RunTerminalNotification & {
  unread: boolean
  /** Finished at/before enable baseline — visible but never counts as unread. */
  beforeBaseline: boolean
}

export type RunTerminalRefreshSource =
  | 'mount'
  | 'sidebar-poll'
  | 'visibility'
  | 'focus'
  | 'manual'
  | 'auth'

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

/**
 * @deprecated Legacy localStorage key; server prefs are authoritative.
 * Kept for e2e/harness cleanup of old keys only.
 */
export function storageKeyForUser(username: string): string {
  return `approving.runTerminalNotifications.readIds.${username || 'anonymous'}`
}

/**
 * @deprecated Legacy localStorage prefs key; server prefs are authoritative.
 * Kept for e2e/harness cleanup of old keys only — do not read/write as source of truth.
 */
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

function applyPrefs(prefs: NotificationPrefs) {
  enabledAt.value = prefs.enabledAt
  readIds.value = new Set(prefs.readIds)
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
let authWatchInstalled = false
let prefsFetchPromise: Promise<boolean> | null = null

/**
 * Resolve the prefs username only after auth has settled.
 * Avoid hydrating as `anonymous` while /auth/me is in flight — that stamped
 * enabledAt≈now and under-counted the badge until focus/poll rehydrated.
 */
function resolveUsername(): { name: string; settled: boolean } {
  const { user, ready } = useAuth()
  const name = user.value?.username?.trim() || ''
  if (name) return { name, settled: true }
  // Auth explicitly in flight (ready===false) → wait; do not stamp anonymous prefs.
  if (ready && ready.value === false) return { name: '', settled: false }
  // ready===true with no user, or callers/tests without ready → anonymous fallback.
  return { name: 'anonymous', settled: true }
}

function finishedMs(n: RunTerminalNotification): number {
  const finished = n.finishedApprox || n.startedAt
  if (!finished) return NaN
  return Date.parse(finished)
}

/** True when terminal time is at/before enable baseline (history, not unread). */
export function isBeforeBaseline(
  n: Pick<RunTerminalNotification, 'finishedApprox' | 'startedAt'>,
  baselineIso: string,
): boolean {
  if (!baselineIso) return false
  const finished = n.finishedApprox || n.startedAt
  if (!finished) return false
  const ft = Date.parse(finished)
  const bt = Date.parse(baselineIso)
  if (Number.isNaN(ft) || Number.isNaN(bt)) return false
  return ft <= bt
}

function isUnread(n: RunTerminalNotification): boolean {
  if (readIds.value.has(n.runId)) return false
  const baseline = enabledAt.value
  if (!baseline) return false
  const ft = finishedMs(n)
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

function withUnread(n: RunTerminalNotification): RunTerminalNotificationItem {
  return {
    ...n,
    unread: isUnread(n),
    beforeBaseline: isBeforeBaseline(n, enabledAt.value),
  }
}

const previewItems = computed(() =>
  pool.value.slice(0, RUN_TERMINAL_PANEL_LIMIT).map(withUnread),
)

/** Full pool with unread flags for the independent notifications page. */
const listItems = computed(() => pool.value.map(withUnread))

const remainingCount = computed(() => Math.max(pool.value.length - RUN_TERMINAL_PANEL_LIMIT, 0))

const poolTotal = computed(() => pool.value.length)

async function fetchServerPrefs(signal?: AbortSignal): Promise<NotificationPrefs | null> {
  try {
    const prefs = await api.getNotificationReadPrefs(signal ? { signal } : undefined)
    return {
      enabledAt:
        typeof prefs.enabledAt === 'string' && prefs.enabledAt.trim()
          ? prefs.enabledAt
          : new Date().toISOString(),
      readIds: Array.isArray(prefs.readIds)
        ? prefs.readIds.filter((x): x is string => typeof x === 'string')
        : [],
    }
  } catch (err) {
    if (isAbortError(err)) return null
    throw err
  }
}

/**
 * Ensure auth identity is settled. Kicks off async server prefs hydrate when needed.
 * @returns true when auth identity is settled (prefs may still be loading).
 */
function ensureUsername(): boolean {
  const { name, settled } = resolveUsername()
  if (!settled) {
    prefsHydrated = false
    return false
  }
  if (name !== usernameKey.value) {
    usernameKey.value = name
    prefsHydrated = false
    enabledAt.value = ''
    readIds.value = new Set()
  }
  if (!prefsHydrated) {
    void ensurePrefsHydrated()
  }
  return true
}

async function ensurePrefsHydrated(signal?: AbortSignal): Promise<boolean> {
  const { name, settled } = resolveUsername()
  if (!settled) {
    prefsHydrated = false
    return false
  }
  if (name !== usernameKey.value) {
    usernameKey.value = name
    prefsHydrated = false
    enabledAt.value = ''
    readIds.value = new Set()
  }
  if (prefsHydrated) return true
  if (prefsFetchPromise) return prefsFetchPromise

  // Pre-init so finally can compare the same handle (avoids TS2454 on self-ref).
  let flight: Promise<boolean> | null = null
  flight = (async () => {
    try {
      const prefs = await fetchServerPrefs(signal)
      // Identity may have flipped while the request was in flight.
      const again = resolveUsername()
      if (!again.settled || again.name !== name) return false
      if (!prefs) return false
      applyPrefs(prefs)
      usernameKey.value = name
      prefsHydrated = true
      return true
    } catch (err) {
      error.value = err instanceof Error && err.message ? err.message : String(err || 'prefs load failed')
      return false
    } finally {
      if (prefsFetchPromise === flight) prefsFetchPromise = null
    }
  })()
  prefsFetchPromise = flight
  return flight
}

function onAuthIdentityChange() {
  const prevKey = usernameKey.value
  const prevHydrated = prefsHydrated
  const { name, settled } = resolveUsername()
  if (!settled) {
    prefsHydrated = false
    return
  }
  if (name !== usernameKey.value) {
    usernameKey.value = name
    prefsHydrated = false
    enabledAt.value = ''
    readIds.value = new Set()
  }
  // Rehydrate + refresh as soon as auth paints — do not wait for focus/15s poll.
  if (!prevHydrated || prevKey !== name) {
    void refresh({ source: 'auth' })
  }
}

function installAuthWatch() {
  if (authWatchInstalled) return
  authWatchInstalled = true
  const { user, ready } = useAuth()
  watch(
    () =>
      [
        // Treat missing ready (legacy mocks) as settled; only ready===false blocks.
        ready ? ready.value : true,
        user.value?.username?.trim() || '',
      ] as const,
    () => {
      onAuthIdentityChange()
    },
    { flush: 'sync' },
  )
}

function markRead(runId: string) {
  if (!runId || readIds.value.has(runId)) return
  const { settled } = resolveUsername()
  if (!settled) return
  // Optimistic local update; server is authoritative.
  const next = new Set(readIds.value)
  next.add(runId)
  readIds.value = next
  void (async () => {
    try {
      const prefs = await api.markNotificationRead(runId)
      applyPrefs({
        enabledAt: prefs.enabledAt || enabledAt.value || new Date().toISOString(),
        readIds: Array.isArray(prefs.readIds) ? prefs.readIds : [...next],
      })
      prefsHydrated = true
    } catch (err) {
      error.value = err instanceof Error && err.message ? err.message : String(err || 'mark read failed')
    }
  })()
}

function markAllRead() {
  const { settled } = resolveUsername()
  if (!settled) return
  const ids = pool.value.map((n) => n.runId)
  const next = new Set(readIds.value)
  for (const id of ids) next.add(id)
  readIds.value = next
  void (async () => {
    try {
      const prefs = await api.markAllNotificationsRead(ids)
      applyPrefs({
        enabledAt: prefs.enabledAt || enabledAt.value || new Date().toISOString(),
        readIds: Array.isArray(prefs.readIds) ? prefs.readIds : [...next],
      })
      prefsHydrated = true
    } catch (err) {
      error.value = err instanceof Error && err.message ? err.message : String(err || 'mark all read failed')
    }
  })()
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
  installAuthWatch()
  const { settled } = resolveUsername()
  // Auth still in flight: keep pool empty of unread semantics until identity settles.
  if (!settled && opts?.source !== 'auth') {
    return
  }
  if (refreshPromise) {
    // Identity flip must not join the prior flight that may have started pre-auth.
    if (opts?.source === 'auth') {
      refreshGeneration++
      abortCtrl?.abort()
      refreshPromise = null
      prefsFetchPromise = null
    } else {
      return refreshPromise
    }
  }

  const gen = ++refreshGeneration
  abortCtrl?.abort()
  abortCtrl = new AbortController()
  const tc = createTimeoutController(DEFAULT_LOADING_TIMEOUT_MS, abortCtrl.signal)

  const holder: { flight: Promise<void> | null } = { flight: null }
  holder.flight = (async () => {
    loading.value = true
    try {
      // Server prefs first — never derive unread from localStorage.
      const hydrated = await ensurePrefsHydrated(tc.signal)
      if (gen !== refreshGeneration) return
      if (!hydrated && opts?.source !== 'auth') {
        // Keep waiting for a successful prefs hydrate on non-auth refreshes.
      }
      const next = await fetchPool(tc.signal)
      if (gen !== refreshGeneration) return
      pool.value = next
      // Do NOT prune/persist readIds against the pool: empty/partial pulls must
      // never clear server-authoritative readIds (clarified business rule).
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
  installAuthWatch()
  stopPolling()
  // If auth is not ready yet, skip mount fetch; auth watch will refresh on settle.
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
  prefsFetchPromise = null
  // Keep authWatchInstalled: watch is idempotent and must survive test resets.
}

/** Test helper: apply server prefs without HTTP (unit tests). */
export function __setRunTerminalPrefsForTests(prefs: NotificationPrefs) {
  applyPrefs(prefs)
  prefsHydrated = true
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
