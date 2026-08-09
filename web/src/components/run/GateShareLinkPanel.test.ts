// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'
import GateShareLinkPanel from './GateShareLinkPanel.vue'
import type { GateInboxItem } from '@/lib/types'
import { forgetShareUrl, rememberShareUrl } from '@/lib/gateShareLink'

const mocks = vi.hoisted(() => ({
  create: vi.fn(),
  regen: vi.fn(),
  revoke: vi.fn(),
  toastSuccess: vi.fn(),
  toastInfo: vi.fn(),
}))

vi.mock('@/lib/api', async () => {
  const actual = await vi.importActual<typeof import('@/lib/api')>('@/lib/api')
  return {
    ...actual,
    api: {
      ...actual.api,
      createGateShareLink: mocks.create,
      regenGateShareLink: mocks.regen,
      revokeGateShareLink: mocks.revoke,
    },
  }
})

vi.mock('@/lib/useToast', () => ({
  useToast: () => ({
    success: mocks.toastSuccess,
    info: mocks.toastInfo,
    error: vi.fn(),
    warning: vi.fn(),
  }),
}))

const i18n = createI18n({
  legacy: false,
  locale: 'zh-CN',
  messages: { 'zh-CN': { ...common, ...pages } },
})

function item(over: Partial<GateInboxItem> = {}): GateInboxItem {
  return {
    type: 'gate',
    runId: 'run-1',
    nodeId: 'hg1',
    iteration: 1,
    workflowName: 'wf',
    title: '审阅',
    bodyMd: '',
    actions: [{ id: 'approve', label: '批准' }],
    requestedAt: '2026-08-01T00:00:00Z',
    shareLink: { state: 'none', canCreate: true },
    ...over,
  }
}

function mockClipboard(writeText: ReturnType<typeof vi.fn>) {
  Object.defineProperty(navigator, 'clipboard', {
    configurable: true,
    value: { writeText },
  })
}

beforeEach(() => {
  mocks.create.mockReset()
  mocks.regen.mockReset()
  mocks.revoke.mockReset()
  mocks.toastSuccess.mockReset()
  mocks.toastInfo.mockReset()
  forgetShareUrl('run-1', 'hg1', 1)
})

describe('GateShareLinkPanel', () => {
  it('creates with default 24h, masks URL, copies full link', async () => {
    const token = 'ab'.repeat(32)
    const url = `https://app.example/public/gate-approvals#t=${token}`
    mocks.create.mockResolvedValue({ id: 'gsl-1', url, ttlTier: '24h', expiresAt: '2026-08-10T00:00:00Z', state: 'active' })
    mockClipboard(vi.fn().mockResolvedValue(undefined))

    const w = mount(GateShareLinkPanel, {
      props: { open: true, gate: item() },
      global: { plugins: [i18n], stubs: { Teleport: true } },
    })
    await flushPromises()
    expect(w.get('[data-testid="gate-share-panel-body"]').text()).toContain('信任')
    const ttl24 = w.findAll('[data-testid="gate-share-ttl"]').find((b) => b.attributes('data-tier') === '24h')
    expect(ttl24).toBeTruthy()

    await w.get('[data-testid="gate-share-create"]').trigger('click')
    await flushPromises()
    expect(mocks.create).toHaveBeenCalledWith('run-1', 'hg1', '24h')
    expect(navigator.clipboard.writeText).toHaveBeenCalledWith(url)
    const shown = (w.get('[data-testid="gate-share-url"]').element as HTMLTextAreaElement).value
    expect(shown).toContain('••••')
    expect(shown).not.toContain(token)
  })

  it('reveals full URL when clipboard fails', async () => {
    const token = 'cd'.repeat(32)
    const url = `https://app.example/public/gate-approvals#t=${token}`
    mocks.create.mockResolvedValue({ id: 'gsl-2', url, ttlTier: '24h', expiresAt: '2026-08-10T00:00:00Z', state: 'active' })
    mockClipboard(vi.fn().mockRejectedValue(new Error('denied')))

    const w = mount(GateShareLinkPanel, {
      props: { open: true, gate: item() },
      global: { plugins: [i18n], stubs: { Teleport: true } },
    })
    await w.get('[data-testid="gate-share-create"]').trigger('click')
    await flushPromises()
    const shown = (w.get('[data-testid="gate-share-url"]').element as HTMLTextAreaElement).value
    expect(shown).toBe(url)
    expect(mocks.toastInfo).toHaveBeenCalled()
  })

  it('manage mode copies remembered URL without regenerating', async () => {
    const token = 'ef'.repeat(32)
    const url = `https://app.example/public/gate-approvals#t=${token}`
    rememberShareUrl('run-1', 'hg1', 1, url)
    mockClipboard(vi.fn().mockResolvedValue(undefined))

    const w = mount(GateShareLinkPanel, {
      props: {
        open: true,
        gate: item({ shareLink: { state: 'active', ttlTier: '24h', canManage: true, canCreate: false } }),
      },
      global: { plugins: [i18n], stubs: { Teleport: true } },
    })
    await flushPromises()
    expect(w.find('[data-testid="gate-share-ttl"]').exists()).toBe(false)
    await w.get('[data-testid="gate-share-copy"]').trigger('click')
    await flushPromises()
    expect(mocks.regen).not.toHaveBeenCalled()
    expect(navigator.clipboard.writeText).toHaveBeenCalledWith(url)
  })
})
