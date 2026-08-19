// @vitest-environment happy-dom
import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { createI18n } from 'vue-i18n'
import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'
import BrandLogo from './BrandLogo.vue'

describe('BrandLogo', () => {
  it('renders app name and tagline', () => {
    const i18n = createI18n({
      legacy: false,
      locale: 'zh-CN',
      messages: { 'zh-CN': { ...common, ...pages } },
    })
    const wrapper = mount(BrandLogo, {
      props: { size: 'md', align: 'center' },
      global: { plugins: [i18n] },
    })
    expect(wrapper.find('.brand-logo__name').text().length).toBeGreaterThan(0)
    expect(wrapper.find('.brand-logo__tagline').exists()).toBe(true)
    expect(wrapper.find('.brand-logo__mark').exists()).toBe(false)
    wrapper.unmount()
  })

  it('shows 28px square A mark only when showMark is true (g1.1 / g1.3)', () => {
    const i18n = createI18n({
      legacy: false,
      locale: 'zh-CN',
      messages: { 'zh-CN': { ...common, ...pages } },
    })
    const withMark = mount(BrandLogo, {
      props: { size: 'md', showMark: true },
      global: { plugins: [i18n] },
    })
    const mark = withMark.find('.brand-logo__mark')
    expect(mark.exists()).toBe(true)
    expect(mark.text()).toBe('A')
    expect(mark.classes()).toContain('brand-logo__mark')
    expect(withMark.classes()).toContain('brand-logo--with-mark')
    withMark.unmount()

    const without = mount(BrandLogo, {
      props: { size: 'lg', align: 'center' },
      global: { plugins: [i18n] },
    })
    expect(without.find('.brand-logo__mark').exists()).toBe(false)
    without.unmount()
  })

  it('does not leak mark to login, boot shell, or mobile drawer (g1.3)', () => {
    const dir = dirname(fileURLToPath(import.meta.url))
    const sidebar = readFileSync(join(dir, 'AppSidebar.vue'), 'utf8')
    const shell = readFileSync(join(dir, 'AppShell.vue'), 'utf8')
    const login = readFileSync(join(dir, '../../views/LoginView.vue'), 'utf8')
    const boot = readFileSync(join(dir, '../../../index.html'), 'utf8')
    expect(sidebar).toMatch(/show-mark/)
    expect(shell).not.toMatch(/show-mark|showMark/)
    expect(login).not.toMatch(/show-mark|showMark/)
    expect(boot).not.toMatch(/brand-logo__mark/)
  })
})
