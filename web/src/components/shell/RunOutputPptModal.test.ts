// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'
import shell from '@/locales/zh-CN/shell.json'
import type { Artifact, OutputCard, Run, WFNode } from '@/lib/shared/types'

const push = vi.fn()

vi.mock('vue-router', () => ({
  useRouter: () => ({ push }),
}))

const apiMocks = vi.hoisted(() => ({
  getRun: vi.fn(),
  artifactContent: vi.fn(),
  artifactDownloadUrl: vi.fn((id: string) => `http://test/api/artifacts/${id}/download`),
}))

vi.mock('@/lib/api/api', async () => {
  const actual = await vi.importActual<typeof import('@/lib/api/api')>('@/lib/api/api')
  return {
    ...actual,
    api: {
      ...actual.api,
      getRun: apiMocks.getRun,
      artifactContent: apiMocks.artifactContent,
      artifactDownloadUrl: apiMocks.artifactDownloadUrl,
    },
  }
})

vi.mock('@/lib/composables/useToast', () => ({
  useToast: () => ({ success: vi.fn(), error: vi.fn(), warn: vi.fn() }),
}))

import RunOutputPptModal from './RunOutputPptModal.vue'

function artifact(partial: Partial<Artifact> & Pick<Artifact, 'id' | 'name' | 'kind'>): Artifact {
  return {
    nodeId: partial.nodeId ?? 'out-1',
    runId: partial.runId ?? 'run-1',
    workflowName: partial.workflowName ?? 'wf',
    sizeBytes: partial.sizeBytes ?? 128,
    createdAt: partial.createdAt ?? '2026-08-10T12:00:00Z',
    ...partial,
  }
}

function outputCard(partial: Partial<OutputCard> & Pick<OutputCard, 'index' | 'title'>): OutputCard {
  return {
    template: partial.template ?? `artifact("${partial.artifactName || 'page.html'}")`,
    typeTag: partial.typeTag ?? '自定义产物',
    status: partial.status ?? 'ok',
    artifactName: partial.artifactName ?? 'page.html',
    ...partial,
  }
}

function baseRun(partial: Partial<Run> = {}): Run {
  const nodes: WFNode[] = partial.nodes ?? [
    { id: 'out-1', type: 'output', label: '输出', position: { x: 0, y: 0 }, config: {} },
  ]
  return {
    id: 'run-1',
    workflowId: 'wf',
    workflowName: 'wf',
    title: 'done',
    status: 'completed',
    trigger: 'manual',
    startedAt: '2026-08-10T12:00:00Z',
    durationSec: 1,
    progress: 100,
    nodes,
    edges: [],
    nodeRuns: {},
    artifacts: [],
    ...partial,
  }
}

function mountModal(props: { open?: boolean; runId?: string | null } = {}) {
  const i18n = createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...common, ...pages, ...shell } },
  })
  return mount(RunOutputPptModal, {
    props: {
      open: props.open ?? true,
      runId: props.runId ?? 'run-1',
      contextLabel: '自我迭代',
    },
    global: {
      plugins: [i18n],
      stubs: {
        Icon: true,
        HtmlPreview: { template: '<div data-testid="html-stub" />' },
        StructuredArtifactView: {
          props: ['name', 'doc'],
          template:
            '<div data-testid="structured-stub">{{ name }} · {{ doc?.title || doc?.summary || "ui" }}</div>',
        },
        AppModal: {
          props: ['open', 'title', 'width', 'bodyOverflow', 'bodyMinHeight'],
          template:
            '<div v-if="open" data-testid="run-output-modal"><div data-testid="modal-title">{{ title }}</div><slot /><div data-testid="modal-footer"><slot name="footer" /></div></div>',
        },
        AppButton: {
          emits: ['click'],
          template: '<button v-bind="$attrs" @click="$emit(\'click\')"><slot /></button>',
        },
      },
    },
    attachTo: document.body,
  })
}

