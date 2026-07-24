// @vitest-environment happy-dom
import { defineComponent, h, nextTick } from 'vue'
import { createI18n } from 'vue-i18n'
import { createMemoryHistory, createRouter } from 'vue-router'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'
import type { Agent } from '@/lib/api'

const mocks = vi.hoisted(() => ({
  listAgents: vi.fn(),
  listProjects: vi.fn(),
  saveAgent: vi.fn(),
  getAgentsOrg: vi.fn(),
  saveAgentsOrg: vi.fn(),
  renameAgent: vi.fn(),
}))

const breakpointMocks = vi.hoisted(() => {
  // eslint-disable-next-line @typescript-eslint/no-require-imports
  const vue = require('vue') as typeof import('vue')
  return { isMobile: vue.ref(false) }
})

vi.mock('@/lib/api', async () => {
  const actual = await vi.importActual<typeof import('@/lib/api')>('@/lib/api')
  return {
    ...actual,
    api: {
      ...actual.api,
      listAgents: mocks.listAgents,
      listProjects: mocks.listProjects,
      saveAgent: mocks.saveAgent,
      getAgentsOrg: mocks.getAgentsOrg,
      saveAgentsOrg: mocks.saveAgentsOrg,
      renameAgent: mocks.renameAgent,
    },
  }
})

vi.mock('@/lib/useProjectContext', () => ({
  useProjectContext: () => ({
    selected: { value: 'proj-default' },
    ensureHydrated: vi.fn(),
    setProject: vi.fn(),
  }),
}))

vi.mock('@/lib/useBreakpoint', () => ({
  useBreakpoint: () => ({ isMobile: breakpointMocks.isMobile }),
}))

import AgentStudioView from './AgentStudioView.vue'

const ButtonStub = defineComponent({
  inheritAttrs: false,
  template: '<button v-bind="$attrs"><slot /></button>',
})
const CodeEditorStub = defineComponent({
  props: { modelValue: String },
  emits: ['update:modelValue'],
  template: '<textarea data-test="code-editor" :value="modelValue" @input="$emit(\'update:modelValue\', $event.target.value)" />',
})

function agent(region?: string): Agent {
  return {
    name: 'legacy',
    projectId: 'proj-default',
    acpBackend: 'codebuddy',
    files: [],
    mcp: [],
    env: region === undefined ? {} : { APPROVING_CODEBUDDY_REGION: region },
    layout: { configRoot: '/root/.codebuddy', workspaceDir: '/root/workspace' },
  }
}

async function createStudioRouter(query: Record<string, string> = {}) {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/agents', component: { render: () => h('div') } }],
  })
  await router.push({ path: '/agents', query })
  await router.isReady()
  return router
}

async function mountStudio(query: Record<string, string> = {}) {
  const i18n = createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...common, ...pages } },
  })
  const router = await createStudioRouter(query)
  return mount(AgentStudioView, {
    global: {
      plugins: [i18n, router],
      stubs: {
        AppButton: ButtonStub,
        Icon: true,
        AppModal: true,
        CodeEditor: CodeEditorStub,
        MarkdownSplitEditor: true,
        ExplorerContextMenu: true,
        AgentChatTester: true,
        AgentGitGuide: true,
        AgentCreateWizard: true,
        AgentOrgSidebar: true,
        AgentDataPanel: true,
      },
    },
  })
}

beforeEach(() => {
  vi.clearAllMocks()
  breakpointMocks.isMobile.value = false
  mocks.listProjects.mockResolvedValue([{ id: 'proj-default', name: 'Default' }])
  mocks.saveAgent.mockImplementation(async (payload: Agent) => payload)
  mocks.getAgentsOrg.mockResolvedValue({ revision: 0, groups: [], agents: {} })
  mocks.saveAgentsOrg.mockImplementation(async (org: { revision?: number }) => ({
    revision: (org.revision || 0) + 1,
    groups: [],
    agents: {},
    ...org,
  }))
})

