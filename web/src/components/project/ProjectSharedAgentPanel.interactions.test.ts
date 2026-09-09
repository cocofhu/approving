// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'

const mocks = vi.hoisted(() => ({
  getConfig: vi.fn(), listAgents: vi.fn(), putConfig: vi.fn(), createTest: vi.fn(),
  success: vi.fn(), error: vi.fn(),
}))
vi.mock('@/lib/api/api', async () => {
  const actual = await vi.importActual<typeof import('@/lib/api/api')>('@/lib/api/api')
  return { ...actual, api: { ...actual.api,
    getProjectSharedAgentConfig: mocks.getConfig, listAgents: mocks.listAgents,
    putProjectSharedAgentConfig: mocks.putConfig, createProjectSharedAgentTest: mocks.createTest,
  } }
})
vi.mock('@/lib/composables/useToast', () => ({ useToast: () => mocks }))
import ProjectSharedAgentPanel from './ProjectSharedAgentPanel.vue'

const cfg = {
  acpBackend: 'cursor', defaultProjectId: 'p1', gitCredentialType: '',
  gitSshKnownHosts: '', gitSshPrivateKey: '', files: [], mcp: [], env: {},
  layout: { configRoot: '~/.cursor', workspaceDir: '/workspace' }, prompts: {},
}
const FilesStub = { name: 'AgentFilesPanel', props: ['draft', 'save'], methods: { openPathOrCreate: vi.fn() }, template: '<div data-testid="files"/>' }
function mountPanel(projectId = 'p1') {
  const i18n = createI18n({ legacy: false, locale: 'zh-CN', messages: { 'zh-CN': { ...common, ...pages } } })
  return mount(ProjectSharedAgentPanel, {
    props: { projectId },
    global: { plugins: [i18n], stubs: {
      Icon: true, AppButton: { template: '<button v-bind="$attrs"><slot/></button>' },
      AgentFilesPanel: FilesStub, AgentMcpPanel: { template: '<div data-testid="mcp"/>' },
      AgentEnvPanel: { emits: ['open-settings-file'], template: '<button data-testid="env-settings" @click="$emit(\'open-settings-file\')"/>' },
      AgentPromptsPanel: { template: '<div data-testid="prompts"/>' },
      AgentChatTester: { props: ['profile', 'createTest'], template: '<div data-testid="tester">{{profile}}</div>' },
      ProjectAgentSelect: { props: ['modelValue'], template: '<div data-testid="picker">{{modelValue}}</div>' },
    } },
  })
}

describe('ProjectSharedAgentPanel interactions', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.getConfig.mockResolvedValue(cfg)
    mocks.listAgents.mockResolvedValue([{ name: 'a1', projectId: 'p1' }, { name: 'other', projectId: 'p2' }])
    mocks.putConfig.mockImplementation(async (_: string, body: any) => ({ ...cfg, ...body }))
    mocks.createTest.mockResolvedValue({ id: 'sandbox' })
  })

  it('edits metadata, switches backend/region and saves normalized payload', async () => {
    const w = mountPanel()
    await flushPromises()
    const vm = w.vm as any
    await w.get('[data-testid="shared-agent-subtab-meta"]').trigger('click')
    await flushPromises()
    expect(vm.derivedPaths[0].path).toContain('mcp.json')
    vm.selectAcpBackend('claude_code')
    await flushPromises()
    expect(vm.draft.acpBackend).toBe('claude_code')
    if (vm.metaRegionOptions.length) vm.selectRegion(vm.metaRegionOptions[0].id)
    vm.draft.projectId = 'p-new'
    expect(vm.dirty).toBe(true)
    expect(await vm.save()).toBe(true)
    expect(mocks.putConfig).toHaveBeenCalledWith('p1', expect.objectContaining({ defaultProjectId: 'p-new', files: [] }))
    expect(mocks.success).toHaveBeenCalled()
    expect(vm.dirty).toBe(false)
    w.unmount()
  })

  it('surfaces load/save failures and retries/discards', async () => {
    mocks.getConfig.mockRejectedValueOnce(new Error('load failed'))
    mocks.listAgents.mockRejectedValueOnce(new Error('agents failed'))
    const w = mountPanel()
    await flushPromises()
    const vm = w.vm as any
    expect(w.text()).toContain('load failed')
    mocks.getConfig.mockResolvedValue(cfg)
    await vm.load()
    vm.draft.projectId = 'dirty'
    mocks.putConfig.mockRejectedValueOnce(new Error('save failed'))
    expect(await vm.save()).toBe(false)
    expect(mocks.error).toHaveBeenCalledWith('save failed')
    vm.discard()
    await flushPromises()
    expect(vm.draft.projectId).toBe('p1')
    vm.draft = null
    expect(await vm.save()).toBe(false)
    vm.discard()
    vm.selectAcpBackend('cursor')
    vm.selectRegion('global')
    vm.openSettingsInFiles()
    w.unmount()
  })

  it('opens settings via env, synchronizes agent selection and creates project tests', async () => {
    const w = mountPanel()
    await flushPromises()
    const vm = w.vm as any
    await w.get('[data-testid="shared-agent-subtab-env"]').trigger('click')
    await w.get('[data-testid="env-settings"]').trigger('click')
    await flushPromises()
    expect(vm.subTab).toBe('files')
    await w.get('[data-testid="shared-agent-subtab-test"]').trigger('click')
    expect(vm.testAgentName).toBe('a1')
    const res = await vm.createProjectContextTest('a1', { repos: [{ name: 'r' }], repoUrl: 'https://x' })
    expect(res.id).toBe('sandbox')
    expect(mocks.createTest).toHaveBeenCalledWith('p1', { agentName: 'a1', repos: [{ name: 'r' }], repoUrl: 'https://x' })
    vm.agents = []
    vm.syncTestAgentSelection()
    expect(vm.testAgentName).toBe('')
    w.unmount()
  })

  it('reloads when project id changes and resets the selected tab', async () => {
    const w = mountPanel()
    await flushPromises()
    ;(w.vm as any).subTab = 'meta'
    await w.setProps({ projectId: 'p2' })
    await flushPromises()
    expect(mocks.getConfig).toHaveBeenCalledWith('p2')
    expect((w.vm as any).subTab).toBe('files')
    w.unmount()
  })
})
