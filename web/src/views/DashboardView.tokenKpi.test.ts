// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'

const mocks = vi.hoisted(() => ({
  push: vi.fn(),
  dashboard: vi.fn(),
}))

vi.mock('vue-router', () => ({
  useRouter: () => ({ push: mocks.push }),
}))

vi.mock('@/lib/api', async () => {
  const actual = await vi.importActual<typeof import('@/lib/api')>('@/lib/api')
  return {
    ...actual,
    api: {
      ...actual.api,
      dashboard: mocks.dashboard,
    },
  }
})

vi.mock('@/lib/useRunBoard', async () => {
  const { ref: vueRef } = await import('vue')
  return {
    useRunBoard: () => ({
      load: vi.fn(async () => undefined),
      column: () => ({ items: [], total: 0 }),
      loading: vueRef(false),
      hasLoaded: vueRef(true),
      error: vueRef(null),
    }),
  }
})

vi.mock('@/lib/useProjectContext', () => ({
  readStoredProjectId: () => '',
}))

import DashboardView from './DashboardView.vue'

function mountDashboard() {
  const i18n = createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...common, ...pages } },
  })
  return mount(DashboardView, {
    global: {
      plugins: [i18n],
      stubs: {
        Icon: true,
        RunBoardColumn: true,
        RunBoardPreviewDrawer: true,
        TokenUsageHoverTip: true,
      },
    },
  })
}

const baseStats = {
  running: 1,
  waitingHuman: 2,
  failed: 0,
  completed: 10,
  workflows: 3,
  artifacts: 4,
}

describe('DashboardView total Token KPI (g2 / g3.1)', () => {
  beforeEach(() => {
    mocks.push.mockReset()
    mocks.dashboard.mockReset()
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('shows 总 Token label and platform scope; card is not a navigable button', async () => {
    mocks.dashboard.mockResolvedValue({
      ...baseStats,
      totalTokens: 1_240_000,
      workflowTokens: 986_200,
      pmTokens: 253_800,
    })
    const wrapper = mountDashboard()
    await flushPromises()

    const card = wrapper.get('[data-testid="dashboard-kpi-total-tokens"]')
    expect(card.element.tagName).not.toBe('BUTTON')
    expect(card.text()).toContain('总 Token')
    expect(card.text()).toContain('全平台 · 历史累计')
    expect(wrapper.get('[data-testid="dashboard-kpi-total-tokens-value"]').text()).toBe('1.24M')

    mocks.push.mockClear()
    await card.trigger('click')
    expect(mocks.push).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('renders — for never-reported (null) and 0 for reported zero', async () => {
    mocks.dashboard.mockResolvedValue({
      ...baseStats,
      totalTokens: null,
      workflowTokens: null,
      pmTokens: null,
    })
    const wrapper = mountDashboard()
    await flushPromises()
    expect(wrapper.get('[data-testid="dashboard-kpi-total-tokens-value"]').text()).toBe('—')
    expect(wrapper.get('[data-testid="dashboard-kpi-total-tokens-foot"]').text()).toContain(
      '尚未上报',
    )
    wrapper.unmount()

    mocks.dashboard.mockResolvedValue({
      ...baseStats,
      totalTokens: 0,
      workflowTokens: 0,
      pmTokens: 0,
    })
    const zero = mountDashboard()
    await flushPromises()
    expect(zero.get('[data-testid="dashboard-kpi-total-tokens-value"]').text()).toBe('0')
    expect(zero.get('[data-testid="dashboard-kpi-total-tokens-foot"]').text()).toContain(
      '已上报且合计为 0',
    )
    zero.unmount()
  })

  it('keeps last success value while refreshing and shows 更新中', async () => {
    vi.useFakeTimers()
    let resolveSecond!: (v: unknown) => void
    mocks.dashboard
      .mockResolvedValueOnce({
        ...baseStats,
        totalTokens: 128400,
        workflowTokens: 128400,
        pmTokens: null,
      })
      .mockImplementationOnce(
        () =>
          new Promise((resolve) => {
            resolveSecond = resolve
          }),
      )

    const wrapper = mountDashboard()
    await flushPromises()
    expect(wrapper.get('[data-testid="dashboard-kpi-total-tokens-value"]').text()).toBe('128.4K')

    // Trigger interval refresh while second call is pending.
    await vi.advanceTimersByTimeAsync(8000)
    await flushPromises()
    expect(wrapper.get('[data-testid="dashboard-kpi-total-tokens-value"]').text()).toBe('128.4K')
    expect(wrapper.get('[data-testid="dashboard-kpi-total-tokens-foot"]').text()).toContain(
      '更新中',
    )

    resolveSecond({
      ...baseStats,
      totalTokens: 200000,
      workflowTokens: 200000,
      pmTokens: null,
    })
    await flushPromises()
    expect(wrapper.get('[data-testid="dashboard-kpi-total-tokens-value"]').text()).toBe('200K')
    expect(wrapper.get('[data-testid="dashboard-kpi-total-tokens-foot"]').text()).toContain(
      '全平台 · 历史累计',
    )
    wrapper.unmount()
  })

  it('failed refresh does not flash last success to 0', async () => {
    vi.useFakeTimers()
    mocks.dashboard
      .mockResolvedValueOnce({
        ...baseStats,
        totalTokens: 42,
        workflowTokens: 42,
        pmTokens: null,
      })
      .mockRejectedValueOnce(new Error('network'))

    const wrapper = mountDashboard()
    await flushPromises()
    expect(wrapper.get('[data-testid="dashboard-kpi-total-tokens-value"]').text()).toBe('42')

    await vi.advanceTimersByTimeAsync(8000)
    await flushPromises()
    expect(wrapper.get('[data-testid="dashboard-kpi-total-tokens-value"]').text()).toBe('42')
    expect(wrapper.get('[data-testid="dashboard-kpi-total-tokens-value"]').text()).not.toBe('0')
    wrapper.unmount()
  })

  it('Token card stays after status KPIs in the same grid', async () => {
    mocks.dashboard.mockResolvedValue({
      ...baseStats,
      totalTokens: 10,
      workflowTokens: 10,
      pmTokens: null,
    })
    const wrapper = mountDashboard()
    await flushPromises()
    const grid = wrapper.get('[data-testid="dashboard-kpi-grid"]')
    const kids = Array.from(grid.element.children) as HTMLElement[]
    expect(kids).toHaveLength(5)
    expect(kids[0].getAttribute('data-testid')).toBe('dashboard-kpi-running')
    expect(kids[4].getAttribute('data-testid')).toBe('dashboard-kpi-total-tokens')
    wrapper.unmount()
  })
})
