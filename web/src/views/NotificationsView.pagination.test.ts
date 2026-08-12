// @vitest-environment happy-dom
import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { createI18n } from 'vue-i18n'
import { createMemoryHistory, createRouter } from 'vue-router'
import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick, ref } from 'vue'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'
import shell from '@/locales/zh-CN/shell.json'
import type { Run } from '@/lib/shared/types'
import { requestNotificationsPageReset, __resetNotificationsPageEntryForTests } from '@/lib/composables/useNotificationsPageEntry'

vi.mock('@/lib/composables/useBreakpoint', () => ({
  useBreakpoint: () => ({ isMobile: ref(false) }),
}))

vi.mock('@/lib/composables/useAuth', () => ({
  useAuth: () => ({
    user: ref({ username: 'tester', expiresAt: 't' }),
    ready: ref(true),
  }),
}))

vi.mock('@/lib/api/api', () => ({
  api: {
    listRuns: vi.fn(),
    getRun: vi.fn(),
    artifactContent: vi.fn(),
    artifactDownloadUrl: vi.fn((id: string) => `http://test/api/artifacts/${id}/download`),
  },
  isPaginated: (data: unknown): data is { items: unknown[]; total: number } =>
    data != null && typeof data === 'object' && !Array.isArray(data) && 'items' in data,
}))

import { api } from '@/lib/api/api'
import {
  __resetRunTerminalNotificationsForTests,
  prefsKeyForUser,
  RUN_TERMINAL_PANEL_LIMIT,
  RUN_TERMINAL_POOL_SIZE,
  useRunTerminalNotifications,
} from '@/lib/run/useRunTerminalNotifications'
import NotificationsView from './NotificationsView.vue'

const viewSrc = readFileSync(join(dirname(fileURLToPath(import.meta.url)), 'NotificationsView.vue'), 'utf8')
const poolSrc = readFileSync(
  join(dirname(fileURLToPath(import.meta.url)), '../lib/run/useRunTerminalNotifications.ts'),
  'utf8',
)

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
  return { items, total, page: 1, pageSize: RUN_TERMINAL_POOL_SIZE, hasMore: false }
}

function seedBaseline(enabledAt = '2020-01-01T00:00:00Z', readIds: string[] = []) {
  localStorage.setItem(prefsKeyForUser('tester'), JSON.stringify({ enabledAt, readIds }))
}

function makeItems(n: number, unreadCount = n) {
  return Array.from({ length: n }, (_, i) =>
    run({
      id: `n-${String(i).padStart(2, '0')}`,
      status: i % 7 === 0 ? 'failed' : 'completed',
      startedAt: `2026-08-10T${String(10 + Math.floor(i / 60)).padStart(2, '0')}:${String(i % 60).padStart(2, '0')}:00Z`,
      title: `通知 ${i}`,
    }),
  ).map((item, i) => {
    if (i >= unreadCount) {
      return item
    }
    return item
  })
}

async function mountView(items: Run[], readIds: string[] = []) {
  seedBaseline('2020-01-01T00:00:00Z', readIds)
  vi.mocked(api.listRuns).mockResolvedValue(paged(items, items.length))
  const i18n = createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...common, ...pages, ...shell } },
  })
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/notifications', component: NotificationsView },
      { path: '/runs/:id', component: { template: '<div data-testid="run-detail" />' } },
    ],
  })
  await router.push('/notifications')
  await router.isReady()
  const wrapper = mount(NotificationsView, {
    global: {
      plugins: [i18n, router],
      stubs: { Icon: true, Transition: false },
    },
  })
  await flushPromises()
  return { wrapper, router }
}

