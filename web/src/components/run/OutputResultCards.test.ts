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
    index: 1,
    template: 'research',
    title: '调研结果',
    status: 'ok',
    typeTag: '结构化产物',
    structuredArtifactName: 'research.json',
    jsonSnapshot: JSON.stringify({ summary: 'ok', findings: [] }),
  },
  {
    index: 2,
    template: 'custom',
    title: '自定义产物',
    status: 'ok',
    typeTag: '自定义产物',
    artifactName: 'page.html',
  },
  {
    index: 3,
    template: 'plan',
    title: '计划',
    status: 'ok',
    typeTag: 'Markdown',
    markdown: '## 计划正文',
  },
]

const run = {
  id: 'run-1',
  artifacts: [
    {
      id: 'a-page',
      name: 'page.html',
      kind: 'html',
      nodeId: 'output',
      runId: 'run-1',
      workflowName: 'wf',
      sizeBytes: 5,
      createdAt: '2026-07-18T00:00:00Z',
    },
  ],
} as Run

function mountCards(list: OutputCard[] = cards) {
  const i18n = createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...common, ...pages } },
  })
  return mount(OutputResultCards, {
    props: { cards: list, run },
    global: {
      plugins: [i18n],
      stubs: { StructuredArtifactView: StructuredStub, HtmlPreview: HtmlPreviewStub },
    },
  })
}

describe('OutputResultCards exclusive accordion (g4)', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    apiMocks.artifactContent.mockResolvedValue({ content: '<html>ok</html>' })
  })

  it('defaults to first card expanded; others collapsed and not mounted', async () => {
    const wrapper = mountCards()
    await flushPromises()
    const toggles = wrapper.findAll('[data-testid^="output-result-card-toggle-"]')
    expect(toggles).toHaveLength(3)
    expect(toggles[0].attributes('aria-expanded')).toBe('true')
    expect(toggles[1].attributes('aria-expanded')).toBe('false')
    expect(wrapper.find('[data-testid="output-result-card-body-0"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="output-result-card-body-1"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="structured-view"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="html-preview"]').exists()).toBe(false)
    expect(apiMocks.artifactContent).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('clicking another title switches exclusive expand and lazy-loads HTML', async () => {
    const wrapper = mountCards()
    await flushPromises()
    await wrapper.get('[data-testid="output-result-card-toggle-1"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-testid="output-result-card-toggle-0"]').attributes('aria-expanded')).toBe(
      'false',
    )
    expect(wrapper.get('[data-testid="output-result-card-toggle-1"]').attributes('aria-expanded')).toBe(
      'true',
    )
    expect(wrapper.find('[data-testid="output-result-card-body-0"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="html-preview"]').exists()).toBe(true)
    expect(apiMocks.artifactContent).toHaveBeenCalledWith('a-page')
    wrapper.unmount()
  })

  it('clicking the open card collapses all', async () => {
    const wrapper = mountCards()
    await flushPromises()
    await wrapper.get('[data-testid="output-result-card-toggle-0"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-testid="output-result-card-toggle-0"]').attributes('aria-expanded')).toBe(
      'false',
    )
    expect(wrapper.find('[data-testid="output-result-card-body-0"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="structured-view"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('failed cards remain in the list and can expand errorReason', async () => {
    const wrapper = mountCards([
      {
        index: 1,
        template: 'research',
        title: '失败卡片',
        status: 'failed',
        typeTag: '来源失败',
        errorReason: '上游节点失败',
      },
      cards[2],
    ])
    await flushPromises()
    expect(wrapper.text()).toContain('失败卡片')
    expect(wrapper.text()).toContain('上游节点失败')
    await wrapper.get('[data-testid="output-result-card-toggle-1"]').trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('计划正文')
    expect(wrapper.text()).not.toMatch(/打开原始文件|下载/)
    wrapper.unmount()
  })

  it('does not render open/download actions on result cards', async () => {
    const wrapper = mountCards([cards[0]])
    await flushPromises()
    expect(wrapper.text()).not.toMatch(/打开原始文件|下载/)
    wrapper.unmount()
  })
})
