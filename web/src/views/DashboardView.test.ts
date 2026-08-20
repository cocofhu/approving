// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'
import type { Workflow } from '@/lib/shared/types'

const mocks = vi.hoisted(() => ({
  push: vi.fn(),
  listWorkflows: vi.fn(),
  startRun: vi.fn(),
  getRun: vi.fn(),
  reactReply: vi.fn(),
  readStoredProjectId: vi.fn(() => 'proj-1'),
}))

vi.mock('vue-router', () => ({
  useRouter: () => ({ push: mocks.push }),
}))

vi.mock('@/lib/api/api', async () => {
  const actual = await vi.importActual<typeof import('@/lib/api/api')>('@/lib/api/api')
  return {
    ...actual,
    api: {
      ...actual.api,
      listWorkflows: mocks.listWorkflows,
      startRun: mocks.startRun,
      getRun: mocks.getRun,
      reactReply: mocks.reactReply,
    },
  }
})

vi.mock('@/lib/composables/useProjectContext', () => ({
  readStoredProjectId: () => mocks.readStoredProjectId(),
}))

vi.mock('@/lib/composables/useToast', () => ({
  useToast: () => ({ warn: vi.fn(), error: vi.fn(), success: vi.fn() }),
}))

import DashboardView from './DashboardView.vue'

const approveWf: Workflow = {
  id: 'wf-ap',
  name: '自我迭代PRO',
  description: '开发前澄清 + 计划',
  status: 'published',
  version: 1,
  updatedAt: '',
  needsRepo: false,
  nodes: [
    { id: 'in', type: 'input', label: '开始', position: { x: 0, y: 0 }, config: {} },
    { id: 'ap', type: 'approve', label: '澄清', position: { x: 0, y: 0 }, config: {} },
  ],
  edges: [{ id: 'e1', source: 'in', target: 'ap' }],
}

function mountDashboard() {
  const i18n = createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...common, ...pages } },
  })
  return mount(DashboardView, {
    global: {
      plugins: [i18n],
      stubs: { Icon: true, RunLaunchModal: true },
    },
  })
}

describe('DashboardView home composer', () => {
  beforeEach(() => {
    mocks.push.mockReset()
    mocks.listWorkflows.mockReset()
    mocks.startRun.mockReset()
    mocks.getRun.mockReset()
    mocks.reactReply.mockReset()
    mocks.readStoredProjectId.mockReturnValue('proj-1')
    mocks.listWorkflows.mockResolvedValue([approveWf])
    mocks.startRun.mockResolvedValue({ id: 'run-9', status: 'queued' })
    mocks.getRun.mockResolvedValue({
      id: 'run-9',
      status: 'waiting_human',
      nodes: [{ id: 'ap', type: 'approve', label: '', position: { x: 0, y: 0 }, config: {} }],
      nodeRuns: { ap: { nodeId: 'ap', status: 'waiting_human' } },
    })
    mocks.reactReply.mockResolvedValue({ status: 'ok' })
  })

  it('renders composer and approve-first cards when a project is selected', async () => {
    const wrapper = mountDashboard()
    await flushPromises()
    expect(wrapper.get('[data-testid="home-title"]').text()).toContain('从一句话开始一次开发前澄清')
    expect(wrapper.find('[data-testid="home-composer"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="home-pipeline-card-wf-ap"]').text()).toContain('自我迭代PRO')
    wrapper.unmount()
  })

  it('shows project empty state when no project is stored', async () => {
    mocks.readStoredProjectId.mockReturnValue('')
    const wrapper = mountDashboard()
    await flushPromises()
    expect(wrapper.find('[data-testid="home-no-project"]').exists()).toBe(true)
    await wrapper.get('[data-testid="dashboard-select-project"]').trigger('click')
    expect(mocks.push).toHaveBeenCalledWith('/projects')
    wrapper.unmount()
  })

  it('shows pipeline empty state when none are approve-first', async () => {
    mocks.listWorkflows.mockResolvedValue([
      {
        ...approveWf,
        id: 'wf-react',
        name: '实现',
        nodes: [
          { id: 'in', type: 'input', label: '开始', position: { x: 0, y: 0 }, config: {} },
          { id: 'r', type: 'react', label: '实现', position: { x: 0, y: 0 }, config: {} },
        ],
        edges: [{ id: 'e1', source: 'in', target: 'r' }],
      },
    ])
    const wrapper = mountDashboard()
    await flushPromises()
    expect(wrapper.find('[data-testid="home-pipelines-empty"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('sending the first message starts the run and opens clarify', async () => {
    const wrapper = mountDashboard()
    await flushPromises()
    await wrapper.get('[data-testid="home-composer-input"]').setValue('把登录做清楚')
    await wrapper.get('[data-testid="home-composer"]').trigger('submit')
    await flushPromises()
    expect(mocks.startRun).toHaveBeenCalledWith('wf-ap', {}, 'manual', 'normal', [], {
      title: '把登录做清楚',
    })
    expect(mocks.reactReply).toHaveBeenCalledWith('run-9', 'ap', '把登录做清楚')
    expect(mocks.push).toHaveBeenCalledWith({ path: '/runs/run-9', query: { node: 'ap' } })
    wrapper.unmount()
  })
})