describe('NotificationsView pagination (g1/g2/g4)', () => {
  beforeEach(() => {
    localStorage.clear()
    __resetRunTerminalNotificationsForTests()
    __resetNotificationsPageEntryForTests()
    vi.mocked(api.listRuns).mockReset()
    vi.mocked(api.listRuns).mockResolvedValue(paged([]))
  })

  afterEach(() => {
    __resetRunTerminalNotificationsForTests()
    __resetNotificationsPageEntryForTests()
  })

  it('N=20 has no pager and shows page-1 range 1–20 / 20 (g1.2 / g2.1 / g4.1)', async () => {
    const { wrapper } = await mountView(makeItems(20))
    expect(wrapper.findAll('[data-testid="notifications-item"]')).toHaveLength(20)
    expect(wrapper.find('[data-testid="notifications-pagination"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="notifications-page-range"]').text()).toBe('第 1 页 · 1–20 / 20')
    wrapper.unmount()
  })

  it('N=21 shows two pages and slices 20+1 (g1.1 / g1.2 / g4.1)', async () => {
    const { wrapper } = await mountView(makeItems(21))
    expect(wrapper.findAll('[data-testid="notifications-item"]')).toHaveLength(20)
    const pager = wrapper.find('[data-testid="notifications-pagination"]')
    expect(pager.exists()).toBe(true)
    expect(wrapper.find('[data-testid="notifications-pager-summary"]').text()).toBe('共 21 条 · 每页 20')
    expect(wrapper.find('[data-testid="notifications-page-range"]').text()).toBe('第 1 页 · 1–20 / 21')

    const next = pager.findAll('button.pg-btn')[1]!
    await next.trigger('click')
    await nextTick()
    expect(wrapper.findAll('[data-testid="notifications-item"]')).toHaveLength(1)
    expect(wrapper.find('[data-testid="notifications-page-range"]').text()).toBe('第 2 页 · 21–21 / 21')
    wrapper.unmount()
  })

  it('switching filter resets to page 1 and may hide pager (g1.3 / g4.1)', async () => {
    const items = makeItems(21)
    const { wrapper } = await mountView(items, items.slice(0, 18).map((r) => r.id))
    await wrapper.find('[data-testid="notifications-pagination"]').findAll('button.pg-btn')[1]!.trigger('click')
    await nextTick()
    expect(wrapper.find('[data-testid="notifications-page-range"]').text()).toMatch(/第 2 页/)

    await wrapper.find('[data-testid="notifications-filter-unread"]').trigger('click')
    await nextTick()
    expect(wrapper.find('[data-testid="notifications-page-range"]').text()).toMatch(/第 1 页/)
    expect(wrapper.find('[data-testid="notifications-pagination"]').exists()).toBe(false)
    expect(wrapper.findAll('[data-testid="notifications-item"]').length).toBeLessThanOrEqual(20)
    wrapper.unmount()
  })

  it('clamps to last data page when current unread page becomes empty (g1.4 / g4.1)', async () => {
    const items = makeItems(21)
    const { wrapper } = await mountView(items)
    await wrapper.find('[data-testid="notifications-filter-unread"]').trigger('click')
    await wrapper.find('[data-testid="notifications-pagination"]').findAll('button.pg-btn')[1]!.trigger('click')
    await nextTick()
    expect(wrapper.findAll('[data-testid="notifications-item"]')).toHaveLength(1)
    const lastId = wrapper.find('[data-testid="notifications-item"]').attributes('data-run-id')!
    useRunTerminalNotifications().markRead(lastId)
    await nextTick()
    await nextTick()
    expect(wrapper.find('[data-testid="notifications-pagination"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="notifications-page-range"]').text()).toMatch(/第 1 页/)
    expect(wrapper.findAll('[data-testid="notifications-item"]')).toHaveLength(20)
    wrapper.unmount()
  })

  it('keeps page on non-filter data change when still valid (g1.4)', async () => {
    const items = makeItems(25)
    const { wrapper } = await mountView(items)
    await wrapper.find('[data-testid="notifications-pagination"]').findAll('button.pg-btn')[1]!.trigger('click')
    await nextTick()
    expect(wrapper.find('[data-testid="notifications-page-range"]').text()).toBe('第 2 页 · 21–25 / 25')
    const firstOnPage = wrapper.find('[data-testid="notifications-item"]').attributes('data-run-id')!
    useRunTerminalNotifications().markRead(firstOnPage)
    await nextTick()
    expect(wrapper.find('[data-testid="notifications-page-range"]').text()).toMatch(/第 2 页/)
    expect(wrapper.findAll('[data-testid="notifications-item"]').length).toBeGreaterThan(0)
    wrapper.unmount()
  })

  it('paging does not change unread badge or preview slice / fetchPool (g1.5 / g4.1)', async () => {
    const items = makeItems(25)
    const { wrapper } = await mountView(items)
    const callsAfterMount = vi.mocked(api.listRuns).mock.calls.length
    const n = useRunTerminalNotifications()
    expect(n.unreadCount.value).toBe(25)
    expect(n.previewItems.value).toHaveLength(RUN_TERMINAL_PANEL_LIMIT)
    const previewIds = n.previewItems.value.map((x) => x.runId)

    await wrapper.find('[data-testid="notifications-pagination"]').findAll('button.pg-btn')[1]!.trigger('click')
    await nextTick()
    expect(n.unreadCount.value).toBe(25)
    expect(wrapper.find('[data-testid="notifications-unread-count"]').text()).toContain('25')
    expect(n.previewItems.value.map((x) => x.runId)).toEqual(previewIds)
    expect(vi.mocked(api.listRuns).mock.calls.length).toBe(callsAfterMount)
    expect(api.listRuns).toHaveBeenCalledWith(
      expect.objectContaining({ page: 1, pageSize: RUN_TERMINAL_POOL_SIZE }),
    )
    wrapper.unmount()
  })

  it('does not write page into the URL (g3.3 / g4.1)', async () => {
    const { wrapper, router } = await mountView(makeItems(21))
    await wrapper.find('[data-testid="notifications-pagination"]').findAll('button.pg-btn')[1]!.trigger('click')
    await nextTick()
    expect(router.currentRoute.value.query.page).toBeUndefined()
    expect(router.currentRoute.value.fullPath).not.toContain('page=')
    wrapper.unmount()
  })

  it('empty list hides range and pager (g1.4 / g2.1)', async () => {
    const { wrapper } = await mountView([])
    expect(wrapper.find('[data-testid="notifications-page-range"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="notifications-pagination"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('topbar entry signal resets filter and page without query (g3.1)', async () => {
    const { wrapper, router } = await mountView(makeItems(21))
    await wrapper.find('[data-testid="notifications-pagination"]').findAll('button.pg-btn')[1]!.trigger('click')
    await wrapper.find('[data-testid="notifications-filter-unread"]').trigger('click')
    await nextTick()
    requestNotificationsPageReset()
    await nextTick()
    expect(wrapper.find('[data-testid="notifications-page-range"]').text()).toMatch(/第 1 页/)
    expect(wrapper.find('[data-testid="notifications-filter-all"]').classes().join(' ')).toMatch(/border-accent/)
    expect(router.currentRoute.value.fullPath).not.toContain('page=')
    wrapper.unmount()
  })

  it('mark-all-read still clears the whole pool unread count (g1.5)', async () => {
    const { wrapper } = await mountView(makeItems(25))
    await wrapper.find('[data-testid="notifications-pagination"]').findAll('button.pg-btn')[1]!.trigger('click')
    await wrapper.find('[data-testid="notifications-mark-all"]').trigger('click')
    await nextTick()
    expect(useRunTerminalNotifications().unreadCount.value).toBe(0)
    wrapper.unmount()
  })
})

describe('NotificationsView pagination conventions (g4.4 / g1.5)', () => {
  it('keeps min-h-11 on filter and mark-all', () => {
    expect(viewSrc).toMatch(/min-h-11 border border-line bg-transparent/)
    expect(viewSrc).toMatch(/min-h-11 border px-2.5/)
  })

  it('gates Pagination on filteredTotal > PAGE_SIZE with shrink-0 and no pageSizeOptions', () => {
    expect(viewSrc).toMatch(/const PAGE_SIZE = 20/)
    expect(viewSrc).toMatch(/<Pagination[\s\S]*v-if="filteredTotal > PAGE_SIZE"/)
    expect(viewSrc).toMatch(/class="shrink-0"/)
    expect(viewSrc).not.toMatch(/page-size-options/)
    expect(viewSrc).not.toMatch(/pageSizeOptions/)
  })

  it('does not change fetchPool page=1 / pageSize=50', () => {
    expect(poolSrc).toMatch(/page:\s*1/)
    expect(poolSrc).toMatch(/pageSize:\s*RUN_TERMINAL_POOL_SIZE/)
    expect(poolSrc).toMatch(/export const RUN_TERMINAL_POOL_SIZE = 50/)
  })
})
