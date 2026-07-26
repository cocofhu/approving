// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { createMemoryHistory, createRouter } from 'vue-router'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'
import type { PmLeaderBinding } from '@/lib/types'
import PmSettingsPanel from './PmSettingsPanel.vue'

const apiMocks = vi.hoisted(() => ({
  getPmLeader: vi.fn(),
  updatePmLeader: vi.fn(),
  listAgents: vi.fn(),
  getProjectChannel: vi.fn(),
  putProjectChannel: vi.fn(),
  listPmThreads: vi.fn(),
}))

const toastMocks = vi.hoisted(() => ({
  success: vi.fn(),
  error: vi.fn(),
}))

vi.mock('@/lib/api', async () => {
  const actual = await vi.importActual<typeof import('@/lib/api')>('@/lib/api')
  return {
    ...actual,
    api: {
      ...actual.api,
      getPmLeader: apiMocks.getPmLeader,
      updatePmLeader: apiMocks.updatePmLeader,
      listAgents: apiMocks.listAgents,
      getProjectChannel: apiMocks.getProjectChannel,
      putProjectChannel: apiMocks.putProjectChannel,
      listPmThreads: apiMocks.listPmThreads,
    },
  }
})

vi.mock('@/lib/useToast', () => ({
  useToast: () => toastMocks,
}))

const BINDING: PmLeaderBinding = {
  enabled: true,
  agentAvailable: true,
  agentConfigRef: 'agent-1',
  enabledMcps: ['pm-progress', 'pm-workflow-read', 'pm-workflow-write'],
  aclNote: 'note',
}

type ChannelFixture = {
  channel?: Record<string, unknown> | null
  secretsKeyConfigured?: boolean
}

async function mountPanel(
  binding: PmLeaderBinding = BINDING,
  channelFixture: ChannelFixture = { channel: null, secretsKeyConfigured: true },
) {
  apiMocks.getPmLeader.mockResolvedValue(binding)
  apiMocks.listAgents.mockResolvedValue([{ name: 'agent-1' }])
  apiMocks.getProjectChannel.mockResolvedValue({
    channel: channelFixture.channel ?? null,
    secretsKeyConfigured: channelFixture.secretsKeyConfigured ?? true,
  })
  apiMocks.updatePmLeader.mockImplementation(async (_id: string, body: Record<string, unknown>) => ({
    ...BINDING,
    ...body,
    agentAvailable: true,
    aclNote: BINDING.aclNote,
  }))

  const i18n = createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...common, ...pages } },
  })
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/', component: { template: '<div />' } }, { path: '/agents', component: { template: '<div />' } }],
  })
  await router.push('/')
  const w = mount(PmSettingsPanel, {
    props: { projectId: 'proj-1' },
    global: { plugins: [i18n, router] },
  })
  await flushPromises()
  return w
}

function cronDeliverSwitch(w: Awaited<ReturnType<typeof mountPanel>>) {
  return w.find('[data-testid="cron-deliver-enable"]')
}

async function setSwitch(
  el: ReturnType<Awaited<ReturnType<typeof mountPanel>>['find']>,
  on: boolean,
) {
  const checked = el.attributes('aria-checked') === 'true'
  if (checked !== on) await el.trigger('click')
}

function mcpSwitches(w: Awaited<ReturnType<typeof mountPanel>>) {
  return ['pm-progress', 'pm-workflow-read', 'pm-workflow-write'].map((id) =>
    w.get(`[aria-label="${id}"]`),
  )
}

