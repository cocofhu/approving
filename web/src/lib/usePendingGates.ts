import { ref, computed } from 'vue'
import { api, isPaginated } from '@/lib/api'
import { i18n } from '@/lib/i18n'
import { beginRefresh, endRefresh } from '@/lib/refreshChrome'
import { createTimeoutController, isAbortError } from '@/lib/loadingRequest'
import { DEFAULT_LOADING_TIMEOUT_MS } from '@/lib/loadingTypes'
import type { InboxItem } from '@/lib/types'

export type RefreshSource =
  | 'sidebar-poll'
  | 'visibility'
  | 'focus'
  | 'navigate'
  | 'submit'
  | 'mount'
  | 'manual'

export interface RefreshOptions {
  source?: RefreshSource
  mode?: 'force' | 'peek'
}

export interface PeekOptions {
  source?: RefreshSource
}

export interface PendingMeta {
  added: number
  removed: number
}

export function itemKey(it: InboxItem): string {
  return `${it.runId}:${it.nodeId}`
}

function diffKeys(displayed: InboxItem[], remote: InboxItem[]): PendingMeta {
  const displayedSet = new Set(displayed.map(itemKey))
  const remoteSet = new Set(remote.map(itemKey))
  let added = 0
  let removed = 0
  for (const k of remoteSet) {
    if (!displayedSet.has(k)) added++
  }
  for (const k of displayedSet) {
    if (!remoteSet.has(k)) removed++
  }
  return { added, removed }
}

function errorMessage(err: unknown): string {
  if (err instanceof Error && err.message) return err.message
  return String(err || 'load failed')
}

// Module-level singleton
const displayedItems = ref<InboxItem[]>([])
const remoteItems = ref<InboxItem[]>([])
const totalCount = ref(0)
const hasPendingUpdate = ref(false)
const pendingMeta = ref<PendingMeta | null>(null)
const lastRefreshSource = ref<RefreshSource | null>(null)
const lastPeekAt = ref(0)
const error = ref<string | null>(null)
const ariaBusy = ref(false)

// Backward-compatible alias: list UI binds to displayedItems.
const items = displayedItems
const count = computed(() => totalCount.value)

/** Separate flight slots so force/submit never joins an in-flight peek. */
let peekPromise: Promise<void> | null = null
let forcePromise: Promise<void> | null = null
/**
 * Monotonic generation: bumping invalidates in-flight peek/force writebacks.
 * Force always bumps so a slower peek cannot overwrite with setPending.
 */
let refreshGeneration = 0
let abortCtrl: AbortController | null = null

async function fetchPeek(signal?: AbortSignal): Promise<{ items: InboxItem[]; total: number }> {
  const data = await api.listGates({ page: 1, pageSize: 20, signal })
  if (isPaginated(data)) {
    return { items: data.items, total: data.total }
  }
  return { items: data, total: data.length }
}

function setPending(remote: InboxItem[], total: number) {
  remoteItems.value = remote
  totalCount.value = total
  const meta = diffKeys(displayedItems.value, remote)
  if (meta.added > 0 || meta.removed > 0) {
    hasPendingUpdate.value = true
    pendingMeta.value = meta
  } else {
    hasPendingUpdate.value = false
    pendingMeta.value = null
  }
}

function applyRemoteToDisplayed(remote: InboxItem[], total: number) {
  remoteItems.value = remote
  totalCount.value = total
  displayedItems.value = remote
  hasPendingUpdate.value = false
  pendingMeta.value = null
}

function syncPendingMetaFromDiff() {
  const meta = diffKeys(displayedItems.value, remoteItems.value)
  if (meta.added > 0 || meta.removed > 0) {
    hasPendingUpdate.value = true
    pendingMeta.value = meta
  } else {
    hasPendingUpdate.value = false
    pendingMeta.value = null
  }
}

function syncAriaBusy() {
  ariaBusy.value = peekPromise != null || forcePromise != null
}

/**
 * Optimistic local removal after approve/reject/clarify-force succeeds.
 * Keeps sidebar totalCount/displayedItems aligned even when force refresh fails
 * or a stale peek would otherwise linger.
 */
