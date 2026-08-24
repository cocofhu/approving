// @vitest-environment happy-dom
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'
import type { Run } from '@/lib/shared/types'

const authUser = ref<{ username: string; expiresAt: string } | null>({
  username: 'alice',
  expiresAt: 't',
})
const authReady = ref(true)

const serverPrefs = ref({ enabledAt: '2020-01-01T00:00:00Z', readIds: [] as string[] })

vi.mock('@/lib/api/api', () => ({
  api: {
    listRuns: vi.fn(),
    getNotificationReadPrefs: vi.fn(async () => ({
      enabledAt: serverPrefs.value.enabledAt,
      readIds: [...serverPrefs.value.readIds],
    })),
    markNotificationRead: vi.fn(async (runId: string) => {
      if (!serverPrefs.value.readIds.includes(runId)) {
        serverPrefs.value = {
          ...serverPrefs.value,
          readIds: [...serverPrefs.value.readIds, runId],
        }
      }
      return { enabledAt: serverPrefs.value.enabledAt, readIds: [...serverPrefs.value.readIds] }
    }),
    markAllNotificationsRead: vi.fn(async (runIds: string[]) => {
      const next = new Set(serverPrefs.value.readIds)
      for (const id of runIds) next.add(id)
      serverPrefs.value = { ...serverPrefs.value, readIds: [...next] }
      return { enabledAt: serverPrefs.value.enabledAt, readIds: [...serverPrefs.value.readIds] }
    }),
  },
  isPaginated: (data: unknown): data is { items: unknown[]; total: number } =>
    data != null && typeof data === 'object' && !Array.isArray(data) && 'items' in data,
}))

vi.mock('@/lib/composables/useAuth', () => ({
  useAuth: () => ({
    user: authUser,
    ready: authReady,
  }),
}))