describe('PmSettingsPanel enabledMcps', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    apiMocks.listPmThreads.mockResolvedValue({ items: [] })
  })

  it('loads enabledMcps and saves toggled selection', async () => {
    const w = await mountPanel({
      ...BINDING,
      enabledMcps: ['pm-progress'],
    })
    const mcpCodes = w.findAll('code').map((c) => c.text())
    expect(mcpCodes).toEqual(['pm-progress', 'pm-workflow-read', 'pm-workflow-write'])

    // PM MCP toggles are AppSwitch (role=switch); channel section has more switches after them.
    const boxes = mcpSwitches(w)
    expect(boxes).toHaveLength(3)
    expect(boxes[0].attributes('aria-checked')).toBe('true')
    expect(boxes[1].attributes('aria-checked')).toBe('false')
    expect(boxes[2].attributes('aria-checked')).toBe('false')

    await boxes[1].trigger('click')
    const saveBtn = w.find('[data-testid="pm-leader-save"]')
    expect(saveBtn).toBeTruthy()
    await saveBtn!.trigger('click')
    await flushPromises()

    const body = apiMocks.updatePmLeader.mock.calls[0][1] as { enabledMcps: string[] }
    expect(body.enabledMcps).toEqual(['pm-progress', 'pm-workflow-read'])
  })

  it('defaults all PM mcps when binding omits enabledMcps', async () => {
    const { enabledMcps: _drop, ...rest } = BINDING
    const w = await mountPanel(rest as PmLeaderBinding)
    const boxes = mcpSwitches(w)
    expect(boxes[0].attributes('aria-checked')).toBe('true')
    expect(boxes[1].attributes('aria-checked')).toBe('true')
    expect(boxes[2].attributes('aria-checked')).toBe('true')
  })

  it('can toggle off a PM mcp and persist only the remaining ids', async () => {
    const w = await mountPanel()
    const boxes = mcpSwitches(w)
    expect(boxes[0].attributes('aria-checked')).toBe('true')
    expect(boxes[1].attributes('aria-checked')).toBe('true')
    expect(boxes[2].attributes('aria-checked')).toBe('true')

    await boxes[0].trigger('click') // uncheck pm-progress
    const saveBtn = w.find('[data-testid="pm-leader-save"]')
    await saveBtn!.trigger('click')
    await flushPromises()

    const body = apiMocks.updatePmLeader.mock.calls[0][1] as { enabledMcps: string[] }
    expect(body.enabledMcps).toEqual(['pm-workflow-read', 'pm-workflow-write'])
  })

  it('preserves explicit empty enabledMcps from the API', async () => {
    const w = await mountPanel({
      ...BINDING,
      enabledMcps: [],
    })
    const boxes = mcpSwitches(w)
    expect(boxes[0].attributes('aria-checked')).toBe('false')
    expect(boxes[1].attributes('aria-checked')).toBe('false')
    expect(boxes[2].attributes('aria-checked')).toBe('false')

    const saveBtn = w.find('[data-testid="pm-leader-save"]')
    await saveBtn!.trigger('click')
    await flushPromises()

    const body = apiMocks.updatePmLeader.mock.calls[0][1] as { enabledMcps: string[] }
    expect(body.enabledMcps).toEqual([])
  })

  it('hides secrets_key hint when server reports key configured', async () => {
    const w = await mountPanel()
    expect(w.text()).not.toContain('需要在服务端配置加密密钥')
  })

  it('shows secrets_key hint only when server reports key missing', async () => {
    apiMocks.getPmLeader.mockResolvedValue(BINDING)
    apiMocks.listAgents.mockResolvedValue([{ name: 'agent-1' }])
    apiMocks.getProjectChannel.mockResolvedValue({ channel: null, secretsKeyConfigured: false })
    const i18n = createI18n({
      legacy: false,
      locale: 'zh-CN',
      messages: { 'zh-CN': { ...common, ...pages } },
    })
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [{ path: '/', component: { template: '<div />' } }],
    })
    await router.push('/')
    const w = mount(PmSettingsPanel, {
      props: { projectId: 'proj-1' },
      global: { plugins: [i18n, router] },
    })
    await flushPromises()
    expect(w.text()).toContain('需要在服务端配置加密密钥')
  })

  it('refreshes secrets_key hint after channel save', async () => {
    apiMocks.getPmLeader.mockResolvedValue(BINDING)
    apiMocks.listAgents.mockResolvedValue([{ name: 'agent-1' }])
    apiMocks.getProjectChannel
      .mockResolvedValueOnce({ channel: null, secretsKeyConfigured: false })
      .mockResolvedValue({ channel: null, secretsKeyConfigured: true })
    apiMocks.putProjectChannel.mockResolvedValue({
      id: 'chn-1',
      type: 'qq',
      name: 'bot',
      enabled: true,
      projectId: 'proj-1',
      appId: 'app',
      appSecretSet: true,
      turnTimeoutSeconds: 0,
      cronDeliver: false,
      createdAt: '',
      updatedAt: '',
    })
    const i18n = createI18n({
      legacy: false,
      locale: 'zh-CN',
      messages: { 'zh-CN': { ...common, ...pages } },
    })
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [{ path: '/', component: { template: '<div />' } }],
    })
    await router.push('/')
    const w = mount(PmSettingsPanel, {
      props: { projectId: 'proj-1' },
      global: { plugins: [i18n, router] },
    })
    await flushPromises()
    expect(w.text()).toContain('需要在服务端配置加密密钥')

    const saveBtns = w.findAll('button').filter((b) => b.text().includes('保存渠道配置'))
    expect(saveBtns.length).toBeGreaterThan(0)
    await saveBtns[0].trigger('click')
    await flushPromises()
    expect(w.text()).not.toContain('需要在服务端配置加密密钥')
    expect(apiMocks.getProjectChannel.mock.calls.length).toBeGreaterThanOrEqual(2)
  })
})

