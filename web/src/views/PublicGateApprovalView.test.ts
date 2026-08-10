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
import type { VueWrapper } from '@vue/test-utils'

const mounted: VueWrapper[] = []

function mountView(locale: 'zh-CN' | 'en' = 'zh-CN') {
  const i18n = createI18n({
    legacy: false,
    locale,
    messages: {
      'zh-CN': { ...common, ...pages, ...shell },
      en: { ...commonEn, ...pagesEn, ...shellEn },
    },
  })
  const w = mount(PublicGateApprovalView, { global: { plugins: [i18n] } })
  mounted.push(w)
  return w
}

beforeEach(() => {
  mocks.preview.mockReset()
  mocks.decide.mockReset()
  window.location.hash = ''
  localStorage.clear()
  setTheme('dark')
})

afterEach(() => {
  while (mounted.length) mounted.pop()?.unmount()
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

  it('review preview only shows confirm-and-advance', async () => {
    window.location.hash = `#t=${'ee'.repeat(32)}`
    mocks.preview.mockResolvedValue({
      status: 'active',
      kind: 'review',
      title: '调研',
      description: '待复审',
      remainingSec: 3600,
      nonce: 'n3',
      actions: { confirm: 'confirm' },
      structured: { name: 'research.json', title: '调研摘要' },
    })
    const w = mountView()
    await flushPromises()
    expect(w.get('[data-testid="public-gate-root"]').text()).toContain('外部复审')
    expect(w.find('[data-testid="public-gate-confirm"]').exists()).toBe(true)
    expect(w.get('[data-testid="public-gate-confirm"]').text()).toContain('确认并流转')
    expect(w.find('[data-testid="public-gate-approve"]').exists()).toBe(false)
    expect(w.find('[data-testid="public-gate-reject"]').exists()).toBe(false)
    expect(w.find('[data-testid="public-gate-name"]').exists()).toBe(false)
    expect(w.find('[data-testid="public-gate-comment"]').exists()).toBe(false)

    mocks.decide.mockResolvedValue({ status: 'confirmed', action: 'confirm' })
    await w.get('[data-testid="public-gate-confirm"]').trigger('click')
    await flushPromises()
    expect(mocks.decide).toHaveBeenCalledWith(
      expect.objectContaining({ action: 'confirm' }),
      expect.any(AbortSignal),
    )
    expect(w.get('[data-testid="public-gate-done"]').text()).toContain('已确认')
  })

  it('gate preview still has approve/reject plus optional name/comment', async () => {
    window.location.hash = `#t=${'ff'.repeat(32)}`
    mocks.preview.mockResolvedValue({
      status: 'active',
      kind: 'human_gate',
      title: '审阅视觉稿',
      nonce: 'n4',
      actions: { approve: 'approve', reject: 'revise' },
    })
    const w = mountView()
    await flushPromises()
    expect(w.find('[data-testid="public-gate-approve"]').exists()).toBe(true)
    expect(w.find('[data-testid="public-gate-reject"]').exists()).toBe(true)
    expect(w.find('[data-testid="public-gate-name"]').exists()).toBe(true)
    expect(w.find('[data-testid="public-gate-comment"]').exists()).toBe(true)
    expect(w.find('[data-testid="public-gate-confirm"]').exists()).toBe(false)
  })

  it('review busy/validation keeps confirm and does not clear the link', async () => {
    window.location.hash = `#t=${'aa'.repeat(32)}`
    mocks.preview.mockResolvedValue({
      status: 'active',
      kind: 'review',
      title: '调研',
      nonce: 'n-busy',
      actions: { confirm: 'confirm' },
    })
    const w = mountView()
    await flushPromises()
    mocks.decide.mockResolvedValueOnce({
      status: 'busy',
      error: 'review_busy',
      message: '复审进行中，请稍后再试',
    })
    await w.get('[data-testid="public-gate-confirm"]').trigger('click')
    await flushPromises()
    expect(w.get('[data-testid="public-gate-error"]').text()).toContain('复审进行中')
    expect(w.find('[data-testid="public-gate-confirm"]').exists()).toBe(true)
    expect(w.find('[data-testid="public-gate-done"]').exists()).toBe(false)

    mocks.decide.mockResolvedValueOnce({
      status: 'validation_failed',
      error: 'review_validation_failed',
      message: '产物校验未通过，链接仍有效，请稍后重试',
    })
    await w.get('[data-testid="public-gate-confirm"]').trigger('click')
    await flushPromises()
    expect(w.get('[data-testid="public-gate-error"]').text()).toContain('产物校验')
    expect(w.find('[data-testid="public-gate-confirm"]').exists()).toBe(true)
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
    expect(mocks.decide).toHaveBeenCalledWith(
      {
        token: 'aa'.repeat(32),
        action: 'approve',
        comment: '',
        name: '',
        nonce: 'n1',
      },
      expect.any(AbortSignal),
    )
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
    expect(mocks.decide).toHaveBeenCalledWith(
      {
        token: 'aa'.repeat(32),
        action: 'revise',
        comment: '需要改文案',
        name: 'Jordan',
        nonce: 'n1',
      },
      expect.any(AbortSignal),
    )
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

  it('clears previous preview before loading and discards stale hash race', async () => {
    const tokenA = 'aa'.repeat(32)
    const tokenB = 'bb'.repeat(32)
    let resolveA!: (v: unknown) => void
    mocks.preview.mockImplementation((tok: string) => {
      if (tok === tokenA) {
        return new Promise((resolve) => {
          resolveA = resolve
        })
      }
      return Promise.resolve({
        status: 'active',
        title: 'TokenB-only',
        nonce: 'nB',
        remainingSec: 3600,
        actions: { approve: 'approve', reject: 'revise' },
      })
    })

    window.location.hash = `#t=${tokenA}`
    const w = mountView()
    await flushPromises()
    expect(w.get('[data-testid="public-gate-loading"]').exists()).toBe(true)
    expect(w.get('[data-testid="public-gate-loading"]').attributes('role')).toBe('status')
    expect(w.get('[data-testid="public-gate-loading"]').text()).toContain('加载中…')
    expect(w.get('[data-testid="public-gate-root"]').text()).not.toMatch(/run-|projectId|TokenA|内部复审|审阅视觉稿/)

    window.location.hash = `#t=${tokenB}`
    window.dispatchEvent(new HashChangeEvent('hashchange'))
    await flushPromises()
    expect(w.get('[data-testid="public-gate-gate-title"]').text()).toBe('TokenB-only')
    expect(w.text()).not.toContain('TokenA-SECRET')

    resolveA({
      status: 'active',
      title: 'TokenA-SECRET',
      nonce: 'nA',
      visualHtml: '<p>run-old-internal</p>',
      structured: { title: 'internal review run-1' },
      actions: { approve: 'approve' },
    })
    await flushPromises()
    expect(w.text()).not.toContain('TokenA-SECRET')
    expect(w.text()).not.toContain('run-old-internal')
    expect(w.text()).not.toMatch(/run-|internal review/)
    expect(w.get('[data-testid="public-gate-gate-title"]').text()).toBe('TokenB-only')
  })

  it('shows submitting copy on both actions and ignores duplicate clicks', async () => {
    window.location.hash = `#t=${'aa'.repeat(32)}`
    mocks.preview.mockResolvedValue({
      status: 'active',
      title: '审阅视觉稿',
      nonce: 'n1',
      remainingSec: 3600,
      actions: { approve: 'approve', reject: 'revise' },
    })
    let resolveDecide!: (v: unknown) => void
    mocks.decide.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          resolveDecide = resolve
        }),
    )
    const w = mountView()
    await flushPromises()
    await w.get('[data-testid="public-gate-approve"]').trigger('click')
    await w.get('[data-testid="public-gate-approve"]').trigger('click')
    await w.get('[data-testid="public-gate-reject"]').trigger('click')
    await flushPromises()
    expect(mocks.decide).toHaveBeenCalledTimes(1)
    expect(w.get('[data-testid="public-gate-approve"]').text()).toContain('提交中…')
    expect(w.get('[data-testid="public-gate-reject"]').text()).toContain('提交中…')
    expect(w.get('[data-testid="public-gate-approve"]').attributes('aria-busy')).toBe('true')
    expect((w.get('[data-testid="public-gate-approve"]').element as HTMLButtonElement).disabled).toBe(true)
    expect((w.get('[data-testid="public-gate-reject"]').element as HTMLButtonElement).disabled).toBe(true)
    expect(w.get('[data-testid="public-gate-approve"]').classes().join(' ')).toMatch(/min-h-11/)
    resolveDecide({ status: 'approved' })
    await flushPromises()
    expect(w.get('[data-testid="public-gate-done"]').text()).toContain('已确认')
  })

  it('sandboxes preview/decide errors and never renders e.message', async () => {
    window.location.hash = `#t=${'aa'.repeat(32)}`
    mocks.preview.mockRejectedValue(new Error('internal stack /api/runs/run-123 projectId=p1'))
    const w = mountView()
    await flushPromises()
    expect(w.get('[data-testid="public-gate-network-error"]').text()).toContain('网络错误')
    expect(w.text()).not.toContain('internal stack')
    expect(w.text()).not.toContain('run-123')
    expect(w.text()).not.toContain('projectId')
    expect(w.find('[data-testid="public-gate-invalid"]').exists()).toBe(false)

    mocks.preview.mockResolvedValue({
      status: 'active',
      title: '审阅视觉稿',
      nonce: 'n1',
      remainingSec: 3600,
      actions: { approve: 'approve', reject: 'revise' },
    })
    await w.get('[data-testid="public-gate-network-retry"]').trigger('click')
    await flushPromises()
    mocks.decide.mockRejectedValueOnce(new Error('postgres connection refused at 10.1.2.3'))
    await w.get('[data-testid="public-gate-approve"]').trigger('click')
    await flushPromises()
    expect(w.get('[role="alert"]').text()).toContain('网络错误')
    expect(w.text()).not.toContain('postgres')
    expect(w.text()).not.toContain('10.1.2.3')
  })
})
