// @vitest-environment happy-dom
import { computed, ref } from 'vue'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { AcpEvent, Run } from '@/lib/shared/types'
import { useRunDetailWs } from './useRunDetailWs'

const sampleEvents: AcpEvent[] = [
  { kind: 'thought', text: 'thinking', t: 1 },
  { kind: 'message', text: 'hello', t: 2 },
]

describe('useRunDetailWs', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('seeds busy dialogue rails, buffers ACP, and tears down realtime', async () => {
    const run = ref({
      id: 'run-1',
      status: 'running',
      reactSessions: {
        c1: { busy: true, waiting: 0, items: [], activeItem: null },
      },
      gate: { reactUpstreamNodeId: 'c1', nodeId: 'g1' },
      nodeRuns: {},
    } as unknown as Run)
    const selected = ref<string | null>('c1')
    const manual = ref(false)
    const liveBusy: Record<string, boolean> = {}
    const liveNode = ref<string | null>(null)
    const eventPages: Record<string, any> = {
      c1: { events: sampleEvents, nextCursor: '', hasMore: false, live: false },
    }
    const nodeTab = ref('clarify')
    const reviewChatRef = ref<{
      applyReviewFrame: (frame: any) => boolean
      applyAcpEvents: (events: AcpEvent[] | undefined, nodeId?: string) => boolean
    } | null>({
      applyReviewFrame: () => true,
      applyAcpEvents: () => true,
    })
    const gateApprovalRef = ref<{
      applyReviewFrame: (frame: any) => void
      applyAcpEvents: (events: AcpEvent[] | undefined) => boolean
    } | null>({
      applyReviewFrame: () => undefined,
      applyAcpEvents: () => true,
    })
    const rehydrateByNode: Record<string, string> = {}
    const fetchNodeEvents = vi.fn(async () => true)
    const loadRun = vi.fn()

    const api = useRunDetailWs({
      runId: computed(() => 'run-1'),
      run,
      selected,
      manual,
      liveBusy,
      liveNode,
      eventPages,
      nodeTab,
      reviewChatRef,
      gateApprovalRef,
      selClarify: computed(() => ({ nodeId: 'c1' })),
      fetchNodeEvents,
      mergeLiveWsAcpPage: vi.fn(),
      rehydrateByNode,
      rehydrateNodeEvents: vi.fn(async () => undefined),
      fetchSandboxLog: vi.fn(),
      maybePollSandboxForBoot: vi.fn(),
      isClarifySessionBusy: () => false,
      loadRun,
    })

    await api.projectDialogueAfterLoad({
      reactSessions: {
        c1: { busy: true, waiting: 0, items: [], activeItem: null },
      },
    })
    expect(liveBusy.c1).toBe(true)

    await api.seedDialogueAcpAfterRestore(['c1'])
    api.applyOrBufferDialogueAcp('c1', sampleEvents, true)

    // Buffer path when surfaces missing
    reviewChatRef.value = null
    gateApprovalRef.value = null
    api.applyOrBufferDialogueAcp('c1', sampleEvents, true)

    api.resetDialogueState()
    await api.initAfterLoadSuccess({
      applyDetailArtifactsDeepLink: () => false,
      applyOutputDeepLinkFocus: () => false,
      defaultNode: 'c1',
      syncAllMcpCallsFromRun: () => undefined,
    })
    api.teardownRealtime()
    expect(api.wsConnected).toBe(false)
    expect(typeof api.connectWs).toBe('function')
  })

  it('restores idle sessions and flushes without busy seed', async () => {
    const run = ref({
      id: 'run-2',
      status: 'waiting_human',
      reactSessions: {},
      nodeRuns: {},
    } as unknown as Run)
    const api = useRunDetailWs({
      runId: computed(() => 'run-2'),
      run,
      selected: ref(null),
      manual: ref(false),
      liveBusy: {},
      liveNode: ref(null),
      eventPages: {},
      nodeTab: ref('output'),
      reviewChatRef: ref(null),
      gateApprovalRef: ref(null),
      selClarify: computed(() => null),
      fetchNodeEvents: vi.fn(async () => false),
      mergeLiveWsAcpPage: vi.fn(),
      rehydrateByNode: {},
      rehydrateNodeEvents: vi.fn(async () => undefined),
      fetchSandboxLog: vi.fn(),
      maybePollSandboxForBoot: vi.fn(),
      isClarifySessionBusy: () => false,
      loadRun: vi.fn(),
    })

    await api.projectDialogueAfterLoad({
      reactSessions: {
        idle: { busy: false, waiting: 0, items: [] },
      },
    })
    api.teardownRealtime()
  })
})