describe('PmSettingsPanel session capabilities', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    apiMocks.listPmThreads.mockResolvedValue({ items: [] })
  })

  it('defaults session caps off and saves with sandbox preserved', async () => {
    apiMocks.putProjectChannel.mockResolvedValue({
      id: 'chn-1',
      type: 'qq',
      name: 'bot',
      enabled: true,
      projectId: 'proj-1',
      appId: 'app',
      appSecretSet: true,
      turnTimeoutSeconds: 0,
      cronDeliver: false,
      config: { sandbox: true, allowMemoryWrite: true, allowSchedulerWrite: false },
      createdAt: '',
      updatedAt: '',
    })
    const w = await mountPanel(BINDING, {
      channel: {
        id: 'chn-1',
        type: 'qq',
        name: 'bot',
        enabled: true,
        projectId: 'proj-1',
        appId: 'app',
        appSecretSet: true,
        turnTimeoutSeconds: 0,
        cronDeliver: false,
        config: { sandbox: true },
        createdAt: '',
        updatedAt: '',
      },
    })

    expect(w.find('[data-testid="channel-session-caps"]').exists()).toBe(true)
    expect(w.text()).toContain('会话能力')
    expect(w.text()).toContain('风险提示')
    const mem = w.find('[data-testid="channel-allow-memory-write"]')
    const sch = w.find('[data-testid="channel-allow-scheduler-write"]')
    expect(mem.attributes('aria-checked')).toBe('false')
    expect(sch.attributes('aria-checked')).toBe('false')

    await setSwitch(mem, true)
    const saveBtns = w.findAll('button').filter((b) => b.text().includes('保存渠道配置'))
    await saveBtns[0].trigger('click')
    await flushPromises()

    const body = apiMocks.putProjectChannel.mock.calls[0][1] as {
      config: { sandbox: boolean; allowMemoryWrite: boolean; allowSchedulerWrite: boolean }
    }
    expect(body.config).toEqual({
      sandbox: true,
      allowMemoryWrite: true,
      allowSchedulerWrite: false,
    })
    expect(mem.attributes('aria-checked')).toBe('true')
    expect(sch.attributes('aria-checked')).toBe('false')
  })

  it('loads saved session caps and keeps sandbox when toggling scheduler write', async () => {
    apiMocks.putProjectChannel.mockResolvedValue({
      id: 'chn-1',
      type: 'qq',
      name: 'bot',
      enabled: true,
      projectId: 'proj-1',
      appId: 'app',
      appSecretSet: true,
      turnTimeoutSeconds: 0,
      cronDeliver: false,
      config: { sandbox: false, allowMemoryWrite: true, allowSchedulerWrite: true },
      createdAt: '',
      updatedAt: '',
    })
    const w = await mountPanel(BINDING, {
      channel: {
        id: 'chn-1',
        type: 'qq',
        name: 'bot',
        enabled: true,
        projectId: 'proj-1',
        appId: 'app',
        appSecretSet: true,
        turnTimeoutSeconds: 0,
        cronDeliver: false,
        config: { sandbox: false, allowMemoryWrite: true, allowSchedulerWrite: false },
        createdAt: '',
        updatedAt: '',
      },
    })

    const mem = w.find('[data-testid="channel-allow-memory-write"]')
    const sch = w.find('[data-testid="channel-allow-scheduler-write"]')
    expect(mem.attributes('aria-checked')).toBe('true')
    expect(sch.attributes('aria-checked')).toBe('false')

    await setSwitch(sch, true)
    const saveBtns = w.findAll('button').filter((b) => b.text().includes('保存渠道配置'))
    await saveBtns[0].trigger('click')
    await flushPromises()

    const body = apiMocks.putProjectChannel.mock.calls[0][1] as {
      config: { sandbox: boolean; allowMemoryWrite: boolean; allowSchedulerWrite: boolean }
    }
    expect(body.config).toEqual({
      sandbox: false,
      allowMemoryWrite: true,
      allowSchedulerWrite: true,
    })
  })
})

