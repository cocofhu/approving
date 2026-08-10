// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { mount, flushPromises } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick, ref } from 'vue'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'
import shell from '@/locales/zh-CN/shell.json'
import type { Run } from '@/lib/types'

const push = vi.fn()

vi.mock('vue-router', () => ({
  useRoute: () => ({ path: '/', meta: {}, query: {} }),
  useRouter: () => ({ push }),
}))

vi.mock('@/lib/useShutdownState', () => ({
  isDraining: () => false,
}))

vi.mock('@/lib/useAuth', () => ({
  useAuth: () => ({
    user: ref({ username: 'tester', expiresAt: 't' }),
    ready: ref(true),
  }),
}))

vi.mock('@/lib/api', () => ({
  api: {
    listRuns: vi.fn(),
    getRun: vi.fn(),
  },
  isPaginated: (data: unknown): data is { items: unknown[]; total: number } =>
    data != null && typeof data === 'object' && !Array.isArray(data) && 'items' in data,
}))

import { api } from '@/lib/api'
import {
  __resetRunTerminalNotificationsForTests,
  prefsKeyForUser,
  RUN_TERMINAL_PANEL_LIMIT,
  RUN_TERMINAL_POOL_SIZE,
} from '@/lib/useRunTerminalNotifications'
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
    push.mockReset()
    vi.mocked(api.listRuns).mockReset()
    vi.mocked(api.getRun).mockReset()
    vi.mocked(api.listRuns).mockResolvedValue(paged([]))
    vi.mocked(api.getRun).mockResolvedValue(run({ id: 'r1', status: 'completed', artifacts: [] }))
  })

  afterEach(() => {
    __resetRunTerminalNotificationsForTests()
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

  it('caps dropdown at 10 items; view-all goes to /notifications; mark-all clears badge', async () => {
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
    expect(wrapper.find('[data-testid="run-notifications-more"]').text()).toContain('还有 2 条')

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

  it('clicking failed item marks read and navigates to run detail without output modal', async () => {
    seedBaseline()
    vi.mocked(api.listRuns).mockResolvedValue(
      paged([run({ id: 'fail-1', status: 'failed', title: 'boom' })]),
    )
    const wrapper = mountTopbar()
    await flushPromises()
    await wrapper.find('[data-testid="run-notifications-bell"]').trigger('click')
    await nextTick()
    await wrapper.find('[data-testid="run-notifications-item"]').trigger('click')
    await flushPromises()
    expect(push).toHaveBeenCalledWith('/runs/fail-1')
    expect(wrapper.find('[data-testid="run-notifications-badge"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="run-output-empty"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('clicking completed item marks read and opens output modal (empty artifacts)', async () => {
    seedBaseline()
    vi.mocked(api.listRuns).mockResolvedValue(
      paged([run({ id: 'ok-1', status: 'completed', title: 'done' })]),
    )
    vi.mocked(api.getRun).mockResolvedValue(
      run({ id: 'ok-1', status: 'completed', artifacts: [] }),
    )
    const wrapper = mountTopbar()
    await flushPromises()
    await wrapper.find('[data-testid="run-notifications-bell"]').trigger('click')
    await nextTick()
    await wrapper.find('[data-testid="run-notifications-item"]').trigger('click')
    await flushPromises()
    await nextTick()
    expect(push).not.toHaveBeenCalled()
    expect(api.getRun).toHaveBeenCalledWith('ok-1')
    expect(wrapper.vm.outputOpen).toBe(true)
    expect(wrapper.vm.outputRunId).toBe('ok-1')
    await vi.waitFor(() => {
      expect(document.body.querySelector('[data-testid="run-output-empty"]')).toBeTruthy()
    })
    expect(document.body.querySelector('[data-testid="run-output-empty"]')?.textContent).toContain(
      '本次运行暂无产出',
    )
    expect(wrapper.find('[data-testid="run-notifications-badge"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('completed with artifacts shows ppt cards and preview; no download control', async () => {
    seedBaseline()
    vi.mocked(api.listRuns).mockResolvedValue(
      paged([run({ id: 'ok-2', status: 'completed', title: 'done' })]),
    )
    vi.mocked(api.getRun).mockResolvedValue(
      run({
        id: 'ok-2',
        status: 'completed',
        artifacts: [
          {
            id: 'a1',
            name: 'summary.md',
            kind: 'markdown',
            nodeId: 'n1',
            runId: 'ok-2',
            workflowName: 'demo-wf',
            sizeBytes: 10,
            createdAt: '2026-08-10T12:00:00Z',
          },
          {
            id: 'a2',
            name: 'result.json',
            kind: 'json',
            nodeId: 'n1',
            runId: 'ok-2',
            workflowName: 'demo-wf',
            sizeBytes: 20,
            createdAt: '2026-08-10T12:00:00Z',
          },
        ],
      }),
    )
    const wrapper = mountTopbar()
    await flushPromises()
    await wrapper.find('[data-testid="run-notifications-bell"]').trigger('click')
    await nextTick()
    await wrapper.find('[data-testid="run-notifications-item"]').trigger('click')
    await flushPromises()
    await nextTick()
    expect(wrapper.vm.outputOpen).toBe(true)
    await vi.waitFor(() => {
      expect(document.body.querySelector('[data-testid="run-output-deck"]')).toBeTruthy()
    })
    expect(document.body.querySelectorAll('[data-testid="run-output-card"]')).toHaveLength(2)
    expect(document.body.querySelector('[data-testid="run-output-preview"]')).toBeTruthy()
    expect(document.body.querySelector('[data-testid="run-output-open-run"]')).toBeTruthy()
    expect(document.body.querySelector('[data-testid="run-output-done"]')).toBeTruthy()
    expect(document.body.querySelector('[data-testid="artifact-download"]')).toBeNull()
    expect(document.body.querySelector('a[download]')).toBeNull()
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