describe('AgentStudio region UI', () => {
  it('shows special CodeBuddy values without selecting a canonical site', async () => {
    mocks.listAgents.mockResolvedValue([agent('ioa')])
    const wrapper = await mountStudio()
    await flushPromises()
    await wrapper.findAll('button').find((item) => item.text() === '元信息')!.trigger('click')

    expect(wrapper.text()).toContain('现有特殊配置：ioa')
    expect(wrapper.find('[role="radiogroup"]').exists()).toBe(true)
    const siteRadios = wrapper.findAll('button[role="radio"]')
    expect(siteRadios).toHaveLength(2)
    expect(siteRadios.every((item) => item.attributes('aria-checked') === 'false')).toBe(true)
  })

  it('preserves special CodeBuddy region when saving without site interaction', async () => {
    mocks.listAgents.mockResolvedValue([agent('ioa')])
    const wrapper = await mountStudio()
    await flushPromises()
    await wrapper.findAll('button').find((item) => item.text() === '元信息')!.trigger('click')

    const workspaceInput = wrapper.findAll('input').find((item) => {
      return (item.element as HTMLInputElement).value === '/root/workspace'
    })!
    await workspaceInput.setValue('/root/workspace-edited')
    await wrapper.findAll('button').find((item) => item.text() === '保存')!.trigger('click')
    await flushPromises()

    expect(mocks.saveAgent).toHaveBeenCalledWith(
      expect.objectContaining({
        env: { APPROVING_CODEBUDDY_REGION: 'ioa' },
        layout: expect.objectContaining({ workspaceDir: '/root/workspace-edited' }),
      }),
    )
  })

  it('overwrites special CodeBuddy region after explicitly selecting a site', async () => {
    mocks.listAgents.mockResolvedValue([agent('ioa')])
    const wrapper = await mountStudio()
    await flushPromises()
    await wrapper.findAll('button').find((item) => item.text() === '元信息')!.trigger('click')

    await wrapper.findAll('button').find((item) => item.text().trim().startsWith('国内站'))!.trigger('click')
    await wrapper.findAll('button').find((item) => item.text() === '保存')!.trigger('click')
    await flushPromises()
    expect(mocks.saveAgent).toHaveBeenCalledWith(
      expect.objectContaining({ env: { APPROVING_CODEBUDDY_REGION: 'internal' } }),
    )
  })

  it('makes a missing region dirty and saves the international default', async () => {
    mocks.listAgents.mockResolvedValue([agent()])
    const wrapper = await mountStudio()
    await flushPromises()

    expect(wrapper.text()).toContain('未保存')
    await wrapper.findAll('button').find((item) => item.text() === '保存')!.trigger('click')
    await flushPromises()
    expect(mocks.saveAgent).toHaveBeenCalledWith(
      expect.objectContaining({ env: { APPROVING_CODEBUDDY_REGION: 'public' } }),
    )
  })

  it('normalizes Raw JSON conflicts without overriding the ACP site selection', async () => {
    mocks.listAgents.mockResolvedValue([agent('public')])
    const wrapper = await mountStudio()
    await flushPromises()
    await wrapper.findAll('button').find((item) => item.text() === '环境变量 (0)')!.trigger('click')
    await wrapper.findAll('button').find((item) => item.text() === '原始 JSON')!.trigger('click')
    await wrapper.get('[data-test="code-editor"]').setValue(
      JSON.stringify({
        APPROVING_CODEBUDDY_REGION: 'internal',
        APPROVING_TRAE_REGION: 'cn',
        OTHER: 'ok',
      }),
    )
    await wrapper.findAll('button').find((item) => item.text() === '保存')!.trigger('click')
    await flushPromises()

    expect(mocks.saveAgent).toHaveBeenCalledWith(
      expect.objectContaining({
        env: { APPROVING_CODEBUDDY_REGION: 'public', OTHER: 'ok' },
      }),
    )
  })
})

