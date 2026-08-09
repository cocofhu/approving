// @vitest-environment happy-dom
import { defineComponent, h, nextTick } from 'vue'
import { createI18n } from 'vue-i18n'
import { createMemoryHistory, createRouter } from 'vue-router'
import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'
import enCommon from '@/locales/en/common.json'
import enPages from '@/locales/en/pages.json'
import type { Agent } from '@/lib/api'

const mocks = vi.hoisted(() => ({
  listAgents: vi.fn(),
  listProjects: vi.fn(),
  saveAgent: vi.fn(),
  patchAgentProject: vi.fn(),
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
      patchAgentProject: mocks.patchAgentProject,
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

const mountedWrappers: Array<{ unmount: () => void }> = []

function trackMount<T extends { unmount: () => void }>(wrapper: T): T {
  mountedWrappers.push(wrapper)
  return wrapper
}

function removeStudioToasts() {
  document.querySelectorAll('[data-test="studio-toast"]').forEach((el) => el.remove())
}

async function mountStudio(query: Record<string, string> = {}) {
  const i18n = createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...common, ...pages } },
  })
  const router = await createStudioRouter(query)
  return trackMount(
    mount(AgentStudioView, {
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
    }),
  )
}

beforeEach(() => {
  removeStudioToasts()
  vi.clearAllMocks()
  breakpointMocks.isMobile.value = false
  mocks.listProjects.mockResolvedValue([{ id: 'proj-default', name: 'Default' }])
  mocks.saveAgent.mockImplementation(async (payload: Agent) => payload)
  mocks.patchAgentProject.mockImplementation(async (name: string, projectId: string) => ({
    status: 'saved',
    projectId,
  }))
  mocks.getAgentsOrg.mockResolvedValue({ revision: 0, groups: [], agents: {} })
  mocks.saveAgentsOrg.mockImplementation(async (org: { revision?: number }) => ({
    revision: (org.revision || 0) + 1,
    groups: [],
    agents: {},
    ...org,
  }))
})

