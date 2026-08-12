// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { mount, flushPromises } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick, ref } from 'vue'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'
import shell from '@/locales/zh-CN/shell.json'
import type { Run } from '@/lib/shared/types'

const push = vi.fn()

vi.mock('vue-router', () => ({
  useRoute: () => ({ path: '/', meta: {}, query: {} }),
  useRouter: () => ({ push }),
}))

vi.mock('@/lib/composables/useShutdownState', () => ({
  isDraining: () => false,
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
    platformStatus: vi.fn().mockResolvedValue({
      cumulativeTokens: null,
      current5mBucketTokens: null,
      todayMaxCompleted5mTokens: null,
      runningCount: 0,
      queuedCount: 0,
      asOf: '2026-08-12T00:00:00Z',
      timezone: 'UTC',
    }),
  },
  isPaginated: (data: unknown): data is { items: unknown[]; total: number } =>
    data != null && typeof data === 'object' && !Array.isArray(data) && 'items' in data,
}))

import { api } from '@/lib/api/api'
import { __resetNotificationsPageEntryForTests } from '@/lib/composables/useNotificationsPageEntry'
import {
  __resetRunTerminalNotificationsForTests,
  prefsKeyForUser,
  RUN_TERMINAL_PANEL_LIMIT,
  RUN_TERMINAL_POOL_SIZE,
} from '@/lib/run/useRunTerminalNotifications'
import AppTopbar from './AppTopbar.vue'

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

function mountTopbar() {
  const i18n = createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...common, ...pages, ...shell } },
  })
  return mount(AppTopbar, {
    global: {
      plugins: [i18n],
      stubs: {
        Icon: true,
        LangSelect: { template: '<div data-testid="lang" />' },
        StatusMetrics: { template: '<div data-testid="status-metrics" />' },
        Transition: false,
      },
    },
    attachTo: document.body,
  })
}

