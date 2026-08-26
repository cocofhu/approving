// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { flushPromises, mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'
import ProjectAgentSelect from './ProjectAgentSelect.vue'

function mountSelect(props: {
  agents?: { name: string }[]
  modelValue?: string
  disabled?: boolean
}) {
  const i18n = createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...common, ...pages } },
  })
  return mount(ProjectAgentSelect, {
    props: {
      agents: props.agents ?? [
        { name: 'alpha-agent' },
        { name: 'beta-agent' },
      ],
      modelValue: props.modelValue ?? 'alpha-agent',
      disabled: props.disabled ?? false,
    },
    global: { plugins: [i18n] },
  })
}

describe('ProjectAgentSelect', () => {
  it('filters options by search keyword', async () => {
    const wrapper = mountSelect({})
    await flushPromises()
    await wrapper.get('[data-testid="project-agent-select-trigger"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-testid="project-agent-select-search"]').setValue('beta')
    await flushPromises()
    expect(wrapper.find('[data-testid="project-agent-select-option-beta-agent"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="project-agent-select-option-alpha-agent"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('shows empty result when keyword matches nothing', async () => {
    const wrapper = mountSelect({})
    await flushPromises()
    await wrapper.get('[data-testid="project-agent-select-trigger"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-testid="project-agent-select-search"]').setValue('missing')
    await flushPromises()
    expect(wrapper.find('[data-testid="project-agent-select-empty"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('emits selection on option click', async () => {
    const wrapper = mountSelect({})
    await flushPromises()
    await wrapper.get('[data-testid="project-agent-select-trigger"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-testid="project-agent-select-option-beta-agent"]').trigger('click')
    await flushPromises()
    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual(['beta-agent'])
    wrapper.unmount()
  })
})
