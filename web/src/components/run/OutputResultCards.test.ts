// @vitest-environment happy-dom
import { defineComponent } from 'vue'
import { createI18n } from 'vue-i18n'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'
import type { OutputCard, Run } from '@/lib/shared/types'
import OutputResultCards from './OutputResultCards.vue'

const apiMocks = vi.hoisted(() => ({
  artifactContent: vi.fn(),
}))

vi.mock('@/lib/api/api', async () => {
  const actual = await vi.importActual<typeof import('@/lib/api/api')>('@/lib/api/api')
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
  props: {
    html: String,
    enlargeable: { type: Boolean, default: true },
    fitContent: { type: Boolean, default: false },
  },
  template:
    '<div data-testid="html-preview" :data-enlargeable="enlargeable === false ? \'0\' : \'1\'" :data-fit-content="fitContent ? \'1\' : \'0\'" />',
})

const AppModalStub = defineComponent({
  name: 'AppModal',
  props: {
    open: Boolean,
    title: String,
    width: Number,
    closeOnEsc: Boolean,
  },
  emits: ['close'],
  template: `
    <div
      v-if="open"
      data-testid="output-result-enlarge-modal"
      role="dialog"
      :data-width="String(width || '')"
      :data-close-on-esc="closeOnEsc ? '1' : '0'"
    >
      <div data-testid="output-result-enlarge-title">{{ title }}</div>
      <button type="button" data-testid="output-result-enlarge-close" @click="$emit('close')">close</button>
      <slot />
    </div>
  `,
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
    {
      id: 'a-notes',
      name: 'notes.txt',
      kind: 'text',
      nodeId: 'output',
      runId: 'run-1',
      workflowName: 'wf',
      sizeBytes: 4,
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
      stubs: {
        StructuredArtifactView: StructuredStub,
        HtmlPreview: HtmlPreviewStub,
        AppModal: AppModalStub,
        Icon: true,
      },
    },
  })
}

