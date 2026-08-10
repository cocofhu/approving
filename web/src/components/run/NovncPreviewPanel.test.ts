// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'
import NovncPreviewPanel from './NovncPreviewPanel.vue'

class MockWebSocket {
  static OPEN = 1
  readyState = MockWebSocket.OPEN
  onopen: (() => void) | null = null
  onmessage: ((ev: { data: string }) => void) | null = null
  onclose: (() => void) | null = null
  onerror: (() => void) | null = null
  sent: string[] = []
  constructor(_url: string) {
    MockWebSocket.instances.push(this)
    queueMicrotask(() => {
      this.onopen?.()
      this.onmessage?.({ data: JSON.stringify({ type: 'ready', url: 'http://localhost:5173' }) })
    })
  }
  static instances: MockWebSocket[] = []
  static reset() {
    MockWebSocket.instances = []
  }
  send(data: string) {
    this.sent.push(data)
  }
  close() {}
}

vi.mock('@novnc/novnc/lib/rfb.js', () => ({
  default: class MockRFB {
    scaleViewport = false
    resizeSession = false
    viewOnly = false
    focusOnClick = false
    showDotCursor = false
    background = ''
    constructor(_host: HTMLElement, _channel: unknown) {}
    addEventListener(event: string, cb: () => void) {
      if (event === 'connect') queueMicrotask(cb)
    }
    disconnect() {}
  },
}))

const apiMocks = vi.hoisted(() => ({
  previewVncWsUrl: vi.fn(() => 'ws://localhost/preview-vnc/run-1/node-1/5173/ws'),
  sandboxVncWsUrl: vi.fn(() => 'ws://localhost/sandbox-vnc/1/ws'),
}))

vi.mock('@/lib/api', async () => {
  const actual = await vi.importActual<typeof import('@/lib/api')>('@/lib/api')
  return {
    ...actual,
    api: {
      ...actual.api,
      previewVncWsUrl: apiMocks.previewVncWsUrl,
      sandboxVncWsUrl: apiMocks.sandboxVncWsUrl,
    },
  }
})

function mountNovnc(props: Record<string, unknown> = {}) {
  const i18n = createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...common, ...pages } },
  })
  return mount(NovncPreviewPanel, {
    props: {
      runId: 'run-1',
      nodeId: 'node-1',
      port: 5173,
      fill: false,
      compact: false,
      ...props,
    },
    global: { plugins: [i18n], stubs: { Icon: true } },
    attachTo: document.body,
  })
}

function inspectButton(wrapper: ReturnType<typeof mountNovnc>) {
  return wrapper.find('[data-testid="novnc-inspect-toggle"]')
}

function lastInspectCtrl(): { type: string; on: boolean } | null {
  const ws = MockWebSocket.instances[0]
  if (!ws) return null
  for (let i = ws.sent.length - 1; i >= 0; i--) {
    try {
      const msg = JSON.parse(ws.sent[i]) as { type?: string; on?: boolean }
      if (msg.type === 'inspect') return msg as { type: string; on: boolean }
    } catch {
      /* ignore */
    }
  }
  return null
}

