// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'
import shell from '@/locales/zh-CN/shell.json'
import commonEn from '@/locales/en/common.json'
import pagesEn from '@/locales/en/pages.json'
import shellEn from '@/locales/en/shell.json'
import { setTheme } from '@/lib/theme'

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
  localStorage.clear()
  setTheme('dark')
})

afterEach(() => {
  document.documentElement.classList.remove('light')
})

describe('PublicGateApprovalView', () => {
  it('renders one-shot confirm chrome with gate title as meta only', async () => {
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
    expect(document.documentElement.classList.contains('light')).toBe(true)
    expect(localStorage.getItem('approving-theme')).toBe('dark')
    expect(w.get('[data-testid="public-gate-title"]').text()).toBe('请确认本次交付')
    expect(w.get('[data-testid="public-gate-gate-title"]').text()).toBe('审阅视觉稿')
    expect(w.get('[data-testid="public-gate-badge"]').text()).toBe('外部一次决策')
    expect(w.get('[data-testid="public-gate-root"]').text()).toContain('待确认的内容')
    expect(w.get('[data-testid="public-gate-root"]').text()).toContain('链接仅可使用一次')
    expect(w.get('[data-testid="public-gate-root"]').text()).toContain('无需登录')
    expect(w.get('[data-testid="public-gate-root"]').text()).toContain('预览已脱敏')
    expect(w.get('[data-testid="public-gate-root"]').text()).not.toMatch(/run-|projectId|10\.1\.2\.3|\/api\/blobs/)
    expect(w.get('[data-testid="public-gate-root"]').text()).not.toMatch(/确认并流转|取点标注|发送就地改|人工评审/)
    expect(w.get('[data-testid="public-gate-approve"]').text()).toBe('确认')
    expect(w.get('[data-testid="public-gate-reject"]').text()).toBe('驳回并说明原因')
    expect(w.find('[data-testid="html-preview-inline"]').exists()).toBe(true)
    expect(w.find('[data-testid="html-preview-toolbar"]').exists()).toBe(false)
    expect(w.find('[data-testid="html-preview-enlarge"]').exists()).toBe(false)
    expect(w.find('[data-testid="html-preview-inspect-toggle"]').exists()).toBe(false)
    expect(w.get('[data-testid="public-gate-content"]').text()).toContain('外部一次审批')
  })

  it('hides reject when PreviewDTO has no fail action', async () => {
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
    expect(w.get('[data-testid="public-gate-badge"]').text()).toBe('One-time external decision')
    expect(w.get('[data-testid="public-gate-title"]').text()).toBe('Please confirm this delivery')
    expect(w.get('[data-testid="public-gate-approve"]').text()).toBe('Confirm')
    expect(w.find('[data-testid="public-gate-reject"]').exists()).toBe(false)
  })

  it('allows confirm with empty comment and still maps approve action', async () => {
    window.location.hash = `#t=${'aa'.repeat(32)}`
    mocks.preview.mockResolvedValue({
      status: 'active',
      title: '审阅视觉稿',
      nonce: 'n1',
      remainingSec: 3600,
      actions: { approve: 'approve', reject: 'revise' },
    })
    mocks.decide.mockResolvedValue({ status: 'approved' })
    const w = mountView()
    await flushPromises()
    await w.get('[data-testid="public-gate-approve"]').trigger('click')
    await flushPromises()
    expect(mocks.decide).toHaveBeenCalledWith({
      token: 'aa'.repeat(32),
      action: 'approve',
      comment: '',
      name: '',
      nonce: 'n1',
    })
    expect(w.get('[data-testid="public-gate-done"]').text()).toContain('已确认')
    expect(w.find('[data-testid="public-gate-approve"]').exists()).toBe(false)
    expect(w.find('[data-testid="public-gate-reject"]').exists()).toBe(false)
  })

  it('blocks reject without comment and keeps token usable', async () => {
    window.location.hash = `#t=${'aa'.repeat(32)}`
    mocks.preview.mockResolvedValue({
      status: 'active',
      title: '审阅视觉稿',
      nonce: 'n1',
      remainingSec: 3600,
      actions: { approve: 'approve', reject: 'revise' },
    })
    const w = mountView()
    await flushPromises()
    await w.get('[data-testid="public-gate-reject"]').trigger('click')
    await flushPromises()
    expect(mocks.decide).not.toHaveBeenCalled()
    expect(w.get('[role="alert"]').text()).toContain('驳回必须填写意见')
    expect(w.find('[data-testid="public-gate-approve"]').exists()).toBe(true)
    expect(w.find('[data-testid="public-gate-reject"]').exists()).toBe(true)

    await w.get('[data-testid="public-gate-comment"]').setValue('需要改文案')
    await w.get('[data-testid="public-gate-name"]').setValue('Jordan')
    mocks.decide.mockResolvedValue({ status: 'rejected' })
    await w.get('[data-testid="public-gate-reject"]').trigger('click')
    await flushPromises()
    expect(mocks.decide).toHaveBeenCalledWith({
      token: 'aa'.repeat(32),
      action: 'revise',
      comment: '需要改文案',
      name: 'Jordan',
      nonce: 'n1',
    })
    expect(w.get('[data-testid="public-gate-done"]').text()).toContain('已驳回')
  })

  it('shows expired / used / revoked / invalid with same chrome and no write actions', async () => {
    window.location.hash = `#t=${'bb'.repeat(32)}`
    mocks.preview.mockResolvedValue({ status: 'expired' })
    const w = mountView()
    await flushPromises()
    expect(w.get('[data-testid="public-gate-badge"]').text()).toBe('外部一次决策')
    expect(w.get('[data-testid="public-gate-invalid"]').text()).toContain('已过期')
    expect(w.get('[data-testid="public-gate-invalid"]').text()).toContain('一次性链接已过期')
    expect(w.find('[data-testid="public-gate-approve"]').exists()).toBe(false)
    expect(w.find('[data-testid="public-gate-reject"]').exists()).toBe(false)
    expect(w.get('[data-testid="public-gate-root"]').text()).not.toMatch(/取点标注|发送就地改|确认并流转/)

    mocks.preview.mockResolvedValue({ status: 'used' })
    window.location.hash = `#t=${'cc'.repeat(32)}`
    const w2 = mountView()
    await flushPromises()
    expect(w2.get('[data-testid="public-gate-badge"]').text()).toBe('外部一次决策')
    expect(w2.get('[data-testid="public-gate-invalid"]').text()).toContain('已使用')
    expect(w2.get('[data-testid="public-gate-invalid"]').text()).toContain('不能再次确认或驳回')
    expect(w2.find('[data-testid="public-gate-approve"]').exists()).toBe(false)

    mocks.preview.mockResolvedValue({ status: 'revoked' })
    const w3 = mountView()
    await flushPromises()
    expect(w3.get('[data-testid="public-gate-invalid"]').text()).toContain('已撤销')
    expect(w3.get('[data-testid="public-gate-invalid"]').text()).toContain('已撤销此链接')

    window.location.hash = ''
    const w4 = mountView()
    await flushPromises()
    expect(w4.get('[data-testid="public-gate-invalid"]').text()).toContain('无效')
    expect(w4.get('[data-testid="public-gate-invalid"]').text()).toContain('外部一次决策')
  })
})
