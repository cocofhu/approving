// @vitest-environment happy-dom
import { defineComponent } from 'vue'
import { createI18n } from 'vue-i18n'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'
import AppPreviewPanel from './AppPreviewPanel.vue'

vi.mock('@novnc/novnc/lib/rfb.js', () => ({
  default: class MockRFB {},
}))

const apiMocks = vi.hoisted(() => ({
  nodePreviews: vi.fn(),
}))

vi.mock('@/lib/api', async () => {
  const actual = await vi.importActual<typeof import('@/lib/api')>('@/lib/api')
  return {
    ...actual,
    api: {
      ...actual.api,
      nodePreviews: apiMocks.nodePreviews,
    },
  }
})

const NovncStub = defineComponent({
  name: 'NovncPreviewPanel',
  props: { runId: String, nodeId: String, port: Number, fill: Boolean, compact: Boolean },
  emits: ['pick'],
  template: '<div data-testid="novnc-stub" :data-port="port" />',
})

const FeedbackStub = defineComponent({
  name: 'PreviewFeedbackChat',
  props: { runId: String, nodeId: String, port: Number, compact: Boolean },
  template: '<div data-testid="feedback-stub" />',
})

function mountPanel(opts: { compact?: boolean; fill?: boolean } = {}) {
  const i18n = createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...common, ...pages } },
  })
  return mount(AppPreviewPanel, {
    props: {
      runId: 'run-1',
      nodeId: 'preview-1',
      compact: opts.compact ?? false,
      fill: opts.fill ?? false,
    },
    global: {
      plugins: [i18n],
      stubs: { NovncPreviewPanel: NovncStub, PreviewFeedbackChat: FeedbackStub },
    },
  })
}

describe('AppPreviewPanel', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    apiMocks.nodePreviews.mockResolvedValue({
      ports: [
        { port: 5173, label: '前端' },
        { port: 8080, label: 'API' },
      ],
    })
  })

  it('loads ports and renders preview tabs', async () => {
    const wrapper = mountPanel()
    await flushPromises()
    expect(apiMocks.nodePreviews).toHaveBeenCalledWith('run-1', 'preview-1')
    expect(wrapper.text()).toContain('前端')
    expect(wrapper.find('[data-testid="novnc-stub"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('switches active port tab', async () => {
    apiMocks.nodePreviews.mockResolvedValue({
      ports: [
        { port: 5173, label: '前端' },
        { port: 5174, label: '管理端' },
      ],
    })
    const wrapper = mountPanel()
    await flushPromises()
    const tabs = wrapper.findAll('button').filter((b) => /前端|管理端/.test(b.text()))
    expect(tabs.length).toBe(2)
    await tabs[1].trigger('click')
    await flushPromises()
    expect(tabs[1].classes().some((c) => c.includes('accent'))).toBe(true)
    expect(tabs[0].classes().some((c) => c.includes('accent'))).toBe(false)
    wrapper.unmount()
  })

  it('shows no ports message when list empty', async () => {
    apiMocks.nodePreviews.mockResolvedValue({ ports: [] })
    const wrapper = mountPanel()
    await flushPromises()
    expect(wrapper.text()).toMatch(/暂无|没有|未/)
    wrapper.unmount()
  })
})
