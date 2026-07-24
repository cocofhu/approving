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

let refreshPromise: Promise<void> | null = null

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

async function peek(opts?: PeekOptions): Promise<void> {
  if (refreshPromise) return refreshPromise

  refreshPromise = (async () => {
    try {
      const { items: remote, total } = await fetchPeek()
      lastRefreshSource.value = opts?.source ?? null
      setPending(remote, total)
      lastPeekAt.value = Date.now()
    } catch {
      /* keep last known items on transient errors */
    } finally {
      refreshPromise = null
    }
  })()

  return refreshPromise
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

  if (refreshPromise) return refreshPromise

  refreshPromise = (async () => {
    try {
      const { items: remote, total } = await fetchPeek()
      lastRefreshSource.value = opts?.source ?? null
      applyRemoteToDisplayed(remote, total)
    } catch {
      /* keep last known items on transient errors */
    } finally {
      refreshPromise = null
    }
  })()

  return refreshPromise
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
    itemKey,
  }
}
