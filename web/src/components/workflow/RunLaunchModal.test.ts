// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'

function findStartButton(wrapper: ReturnType<typeof mount>) {
  return wrapper.findAll('button').find((b) => b.text().includes('开始运行') || b.text().match(/Start/i))
}

const apiMocks = vi.hoisted(() => ({
  startRun: vi.fn(),
  listRepos: vi.fn(),
  listProjectRunTags: vi.fn(),
}))

vi.mock('@/lib/api/api', async () => {
  const actual = await vi.importActual<typeof import('@/lib/api/api')>('@/lib/api/api')
  return {
    ...actual,
    api: {
      ...actual.api,
      startRun: apiMocks.startRun,
      listRepos: apiMocks.listRepos,
      listProjectRunTags: apiMocks.listProjectRunTags,
    },
  }
})

import RunLaunchModal from './RunLaunchModal.vue'

function mountModal(open = true, extraProps: Record<string, unknown> = {}) {
  const i18n = createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...common, ...pages } },
  })
  return mount(RunLaunchModal, {
    props: {
      open,
      workflowId: 'wf-1',
      workflowName: '测试流水线',
      fields: [{ key: 'topic', desc: '主题', required: true }],
      runInputs: { topic: 'hello' },
      runImages: {},
      ...extraProps,
    },
    global: {
      plugins: [i18n],
      stubs: {
        Icon: true,
        AppButton: { template: '<button v-bind="$attrs"><slot /></button>' },
        AppModal: {
          props: ['open', 'title'],
          emits: ['close'],
          template:
            '<div v-if="open" data-testid="modal"><button data-testid="modal-close" @click="$emit(\'close\')" /><slot /><slot name="footer" /></div>',
        },
        ParagraphInput: {
          props: ['text'],
          emits: ['update:text'],
          template: '<textarea data-testid="paragraph" :value="text" @input="$emit(\'update:text\', $event.target.value)" />',
        },
        HardLoadLayer: true,
        ReposEditor: true,
        PrioritySegmented: true,
      },
    },
  })
}

beforeEach(() => {
  vi.clearAllMocks()
  apiMocks.listRepos.mockResolvedValue([])
  apiMocks.listProjectRunTags.mockResolvedValue({ tags: [] })
})

