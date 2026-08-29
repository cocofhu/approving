// @vitest-environment happy-dom
import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import Icon from './Icon.vue'

describe('Icon', () => {
  it('renders svg for known icon name', () => {
    const wrapper = mount(Icon, { props: { name: 'check', size: 16 } })
    expect(wrapper.find('svg').exists()).toBe(true)
    wrapper.unmount()
  })

  it('falls back for unknown icon', () => {
    const wrapper = mount(Icon, { props: { name: 'unknown-icon-xyz' } })
    expect(wrapper.find('svg').exists()).toBe(true)
    wrapper.unmount()
  })

  it('renders lock glyph used by 403 states', () => {
    const wrapper = mount(Icon, { props: { name: 'lock', size: 18 } })
    expect(wrapper.find('svg').exists()).toBe(true)
    expect(wrapper.html()).toMatch(/rect|M8 11V8/)
    wrapper.unmount()
  })

  it('renders panel-left glyph used to hide desktop nav (g2.1 / g5.1)', () => {
    const wrapper = mount(Icon, { props: { name: 'panel-left', size: 18 } })
    expect(wrapper.find('svg').exists()).toBe(true)
    expect(wrapper.html()).toMatch(/rx="2"|M9 3v18|m16 15-3-3 3-3/)
    expect(wrapper.html()).not.toMatch(/M9 4v16/)
    wrapper.unmount()
  })

  it('spinner is a dual-tone ring (track circle + 90° arc), not the old bubble-like 270° arc (plan g1.1)', () => {
    const wrapper = mount(Icon, { props: { name: 'spinner', size: 18 } })
    const html = wrapper.html()
    expect(html).toMatch(/<circle[^>]*cx="12"[^>]*cy="12"[^>]*r="9"/)
    expect(html).toMatch(/M21 12a9 9 0 0 1-9 9/)
    expect(html).not.toMatch(/M12 3a9 9 0 1 0 9 9/)
    // Must not look like chat bubble (no tail path).
    expect(html).not.toMatch(/M21 12a8 8 0 0 1-11\.3 7\.3/)
    expect(html).not.toMatch(/L4 20/)
    expect(wrapper.find('svg').classes()).toContain('icon-spinner')
    wrapper.unmount()
  })

  it('spinner root SVG spins around center without inline wobble (plan g1.2)', () => {
    const wrapper = mount(Icon, { props: { name: 'spinner', size: 18 } })
    const svg = wrapper.find('svg')
    expect(svg.classes()).toContain('icon-spinner')
    expect(svg.attributes('style')).toMatch(/display:\s*block/)
    expect(svg.attributes('style')).toMatch(/transform-origin:\s*center/)
    wrapper.unmount()
  })
})