afterEach(() => {
  while (mountedWrappers.length) {
    mountedWrappers.pop()?.unmount()
  }
  removeStudioToasts()
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
    return trackMount(
      mount(AgentStudioView, {
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
      }),
    )
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

  it('management Rename opens dialog with cascade hint; cancel does not rename', async () => {
    mocks.listAgents.mockResolvedValue([agent('public')])
    const wrapper = await mountRenameStudio()
    await flushPromises()

    await wrapper.get('[data-test="manage"]').trigger('click')
    await flushPromises()
    await wrapper.findAll('button').find((b) => b.text() === 'Rename')!.trigger('click')
    await flushPromises()

    expect(wrapper.text()).not.toContain('工作流引用不会自动更新')
    expect(wrapper.text()).toContain('将同步更新目录、项目管理、组织关系，以及工作流中的 skill_profile 引用')
    await wrapper.findAll('button').find((b) => b.text() === '取消')!.trigger('click')
    await flushPromises()
    expect(mocks.renameAgent).not.toHaveBeenCalled()
  })

  it('rename success toast shows workflow count when N>0 and omits count when N=0', async () => {
    mocks.listAgents.mockResolvedValue([agent('public')])
    mocks.renameAgent
      .mockResolvedValueOnce({ ...agent('public'), name: 'legacy2', updatedWorkflowCount: 3 })
      .mockResolvedValueOnce({ ...agent('public'), name: 'legacy3', updatedWorkflowCount: 0 })

    const wrapper = await mountRenameStudio()
    await flushPromises()

    const renameInput = () =>
      wrapper.findAll('input').find((el) => {
        const node = el.element as HTMLInputElement
        return node.type !== 'file' && (node.value === 'legacy' || node.value === 'legacy2' || node.value === 'legacy3')
      })!

    await wrapper.get('[data-test="manage"]').trigger('click')
    await flushPromises()
    await wrapper.findAll('button').find((b) => b.text() === 'Rename')!.trigger('click')
    await flushPromises()
    await renameInput().setValue('legacy2')
    await wrapper.findAll('button').find((b) => b.text() === '确定')!.trigger('click')
    await flushPromises()
    expect(mocks.renameAgent).toHaveBeenCalledWith('legacy', 'legacy2')
    expect(document.body.textContent || '').toContain('改名成功，已更新 3 个工作流')
    expect(document.body.textContent || '').not.toContain('已更新 0 个工作流')

    await wrapper.get('[data-test="manage"]').trigger('click')
    await flushPromises()
    await wrapper.findAll('button').find((b) => b.text() === 'Rename')!.trigger('click')
    await flushPromises()
    await renameInput().setValue('legacy3')
    await wrapper.findAll('button').find((b) => b.text() === '确定')!.trigger('click')
    await flushPromises()
    expect(mocks.renameAgent).toHaveBeenCalledWith('legacy2', 'legacy3')
    const body = document.body.textContent || ''
    expect(body).toContain('改名成功')
    expect(body).not.toContain('已更新 0 个工作流')
    expect(body).not.toContain('已更新 3 个工作流')
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
    return trackMount(
      mount(AgentStudioView, {
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
      }),
    )
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

  it('mounts data panel on mobile instead of desktop-only tip', async () => {
    mocks.listAgents.mockResolvedValue([agentWithFiles()])
    const wrapper = await mountMobileStudio()
    await flushPromises()

    await wrapper.findAll('button').find((b) => b.text() === '数据')!.trigger('click')
    await flushPromises()

    expect(wrapper.text()).not.toContain('请在桌面端完成')
    expect(wrapper.find('agent-data-panel-stub').exists()).toBe(true)
  })

  it('keeps mcp/env/prompts desktop-only while data is allowed', async () => {
    mocks.listAgents.mockResolvedValue([agentWithFiles()])
    const wrapper = await mountMobileStudio()
    await flushPromises()

    for (const label of ['MCP', '环境变量', '提示词']) {
      const btn = wrapper.findAll('button').find((b) => b.text().startsWith(label))
      expect(btn).toBeTruthy()
      await btn!.trigger('click')
      await flushPromises()
      expect(wrapper.text()).toContain('请在桌面端完成')
      expect(wrapper.find('agent-data-panel-stub').exists()).toBe(false)
    }
  })

  it('deep-links to data sub-tab on mobile without desktop-only tip', async () => {
    mocks.listAgents.mockResolvedValue([{ ...agentWithFiles(), name: 'alpha' }])
    const i18n = createI18n({
      legacy: false,
      locale: 'zh-CN',
      messages: { 'zh-CN': { ...common, ...pages } },
    })
    const router = await createStudioRouter({ agent: 'alpha', tab: 'data', sub: 'jobs' })
    const wrapper = trackMount(
      mount(AgentStudioView, {
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
      }),
    )
    await flushPromises()

    expect(wrapper.text()).not.toContain('请在桌面端完成')
    const panel = wrapper.find('agent-data-panel-stub')
    expect(panel.exists()).toBe(true)
    expect(panel.attributes('sub-tab') || panel.attributes('subtab')).toBeTruthy()
    wrapper.unmount()
  })

  it('unbound agent shows desktop-bind empty state without goBind on mobile', async () => {
    mocks.listAgents.mockResolvedValue([
      {
        ...agentWithFiles(),
        projectId: '',
      },
    ])
    const wrapper = await mountMobileStudio()
    await flushPromises()

    await wrapper.findAll('button').find((b) => b.text() === '数据')!.trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('尚未绑定主项目')
    expect(wrapper.text()).toContain('请在桌面端')
    expect(wrapper.text()).not.toContain('去绑定主项目')
    expect(wrapper.find('agent-data-panel-stub').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('请在桌面端完成')
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
    expect(wrapper.text()).not.toContain('配置可复用的 Agent')
    expect(wrapper.text()).not.toContain('复制进沙箱')
    wrapper.unmount()
  })
})

describe('AgentStudio copy removal (subtitle + toolbar)', () => {
  const subtitleZh = ['配置可复用的 Agent', '复制进沙箱', '/root/.cursor']
  const subtitleEn = ['Reusable agents', 'copied to', '/root/.cursor']
  const demoShell = [
    '可复用 Agent 配置 · 组织与 skill_profile 引用',
    '已拖入未分组',
    'clearToast',
  ]

  function toolbar(wrapper: Awaited<ReturnType<typeof mountStudio>>) {
    return wrapper.find('.mb-5.flex.shrink-0')
  }

  async function mountStudioEn() {
    const i18n = createI18n({
      legacy: false,
      locale: 'en',
      messages: { en: { ...enCommon, ...enPages } },
    })
    const router = await createStudioRouter()
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

  it('hides zh subtitle, right-aligns desktop import/new, and does not add a page title', async () => {
    mocks.listAgents.mockResolvedValue([agent('public')])
    const wrapper = await mountStudio()
    await flushPromises()

    const text = wrapper.text()
    for (const s of subtitleZh) expect(text).not.toContain(s)
    for (const s of demoShell) expect(text).not.toContain(s)
    expect(text).not.toMatch(/改前|改后/)
    expect(wrapper.findAll('h1,h2').some((el) => /Agent\s*(管理|Studio)/i.test(el.text()))).toBe(false)

    const bar = toolbar(wrapper)
    expect(bar.exists()).toBe(true)
    expect(bar.classes()).toContain('justify-end')
    expect(bar.classes()).not.toContain('justify-between')
    expect(bar.text()).toContain('导入')
    expect(bar.text()).toContain('新建 Agent')
    wrapper.unmount()
  })

  it('hides en subtitle and keeps Import / New agent', async () => {
    mocks.listAgents.mockResolvedValue([agent('public')])
    const wrapper = await mountStudioEn()
    await flushPromises()

    const text = wrapper.text()
    for (const s of subtitleEn) expect(text).not.toContain(s)
    expect(text).toContain('Import')
    expect(text).toContain('New agent')
    wrapper.unmount()
  })
})

describe('AgentStudio org toast and remaining hints', () => {
  const ModalStub = defineComponent({
    props: { open: Boolean, title: String },
    emits: ['close'],
    template: '<div v-if="open" data-test="modal"><h2>{{ title }}</h2><slot /><slot name="footer" /></div>',
  })
  const SidebarStub = defineComponent({
    emits: ['remove-from-group', 'move-agent', 'open-manage', 'select-agent'],
    template:
      '<div data-test="sidebar">' +
      '<button data-test="remove-from-group" @click="$emit(\'remove-from-group\', \'legacy\', \'g1\')">remove</button>' +
      '<button data-test="move-ungrouped" @click="$emit(\'move-agent\', \'legacy\', \'g1\', \'\')">ungroup</button>' +
      '<button data-test="manage" @click="$emit(\'open-manage\')">manage</button>' +
      '</div>',
  })

  async function mountHintStudio() {
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

  beforeEach(() => {
    mocks.listAgents.mockResolvedValue([agent('public')])
    mocks.getAgentsOrg.mockResolvedValue({
      revision: 1,
      groups: [{ id: 'g1', name: 'Dev' }],
      agents: { legacy: { groupIds: ['g1'] } },
    })
    mocks.saveAgentsOrg.mockImplementation(async (org: { revision?: number; groups?: unknown; agents?: unknown }) => ({
      revision: (org.revision || 0) + 1,
      groups: org.groups || [],
      agents: org.agents || {},
    }))
  })

  it('toasts on 移出本组 but not when dragging to ungrouped', async () => {
    const wrapper = await mountHintStudio()
    await flushPromises()

    await wrapper.get('[data-test="move-ungrouped"]').trigger('click')
    await flushPromises()
    expect(document.body.textContent || '').not.toContain('已拖入未分组')
    expect(document.body.textContent || '').not.toContain('已移出本组并立即保存')
    expect(mocks.saveAgentsOrg).toHaveBeenCalled()

    await wrapper.get('[data-test="remove-from-group"]').trigger('click')
    await flushPromises()
    expect(document.body.textContent || '').toContain('已移出本组并立即保存 · 实体仍在')
    expect(document.body.textContent || '').not.toContain('已拖入未分组')
    wrapper.unmount()
  })

  it('keeps MCP hint, platform-rules subtitle, manageIntro, and data/meta tabs', async () => {
    const wrapper = await mountHintStudio()
    await flushPromises()

    expect(wrapper.text()).toContain('数据')
    expect(wrapper.text()).toContain('元信息')
    expect(wrapper.text()).not.toContain('可复用 Agent 配置 · 组织与 skill_profile 引用')
    expect(wrapper.text()).not.toMatch(/改前|改后/)

    await wrapper.findAll('button').find((item) => item.text().startsWith('MCP'))!.trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('整份 mcp.json 由你配置')
    expect(wrapper.text()).toContain('/root/.codebuddy/mcp.json')

    await wrapper.findAll('button').find((item) => item.text() === '平台规则')!.trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('profiles/legacy/platform-rules/')

    await wrapper.get('[data-test="manage"]').trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('列出全部 Agent。此处「Rename」修改身份名称')
    wrapper.unmount()
  })
})

describe('AgentStudio env credential help', () => {
  const HelpAppModalStub = defineComponent({
    props: { open: Boolean, title: String, width: Number },
    emits: ['close'],
    template:
      '<div v-if="open" data-test="env-help-modal" :data-width="width">' +
      '<h2>{{ title }}</h2><div data-test="env-help-scroll"><slot /></div><slot name="footer" />' +
      '</div>',
  })
  const GitGuideHelpStub = defineComponent({
    emits: ['help', 'update:credentialType'],
    template:
      '<div data-test="git-guide"><button type="button" data-test="git-help-emit" @click="$emit(\'help\', \'git\')">帮助</button></div>',
  })

  async function mountStudioWithHelp() {
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
          AppModal: HelpAppModalStub,
          CodeEditor: CodeEditorStub,
          MarkdownSplitEditor: true,
          ExplorerContextMenu: true,
          AgentChatTester: true,
          AgentGitGuide: GitGuideHelpStub,
          AgentCreateWizard: true,
          AgentOrgSidebar: true,
          AgentDataPanel: true,
        },
      },
    })
  }

  async function openEnvTab(wrapper: Awaited<ReturnType<typeof mountStudioWithHelp>>) {
    await wrapper.findAll('button').find((item) => item.text().startsWith('环境变量'))!.trigger('click')
    await flushPromises()
  }

  it('moves long copy into one help modal and jumps inject / git / acp', async () => {
    mocks.listAgents.mockResolvedValue([agent('public')])
    const wrapper = await mountStudioWithHelp()
    await flushPromises()
    await openEnvTab(wrapper)

    expect(wrapper.get('[data-test="env-help-inject"]').text()).toBe('帮助')
    expect(wrapper.get('[data-test="env-help-acp"]').text()).toBe('帮助')
    expect(wrapper.text()).toContain('APPROVING_CODEBUDDY_API_KEY')
    expect(wrapper.text()).not.toContain('环境变量会注入该 Agent 的沙箱容器')
    expect(wrapper.text()).not.toContain('请在下方添加对应 Key')
    expect(wrapper.text()).not.toContain('保存后写入 agent.json')

    await wrapper.get('[data-test="env-help-inject"]').trigger('click')
    await flushPromises()
    expect(wrapper.findAll('[data-test="env-help-modal"]')).toHaveLength(1)
    expect(wrapper.get('[data-test="env-help-modal"]').attributes('data-width')).toBe('640')
    expect(wrapper.get('[data-test="env-help-modal"]').text()).toContain('环境变量与凭据')
    expect(wrapper.get('[data-help-chip="inject"]').classes().join(' ')).toContain('border-accent')
    expect(wrapper.get('[data-test="env-help-modal"]').text()).toContain('环境变量会注入该 Agent 的沙箱容器')

    await wrapper.get('[data-test="git-help-emit"]').trigger('click')
    await flushPromises()
    expect(wrapper.findAll('[data-test="env-help-modal"]')).toHaveLength(1)
    expect(wrapper.get('[data-help-chip="git"]').classes().join(' ')).toContain('border-accent')
    expect(wrapper.get('[data-test="env-help-modal"]').text()).toContain('不会验证变量引用的实际值')

    await wrapper.get('[data-test="env-help-acp"]').trigger('click')
    await flushPromises()
    expect(wrapper.findAll('[data-test="env-help-modal"]')).toHaveLength(1)
    expect(wrapper.get('[data-help-chip="acp"]').classes().join(' ')).toContain('border-accent')
    expect(wrapper.get('[data-test="env-help-modal"]').text()).toContain('CodeBuddy ACP 鉴权')
    wrapper.unmount()
  })

  it('keeps unsaved env draft after closing help', async () => {
    mocks.listAgents.mockResolvedValue([agent('public')])
    const wrapper = await mountStudioWithHelp()
    await flushPromises()
    await openEnvTab(wrapper)

    await wrapper.findAll('button').find((item) => item.text() === '添加环境变量')!.trigger('click')
    await flushPromises()
    const keyInput = wrapper.findAll('input').find((el) => {
      const node = el.element as HTMLInputElement
      return node.placeholder === 'KEY' && !node.readOnly && node.value === ''
    })!
    await keyInput.setValue('MY_DRAFT_KEY')
    expect((keyInput.element as HTMLInputElement).value).toBe('MY_DRAFT_KEY')

    await wrapper.get('[data-test="env-help-inject"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-test="env-help-modal"]').exists()).toBe(true)
    await wrapper.get('[data-test="env-help-got-it"]').trigger('click')
    await flushPromises()

    expect(wrapper.find('[data-test="env-help-modal"]').exists()).toBe(false)
    const kept = wrapper.findAll('input').find((el) => (el.element as HTMLInputElement).value === 'MY_DRAFT_KEY')
    expect(kept).toBeTruthy()
    wrapper.unmount()
  })
})

