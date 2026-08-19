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
})