describe('AgentStudio MCP PM leader prefills', () => {
  async function openMcpTab(wrapper: Awaited<ReturnType<typeof mountStudio>>) {
    await wrapper.findAll('button').find((item) => item.text().startsWith('MCP'))!.trigger('click')
    await flushPromises()
  }

  it('adds memory-store from the agent platform card without changing artifact-store', async () => {
    mocks.listAgents.mockResolvedValue([
      {
        ...agent(),
        mcp: [
          {
            name: 'artifact-store',
            url: '${APPROVING_ARTIFACT_URL}',
            headers: { Authorization: 'Bearer ${APPROVING_ARTIFACT_TOKEN}' },
          },
        ],
      },
    ])
    const wrapper = await mountStudio()
    await flushPromises()
    await openMcpTab(wrapper)

    expect(wrapper.text()).toContain('Agent 通用平台 MCP')
    expect(wrapper.text()).toContain('APPROVING_MEMORY_URL')
    expect(wrapper.text()).toContain('pm-progress')

    await wrapper.findAll('button').find((item) => item.text().includes('+ memory-store'))!.trigger('click')
    await flushPromises()

    expect(wrapper.findAll('input').filter((el) => (el.element as HTMLInputElement).value === 'memory-store')).toHaveLength(1)
    expect(wrapper.findAll('input').filter((el) => (el.element as HTMLInputElement).value === '${APPROVING_MEMORY_URL}')).toHaveLength(1)
    expect(wrapper.findAll('input').filter((el) => (el.element as HTMLInputElement).value === 'Bearer ${APPROVING_MEMORY_TOKEN}')).toHaveLength(1)
    expect(wrapper.text()).toContain('Agent 级平台 MCP')
    expect(wrapper.findAll('input').filter((el) => (el.element as HTMLInputElement).value === 'artifact-store')).toHaveLength(1)
  })

  it('toasts and keeps a single memory-store when adding again', async () => {
    mocks.listAgents.mockResolvedValue([
      {
        ...agent(),
        mcp: [
          {
            name: 'memory-store',
            url: '${APPROVING_MEMORY_URL}',
            headers: { Authorization: 'Bearer ${APPROVING_MEMORY_TOKEN}' },
          },
        ],
      },
    ])
    const wrapper = await mountStudio()
    await flushPromises()
    await openMcpTab(wrapper)

    const addBtn = wrapper.findAll('button').find((item) => item.text().includes('+ memory-store'))!
    expect(addBtn.exists()).toBe(true)
    await addBtn.trigger('click')
    await flushPromises()

    expect(document.body.textContent || '').toContain('已存在约定名 memory-store')
    expect(wrapper.findAll('input').filter((el) => (el.element as HTMLInputElement).value === 'memory-store')).toHaveLength(1)
  })

  it('shows legacy upgrade hint for pm-leader and upgrades in place', async () => {
    mocks.listAgents.mockResolvedValue([
      {
        ...agent(),
        mcp: [
          {
            name: 'artifact-store',
            url: '${APPROVING_ARTIFACT_URL}',
            headers: { Authorization: 'Bearer ${APPROVING_ARTIFACT_TOKEN}' },
          },
          {
            name: 'pm-leader',
            url: '${APPROVING_PM_URL}',
            headers: { Authorization: 'Bearer ${APPROVING_PM_TOKEN}' },
          },
        ],
      },
    ])
    const wrapper = await mountStudio()
    await flushPromises()
    await openMcpTab(wrapper)

    expect(wrapper.text()).toContain('检测到旧约定名 pm-leader')
    await wrapper.findAll('button').find((item) => item.text().includes('一键升级'))!.trigger('click')
    await flushPromises()

    expect(wrapper.findAll('input').filter((el) => (el.element as HTMLInputElement).value === 'pm-leader')).toHaveLength(0)
    expect(wrapper.findAll('input').filter((el) => (el.element as HTMLInputElement).value === 'memory-store')).toHaveLength(1)
    expect(wrapper.findAll('input').filter((el) => (el.element as HTMLInputElement).value === 'context-store')).toHaveLength(1)
    expect(wrapper.findAll('input').filter((el) => (el.element as HTMLInputElement).value === 'task-scheduler')).toHaveLength(1)
    expect(wrapper.findAll('input').filter((el) => (el.element as HTMLInputElement).value === 'artifact-store')).toHaveLength(1)
  })

  it('hides the agent scope badge after renaming away from memory-store', async () => {
    mocks.listAgents.mockResolvedValue([
      {
        ...agent(),
        mcp: [
          {
            name: 'memory-store',
            url: '${APPROVING_MEMORY_URL}',
            headers: { Authorization: 'Bearer ${APPROVING_MEMORY_TOKEN}' },
          },
        ],
      },
    ])
    const wrapper = await mountStudio()
    await flushPromises()
    await openMcpTab(wrapper)

    expect(wrapper.text()).toContain('Agent 级平台 MCP')
    const nameInput = wrapper.findAll('input').find((el) => (el.element as HTMLInputElement).value === 'memory-store')!
    await nameInput.setValue('memory_store')
    await flushPromises()
    expect(wrapper.text()).not.toContain('Agent 级平台 MCP')
  })

  it('removing memory-store keeps artifact-store intact', async () => {
    mocks.listAgents.mockResolvedValue([
      {
        ...agent(),
        mcp: [
          {
            name: 'artifact-store',
            url: '${APPROVING_ARTIFACT_URL}',
            headers: { Authorization: 'Bearer ${APPROVING_ARTIFACT_TOKEN}' },
          },
          {
            name: 'memory-store',
            url: '${APPROVING_MEMORY_URL}',
            headers: { Authorization: 'Bearer ${APPROVING_MEMORY_TOKEN}' },
          },
        ],
      },
    ])
    const wrapper = await mountStudio()
    await flushPromises()
    await openMcpTab(wrapper)

    const nameInput = wrapper.findAll('input').find((el) => (el.element as HTMLInputElement).value === 'memory-store')!
    const card = nameInput.element.closest('.rounded-md.border') as HTMLElement
    const removeBtn = wrapper.findAll('button').find((item) => {
      return item.attributes('title') === '移除' && item.element.closest('.rounded-md.border') === card
    })!
    await removeBtn.trigger('click')
    await flushPromises()

    expect(wrapper.findAll('input').filter((el) => (el.element as HTMLInputElement).value === 'memory-store')).toHaveLength(0)
    expect(wrapper.findAll('input').filter((el) => (el.element as HTMLInputElement).value === 'artifact-store')).toHaveLength(1)
  })
})