describe('OutputResultCards master-detail list + enlarge (g4.1)', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    apiMocks.artifactContent.mockResolvedValue({ content: '<html>ok</html>' })
  })

  it('multi-card: shows name list, always has current item, default first (g1.1/g1.2)', async () => {
    const wrapper = mountCards()
    await flushPromises()
    expect(wrapper.find('[data-testid="output-result-list"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="output-result-list-header"]').text()).toMatch(/产出/)
    expect(wrapper.get('[data-testid="output-result-list-header"]').text()).toContain('3')
    const toggles = wrapper.findAll('[data-testid^="output-result-card-toggle-"]')
    expect(toggles).toHaveLength(3)
    expect(toggles[0].attributes('aria-selected')).toBe('true')
    expect(toggles[1].attributes('aria-selected')).toBe('false')
    expect(wrapper.find('[data-testid="output-result-card-body-0"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="output-result-card-body-1"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="structured-view"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="html-preview"]').exists()).toBe(false)
    expect(apiMocks.artifactContent).not.toHaveBeenCalled()
    expect(wrapper.get('[data-testid="output-result-list-kind-0"]').text()).toBe('结构化')
    expect(wrapper.get('[data-testid="output-result-list-kind-1"]').text()).toBe('HTML')
    expect(wrapper.get('[data-testid="output-result-list-kind-2"]').text()).toBe('Markdown')
    wrapper.unmount()
  })

  it('clicking another row switches exclusive detail and lazy-loads HTML only for current (g1.4/g1.6)', async () => {
    const wrapper = mountCards()
    await flushPromises()
    await wrapper.get('[data-testid="output-result-card-toggle-1"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-testid="output-result-card-toggle-0"]').attributes('aria-selected')).toBe(
      'false',
    )
    expect(wrapper.get('[data-testid="output-result-card-toggle-1"]').attributes('aria-selected')).toBe(
      'true',
    )
    expect(wrapper.find('[data-testid="output-result-card-body-0"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="output-result-card-body-1"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="html-preview"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="html-preview"]').attributes('data-enlargeable')).toBe('0')
    expect(wrapper.get('[data-testid="html-preview"]').attributes('data-fit-content')).toBe('1')
    expect(apiMocks.artifactContent).toHaveBeenCalledWith('a-page')
    expect(wrapper.find('[data-testid="html-preview-enlarge"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('clicking the current row does not collapse (g1.1)', async () => {
    const wrapper = mountCards()
    await flushPromises()
    await wrapper.get('[data-testid="output-result-card-toggle-0"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-testid="output-result-card-toggle-0"]').attributes('aria-selected')).toBe(
      'true',
    )
    expect(wrapper.find('[data-testid="output-result-card-body-0"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="structured-view"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('single card hides list and still shows detail (g1.3)', async () => {
    const wrapper = mountCards([cards[0]])
    await flushPromises()
    expect(wrapper.find('[data-testid="output-result-list"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid^="output-result-card-toggle-"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="output-result-detail-bar"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="output-result-card-body-0"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="output-result-enlarge"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('failed row is visible without enlarge; switching back restores enlarge (g2.1/g1.5)', async () => {
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
    expect(wrapper.get('[data-testid="output-result-list-kind-0"]').text()).toBe('失败')
    expect(wrapper.text()).toContain('上游节点失败')
    expect(wrapper.find('[data-testid="output-result-enlarge"]').exists()).toBe(false)
    await wrapper.get('[data-testid="output-result-card-toggle-1"]').trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('计划正文')
    expect(wrapper.find('[data-testid="output-result-enlarge"]').exists()).toBe(true)
    expect(wrapper.text()).not.toMatch(/打开原始文件|下载/)
    wrapper.unmount()
  })

  it('success item opens/closes enlarge modal (width=960, Esc) without changing selection (g2.2/g2.3)', async () => {
    const wrapper = mountCards()
    await flushPromises()
    expect(wrapper.find('[data-testid="output-result-enlarge-modal"]').exists()).toBe(false)
    await wrapper.get('[data-testid="output-result-enlarge"]').trigger('click')
    await flushPromises()
    const modal = wrapper.get('[data-testid="output-result-enlarge-modal"]')
    expect(modal.attributes('data-width')).toBe('960')
    expect(modal.attributes('data-close-on-esc')).toBe('1')
    expect(wrapper.get('[data-testid="output-result-enlarge-title"]').text()).toBe('调研结果')
    expect(wrapper.get('[data-testid="output-result-enlarge-body"]').text()).toContain('research.json')
    expect(modal.text()).not.toMatch(/打开原始文件|下载/)
    await wrapper.get('[data-testid="output-result-enlarge-close"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="output-result-enlarge-modal"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="output-result-card-toggle-0"]').attributes('aria-selected')).toBe(
      'true',
    )
    wrapper.unmount()
  })

  it('custom non-HTML success item can enlarge (g2.1)', async () => {
    apiMocks.artifactContent.mockResolvedValue({ content: 'plain notes' })
    const wrapper = mountCards([
      {
        index: 1,
        template: 'custom',
        title: '备注',
        status: 'ok',
        typeTag: '自定义产物',
        artifactName: 'notes.txt',
      },
    ])
    await flushPromises()
    expect(wrapper.find('[data-testid="output-result-list"]').exists()).toBe(false)
    expect(wrapper.text()).toContain('plain notes')
    expect(wrapper.find('[data-testid="output-result-enlarge"]').exists()).toBe(true)
    await wrapper.get('[data-testid="output-result-enlarge"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-testid="output-result-enlarge-body"]').text()).toContain('plain notes')
    wrapper.unmount()
  })

  it('does not render open/download actions on result cards (g2.3)', async () => {
    const wrapper = mountCards([cards[0]])
    await flushPromises()
    expect(wrapper.text()).not.toMatch(/打开原始文件|下载/)
    expect(wrapper.text()).not.toContain('窗口放大查看')
    wrapper.unmount()
  })

  it('list has no independent overflow / max-height (g3.2)', async () => {
    const wrapper = mountCards()
    await flushPromises()
    const list = wrapper.get('[data-testid="output-result-list"]')
    expect(list.classes()).not.toContain('overflow-y-auto')
    expect(list.classes()).not.toContain('max-h-48')
    expect(list.classes().some((c) => c.startsWith('max-h-'))).toBe(false)
    wrapper.unmount()
  })

  it('success card without renderable body has no enlarge (g2.1/F3)', async () => {
    const wrapper = mountCards([
      {
        index: 1,
        template: 'research',
        title: '空结构化',
        status: 'ok',
        typeTag: '结构化产物',
        structuredArtifactName: 'research.json',
      },
    ])
    await flushPromises()
    expect(wrapper.text()).toContain('无效或无法解析的来源引用')
    expect(wrapper.find('[data-testid="output-result-enlarge"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('HTML enlarge modal uses 70vh viewport and keeps enlargeable=false (g1.6/g2.3/F4)', async () => {
    const wrapper = mountCards([cards[1]])
    await flushPromises()
    expect(wrapper.find('[data-testid="output-result-list"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="output-result-enlarge"]').exists()).toBe(true)
    const detailPreview = wrapper.get('[data-testid="html-preview"]')
    expect(detailPreview.attributes('data-enlargeable')).toBe('0')
    expect(detailPreview.attributes('data-fit-content')).toBe('1')
    await wrapper.get('[data-testid="output-result-enlarge"]').trigger('click')
    await flushPromises()
    const viewport = wrapper.get('[data-testid="output-result-enlarge-html-viewport"]')
    expect(viewport.classes()).toContain('h-[70vh]')
    const modalPreview = viewport.get('[data-testid="html-preview"]')
    expect(modalPreview.attributes('data-enlargeable')).toBe('0')
    expect(modalPreview.attributes('data-fit-content')).toBe('0')
    expect(wrapper.get('[data-testid="output-result-enlarge-body"]').text()).not.toMatch(
      /打开原始文件|下载|窗口放大查看/,
    )
    wrapper.unmount()
  })

  it('nodes.*.outputs.page card: HtmlPreview + 自定义产物·HTML kind (g2.1/g2.2)', async () => {
    const pageCard: OutputCard = {
      index: 1,
      template: '{{nodes.visual.outputs.page}}',
      title: '网页预览 · 视觉网页',
      status: 'ok',
      typeTag: '自定义产物',
      artifactName: 'page.html',
      nodeId: 'visual',
      outputKey: 'page',
      markdown: '<!doctype html><html><body><h1>视觉网页</h1></body></html>',
    }
    const wrapper = mountCards([pageCard])
    await flushPromises()
    expect(wrapper.text()).not.toContain('结构化产物')
    expect(wrapper.get('[data-testid="output-result-detail-kind"]').text()).toBe('自定义产物 · HTML')
    expect(wrapper.find('[data-testid="html-preview"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="structured-view"]').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('<!doctype html>')
    await wrapper.get('[data-testid="output-result-enlarge"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-testid="output-result-enlarge-title"]').text()).toBe(
      '网页预览 · 视觉网页',
    )
    expect(wrapper.find('[data-testid="output-result-enlarge-html-viewport"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="html-preview"]').exists()).toBe(true)
    expect(wrapper.text()).not.toContain('无法预览')
    wrapper.unmount()
  })

  it('custom HTML empty/load-fail shows 无法预览, never Markdown source (g2.3)', async () => {
    apiMocks.artifactContent.mockRejectedValue(new Error('fetch failed'))
    const emptyPage: OutputCard = {
      index: 1,
      template: '{{nodes.visual.outputs.page}}',
      title: '网页预览 · 视觉网页',
      status: 'ok',
      typeTag: '自定义产物',
      artifactName: 'page.html',
      nodeId: 'visual',
      outputKey: 'page',
    }
    const wrapper = mountCards([emptyPage])
    await flushPromises()
    expect(wrapper.find('[data-testid="output-result-html-unavailable"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('无法预览')
    expect(wrapper.find('[data-testid="html-preview"]').exists()).toBe(false)
    expect(wrapper.find('.md').exists()).toBe(false)
    expect(wrapper.text()).not.toMatch(/<!doctype|<html|<body/)
    wrapper.unmount()
  })

  it('custom HTML uses markdown fallback when artifact missing (no source leak)', async () => {
    const pageOnlyMd: OutputCard = {
      index: 1,
      template: '{{nodes.visual.outputs.page}}',
      title: '网页预览 · 视觉网页',
      status: 'ok',
      typeTag: '自定义产物',
      artifactName: 'page.html',
      markdown: '<!doctype html><html><body><p>from-markdown</p></body></html>',
    }
    const wrapper = mountCards([pageOnlyMd])
    await flushPromises()
    // No matching artifact in run → cache empty, markdown feeds HtmlPreview
    expect(wrapper.find('[data-testid="html-preview"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="output-result-html-unavailable"]').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('<!doctype html>')
    wrapper.unmount()
  })
})
