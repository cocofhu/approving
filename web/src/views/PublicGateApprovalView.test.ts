// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'
import shell from '@/locales/zh-CN/shell.json'
import commonEn from '@/locales/en/common.json'
import pagesEn from '@/locales/en/pages.json'
import shellEn from '@/locales/en/shell.json'

const mocks = vi.hoisted(() => ({
  preview: vi.fn(),
  decide: vi.fn(),
}))

vi.mock('@/lib/gateShareLink', async () => {
  const actual = await vi.importActual<typeof import('@/lib/gateShareLink')>('@/lib/gateShareLink')
  return {
    ...actual,
    publicGateApi: {
      preview: mocks.preview,
      decide: mocks.decide,
    },
  }
})

vi.mock('@/lib/locale', async () => {
  const actual = await vi.importActual<typeof import('@/lib/locale')>('@/lib/locale')
  return {
    ...actual,
    applyPublicLocale: vi.fn().mockResolvedValue(undefined),
  }
})

import PublicGateApprovalView from './PublicGateApprovalView.vue'

function mountView(locale: 'zh-CN' | 'en' = 'zh-CN') {
  const i18n = createI18n({
    legacy: false,
    locale,
    messages: {
      'zh-CN': { ...common, ...pages, ...shell },
      en: { ...commonEn, ...pagesEn, ...shellEn },
    },
  })
  return mount(PublicGateApprovalView, { global: { plugins: [i18n] } })
}

beforeEach(() => {
  mocks.preview.mockReset()
  mocks.decide.mockReset()
  window.location.hash = ''
})

describe('PublicGateApprovalView', () => {
  it('renders desensitized active preview without run/project identifiers', async () => {
    window.location.hash = `#t=${'aa'.repeat(32)}`
    mocks.preview.mockResolvedValue({
      status: 'active',
      title: '审阅视觉稿',
      description: '请审阅产物',
      remainingSec: 3600,
      nonce: 'n1',
      actions: { approve: 'approve', reject: 'revise' },
      visualHtml: '<p>ok</p>',
      structured: { name: 'clarified_requirement.json', title: '外部一次审批', goals: ['g1'] },
    })
    const w = mountView()
    await flushPromises()
    expect(w.get('[data-testid="public-gate-title"]').text()).toBe('审阅视觉稿')
    expect(w.get('[data-testid="public-gate-root"]').text()).toContain('外部审批')
    expect(w.get('[data-testid="public-gate-root"]').text()).not.toMatch(/run-|projectId|10\.1\.2\.3|\/api\/blobs/)
    expect(w.find('[data-testid="public-gate-approve"]').exists()).toBe(true)
    expect(w.find('[data-testid="public-gate-reject"]').exists()).toBe(true)
  })

  it('shows expired / used / revoked / invalid separately', async () => {
    window.location.hash = `#t=${'bb'.repeat(32)}`
    mocks.preview.mockResolvedValue({ status: 'expired' })
    const w = mountView()
    await flushPromises()
    expect(w.get('[data-testid="public-gate-invalid"]').text()).toContain('已过期')
    expect(w.find('[data-testid="public-gate-approve"]').exists()).toBe(false)

    mocks.preview.mockResolvedValue({ status: 'used' })
    window.location.hash = `#t=${'cc'.repeat(32)}`
    const w2 = mountView()
    await flushPromises()
    expect(w2.get('[data-testid="public-gate-invalid"]').text()).toContain('已使用')

    mocks.preview.mockResolvedValue({ status: 'revoked' })
    const w3 = mountView()
    await flushPromises()
    expect(w3.get('[data-testid="public-gate-invalid"]').text()).toContain('已撤销')

    window.location.hash = ''
    const w4 = mountView()
    await flushPromises()
    expect(w4.get('[data-testid="public-gate-invalid"]').text()).toContain('无效')
  })

  it('english locale uses External approval badge', async () => {
    window.location.hash = `#t=${'dd'.repeat(32)}`
    mocks.preview.mockResolvedValue({
      status: 'active',
      title: 'Review',
      nonce: 'n2',
      remainingSec: 120,
      actions: { approve: 'approve' },
    })
    const w = mountView('en')
    await flushPromises()
    expect(w.get('[data-testid="public-gate-root"]').text()).toContain('External approval')
    expect(w.find('[data-testid="public-gate-reject"]').exists()).toBe(false)
  })
})