describe('AgentStudio rename entry migration', () => {
  const ModalStub = defineComponent({
    props: { open: Boolean, title: String },
    emits: ['close'],
    template: '<div v-if="open" data-test="modal"><h2>{{ title }}</h2><slot /><slot name="footer" /></div>',
  })
  const SidebarStub = defineComponent({
    emits: ['rename-agent', 'open-manage', 'select-agent'],
    template:
      '<div data-test="sidebar">' +
      '<button data-test="pencil" @click="$emit(\'rename-agent\', \'legacy\')">pencil</button>' +
      '<button data-test="manage" @click="$emit(\'open-manage\')">manage</button>' +
      '</div>',
  })

  async function mountRenameStudio() {
    const i18n = createI18n({
      legacy: false,
      locale: 'zh-CN',
      messages: { 'zh-CN': { ...common, ...pages } },
    })
    const router = await createStudioRouter()
    return mount(AgentStudioView, {
      global: {
        plugins: [i18n, router],
        stubs: {
          AppButton: ButtonStub,
          Icon: true,
          AppModal: ModalStub,
          CodeEditor: CodeEditorStub,
          MarkdownSplitEditor: true,
          ExplorerContextMenu: true,
          AgentChatTester: true,
          AgentGitGuide: true,
          AgentCreateWizard: true,
          AgentOrgSidebar: SidebarStub,
          AgentDataPanel: true,
        },
      },
    })
  }

  it('blocks sidebar pencil rename and does not call api.renameAgent', async () => {
    mocks.listAgents.mockResolvedValue([agent('public')])
    const wrapper = await mountRenameStudio()
    await flushPromises()

    await wrapper.get('[data-test="pencil"]').trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('这里不支持修改 Agent 名字')
    expect(wrapper.text()).toContain('前往 Agent 管理')
    expect(mocks.renameAgent).not.toHaveBeenCalled()
  })

  it('opens Agent management with Rename and focuses the blocked target', async () => {
    mocks.listAgents.mockResolvedValue([agent('public')])
    const wrapper = await mountRenameStudio()
    await flushPromises()

    await wrapper.get('[data-test="pencil"]').trigger('click')
    await flushPromises()
    await wrapper.findAll('button').find((b) => b.text() === '前往 Agent 管理')!.trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('Agent 管理')
    expect(wrapper.text()).toContain('Rename')
    const focused = wrapper.find('[data-manage-agent="legacy"]')
    expect(focused.exists()).toBe(true)
    expect(focused.classes().join(' ')).toMatch(/accent/)
  })

  it('shows Metadata rename hint and keeps header name read-only', async () => {
    mocks.listAgents.mockResolvedValue([agent('public')])
    const wrapper = await mountRenameStudio()
    await flushPromises()

    await wrapper.findAll('button').find((item) => item.text() === '元信息')!.trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('名称请在 Agent 管理中重命名')
    // header name is a span, not an input
    const headerName = wrapper.findAll('span').find((s) => s.text() === 'legacy' && s.classes().includes('font-medium'))
    expect(headerName).toBeTruthy()
  })

  it('management Rename opens confirm with workflow warning; cancel does not rename', async () => {
    mocks.listAgents.mockResolvedValue([agent('public')])
    const wrapper = await mountRenameStudio()
    await flushPromises()

    await wrapper.get('[data-test="manage"]').trigger('click')
    await flushPromises()
    await wrapper.findAll('button').find((b) => b.text() === 'Rename')!.trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('工作流引用不会自动更新')
    expect(wrapper.text()).toContain('skill_profile')
    await wrapper.findAll('button').find((b) => b.text() === '取消')!.trigger('click')
    await flushPromises()
    expect(mocks.renameAgent).not.toHaveBeenCalled()
  })
})

