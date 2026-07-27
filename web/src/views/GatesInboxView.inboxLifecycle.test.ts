// @vitest-environment happy-dom
import { defineComponent, nextTick } from 'vue'
import { createI18n } from 'vue-i18n'
import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'
import type { InboxItem } from '@/lib/types'

const mocks = vi.hoisted(() => ({
  listGates: vi.fn(),
  inboxContext: vi.fn(),
  resumeGate: vi.fn(),
  reactReply: vi.fn(),
  runEventsWsUrl: vi.fn((id: string) => `ws://test/runs/${id}/events`),
  toastError: vi.fn(),
  toastSuccess: vi.fn(),
}))

vi.mock('@/lib/api', async () => {
  const actual = await vi.importActual<typeof import('@/lib/api')>('@/lib/api')
  return {
    ...actual,
    api: {
      ...actual.api,
      listGates: mocks.listGates,
      inboxContext: mocks.inboxContext,
      resumeGate: mocks.resumeGate,
      reactReply: mocks.reactReply,
      runEventsWsUrl: mocks.runEventsWsUrl,
    },
  }
})

vi.mock('vue-router', () => ({
  useRouter: () => ({ push: vi.fn() }),
}))

vi.mock('@/lib/useBreakpoint', async () => {
  const { ref } = await import('vue')
  return {
    useBreakpoint: () => ({ isMobile: ref(false) }),
  }
})

vi.mock('@/lib/useToast', () => ({
  useToast: () => ({
    error: mocks.toastError,
    success: mocks.toastSuccess,
    info: vi.fn(),
    warning: vi.fn(),
  }),
}))

const filterState = vi.hoisted(() => ({
  pipelineSelected: null as { value: string } | null,
  projectSelected: null as { value: string } | null,
}))

vi.mock('@/lib/usePipelineFilter', async () => {
  const { ref } = await import('vue')
  filterState.pipelineSelected = ref('')
  return {
    usePipelineFilter: () => ({ selected: filterState.pipelineSelected! }),
  }
})

vi.mock('@/lib/useProjectContext', async () => {
  const { ref } = await import('vue')
  filterState.projectSelected = ref('')
  return {
    useProjectContext: () => ({
      selected: filterState.projectSelected!,
      ensureHydrated: vi.fn(),
    }),
  }
})

vi.mock('@/lib/useClarifyDraft', async () => {
  const { ref } = await import('vue')
  return {
    useClarifyDraft: () => ({
      draft: ref(''),
      attachments: ref([]),
      annotations: ref([]),
    }),
  }
})

// Use the real usePendingGates singleton (api.listGates is mocked) so peek/force
// races and sidebar totalCount stay covered — do not stub the composable away.

import GatesInboxView from './GatesInboxView.vue'
import { usePendingGates } from '@/lib/usePendingGates'

function paged(items: InboxItem[]) {
  return { items, total: items.length, page: 1, pageSize: 20 }
}

function gateItem(id: string, iteration = 1): InboxItem {
  return {
    type: 'gate',
    runId: `run-${id}`,
    nodeId: `gate-${id}`,
    iteration,
    workflowName: 'wf',
    title: `Gate ${id}`,
    bodyMd: '',
    actions: [{ id: 'approve', label: 'Approve' }],
    requestedAt: new Date().toISOString(),
    status: 'waiting_human' as any,
  } as InboxItem
}

function clarifyItem(id: string, iteration = 1): InboxItem {
  return {
    type: 'clarify',
    runId: `run-${id}`,
    nodeId: `clarify-${id}`,
    iteration,
    workflowName: 'wf',
    label: `Clarify ${id}`,
    done: false,
    requestedAt: new Date().toISOString(),
    updatedAt: new Date().toISOString(),
  }
}

class FakeWebSocket {
  static instances: FakeWebSocket[] = []
  onmessage: ((ev: { data: string }) => void) | null = null
  onclose: (() => void) | null = null
  readyState = 1
  url: string
  constructor(url: string) {
    this.url = url
    FakeWebSocket.instances.push(this)
  }
  close() {
    this.readyState = 3
    this.onclose?.()
  }
  emit(type: string, extra?: Record<string, unknown>) {
    this.onmessage?.({ data: JSON.stringify({ type, ...extra }) })
  }
}

const GateApprovalStub = defineComponent({
  name: 'GateApproval',
  emits: ['resolve', 'react-revised'],
  template: '<button data-testid="resolve-btn" @click="$emit(\'resolve\', \'approve\', {})">resolve</button>',
})

