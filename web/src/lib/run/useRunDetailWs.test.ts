import { computed, ref } from 'vue'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { Run } from '@/lib/shared/types'
import { useRunDetailWs } from './useRunDetailWs'

describe('useRunDetailWs', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('projects dialogue after load and tears down realtime cleanly', async () => {
    const run = ref({
      id: 'run-1',
      status: 'running',
      reactSessions: {},
      nodeRuns: {},
    } as unknown as Run)
    const selected = ref<string | null>(null)
    const manual = ref(false)
    const liveBusy: Record<string, boolean> = {}
    const liveNode = ref<string | null>(null)
    const eventPages: Record<string, any> = {}
    const nodeTab = ref('output')
    const reviewChatRef = ref(null)
    const gateApprovalRef = ref(null)
    const rehydrateByNode: Record<string, string> = {}

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
      selClarify: computed(() => null),
      fetchNodeEvents: vi.fn(async () => false),
      mergeLiveWsAcpPage: vi.fn(),
      rehydrateByNode,
      rehydrateNodeEvents: vi.fn(async () => undefined),
      fetchSandboxLog: vi.fn(),
      maybePollSandboxForBoot: vi.fn(),
      isClarifySessionBusy: () => false,
      loadRun: vi.fn(),
    })

    await api.projectDialogueAfterLoad({ reactSessions: {} })
    await api.seedDialogueAcpAfterRestore([])
    api.resetDialogueState()
    api.teardownRealtime()
    expect(api.wsConnected).toBe(false)
    expect(typeof api.connectWs).toBe('function')
    expect(typeof api.initAfterLoadSuccess).toBe('function')
  })
})
