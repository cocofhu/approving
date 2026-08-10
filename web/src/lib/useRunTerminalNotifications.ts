import { computed, ref } from 'vue'
import { api, isPaginated } from '@/lib/api'
import type { Run } from '@/lib/types'
import { createTimeoutController, isAbortError } from '@/lib/loadingRequest'
import { DEFAULT_LOADING_TIMEOUT_MS } from '@/lib/loadingTypes'
import { useAuth } from '@/lib/useAuth'

/** Recent completed/failed runs that form the notification pool (plan: ~50). */
export const RUN_TERMINAL_POOL_SIZE = 50
/** Dropdown preview hard limit. */
export const RUN_TERMINAL_PANEL_LIMIT = 5
/** Align with AppSidebarNav pending-gates peek interval. */
export const RUN_TERMINAL_POLL_MS = 15_000

export type RunTerminalStatus = 'completed' | 'failed'

export interface RunTerminalNotification {
  runId: string
  status: RunTerminalStatus
  title: string
  workflowName: string
  startedAt: string
}

export type RunTerminalRefreshSource =
  | 'mount'
  | 'sidebar-poll'
  | 'visibility'
  | 'focus'
  | 'manual'

export function formatUnreadBadge(n: number): string {
  if (n <= 0) return ''
  if (n >= 99) return '99+'
  return String(n)
}

export function storageKeyForUser(username: string): string {
  return `approving.runTerminalNotifications.readIds.${username || 'anonymous'}`
}

export function mapRunToNotification(run: Run): RunTerminalNotification | null {
  if (run.status !== 'completed' && run.status !== 'failed') return null
  const title = (run.title || '').trim() || run.workflowName || run.id
  return {
    runId: run.id,
    status: run.status,
    title,
    workflowName: run.workflowName || '',
    startedAt: run.startedAt || run.createdAt || '',
  }
}

function loadReadIds(username: string): Set<string> {
  if (typeof localStorage === 'undefined') return new Set()
  try {
    const raw = localStorage.getItem(storageKeyForUser(username))
    if (!raw) return new Set()
    const parsed = JSON.parse(raw) as unknown
    if (!Array.isArray(parsed)) return new Set()
    return new Set(parsed.filter((x): x is string => typeof x === 'string'))
  } catch {
    return new Set()
  }
}

function persistReadIds(username: string, ids: Set<string>) {
  if (typeof localStorage === 'undefined') return
  try {
    localStorage.setItem(storageKeyForUser(username), JSON.stringify([...ids]))
  } catch {
    // Quota / private mode — ignore; unread will reset next session.
  }
}

const pool = ref<RunTerminalNotification[]>([])
const readIds = ref<Set<string>>(new Set())
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
let readIdsHydrated = false

const unreadCount = computed(
  () => pool.value.filter((n) => !readIds.value.has(n.runId)).length,
)

const hasUnreadFailed = computed(() =>
  pool.value.some((n) => !readIds.value.has(n.runId) && n.status === 'failed'),
)

const badgeLabel = computed(() => formatUnreadBadge(unreadCount.value))

const previewItems = computed(() =>
  pool.value.slice(0, RUN_TERMINAL_PANEL_LIMIT).map((n) => ({
    ...n,
    unread: !readIds.value.has(n.runId),
  })),
)

const remainingCount = computed(() => Math.max(pool.value.length - RUN_TERMINAL_PANEL_LIMIT, 0))

const poolTotal = computed(() => pool.value.length)

function ensureUsername() {
  const { user } = useAuth()
  const next = user.value?.username?.trim() || 'anonymous'
  if (next !== usernameKey.value) {
    usernameKey.value = next
    readIds.value = loadReadIds(next)
    readIdsHydrated = true
    return
  }
  if (!readIdsHydrated) {
    readIds.value = loadReadIds(next)
    readIdsHydrated = true
  }
}

function markRead(runId: string) {
  if (!runId || readIds.value.has(runId)) return
  const next = new Set(readIds.value)
  next.add(runId)
  readIds.value = next
  persistReadIds(usernameKey.value, next)
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
        persistReadIds(usernameKey.value, pruned)
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
  usernameKey.value = 'anonymous'
  error.value = null
  lastRefreshSource.value = null
  lastPeekAt.value = 0
  loading.value = false
  refreshPromise = null
  refreshGeneration = 0
  readIdsHydrated = false
}

export function useRunTerminalNotifications() {
  return {
    pool,
    poolTotal,
    previewItems,
    remainingCount,
    unreadCount,
    hasUnreadFailed,
    badgeLabel,
    error,
    loading,
    lastRefreshSource,
    lastPeekAt,
    markRead,
    refresh,
    startPolling,
    stopPolling,
    ensureUsername,
  }
}
