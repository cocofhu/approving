// @vitest-environment happy-dom
import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import CompositeImageStrip from './CompositeImageStrip.vue'

describe('CompositeImageStrip', () => {
  it('renders images from composite value', () => {
    const wrapper = mount(CompositeImageStrip, {
      props: {
        value: {
          text: 'hi',
          images: [{ mime: 'image/png', data: 'abc', name: 'a.png' }],
        },
      },
    })
    expect(wrapper.findAll('img').length).toBe(1)
    wrapper.unmount()
  })

  it('renders nothing when no images', () => {
    const wrapper = mount(CompositeImageStrip, {
      props: { value: 'plain text' },
    })
    expect(wrapper.find('img').exists()).toBe(false)
    wrapper.unmount()
  })
})
