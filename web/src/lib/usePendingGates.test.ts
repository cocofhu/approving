import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { InboxItem } from '@/lib/types'

vi.mock('@/lib/api', () => ({
  api: {
    listGates: vi.fn(),
  },
  isPaginated: (data: unknown): data is { items: unknown[]; total: number } =>
    data != null && typeof data === 'object' && !Array.isArray(data) && 'items' in data,
}))

import { api } from '@/lib/api'
import { usePendingGates } from '@/lib/usePendingGates'

function gate(id: string): InboxItem {
  return {
    type: 'gate',
    runId: `run-${id}`,
    nodeId: `node-${id}`,
    workflowName: 'Test',
    title: `Gate ${id}`,
    bodyMd: '',
    actions: [{ id: 'approve', label: 'Approve' }],
    requestedAt: '2026-07-04T00:00:00Z',
  }
}

function paged(items: InboxItem[], total = items.length) {
  return { items, total, page: 1, pageSize: 20, hasMore: total > items.length }
}

describe('usePendingGates', () => {
  beforeEach(async () => {
    vi.mocked(api.listGates).mockReset()
    vi.mocked(api.listGates).mockResolvedValue(paged([]))
    await usePendingGates().refresh({ mode: 'force' })
  })

  it('shares items and count across composable instances', async () => {
    const a = usePendingGates()
    const b = usePendingGates()
    expect(a.items).toBe(b.items)
    expect(a.displayedItems).toBe(b.displayedItems)
    expect(a.count).toBe(b.count)

    vi.mocked(api.listGates).mockResolvedValue(paged([gate('1'), gate('2')], 2))
    await a.refresh({ mode: 'force' })
    expect(a.count.value).toBe(2)
    expect(b.count.value).toBe(2)
    expect(a.displayedItems.value).toHaveLength(2)
  })

  it('updates items and count from listGates on force refresh', async () => {
    vi.mocked(api.listGates).mockResolvedValue(paged([gate('1')], 1))
    const { items, count, refresh } = usePendingGates()
    await refresh({ mode: 'force' })
    expect(items.value).toHaveLength(1)
    expect(count.value).toBe(1)
  })

  it('keeps last known items when refresh fails', async () => {
    vi.mocked(api.listGates).mockResolvedValueOnce(paged([gate('1')], 1))
    const { items, count, refresh } = usePendingGates()
    await refresh({ mode: 'force' })
    expect(count.value).toBe(1)

    vi.mocked(api.listGates).mockRejectedValueOnce(new Error('network'))
    await refresh({ mode: 'force' })
    expect(count.value).toBe(1)
    expect(items.value).toHaveLength(1)
  })

  it('deduplicates concurrent refresh calls', async () => {
    let resolve!: (value: ReturnType<typeof paged>) => void
    const pending = new Promise<ReturnType<typeof paged>>((r) => {
      resolve = r
    })
    vi.mocked(api.listGates).mockReturnValue(pending)
    vi.mocked(api.listGates).mockClear()

    const { refresh } = usePendingGates()
    const first = refresh({ mode: 'force' })
    const second = refresh({ mode: 'force' })
    expect(api.listGates).toHaveBeenCalledTimes(1)

    resolve(paged([gate('1')], 1))
    await Promise.all([first, second])
    expect(usePendingGates().count.value).toBe(1)
  })

  it('records lastRefreshSource after a successful refresh', async () => {
    vi.mocked(api.listGates).mockResolvedValue(paged([gate('1')], 1))
    const { refresh, lastRefreshSource } = usePendingGates()
    expect(lastRefreshSource.value).toBeNull()
    await refresh({ source: 'sidebar-poll', mode: 'force' })
    expect(lastRefreshSource.value).toBe('sidebar-poll')
  })

  it('peek updates remoteItems and count without changing displayedItems', async () => {
    vi.mocked(api.listGates).mockResolvedValueOnce(paged([gate('1')], 1))
    const pg = usePendingGates()
    await pg.refresh({ mode: 'force' })
    expect(pg.displayedItems.value).toHaveLength(1)
    expect(pg.count.value).toBe(1)

    vi.mocked(api.listGates).mockResolvedValueOnce(paged([gate('1'), gate('2')], 2))
    await pg.peek({ source: 'sidebar-poll' })

    expect(pg.count.value).toBe(2)
    expect(pg.remoteItems.value).toHaveLength(2)
    expect(pg.displayedItems.value).toHaveLength(1)
    expect(pg.hasPendingUpdate.value).toBe(true)
    expect(pg.pendingMeta.value).toEqual({ added: 1, removed: 0 })
  })

  it('applyPending writes remote snapshot to displayedItems', async () => {
    vi.mocked(api.listGates).mockResolvedValueOnce(paged([gate('1')], 1))
    const pg = usePendingGates()
    await pg.refresh({ mode: 'force' })

    vi.mocked(api.listGates).mockResolvedValueOnce(paged([gate('1'), gate('2')], 2))
    await pg.peek()
    pg.applyPending()

    expect(pg.displayedItems.value).toHaveLength(2)
    expect(pg.hasPendingUpdate.value).toBe(false)
    expect(pg.pendingMeta.value).toBeNull()
  })

  it('peek with no changes clears pending state', async () => {
    vi.mocked(api.listGates).mockResolvedValue(paged([gate('1')], 1))
    const pg = usePendingGates()
    await pg.refresh({ mode: 'force' })
    await pg.peek()
    expect(pg.hasPendingUpdate.value).toBe(false)
  })

  it('peek diff tracks removed items', async () => {
    vi.mocked(api.listGates).mockResolvedValueOnce(paged([gate('1'), gate('2')], 2))
    const pg = usePendingGates()
    await pg.refresh({ mode: 'force' })

    vi.mocked(api.listGates).mockResolvedValueOnce(paged([gate('1')], 1))
    await pg.peek()

    expect(pg.hasPendingUpdate.value).toBe(true)
    expect(pg.pendingMeta.value).toEqual({ added: 0, removed: 1 })
  })

  it('submit source force-refreshes and applies immediately', async () => {
    vi.mocked(api.listGates).mockResolvedValueOnce(paged([gate('1'), gate('2')], 2))
    const pg = usePendingGates()
    await pg.refresh({ mode: 'force' })

    vi.mocked(api.listGates).mockResolvedValueOnce(paged([gate('2')], 1))
    await pg.refresh({ source: 'submit' })

    expect(pg.displayedItems.value).toHaveLength(1)
    expect(pg.hasPendingUpdate.value).toBe(false)
    expect(pg.lastRefreshSource.value).toBe('submit')
  })

  it('deduplicates concurrent peek calls', async () => {
    let resolve!: (value: ReturnType<typeof paged>) => void
    const pending = new Promise<ReturnType<typeof paged>>((r) => {
      resolve = r
    })
    vi.mocked(api.listGates).mockReturnValue(pending)
    vi.mocked(api.listGates).mockClear()

    const pg = usePendingGates()
    const first = pg.peek()
    const second = pg.peek()
    expect(api.listGates).toHaveBeenCalledTimes(1)

    resolve(paged([gate('1')], 1))
    await Promise.all([first, second])
  })
})
