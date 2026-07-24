// @vitest-environment happy-dom
import { defineComponent } from 'vue'
import { createI18n } from 'vue-i18n'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'
import type { OutputCard, Run } from '@/lib/types'
import OutputResultCards from './OutputResultCards.vue'

const apiMocks = vi.hoisted(() => ({
  artifactContent: vi.fn(),
}))

vi.mock('@/lib/api', async () => {
  const actual = await vi.importActual<typeof import('@/lib/api')>('@/lib/api')
  return {
    ...actual,
    api: {
      ...actual.api,
      artifactContent: apiMocks.artifactContent,
    },
  }
})

const StructuredStub = defineComponent({
  name: 'StructuredArtifactView',
  props: { name: String, doc: Object },
  template: '<div data-testid="structured-view">{{ name }}</div>',
})

const HtmlPreviewStub = defineComponent({
  name: 'HtmlPreview',
  props: { html: String },
  template: '<div data-testid="html-preview" />',
})

const cards: OutputCard[] = [
  {
    index: 0,
    template: 'research',
    title: '调研结果',
    status: 'ok',
    typeTag: '结构化产物',
    structuredArtifactName: 'research.json',
    jsonSnapshot: JSON.stringify({ summary: 'ok', findings: [] }),
  },
  {
    index: 1,
    template: 'custom',
    title: '自定义产物',
    status: 'ok',
    typeTag: '自定义产物',
    artifactName: 'notes.md',
  },
]

const run = {
  id: 'run-1',
  artifacts: [
    {
      id: 'a-notes',
      name: 'notes.md',
      kind: 'markdown',
      nodeId: 'output',
      runId: 'run-1',
      workflowName: 'wf',
      sizeBytes: 5,
      createdAt: '2026-07-18T00:00:00Z',
    },
  ],
} as Run

describe('OutputResultCards', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    apiMocks.artifactContent.mockResolvedValue({ content: '# notes' })
  })

  it('renders structured card from json snapshot', async () => {
    const i18n = createI18n({
      legacy: false,
      locale: 'zh-CN',
      messages: { 'zh-CN': { ...common, ...pages } },
    })
    const wrapper = mount(OutputResultCards, {
      props: { cards: [cards[0]], run },
      global: {
        plugins: [i18n],
        stubs: { StructuredArtifactView: StructuredStub, HtmlPreview: HtmlPreviewStub },
      },
    })
    await flushPromises()
    expect(wrapper.text()).toContain('调研结果')
    expect(wrapper.find('[data-testid="structured-view"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('loads custom artifact content', async () => {
    const i18n = createI18n({
      legacy: false,
      locale: 'zh-CN',
      messages: { 'zh-CN': { ...common, ...pages } },
    })
    const wrapper = mount(OutputResultCards, {
      props: { cards: [cards[1]], run },
      global: {
        plugins: [i18n],
        stubs: { StructuredArtifactView: StructuredStub, HtmlPreview: HtmlPreviewStub },
      },
    })
    await flushPromises()
    expect(apiMocks.artifactContent).toHaveBeenCalledWith('a-notes')
    wrapper.unmount()
  })

  it('applies failed styling for failed cards', async () => {
    const i18n = createI18n({
      legacy: false,
      locale: 'zh-CN',
      messages: { 'zh-CN': { ...common, ...pages } },
    })
    const wrapper = mount(OutputResultCards, {
      props: {
        cards: [{ ...cards[0], status: 'failed', title: '失败卡片' }],
        run,
      },
      global: {
        plugins: [i18n],
        stubs: { StructuredArtifactView: StructuredStub, HtmlPreview: HtmlPreviewStub },
      },
    })
    await flushPromises()
    expect(wrapper.find('article').classes().some((c) => c.includes('err'))).toBe(true)
    wrapper.unmount()
  })
})