describe('AppTopbar notifications', () => {
  beforeEach(() => {
    localStorage.clear()
    __resetRunTerminalNotificationsForTests()
    __resetNotificationsPageEntryForTests()
    push.mockReset()
    vi.mocked(api.listRuns).mockReset()
    vi.mocked(api.getRun).mockReset()
    vi.mocked(api.artifactContent).mockReset()
    vi.mocked(api.listRuns).mockResolvedValue(paged([]))
    vi.mocked(api.getRun).mockResolvedValue(run({ id: 'r1', status: 'completed', artifacts: [] }))
    vi.mocked(api.artifactContent).mockImplementation(async (id: string) => ({
      id,
      name: 'summary.md',
      kind: 'markdown',
      nodeId: 'n1',
      runId: 'ok-2',
      workflowName: 'demo-wf',
      sizeBytes: 10,
      createdAt: '2026-08-10T12:00:00Z',
      content: '# hello',
    }))
  })

  afterEach(() => {
    __resetRunTerminalNotificationsForTests()
    __resetNotificationsPageEntryForTests()
  })

  it('renders header with theme toggle and bell aria titled 通知', async () => {
    const wrapper = mountTopbar()
    await flushPromises()
    expect(wrapper.find('header').exists()).toBe(true)
    expect(wrapper.find('[data-testid="lang"]').exists()).toBe(true)
    const bell = wrapper.find('[data-testid="run-notifications-bell"]')
    expect(bell.exists()).toBe(true)
    expect(bell.attributes('aria-label')).toBe('通知')
    expect(bell.attributes('aria-haspopup')).toBe('true')
    expect(bell.attributes('aria-expanded')).toBe('false')
    expect(wrapper.find('[data-testid="run-notifications-badge"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('emits toggle-menu from mobile menu button', async () => {
    const wrapper = mountTopbar()
    await flushPromises()
    await wrapper.find('button').trigger('click')
    expect(wrapper.emitted('toggle-menu')).toHaveLength(1)
    wrapper.unmount()
  })

  it('shows panel empty state without a clickable runs escape', async () => {
    const wrapper = mountTopbar()
    await flushPromises()
    await wrapper.find('[data-testid="run-notifications-bell"]').trigger('click')
    await nextTick()
    expect(wrapper.find('[data-testid="run-notifications-panel"]').exists()).toBe(true)
    const empty = wrapper.find('[data-testid="run-notifications-empty"]')
    expect(empty.text()).toContain('暂无通知')
    expect(empty.text()).toContain('执行完成或失败后才会出现')
    expect(empty.text()).toContain('运行')
    expect(empty.find('a').exists()).toBe(false)
    expect(empty.find('button').exists()).toBe(false)
    wrapper.unmount()
  })

  it('caps dropdown at 5 items; view-all goes to /notifications; mark-all clears badge', async () => {
    seedBaseline()
    const items = Array.from({ length: 12 }, (_, i) =>
      run({
        id: `r${i}`,
        status: i === 0 ? 'failed' : 'completed',
        startedAt: `2026-08-10T${String(12 + (i % 10)).padStart(2, '0')}:${String(i).padStart(2, '0')}:00Z`,
      }),
    )
    vi.mocked(api.listRuns).mockResolvedValue(paged(items, 12))
    const wrapper = mountTopbar()
    await flushPromises()

    const badge = wrapper.find('[data-testid="run-notifications-badge"]')
    expect(badge.exists()).toBe(true)
    expect(badge.text()).toBe('12')
    expect(badge.classes().join(' ')).toMatch(/bg-err/)

    await wrapper.find('[data-testid="run-notifications-bell"]').trigger('click')
    await nextTick()
    expect(wrapper.findAll('[data-testid="run-notifications-item"]')).toHaveLength(
      RUN_TERMINAL_PANEL_LIMIT,
    )
    expect(RUN_TERMINAL_PANEL_LIMIT).toBe(5)
    expect(wrapper.find('[data-testid="run-notifications-more"]').text()).toContain('还有 7 条')
    expect(wrapper.find('[data-testid="run-notifications-view-all"]').text()).toBe('查看全部通知')

    await wrapper.find('[data-testid="run-notifications-mark-all"]').trigger('click')
    await nextTick()
    expect(wrapper.find('[data-testid="run-notifications-badge"]').exists()).toBe(false)

    await wrapper.find('[data-testid="run-notifications-view-all"]').trigger('click')
    expect(push).toHaveBeenCalledWith({ path: '/notifications' })
    expect(push).not.toHaveBeenCalledWith(
      expect.objectContaining({ path: '/runs' }),
    )
    wrapper.unmount()
  })

  it('shows before-baseline label on history items without counting them unread', async () => {
    // First-enable baseline ≈ now; past fixtures → beforeBaseline, unread=false.
    vi.mocked(api.listRuns).mockResolvedValue(
      paged([run({ id: 'hist', status: 'completed', startedAt: '2026-08-01T12:00:00Z' })]),
    )
    const wrapper = mountTopbar()
    await flushPromises()
    expect(wrapper.find('[data-testid="run-notifications-badge"]').exists()).toBe(false)
    await wrapper.find('[data-testid="run-notifications-bell"]').trigger('click')
    await nextTick()
    const item = wrapper.find('[data-testid="run-notifications-item"]')
    expect(item.attributes('data-before-baseline')).toBe('true')
    expect(item.attributes('data-unread')).toBe('false')
    expect(item.text()).toContain('基线前·不计未读')
    wrapper.unmount()
  })

  it('clicking a preview item enters /notifications page 1 without locating (g3.2)', async () => {
    seedBaseline()
    vi.mocked(api.listRuns).mockResolvedValue(
      paged([run({ id: 'fail-1', status: 'failed', title: 'boom' })]),
    )
    const wrapper = mountTopbar()
    await flushPromises()
    expect(wrapper.find('[data-testid="run-notifications-badge"]').text()).toBe('1')
    await wrapper.find('[data-testid="run-notifications-bell"]').trigger('click')
    await nextTick()
    await wrapper.find('[data-testid="run-notifications-item"]').trigger('click')
    await flushPromises()
    expect(push).toHaveBeenCalledWith({ path: '/notifications' })
    expect(push).not.toHaveBeenCalledWith('/runs/fail-1')
    expect(wrapper.find('[data-testid="run-notifications-badge"]').text()).toBe('1')
    expect(wrapper.find('[data-testid="run-output-empty"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('clicking completed preview also goes to /notifications and does not open output modal (g3.2)', async () => {
    seedBaseline()
    vi.mocked(api.listRuns).mockResolvedValue(
      paged([run({ id: 'ok-1', status: 'completed', title: 'done' })]),
    )
    vi.mocked(api.getRun).mockResolvedValue(
      run({
        id: 'ok-1',
        status: 'completed',
        artifacts: [
          {
            id: 'a-nc',
            name: 'node_complete.json',
            kind: 'json',
            nodeId: 'agent',
            runId: 'ok-1',
            workflowName: 'demo-wf',
            sizeBytes: 32,
            createdAt: '2026-08-10T12:00:00Z',
          },
        ],
        nodes: [{ id: 'out-1', type: 'output', label: '输出', position: { x: 0, y: 0 }, config: {} }],
        nodeRuns: {
          'out-1': { nodeId: 'out-1', status: 'completed', outputs: { outputCards: [] } },
        },
      }),
    )
    const wrapper = mountTopbar()
    await flushPromises()
    await wrapper.find('[data-testid="run-notifications-bell"]').trigger('click')
    await nextTick()
    expect(wrapper.find('[data-testid="run-notifications-badge"]').text()).toBe('1')
    await wrapper.find('[data-testid="run-notifications-item"]').trigger('click')
    await flushPromises()
    await nextTick()
    expect(push).toHaveBeenCalledWith({ path: '/notifications' })
    expect(api.getRun).not.toHaveBeenCalled()
    expect(wrapper.find('[data-testid="run-output-empty"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="run-notifications-badge"]').text()).toBe('1')
    wrapper.unmount()
  })

  it('cleans noisy progress titles in the panel', async () => {
    seedBaseline()
    vi.mocked(api.listRuns).mockResolvedValue(
      paged([
        run({
          id: 'noisy',
          status: 'completed',
          title: '运行中 3 / 等待 1',
          workflowName: '自我迭代',
        }),
      ]),
    )
    const wrapper = mountTopbar()
    await flushPromises()
    await wrapper.find('[data-testid="run-notifications-bell"]').trigger('click')
    await nextTick()
    const item = wrapper.find('[data-testid="run-notifications-item"]')
    expect(item.text()).toContain('自我迭代 · 已完成')
    expect(item.text()).not.toMatch(/运行中/)
    wrapper.unmount()
  })
})