const ReviewComposerStub = defineComponent({
  name: 'ReviewComposer',
  emits: ['send', 'finish'],
  template:
    `<div>
      <button data-testid="clarify-send" @click="$emit('send', 'hi', [], [], true)">finish</button>
      <button data-testid="clarify-turn" @click="$emit('send', 'hi', [], [], false)">turn</button>
    </div>`,
})

/** Hang until aborted via opts.signal; otherwise never resolves. */
function hangInboxContext(_runId: string, _nodeId: string, _iteration?: number, opts?: { signal?: AbortSignal }) {
  const signal = opts?.signal
  if (!signal) return new Promise(() => {})
  if (signal.aborted) return Promise.reject(new DOMException('Aborted', 'AbortError'))
  return new Promise((_resolve, reject) => {
    signal.addEventListener(
      'abort',
      () => reject(new DOMException('Aborted', 'AbortError')),
      { once: true },
    )
  })
}

function mountInbox() {
  const i18n = createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...common, ...pages } },
  })
  return mount(GatesInboxView, {
    global: {
      plugins: [i18n],
      stubs: {
        Icon: true,
        EmptyState: true,
        PipelineFilter: true,
        ProjectFilter: true,
        Pagination: true,
        ArtifactLoadingPane: true,
        GateApproval: GateApprovalStub,
        ReviewShell: defineComponent({
          template: '<div><slot name="stage" /><slot name="sidebar" /></div>',
        }),
        ReviewComposer: ReviewComposerStub,
        ClarifyProductStage: true,
      },
    },
  })
}

function inboxCallsFor(runId: string, nodeId: string, iteration = 1) {
  return mocks.inboxContext.mock.calls.filter(
    (c) => c[0] === runId && c[1] === nodeId && (c[2] ?? 1) === iteration,
  )
}

