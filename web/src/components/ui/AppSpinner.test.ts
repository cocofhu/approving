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
    expect(icon.classes().join(' ')).not.toContain('animate-spin')
    expect(icon.attributes('aria-hidden')).toBe('true')
    wrapper.unmount()
  })
})
