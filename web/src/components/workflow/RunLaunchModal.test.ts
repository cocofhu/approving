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
}))

vi.mock('@/lib/api', async () => {
  const actual = await vi.importActual<typeof import('@/lib/api')>('@/lib/api')
  return {
    ...actual,
    api: {
      ...actual.api,
      startRun: apiMocks.startRun,
      listRepos: apiMocks.listRepos,
    },
  }
})

import RunLaunchModal from './RunLaunchModal.vue'

function mountModal(open = true) {
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
    },
    global: {
      plugins: [i18n],
      stubs: {
        Icon: true,
        AppButton: { template: '<button v-bind="$attrs"><slot /></button>' },
        AppModal: {
          props: ['open', 'title'],
          template: '<div v-if="open" data-testid="modal"><slot /><slot name="footer" /></div>',
        },
        ParagraphInput: {
          props: ['text'],
          emits: ['update:text'],
          template: '<textarea data-testid="paragraph" :value="text" @input="$emit(\'update:text\', $event.target.value)" />',
        },
        ArtifactLoadingPane: true,
        ReposEditor: true,
        PrioritySegmented: true,
      },
    },
  })
}

beforeEach(() => {
  vi.clearAllMocks()
  apiMocks.listRepos.mockResolvedValue([])
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
          ArtifactLoadingPane: true,
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

  it('calls startRun and switches to success phase', async () => {
    apiMocks.startRun.mockResolvedValue({ id: 'run-99' })
    const wrapper = mountModal(true)
    await flushPromises()
    const startBtn = findStartButton(wrapper)
    await startBtn!.trigger('click')
    await flushPromises()
    expect(apiMocks.startRun).toHaveBeenCalledWith('wf-1', expect.objectContaining({ topic: 'hello' }), '手动触发', 'normal')
    expect(wrapper.emitted('started')?.[0]?.[0]).toBe('run-99')
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
})