describe('AgentStudio group assign project', () => {
  const ModalStub = defineComponent({
    props: { open: Boolean, title: String },
    emits: ['close'],
    template: '<div v-if="open" data-test="modal"><h2>{{ title }}</h2><slot /><slot name="footer" /></div>',
  })
  const SidebarStub = defineComponent({
    props: { org: Object, agents: Array, projects: Array },
    emits: ['assign-project', 'select-agent'],
    template: '<div data-test="sidebar"><button data-test="assign-g1" @click="$emit(\'assign-project\', \'g1\')">assign</button></div>',
  })

  function studioAgent(name: string, projectId: string): Agent {
    return {
      name,
      projectId,
      acpBackend: 'cursor',
      files: [{ path: 'AGENTS.md', content: `# ${name}\n` }],
      mcp: [],
      env: {},
      layout: { configRoot: '/root/.cursor', workspaceDir: '/root/workspace' },
    }
  }

  async function mountAssignStudio() {
    const i18n = createI18n({
      legacy: false,
      locale: 'zh-CN',
      messages: { 'zh-CN': { ...common, ...pages } },
    })
    const router = await createStudioRouter()
    return trackMount(
      mount(AgentStudioView, {
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
      }),
    )
  }

  beforeEach(() => {
    breakpointMocks.isMobile.value = false
    mocks.listProjects.mockResolvedValue([
      { id: 'github', name: 'GitHub' },
      { id: 'figma', name: 'Figma' },
    ])
    mocks.getAgentsOrg.mockResolvedValue({
      revision: 1,
      groups: [{ id: 'g1', name: '设计组' }],
      agents: {
        alice: { groupIds: ['g1'] },
        bob: { groupIds: ['g1'] },
      },
    })
  })

  it('集合内非 dirty 成功后同步草稿 projectId，不弹草稿冲突', async () => {
    mocks.listAgents.mockResolvedValue([
      studioAgent('alice', 'figma'),
      studioAgent('bob', 'github'),
    ])
    mocks.listAgents
      .mockResolvedValueOnce([
        studioAgent('alice', 'figma'),
        studioAgent('bob', 'github'),
      ])
      .mockResolvedValueOnce([
        studioAgent('alice', 'github'),
        studioAgent('bob', 'github'),
      ])
    const wrapper = await mountAssignStudio()
    await flushPromises()

    await wrapper.get('[data-test="assign-g1"]').trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('指定项目 · 设计组')
    expect(wrapper.text()).toContain('alice')
    expect(wrapper.text()).toContain('bob')

    await wrapper.get('[data-test="org-assign-submit"]').trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('确认切换主项目')
    expect(wrapper.text()).toContain('立即生效')
    expect(wrapper.text()).not.toContain('草稿主项目冲突')

    await wrapper.get('[data-test="org-assign-cover-ok"]').trigger('click')
    await flushPromises()

    expect(mocks.patchAgentProject).toHaveBeenCalledTimes(2)
    expect(mocks.patchAgentProject).toHaveBeenNthCalledWith(1, 'alice', 'github')
    expect(mocks.patchAgentProject).toHaveBeenNthCalledWith(2, 'bob', 'github')
    expect(document.querySelector('[data-test="studio-toast"]')?.textContent).toContain('已绑定到 GitHub')
    wrapper.unmount()
  })

  it('dirty 保留草稿则整次不写 PATCH', async () => {
    mocks.listAgents.mockResolvedValue([
      studioAgent('alice', 'figma'),
      studioAgent('bob', 'github'),
    ])
    const wrapper = await mountAssignStudio()
    await flushPromises()

    await wrapper.findAll('button').find((b) => b.text() === '元信息')!.trigger('click')
    await flushPromises()
    const select = wrapper.get('[data-test="agent-project-select"]')
    await select.setValue('github')
    await flushPromises()
    // confirm single-agent draft switch modal if it appears
    const confirmSwitch = wrapper.findAll('button').find((b) => b.text().includes('切换到'))
    if (confirmSwitch) {
      await confirmSwitch.trigger('click')
      await flushPromises()
    }

    await wrapper.get('[data-test="assign-g1"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-test="org-assign-submit"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-test="org-assign-cover-ok"]').trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('草稿主项目冲突')
    await wrapper.get('[data-test="org-assign-draft-keep"]').trigger('click')
    await flushPromises()

    expect(mocks.patchAgentProject).not.toHaveBeenCalled()
    expect(document.querySelector('[data-test="studio-toast"]')?.textContent).toContain('组级指定未执行')
    wrapper.unmount()
  })

  it('部分失败 err-box 列出原因，当前 Agent fail 不同步草稿', async () => {
    mocks.listAgents.mockResolvedValue([
      studioAgent('alice', 'figma'),
      studioAgent('bob', 'github'),
    ])
    mocks.patchAgentProject.mockImplementation(async (name: string, projectId: string) => {
      if (name === 'alice') throw Object.assign(new Error('绑定不被允许（含项目级 MCP 约束）'), { status: 400 })
      return { status: 'saved', projectId }
    })
    mocks.listAgents
      .mockResolvedValueOnce([
        studioAgent('alice', 'figma'),
        studioAgent('bob', 'github'),
      ])
      .mockResolvedValueOnce([
        studioAgent('alice', 'figma'),
        studioAgent('bob', 'github'),
      ])

    const wrapper = await mountAssignStudio()
    await flushPromises()
    await wrapper.get('[data-test="assign-g1"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-test="org-assign-submit"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-test="org-assign-cover-ok"]').trigger('click')
    await flushPromises()

    const failBox = wrapper.get('[data-test="org-assign-fail"]')
    expect(failBox.text()).toContain('部分成功')
    expect(failBox.text()).toContain('alice')
    expect(failBox.text()).toContain('绑定不被允许')
    expect(failBox.text()).toContain('括号按完成后实际绑定刷新')
    expect(mocks.patchAgentProject).toHaveBeenCalledTimes(2)
    wrapper.unmount()
  })
})