describe('RunOutputPptModal output result cards (g1/g2)', () => {
  beforeEach(() => {
    push.mockReset()
    apiMocks.getRun.mockReset()
    apiMocks.artifactContent.mockReset()
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('shows empty dual exits when no outputCards (g2.1/g2.2/g2.3/g2.4)', async () => {
    apiMocks.getRun.mockResolvedValue(
      baseRun({
        artifacts: [
          artifact({ id: 'a-nc', name: 'node_complete.json', kind: 'json', nodeId: 'agent-1' }),
          artifact({ id: 'a-plan', name: 'plan.json', kind: 'json', nodeId: 'plan' }),
        ],
        nodeRuns: {
          'out-1': {
            nodeId: 'out-1',
            status: 'completed',
            outputs: { outputCards: [] },
          },
        },
      }),
    )
    const wrapper = mountModal()
    await flushPromises()

    expect(wrapper.find('[data-testid="run-output-empty"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('暂无最终结果可预览')
    expect(wrapper.text()).toContain('不会回退成全量产物列表')
    expect(wrapper.find('[data-testid="run-output-empty-open-run"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="run-output-empty-open-artifacts"]').exists()).toBe(true)
    // Must not render full artifact browser / node_complete (g1.3 / g2.4)
    expect(wrapper.find('[data-testid="run-output-master-detail"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="run-output-list"]').exists()).toBe(false)
    expect(wrapper.findAll('[data-testid="run-output-row"]')).toHaveLength(0)
    expect(wrapper.text()).not.toContain('node_complete.json')
    expect(wrapper.text()).not.toContain('plan.json')

    await wrapper.find('[data-testid="run-output-empty-open-artifacts"]').trigger('click')
    expect(push).toHaveBeenCalledWith({ path: '/runs/run-1', query: { detail: 'artifacts' } })
    wrapper.unmount()
  })

  it('shows load error without fake preview', async () => {
    apiMocks.getRun.mockRejectedValue(new Error('network down'))
    const wrapper = mountModal()
    await flushPromises()
    expect(wrapper.find('[data-testid="run-output-load-error"]').text()).toContain('network down')
    expect(wrapper.find('[data-testid="run-output-result-cards"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('embeds OutputResultCards for focused output node and hides node_complete (g1.1/g1.2/g1.3/g1.4)', async () => {
    const cards = [
      outputCard({
        index: 1,
        title: '视觉 Demo',
        template: 'artifact("page.html")',
        artifactName: 'page.html',
        typeTag: '自定义产物',
      }),
      outputCard({
        index: 2,
        title: '调研结论',
        template: 'research',
        typeTag: '结构化产物',
        structuredArtifactName: 'research.json',
        jsonSnapshot: JSON.stringify({ summary: 'ok', findings: [] }),
        artifactName: undefined,
      }),
    ]
    apiMocks.getRun.mockResolvedValue(
      baseRun({
        artifacts: [
          artifact({ id: 'a-page', name: 'page.html', kind: 'html', nodeId: 'visual' }),
          artifact({ id: 'a-nc', name: 'node_complete.json', kind: 'json', nodeId: 'submit_mr' }),
          artifact({ id: 'a-plan', name: 'plan.json', kind: 'json', nodeId: 'plan' }),
          artifact({
            id: 'a-research',
            name: 'research.json',
            kind: 'json',
            nodeId: 'research',
          }),
        ],
        nodeRuns: {
          'out-1': {
            nodeId: 'out-1',
            status: 'completed',
            startedAt: '2026-08-10T12:01:00Z',
            outputs: { outputCards: cards },
          },
        },
      }),
    )
    apiMocks.artifactContent.mockResolvedValue({
      id: 'a-page',
      name: 'page.html',
      kind: 'html',
      nodeId: 'visual',
      runId: 'run-1',
      workflowName: 'wf',
      sizeBytes: 128,
      createdAt: '2026-08-10T12:00:00Z',
      content: '<html><body>demo</body></html>',
    })

    const wrapper = mountModal()
    await flushPromises()
    await nextTick()

    expect(wrapper.find('[data-testid="run-output-result-cards"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="run-output-focus-bar"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="run-output-focus-bar"]').text()).toContain('聚焦输出节点')
    expect(wrapper.find('[data-testid="run-output-focus-bar"]').text()).toContain('type: output')
    expect(wrapper.find('[data-testid="output-result-cards"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="output-result-list"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('视觉 Demo')
    expect(wrapper.text()).toContain('调研结论')
    // Full artifact browser must be gone (plan evidence: no node_complete / plan list)
    expect(wrapper.find('[data-testid="run-output-list"]').exists()).toBe(false)
    expect(wrapper.findAll('[data-testid="run-output-row"]')).toHaveLength(0)
    expect(wrapper.text()).not.toContain('node_complete.json')
    expect(wrapper.text()).not.toMatch(/PPT|幻灯片|16:9/)

    await wrapper.find('[data-testid="run-output-open-run"]').trigger('click')
    expect(push).toHaveBeenCalledWith({
      path: '/runs/run-1',
      query: { node: 'out-1', tab: 'output' },
    })
    wrapper.unmount()
  })

  it('focuses last executed output node among multiple outputs (g1.1 / f3)', async () => {
    const cardsA = [outputCard({ index: 1, title: '旧输出卡', artifactName: 'old.html' })]
    const cardsB = [outputCard({ index: 1, title: '最新输出卡', artifactName: 'new.html' })]
    apiMocks.getRun.mockResolvedValue(
      baseRun({
        nodes: [
          { id: 'out-a', type: 'output', label: '输出A', position: { x: 0, y: 0 }, config: {} },
          { id: 'out-b', type: 'output', label: '输出B', position: { x: 1, y: 0 }, config: {} },
        ],
        nodeRuns: {
          'out-a': {
            nodeId: 'out-a',
            status: 'completed',
            startedAt: '2026-08-10T12:00:00Z',
            outputs: { outputCards: cardsA },
          },
          'out-b': {
            nodeId: 'out-b',
            status: 'completed',
            startedAt: '2026-08-10T12:05:00Z',
            outputs: { outputCards: cardsB },
          },
        },
        artifacts: [artifact({ id: 'a1', name: 'new.html', kind: 'html' })],
      }),
    )
    apiMocks.artifactContent.mockResolvedValue({
      id: 'a1',
      name: 'new.html',
      kind: 'html',
      content: '<p>new</p>',
    })

    const wrapper = mountModal()
    await flushPromises()
    expect(wrapper.find('[data-testid="run-output-focus-bar"]').text()).toContain('输出B')
    expect(wrapper.text()).toContain('最新输出卡')
    expect(wrapper.text()).not.toContain('旧输出卡')
    wrapper.unmount()
  })

  it('keeps footer open-run and done actions', async () => {
    apiMocks.getRun.mockResolvedValue(baseRun({ nodeRuns: {} }))
    const wrapper = mountModal()
    await flushPromises()
    expect(wrapper.find('[data-testid="run-output-open-run"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="run-output-done"]').exists()).toBe(true)
    await wrapper.find('[data-testid="run-output-open-run"]').trigger('click')
    // Falls back to first graph output node even when none executed
    expect(push).toHaveBeenCalledWith({
      path: '/runs/run-1',
      query: { node: 'out-1', tab: 'output' },
    })
    wrapper.unmount()
  })
})