describe('AgentStudio mobile core path', () => {
  const ModalStub = defineComponent({
    props: { open: Boolean, title: String },
    emits: ['close'],
    template: '<div v-if="open" data-test="modal"><h2>{{ title }}</h2><slot /><slot name="footer" /></div>',
  })
  const MdStub = defineComponent({
    props: { modelValue: String, filePath: String, variant: String },
    emits: ['update:modelValue'],
    template:
      '<div data-test="md-editor" :data-variant="variant">' +
      '<textarea :value="modelValue" @input="$emit(\'update:modelValue\', $event.target.value)" />' +
      '</div>',
  })

  function agentWithFiles(): Agent {
    return {
      ...agent('public'),
      files: [
        { path: 'AGENTS.md', content: '# hello\n' },
        { path: 'rules/system.md', content: '---\ntitle: system\n---\n\n# rule\n' },
      ],
    }
  }

  async function mountMobileStudio() {
    const i18n = createI18n({
      legacy: false,
      locale: 'zh-CN',
      messages: { 'zh-CN': { ...common, ...pages } },
    })
    const router = await createStudioRouter()
    return mount(AgentStudioView, {
      global: {
        plugins: [i18n, router],
        stubs: {
          AppButton: ButtonStub,
          Icon: true,
          AppModal: ModalStub,
          CodeEditor: CodeEditorStub,
          MarkdownSplitEditor: MdStub,
          ExplorerContextMenu: true,
          AgentChatTester: true,
          AgentGitGuide: true,
          AgentCreateWizard: true,
          AgentOrgSidebar: true,
          AgentDataPanel: true,
        },
      },
    })
  }

  beforeEach(() => {
    breakpointMocks.isMobile.value = true
  })

  async function openSystemMd(wrapper: Awaited<ReturnType<typeof mountMobileStudio>>) {
    await wrapper.findAll('button').find((b) => b.text().includes('rules'))!.trigger('click')
    await flushPromises()
    await wrapper.findAll('button').find((b) => b.text().includes('system.md'))!.trigger('click')
    await flushPromises()
  }

  it('starts on files list step without side-by-side editor', async () => {
    mocks.listAgents.mockResolvedValue([agentWithFiles()])
    const wrapper = await mountMobileStudio()
    await flushPromises()

    expect(wrapper.text()).toContain('资源管理器')
    expect(wrapper.find('[data-test="md-editor"]').exists()).toBe(false)
    expect(wrapper.text()).toContain('AGENTS.md')
    expect(wrapper.text()).toContain('rules')
  })

  it('opens file into edit step with stack markdown editor', async () => {
    mocks.listAgents.mockResolvedValue([agentWithFiles()])
    const wrapper = await mountMobileStudio()
    await flushPromises()

    await openSystemMd(wrapper)

    expect(wrapper.text()).toContain('返回')
    const md = wrapper.get('[data-test="md-editor"]')
    expect(md.attributes('data-variant')).toBe('stack')
    expect(wrapper.text()).toContain('rules/system.md')
  })

  it('stays on edit step after save and shows saved state', async () => {
    mocks.listAgents.mockResolvedValue([agentWithFiles()])
    const wrapper = await mountMobileStudio()
    await flushPromises()

    await openSystemMd(wrapper)

    const ta = wrapper.get('[data-test="md-editor"] textarea')
    await ta.setValue('---\ntitle: system\n---\n\n# changed\n')
    await flushPromises()
    expect(wrapper.text()).toContain('未保存')

    await wrapper.findAll('button').find((b) => b.text() === '保存')!.trigger('click')
    await flushPromises()

    expect(mocks.saveAgent).toHaveBeenCalled()
    expect(wrapper.text()).toContain('返回')
    expect(wrapper.find('[data-test="md-editor"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('已保存')
  })

  it('prompts save/discard/cancel when returning dirty from edit', async () => {
    mocks.listAgents.mockResolvedValue([agentWithFiles()])
    const wrapper = await mountMobileStudio()
    await flushPromises()

    await openSystemMd(wrapper)
    await wrapper.get('[data-test="md-editor"] textarea').setValue('# dirty\n')
    await flushPromises()

    await wrapper.findAll('button').find((b) => b.text().includes('返回'))!.trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('有未保存的修改')
    expect(wrapper.text()).toContain('保存并返回')
    expect(wrapper.text()).toContain('丢弃修改')

    await wrapper.findAll('button').find((b) => b.text() === '取消')!.trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-test="md-editor"]').exists()).toBe(true)
  })

  it('keeps leave confirm open when save fails', async () => {
    mocks.listAgents.mockResolvedValue([agentWithFiles()])
    mocks.saveAgent.mockRejectedValueOnce(new Error('save failed'))
    const wrapper = await mountMobileStudio()
    await flushPromises()

    await openSystemMd(wrapper)
    await wrapper.get('[data-test="md-editor"] textarea').setValue('# dirty\n')
    await flushPromises()

    await wrapper.findAll('button').find((b) => b.text().includes('返回'))!.trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('保存并返回')

    await wrapper.findAll('button').find((b) => b.text() === '保存并返回')!.trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('有未保存的修改')
    expect(wrapper.text()).toContain('保存并返回')
    expect(wrapper.find('[data-test="md-editor"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('save failed')
  })

  it('prompts when switching tab from dirty edit step', async () => {
    mocks.listAgents.mockResolvedValue([agentWithFiles()])
    const wrapper = await mountMobileStudio()
    await flushPromises()

    await openSystemMd(wrapper)
    await wrapper.get('[data-test="md-editor"] textarea').setValue('# dirty\n')
    await flushPromises()

    await wrapper.findAll('button').find((b) => b.text().startsWith('MCP'))!.trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('有未保存的修改')
    expect(wrapper.text()).toContain('保存并继续')
    await wrapper.findAll('button').find((b) => b.text() === '丢弃修改')!.trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('请在桌面端完成')
  })

  it('shows desktop-only tip for non-core tabs including meta', async () => {
    mocks.listAgents.mockResolvedValue([agentWithFiles()])
    const wrapper = await mountMobileStudio()
    await flushPromises()

    await wrapper.findAll('button').find((b) => b.text() === '元信息')!.trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('请在桌面端完成')
    expect(wrapper.text()).not.toContain('ACP 后端')
  })

  it('shows switch entry and opens org sheet with groups/ungrouped', async () => {
    mocks.listAgents.mockResolvedValue([
      { ...agentWithFiles(), name: 'alpha' },
      { ...agentWithFiles(), name: 'beta' },
    ])
    mocks.getAgentsOrg.mockResolvedValue({
      revision: 1,
      groups: [{ id: 'g1', name: '工程部' }],
      agents: {
        alpha: { groupIds: ['g1'] },
        beta: { groupIds: [] },
      },
    })
    const wrapper = await mountMobileStudio()
    await flushPromises()

    const switchBtn = wrapper.get('[data-test="org-switch"]')
    expect(switchBtn.text()).toContain('切换')
    await switchBtn.trigger('click')
    await flushPromises()

    const sheet = document.querySelector('[data-test="org-sheet"]')
    expect(sheet).toBeTruthy()
    expect(sheet!.textContent).toContain('Agent 组织树')
    expect(sheet!.textContent).toContain('工程部')
    expect(sheet!.textContent).toContain('未分组')
    expect(sheet!.textContent).toContain('alpha')
    expect(sheet!.textContent).toContain('beta')

    // Agent 叶子名称主文字色契约（选中/未选中均 text-txt，非灰）
    const agentBtns = Array.from(document.querySelectorAll('[data-test="org-sheet-agent"]'))
    for (const btn of agentBtns) {
      const nameSpan = btn.querySelector('.truncate.text-txt') as HTMLElement | null
      expect(nameSpan).toBeTruthy()
      expect(nameSpan!.className).not.toMatch(/text-txt2|text-txt3/)
    }
    const alphaRow = document.querySelector('[data-org-kind="agent"][data-org-name="alpha"]')
    expect(alphaRow?.className).toContain('bg-accent-dim')
    const groupName = sheet!.querySelector('.truncate.font-medium.text-txt2')
    expect(groupName?.textContent).toContain('工程部')

    wrapper.unmount()
  })

  it('switches agent without dirty and closes sheet', async () => {
    mocks.listAgents.mockResolvedValue([
      { ...agentWithFiles(), name: 'alpha' },
      {
        ...agentWithFiles(),
        name: 'beta',
        files: [{ path: 'README.md', content: '# beta\n' }],
      },
    ])
    mocks.getAgentsOrg.mockResolvedValue({
      revision: 1,
      groups: [],
      agents: {},
    })
    const wrapper = await mountMobileStudio()
    await flushPromises()
    expect(wrapper.text()).toContain('alpha')

    await wrapper.get('[data-test="org-switch"]').trigger('click')
    await flushPromises()
    const betaBtn = Array.from(document.querySelectorAll('[data-test="org-sheet-agent"]')).find(
      (el) => el.textContent?.includes('beta'),
    ) as HTMLElement
    expect(betaBtn).toBeTruthy()
    betaBtn.click()
    await flushPromises()

    expect(document.querySelector('[data-test="org-sheet"]')).toBeNull()
    expect(wrapper.text()).toContain('beta')
    expect(wrapper.text()).toContain('README.md')
    wrapper.unmount()
  })

  it('dirty switch shows three actions; cancel keeps sheet and dirty state', async () => {
    mocks.listAgents.mockResolvedValue([
      { ...agentWithFiles(), name: 'alpha' },
      { ...agentWithFiles(), name: 'beta' },
    ])
    const wrapper = await mountMobileStudio()
    await flushPromises()

    await openSystemMd(wrapper)
    await wrapper.get('[data-test="md-editor"] textarea').setValue('# dirty\n')
    await flushPromises()
    expect(wrapper.text()).toContain('未保存')

    await wrapper.get('[data-test="org-switch"]').trigger('click')
    await flushPromises()
    const betaBtn = Array.from(document.querySelectorAll('[data-test="org-sheet-agent"]')).find(
      (el) => el.textContent?.includes('beta'),
    ) as HTMLElement
    betaBtn.click()
    await flushPromises()

    expect(wrapper.text()).toContain('保存并切换')
    expect(wrapper.text()).toContain('丢弃并切换')
    expect(wrapper.text()).toContain('取消')
    expect(document.querySelector('[data-test="org-sheet"]')).toBeTruthy()

    await wrapper.findAll('button').find((b) => b.text() === '取消')!.trigger('click')
    await flushPromises()

    expect(document.querySelector('[data-test="org-sheet"]')).toBeTruthy()
    expect(wrapper.text()).toContain('未保存')
    expect(wrapper.text()).toContain('alpha')
    wrapper.unmount()
  })

  it('dirty switch save-and-switch closes sheet and selects target agent', async () => {
    mocks.listAgents.mockResolvedValue([
      { ...agentWithFiles(), name: 'alpha' },
      {
        ...agentWithFiles(),
        name: 'beta',
        files: [{ path: 'README.md', content: '# beta\n' }],
      },
    ])
    const wrapper = await mountMobileStudio()
    await flushPromises()

    await openSystemMd(wrapper)
    await wrapper.get('[data-test="md-editor"] textarea').setValue('# dirty save\n')
    await flushPromises()
    expect(wrapper.text()).toContain('未保存')

    await wrapper.get('[data-test="org-switch"]').trigger('click')
    await flushPromises()
    const betaBtn = Array.from(document.querySelectorAll('[data-test="org-sheet-agent"]')).find(
      (el) => el.textContent?.includes('beta'),
    ) as HTMLElement
    betaBtn.click()
    await flushPromises()

    await wrapper.findAll('button').find((b) => b.text() === '保存并切换')!.trigger('click')
    await flushPromises()

    expect(mocks.saveAgent).toHaveBeenCalled()
    expect(document.querySelector('[data-test="org-sheet"]')).toBeNull()
    expect(wrapper.text()).toContain('beta')
    expect(wrapper.text()).toContain('README.md')
    expect(wrapper.text()).not.toContain('未保存')
    wrapper.unmount()
  })

  it('dirty switch discard-and-switch closes sheet and selects target agent', async () => {
    mocks.listAgents.mockResolvedValue([
      { ...agentWithFiles(), name: 'alpha' },
      {
        ...agentWithFiles(),
        name: 'beta',
        files: [{ path: 'README.md', content: '# beta\n' }],
      },
    ])
    const wrapper = await mountMobileStudio()
    await flushPromises()

    await openSystemMd(wrapper)
    await wrapper.get('[data-test="md-editor"] textarea').setValue('# dirty discard\n')
    await flushPromises()
    expect(wrapper.text()).toContain('未保存')

    await wrapper.get('[data-test="org-switch"]').trigger('click')
    await flushPromises()
    const betaBtn = Array.from(document.querySelectorAll('[data-test="org-sheet-agent"]')).find(
      (el) => el.textContent?.includes('beta'),
    ) as HTMLElement
    betaBtn.click()
    await flushPromises()

    await wrapper.findAll('button').find((b) => b.text() === '丢弃并切换')!.trigger('click')
    await flushPromises()

    expect(mocks.saveAgent).not.toHaveBeenCalled()
    expect(document.querySelector('[data-test="org-sheet"]')).toBeNull()
    expect(wrapper.text()).toContain('beta')
    expect(wrapper.text()).toContain('README.md')
    wrapper.unmount()
  })

  it('org sheet manage opens existing agent manage modal', async () => {
    mocks.listAgents.mockResolvedValue([agentWithFiles()])
    const wrapper = await mountMobileStudio()
    await flushPromises()

    await wrapper.get('[data-test="org-switch"]').trigger('click')
    await flushPromises()
    const manageBtn = document.querySelector('[data-test="org-sheet-manage"]') as HTMLButtonElement
    expect(manageBtn).toBeTruthy()
    manageBtn.click()
    await flushPromises()

    expect(wrapper.text()).toContain('Agent 管理')
    wrapper.unmount()
  })

  it('keeps file row actions visible without hover on mobile', async () => {
    mocks.listAgents.mockResolvedValue([agentWithFiles()])
    const wrapper = await mountMobileStudio()
    await flushPromises()

    const actions = wrapper.findAll('[data-test="file-row-action"]')
    expect(actions.length).toBeGreaterThan(0)
    for (const btn of actions) {
      expect(btn.classes().join(' ')).toContain('opacity-100')
      expect(btn.classes().join(' ')).not.toContain('opacity-0')
    }
    wrapper.unmount()
  })

  it('closes org sheet when breakpoint flips to desktop', async () => {
    mocks.listAgents.mockResolvedValue([agentWithFiles()])
    const wrapper = await mountMobileStudio()
    await flushPromises()

    await wrapper.get('[data-test="org-switch"]').trigger('click')
    await flushPromises()
    expect(document.querySelector('[data-test="org-sheet"]')).toBeTruthy()

    breakpointMocks.isMobile.value = false
    await nextTick()
    await flushPromises()
    expect(document.querySelector('[data-test="org-sheet"]')).toBeNull()
    wrapper.unmount()
  })

  it('keeps new/import entries available alongside switch', async () => {
    mocks.listAgents.mockResolvedValue([agentWithFiles()])
    const wrapper = await mountMobileStudio()
    await flushPromises()

    expect(wrapper.find('[data-test="org-switch"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('导入')
    expect(wrapper.text()).toContain('新建 Agent')
    wrapper.unmount()
  })
})
