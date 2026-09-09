// @vitest-environment happy-dom
import { computed, nextTick, ref } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { AcpEvent, Run } from '@/lib/shared/types'

const mocks = vi.hoisted(() => ({
  runEventsWsUrl: vi.fn((id: string) => `ws://test/${id}`),
}))

vi.mock('@/lib/api/api', async () => {
  const actual = await vi.importActual<typeof import('@/lib/api/api')>('@/lib/api/api')
  return { ...actual, api: { ...actual.api, runEventsWsUrl: mocks.runEventsWsUrl } }
})

import { useRunDetailWs } from './useRunDetailWs'

const rails: AcpEvent[] = [
  { kind: 'thought', text: 'thinking', t: 1 },
  { kind: 'message', text: 'answer', t: 2 },
]

class MockWebSocket {
  static instances: MockWebSocket[] = []
  static throwNext = false
  onopen: ((event: Event) => void) | null = null
  onclose: ((event: CloseEvent) => void) | null = null
  onmessage: ((event: MessageEvent) => void) | null = null
  close = vi.fn(() => this.onclose?.(new CloseEvent('close')))

  constructor(public url: string) {
    if (MockWebSocket.throwNext) {
      MockWebSocket.throwNext = false
      throw new Error('socket construction failed')
    }
    MockWebSocket.instances.push(this)
  }

  open() {
    this.onopen?.(new Event('open'))
  }

  message(value: unknown) {
    const data = typeof value === 'string' ? value : JSON.stringify(value)
    this.onmessage?.(new MessageEvent('message', { data }))
  }

  remoteClose() {
    this.onclose?.(new CloseEvent('close'))
  }
}

function harness(over: {
  status?: Run['status']
  runId?: string
  selected?: string | null
  manual?: boolean
  nodeTab?: string
  sessions?: Record<string, any>
  gateNode?: string
  clarifyNode?: string | null
  eventPages?: Record<string, any>
  clarifyBusy?: boolean
} = {}) {
  const run = ref({
    id: over.runId ?? 'run-1',
    status: over.status ?? 'running',
    reactSessions: over.sessions ?? {},
    gate: over.gateNode ? { reactUpstreamNodeId: over.gateNode, nodeId: 'gate' } : undefined,
    nodeRuns: {},
  } as unknown as Run)
  const selected = ref<string | null>(over.selected === undefined ? 'n1' : over.selected)
  const manual = ref(over.manual ?? false)
  const liveBusy: Record<string, boolean> = {}
  const liveNode = ref<string | null>(null)
  const eventPages: Record<string, any> = over.eventPages ?? {}
  const nodeTab = ref(over.nodeTab ?? 'output')
  const review = {
    applyReviewFrame: vi.fn((): boolean | void => true),
    applyAcpEvents: vi.fn((): boolean | void => true),
    isSessionBusy: vi.fn(() => false),
  }
  const gate = {
    applyReviewFrame: vi.fn(),
    applyAcpEvents: vi.fn((): boolean | void => true),
  }
  const reviewChatRef = ref<any>(review)
  const gateApprovalRef = ref<any>(gate)
  const fetchNodeEvents = vi.fn(async () => true)
  const mergeLiveWsAcpPage = vi.fn()
  const rehydrateByNode: Record<string, string> = {}
  const rehydrateNodeEvents = vi.fn(async () => undefined)
  const fetchSandboxLog = vi.fn()
  const maybePollSandboxForBoot = vi.fn()
  const loadRun = vi.fn()
  const refreshArtifactPreview = vi.fn()
  const clarifyNode = ref(over.clarifyNode === undefined ? 'n1' : over.clarifyNode)
  const api = useRunDetailWs({
    runId: computed(() => over.runId ?? 'run-1'),
    run,
    selected,
    manual,
    liveBusy,
    liveNode,
    eventPages,
    nodeTab,
    reviewChatRef,
    gateApprovalRef,
    selClarify: computed(() => (clarifyNode.value ? { nodeId: clarifyNode.value } : null)),
    fetchNodeEvents,
    mergeLiveWsAcpPage,
    rehydrateByNode,
    rehydrateNodeEvents,
    fetchSandboxLog,
    maybePollSandboxForBoot,
    isClarifySessionBusy: () => over.clarifyBusy ?? false,
    loadRun,
    refreshArtifactPreview,
  })
  return {
    api,
    run,
    selected,
    manual,
    liveBusy,
    liveNode,
    eventPages,
    nodeTab,
    review,
    gate,
    reviewChatRef,
    gateApprovalRef,
    fetchNodeEvents,
    mergeLiveWsAcpPage,
    rehydrateByNode,
    rehydrateNodeEvents,
    fetchSandboxLog,
    maybePollSandboxForBoot,
    loadRun,
    refreshArtifactPreview,
    clarifyNode,
  }
}

