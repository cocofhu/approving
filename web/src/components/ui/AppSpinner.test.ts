// @vitest-environment happy-dom
import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import AppSpinner from './AppSpinner.vue'

describe('AppSpinner', () => {
  it('reuses Icon spinner and exposes controllable size without animate-spin', () => {
    const wrapper = mount(AppSpinner, { props: { size: 20 } })
    const icon = wrapper.find('svg')
    expect(icon.exists()).toBe(true)
    expect(icon.attributes('width')).toBe('20')
    expect(icon.classes()).toContain('app-spinner')
    expect(icon.classes()).toContain('icon-spinner')
    expect(icon.classes().join(' ')).not.toContain('animate-spin')
    expect(icon.attributes('aria-hidden')).toBe('true')
    // Dual-tone ring, distinguishable from chat bubble (plan g1.1 / g2.2).
    const html = icon.html()
    expect(html).toMatch(/<circle[^>]*r="9"/)
    expect(html).toMatch(/M21 12a9 9 0 0 1-9 9/)
    expect(html).not.toMatch(/M12 3a9 9 0 1 0 9 9/)
    expect(html).not.toMatch(/L4 20/)
    wrapper.unmount()
  })
})
