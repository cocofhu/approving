// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createRouter, createMemoryHistory } from 'vue-router'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'
import ProjectAgentsPanel from './ProjectAgentsPanel.vue'

const apiMocks = vi.hoisted(() => ({
  listAgents: vi.fn(),
  createProjectSharedAgentTest: vi.fn(),
}))

vi.mock('@/lib/api/api', async () => {
  const actual = await vi.importActual<typeof import('@/lib/api/api')>('@/lib/api/api')
  return {
    ...actual,
    api: {
      ...actual.api,
      listAgents: apiMocks.listAgents,
      createProjectSharedAgentTest: apiMocks.createProjectSharedAgentTest,
    },
  }
})

async function mountPanel(projectId = 'proj-a', query: Record<string, string> = {}) {
  const i18n = createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...common, ...pages } },
  })
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/', component: { template: '<div/>' } }],
  })
  await router.push({ path: '/', query })
  await router.isReady()
  return mount(ProjectAgentsPanel, {
    props: { projectId },
    global: {
      plugins: [i18n, router],
      stubs: {
        Icon: true,
        AgentStudioView: {
          template: '<div data-testid="agents-studio-stub">studio</div>',
        },
        AgentChatTester: {
          props: ['profile', 'homeProjectId', 'createTest'],
          template: '<div data-testid="agents-chat-tester">{{ profile }}</div>',
        },
      },
    },
  })
}

describe('ProjectAgentsPanel second-level tabs (g1.1 / g1.2)', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    apiMocks.listAgents.mockResolvedValue([
      { name: 'mine', projectId: 'proj-a' },
      { name: 'other', projectId: 'proj-b' },
    ])
    apiMocks.createProjectSharedAgentTest.mockResolvedValue({ id: 1 })
  })

  it('shows meta | chat-test second-level tabs aligned with confirmed preview', async () => {
    const wrapper = await mountPanel()
    await flushPromises()
    expect(wrapper.get('[data-testid="project-agents-subtab-meta"]').text()).toBe('元信息')
    expect(wrapper.get('[data-testid="project-agents-subtab-test"]').text()).toBe('对话测试')
    expect(wrapper.find('[data-testid="agents-studio-stub"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="agents-chat-tester"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('mounts AgentChatTester with current Agent via createProjectSharedAgentTest', async () => {
    const wrapper = await mountPanel('proj-a', { agent: 'mine' })
    await flushPromises()
    await wrapper.get('[data-testid="project-agents-subtab-test"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="project-agents-chat-test"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="agents-chat-tester"]').text()).toBe('mine')
    const vm = wrapper.vm as unknown as {
      createProjectContextTest: (p: string, payload: { repos?: unknown[] }) => Promise<unknown>
    }
    await vm.createProjectContextTest('mine', { repos: [{ name: 'r', url: 'https://x' }] })
    expect(apiMocks.createProjectSharedAgentTest).toHaveBeenCalledWith('proj-a', {
      agentName: 'mine',
      repos: [{ name: 'r', url: 'https://x' }],
    })
    wrapper.unmount()
  })

  it('remounts tester when route agent changes (:key)', async () => {
    apiMocks.listAgents.mockResolvedValue([
      { name: 'alpha', projectId: 'proj-a' },
      { name: 'beta', projectId: 'proj-a' },
    ])
    const wrapper = await mountPanel('proj-a', { agent: 'alpha' })
    await flushPromises()
    await wrapper.get('[data-testid="project-agents-subtab-test"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-testid="agents-chat-tester"]').text()).toBe('alpha')
    await wrapper.vm.$router.replace({ query: { agent: 'beta' } })
    await flushPromises()
    expect(wrapper.get('[data-testid="agents-chat-tester"]').text()).toBe('beta')
    wrapper.unmount()
  })

  it('shows empty state when no project-bound agents', async () => {
    apiMocks.listAgents.mockResolvedValue([{ name: 'other', projectId: 'proj-b' }])
    const wrapper = await mountPanel('proj-a')
    await flushPromises()
    await wrapper.get('[data-testid="project-agents-subtab-test"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="project-agents-chat-test-empty"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="agents-chat-tester"]').exists()).toBe(false)
    wrapper.unmount()
  })
})