describe('PmSettingsPanel cron deliver target Combobox', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    apiMocks.listPmThreads.mockResolvedValue({ items: [] })
  })

  it('does not fetch recent targets until cron deliver is checked', async () => {
    await mountPanel()
    expect(apiMocks.listPmThreads).not.toHaveBeenCalled()
    expect(document.querySelector('[data-testid="cron-deliver-target-input"]')).toBeNull()
  })

  it('fetches once on check, reuses cache on reopen, and refetches after uncheck', async () => {
    const w = await mountPanel()
    apiMocks.listPmThreads.mockResolvedValue({
      items: [
        {
          id: 't1',
          projectId: 'proj-1',
          userId: 'qq:guild:111',
          title: '周会',
          updatedAt: '2026-01-02T00:00:00Z',
          createdAt: '2026-01-01T00:00:00Z',
        },
        {
          id: 't2',
          projectId: 'proj-1',
          userId: 'cron:job',
          title: 'cron',
          updatedAt: '2026-01-03T00:00:00Z',
          createdAt: '2026-01-01T00:00:00Z',
        },
      ],
    })
    await setSwitch(cronDeliverSwitch(w), true)
    await flushPromises()
    expect(apiMocks.listPmThreads).toHaveBeenCalledTimes(1)

    const input = w.find('[data-testid="cron-deliver-target-input"]')
    await input.trigger('focus')
    await flushPromises()
    expect(apiMocks.listPmThreads).toHaveBeenCalledTimes(1)
    expect(w.find('[data-testid="cron-deliver-target-listbox"]').text()).toContain('周会')
    expect(w.find('[data-testid="cron-deliver-target-listbox"]').text()).toContain('guild:111')
    expect(w.find('[data-testid="cron-deliver-target-listbox"]').text()).not.toContain('cron')

    await w.find('[data-testid="cron-deliver-target-toggle"]').trigger('click')
    await flushPromises()
    expect(w.find('[data-testid="cron-deliver-target-listbox"]').exists()).toBe(false)

    await w.find('[data-testid="cron-deliver-target-toggle"]').trigger('click')
    await flushPromises()
    expect(apiMocks.listPmThreads).toHaveBeenCalledTimes(1)

    await setSwitch(cronDeliverSwitch(w), false)
    await flushPromises()
    await setSwitch(cronDeliverSwitch(w), true)
    await flushPromises()
    expect(apiMocks.listPmThreads).toHaveBeenCalledTimes(2)
  })

  it('selects an option into the input without qq: prefix and keeps hand-edit path', async () => {
    apiMocks.putProjectChannel.mockResolvedValue({
      id: 'chn-1',
      type: 'qq',
      name: 'bot',
      enabled: true,
      projectId: 'proj-1',
      appId: 'app',
      appSecretSet: true,
      turnTimeoutSeconds: 0,
      cronDeliver: true,
      cronDeliverTarget: 'group:ABC',
      createdAt: '',
      updatedAt: '',
    })
    const w = await mountPanel()
    apiMocks.listPmThreads.mockResolvedValue({
      items: [
        {
          id: 't1',
          projectId: 'proj-1',
          userId: 'qq:group:ABC',
          title: '值班群',
          updatedAt: '2026-01-02T00:00:00Z',
          createdAt: '2026-01-01T00:00:00Z',
        },
      ],
    })
    await setSwitch(cronDeliverSwitch(w), true)
    await flushPromises()
    await w.find('[data-testid="cron-deliver-target-toggle"]').trigger('click')
    await flushPromises()
    const opt = w
      .findAll('[data-testid="cron-deliver-target-listbox"] [role="option"]')
      .find((b) => b.text().includes('值班群'))
    expect(opt).toBeTruthy()
    await opt!.trigger('click')
    await flushPromises()
    const input = w.find('[data-testid="cron-deliver-target-input"]')
    expect((input.element as HTMLInputElement).value).toBe('group:ABC')
    await input.setValue('group:ABC-edited')
    const saveBtns = w.findAll('button').filter((b) => b.text().includes('保存渠道配置'))
    await saveBtns[0].trigger('click')
    await flushPromises()
    const body = apiMocks.putProjectChannel.mock.calls[0][1] as { cronDeliverTarget: string }
    expect(body.cronDeliverTarget).toBe('group:ABC-edited')
  })

  it('keeps orphan saved value and shows empty-state when list is empty', async () => {
    const w = await mountPanel(BINDING, {
      channel: {
        id: 'chn-1',
        type: 'qq',
        name: 'bot',
        enabled: true,
        projectId: 'proj-1',
        appId: 'app',
        appSecretSet: true,
        turnTimeoutSeconds: 0,
        cronDeliver: true,
        cronDeliverTarget: 'guild:legacy-999',
        createdAt: '',
        updatedAt: '',
      },
      secretsKeyConfigured: true,
    })
    await flushPromises()
    const input = w.find('[data-testid="cron-deliver-target-input"]')
    expect((input.element as HTMLInputElement).value).toBe('guild:legacy-999')
    expect(apiMocks.listPmThreads).toHaveBeenCalledTimes(1)

    await w.find('[data-testid="cron-deliver-target-toggle"]').trigger('click')
    await flushPromises()
    expect(w.find('[data-testid="cron-deliver-target-listbox"]').text()).toContain(
      '暂无最近渠道会话',
    )
    expect((input.element as HTMLInputElement).value).toBe('guild:legacy-999')
  })

  it('degrades to empty list on fetch failure and still allows save', async () => {
    apiMocks.putProjectChannel.mockResolvedValue({
      id: 'chn-1',
      type: 'qq',
      name: 'bot',
      enabled: true,
      projectId: 'proj-1',
      appId: 'app',
      appSecretSet: true,
      turnTimeoutSeconds: 0,
      cronDeliver: true,
      cronDeliverTarget: 'guild:hand',
      createdAt: '',
      updatedAt: '',
    })
    const w = await mountPanel()
    apiMocks.listPmThreads.mockRejectedValue(new Error('network down'))
    await setSwitch(cronDeliverSwitch(w), true)
    await flushPromises()
    expect(toastMocks.error).toHaveBeenCalled()
    const toastMsg = String(toastMocks.error.mock.calls[0]?.[0] ?? '')
    expect(toastMsg).toContain('最近目标加载失败')
    expect(toastMsg).toContain('network down')
    await w.find('[data-testid="cron-deliver-target-input"]').setValue('guild:hand')
    const saveBtns = w.findAll('button').filter((b) => b.text().includes('保存渠道配置'))
    await saveBtns[0].trigger('click')
    await flushPromises()
    expect(apiMocks.putProjectChannel).toHaveBeenCalled()
    const body = apiMocks.putProjectChannel.mock.calls[0][1] as { cronDeliverTarget: string }
    expect(body.cronDeliverTarget).toBe('guild:hand')
  })

  it('exposes Combobox ARIA attributes on input and toggle', async () => {
    const w = await mountPanel()
    apiMocks.listPmThreads.mockResolvedValue({
      items: [
        {
          id: 't1',
          projectId: 'proj-1',
          userId: 'qq:guild:111',
          title: '周会',
          updatedAt: '2026-01-02T00:00:00Z',
          createdAt: '2026-01-01T00:00:00Z',
        },
      ],
    })
    await setSwitch(cronDeliverSwitch(w), true)
    await flushPromises()
    const input = w.find('[data-testid="cron-deliver-target-input"]')
    const toggle = w.find('[data-testid="cron-deliver-target-toggle"]')
    expect(input.attributes('aria-autocomplete')).toBe('list')
    expect(input.attributes('aria-controls')).toBe('ch-cron-target-listbox')
    expect(input.attributes('aria-expanded')).toBe('false')
    expect(toggle.attributes('aria-controls')).toBe('ch-cron-target-listbox')
    expect(toggle.attributes('aria-expanded')).toBe('false')
    expect(toggle.attributes('aria-label')).toContain('最近目标')
    await toggle.trigger('click')
    await flushPromises()
    expect(input.attributes('aria-expanded')).toBe('true')
    expect(toggle.attributes('aria-expanded')).toBe('true')
    expect(input.attributes('aria-activedescendant')).toBeUndefined()
    await input.trigger('keydown', { key: 'ArrowDown' })
    expect(input.attributes('aria-activedescendant')).toBe('ch-cron-target-listbox-opt-0')
    expect(w.find('#ch-cron-target-listbox-opt-0').exists()).toBe(true)
  })
})

