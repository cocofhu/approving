// @vitest-environment happy-dom
import { defineComponent, h, nextTick } from 'vue'
import { createI18n } from 'vue-i18n'
import { createMemoryHistory, createRouter, RouterView } from 'vue-router'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'
import type { SandboxView } from '@/lib/api'

const apiMocks = vi.hoisted(() => ({
  getSandbox: vi.fn(),
  listSandboxes: vi.fn(),
  sandboxLog: vi.fn(),
  sandboxIdeUrl: (_id: number) => 'about:blank#ide',
  sandboxBridgeUrl: (_id: number) => 'about:blank#acp',
  sandboxTerminalWsUrl: (id: number) => `wss://test.local/sandboxes/${id}/terminal`,
}))

vi.mock('@/lib/api', async () => {
  const actual = await vi.importActual<typeof import('@/lib/api')>('@/lib/api')
  return {
    ...actual,
    api: {
      ...actual.api,
      getSandbox: apiMocks.getSandbox,
      listSandboxes: apiMocks.listSandboxes,
      sandboxLog: apiMocks.sandboxLog,
      sandboxIdeUrl: apiMocks.sandboxIdeUrl,
      sandboxBridgeUrl: apiMocks.sandboxBridgeUrl,
      sandboxTerminalWsUrl: apiMocks.sandboxTerminalWsUrl,
    },
  }
})

vi.mock('@/lib/useToast', () => ({
  useToast: () => ({ success: vi.fn(), error: vi.fn() }),
}))

vi.mock('@xterm/xterm', () => {
  class Terminal {
    cols = 80
    rows = 24
    loadAddon() {}
    open() {}
    write() {}
    dispose() {}
    onData() {
      return { dispose() {} }
    }
  }
  return { Terminal }
})

vi.mock('@xterm/addon-fit', () => {
  class FitAddon {
    fit() {}
  }
  return { FitAddon }
})

vi.mock('@xterm/xterm/css/xterm.css', () => ({}))

vi.mock('@/components/run/NovncPreviewPanel.vue', () => ({
  default: {
    name: 'NovncPreviewPanel',
    props: ['sandboxId', 'fill'],
    template: '<div data-testid="novnc-stub" />',
  },
}))

import SandboxConsoleView from './SandboxConsoleView.vue'

const MOCK_SANDBOX: SandboxView = {
  id: 42,
  name: 'sb-42',
  profile: 'default',
  purpose: 'test',
  status: 'running',
  createdAt: '2026-01-01T00:00:00Z',
  updatedAt: '2026-01-01T00:00:00Z',
  containerStatus: 'running',
  busy: false,
  connected: true,
  hasCodeServer: true,
  hasAcp: true,
}

class MockWebSocket {
  static CONNECTING = 0
  static OPEN = 1
  static CLOSING = 2
  static CLOSED = 3
  readyState = MockWebSocket.OPEN
  binaryType = 'arraybuffer'
  onopen: ((ev: Event) => void) | null = null
  onmessage: ((ev: MessageEvent) => void) | null = null
  onclose: ((ev: CloseEvent) => void) | null = null
  onerror: ((ev: Event) => void) | null = null
  constructor(_url: string) {
    queueMicrotask(() => this.onopen?.(new Event('open')))
  }
  send() {}
  close() {
    this.readyState = MockWebSocket.CLOSED
  }
}

async function mountConsole(tabQuery?: string): Promise<VueWrapper> {
  apiMocks.getSandbox.mockResolvedValue(MOCK_SANDBOX)
  apiMocks.listSandboxes.mockResolvedValue([MOCK_SANDBOX])
  apiMocks.sandboxLog.mockResolvedValue({ content: '', live: false, found: false })

  const i18n = createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...common, ...pages } },
  })
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/sandboxes', component: { render: () => h('div') } },
      { path: '/sandboxes/:id/console', component: SandboxConsoleView },
    ],
  })
  await router.push({
    path: '/sandboxes/42/console',
    query: tabQuery ? { tab: tabQuery } : {},
  })
  await router.isReady()

  const wrapper = mount(
    defineComponent({
      setup() {
        return () => h(RouterView)
      },
    }),
    {
      global: {
        plugins: [i18n, router],
        stubs: { Icon: true },
      },
      attachTo: document.body,
    },
  )
  await flushPromises()
  await nextTick()
  return wrapper
}

describe('SandboxConsoleView IDE/ACP lazy mount', () => {
  beforeEach(() => {
    vi.stubGlobal('WebSocket', MockWebSocket as unknown as typeof WebSocket)
    vi.stubGlobal(
      'ResizeObserver',
      class {
        observe() {}
        disconnect() {}
        unobserve() {}
      },
    )
  })

  afterEach(() => {
    vi.unstubAllGlobals()
    vi.clearAllMocks()
    document.body.innerHTML = ''
  })

  it('default terminal tab does not mount IDE/ACP iframes', async () => {
    const wrapper = await mountConsole()
    expect(wrapper.find('iframe[title="code-server"]').exists()).toBe(false)
    expect(wrapper.find('iframe[title="ACP bridge"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('mounts IDE iframe on first IDE tab click and keeps it after switch back', async () => {
    const wrapper = await mountConsole()
    expect(wrapper.find('iframe[title="code-server"]').exists()).toBe(false)

    const ideBtn = wrapper.findAll('button').find((b) => b.text().includes('IDE'))
    expect(ideBtn).toBeTruthy()
    await ideBtn!.trigger('click')
    await flushPromises()
    await nextTick()

    const ideIframe = wrapper.find('iframe[title="code-server"]')
    expect(ideIframe.exists()).toBe(true)
    expect(wrapper.find('iframe[title="ACP bridge"]').exists()).toBe(false)

    const termBtn = wrapper.findAll('button').find((b) => b.text().includes('终端'))
    expect(termBtn).toBeTruthy()
    await termBtn!.trigger('click')
    await nextTick()

    expect(wrapper.find('iframe[title="code-server"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('?tab=ide deep link mounts IDE iframe', async () => {
    const wrapper = await mountConsole('ide')
    await flushPromises()
    expect(wrapper.find('iframe[title="code-server"]').exists()).toBe(true)
    expect(wrapper.find('iframe[title="ACP bridge"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('?tab=acp-native deep link mounts ACP iframe; legacy ?tab=acp does not', async () => {
    const native = await mountConsole('acp-native')
    await flushPromises()
    expect(native.find('iframe[title="ACP bridge"]').exists()).toBe(true)
    native.unmount()

    const legacy = await mountConsole('acp')
    await flushPromises()
    expect(legacy.find('iframe[title="ACP bridge"]').exists()).toBe(false)
    expect(legacy.find('iframe[title="code-server"]').exists()).toBe(false)
    legacy.unmount()
  })
})
