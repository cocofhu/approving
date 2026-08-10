// @vitest-environment happy-dom
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { Run } from '@/lib/types'

vi.mock('@/lib/api', () => ({
  api: {
    listRuns: vi.fn(),
  },
  isPaginated: (data: unknown): data is { items: unknown[]; total: number } =>
    data != null && typeof data === 'object' && !Array.isArray(data) && 'items' in data,
}))

vi.mock('@/lib/useAuth', () => ({
  useAuth: () => ({
    user: { value: { username: 'alice', expiresAt: 't' } },
  }),
}))

import { api } from '@/lib/api'
import {
  __resetRunTerminalNotificationsForTests,
  formatUnreadBadge,
  mapRunToNotification,
  RUN_TERMINAL_PANEL_LIMIT,
  RUN_TERMINAL_POOL_SIZE,
  storageKeyForUser,
  useRunTerminalNotifications,
} from './useRunTerminalNotifications'

function run(partial: Partial<Run> & Pick<Run, 'id' | 'status'>): Run {
  return {
    workflowId: 'wf',
    workflowName: 'demo-wf',
    title: partial.title ?? `Run ${partial.id}`,
    trigger: 'manual',
    startedAt: partial.startedAt ?? '2026-08-10T12:00:00Z',
    durationSec: 1,
    progress: 100,
    nodeRuns: {},
    artifacts: [],
    ...partial,
  }
}

function paged(items: Run[], total = items.length) {
  return { items, total, page: 1, pageSize: RUN_TERMINAL_POOL_SIZE, hasMore: total > items.length }
}

describe('formatUnreadBadge', () => {
  it('hides zero, shows 1–98, caps at 99+', () => {
    expect(formatUnreadBadge(0)).toBe('')
    expect(formatUnreadBadge(1)).toBe('1')
    expect(formatUnreadBadge(98)).toBe('98')
    expect(formatUnreadBadge(99)).toBe('99+')
    expect(formatUnreadBadge(120)).toBe('99+')
  })
})

describe('mapRunToNotification', () => {
  it('maps completed/failed and rejects other statuses', () => {
    expect(mapRunToNotification(run({ id: 'r1', status: 'completed' }))).toMatchObject({
      runId: 'r1',
      status: 'completed',
    })
    expect(mapRunToNotification(run({ id: 'r2', status: 'failed' }))?.status).toBe('failed')
    expect(mapRunToNotification(run({ id: 'r3', status: 'running' }))).toBeNull()
  })

  it('falls back title to workflowName then runId', () => {
    expect(
      mapRunToNotification(run({ id: 'r1', status: 'completed', title: '  ' }))?.title,
    ).toBe('demo-wf')
    expect(
      mapRunToNotification(
        run({ id: 'r2', status: 'completed', title: '', workflowName: '' }),
      )?.title,
    ).toBe('r2')
  })
})

describe('useRunTerminalNotifications', () => {
  beforeEach(() => {
    localStorage.clear()
    __resetRunTerminalNotificationsForTests()
    vi.mocked(api.listRuns).mockReset()
    vi.mocked(api.listRuns).mockResolvedValue(paged([]))
  })

  it('loads pool via listRuns(completed,failed) newest-first window', async () => {
    const items = [
      run({ id: 'a', status: 'completed', startedAt: '2026-08-10T13:00:00Z' }),
      run({ id: 'b', status: 'failed', startedAt: '2026-08-10T12:00:00Z' }),
    ]
    vi.mocked(api.listRuns).mockResolvedValue(paged(items, 2))
    const n = useRunTerminalNotifications()
    await n.refresh({ source: 'mount' })
    expect(api.listRuns).toHaveBeenCalledWith(
      expect.objectContaining({
        status: 'completed,failed',
        page: 1,
        pageSize: RUN_TERMINAL_POOL_SIZE,
        sort: 'started_at',
        order: 'desc',
      }),
    )
    expect(n.pool.value.map((x) => x.runId)).toEqual(['a', 'b'])
    expect(n.unreadCount.value).toBe(2)
    expect(n.badgeLabel.value).toBe('2')
    expect(n.hasUnreadFailed.value).toBe(true)
  })

  it('computes unread from per-user localStorage read set', async () => {
    localStorage.setItem(storageKeyForUser('alice'), JSON.stringify(['a']))
    vi.mocked(api.listRuns).mockResolvedValue(
      paged([
        run({ id: 'a', status: 'completed' }),
        run({ id: 'b', status: 'completed' }),
      ]),
    )
    const n = useRunTerminalNotifications()
    await n.refresh({ source: 'mount' })
    expect(n.unreadCount.value).toBe(1)
    expect(n.badgeLabel.value).toBe('1')
    expect(n.hasUnreadFailed.value).toBe(false)
    expect(n.previewItems.value.find((x) => x.runId === 'a')?.unread).toBe(false)
    expect(n.previewItems.value.find((x) => x.runId === 'b')?.unread).toBe(true)
  })

  it('markRead updates only that run and persists', async () => {
    vi.mocked(api.listRuns).mockResolvedValue(
      paged([
        run({ id: 'a', status: 'failed' }),
        run({ id: 'b', status: 'completed' }),
      ]),
    )
    const n = useRunTerminalNotifications()
    await n.refresh({ source: 'mount' })
    expect(n.unreadCount.value).toBe(2)
    n.markRead('a')
    expect(n.unreadCount.value).toBe(1)
    expect(n.hasUnreadFailed.value).toBe(false)
    expect(JSON.parse(localStorage.getItem(storageKeyForUser('alice')) || '[]')).toContain('a')
    // No batch API — second item stays unread
    expect(n.previewItems.value.find((x) => x.runId === 'b')?.unread).toBe(true)
  })

  it('previewItems caps at 5 and remainingCount is T-5', async () => {
    const items = Array.from({ length: 7 }, (_, i) =>
      run({ id: `r${i}`, status: i % 2 === 0 ? 'completed' : 'failed' }),
    )
    vi.mocked(api.listRuns).mockResolvedValue(paged(items, 7))
    const n = useRunTerminalNotifications()
    await n.refresh({ source: 'manual' })
    expect(n.previewItems.value).toHaveLength(RUN_TERMINAL_PANEL_LIMIT)
    expect(n.remainingCount.value).toBe(2)
    expect(n.poolTotal.value).toBe(7)
  })

  it('badge is empty when unread is 0', async () => {
    vi.mocked(api.listRuns).mockResolvedValue(paged([run({ id: 'a', status: 'completed' })]))
    const n = useRunTerminalNotifications()
    await n.refresh({ source: 'mount' })
    n.markRead('a')
    expect(n.unreadCount.value).toBe(0)
    expect(n.badgeLabel.value).toBe('')
  })
})
