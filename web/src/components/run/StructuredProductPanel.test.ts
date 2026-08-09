// @vitest-environment happy-dom
import { defineComponent, nextTick } from 'vue'
import { createI18n } from 'vue-i18n'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'
import type { Artifact, NodeRun, Run, WFNode } from '@/lib/types'
import { useReviewAnnotate } from '@/lib/reviewAnnotate'
import StructuredProductPanel from './StructuredProductPanel.vue'

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
  setup() {
    const channel = useReviewAnnotate()
    return { channel }
  },
  template: `
    <div data-testid="structured-view">
      <span>{{ name }}</span>
      <span v-if="doc && doc.title">{{ doc.title }}</span>
      <ul v-if="doc && doc.goals">
        <li v-for="(g, i) in doc.goals" :key="i">{{ g }}</li>
      </ul>
      <button
        v-if="channel && channel.enabled"
        data-testid="annotate-affordance"
        type="button"
      >↗ 标注</button>
    </div>
  `,
})

const HtmlPreviewStub = defineComponent({
  name: 'HtmlPreview',
  props: { html: String, inspectable: Boolean, fillParent: Boolean },
  template:
    '<div data-testid="html-preview" :data-fill-parent="fillParent ? \'1\' : \'0\'" :data-inspectable="inspectable ? \'1\' : \'0\'">{{ html }}</div>',
})

const AppModalStub = defineComponent({
  name: 'AppModal',
  props: {
    open: Boolean,
    title: String,
    width: Number,
    closeOnBackdrop: { type: Boolean, default: true },
  },
  emits: ['close'],
  template: `
    <div
      v-if="open"
      data-testid="upstream-enlarge-modal"
      :data-width="width"
      :data-close-on-backdrop="closeOnBackdrop ? '1' : '0'"
    >
      <div data-testid="modal-backdrop" @click="closeOnBackdrop && $emit('close')" />
      <button data-testid="modal-close" type="button" @click="$emit('close')">close</button>
      <slot />
      <slot name="footer" />
    </div>
  `,
})

function artifact(overrides: Partial<Artifact> = {}): Artifact {
  return {
    id: 'a1',
    name: 'research.json',
    kind: 'json',
    nodeId: 'research',
    runId: 'run-1',
    workflowName: 'wf',
    sizeBytes: 10,
    createdAt: '2026-07-18T00:00:00Z',
    ...overrides,
  }
}

function visualNode(): WFNode {
  return { id: 'visual', type: 'visual', label: '视觉', position: { x: 0, y: 0 }, config: {} }
}

function reactNode(): WFNode {
  return { id: 'react', type: 'react', label: '需求澄清', position: { x: 0, y: 0 }, config: {} }
}

const REQ_DOC = {
  title: '复审产物台展示上游澄清需求文档',
  summary: '对照审阅当前主产物',
  goals: ['就地查看上游澄清需求'],
}

function runWithArtifacts(artifacts: Artifact[], extras: Partial<Run> = {}): Run {
  return {
    id: 'run-1',
    status: 'waiting_human',
    artifacts,
    ...extras,
  } as unknown as Run
}

function mountPanel(node: WFNode, nodeRun: NodeRun, run: Run, opts?: { annotatable?: boolean }) {
  const i18n = createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...common, ...pages } },
  })
  return mount(StructuredProductPanel, {
    props: { node, nodeRun, run, annotatable: opts?.annotatable },
    global: {
      plugins: [i18n],
      stubs: {
        Icon: true,
        StructuredArtifactView: StructuredStub,
        HtmlPreview: HtmlPreviewStub,
        AppModal: AppModalStub,
      },
    },
    attachTo: document.body,
  })
}

