// @vitest-environment happy-dom
import { createApp, computed, ref } from 'vue'
import { createI18n } from 'vue-i18n'
import { createMemoryHistory, createRouter } from 'vue-router'
import { afterEach, describe, expect, it, vi } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'
import type { Run } from '@/lib/shared/types'

vi.mock('@/lib/api/api', async () => {
  const actual = await vi.importActual<typeof import('@/lib/api/api')>('@/lib/api/api')
  return {
    ...actual,
    api: {
      ...actual.api,
      nodeEvents: vi.fn(async () => ({ events: [], nextCursor: '', hasMore: false })),
      nodeSandboxLog: vi.fn(async () => ({ content: 'log', live: false, found: true })),
      getRunNodeSandbox: vi.fn(async () => ({ id: 'sbx-1' })),
    },
  }
})

import { useRunDetailLiveLog } from './useRunDetailLiveLog'

describe('useRunDetailLiveLog', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('exposes log helpers and selection side effects without hanging', async () => {
    vi.stubGlobal('requestAnimationFrame', (cb: FrameRequestCallback) => {
      cb(0)
      return 1
    })

    const i18n = createI18n({
      legacy: false,
      locale: 'zh-CN',
      messages: { 'zh-CN': { ...common, ...pages } },
    })
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [
        { path: '/', component: { template: '<div />' } },
        { path: '/sandboxes/:id/console', component: { template: '<div />' } },
      ],
    })
    await router.push('/')
    await router.isReady()

    const run = ref({
      id: 'run-1',
      status: 'running',
      nodeRuns: { a1: { nodeId: 'a1', status: 'running', events: [], mcpCalls: [] } },
      artifacts: [],
    } as unknown as Run)
    const selected = ref<string | null>(null)
    const nodeTab = ref('log')
    const selIterIdx = ref<number | null>(null)

    let live!: ReturnType<typeof useRunDetailLiveLog>
    const app = createApp({
      setup() {
        live = useRunDetailLiveLog({
          runId: computed(() => 'run-1'),
          run,
          selected,
          selExecIdx: computed(() => 0),
          selIterIdx,
          selRun: computed(() => run.value.nodeRuns.a1 || null),
          selStatus: computed(() => 'running'),
          viewingLatest: computed(() => true),
          nodeTab,
          hasLog: computed(() => true),
        })
        return () => null
      },
    })
    app.use(i18n)
    app.use(router)
    app.mount(document.createElement('div'))

    // Trigger selection watch after mount (not immediate).
    selected.value = 'a1'
    await Promise.resolve()
    await Promise.resolve()

    expect(live.logEvents.value).toEqual([])
    live.mergeLiveWsAcpPage('a1', [{ kind: 'message', text: 'hi', t: 1 }])
    live.syncAllMcpCallsFromRun()
    live.goSandboxLogTab()
    expect(nodeTab.value).toBe('sandbox')
    live.openSandboxConsole()
    live.maybePollSandboxForBoot()
    live.resetLiveLogState('run-1')
    live.abortSandboxFetches()
    live.disposeAllRehydrateOrchs()

    app.unmount()
  })
})
