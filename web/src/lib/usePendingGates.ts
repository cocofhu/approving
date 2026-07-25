import { ref, computed } from 'vue'
import { api, isPaginated } from '@/lib/api'
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

// Module-level singleton
const displayedItems = ref<InboxItem[]>([])
const remoteItems = ref<InboxItem[]>([])
const totalCount = ref(0)
const hasPendingUpdate = ref(false)
const pendingMeta = ref<PendingMeta | null>(null)
const lastRefreshSource = ref<RefreshSource | null>(null)
const lastPeekAt = ref(0)

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

async function fetchPeek(): Promise<{ items: InboxItem[]; total: number }> {
  const data = await api.listGates({ page: 1, pageSize: 20 })
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
  const flight = (async () => {
    try {
      const { items: remote, total } = await fetchPeek()
      // Discard if a newer peek/force superseded this request.
      if (gen !== refreshGeneration) return
      lastRefreshSource.value = opts?.source ?? null
      setPending(remote, total)
      lastPeekAt.value = Date.now()
    } catch {
      /* keep last known items on transient errors */
    } finally {
      if (peekPromise === flight) peekPromise = null
    }
  })()
  peekPromise = flight
  return flight
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

  // Invalidate any in-flight peek writeback (setPending) before we apply.
  const gen = ++refreshGeneration
  const flight = (async () => {
    try {
      const { items: remote, total } = await fetchPeek()
      if (gen !== refreshGeneration) return
      lastRefreshSource.value = opts?.source ?? null
      applyRemoteToDisplayed(remote, total)
    } catch {
      /* keep last known items on transient errors */
    } finally {
      if (forcePromise === flight) forcePromise = null
    }
  })()
  forcePromise = flight
  return flight
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
    refresh,
    peek,
    applyPending,
    removeItemLocally,
    itemKey,
  }
}
