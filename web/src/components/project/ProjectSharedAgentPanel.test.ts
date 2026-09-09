// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'
import ProjectSharedAgentPanel from './ProjectSharedAgentPanel.vue'

const apiMocks = vi.hoisted(() => ({
  getProjectSharedAgentConfig: vi.fn(),
  listAgents: vi.fn(),
  putProjectSharedAgentConfig: vi.fn(),
  createProjectSharedAgentTest: vi.fn(),
}))

vi.mock('@/lib/api/api', async () => {
  const actual = await vi.importActual<typeof import('@/lib/api/api')>('@/lib/api/api')
  return {
    ...actual,
    api: {
      ...actual.api,
      getProjectSharedAgentConfig: apiMocks.getProjectSharedAgentConfig,
      listAgents: apiMocks.listAgents,
      putProjectSharedAgentConfig: apiMocks.putProjectSharedAgentConfig,
      createProjectSharedAgentTest: apiMocks.createProjectSharedAgentTest,
    },
  }
})

const sharedCfg = {
  acpBackend: 'cursor',
  defaultProjectId: 'proj-a',
  gitCredentialType: '',
  files: [],
  mcp: [],
  env: {},
  layout: {},
  prompts: {},
}

function mountPanel(projectId = 'proj-a') {
  const i18n = createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...common, ...pages } },
  })
  return mount(ProjectSharedAgentPanel, {
    props: { projectId },
    global: {
      plugins: [i18n],
      stubs: {
        Icon: true,
        AgentFilesPanel: true,
        AgentMcpPanel: true,
        AgentEnvPanel: true,
        AgentPromptsPanel: true,
        AgentChatTester: {
          props: ['profile', 'homeProjectId', 'createTest'],
          template: '<div data-testid="shared-agent-chat-tester">{{ profile }}</div>',
        },
      },
    },
  })
}

describe('ProjectSharedAgentPanel chat test picker', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    apiMocks.getProjectSharedAgentConfig.mockResolvedValue(sharedCfg)
  })

  it('lists only agents bound to the current project', async () => {
    apiMocks.listAgents.mockResolvedValue([
      { name: 'mine', projectId: 'proj-a' },
      { name: 'other', projectId: 'proj-b' },
      { name: 'free' },
    ])
    const wrapper = mountPanel('proj-a')
    await flushPromises()
    await wrapper.get('[data-testid="shared-agent-subtab-test"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-testid="project-agent-select-trigger"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="project-agent-select-option-mine"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="project-agent-select-option-other"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="shared-agent-chat-tester"]').text()).toBe('mine')
    wrapper.unmount()
  })

  it('replaces the standalone hint with an accessible help disclosure', async () => {
    apiMocks.listAgents.mockResolvedValue([])
    const wrapper = mountPanel()
    await flushPromises()

    expect(wrapper.find('[data-testid="shared-agent-hint"]').exists()).toBe(false)
    const help = wrapper.get('[data-testid="shared-agent-help"]')
    const summary = help.get('summary')
    expect(summary.text()).toBe('?')
    expect(summary.attributes('aria-label')).toContain('extend')
    expect(summary.classes()).toEqual(expect.arrayContaining(['h-[18px]', 'w-[18px]']))
    expect(wrapper.get('[data-testid="shared-agent-help-text"]').text()).toContain('extend')
    expect(wrapper.get('[data-testid="project-shared-agent-panel"]').classes()).toEqual(
      expect.arrayContaining(['min-h-0', 'flex-1']),
    )
    wrapper.unmount()
  })

  it('shows empty state when no project-bound agents exist', async () => {
    apiMocks.listAgents.mockResolvedValue([
      { name: 'other', projectId: 'proj-b' },
    ])
    const wrapper = mountPanel('proj-a')
    await flushPromises()
    await wrapper.get('[data-testid="shared-agent-subtab-test"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="shared-agent-test-empty"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="shared-agent-chat-tester"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('filters combobox candidates and drives tester selection', async () => {
    apiMocks.listAgents.mockResolvedValue([
      { name: 'alpha', projectId: 'proj-a' },
      { name: 'beta', projectId: 'proj-a' },
    ])
    const wrapper = mountPanel('proj-a')
    await flushPromises()
    await wrapper.get('[data-testid="shared-agent-subtab-test"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-testid="project-agent-select-trigger"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-testid="project-agent-select-search"]').setValue('beta')
    await flushPromises()
    await wrapper.get('[data-testid="project-agent-select-option-beta"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="shared-agent-chat-tester"]').text()).toBe('beta')
    wrapper.unmount()
  })
})

function mountPanelWithRealPrompts(projectId = 'proj-a') {
  const i18n = createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...common, ...pages } },
  })
  return mount(ProjectSharedAgentPanel, {
    props: { projectId },
    global: {
      plugins: [i18n],
      stubs: {
        Icon: true,
        AgentFilesPanel: true,
        AgentMcpPanel: true,
        AgentEnvPanel: true,
        AgentChatTester: true,
      },
    },
  })
}

describe('ProjectSharedAgentPanel prompts dirty (g1.3 / g2.1 / g2.2)', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    apiMocks.listAgents.mockResolvedValue([])
  })

  it('zero-edit switch to prompts stays clean for empty and CRLF prompts', async () => {
    apiMocks.getProjectSharedAgentConfig.mockResolvedValue({
      ...sharedCfg,
      prompts: { producesContract: '共享\r\n片段' },
    })
    const wrapper = mountPanelWithRealPrompts()
    await flushPromises()
    const saveBtn = () => wrapper.get('[data-testid="shared-agent-save"]')
    expect(saveBtn().attributes('disabled')).toBeDefined()
    await wrapper.get('[data-testid="shared-agent-subtab-prompts"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="prompt-producesContract"]').exists()).toBe(true)
    expect(saveBtn().attributes('disabled')).toBeDefined()
    await wrapper.get('[data-testid="shared-agent-subtab-files"]').trigger('click')
    await flushPromises()
    expect(saveBtn().attributes('disabled')).toBeDefined()
    wrapper.unmount()
  })

  it('real prompt edit enables save; put success clears dirty', async () => {
    apiMocks.getProjectSharedAgentConfig.mockResolvedValue({
      ...sharedCfg,
      prompts: { reactOpenSuffix: 'base' },
    })
    apiMocks.putProjectSharedAgentConfig.mockImplementation(async (_id: string, body: Record<string, unknown>) => ({
      ...sharedCfg,
      ...body,
    }))
    const wrapper = mountPanelWithRealPrompts()
    await flushPromises()
    await wrapper.get('[data-testid="shared-agent-subtab-prompts"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-testid="prompt-reactOpenSuffix"]').setValue('base-changed')
    await flushPromises()
    const save = wrapper.get('[data-testid="shared-agent-save"]')
    expect(save.attributes('disabled')).toBeUndefined()
    await save.trigger('click')
    await flushPromises()
    expect(apiMocks.putProjectSharedAgentConfig).toHaveBeenCalled()
    expect(wrapper.get('[data-testid="shared-agent-save"]').attributes('disabled')).toBeDefined()
    wrapper.unmount()
  })
})
