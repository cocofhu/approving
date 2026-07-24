// @vitest-environment happy-dom
import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import EmptyState from './EmptyState.vue'

describe('EmptyState', () => {
  it('renders title and optional description', () => {
    const wrapper = mount(EmptyState, {
      props: { title: '暂无数据', desc: '请先创建' },
      global: { stubs: { Icon: true } },
    })
    expect(wrapper.text()).toContain('暂无数据')
    expect(wrapper.text()).toContain('请先创建')
    wrapper.unmount()
  })
})