describe('PmSettingsPanel gate-auto config', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    apiMocks.listPmThreads.mockResolvedValue({ items: [] })
  })

  it('loads and saves gateAutoVar + gateAutoPrompt as text fields', async () => {
    const binding: PmLeaderBinding = {
      ...BINDING,
      gateAutoVar: 'pm_auto_gate',
      gateAutoPrompt: '优先批准低风险',
    }
    const w = await mountPanel(binding)
    const varInput = w.get('[data-testid="pm-gate-auto-var"]')
    const promptInput = w.get('[data-testid="pm-gate-auto-prompt"]')
    expect((varInput.element as HTMLInputElement).value).toBe('pm_auto_gate')
    expect((promptInput.element as HTMLTextAreaElement).value).toBe('优先批准低风险')

    await varInput.setValue('other_switch')
    await promptInput.setValue('')
    await w.get('[data-testid="pm-leader-save"]').trigger('click')
    await flushPromises()

    const updateCalls = apiMocks.updatePmLeader.mock.calls
    const body = updateCalls[updateCalls.length - 1]?.[1] as {
      gateAutoVar: string
      gateAutoPrompt: string
    }
    expect(body.gateAutoVar).toBe('other_switch')
    expect(body.gateAutoPrompt).toBe('')
  })
})
