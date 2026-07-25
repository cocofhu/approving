// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'
import { useAuth } from '@/lib/useAuth'
import PmCronJobsPanel from './PmCronJobsPanel.vue'

const toastSuccess = vi.fn()
const toastError = vi.fn()

const apiMocks = vi.hoisted(() => ({
  listProjectCronJobs: vi.fn(),
  patchProjectCronJob: vi.fn(),
  deleteProjectCronJob: vi.fn(),
}))

vi.mock('@/lib/api', async () => {
  const actual = await vi.importActual<typeof import('@/lib/api')>('@/lib/api')
  return {
    ...actual,
    api: {
      ...actual.api,
      listProjectCronJobs: apiMocks.listProjectCronJobs,
      patchProjectCronJob: apiMocks.patchProjectCronJob,
      deleteProjectCronJob: apiMocks.deleteProjectCronJob,
    },
  }
})

vi.mock('@/lib/useToast', () => ({
  useToast: () => ({ success: toastSuccess, error: toastError }),
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
    apiMocks.deleteProjectCronJob.mockResolvedValue({ status: 'deleted' })
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
    expect(w.text()).toMatch(/可查看与删除/)
  })

  it('allows deliver toggle for non-admin without readonly banner', async () => {
    useAuth().setUser({ username: 'u', expiresAt: 't', isAdmin: false })
    const w = mountPanel()
    await flushPromises()
    const toggle = w.get('[data-testid="cron-deliver-toggle"]')
    expect((toggle.element as HTMLButtonElement).disabled).toBe(false)
    expect(toggle.attributes('role')).toBe('switch')
    expect(w.text()).toContain('任意已登录用户')
    expect(w.text()).not.toMatch(/修改需平台管理员|但不能修改渠道推送/)
    expect(w.find('.bg-amber-500\\/10').exists()).toBe(false)
    await toggle.trigger('click')
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
    expect((toggle.element as HTMLButtonElement).disabled).toBe(false)
    await toggle.trigger('click')
    await flushPromises()
    expect(apiMocks.patchProjectCronJob).toHaveBeenCalledWith('proj-1', 'cron-1', {
      deliverToChannel: true,
    })
  })

  it('cancel confirm does not call delete API', async () => {
    useAuth().setUser({ username: 'u', expiresAt: 't', isAdmin: false })
    const w = mountPanel()
    await flushPromises()
    vi.spyOn(window, 'confirm').mockReturnValue(false)
    await w.get('[data-testid="project-cron-delete"]').trigger('click')
    await flushPromises()
    expect(apiMocks.deleteProjectCronJob).not.toHaveBeenCalled()
    expect(w.text()).toContain('每日汇报')
  })

  it('confirm delete calls API, refreshes list, and toasts success', async () => {
    useAuth().setUser({ username: 'u', expiresAt: 't', isAdmin: false })
    apiMocks.listProjectCronJobs
      .mockResolvedValueOnce({ items: [{ ...SAMPLE_JOB }] })
      .mockResolvedValueOnce({ items: [] })
    const w = mountPanel()
    await flushPromises()
    expect(w.text()).toContain('每日汇报')

    vi.spyOn(window, 'confirm').mockReturnValue(true)
    await w.get('[data-testid="project-cron-delete"]').trigger('click')
    await flushPromises()

    expect(apiMocks.deleteProjectCronJob).toHaveBeenCalledWith('proj-1', 'cron-1')
    expect(apiMocks.listProjectCronJobs).toHaveBeenCalledTimes(2)
    expect(toastSuccess).toHaveBeenCalled()
    expect(toastError).not.toHaveBeenCalled()
    expect(w.text()).not.toContain('每日汇报')
  })
})
