// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import commonEn from '@/locales/en/common.json'
import RefreshStrip from './RefreshStrip.vue'

function mountStrip(locale: 'zh-CN' | 'en' = 'zh-CN', message?: string) {
  const i18n = createI18n({
    legacy: false,
    locale,
    messages: { 'zh-CN': { ...common }, en: { ...commonEn } },
  })
  return mount(RefreshStrip, {
    props: message ? { message } : {},
    global: { plugins: [i18n], stubs: { Icon: true } },
  })
}

describe('RefreshStrip', () => {
  it('shows Demo-locked refreshing copy with live region', () => {
    const w = mountStrip()
    expect(w.get('[data-testid="refresh-strip"]').attributes('role')).toBe('status')
    expect(w.get('[data-testid="refresh-strip"]').attributes('aria-live')).toBe('polite')
    expect(w.text()).toBe('正在刷新…')
    w.unmount()
  })

  it('renders English Refreshing…', () => {
    const w = mountStrip('en')
    expect(w.text()).toBe('Refreshing…')
    w.unmount()
  })

  it('accepts a custom message without changing a11y', () => {
    const w = mountStrip('zh-CN', '正在刷新端口列表…')
    expect(w.text()).toBe('正在刷新端口列表…')
    expect(w.get('[data-testid="refresh-strip"]').attributes('role')).toBe('status')
    w.unmount()
  })
})
