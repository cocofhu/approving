// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'
import GateShareLinkPanel from './GateShareLinkPanel.vue'
import type { GateInboxItem } from '@/lib/shared/types'
import { forgetShareUrl, rememberShareUrl } from '@/lib/inbox/gateShareLink'

const mocks = vi.hoisted(() => ({
  create: vi.fn(),
  regen: vi.fn(),
  revoke: vi.fn(),
  createReview: vi.fn(),
  regenReview: vi.fn(),
  revokeReview: vi.fn(),
  toastSuccess: vi.fn(),
  toastShow: vi.fn(),
}))

vi.mock('@/lib/api/api', async () => {
  const actual = await vi.importActual<typeof import('@/lib/api/api')>('@/lib/api/api')
  return {
    ...actual,
    api: {
      ...actual.api,
      createGateShareLink: mocks.create,
      regenGateShareLink: mocks.regen,
      revokeGateShareLink: mocks.revoke,
      createReviewShareLink: mocks.createReview,
      regenReviewShareLink: mocks.regenReview,
      revokeReviewShareLink: mocks.revokeReview,
    },
  }
})

vi.mock('@/lib/composables/useToast', () => ({
  useToast: () => ({
    success: mocks.toastSuccess,
    show: mocks.toastShow,
    error: vi.fn(),
    warn: vi.fn(),
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
    nodeType: 'human_gate',
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
  mocks.createReview.mockReset()
  mocks.regenReview.mockReset()
  mocks.revokeReview.mockReset()
  mocks.toastSuccess.mockReset()
  mocks.toastShow.mockReset()
  forgetShareUrl('run-1', 'hg1', 1)
  forgetShareUrl('run-1', 'research1', 1)
})

describe('GateShareLinkPanel', () => {
  it('creates with default 24h, masks URL, copies full link', async () => {
    const token = 'ab'.repeat(32)
    const url = `https://app.example/public/gate-approvals#t=${token}`
    mocks.create.mockResolvedValue({ id: 'gsl-1', url, ttlTier: '24h', expiresAt: '2026-08-10T00:00:00Z', state: 'active' })
    mockClipboard(vi.fn().mockResolvedValue(undefined))

    const w = mount(GateShareLinkPanel, {
      props: { open: true, target: item() },
      global: { plugins: [i18n], stubs: { Teleport: true } },
    })
    await flushPromises()
    expect(w.get('[data-testid="gate-share-panel-body"]').text()).toContain('信任')
    expect(w.get('[data-testid="gate-share-panel-body"]').text()).toContain('审批工作台')
    expect(w.get('[data-testid="gate-share-panel-body"]').text()).toContain('可取点')
    expect(w.get('[data-testid="gate-share-panel-body"]').text()).not.toMatch(/不是内部审批工作台|不可取点|外部一次确认页/)
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
      props: { open: true, target: item() },
      global: { plugins: [i18n], stubs: { Teleport: true } },
    })
    await w.get('[data-testid="gate-share-create"]').trigger('click')
    await flushPromises()
    const shown = (w.get('[data-testid="gate-share-url"]').element as HTMLTextAreaElement).value
    expect(shown).toBe(url)
    expect(mocks.toastShow).toHaveBeenCalled()
  })

  it('copyExisting sets busy and ignores double click', async () => {
    const token = 'ef'.repeat(32)
    const url = `https://app.example/public/gate-approvals#t=${token}`
    rememberShareUrl('run-1', 'hg1', 1, url)
    let resolveWrite!: () => void
    mockClipboard(
      vi.fn(
        () =>
          new Promise<void>((resolve) => {
            resolveWrite = resolve
          }),
      ),
    )

    const w = mount(GateShareLinkPanel, {
      props: {
        open: true,
        target: item({ shareLink: { state: 'active', ttlTier: '24h', canManage: true, canCreate: false } }),
      },
      global: { plugins: [i18n], stubs: { Teleport: true } },
    })
    await flushPromises()
    const copyBtn = w.get('[data-testid="gate-share-copy"]')
    await copyBtn.trigger('click')
    await copyBtn.trigger('click')
    expect(copyBtn.attributes('disabled')).toBeDefined()
    expect(copyBtn.attributes('aria-busy')).toBe('true')
    expect(copyBtn.text()).toMatch(/复制中/)
    expect(navigator.clipboard.writeText).toHaveBeenCalledTimes(1)
    resolveWrite()
    await flushPromises()
    expect(mocks.toastSuccess).toHaveBeenCalled()
    w.unmount()
  })

  it('createAndCopy shows 创建中… and does not create twice', async () => {
    const token = 'ab'.repeat(32)
    const url = `https://app.example/public/gate-approvals#t=${token}`
    let resolveCreate!: (v: unknown) => void
    mocks.create.mockImplementation(
      () =>
        new Promise((resolve) => {
          resolveCreate = resolve
        }),
    )
    mockClipboard(vi.fn().mockResolvedValue(undefined))
    const w = mount(GateShareLinkPanel, {
      props: { open: true, target: item() },
      global: { plugins: [i18n], stubs: { Teleport: true } },
    })
    await flushPromises()
    const createBtn = w.get('[data-testid="gate-share-create"]')
    await createBtn.trigger('click')
    await createBtn.trigger('click')
    expect(createBtn.text()).toMatch(/创建中/)
    expect(mocks.create).toHaveBeenCalledTimes(1)
    resolveCreate({ id: 'gsl-1', url, ttlTier: '24h', expiresAt: '2026-08-10T00:00:00Z', state: 'active' })
    await flushPromises()
    w.unmount()
  })

  it('manage mode copies remembered URL without regenerating', async () => {
    const token = 'ef'.repeat(32)
    const url = `https://app.example/public/gate-approvals#t=${token}`
    rememberShareUrl('run-1', 'hg1', 1, url)
    mockClipboard(vi.fn().mockResolvedValue(undefined))

    const w = mount(GateShareLinkPanel, {
      props: {
        open: true,
        target: item({ shareLink: { state: 'active', ttlTier: '24h', canManage: true, canCreate: false } }),
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

  it('recalls URL from sessionStorage after in-memory miss (refresh)', async () => {
    const token = 'ef'.repeat(32)
    const url = `https://app.example/public/gate-approvals#t=${token}`
    rememberShareUrl('run-1', 'hg1', 1, url)
    forgetShareUrl('run-1', 'hg1', 1)
    sessionStorage.setItem('approving.gateShareUrl.run-1:hg1:1', url)
    mockClipboard(vi.fn().mockResolvedValue(undefined))

    const w = mount(GateShareLinkPanel, {
      props: {
        open: true,
        target: item({ shareLink: { state: 'active', ttlTier: '24h', canManage: true, canCreate: false } }),
      },
      global: { plugins: [i18n], stubs: { Teleport: true } },
    })
    await flushPromises()
    await w.get('[data-testid="gate-share-copy"]').trigger('click')
    await flushPromises()
    expect(mocks.regen).not.toHaveBeenCalled()
    expect(navigator.clipboard.writeText).toHaveBeenCalledWith(url)
  })

  it('disables copy when active URL cannot be recalled', async () => {
    sessionStorage.clear()
    forgetShareUrl('run-1', 'hg1', 1)
    mockClipboard(vi.fn().mockResolvedValue(undefined))

    const w = mount(GateShareLinkPanel, {
      props: {
        open: true,
        target: item({ shareLink: { state: 'active', ttlTier: '24h', canManage: true, canCreate: false } }),
      },
      global: { plugins: [i18n], stubs: { Teleport: true } },
    })
    await flushPromises()
    expect((w.get('[data-testid="gate-share-copy"]').element as HTMLButtonElement).disabled).toBe(true)
    expect(w.get('[data-testid="gate-share-copy-unavailable"]').text()).toMatch(/重新生成或撤销/)
    expect(mocks.regen).not.toHaveBeenCalled()
  })

  it('maps API error codes to locale text', async () => {
    mocks.create.mockRejectedValue(new Error('no_standard_action'))
    mockClipboard(vi.fn().mockResolvedValue(undefined))
    const w = mount(GateShareLinkPanel, {
      props: { open: true, target: item() },
      global: { plugins: [i18n], stubs: { Teleport: true } },
    })
    await w.get('[data-testid="gate-share-create"]').trigger('click')
    await flushPromises()
    expect(w.get('[data-testid="gate-share-error"]').text()).toContain('标准批准或驳回')
  })

  it('review kind calls reviews API and keeps primary actions at min-h-11', async () => {
    const token = 'ab'.repeat(32)
    const url = `https://app.example/public/gate-approvals#t=${token}`
    mocks.createReview.mockResolvedValue({ id: 'gsl-r1', url, ttlTier: '24h', expiresAt: '2026-08-10T00:00:00Z', state: 'active' })
    mockClipboard(vi.fn().mockResolvedValue(undefined))
    const w = mount(GateShareLinkPanel, {
      props: {
        open: true,
        kind: 'review',
        target: {
          runId: 'run-1',
          nodeId: 'research1',
          iteration: 1,
          shareLink: { state: 'none', canCreate: true },
          kind: 'review',
        },
      },
      global: { plugins: [i18n], stubs: { Teleport: true } },
    })
    const createBtn = w.get('[data-testid="gate-share-create"]')
    expect(createBtn.classes().join(' ')).toContain('min-h-11')
    await createBtn.trigger('click')
    await flushPromises()
    expect(mocks.createReview).toHaveBeenCalledWith('run-1', 'research1', '24h')
    expect(mocks.create).not.toHaveBeenCalled()
  })
})