beforeEach(async () => {
  vi.clearAllMocks()
  FakeWebSocket.instances = []
  vi.stubGlobal('WebSocket', FakeWebSocket as unknown as typeof WebSocket)
  mocks.resumeGate.mockResolvedValue({ status: 'ok' })
  mocks.reactReply.mockResolvedValue({ status: 'ok' })
  if (filterState.pipelineSelected) filterState.pipelineSelected.value = ''
  if (filterState.projectSelected) filterState.projectSelected.value = ''
  // Reset singleton so each test starts from an empty pending badge/list.
  mocks.listGates.mockResolvedValue(paged([]))
  await usePendingGates().refresh({ mode: 'force' })
  mocks.inboxContext.mockImplementation(async (runId: string, nodeId: string) => {
    if (nodeId.startsWith('clarify-')) {
      return {
        type: 'clarify',
        status: 'waiting_human',
        nodes: [{ id: nodeId, type: 'react', label: nodeId }],
        artifacts: [],
        nodeExecutions: {},
        clarify: { nodeId, iteration: 1, turns: [], done: false, label: nodeId },
      }
    }
    return {
      type: 'gate',
      nodes: [],
      artifacts: [],
      nodeExecutions: {},
    }
  })
})

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('GatesInboxView inbox-context lifecycle', () => {
  it('gate resolve: stops requesting processed triple and selects neighbor', async () => {
    const a = gateItem('a')
    const b = gateItem('b')
    const c = gateItem('c')
    let list = [a, b, c]
    mocks.listGates.mockImplementation(async () => paged(list))

    const wrapper = mountInbox()
    await flushPromises()
    await nextTick()

    // Select middle item b
    const buttons = wrapper.findAll('button').filter((btn) => btn.text().includes('Gate b'))
    expect(buttons.length).toBeGreaterThan(0)
    await buttons[0].trigger('click')
    await flushPromises()

    const before = inboxCallsFor('run-b', 'gate-b', 1).length
    expect(before).toBeGreaterThan(0)

    list = [a, c]
    await wrapper.get('[data-testid="resolve-btn"]').trigger('click')
    await flushPromises()
    await nextTick()
    await flushPromises()

    const after = inboxCallsFor('run-b', 'gate-b', 1).length
    expect(after).toBe(before)

    // Neighbor after removing middle b is c
    expect(wrapper.text()).toContain('Gate c')
    expect(inboxCallsFor('run-c', 'gate-c', 1).length).toBeGreaterThan(0)

    // WS status must not re-fetch processed triple
    const ws = FakeWebSocket.instances.find((w) => w.url.includes('run-b'))
    ws?.emit('status')
    await flushPromises()
    expect(inboxCallsFor('run-b', 'gate-b', 1).length).toBe(after)

    wrapper.unmount()
  })

  it('gate resolve on last item: selects previous neighbor', async () => {
    const a = gateItem('a')
    const b = gateItem('b')
    const c = gateItem('c')
    let list = [a, b, c]
    mocks.listGates.mockImplementation(async () => paged(list))

    const wrapper = mountInbox()
    await flushPromises()
    await nextTick()

    const buttons = wrapper.findAll('button').filter((btn) => btn.text().includes('Gate c'))
    expect(buttons.length).toBeGreaterThan(0)
    await buttons[0].trigger('click')
    await flushPromises()

    const before = inboxCallsFor('run-c', 'gate-c', 1).length
    expect(before).toBeGreaterThan(0)

    list = [a, b]
    await wrapper.get('[data-testid="resolve-btn"]').trigger('click')
    await flushPromises()
    await nextTick()
    await flushPromises()

    expect(inboxCallsFor('run-c', 'gate-c', 1).length).toBe(before)
    expect(wrapper.text()).toContain('Gate b')
    expect(inboxCallsFor('run-b', 'gate-b', 1).length).toBeGreaterThan(0)
    wrapper.unmount()
  })

  it('gate resolve on sole item: clears active and never re-fetches', async () => {
    const only = gateItem('solo')
    let list: InboxItem[] = [only]
    mocks.listGates.mockImplementation(async () => paged(list))

    const wrapper = mountInbox()
    await flushPromises()
    await nextTick()

    const before = inboxCallsFor('run-solo', 'gate-solo', 1).length
    expect(before).toBeGreaterThan(0)

    list = []
    await wrapper.get('[data-testid="resolve-btn"]').trigger('click')
    await flushPromises()
    await nextTick()
    await flushPromises()

    expect(inboxCallsFor('run-solo', 'gate-solo', 1).length).toBe(before)
    expect(wrapper.find('[data-testid="resolve-btn"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('clarify force finish: stops requesting processed triple', async () => {
    const a = clarifyItem('a')
    const b = clarifyItem('b')
    let list: InboxItem[] = [a, b]
    mocks.listGates.mockImplementation(async () => paged(list))

    const wrapper = mountInbox()
    await flushPromises()
    await nextTick()

    const before = inboxCallsFor('run-a', 'clarify-a', 1).length
    expect(before).toBeGreaterThan(0)

    list = [b]
    await wrapper.get('[data-testid="clarify-send"]').trigger('click')
    await flushPromises()
    await nextTick()
    await flushPromises()

    expect(mocks.reactReply).toHaveBeenCalled()
    expect(inboxCallsFor('run-a', 'clarify-a', 1).length).toBe(before)
    expect(inboxCallsFor('run-b', 'clarify-b', 1).length).toBeGreaterThan(0)
    expect(mocks.toastError).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('left-pending 404 converges without toast and without retry loop', async () => {
    const a = gateItem('a')
    const b = gateItem('b')
    mocks.listGates.mockResolvedValue(paged([a, b]))
    mocks.inboxContext
      .mockResolvedValueOnce({
        type: 'gate',
        nodes: [],
        artifacts: [],
        nodeExecutions: {},
      })
      .mockRejectedValueOnce(new Error('no pending inbox item'))

    const wrapper = mountInbox()
    await flushPromises()
    await nextTick()

    // Trigger soft refresh via artifact_edit after initial load
    const ws = FakeWebSocket.instances[FakeWebSocket.instances.length - 1]
    expect(ws).toBeTruthy()
    ws.emit('artifact_edit')
    await flushPromises()
    await nextTick()
    await flushPromises()

    expect(mocks.toastError).not.toHaveBeenCalled()
    // Ghost row removed; active moves to neighbor b
    expect(wrapper.text()).not.toContain('Gate a')
    expect(wrapper.text()).toContain('Gate b')
    const callsForA = inboxCallsFor('run-a', 'gate-a', 1).length
    // Irrelevant status must not re-fetch the processed triple
    ws.emit('status')
    await flushPromises()
    expect(inboxCallsFor('run-a', 'gate-a', 1).length).toBe(callsForA)
    wrapper.unmount()
  })

  it('gate resolve with list lag: drops ghost and selects neighbor without re-fetch', async () => {
    const a = gateItem('a')
    const b = gateItem('b')
    const c = gateItem('c')
    // List lags: still returns a after resolve
    mocks.listGates.mockResolvedValue(paged([a, b, c]))

    const wrapper = mountInbox()
    await flushPromises()
    await nextTick()

    const before = inboxCallsFor('run-a', 'gate-a', 1).length
    expect(before).toBeGreaterThan(0)

    await wrapper.get('[data-testid="resolve-btn"]').trigger('click')
    await flushPromises()
    await nextTick()
    await flushPromises()

    expect(inboxCallsFor('run-a', 'gate-a', 1).length).toBe(before)
    expect(wrapper.text()).not.toContain('Gate a')
    expect(wrapper.text()).toContain('Gate b')
    expect(inboxCallsFor('run-b', 'gate-b', 1).length).toBeGreaterThan(0)
    wrapper.unmount()
  })

  it('manual refresh after external remove: selects neighbor and skips old triple', async () => {
    const a = gateItem('a')
    const b = gateItem('b')
    const c = gateItem('c')
    let list = [a, b, c]
    mocks.listGates.mockImplementation(async () => paged(list))

    const wrapper = mountInbox()
    await flushPromises()
    await nextTick()

    const buttons = wrapper.findAll('button').filter((btn) => btn.text().includes('Gate b'))
    expect(buttons.length).toBeGreaterThan(0)
    await buttons[0].trigger('click')
    await flushPromises()

    const before = inboxCallsFor('run-b', 'gate-b', 1).length
    expect(before).toBeGreaterThan(0)

    // External removal (peek/apply path via manual refresh → loadList + syncActiveAfterApply)
    list = [a, c]
    const refreshBtn = wrapper.findAll('button').find((btn) => btn.text().includes('刷新'))
    expect(refreshBtn).toBeTruthy()
    await refreshBtn!.trigger('click')
    await flushPromises()
    await nextTick()
    await flushPromises()

    expect(inboxCallsFor('run-b', 'gate-b', 1).length).toBe(before)
    expect(wrapper.text()).toContain('Gate c')
    expect(inboxCallsFor('run-c', 'gate-c', 1).length).toBeGreaterThan(0)
    wrapper.unmount()
  })

  it('processing intent before resume resolves: WS status does not fetch processed triple', async () => {
    const a = gateItem('a')
    const b = gateItem('b')
    let list = [a, b]
    mocks.listGates.mockImplementation(async () => paged(list))

    let releaseResume!: (v: unknown) => void
    mocks.resumeGate.mockImplementation(
      () =>
        new Promise((resolve) => {
          releaseResume = resolve
        }),
    )

    const wrapper = mountInbox()
    await flushPromises()
    await nextTick()

    const before = inboxCallsFor('run-a', 'gate-a', 1).length
    expect(before).toBeGreaterThan(0)
    const ws = FakeWebSocket.instances.find((w) => w.url.includes('run-a'))
    expect(ws).toBeTruthy()

    // Click resolve but keep resumeGate pending — intent already short-circuits.
    const resolveClick = wrapper.get('[data-testid="resolve-btn"]').trigger('click')
    await flushPromises()

    expect(ws!.readyState).toBe(3)
    ws!.emit('status')
    await flushPromises()
    expect(inboxCallsFor('run-a', 'gate-a', 1).length).toBe(before)

    list = [b]
    releaseResume({ status: 'ok' })
    await resolveClick
    await flushPromises()
    await nextTick()
    await flushPromises()

    expect(inboxCallsFor('run-a', 'gate-a', 1).length).toBe(before)
    expect(wrapper.text()).toContain('Gate b')
    wrapper.unmount()
  })

  it('aborts in-flight inbox-context on processing intent and does not retry', async () => {
    const a = gateItem('a')
    const b = gateItem('b')
    let list = [a, b]
    mocks.listGates.mockImplementation(async () => paged(list))

    let loadCount = 0
    mocks.inboxContext.mockImplementation(async (runId, nodeId, iteration, opts) => {
      loadCount += 1
      // First call completes so GateApproval mounts; second (softRefresh) hangs.
      if (loadCount >= 2 && runId === 'run-a') {
        return hangInboxContext(runId, nodeId, iteration, opts)
      }
      return {
        type: 'gate',
        nodes: [],
        artifacts: [],
        nodeExecutions: {},
      }
    })

    const wrapper = mountInbox()
    await flushPromises()
    await nextTick()

    expect(wrapper.find('[data-testid="resolve-btn"]').exists()).toBe(true)
    const ws = FakeWebSocket.instances.find((w) => w.url.includes('run-a'))
    expect(ws).toBeTruthy()
    ws!.emit('artifact_edit')
    await flushPromises()

    expect(loadCount).toBeGreaterThanOrEqual(2)
    const hangingCall = [...mocks.inboxContext.mock.calls]
      .reverse()
      .find((c) => c[0] === 'run-a' && c[3]?.signal && !c[3].signal.aborted)
    expect(hangingCall?.[3]?.signal).toBeInstanceOf(AbortSignal)
    const hangingSignal = hangingCall![3]!.signal!
    const beforeResolve = inboxCallsFor('run-a', 'gate-a', 1).length

    list = [b]
    await wrapper.get('[data-testid="resolve-btn"]').trigger('click')
    await flushPromises()
    await nextTick()
    await flushPromises()

    expect(hangingSignal.aborted).toBe(true)
    // Aborted request must not auto-retry the processed triple.
    expect(inboxCallsFor('run-a', 'gate-a', 1).length).toBe(beforeResolve)
    expect(mocks.toastError).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('softRefresh single-flight: concurrent artifact_edit does not fan out requests', async () => {
    const a = gateItem('a')
    mocks.listGates.mockResolvedValue(paged([a]))

    let releaseFirst!: (v: unknown) => void
    let calls = 0
    mocks.inboxContext.mockImplementation(async (_runId, _nodeId, _iteration, opts) => {
      calls += 1
      if (calls === 1) {
        return new Promise((resolve) => {
          releaseFirst = resolve
          opts?.signal?.addEventListener(
            'abort',
            () => resolve(Promise.reject(new DOMException('Aborted', 'AbortError'))),
            { once: true },
          )
        }).catch((e) => {
          throw e
        })
      }
      return hangInboxContext(_runId, _nodeId, _iteration, opts)
    })

    const wrapper = mountInbox()
    await flushPromises()
    await nextTick()
    expect(calls).toBe(1)

    const ws = FakeWebSocket.instances[FakeWebSocket.instances.length - 1]
    expect(ws).toBeTruthy()
    // status/trace must not softRefresh; only artifact_edit starts the in-flight refresh.
    ws.emit('status')
    ws.emit('trace')
    await flushPromises()
    expect(calls).toBe(1)

    ws.emit('artifact_edit')
    ws.emit('artifact_edit')
    ws.emit('status')
    await flushPromises()

    // Single-flight drops duplicates while the first request is in flight.
    expect(calls).toBe(1)

    releaseFirst({
      type: 'gate',
      nodes: [],
      artifacts: [],
      nodeExecutions: {},
    })
    await flushPromises()
    wrapper.unmount()
  })

  it('gate: irrelevant status/trace/react do not softRefresh; artifact_edit and turn_done do', async () => {
    const a = gateItem('a')
    mocks.listGates.mockResolvedValue(paged([a]))

    const wrapper = mountInbox()
    await flushPromises()
    await nextTick()

    const before = inboxCallsFor('run-a', 'gate-a', 1).length
    expect(before).toBeGreaterThan(0)
    const ws = FakeWebSocket.instances[FakeWebSocket.instances.length - 1]
    expect(ws).toBeTruthy()

    ws.emit('status')
    ws.emit('trace')
    ws.emit('react')
    await flushPromises()
    expect(inboxCallsFor('run-a', 'gate-a', 1).length).toBe(before)

    ws.emit('artifact_edit')
    await flushPromises()
    expect(inboxCallsFor('run-a', 'gate-a', 1).length).toBe(before + 1)

    ws.emit('review', { event: 'turn_done' })
    await flushPromises()
    expect(inboxCallsFor('run-a', 'gate-a', 1).length).toBe(before + 2)

    ws.emit('review', { event: 'error' })
    await flushPromises()
    expect(inboxCallsFor('run-a', 'gate-a', 1).length).toBe(before + 3)

    wrapper.unmount()
  })

  it('processing lock: list reselection ignored until resolve converges', async () => {
    const a = gateItem('a')
    const b = gateItem('b')
    let list = [a, b]
    mocks.listGates.mockImplementation(async () => paged(list))

    let releaseResume!: (v: unknown) => void
    mocks.resumeGate.mockImplementation(
      () =>
        new Promise((resolve) => {
          releaseResume = resolve
        }),
    )

    const wrapper = mountInbox()
    await flushPromises()
    await nextTick()

    const resolveClick = wrapper.get('[data-testid="resolve-btn"]').trigger('click')
    await flushPromises()

    const buttons = wrapper.findAll('button').filter((btn) => btn.text().includes('Gate b'))
    expect(buttons.length).toBeGreaterThan(0)
    await buttons[0].trigger('click')
    await flushPromises()

    // Still locked on a — must not start fetching b yet.
    expect(inboxCallsFor('run-b', 'gate-b', 1).length).toBe(0)
    expect(wrapper.find('[data-testid="resolve-btn"]').exists()).toBe(true)

    list = [b]
    releaseResume({ status: 'ok' })
    await resolveClick
    await flushPromises()
    await nextTick()
    await flushPromises()

    expect(wrapper.text()).toContain('Gate b')
    expect(inboxCallsFor('run-b', 'gate-b', 1).length).toBeGreaterThan(0)
    wrapper.unmount()
  })

  it('clarify ordinary turn does not markProcessed; force finish short-circuits like gate', async () => {
    const a = clarifyItem('a')
    const b = clarifyItem('b')
    let list: InboxItem[] = [a, b]
    mocks.listGates.mockImplementation(async () => paged(list))

    const wrapper = mountInbox()
    await flushPromises()
    await nextTick()

    const before = inboxCallsFor('run-a', 'clarify-a', 1).length
    expect(before).toBeGreaterThan(0)

    // Ordinary turn: stay pending; status must not softRefresh.
    // Mid-turn react is gated by liveBusy (turn_begin) — must not softRefresh.
    await wrapper.get('[data-testid="clarify-turn"]').trigger('click')
    await flushPromises()
    await nextTick()
    await flushPromises()

    expect(mocks.reactReply).toHaveBeenCalledWith(
      'run-a',
      'clarify-a',
      'hi',
      [],
      false,
      [],
    )
    const ws = FakeWebSocket.instances.find((w) => w.url.includes('run-a'))
    expect(ws).toBeTruthy()
    expect(ws!.readyState).toBe(1)
    const afterTurn = inboxCallsFor('run-a', 'clarify-a', 1).length
    ws!.emit('status')
    ws!.emit('trace')
    await flushPromises()
    expect(inboxCallsFor('run-a', 'clarify-a', 1).length).toBe(afterTurn)

    // turn_begin marks live busy → react must not softRefresh (stale reactSessions gate was the bug).
    ws!.emit('review', { event: 'turn_begin', nodeId: 'clarify-a' })
    await flushPromises()
    const afterBegin = inboxCallsFor('run-a', 'clarify-a', 1).length
    ws!.emit('react')
    await flushPromises()
    expect(inboxCallsFor('run-a', 'clarify-a', 1).length).toBe(afterBegin)

    // Idle react (no busy) may softRefresh.
    ws!.emit('review', { event: 'turn_done', nodeId: 'clarify-a' })
    await flushPromises()
    const afterDone = inboxCallsFor('run-a', 'clarify-a', 1).length
    expect(afterDone).toBeGreaterThan(afterBegin)
    ws!.emit('react')
    await flushPromises()
    expect(inboxCallsFor('run-a', 'clarify-a', 1).length).toBe(afterDone + 1)

    // Force finish: short-circuit symmetric with gate resolve.
    const afterReact = inboxCallsFor('run-a', 'clarify-a', 1).length
    list = [b]
    await wrapper.get('[data-testid="clarify-send"]').trigger('click')
    await flushPromises()
    await nextTick()
    await flushPromises()

    expect(mocks.reactReply).toHaveBeenLastCalledWith(
      'run-a',
      'clarify-a',
      'hi',
      [],
      true,
      [],
    )
    expect(inboxCallsFor('run-a', 'clarify-a', 1).length).toBe(afterReact)
    expect(wrapper.text()).toContain('Clarify b')
    wrapper.unmount()
  })

  it('resumeGate failure rolls back: unlocks selection and allows refetch', async () => {
    const a = gateItem('a')
    const b = gateItem('b')
    mocks.listGates.mockResolvedValue(paged([a, b]))
    mocks.resumeGate.mockRejectedValueOnce(new Error('resume failed'))

    const wrapper = mountInbox()
    await flushPromises()
    await nextTick()

    const before = inboxCallsFor('run-a', 'gate-a', 1).length
    await wrapper.get('[data-testid="resolve-btn"]').trigger('click')
    await flushPromises()
    await nextTick()

    // Still on a after rollback.
    expect(wrapper.find('[data-testid="resolve-btn"]').exists()).toBe(true)

    const buttons = wrapper.findAll('button').filter((btn) => btn.text().includes('Gate b'))
    await buttons[0].trigger('click')
    await flushPromises()
    expect(inboxCallsFor('run-b', 'gate-b', 1).length).toBeGreaterThan(0)

    // Reselect a — refetch allowed after unmark.
    const buttonsA = wrapper.findAll('button').filter((btn) => btn.text().includes('Gate a'))
    await buttonsA[0].trigger('click')
    await flushPromises()
    expect(inboxCallsFor('run-a', 'gate-a', 1).length).toBeGreaterThan(before)
    wrapper.unmount()
  })

  it('refresh failure after resume still unlocks processing lock and converges', async () => {
    const a = gateItem('a')
    const b = gateItem('b')
    let list = [a, b]
    mocks.listGates.mockImplementation(async () => paged(list))

    const wrapper = mountInbox()
    await flushPromises()
    await nextTick()

    // Force refresh + loadList both fail after resume — local converge must still win.
    list = [b]
    mocks.listGates.mockRejectedValueOnce(new Error('refresh blew up'))
    mocks.listGates.mockRejectedValueOnce(new Error('loadList blew up'))
    await wrapper.get('[data-testid="resolve-btn"]').trigger('click')
    await flushPromises()
    await nextTick()
    await flushPromises()

    // Local converge + unlock must happen even when submit refresh soft-fails.
    expect(wrapper.text()).not.toContain('Gate a')
    expect(wrapper.text()).toContain('Gate b')
    expect(inboxCallsFor('run-b', 'gate-b', 1).length).toBeGreaterThan(0)
    expect(usePendingGates().totalCount.value).toBe(1)

    // Lock released: user can select another (re-select b after converge is fine;
    // add a third via list to prove selectItem is not ignored).
    const c = gateItem('c')
    list = [b, c]
    mocks.listGates.mockImplementation(async () => paged(list))
    const refreshBtn = wrapper.findAll('button').find((btn) => btn.text().includes('刷新'))
    expect(refreshBtn).toBeTruthy()
    await refreshBtn!.trigger('click')
    await flushPromises()
    await nextTick()

    const buttonsC = wrapper.findAll('button').filter((btn) => btn.text().includes('Gate c'))
    expect(buttonsC.length).toBeGreaterThan(0)
    await buttonsC[0].trigger('click')
    await flushPromises()
    expect(inboxCallsFor('run-c', 'gate-c', 1).length).toBeGreaterThan(0)
    wrapper.unmount()
  })

  it('refresh failure after clarify force-finish still unlocks and converges', async () => {
    const a = clarifyItem('a')
    const b = clarifyItem('b')
    let list = [a, b]
    mocks.listGates.mockImplementation(async () => paged(list))

    const wrapper = mountInbox()
    await flushPromises()
    await nextTick()

    list = [b]
    mocks.listGates.mockRejectedValueOnce(new Error('refresh blew up'))
    mocks.listGates.mockRejectedValueOnce(new Error('loadList blew up'))
    await wrapper.get('[data-testid="clarify-send"]').trigger('click')
    await flushPromises()
    await nextTick()
    await flushPromises()

    // Symmetric with gate: ghost gone, neighbor selected, lock released.
    expect(wrapper.text()).not.toContain('Clarify a')
    expect(wrapper.text()).toContain('Clarify b')
    expect(inboxCallsFor('run-b', 'clarify-b', 1).length).toBeGreaterThan(0)
    expect(mocks.toastSuccess).toHaveBeenCalled()
    expect(usePendingGates().totalCount.value).toBe(1)

    const c = clarifyItem('c')
    list = [b, c]
    mocks.listGates.mockImplementation(async () => paged(list))
    const refreshBtn = wrapper.findAll('button').find((btn) => btn.text().includes('刷新'))
    expect(refreshBtn).toBeTruthy()
    expect(refreshBtn!.attributes('disabled')).toBeUndefined()
    await refreshBtn!.trigger('click')
    await flushPromises()
    await nextTick()

    const buttonsC = wrapper.findAll('button').filter((btn) => btn.text().includes('Clarify c'))
    expect(buttonsC.length).toBeGreaterThan(0)
    await buttonsC[0].trigger('click')
    await flushPromises()
    expect(inboxCallsFor('run-c', 'clarify-c', 1).length).toBeGreaterThan(0)
    wrapper.unmount()
  })

  it('processing lock: manual refresh ignored until resolve converges', async () => {
    const a = gateItem('a')
    const b = gateItem('b')
    let list = [a, b]
    mocks.listGates.mockImplementation(async () => paged(list))

    let releaseResume!: (v: unknown) => void
    mocks.resumeGate.mockImplementation(
      () =>
        new Promise((resolve) => {
          releaseResume = resolve
        }),
    )

    const wrapper = mountInbox()
    await flushPromises()
    await nextTick()

    const resolveClick = wrapper.get('[data-testid="resolve-btn"]').trigger('click')
    await flushPromises()

    const refreshBtn = wrapper.findAll('button').find((btn) => btn.text().includes('刷新'))
    expect(refreshBtn).toBeTruthy()
    expect(refreshBtn!.attributes('disabled')).toBeDefined()

    const listCallsBefore = mocks.listGates.mock.calls.length
    // Even if a click is synthesized, processingLock short-circuits onManualRefresh.
    await refreshBtn!.trigger('click')
    await flushPromises()
    expect(mocks.listGates.mock.calls.length).toBe(listCallsBefore)

    // Selection must stay on a while locked.
    expect(wrapper.find('[data-testid="resolve-btn"]').exists()).toBe(true)
    expect(inboxCallsFor('run-b', 'gate-b', 1).length).toBe(0)

    list = [b]
    releaseResume({ status: 'ok' })
    await resolveClick
    await flushPromises()
    await nextTick()
    await flushPromises()

    expect(wrapper.text()).toContain('Gate b')
    const refreshAfter = wrapper.findAll('button').find((btn) => btn.text().includes('刷新'))
    expect(refreshAfter?.attributes('disabled')).toBeUndefined()
    wrapper.unmount()
  })

  it('approve success: list removes item, detail is not Run # - shell, counts sync', async () => {
    const a = gateItem('a')
    const b = gateItem('b')
    let list: InboxItem[] = [a, b]
    mocks.listGates.mockImplementation(async () => paged(list))

    const wrapper = mountInbox()
    await flushPromises()
    await nextTick()

    list = [b]
    await wrapper.get('[data-testid="resolve-btn"]').trigger('click')
    await flushPromises()
    await nextTick()
    await flushPromises()

    expect(wrapper.text()).not.toContain('Gate a')
    expect(wrapper.text()).toContain('Gate b')
    expect(wrapper.text()).not.toMatch(/Run #\s*-/)
    expect(wrapper.text()).toContain('Run #b')
    expect(usePendingGates().totalCount.value).toBe(1)
    expect(usePendingGates().displayedItems.value.map((it) => it.nodeId)).toEqual(['gate-b'])
    wrapper.unmount()
  })

  it('late overlapping listGates after unlock does not restore ghost or Run # - shell', async () => {
    const only = gateItem('solo')
    let list: InboxItem[] = [only]
    mocks.listGates.mockImplementation(async () => paged(list))

    const wrapper = mountInbox()
    await flushPromises()
    await nextTick()
    expect(wrapper.text()).toContain('Gate solo')
    expect(wrapper.text()).toContain('Run #solo')

    // Start a background loadList via filter watch; keep its snapshot pending.
    let releaseStale!: (value: ReturnType<typeof paged>) => void
    mocks.listGates.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          releaseStale = resolve
        }),
    )
    filterState.pipelineSelected!.value = 'wf-stale'
    await flushPromises()

    // Approve while the stale listGates is still in flight.
    list = []
    mocks.listGates.mockImplementation(async () => paged(list))
    await wrapper.get('[data-testid="resolve-btn"]').trigger('click')
    await flushPromises()
    await nextTick()
    await flushPromises()

    // Converged empty inbox — no Run # - shell, no ghost card.
    expect(wrapper.text()).not.toContain('Gate solo')
    expect(wrapper.text()).not.toMatch(/Run #\s*-/)
    expect(wrapper.find('[data-testid="resolve-btn"]').exists()).toBe(false)
    expect(usePendingGates().totalCount.value).toBe(0)

    // Stale snapshot arrives with the approved item — must be discarded.
    releaseStale(paged([only]))
    await flushPromises()
    await nextTick()
    expect(wrapper.text()).not.toContain('Gate solo')
    expect(wrapper.text()).not.toMatch(/Run #\s*-/)
    expect(usePendingGates().totalCount.value).toBe(0)
    wrapper.unmount()
  })
})
