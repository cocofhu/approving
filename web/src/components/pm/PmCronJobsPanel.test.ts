// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'
import { useAuth } from '@/lib/useAuth'
import PmCronJobsPanel from './PmCronJobsPanel.vue'

const apiMocks = vi.hoisted(() => ({
  listProjectCronJobs: vi.fn(),
  patchProjectCronJob: vi.fn(),
}))

vi.mock('@/lib/api', async () => {
  const actual = await vi.importActual<typeof import('@/lib/api')>('@/lib/api')
  return {
    ...actual,
    api: {
      ...actual.api,
      listProjectCronJobs: apiMocks.listProjectCronJobs,
      patchProjectCronJob: apiMocks.patchProjectCronJob,
    },
  }
})

vi.mock('@/lib/useToast', () => ({
  useToast: () => ({ success: vi.fn(), error: vi.fn() }),
}))

const SAMPLE_JOB = {
  id: 'cron-1',
  agentName: 'agent-a',
  projectId: 'proj-1',
  threadId: 'th-1',
  name: '每日汇报',
  prompt: '汇报',
  scheduleKind: 'cron',
  scheduleExpr: '0 9 * * *',
  enabled: true,
  deliverToChannel: false,
  createdAt: '2026-01-01T00:00:00Z',
  updatedAt: '2026-01-01T00:00:00Z',
}

function mountPanel() {
  const i18n = createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...common, ...pages } },
  })
  return mount(PmCronJobsPanel, {
    props: { projectId: 'proj-1' },
    global: {
      plugins: [i18n],
      stubs: { EmptyState: true, StatusPill: true },
    },
  })
}

describe('PmCronJobsPanel', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    const job = { ...SAMPLE_JOB, deliverToChannel: false }
    apiMocks.listProjectCronJobs.mockResolvedValue({ items: [job] })
    apiMocks.patchProjectCronJob.mockResolvedValue({ ...job, deliverToChannel: true })
    useAuth().clearUser()
  })

  it('loads project cron jobs on mount', async () => {
    useAuth().setUser({ username: 'admin', expiresAt: 't', isAdmin: true })
    const w = mountPanel()
    await flushPromises()
    expect(apiMocks.listProjectCronJobs).toHaveBeenCalledWith('proj-1')
    expect(w.text()).toContain('每日汇报')
    expect(w.text()).toContain('agent-a')
    expect(w.text()).toContain('cron: 0 9 * * *')
  })

  it('allows deliver toggle for non-admin without readonly banner', async () => {
    useAuth().setUser({ username: 'u', expiresAt: 't', isAdmin: false })
    const w = mountPanel()
    await flushPromises()
    const toggle = w.get('[data-testid="cron-deliver-toggle"]')
    expect((toggle.element as HTMLInputElement).disabled).toBe(false)
    expect(w.text()).toContain('任意已登录用户')
    expect(w.text()).not.toMatch(/修改需平台管理员|但不能修改渠道推送/)
    expect(w.find('.bg-amber-500\\/10').exists()).toBe(false)
    await toggle.setValue(true)
    await flushPromises()
    expect(apiMocks.patchProjectCronJob).toHaveBeenCalledWith('proj-1', 'cron-1', {
      deliverToChannel: true,
    })
  })

  it('patches deliverToChannel for admin', async () => {
    useAuth().setUser({ username: 'admin', expiresAt: 't', isAdmin: true })
    const w = mountPanel()
    await flushPromises()
    const toggle = w.get('[data-testid="cron-deliver-toggle"]')
    expect((toggle.element as HTMLInputElement).disabled).toBe(false)
    await toggle.setValue(true)
    await flushPromises()
    expect(apiMocks.patchProjectCronJob).toHaveBeenCalledWith('proj-1', 'cron-1', {
      deliverToChannel: true,
    })
  })
})
