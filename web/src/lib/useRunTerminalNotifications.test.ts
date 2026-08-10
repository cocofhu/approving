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
  finishedApproxIso,
  formatUnreadBadge,
  isNoisyNotificationTitle,
  mapRunToNotification,
  prefsKeyForUser,
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
    durationSec: partial.durationSec ?? 1,
    progress: 100,
    nodeRuns: {},
    artifacts: [],
    ...partial,
  }
}

function paged(items: Run[], total = items.length) {
  return { items, total, page: 1, pageSize: RUN_TERMINAL_POOL_SIZE, hasMore: total > items.length }
}

/** Seed enable baseline in the past so fixtures count as post-enable events. */
function seedBaseline(enabledAt = '2020-01-01T00:00:00Z', readIds: string[] = []) {
  localStorage.setItem(prefsKeyForUser('alice'), JSON.stringify({ enabledAt, readIds }))
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

describe('isNoisyNotificationTitle / mapRunToNotification', () => {
  it('maps completed/failed and rejects other statuses', () => {
    expect(mapRunToNotification(run({ id: 'r1', status: 'completed' }))).toMatchObject({
      runId: 'r1',
      status: 'completed',
    })
    expect(mapRunToNotification(run({ id: 'r2', status: 'failed' }))?.status).toBe('failed')
    expect(mapRunToNotification(run({ id: 'r3', status: 'running' }))).toBeNull()
    expect(mapRunToNotification(run({ id: 'r4', status: 'waiting_human' }))).toBeNull()
    expect(mapRunToNotification(run({ id: 'r5', status: 'cancelled' }))).toBeNull()
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

  it('detects progress-noise titles and uses neutral template', () => {
    expect(isNoisyNotificationTitle('运行中 3 / 暂停 1')).toBe(true)
    expect(isNoisyNotificationTitle('产物这里根据Run 分页')).toBe(false)
    const n = mapRunToNotification(
      run({
        id: 'r1',
        status: 'completed',
        title: '运行中 2 · 等待人工 1',
        workflowName: '自我迭代',
        durationSec: 120,
      }),
    )
    expect(n?.titleNeutral).toBe(true)
    expect(n?.title).toBe('自我迭代 · completed')
    expect(n?.finishedApprox).toBe('2026-08-10T12:02:00.000Z')
  })

  it('computes finishedApprox from startedAt + durationSec', () => {
    expect(
      finishedApproxIso(
        run({ id: 'r1', status: 'completed', startedAt: '2026-08-10T12:00:00Z', durationSec: 90 }),
      ),
    ).toBe('2026-08-10T12:01:30.000Z')
  })
})

describe('useRunTerminalNotifications', () => {
  beforeEach(() => {
    localStorage.clear()
    __resetRunTerminalNotificationsForTests()
    vi.mocked(api.listRuns).mockReset()
    vi.mocked(api.listRuns).mockResolvedValue(paged([]))
  })

  it('loads pool via listRuns(completed,failed) and sorts by finishedApprox desc', async () => {
    seedBaseline()
    const items = [
      run({ id: 'a', status: 'completed', startedAt: '2026-08-10T12:00:00Z', durationSec: 3600 }),
      run({ id: 'b', status: 'failed', startedAt: '2026-08-10T13:00:00Z', durationSec: 60 }),
    ]
    // a finishes 13:00, b finishes 13:01 → b first
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
    expect(n.pool.value.map((x) => x.runId)).toEqual(['b', 'a'])
    expect(n.unreadCount.value).toBe(2)
    expect(n.badgeLabel.value).toBe('2')
    expect(n.hasUnreadFailed.value).toBe(true)
  })

  it('treats pre-enable history as read so badge is not inventory', async () => {
    // Default first-enable: enabledAt ≈ now; fixtures are in the past → all read.
    vi.mocked(api.listRuns).mockResolvedValue(
      paged(
        Array.from({ length: 7 }, (_, i) =>
          run({ id: `r${i}`, status: 'completed', startedAt: '2026-08-01T12:00:00Z' }),
        ),
      ),
    )
    const n = useRunTerminalNotifications()
    await n.refresh({ source: 'mount' })
    expect(n.poolTotal.value).toBe(7)
    expect(n.unreadCount.value).toBe(0)
    expect(n.badgeLabel.value).toBe('')
    expect(n.listItems.value.every((x) => x.unread === false)).toBe(true)
  })

  it('computes unread from per-user prefs read set after baseline', async () => {
    seedBaseline('2020-01-01T00:00:00Z', ['a'])
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

  it('markRead updates only that run and persists prefs', async () => {
    seedBaseline()
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
    const prefs = JSON.parse(localStorage.getItem(prefsKeyForUser('alice')) || '{}')
    expect(prefs.readIds).toContain('a')
    expect(n.previewItems.value.find((x) => x.runId === 'b')?.unread).toBe(true)
  })

  it('markAllRead clears badge and persists all pool ids', async () => {
    seedBaseline()
    vi.mocked(api.listRuns).mockResolvedValue(
      paged([
        run({ id: 'a', status: 'failed' }),
        run({ id: 'b', status: 'completed' }),
      ]),
    )
    const n = useRunTerminalNotifications()
    await n.refresh({ source: 'mount' })
    expect(n.unreadCount.value).toBe(2)
    n.markAllRead()
    expect(n.unreadCount.value).toBe(0)
    expect(n.badgeLabel.value).toBe('')
    const prefs = JSON.parse(localStorage.getItem(prefsKeyForUser('alice')) || '{}')
    expect(prefs.readIds).toEqual(expect.arrayContaining(['a', 'b']))
  })

  it('previewItems caps at 10 and remainingCount is T-10', async () => {
    seedBaseline()
    const items = Array.from({ length: 12 }, (_, i) =>
      run({ id: `r${i}`, status: i % 2 === 0 ? 'completed' : 'failed' }),
    )
    vi.mocked(api.listRuns).mockResolvedValue(paged(items, 12))
    const n = useRunTerminalNotifications()
    await n.refresh({ source: 'manual' })
    expect(n.previewItems.value).toHaveLength(RUN_TERMINAL_PANEL_LIMIT)
    expect(RUN_TERMINAL_PANEL_LIMIT).toBe(10)
    expect(n.remainingCount.value).toBe(2)
    expect(n.poolTotal.value).toBe(12)
  })

  it('badge is empty when unread is 0', async () => {
    seedBaseline()
    vi.mocked(api.listRuns).mockResolvedValue(paged([run({ id: 'a', status: 'completed' })]))
    const n = useRunTerminalNotifications()
    await n.refresh({ source: 'mount' })
    n.markRead('a')
    expect(n.unreadCount.value).toBe(0)
    expect(n.badgeLabel.value).toBe('')
  })

  it('migrates legacy readIds storage into prefs', async () => {
    localStorage.setItem(storageKeyForUser('alice'), JSON.stringify(['legacy-1']))
    vi.mocked(api.listRuns).mockResolvedValue(paged([]))
    const n = useRunTerminalNotifications()
    n.ensureUsername()
    const prefs = JSON.parse(localStorage.getItem(prefsKeyForUser('alice')) || '{}')
    expect(prefs.enabledAt).toBeTruthy()
    expect(prefs.readIds).toContain('legacy-1')
  })
})
