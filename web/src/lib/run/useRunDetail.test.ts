// @vitest-environment happy-dom
import { createApp, defineComponent, nextTick, ref } from 'vue'
import { createI18n } from 'vue-i18n'
import { createMemoryHistory, createRouter } from 'vue-router'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises } from '@vue/test-utils'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'
import type { Run, WFNode, Workflow } from '@/lib/shared/types'

const isMobile = ref(false)

const mocks = vi.hoisted(() => ({
  toastWarn: vi.fn(),
  toastSuccess: vi.fn(),
  toastError: vi.fn(),
  getRun: vi.fn(),
  getWorkflow: vi.fn(),
  getProject: vi.fn(),
  runArtifacts: vi.fn(),
  nodeEvents: vi.fn(),
  nodeSandboxLog: vi.fn(),
  getRunNodeSandbox: vi.fn(),
  updateRunPriority: vi.fn(),
}))

vi.mock('@/lib/composables/useBreakpoint', () => ({
  useBreakpoint: () => ({ isMobile }),
}))

vi.mock('@/lib/composables/useToast', () => ({
  useToast: () => ({
    warn: mocks.toastWarn,
    success: mocks.toastSuccess,
    error: mocks.toastError,
    info: vi.fn(),
  }),
}))

vi.mock('@/lib/api/api', async () => {
  const actual = await vi.importActual<typeof import('@/lib/api/api')>('@/lib/api/api')
  return {
    ...actual,
    api: {
      ...actual.api,
      getRun: mocks.getRun,
      getWorkflow: mocks.getWorkflow,
      getProject: mocks.getProject,
      runArtifacts: mocks.runArtifacts,
      nodeEvents: mocks.nodeEvents,
      nodeSandboxLog: mocks.nodeSandboxLog,
      getRunNodeSandbox: mocks.getRunNodeSandbox,
      updateRunPriority: mocks.updateRunPriority,
    },
  }
})

import { useRunDetail } from './useRunDetail'

function stubNode(partial: Partial<WFNode> & Pick<WFNode, 'id' | 'type'>): WFNode {
  return { label: partial.id, position: { x: 0, y: 0 }, config: {}, ...partial }
}

const sampleNodes = [
  stubNode({ id: 'n1', type: 'agent' }),
  stubNode({ id: 'n2', type: 'output' }),
]

const sampleRun = (): Run =>
  ({
    id: 'run-1',
    workflowId: 'wf-1',
    workflowName: '夜间回归',
    status: 'running',
    trigger: 'manual',
    startedAt: new Date().toISOString(),
    durationSec: 0,
    progress: 0.5,
    priority: 'normal',
    nodeRuns: {
      n1: { nodeId: 'n1', status: 'running', outputs: {}, events: [], mcpCalls: [] },
      n2: { nodeId: 'n2', status: 'pending', outputs: {} },
    },
    artifacts: [{ id: 'art-1', name: 'out.txt', nodeId: 'n2' }],
    trace: [],
    vars: [{ name: 'branch', value: 'main' }],
    nodes: sampleNodes,
    edges: [{ id: 'e1', source: 'n1', target: 'n2' }],
    reactSessions: {},
  }) as unknown as Run

const sampleWorkflow = (): Workflow => ({
  id: 'wf-1',
  projectId: 'proj-1',
  name: '夜间回归',
  description: '',
  status: 'published',
  version: 1,
  updatedAt: '',
  needsRepo: false,
  nodes: sampleNodes,
  edges: [{ id: 'e1', source: 'n1', target: 'n2' }],
})

class MockWebSocket {
  static CONNECTING = 0
  static OPEN = 1
  static CLOSING = 2
  static CLOSED = 3
  readyState = MockWebSocket.OPEN
  onopen: ((ev: Event) => void) | null = null
  onclose: ((ev: CloseEvent) => void) | null = null
  onmessage: ((ev: MessageEvent) => void) | null = null
  onerror: ((ev: Event) => void) | null = null
  constructor(_url: string) {
    queueMicrotask(() => this.onopen?.(new Event('open')))
  }
  close() {
    this.readyState = MockWebSocket.CLOSED
    this.onclose?.(new CloseEvent('close'))
  }
  send(_data: string) {}
}

