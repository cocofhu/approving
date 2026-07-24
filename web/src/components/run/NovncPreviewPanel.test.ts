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
  constructor(_url: string) {
    queueMicrotask(() => {
      this.onopen?.()
      this.onmessage?.({ data: JSON.stringify({ type: 'ready', url: 'http://localhost:5173' }) })
    })
  }
  send() {}
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

describe('NovncPreviewPanel', () => {
  beforeEach(() => {
    vi.stubGlobal('WebSocket', MockWebSocket)
  })

  afterEach(() => {
    document.body.innerHTML = ''
    vi.unstubAllGlobals()
  })

  it('connects preview mode and shows live toolbar', async () => {
    const wrapper = mountNovnc()
    await flushPromises()
    expect(apiMocks.previewVncWsUrl).toHaveBeenCalledWith('run-1', 'node-1', 5173)
    expect(wrapper.text()).toMatch(/已连接|连接中/)
    const inspectBtn = wrapper.findAll('button').find((b) => b.text().includes('取点标注'))
    expect(inspectBtn).toBeTruthy()
    wrapper.unmount()
  })

  it('renders console mode address bar', async () => {
    const wrapper = mountNovnc({ sandboxId: 42, runId: undefined, nodeId: undefined, port: undefined })
    await flushPromises()
    expect(apiMocks.sandboxVncWsUrl).toHaveBeenCalledWith(42)
    expect(wrapper.find('form').exists()).toBe(true)
    wrapper.unmount()
  })

  it('toggles inspect mode in preview toolbar', async () => {
    const wrapper = mountNovnc()
    await flushPromises()
    const inspectBtn = wrapper.findAll('button').find((b) => b.text().includes('取点标注'))
    expect(inspectBtn).toBeTruthy()
    await inspectBtn!.trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('取消取点')
    wrapper.unmount()
  })
})
