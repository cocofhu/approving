// @vitest-environment happy-dom
import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import AppSkeleton from './AppSkeleton.vue'

describe('AppSkeleton', () => {
  it('renders title + 2 cards + 5 rows with aria-hidden visual region', () => {
    const wrapper = mount(AppSkeleton)
    const root = wrapper.get('[data-testid="app-skeleton"]')
    expect(root.attributes('aria-hidden')).toBe('true')
    expect(wrapper.findAll('.app-skeleton__title')).toHaveLength(1)
    expect(wrapper.findAll('.app-skeleton__card')).toHaveLength(2)
    expect(wrapper.findAll('.app-skeleton__row')).toHaveLength(5)
    expect(wrapper.html()).not.toContain('animate-pulse')
    wrapper.unmount()
  })
})
