// @vitest-environment happy-dom
import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { createI18n } from 'vue-i18n'
import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'
import { serializeStatusQuery } from '@/lib/useStatusFilter'

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

const KPI_CASES = [
  { status: 'running', testid: 'dashboard-kpi-running', count: 4 },
  { status: 'waiting_human', testid: 'dashboard-kpi-waiting_human', count: 0 },
  { status: 'failed', testid: 'dashboard-kpi-failed', count: 1 },
  { status: 'completed', testid: 'dashboard-kpi-completed', count: 157 },
] as const

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
      },
    },
  })
}

describe('DashboardView KPI → /runs status navigation (g1 / g2.1)', () => {
  beforeEach(() => {
    mocks.push.mockReset()
    mocks.dashboard.mockReset()
    mocks.dashboard.mockResolvedValue({
      running: 4,
      waitingHuman: 0,
      failed: 1,
      completed: 157,
      totalTokens: null,
      workflowTokens: null,
      pmTokens: null,
    })
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('maps four KPI cards to single status query without projectId', async () => {
    const wrapper = mountDashboard()
    await flushPromises()

    for (const kpi of KPI_CASES) {
      const btn = wrapper.get(`[data-testid="${kpi.testid}"]`)
      expect(btn.element.tagName).toBe('BUTTON')
      expect(btn.attributes('type')).toBe('button')
      expect(btn.attributes('disabled')).toBeUndefined()
      expect(btn.text()).toContain(String(kpi.count))
      expect(btn.attributes('aria-label')).toContain(String(kpi.count))

      mocks.push.mockClear()
      await btn.trigger('click')
      expect(mocks.push).toHaveBeenCalledTimes(1)
      expect(mocks.push).toHaveBeenCalledWith({
        path: '/runs',
        query: { status: serializeStatusQuery([kpi.status]) },
      })
      const arg = mocks.push.mock.calls[0][0] as { query: Record<string, string> }
      expect(arg.query).not.toHaveProperty('projectId')
      expect(Object.keys(arg.query)).toEqual(['status'])
    }

    wrapper.unmount()
  })

  it('keeps zero-value waiting_human card clickable', async () => {
    const wrapper = mountDashboard()
    await flushPromises()

    const zero = wrapper.get('[data-testid="dashboard-kpi-waiting_human"]')
    expect(zero.attributes('disabled')).toBeUndefined()
    expect(zero.text()).toContain('0')
    await zero.trigger('click')
    expect(mocks.push).toHaveBeenCalledWith({
      path: '/runs',
      query: { status: 'waiting_human' },
    })
    wrapper.unmount()
  })

  it('source keeps four status mappings and serializeStatusQuery push (plan g1.1/g1.3)', () => {
    const dir = dirname(fileURLToPath(import.meta.url))
    const src = readFileSync(join(dir, 'DashboardView.vue'), 'utf8')
    expect(src).toContain("status: 'running'")
    expect(src).toContain("status: 'waiting_human'")
    expect(src).toContain("status: 'failed'")
    expect(src).toContain("status: 'completed'")
    expect(src).toContain('serializeStatusQuery([status])')
    expect(src).toContain("path: '/runs'")
    expect(src).not.toMatch(/goKpiRuns[\s\S]*projectId/)
  })
})