describe('StructuredProductPanel', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    apiMocks.artifactContent.mockResolvedValue({
      content: JSON.stringify({ summary: '调研', findings: [{ title: 'f1' }] }),
    })
  })

  it('loads research.json from node output snapshot', async () => {
    const node: WFNode = {
      id: 'research',
      type: 'research',
      label: '调研',
      position: { x: 0, y: 0 },
      config: {},
    }
    const nodeRun: NodeRun = {
      nodeId: 'research',
      iteration: 1,
      status: 'completed',
      outputs: { research_json: JSON.stringify({ summary: '调研', findings: [] }) },
    }
    const run = {
      id: 'run-1',
      artifacts: [
        {
          id: 'a1',
          name: 'research.json',
          kind: 'json',
          nodeId: 'research',
          runId: 'run-1',
          workflowName: 'wf',
          sizeBytes: 10,
          createdAt: '2026-07-18T00:00:00Z',
        },
      ],
    } as unknown as Run
    const wrapper = mountPanel(node, nodeRun, run)
    await flushPromises()
    expect(wrapper.find('[data-testid="structured-view"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('renders visual node HTML preview from execution snapshot', async () => {
    const node: WFNode = {
      id: 'visual',
      type: 'visual',
      label: '视觉',
      position: { x: 0, y: 0 },
      config: {},
    }
    const nodeRun: NodeRun = {
      nodeId: 'visual',
      iteration: 1,
      status: 'completed',
      outputs: { page: '<!doctype html><html><body>demo</body></html>' },
    }
    const run = { id: 'run-1', artifacts: [] } as unknown as Run
    const wrapper = mountPanel(node, nodeRun, run)
    await flushPromises()
    expect(wrapper.find('[data-testid="html-preview"]').exists()).toBe(true)
    expect(apiMocks.artifactContent).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('review annotatable visual follows live store over stale snap', async () => {
    apiMocks.artifactContent.mockResolvedValue({
      content: '<!doctype html><html><body>live-review</body></html>',
    })
    const node: WFNode = {
      id: 'visual',
      type: 'visual',
      label: '视觉',
      position: { x: 0, y: 0 },
      config: {},
    }
    const nodeRun: NodeRun = {
      nodeId: 'visual',
      iteration: 1,
      status: 'waiting_human',
      outputs: { page: '<!doctype html><html><body>stale</body></html>' },
    }
    const run = {
      id: 'run-1',
      artifacts: [
        {
          id: 'a-page',
          name: 'page.html',
          kind: 'html',
          nodeId: 'visual',
          runId: 'run-1',
          workflowName: 'wf',
          sizeBytes: 50,
          createdAt: '2026-07-18T00:00:00Z',
        },
      ],
    } as unknown as Run
    const wrapper = mountPanel(node, nodeRun, run, { annotatable: true })
    await flushPromises()
    expect(apiMocks.artifactContent).toHaveBeenCalled()
    expect(wrapper.find('[data-testid="html-preview"]').text()).toContain('live-review')
    wrapper.unmount()
  })

  it('completed historical visual tab keeps frozen snap and ignores store', async () => {
    apiMocks.artifactContent.mockResolvedValue({
      content: '<!doctype html><html><body>latest-store</body></html>',
    })
    const node: WFNode = {
      id: 'visual',
      type: 'visual',
      label: '视觉',
      position: { x: 0, y: 0 },
      config: {},
    }
    const nodeRun: NodeRun = {
      nodeId: 'visual',
      iteration: 1,
      status: 'completed',
      outputs: { page: '<!doctype html><html><body>frozen-v1</body></html>' },
    }
    const run = {
      id: 'run-1',
      artifacts: [
        {
          id: 'a-page',
          name: 'page.html',
          kind: 'html',
          nodeId: 'visual',
          runId: 'run-1',
          workflowName: 'wf',
          sizeBytes: 99,
          createdAt: '2026-07-18T00:00:00Z',
        },
      ],
    } as unknown as Run
    const wrapper = mountPanel(node, nodeRun, run)
    await flushPromises()
    expect(apiMocks.artifactContent).not.toHaveBeenCalled()
    expect(wrapper.find('[data-testid="html-preview"]').text()).toContain('frozen-v1')
    wrapper.unmount()
  })

  it('completed + annotatable still freezes snap (annotatable ≠ followLive)', async () => {
    apiMocks.artifactContent.mockResolvedValue({
      content: '<!doctype html><html><body>live-should-not-win</body></html>',
    })
    const node: WFNode = {
      id: 'visual',
      type: 'visual',
      label: '视觉',
      position: { x: 0, y: 0 },
      config: {},
    }
    const nodeRun: NodeRun = {
      nodeId: 'visual',
      iteration: 1,
      status: 'completed',
      outputs: { page: '<!doctype html><html><body>frozen-history</body></html>' },
    }
    const run = {
      id: 'run-1',
      artifacts: [
        {
          id: 'a-page',
          name: 'page.html',
          kind: 'html',
          nodeId: 'visual',
          runId: 'run-1',
          workflowName: 'wf',
          sizeBytes: 80,
          createdAt: '2026-07-18T00:00:00Z',
        },
      ],
    } as unknown as Run
    const wrapper = mountPanel(node, nodeRun, run, { annotatable: true })
    await flushPromises()
    expect(apiMocks.artifactContent).not.toHaveBeenCalled()
    expect(wrapper.find('[data-testid="html-preview"]').text()).toContain('frozen-history')
    wrapper.unmount()
  })

  it('shows persistent upstream bar when run has clarified_requirement.json and main is page.html', async () => {
    apiMocks.artifactContent.mockImplementation(async (id: string) => {
      if (id === 'a-req') return { content: JSON.stringify(REQ_DOC) }
      return { content: '<!doctype html><html><body>page</body></html>' }
    })
    const nodeRun: NodeRun = {
      nodeId: 'visual',
      iteration: 1,
      status: 'completed',
      outputs: { page: '<!doctype html><html><body>frozen-v1</body></html>' },
    }
    const run = runWithArtifacts([
      artifact({ id: 'a-page', name: 'page.html', kind: 'html', nodeId: 'visual' }),
      artifact({ id: 'a-req', name: 'clarified_requirement.json', kind: 'json', nodeId: 'react' }),
    ])
    const wrapper = mountPanel(visualNode(), nodeRun, run, { annotatable: true })
    await flushPromises()

    const bar = wrapper.find('[data-testid="upstream-context"]')
    expect(bar.exists()).toBe(true)
    expect(bar.attributes('data-variant')).toBe('persistent-bar')
    expect(wrapper.find('[data-testid="upstream-context-toggle"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="upstream-bar-hint"]').text()).toContain('上游上下文')
    expect(wrapper.find('[data-testid="upstream-enlarge"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="structured-product-name"]').text()).toBe('page.html')
    expect(wrapper.find('[data-testid="structured-product-header"]').text()).not.toContain(
      'clarified_requirement.json',
    )
    expect(wrapper.find('[data-testid="html-preview"]').attributes('data-fill-parent')).toBe('1')
    expect(wrapper.find('[data-testid="html-preview"]').attributes('data-inspectable')).toBe('1')
    expect(apiMocks.artifactContent).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('hides upstream bar when clarified_requirement.json is absent', async () => {
    const nodeRun: NodeRun = {
      nodeId: 'visual',
      iteration: 1,
      status: 'completed',
      outputs: { page: '<!doctype html><html><body>demo</body></html>' },
    }
    const run = runWithArtifacts([
      artifact({ id: 'a-page', name: 'page.html', kind: 'html', nodeId: 'visual' }),
    ])
    const wrapper = mountPanel(visualNode(), nodeRun, run)
    await flushPromises()
    expect(wrapper.find('[data-testid="upstream-context"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="html-preview"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('hides upstream bar when main product is clarified_requirement.json', async () => {
    const nodeRun: NodeRun = {
      nodeId: 'react',
      iteration: 1,
      status: 'completed',
      outputs: { clarified_requirement_json: JSON.stringify(REQ_DOC) },
    }
    const run = runWithArtifacts([
      artifact({ id: 'a-req', name: 'clarified_requirement.json', kind: 'json', nodeId: 'react' }),
    ])
    const wrapper = mountPanel(reactNode(), nodeRun, run, { annotatable: true })
    await flushPromises()
    expect(wrapper.find('[data-testid="structured-product-name"]').text()).toBe(
      'clarified_requirement.json',
    )
    expect(wrapper.find('[data-testid="upstream-context"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="annotate-affordance"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('opens 960px readonly upstream modal without ↗; main preview stays annotatable', async () => {
    apiMocks.artifactContent.mockImplementation(async (id: string) => {
      if (id === 'a-req') return { content: JSON.stringify(REQ_DOC) }
      return { content: '<html>page</html>' }
    })
    const nodeRun: NodeRun = {
      nodeId: 'visual',
      iteration: 1,
      status: 'completed',
      outputs: { page: '<!doctype html><html><body>frozen-v1</body></html>' },
    }
    const run = runWithArtifacts([
      artifact({ id: 'a-page', name: 'page.html', kind: 'html', nodeId: 'visual' }),
      artifact({ id: 'a-req', name: 'clarified_requirement.json', kind: 'json', nodeId: 'react' }),
    ])
    const wrapper = mountPanel(visualNode(), nodeRun, run, { annotatable: true })
    await flushPromises()

    await wrapper.find('[data-testid="upstream-enlarge"]').trigger('click')
    await flushPromises()

    const modal = wrapper.find('[data-testid="upstream-enlarge-modal"]')
    expect(modal.exists()).toBe(true)
    expect(modal.attributes('data-width')).toBe('960')
    expect(modal.attributes('data-close-on-backdrop')).toBe('0')
    expect(modal.text()).toContain('复审产物台展示上游澄清需求文档')
    expect(modal.text()).toContain('就地查看上游澄清需求')
    expect(wrapper.find('[data-testid="upstream-modal-readonly-footer"]').text()).toContain(
      '只读对照',
    )
    expect(modal.find('[data-testid="annotate-affordance"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="html-preview"]').attributes('data-inspectable')).toBe('1')

    await wrapper.find('[data-testid="upstream-modal-mode-raw"]').trigger('click')
    await nextTick()
    expect(wrapper.find('.json-code-view').exists()).toBe(true)

    await wrapper.find('[data-testid="modal-backdrop"]').trigger('click')
    await nextTick()
    expect(wrapper.find('[data-testid="upstream-enlarge-modal"]').exists()).toBe(true)

    await wrapper.find('[data-testid="modal-close"]').trigger('click')
    await nextTick()
    expect(wrapper.find('[data-testid="upstream-enlarge-modal"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('lazy-loads requirement content; failed load shows retry without blocking preview', async () => {
    apiMocks.artifactContent.mockImplementation(async (id: string) => {
      if (id === 'a-req') throw new Error('network down')
      return { content: '<html>page</html>' }
    })
    const nodeRun: NodeRun = {
      nodeId: 'visual',
      iteration: 1,
      status: 'completed',
      outputs: { page: '<!doctype html><html><body>frozen-v1</body></html>' },
    }
    const run = runWithArtifacts([
      artifact({ id: 'a-page', name: 'page.html', kind: 'html', nodeId: 'visual' }),
      artifact({ id: 'a-req', name: 'clarified_requirement.json', kind: 'json', nodeId: 'react' }),
    ])
    const wrapper = mountPanel(visualNode(), nodeRun, run, { annotatable: true })
    await flushPromises()
    expect(apiMocks.artifactContent).not.toHaveBeenCalled()
    expect(wrapper.find('[data-testid="html-preview"]').text()).toContain('frozen-v1')

    await wrapper.find('[data-testid="upstream-enlarge"]').trigger('click')
    await flushPromises()
    expect(apiMocks.artifactContent).toHaveBeenCalledWith('a-req')
    expect(wrapper.find('[data-testid="upstream-modal-retry"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="html-preview"]').exists()).toBe(true)

    apiMocks.artifactContent.mockImplementation(async (id: string) => {
      if (id === 'a-req') return { content: JSON.stringify(REQ_DOC) }
      return { content: '<html>page</html>' }
    })
    await wrapper.find('[data-testid="upstream-modal-retry"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="upstream-enlarge-modal"]').text()).toContain(
      '复审产物台展示上游澄清需求文档',
    )
    wrapper.unmount()
  })

  it('hides bar and closes modal when switching main product to clarified_requirement.json', async () => {
    apiMocks.artifactContent.mockImplementation(async (id: string) => {
      if (id === 'a-req') return { content: JSON.stringify(REQ_DOC) }
      return { content: '<html>page</html>' }
    })
    const visualRun: NodeRun = {
      nodeId: 'visual',
      iteration: 1,
      status: 'completed',
      outputs: { page: '<!doctype html><html><body>frozen-v1</body></html>' },
    }
    const reactRun: NodeRun = {
      nodeId: 'react',
      iteration: 1,
      status: 'completed',
      outputs: { clarified_requirement_json: JSON.stringify(REQ_DOC) },
    }
    const run = runWithArtifacts([
      artifact({ id: 'a-page', name: 'page.html', kind: 'html', nodeId: 'visual' }),
      artifact({ id: 'a-req', name: 'clarified_requirement.json', kind: 'json', nodeId: 'react' }),
    ])
    const wrapper = mountPanel(visualNode(), visualRun, run, { annotatable: true })
    await flushPromises()
    await wrapper.find('[data-testid="upstream-enlarge"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="upstream-enlarge-modal"]').exists()).toBe(true)

    await wrapper.setProps({ node: reactNode(), nodeRun: reactRun })
    await flushPromises()
    expect(wrapper.find('[data-testid="upstream-context"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="upstream-enlarge-modal"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="structured-product-name"]').text()).toBe(
      'clarified_requirement.json',
    )

    await wrapper.setProps({ node: visualNode(), nodeRun: visualRun })
    await flushPromises()
    expect(wrapper.find('[data-testid="upstream-context"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="upstream-enlarge-modal"]').exists()).toBe(false)
    wrapper.unmount()
  })
})