describe('useRunDetailWs coverage', () => {
  beforeEach(() => {
    MockWebSocket.instances = []
    MockWebSocket.throwNext = false
    mocks.runEventsWsUrl.mockClear()
    vi.stubGlobal('WebSocket', MockWebSocket)
  })

  afterEach(() => {
    vi.useRealTimers()
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  })

  it('restores valid clarify/gate sessions and seeds rails from cache or fetch', async () => {
    const h = harness({
      sessions: {
        n1: { busy: true, waiting: 2, items: ['x'], activeItem: 'x' },
        gateNode: { busy: false },
        bad: null,
      },
      gateNode: 'gateNode',
      eventPages: { n1: { events: rails } },
    })
    await h.api.projectDialogueAfterLoad({ reactSessions: h.run.value.reactSessions })
    expect(h.liveBusy.n1).toBe(true)
    expect(h.review.applyReviewFrame).toHaveBeenCalledWith(
      expect.objectContaining({ event: 'queue_state', nodeId: 'n1', waiting: 2 }),
    )
    expect(h.gate.applyReviewFrame).toHaveBeenCalledWith(
      expect.objectContaining({ nodeId: 'gateNode' }),
    )
    expect(h.review.applyAcpEvents).toHaveBeenCalledWith(rails, 'n1')

    h.eventPages.fetched = { events: [] }
    h.fetchNodeEvents.mockImplementationOnce(async (id) => {
      h.eventPages[id!] = { events: rails }
      return true
    })
    h.clarifyNode.value = 'fetched'
    await h.api.seedDialogueAcpAfterRestore(['fetched'])
    expect(h.fetchNodeEvents).toHaveBeenCalledWith('fetched')
    expect(h.review.applyAcpEvents).toHaveBeenCalledWith(rails, 'fetched')

    h.api.teardownRealtime()
  })

  it('handles empty, non-rail, failed, and unready seed paths', async () => {
    const h = harness({ eventPages: { empty: { events: [] }, meta: { events: [{ kind: 'tool', t: 1 }] } } })
    h.fetchNodeEvents.mockRejectedValueOnce(new Error('offline'))
    await h.api.seedDialogueAcpAfterRestore(['empty'])
    h.liveBusy.meta = false
    await h.api.seedDialogueAcpAfterRestore(['meta'])

    h.eventPages.n1 = { events: rails }
    h.review.applyAcpEvents.mockReturnValue(false)
    await h.api.seedDialogueAcpAfterRestore(['n1'])
    expect(h.api.pendingDialogueAcp.size).toBe(1)

    h.reviewChatRef.value = null
    h.gateApprovalRef.value = null
    h.api.applyOrBufferDialogueAcp('n1', rails, true)
    expect(h.api.pendingDialogueAcp.size).toBe(1)
    h.reviewChatRef.value = h.review
    h.review.applyAcpEvents.mockReturnValue(true)
    h.api.applyOrBufferDialogueAcp('other', rails)
    h.api.pendingDialogueAcp.push({ nodeId: undefined, events: rails })
    h.api.pendingDialogueAcp.push({ nodeId: 'n1', events: rails })

    // A subsequent restore flushes anonymous/matching frames and retains unmatched ones.
    await h.api.projectDialogueAfterLoad({ reactSessions: {} })
    expect(h.review.applyAcpEvents).toHaveBeenCalled()
    expect(h.api.pendingDialogueAcp.size).toBeGreaterThanOrEqual(1)
    h.api.resetDialogueState()
    expect(h.api.pendingDialogueAcp.size).toBe(0)
  })

  it('connects, reconnects with backoff, ignores stale sockets, and parses snapshots', async () => {
    vi.useFakeTimers()
    vi.spyOn(Math, 'random').mockReturnValue(0)
    const h = harness({
      sessions: { n1: { busy: false } },
      eventPages: { n1: { events: rails } },
    })
    h.api.connectWs()
    const first = MockWebSocket.instances[0]!
    expect(first.url).toBe('ws://test/run-1')
    first.open()
    expect(h.api.wsConnected).toBe(true)
    first.message('not json')
    first.message({ type: 'snapshot', run: { reactSessions: { n1: { busy: false } } } })
    await nextTick()
    expect(h.run.value.reactSessions).toEqual({ n1: { busy: false } })

    first.remoteClose()
    expect(h.api.wsConnected).toBe(false)
    await vi.advanceTimersByTimeAsync(800)
    const second = MockWebSocket.instances[1]!
    second.open()
    await nextTick()
    expect(h.api.wsConnected).toBe(true)
    // Events from the stale first socket are ignored.
    first.message({ type: 'status' })
    expect(h.loadRun).not.toHaveBeenCalled()

    // Explicit reconnect closes the current socket and creates a replacement.
    h.api.connectWs({ fromReconnect: true })
    const third = MockWebSocket.instances[2]!
    expect(second.close).toHaveBeenCalled()
    third.open()
    await nextTick()

    h.api.teardownRealtime()
    third.remoteClose()
    await vi.advanceTimersByTimeAsync(1000)
    expect(MockWebSocket.instances).toHaveLength(3)
  })

  it('recovers when construction throws, but does not reconnect terminal or empty runs', async () => {
    vi.useFakeTimers()
    vi.spyOn(Math, 'random').mockReturnValue(0)
    MockWebSocket.throwNext = true
    const running = harness()
    running.api.connectWs()
    await vi.advanceTimersByTimeAsync(800)
    expect(MockWebSocket.instances).toHaveLength(1)
    running.api.teardownRealtime()

    const terminal = harness({ status: 'completed' })
    terminal.api.connectWs()
    MockWebSocket.instances.at(-1)!.remoteClose()
    await vi.advanceTimersByTimeAsync(2000)
    expect(MockWebSocket.instances).toHaveLength(2)
    terminal.api.teardownRealtime()

    const empty = harness({ runId: '' })
    empty.api.connectWs()
    expect(mocks.runEventsWsUrl).not.toHaveBeenCalledWith('')
  })

  it('dispatches ACP and review frames to live and dialogue state', async () => {
    const h = harness({ gateNode: 'n1' })
    h.api.connectWs()
    const socket = MockWebSocket.instances[0]!
    socket.open()

    socket.message({ type: 'acp', nodeId: 'n1', events: rails, busy: true })
    expect(h.mergeLiveWsAcpPage).toHaveBeenCalledWith('n1', rails)
    expect(h.liveBusy.n1).toBe(true)
    expect(h.liveNode.value).toBe('n1')
    expect(h.selected.value).toBe('n1')
    expect(h.review.applyAcpEvents).toHaveBeenCalled()
    expect(h.gate.applyAcpEvents).toHaveBeenCalled()

    h.manual.value = true
    h.selected.value = 'old'
    socket.message({ type: 'acp', nodeId: 'n2', events: [], busy: false })
    expect(h.selected.value).toBe('old')
    expect(h.liveBusy.n2).toBe(false)

    socket.message({ type: 'review', nodeId: 'n1', event: 'turn_begin' })
    expect(h.liveBusy.n1).toBe(true)
    socket.message({ type: 'review', nodeId: 'n1', event: 'queue_state', busy: false })
    expect(h.liveBusy.n1).toBe(false)
    socket.message({ type: 'review', nodeId: 'n1', event: 'turn_done' })
    socket.message({ type: 'review', nodeId: 'n1', event: 'error' })
    expect(h.loadRun).toHaveBeenCalledTimes(2)
    expect(h.review.applyReviewFrame).toHaveBeenCalled()
    expect(h.gate.applyReviewFrame).toHaveBeenCalled()

    h.clarifyNode.value = 'different'
    h.review.applyReviewFrame.mockClear()
    socket.message({ type: 'review', nodeId: 'n1', event: 'noop' })
    expect(h.review.applyReviewFrame).not.toHaveBeenCalled()
    h.clarifyNode.value = null
    socket.message({ type: 'review', nodeId: 'n1', event: 'noop' })
    expect(h.review.applyReviewFrame).toHaveBeenCalled()

    h.api.teardownRealtime()
  })

  it('dispatches refresh frames and protects a busy clarify session', () => {
    const normal = harness()
    normal.api.connectWs()
    const socket = MockWebSocket.instances[0]!
    socket.open()
    for (const type of ['trace', 'react', 'artifact_edit'] as const) socket.message({ type })
    socket.message({ type: 'status' })
    expect(normal.loadRun).toHaveBeenCalledTimes(4)
    expect(normal.liveNode.value).toBeNull()
    normal.api.teardownRealtime()

    const busy = harness({ clarifyBusy: true })
    busy.api.connectWs()
    const busySocket = MockWebSocket.instances.at(-1)!
    busySocket.message({ type: 'artifact_edit', previewArtifact: 'preview.png' })
    busySocket.message({ type: 'status' })
    expect(busy.refreshArtifactPreview).toHaveBeenCalledWith(
      expect.objectContaining({ previewArtifact: 'preview.png' }),
    )
    expect(busy.loadRun).not.toHaveBeenCalled()
    busy.api.teardownRealtime()
  })

  it('initializes selection and covers every polling state', async () => {
    vi.useFakeTimers()
    const h = harness({ selected: null, nodeTab: 'sandbox' })
    const deep = vi.fn(() => true)
    const output = vi.fn(() => false)
    const sync = vi.fn()
    await h.api.initAfterLoadSuccess({
      applyDetailArtifactsDeepLink: deep,
      applyOutputDeepLinkFocus: output,
      defaultNode: 'n1',
      syncAllMcpCallsFromRun: sync,
    })
    expect(h.selected.value).toBe('n1')
    expect(sync).toHaveBeenCalled()
    expect(h.rehydrateNodeEvents).toHaveBeenCalledWith('n1')
    expect(h.fetchSandboxLog).toHaveBeenCalledWith('n1')
    MockWebSocket.instances[0]!.open()
    h.fetchSandboxLog.mockClear()

    h.rehydrateByNode.n1 = 'ready'
    await vi.advanceTimersByTimeAsync(2000)
    expect(h.loadRun).toHaveBeenCalledWith(false)
    expect(h.fetchNodeEvents).toHaveBeenCalledWith('n1')
    expect(h.fetchSandboxLog).toHaveBeenCalledWith('n1', { intent: 'silent_poll' })
    expect(h.maybePollSandboxForBoot).toHaveBeenCalled()

    h.rehydrateByNode.n1 = 'idle'
    await vi.advanceTimersByTimeAsync(2000)
    expect(h.rehydrateNodeEvents).toHaveBeenCalledWith('n1')
    h.rehydrateByNode.n1 = 'loading'
    h.fetchNodeEvents.mockClear()
    await vi.advanceTimersByTimeAsync(2000)
    expect(h.fetchNodeEvents).not.toHaveBeenCalled()
    h.rehydrateByNode.n1 = 'error'
    await vi.advanceTimersByTimeAsync(2000)

    // Displayable live events suppress the REST event poll.
    h.eventPages.n1 = { live: true, events: rails }
    h.liveNode.value = 'n1'
    h.fetchNodeEvents.mockClear()
    h.rehydrateByNode.n1 = 'ready'
    await vi.advanceTimersByTimeAsync(2000)
    expect(h.fetchNodeEvents).not.toHaveBeenCalled()

    h.run.value = { ...h.run.value, status: 'completed' } as Run
    h.loadRun.mockClear()
    await vi.advanceTimersByTimeAsync(2000)
    expect(h.loadRun).not.toHaveBeenCalled()
    h.api.teardownRealtime()
  })

  it('does not overwrite focused selection and skips full polling while clarify is busy', async () => {
    vi.useFakeTimers()
    const focused = harness({ selected: 'kept', clarifyBusy: true })
    await focused.api.initAfterLoadSuccess({
      applyDetailArtifactsDeepLink: () => false,
      applyOutputDeepLinkFocus: () => true,
      defaultNode: 'default',
      syncAllMcpCallsFromRun: vi.fn(),
    })
    expect(focused.selected.value).toBe('kept')
    await vi.advanceTimersByTimeAsync(2000)
    expect(focused.loadRun).not.toHaveBeenCalled()
    focused.api.teardownRealtime()
  })
})
