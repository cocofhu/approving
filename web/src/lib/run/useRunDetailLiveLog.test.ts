// @vitest-environment happy-dom
import { createApp, computed, ref } from 'vue'
import { createI18n } from 'vue-i18n'
import { createMemoryHistory, createRouter } from 'vue-router'
import { describe, expect, it, vi } from 'vitest'
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
      sandboxLog: vi.fn(async () => ({ content: '', live: false, found: false })),
      getSandbox: vi.fn(async () => null),
    },
  }
})

import { useRunDetailLiveLog } from './useRunDetailLiveLog'

describe('useRunDetailLiveLog', () => {
  it('initializes log state helpers for a selected agent node', async () => {
    const i18n = createI18n({
      legacy: false,
      locale: 'zh-CN',
      messages: { 'zh-CN': { ...common, ...pages } },
    })
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [{ path: '/', component: { template: '<div />' } }],
    })
    await router.isReady()

    const run = ref({
      id: 'run-1',
      status: 'running',
      nodeRuns: { a1: { nodeId: 'a1', status: 'running', events: [], mcpCalls: [] } },
      artifacts: [],
    } as unknown as Run)
    const selected = ref<string | null>('a1')
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

    expect(live.logEvents.value).toEqual([])
    expect(live.logBusy.value).toBe(false)
    live.resetLiveLogState()
    live.abortSandboxFetches()
    live.disposeAllRehydrateOrchs()
    live.goSandboxLogTab()
    expect(nodeTab.value).toBe('sandbox')
    live.mergeLiveWsAcpPage('a1', [{ kind: 'message', text: 'hi', t: 1 }])
    expect(Array.isArray(live.logEvents.value)).toBe(true)

    app.unmount()
  })
})