describe('NovncPreviewPanel', () => {
  beforeEach(() => {
    MockWebSocket.reset()
    vi.stubGlobal('WebSocket', MockWebSocket)
  })

  afterEach(() => {
    document.body.innerHTML = ''
    vi.unstubAllGlobals()
  })

  it('sets aria-busy while connecting', async () => {
    const wrapper = mountNovnc()
    expect(wrapper.find('[aria-busy="true"]').exists()).toBe(true)
    await flushPromises()
    wrapper.unmount()
  })

  it('connects preview mode and shows live toolbar', async () => {
    const wrapper = mountNovnc()
    await flushPromises()
    expect(apiMocks.previewVncWsUrl).toHaveBeenCalledWith('run-1', 'node-1', 5173)
    expect(wrapper.text()).toMatch(/已连接|连接中/)
    expect(inspectButton(wrapper).exists()).toBe(true)
    expect(inspectButton(wrapper).text()).toContain('取点标注')
    wrapper.unmount()
  })

  it('renders console mode address bar', async () => {
    const wrapper = mountNovnc({ sandboxId: 42, runId: undefined, nodeId: undefined, port: undefined })
    await flushPromises()
    expect(apiMocks.sandboxVncWsUrl).toHaveBeenCalledWith(42)
    expect(wrapper.find('form').exists()).toBe(true)
    wrapper.unmount()
  })

  it('uses platform WS only and never mentions websockify/6080 (g4.2)', async () => {
    const preview = mountNovnc()
    await flushPromises()
    expect(apiMocks.previewVncWsUrl).toHaveBeenCalledWith('run-1', 'node-1', 5173)
    expect(preview.html()).not.toMatch(/6080|websockify/i)
    preview.unmount()

    const consoleWrap = mountNovnc({ sandboxId: 42, runId: undefined, nodeId: undefined, port: undefined })
    await flushPromises()
    expect(apiMocks.sandboxVncWsUrl).toHaveBeenCalledWith(42)
    expect(consoleWrap.html()).not.toMatch(/6080|websockify/i)
    consoleWrap.unmount()
  })

  it('toggles inspect with stable label and aria-pressed; second click sends on:false', async () => {
    const wrapper = mountNovnc()
    await flushPromises()
    const btn = inspectButton(wrapper)
    expect(btn.attributes('aria-pressed')).toBe('false')
    expect(btn.text()).toContain('取点标注')
    expect(wrapper.text()).not.toContain('取消取点')

    await btn.trigger('click')
    await flushPromises()
    expect(btn.attributes('aria-pressed')).toBe('true')
    expect(btn.text()).toContain('取点标注')
    expect(wrapper.text()).not.toContain('取消取点')
    expect(lastInspectCtrl()).toEqual({ type: 'inspect', on: true })

    await btn.trigger('click')
    await flushPromises()
    expect(btn.attributes('aria-pressed')).toBe('false')
    expect(btn.text()).toContain('取点标注')
    expect(lastInspectCtrl()).toEqual({ type: 'inspect', on: false })
    wrapper.unmount()
  })

  it('Esc while inspecting clears pressed state but keeps staged pick', async () => {
    const wrapper = mountNovnc()
    await flushPromises()
    const ws = MockWebSocket.instances[0]
    expect(ws).toBeTruthy()

    await inspectButton(wrapper).trigger('click')
    await flushPromises()
    expect(inspectButton(wrapper).attributes('aria-pressed')).toBe('true')

    ws!.onmessage?.({
      data: JSON.stringify({
        type: 'picked',
        pick: { selector: '#x', tagName: 'div', outerHTML: '<div id="x"></div>' },
      }),
    })
    await flushPromises()
    // pick auto-exits inspect; re-enter then Esc must keep staged
    expect(inspectButton(wrapper).attributes('aria-pressed')).toBe('false')
    expect(wrapper.text()).toContain('#x')

    await inspectButton(wrapper).trigger('click')
    await flushPromises()
    expect(inspectButton(wrapper).attributes('aria-pressed')).toBe('true')

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await flushPromises()
    expect(inspectButton(wrapper).attributes('aria-pressed')).toBe('false')
    expect(wrapper.text()).toContain('#x')
    expect(lastInspectCtrl()).toEqual({ type: 'inspect', on: false })
    wrapper.unmount()
  })

  it('remote inspect-canceled clears button without clearing staged pick', async () => {
    const wrapper = mountNovnc()
    await flushPromises()
    const ws = MockWebSocket.instances[0]!

    await inspectButton(wrapper).trigger('click')
    await flushPromises()
    ws.onmessage?.({
      data: JSON.stringify({
        type: 'picked',
        pick: { selector: '#keep', tagName: 'span', outerHTML: '<span id="keep"></span>' },
      }),
    })
    await flushPromises()
    expect(wrapper.text()).toContain('#keep')

    await inspectButton(wrapper).trigger('click')
    await flushPromises()
    const beforeLen = ws.sent.length
    ws.onmessage?.({ data: JSON.stringify({ type: 'inspect-canceled' }) })
    await flushPromises()
    expect(inspectButton(wrapper).attributes('aria-pressed')).toBe('false')
    expect(wrapper.text()).toContain('#keep')
    // Remote already off — do not echo another inspect on:false
    expect(ws.sent.slice(beforeLen)).toEqual([])
    wrapper.unmount()
  })
})