class MockResizeObserver {
  observe = vi.fn()
  disconnect = vi.fn()
  unobserve = vi.fn()
  constructor(_cb: ResizeObserverCallback) {}
}

async function withRunDetail(routePath = '/runs/run-1') {
  let detail!: ReturnType<typeof useRunDetail>
  const i18n = createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...common, ...pages } },
  })
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/', component: { template: '<div />' } },
      { path: '/runs/:id', component: { template: '<div />' } },
    ],
  })
  await router.push(routePath)
  await router.isReady()
  const Comp = defineComponent({
    setup() {
      detail = useRunDetail()
      return () => null
    },
  })
  const app = createApp(Comp)
  app.use(i18n)
  app.use(router)
  app.mount(document.createElement('div'))
  return { detail, app, router }
}

describe('useRunDetail', () => {
  beforeEach(() => {
    vi.stubGlobal('WebSocket', MockWebSocket)
    vi.stubGlobal('ResizeObserver', MockResizeObserver)
    vi.stubGlobal('requestAnimationFrame', (cb: FrameRequestCallback) => {
      cb(0)
      return 1
    })
    isMobile.value = false
    mocks.getRun.mockReset()
    mocks.getWorkflow.mockReset()
    mocks.getProject.mockReset()
    mocks.runArtifacts.mockReset()
    mocks.nodeEvents.mockReset()
    mocks.nodeSandboxLog.mockReset()
    mocks.getRunNodeSandbox.mockReset()
    mocks.updateRunPriority.mockReset()
    mocks.toastWarn.mockReset()
    mocks.toastSuccess.mockReset()
    mocks.toastError.mockReset()

    mocks.getRun.mockResolvedValue(sampleRun())
    mocks.getWorkflow.mockResolvedValue(sampleWorkflow())
    mocks.getProject.mockResolvedValue({ id: 'proj-1', unknownModelDisplayName: '未知模型' })
    mocks.runArtifacts.mockResolvedValue([])
    mocks.nodeEvents.mockResolvedValue({ events: [], nextCursor: '', hasMore: false })
    mocks.nodeSandboxLog.mockResolvedValue({ content: 'boot log', live: false, found: true })
    mocks.getRunNodeSandbox.mockResolvedValue({ id: 'sbx-1' })
    mocks.updateRunPriority.mockResolvedValue({ priority: 'high' })
  })

  afterEach(() => {
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  })

  it('mounts orchestration, loads run, and exposes core state without hanging', async () => {
    const { detail, app } = await withRunDetail()
    await flushPromises()
    await nextTick()

    expect(detail.runId.value).toBe('run-1')
    expect(detail.run.value.id).toBe('run-1')
    expect(detail.wf.value.nodes.length).toBeGreaterThan(0)
    expect(detail.runLoading.value).toBe(false)
    expect(detail.elapsedSec.value).toBeGreaterThanOrEqual(0)
    expect(detail.progressFrac.value).toBeGreaterThan(0)
    expect(detail.nodeTabs.value.length).toBeGreaterThan(0)

    detail.selectNode('n1')
    expect(detail.selected.value).toBe('n1')
    detail.selectExecution('n1', 0)
    detail.onAppPreviewStagedPick(null)
    detail.onArtifactDeleted('art-1')
    detail.openCancelConfirm()
    detail.closeCancelConfirm()
    detail.openDeleteConfirm()
    detail.closeDeleteConfirm()
    detail.goSandboxLogTab()
    detail.mergeStagedAppPreviewPick([])
    expect(detail.classifyRunLoadError(new Error('404 not found'))).toBe('not_found')

    detail.togglePriorityPopover()
    await nextTick()
    detail.closePriorityPopover(true)

    document.dispatchEvent(new Event('visibilitychange'))
    window.dispatchEvent(new Event('focus'))
    window.dispatchEvent(new Event('resize'))

    app.unmount()
    await flushPromises()
  })

  it('normalizes mobile view mode and timeline selection', async () => {
    isMobile.value = true
    const { detail, app } = await withRunDetail()
    await flushPromises()
    await nextTick()

    expect(detail.viewMode.value).toBe('timeline')
    detail.showMobileDetailPanel()
    detail.backToMobileTimeline()
    expect(detail.mobileMainPanel.value).toBe('timeline')

    app.unmount()
  })
})
