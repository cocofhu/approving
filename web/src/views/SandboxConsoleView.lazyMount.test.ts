// @vitest-environment happy-dom
import { defineComponent, h, nextTick } from 'vue'
import { createI18n } from 'vue-i18n'
import { createMemoryHistory, createRouter, RouterView } from 'vue-router'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'
import type { SandboxView } from '@/lib/api/api'

const apiMocks = vi.hoisted(() => ({
  getSandbox: vi.fn(),
  listSandboxes: vi.fn(),
  sandboxLog: vi.fn(),
  sandboxIdeUrl: (_id: number) => 'about:blank#ide',
  sandboxBridgeUrl: (_id: number) => 'about:blank#acp',
  sandboxTerminalWsUrl: (id: number) => `wss://test.local/sandboxes/${id}/terminal`,
}))

vi.mock('@/lib/api/api', async () => {
  const actual = await vi.importActual<typeof import('@/lib/api/api')>('@/lib/api/api')
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

vi.mock('@/lib/composables/useToast', () => ({
  useToast: () => ({ success: vi.fn(), error: vi.fn() }),
}))

const breakpointMocks = vi.hoisted(() => {
  // eslint-disable-next-line @typescript-eslint/no-require-imports
  const vue = require('vue') as typeof import('vue')
  return { isMobile: vue.ref(false) }
})

vi.mock('@/lib/composables/useBreakpoint', () => ({
  useBreakpoint: () => ({ isMobile: breakpointMocks.isMobile }),
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
  password: 'secret-password',
  endpoints: { '5173': '127.0.0.1:5173', '8080': '127.0.0.1:8080' },
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

async function mountConsole(tabQuery?: string, opts?: { empty?: boolean }): Promise<VueWrapper> {
  if (opts?.empty) {
    apiMocks.getSandbox.mockRejectedValue(new Error('missing'))
    apiMocks.listSandboxes.mockResolvedValue([])
  } else {
    apiMocks.getSandbox.mockResolvedValue(MOCK_SANDBOX)
    apiMocks.listSandboxes.mockResolvedValue([MOCK_SANDBOX])
  }
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
    breakpointMocks.isMobile.value = false
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

  it('log tab failure is not empty state and keeps prior content', async () => {
    const wrapper = await mountConsole()
    apiMocks.sandboxLog.mockReset()
    apiMocks.sandboxLog.mockResolvedValueOnce({ content: 'boot ok\n', live: true, found: true })
    apiMocks.sandboxLog.mockRejectedValueOnce(new Error('docker gone'))
    const logBtn = wrapper.findAll('button').find((b) => b.text().includes('日志'))
    expect(logBtn).toBeTruthy()
    await logBtn!.trigger('click')
    await flushPromises()
    await nextTick()
    expect(wrapper.text()).toContain('boot ok')
    const refresh = wrapper.findAll('button').find((b) => b.attributes('title')?.includes('刷新') || b.attributes('title')?.includes('Refresh'))
    expect(refresh).toBeTruthy()
    await refresh!.trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="sandbox-console-log-error"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('boot ok')
    expect(wrapper.find('[data-testid="sandbox-console-log-empty"]').exists()).toBe(false)
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

describe('SandboxConsoleView mobile desktop-recommend', () => {
  beforeEach(() => {
    breakpointMocks.isMobile.value = true
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

  it('shows desktop recommend first and expands a read-only summary without terminal/iframe', async () => {
    const wrapper = await mountConsole()
    expect(wrapper.find('[data-testid="sandbox-console-mobile"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('推荐在桌面操作')
    expect(wrapper.find('iframe').exists()).toBe(false)
    expect(wrapper.find('[data-testid="novnc-stub"]').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('直连密码')

    await wrapper.get('[data-testid="sandbox-console-peek"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="sandbox-console-summary"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('sb-42')
    expect(wrapper.text()).toContain('running')
    expect(wrapper.text()).not.toContain('secret-password')
    wrapper.unmount()
  })

  it('empty session still shows recommend plus readable empty summary', async () => {
    const wrapper = await mountConsole(undefined, { empty: true })
    expect(wrapper.text()).toContain('推荐在桌面操作')
    await wrapper.get('[data-testid="sandbox-console-peek"]').trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('暂无会话数据')
    expect(wrapper.find('iframe').exists()).toBe(false)
    wrapper.unmount()
  })
})
