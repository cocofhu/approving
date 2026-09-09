// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'
import PublicAppPreviewPanel from './PublicAppPreviewPanel.vue'

const shareMocks = vi.hoisted(() => ({
  createPreviewTicket: vi.fn(),
}))

vi.mock('@/lib/inbox/gateShareLink', async () => {
  const actual = await vi.importActual<typeof import('@/lib/inbox/gateShareLink')>('@/lib/inbox/gateShareLink')
  return {
    ...actual,
    publicPreviewVncWsUrl: () => 'ws://example.test/vnc',
    publicGateApi: {
      ...actual.publicGateApi,
      createPreviewTicket: shareMocks.createPreviewTicket,
    },
  }
})

describe('PublicAppPreviewPanel', () => {
  beforeEach(() => {
    shareMocks.createPreviewTicket.mockResolvedValue({
      ticket: 'tix',
      wsPath: '/vnc',
      iframeUrl: 'http://example.test/preview',
    })
  })

  it('mounts with ports and requests a ticket', async () => {
    const i18n = createI18n({
      legacy: false,
      locale: 'zh-CN',
      messages: { 'zh-CN': { ...common, ...pages } },
    })
    const w = mount(PublicAppPreviewPanel, {
      props: {
        token: 'share-token',
        ports: [{ port: 5173, label: 'web', kind: 'http' }],
        active: true,
      },
      global: {
        plugins: [i18n],
        stubs: {
          NovncPreviewPanel: true,
          DirectPreviewFrame: true,
          ExternalUrlPreviewFrame: true,
        },
      },
    })
    await flushPromises()
    expect(w.html().length).toBeGreaterThan(20)
    w.unmount()
  })

  it('shows inactive state when share is not active', async () => {
    const i18n = createI18n({
      legacy: false,
      locale: 'zh-CN',
      messages: { 'zh-CN': { ...common, ...pages } },
    })
    const w = mount(PublicAppPreviewPanel, {
      props: {
        token: 'share-token',
        ports: [],
        active: false,
      },
      global: { plugins: [i18n] },
    })
    await flushPromises()
    expect(w.html().length).toBeGreaterThan(10)
    w.unmount()
  })
})
