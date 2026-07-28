// @vitest-environment happy-dom
import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import AppTabs from './AppTabs.vue'

const tabs = [
  { id: 'a', label: 'Tab A' },
  { id: 'b', label: 'Tab B' },
]

describe('AppTabs', () => {
  it('highlights active tab and emits update on click', async () => {
    const wrapper = mount(AppTabs, {
      props: { tabs, modelValue: 'a' },
    })
    expect(wrapper.text()).toContain('Tab A')
    await wrapper.findAll('button')[1]!.trigger('click')
    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual(['b'])
    wrapper.unmount()
  })

  it('ghosted tab does not switch model and emits disabled-click', async () => {
    const wrapper = mount(AppTabs, {
      props: {
        tabs: [
          { id: 'review', label: '复审' },
          { id: 'gate', label: 'Gate', ghosted: true, disabled: true },
        ],
        modelValue: 'review',
      },
    })
    const gateBtn = wrapper.findAll('button')[1]!
    expect(gateBtn.classes().join(' ')).toMatch(/line-through/)
    await gateBtn.trigger('click')
    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
    expect(wrapper.emitted('disabled-click')?.[0]).toEqual(['gate'])
    wrapper.unmount()
  })
})
