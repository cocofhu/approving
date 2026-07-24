// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'
import PreviewFeedbackChat from './PreviewFeedbackChat.vue'

const apiMocks = vi.hoisted(() => ({
  listPreviewIssues: vi.fn(),
  createPreviewIssue: vi.fn(),
  deletePreviewIssue: vi.fn(),
}))

vi.mock('@/lib/api', async () => {
  const actual = await vi.importActual<typeof import('@/lib/api')>('@/lib/api')
  return {
    ...actual,
    api: {
      ...actual.api,
      listPreviewIssues: apiMocks.listPreviewIssues,
      createPreviewIssue: apiMocks.createPreviewIssue,
      deletePreviewIssue: apiMocks.deletePreviewIssue,
    },
  }
})

function mountChat(opts: { compact?: boolean; selector?: string; copyVariant?: 'review' | 'issue' } = {}) {
  const i18n = createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...common, ...pages } },
  })
  return mount(PreviewFeedbackChat, {
    props: {
      runId: 'run-1',
      nodeId: 'preview-1',
      port: 8080,
      compact: opts.compact ?? false,
      selector: opts.selector ?? '',
      copyVariant: opts.copyVariant ?? 'issue',
    },
    global: {
      plugins: [i18n],
      stubs: { Icon: true, ParagraphInput: false },
    },
  })
}

describe('PreviewFeedbackChat', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    apiMocks.listPreviewIssues.mockResolvedValue({
      issues: [{ id: 'i1', body: '按钮颜色不对', createdAt: '2026-07-18T00:00:00Z' }],
    })
    apiMocks.createPreviewIssue.mockResolvedValue({
      id: 'i2',
      body: '新问题',
      createdAt: '2026-07-18T00:01:00Z',
    })
    apiMocks.deletePreviewIssue.mockResolvedValue(undefined)
  })

  it('loads and renders issue history', async () => {
    const wrapper = mountChat()
    await flushPromises()
    expect(apiMocks.listPreviewIssues).toHaveBeenCalledWith('run-1', 'preview-1')
    expect(wrapper.text()).toContain('按钮颜色不对')
    wrapper.unmount()
  })

  it('submits new issue and clears selector', async () => {
    const wrapper = mountChat({ selector: '#btn-submit' })
    await flushPromises()
    const textarea = wrapper.find('textarea')
    expect(textarea.exists()).toBe(true)
    await textarea.setValue('登录页布局错位')
    const sendBtn = wrapper.findAll('button').find((b) => b.text().includes('提交问题'))
    expect(sendBtn).toBeTruthy()
    await sendBtn!.trigger('click')
    await flushPromises()
    expect(apiMocks.createPreviewIssue).toHaveBeenCalled()
    expect(wrapper.emitted('clear-selector')).toBeTruthy()
    expect(wrapper.emitted('issues-changed')).toBeTruthy()
    wrapper.unmount()
  })

  it('requires body+selector+screenshot in requireElement mode', async () => {
    const wrapper = mount(PreviewFeedbackChat, {
      props: {
        runId: 'run-1',
        nodeId: 'gate-1',
        requireElement: true,
        selector: '',
        elementImage: null,
      },
      global: {
        plugins: [
          createI18n({
            legacy: false,
            locale: 'zh-CN',
            messages: { 'zh-CN': { ...common, ...pages } },
          }),
        ],
        stubs: { Icon: true, ParagraphInput: false },
      },
    })
    await flushPromises()
    const textarea = wrapper.find('textarea')
    await textarea.setValue('问题正文')
    const sendBtn = wrapper.findAll('button').find((b) => b.text().includes('提交问题'))
    expect(sendBtn!.attributes('disabled')).toBeDefined()
    expect(wrapper.text()).toContain('点选')
    await wrapper.setProps({
      selector: 'h1.title',
      elementImage: { data: 'abc', mimeType: 'image/png' },
    })
    await flushPromises()
    expect(sendBtn!.attributes('disabled')).toBeUndefined()
    await sendBtn!.trigger('click')
    await flushPromises()
    expect(apiMocks.createPreviewIssue).toHaveBeenCalledWith(
      'run-1',
      'gate-1',
      '问题正文',
      'h1.title',
      0,
      [{ data: 'abc', mimeType: 'image/png' }],
    )
    wrapper.unmount()
  })

  it('toggles history in compact mode', async () => {
    const wrapper = mountChat({ compact: true })
    await flushPromises()
    const toggle = wrapper.findAll('button').find((b) => b.text().includes('历史') || b.text().includes('反馈'))
    expect(toggle).toBeTruthy()
    await toggle!.trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('按钮颜色不对')
    wrapper.unmount()
  })

  it('uses review copy variant in gate context', async () => {
    const wrapper = mountChat({ copyVariant: 'review' })
    await flushPromises()
    expect(wrapper.text()).toContain('评审意见')
    expect(wrapper.text()).not.toContain('问题反馈')
    const sendBtn = wrapper.findAll('button').find((b) => b.text().includes('提交评审意见'))
    expect(sendBtn).toBeTruthy()
    wrapper.unmount()
  })
})