import { api } from '@/lib/api/api'
import {
  __resetRunTerminalNotificationsForTests,
  finishedApproxIso,
  formatUnreadBadge,
  isNoisyNotificationTitle,
  mapRunToNotification,
  isBeforeBaseline,
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

/** Seed server prefs baseline (mock API store). */
function seedBaseline(enabledAt = '2020-01-01T00:00:00Z', readIds: string[] = []) {
  serverPrefs.value = { enabledAt, readIds: [...readIds] }
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
    authUser.value = { username: 'alice', expiresAt: 't' }
    authReady.value = true
    seedBaseline('2020-01-01T00:00:00Z', [])
    __resetRunTerminalNotificationsForTests()
    vi.mocked(api.listRuns).mockReset()
    vi.mocked(api.listRuns).mockResolvedValue(paged([]))
    vi.mocked(api.getNotificationReadPrefs).mockClear()
    vi.mocked(api.markNotificationRead).mockClear()
    vi.mocked(api.markAllNotificationsRead).mockClear()
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
    expect(api.getNotificationReadPrefs).toHaveBeenCalled()
    expect(n.pool.value.map((x) => x.runId)).toEqual(['b', 'a'])
    expect(n.unreadCount.value).toBe(2)
    expect(n.badgeLabel.value).toBe('2')
    expect(n.hasUnreadFailed.value).toBe(true)
  })

  it('treats pre-enable history as read so badge is not inventory', async () => {
    // First-enable: server returns enabledAt ≈ now; fixtures are in the past → all read.
    const now = new Date().toISOString()
    seedBaseline(now, [])
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
    // History before enable baseline stays visible and is marked beforeBaseline.
    expect(n.listItems.value.every((x) => x.beforeBaseline === true)).toBe(true)
    expect(
      isBeforeBaseline(
        { finishedApprox: '2026-08-01T12:00:00Z', startedAt: '2026-08-01T12:00:00Z' },
        n.enabledAt.value,
      ),
    ).toBe(true)
  })

  it('computes unread from server prefs read set after baseline', async () => {
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

  it('markRead updates only that run and posts to server', async () => {
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
    await vi.waitFor(() => {
      expect(api.markNotificationRead).toHaveBeenCalledWith('a')
      expect(serverPrefs.value.readIds).toContain('a')
    })
    // localStorage must not be the authority.
    expect(localStorage.getItem(prefsKeyForUser('alice'))).toBeNull()
    expect(n.previewItems.value.find((x) => x.runId === 'b')?.unread).toBe(true)
  })

  it('markAllRead clears badge and posts all pool ids', async () => {
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
    await vi.waitFor(() => {
      expect(api.markAllNotificationsRead).toHaveBeenCalledWith(expect.arrayContaining(['a', 'b']))
      expect(serverPrefs.value.readIds).toEqual(expect.arrayContaining(['a', 'b']))
    })
    expect(localStorage.getItem(prefsKeyForUser('alice'))).toBeNull()
  })

  it('previewItems caps at 5 and remainingCount is T-5', async () => {
    seedBaseline()
    const items = Array.from({ length: 12 }, (_, i) =>
      run({ id: `r${i}`, status: i % 2 === 0 ? 'completed' : 'failed' }),
    )
    vi.mocked(api.listRuns).mockResolvedValue(paged(items, 12))
    const n = useRunTerminalNotifications()
    await n.refresh({ source: 'manual' })
    expect(n.previewItems.value).toHaveLength(RUN_TERMINAL_PANEL_LIMIT)
    expect(RUN_TERMINAL_PANEL_LIMIT).toBe(5)
    expect(n.remainingCount.value).toBe(7)
    expect(n.poolTotal.value).toBe(12)
  })

  it('marks post-baseline items without beforeBaseline and keeps them unread until read', async () => {
    seedBaseline('2020-01-01T00:00:00Z')
    vi.mocked(api.listRuns).mockResolvedValue(
      paged([
        run({ id: 'new', status: 'completed', startedAt: '2026-08-10T12:00:00Z' }),
        run({ id: 'old', status: 'completed', startedAt: '2019-06-01T12:00:00Z' }),
      ]),
    )
    const n = useRunTerminalNotifications()
    await n.refresh({ source: 'mount' })
    const newer = n.listItems.value.find((x) => x.runId === 'new')
    const older = n.listItems.value.find((x) => x.runId === 'old')
    expect(newer?.beforeBaseline).toBe(false)
    expect(newer?.unread).toBe(true)
    expect(older?.beforeBaseline).toBe(true)
    expect(older?.unread).toBe(false)
    expect(n.unreadCount.value).toBe(1)
    // beforeBaseline items still appear in preview/list
    expect(n.listItems.value.map((x) => x.runId)).toEqual(expect.arrayContaining(['new', 'old']))
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

  it('does not use localStorage as prefs authority (cleared localStorage still uses server)', async () => {
    seedBaseline('2020-01-01T00:00:00Z', ['legacy-1'])
    localStorage.setItem(prefsKeyForUser('alice'), JSON.stringify({ enabledAt: '1999-01-01T00:00:00Z', readIds: [] }))
    localStorage.setItem(storageKeyForUser('alice'), JSON.stringify(['ignored']))
    vi.mocked(api.listRuns).mockResolvedValue(paged([run({ id: 'legacy-1', status: 'completed' })]))
    const n = useRunTerminalNotifications()
    await n.refresh({ source: 'mount' })
    expect(n.unreadCount.value).toBe(0)
    expect(n.previewItems.value.find((x) => x.runId === 'legacy-1')?.unread).toBe(false)
    // Clearing localStorage does not change server-hydrated read state.
    localStorage.clear()
    expect(n.unreadCount.value).toBe(0)
  })

  it('does not hydrate anonymous prefs before auth; rehydrates on user settle without focus', async () => {
    // Drop ready before clearing user so we never briefly settle as anonymous.
    authReady.value = false
    authUser.value = null
    seedBaseline('2020-01-01T00:00:00Z', [])
    localStorage.removeItem(prefsKeyForUser('anonymous'))
    vi.mocked(api.listRuns).mockResolvedValue(
      paged([
        run({ id: 'a', status: 'completed' }),
        run({ id: 'b', status: 'failed' }),
        run({ id: 'c', status: 'completed' }),
      ]),
    )

    const n = useRunTerminalNotifications()
    n.startPolling()
    // Pre-auth: no anonymous prefs stamp, unread stays 0 (no baseline).
    expect(localStorage.getItem(prefsKeyForUser('anonymous'))).toBeNull()
    expect(n.ensureUsername()).toBe(false)
    expect(n.unreadCount.value).toBe(0)
    expect(n.badgeLabel.value).toBe('')

    // Auth paints: sync watch rehydrates user prefs and refreshes pool.
    authUser.value = { username: 'alice', expiresAt: 't' }
    authReady.value = true
    await vi.waitFor(() => {
      expect(n.unreadCount.value).toBe(3)
      expect(n.badgeLabel.value).toBe('3')
    })
    expect(localStorage.getItem(prefsKeyForUser('anonymous'))).toBeNull()
    n.stopPolling()
  })

  it('markRead survives module reset + auth rehydrate (page refresh)', async () => {
    seedBaseline()
    vi.mocked(api.listRuns).mockResolvedValue(
      paged([
        run({ id: 'a', status: 'failed' }),
        run({ id: 'b', status: 'completed' }),
      ]),
    )
    const n1 = useRunTerminalNotifications()
    await n1.refresh({ source: 'mount' })
    expect(n1.unreadCount.value).toBe(2)
    n1.markRead('a')
    await vi.waitFor(() => expect(serverPrefs.value.readIds).toContain('a'))
    expect(n1.unreadCount.value).toBe(1)

    __resetRunTerminalNotificationsForTests()
    authReady.value = false
    authUser.value = null

    const n2 = useRunTerminalNotifications()
    n2.startPolling()
    expect(n2.ensureUsername()).toBe(false)
    expect(n2.unreadCount.value).toBe(0)

    authUser.value = { username: 'alice', expiresAt: 't' }
    authReady.value = true
    await vi.waitFor(() => {
      expect(n2.unreadCount.value).toBe(1)
      expect(n2.badgeLabel.value).toBe('1')
    })
    expect(n2.previewItems.value.find((x) => x.runId === 'a')?.unread).toBe(false)
    expect(n2.previewItems.value.find((x) => x.runId === 'b')?.unread).toBe(true)
    expect(localStorage.getItem(prefsKeyForUser('anonymous'))).toBeNull()
    n2.stopPolling()
  })

  it('markAllRead survives logout and same-account re-login', async () => {
    seedBaseline()
    vi.mocked(api.listRuns).mockResolvedValue(
      paged([
        run({ id: 'a', status: 'failed' }),
        run({ id: 'b', status: 'completed' }),
      ]),
    )
    const n1 = useRunTerminalNotifications()
    await n1.refresh({ source: 'mount' })
    expect(n1.unreadCount.value).toBe(2)
    n1.markAllRead()
    await vi.waitFor(() =>
      expect(serverPrefs.value.readIds).toEqual(expect.arrayContaining(['a', 'b'])),
    )
    expect(n1.unreadCount.value).toBe(0)

    authUser.value = null
    authReady.value = true
    __resetRunTerminalNotificationsForTests()

    const n2 = useRunTerminalNotifications()
    n2.startPolling()
    await vi.waitFor(() => {
      expect(n2.ensureUsername()).toBe(true)
    })
    // anonymous settle may hydrate empty/new prefs; re-login as alice restores server set.
    authUser.value = { username: 'alice', expiresAt: 't' }
    await vi.waitFor(() => {
      expect(n2.unreadCount.value).toBe(0)
      expect(n2.badgeLabel.value).toBe('')
    })
    expect(serverPrefs.value.readIds).toEqual(expect.arrayContaining(['a', 'b']))
    n2.stopPolling()
  })

  it('markRead before auth settle does not call server or stamp anonymous prefs', async () => {
    authReady.value = false
    authUser.value = null
    localStorage.removeItem(prefsKeyForUser('anonymous'))
    localStorage.removeItem(prefsKeyForUser('alice'))

    const n = useRunTerminalNotifications()
    n.markRead('ghost')
    expect(api.markNotificationRead).not.toHaveBeenCalled()
    expect(localStorage.getItem(prefsKeyForUser('anonymous'))).toBeNull()
    expect(localStorage.getItem(prefsKeyForUser('alice'))).toBeNull()
  })

  it('markAllRead before auth settle does not call server or stamp anonymous prefs', async () => {
    authReady.value = false
    authUser.value = null
    localStorage.removeItem(prefsKeyForUser('anonymous'))
    localStorage.removeItem(prefsKeyForUser('alice'))

    const n = useRunTerminalNotifications()
    n.markAllRead()
    expect(api.markAllNotificationsRead).not.toHaveBeenCalled()
    expect(localStorage.getItem(prefsKeyForUser('anonymous'))).toBeNull()
    expect(localStorage.getItem(prefsKeyForUser('alice'))).toBeNull()
  })

  it('new terminal events after markAllRead still count as unread', async () => {
    seedBaseline()
    vi.mocked(api.listRuns).mockResolvedValue(
      paged([run({ id: 'a', status: 'completed' })]),
    )
    const n = useRunTerminalNotifications()
    await n.refresh({ source: 'mount' })
    expect(n.unreadCount.value).toBe(1)
    n.markAllRead()
    await vi.waitFor(() => expect(serverPrefs.value.readIds).toContain('a'))
    expect(n.unreadCount.value).toBe(0)

    vi.mocked(api.listRuns).mockResolvedValue(
      paged([
        run({ id: 'new', status: 'failed', startedAt: '2026-08-10T14:00:00Z' }),
        run({ id: 'a', status: 'completed' }),
      ]),
    )
    await n.refresh({ source: 'manual' })
    expect(n.unreadCount.value).toBe(1)
    expect(n.previewItems.value.find((x) => x.runId === 'new')?.unread).toBe(true)
    expect(n.previewItems.value.find((x) => x.runId === 'a')?.unread).toBe(false)
  })

  it('empty pool refresh does not clear server readIds', async () => {
    seedBaseline('2020-01-01T00:00:00Z', ['kept-id'])
    vi.mocked(api.listRuns).mockResolvedValue(paged([]))
    const n = useRunTerminalNotifications()
    await n.refresh({ source: 'mount' })
    expect(serverPrefs.value.readIds).toContain('kept-id')
    expect(api.markNotificationRead).not.toHaveBeenCalled()
    expect(api.markAllNotificationsRead).not.toHaveBeenCalled()
    // Rehydrate still sees kept-id from server.
    __resetRunTerminalNotificationsForTests()
    await n.refresh({ source: 'manual' })
    expect(n.enabledAt.value).toBe('2020-01-01T00:00:00Z')
    // pool empty → unread 0, but server prefs untouched
    expect(serverPrefs.value.readIds).toEqual(['kept-id'])
  })
})
