// @vitest-environment happy-dom
import { defineComponent } from 'vue'
import { createI18n } from 'vue-i18n'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'
import type { NodeRun, Run, WFNode } from '@/lib/types'
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
  template: '<div data-testid="structured-view">{{ name }}</div>',
})

const HtmlPreviewStub = defineComponent({
  name: 'HtmlPreview',
  props: { html: String },
  template: '<div data-testid="html-preview">{{ html }}</div>',
})

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
      },
    },
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
})