describe('RunLaunchModal', () => {
  it('shows form when open with workflow name', async () => {
    const wrapper = mountModal(true)
    await flushPromises()
    expect(wrapper.find('[data-testid="modal"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('测试流水线')
    wrapper.unmount()
  })

  it('does not render body when closed', () => {
    const wrapper = mountModal(false)
    expect(wrapper.find('[data-testid="modal"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('shows required validation error when topic empty', async () => {
    const i18n = createI18n({
      legacy: false,
      locale: 'zh-CN',
      messages: { 'zh-CN': { ...common, ...pages } },
    })
    const wrapper = mount(RunLaunchModal, {
      props: {
        open: true,
        workflowId: 'wf-1',
        workflowName: '测试流水线',
        fields: [{ key: 'topic', desc: '主题', required: true }],
        runInputs: { topic: '' },
        runImages: {},
      },
      global: {
        plugins: [i18n],
        stubs: {
          Icon: true,
          AppButton: { template: '<button v-bind="$attrs"><slot /></button>' },
          AppModal: { props: ['open'], template: '<div v-if="open"><slot /><slot name="footer" /></div>' },
          ParagraphInput: true,
          HardLoadLayer: true,
          ReposEditor: true,
          PrioritySegmented: true,
        },
      },
    })
    await flushPromises()
    const startBtn = findStartButton(wrapper)
    expect(startBtn).toBeTruthy()
    await startBtn!.trigger('click')
    await flushPromises()
    expect(apiMocks.startRun).not.toHaveBeenCalled()
    expect(wrapper.text()).toMatch(/主题|topic|必填|required/i)
    wrapper.unmount()
  })

  it('calls startRun and navigates to run detail', async () => {
    apiMocks.startRun.mockResolvedValue({ id: 'run-99' })
    const wrapper = mountModal(true)
    await flushPromises()
    const startBtn = findStartButton(wrapper)
    await startBtn!.trigger('click')
    await flushPromises()
    expect(apiMocks.startRun).toHaveBeenCalledWith(
      'wf-1',
      expect.objectContaining({ topic: 'hello' }),
      'manual',
      'normal',
      [],
      expect.objectContaining({ signal: expect.any(AbortSignal) }),
    )
    expect(wrapper.emitted('started')?.[0]?.[0]).toBe('run-99')
    expect(wrapper.emitted('view-run')?.[0]?.[0]).toBe('run-99')
    expect(wrapper.emitted('close')).toBeTruthy()
    wrapper.unmount()
  })

  it('rejects reserved env keys in modal without calling API', async () => {
    const wrapper = mountModal(true)
    await flushPromises()
    const addBtn = wrapper.findAll('button').find((b) => b.text().includes('添加行'))
    expect(addBtn).toBeTruthy()
    await addBtn!.trigger('click')
    await flushPromises()
    const row = wrapper.find('[data-testid="run-launch-env-row"]')
    const inputs = row.findAll('input')
    await inputs[0].setValue('CURSOR_API_KEY')
    await inputs[1].setValue('x')
    const startBtn = findStartButton(wrapper)
    await startBtn!.trigger('click')
    await flushPromises()
    expect(apiMocks.startRun).not.toHaveBeenCalled()
    expect(wrapper.text()).toMatch(/CURSOR_API_KEY/)
    wrapper.unmount()
  })

  it('passes env entries to startRun when valid', async () => {
    apiMocks.startRun.mockResolvedValue({ id: 'run-env' })
    const wrapper = mountModal(true)
    await flushPromises()
    const addBtn = wrapper.findAll('button').find((b) => b.text().includes('添加行'))
    await addBtn!.trigger('click')
    await flushPromises()
    const row = wrapper.find('[data-testid="run-launch-env-row"]')
    const inputs = row.findAll('input')
    await inputs[0].setValue('LOG_LEVEL')
    await inputs[1].setValue('debug')
    const startBtn = findStartButton(wrapper)
    await startBtn!.trigger('click')
    await flushPromises()
    expect(apiMocks.startRun).toHaveBeenCalledWith(
      'wf-1',
      expect.anything(),
      'manual',
      'normal',
      [],
      expect.objectContaining({
        env: [{ key: 'LOG_LEVEL', value: 'debug', secret: false }],
      }),
    )
    wrapper.unmount()
  })

  it('shows error phase when startRun rejects', async () => {
    apiMocks.startRun.mockRejectedValue(new Error('network down'))
    const wrapper = mountModal(true)
    await flushPromises()
    const startBtn = findStartButton(wrapper)
    await startBtn!.trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('network down')
    expect(apiMocks.startRun).toHaveBeenCalled()
    wrapper.unmount()
  })

  // g5.2: ProjectDetail/WorkflowList 用 v-if + open=true 挂载，须立即拉取项目存量 tags
  it('fetches project run-tags when mounted with open=true and projectId (v-if path)', async () => {
    apiMocks.listProjectRunTags.mockResolvedValue({ tags: ['bugfix-login', 'spike'] })
    const wrapper = mountModal(true, { projectId: 'proj-1' })
    await flushPromises()
    expect(apiMocks.listProjectRunTags).toHaveBeenCalledTimes(1)
    expect(apiMocks.listProjectRunTags).toHaveBeenCalledWith('proj-1')
    wrapper.unmount()
  })

  it('fetches project run-tags when open flips false→true (editor path)', async () => {
    apiMocks.listProjectRunTags.mockResolvedValue({ tags: ['bugfix'] })
    const wrapper = mountModal(false, { projectId: 'proj-2' })
    await flushPromises()
    expect(apiMocks.listProjectRunTags).not.toHaveBeenCalled()
    await wrapper.setProps({ open: true })
    await flushPromises()
    expect(apiMocks.listProjectRunTags).toHaveBeenCalledTimes(1)
    expect(apiMocks.listProjectRunTags).toHaveBeenCalledWith('proj-2')
    wrapper.unmount()
  })

  it('aborts in-flight start when modal closes during loading', async () => {
    let resolveStart!: (v: { id: string }) => void
    apiMocks.startRun.mockImplementation(
      () =>
        new Promise((resolve) => {
          resolveStart = resolve
        }),
    )
    const wrapper = mountModal(true)
    await flushPromises()
    const startBtn = findStartButton(wrapper)
    await startBtn!.trigger('click')
    await flushPromises()
    expect(wrapper.text()).toMatch(/启动中|Starting/i)
    await wrapper.get('[data-testid="modal-close"]').trigger('click')
    resolveStart({ id: 'run-late' })
    await flushPromises()
    expect(wrapper.emitted('started')).toBeFalsy()
    wrapper.unmount()
  })

  it('does not fetch run-tags when open without projectId', async () => {
    const wrapper = mountModal(true)
    await flushPromises()
    expect(apiMocks.listProjectRunTags).not.toHaveBeenCalled()
    wrapper.unmount()
  })
})
