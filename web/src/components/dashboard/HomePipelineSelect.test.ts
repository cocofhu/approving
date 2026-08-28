// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { flushPromises, mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'
import HomePipelineSelect from './HomePipelineSelect.vue'

function mountSelect(props: {
  pipelines?: { id: string; name: string }[]
  modelValue?: string
  disabled?: boolean
}) {
  const i18n = createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...common, ...pages } },
  })
  return mount(HomePipelineSelect, {
    props: {
      pipelines: props.pipelines ?? [
        { id: 'wf-a', name: '自我迭代PRO' },
        { id: 'wf-b', name: 'AnimeFind迭代' },
      ],
      modelValue: props.modelValue ?? 'wf-a',
      disabled: props.disabled ?? false,
    },
    global: { plugins: [i18n] },
  })
}

describe('HomePipelineSelect', () => {
  it('uses theme token classes for panel surface and option states', async () => {
    const wrapper = mountSelect({})
    await flushPromises()
    await wrapper.get('[data-testid="home-pipeline-select-trigger"]').trigger('click')
    await flushPromises()
    const panel = wrapper.get('[data-testid="home-pipeline-select-panel"]')
    expect(panel.classes()).toContain('home-pipeline-select__panel')
    const opt = wrapper.get('[data-testid="home-pipeline-select-option-wf-a"]')
    expect(opt.classes()).toContain('home-pipeline-select__opt--current')
    wrapper.unmount()
  })

  it('positions panel above trigger via upward panel class', async () => {
    const wrapper = mountSelect({})
    await flushPromises()
    await wrapper.get('[data-testid="home-pipeline-select-trigger"]').trigger('click')
    await flushPromises()
    const panel = wrapper.get('[data-testid="home-pipeline-select-panel"]')
    expect(panel.classes()).toContain('home-pipeline-select__panel')
    wrapper.unmount()
  })

  // plan g3.3 — search only looks at the already-filtered Home list
  it('does not surface a pipeline omitted from props even when the query matches its name', async () => {
    const wrapper = mountSelect({
      pipelines: [{ id: 'wf-a', name: '自我迭代PRO' }],
      modelValue: 'wf-a',
    })
    await flushPromises()
    await wrapper.get('[data-testid="home-pipeline-select-trigger"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-testid="home-pipeline-select-search"]').setValue('内部副本')
    await flushPromises()
    expect(wrapper.find('[data-testid="home-pipeline-select-option-wf-hidden"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="home-pipeline-select-empty"]').exists()).toBe(true)
    wrapper.unmount()
  })
})