function removeItemLocally(key: string): void {
  const inDisplayed = displayedItems.value.some((it) => itemKey(it) === key)
  const inRemote = remoteItems.value.some((it) => itemKey(it) === key)
  if (!inDisplayed && !inRemote) return

  displayedItems.value = displayedItems.value.filter((it) => itemKey(it) !== key)
  remoteItems.value = remoteItems.value.filter((it) => itemKey(it) !== key)
  if (inDisplayed || inRemote) {
    totalCount.value = Math.max(0, totalCount.value - 1)
  }
  syncPendingMetaFromDiff()
}

async function peek(opts?: PeekOptions): Promise<void> {
  // Prefer awaiting in-flight force: it applies to displayed and is fresher.
  if (forcePromise) return forcePromise
  if (peekPromise) return peekPromise

  const gen = ++refreshGeneration
  abortCtrl?.abort()
  abortCtrl = new AbortController()
  const tc = createTimeoutController(DEFAULT_LOADING_TIMEOUT_MS, abortCtrl.signal)
  // Holder avoids TS2454: const flight = (async () => flight)() is "used before assigned".
  const holder: { flight: Promise<void> | null } = { flight: null }
  holder.flight = (async () => {
    ariaBusy.value = true
    try {
      const { items: remote, total } = await fetchPeek(tc.signal)
      // Discard if a newer peek/force superseded this request.
      if (gen !== refreshGeneration) return
      lastRefreshSource.value = opts?.source ?? null
      error.value = null
      setPending(remote, total)
      lastPeekAt.value = Date.now()
    } catch (err) {
      if (gen !== refreshGeneration) return
      if (isAbortError(err) && !tc.timedOut) return
      error.value = tc.timedOut ? String(i18n.global.t('common.loading.timeout')) : errorMessage(err)
    } finally {
      tc.clear()
      if (peekPromise === holder.flight) peekPromise = null
      syncAriaBusy()
    }
  })()
  peekPromise = holder.flight
  return holder.flight
}

function applyPending(): void {
  if (!hasPendingUpdate.value) return
  displayedItems.value = [...remoteItems.value]
  hasPendingUpdate.value = false
  pendingMeta.value = null
}

async function refresh(opts?: RefreshOptions): Promise<void> {
  if (opts?.mode === 'peek') {
    return peek({ source: opts?.source })
  }

  const force =
    opts?.mode === 'force' ||
    opts?.source === 'submit' ||
    opts?.source === 'navigate' ||
    opts?.source === 'mount' ||
    opts?.source === 'manual'

  if (!force) {
    return peek({ source: opts?.source })
  }

  // Deduplicate concurrent force calls, but never join an in-flight peek.
  if (forcePromise) return forcePromise

  const isManual = opts?.source === 'manual'

  // Invalidate any in-flight peek writeback (setPending) before we apply.
  const gen = ++refreshGeneration
  abortCtrl?.abort()
  abortCtrl = new AbortController()
  const tc = createTimeoutController(DEFAULT_LOADING_TIMEOUT_MS, abortCtrl.signal)
  // Holder avoids TS2454 on self-referential flight cleanup.
  const holder: { flight: Promise<void> | null } = { flight: null }
  holder.flight = (async () => {
    ariaBusy.value = true
    if (isManual) beginRefresh('user_initiated')
    try {
      const { items: remote, total } = await fetchPeek(tc.signal)
      if (gen !== refreshGeneration) return
      lastRefreshSource.value = opts?.source ?? null
      error.value = null
      applyRemoteToDisplayed(remote, total)
    } catch (err) {
      if (gen !== refreshGeneration) return
      if (isAbortError(err) && !tc.timedOut) return
      error.value = tc.timedOut ? String(i18n.global.t('common.loading.timeout')) : errorMessage(err)
    } finally {
      tc.clear()
      if (forcePromise === holder.flight) forcePromise = null
      syncAriaBusy()
      if (isManual) endRefresh()
    }
  })()
  forcePromise = holder.flight
  return holder.flight
}

export function usePendingGates() {
  return {
    items,
    displayedItems,
    remoteItems,
    totalCount,
    count,
    hasPendingUpdate,
    pendingMeta,
    lastRefreshSource,
    lastPeekAt,
    error,
    ariaBusy,
    refresh,
    peek,
    applyPending,
    removeItemLocally,
    itemKey,
  }
}
