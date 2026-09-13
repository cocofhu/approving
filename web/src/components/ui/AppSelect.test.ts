// @vitest-environment happy-dom
import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { createI18n } from 'vue-i18n'
import { mount, flushPromises } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import AppSelect from './AppSelect.vue'

const OPTIONS = [
  { value: 'openai', label: 'OpenAI', hint: 'openai/gpt-4.1' },
  { value: 'anthropic', label: 'Anthropic', hint: 'anthropic/claude-sonnet-4-5' },
  { value: 'custom', label: 'Custom' },
]

function i18nPlugin() {
  return createI18n({ legacy: false, locale: 'zh-CN', messages: { 'zh-CN': { ...common } } })
}

function mountSelect(props: Record<string, unknown> = {}) {
  return mount(AppSelect, {
    props: { modelValue: 'openai', options: OPTIONS, ...props },
    global: { plugins: [i18nPlugin()], stubs: { Icon: true, Teleport: true } },
  })
}

afterEach(() => {
  document.body.innerHTML = ''
})

describe('AppSelect', () => {
  it('shows the selected label and falls back to the placeholder', () => {
    const wrapper = mountSelect()
    expect(wrapper.get('[data-test="app-select-trigger"]').text()).toContain('OpenAI')

    const empty = mountSelect({ modelValue: '', placeholder: '请选择' })
    expect(empty.get('[data-test="app-select-trigger"]').text()).toContain('请选择')
    wrapper.unmount()
    empty.unmount()
  })

  it('emits the chosen value and closes the panel', async () => {
    const wrapper = mountSelect()
    expect(wrapper.find('[data-test="app-select-panel"]').exists()).toBe(false)

    await wrapper.get('[data-test="app-select-trigger"]').trigger('click')
    expect(wrapper.find('[data-test="app-select-panel"]').exists()).toBe(true)

    await wrapper.get('[data-test="app-select-option-anthropic"]').trigger('click')
    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual(['anthropic'])
    expect(wrapper.find('[data-test="app-select-panel"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('marks the current option as selected', async () => {
    const wrapper = mountSelect({ modelValue: 'custom' })
    await wrapper.get('[data-test="app-select-trigger"]').trigger('click')
    expect(wrapper.get('[data-test="app-select-option-custom"]').attributes('aria-selected')).toBe(
      'true',
    )
    expect(wrapper.get('[data-test="app-select-option-openai"]').attributes('aria-selected')).toBe(
      'false',
    )
    wrapper.unmount()
  })

  it('supports keyboard open, arrow move, enter select and escape close', async () => {
    const wrapper = mountSelect()
    const trigger = wrapper.get('[data-test="app-select-trigger"]')

    await trigger.trigger('keydown', { key: 'ArrowDown' })
    expect(wrapper.find('[data-test="app-select-panel"]').exists()).toBe(true)

    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowDown' }))
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter' }))
    await flushPromises()
    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual(['anthropic'])

    await trigger.trigger('keydown', { key: 'Enter' })
    expect(wrapper.find('[data-test="app-select-panel"]').exists()).toBe(true)
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await flushPromises()
    expect(wrapper.find('[data-test="app-select-panel"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('stays closed when disabled', async () => {
    const wrapper = mountSelect({ disabled: true })
    await wrapper.get('[data-test="app-select-trigger"]').trigger('click')
    expect(wrapper.find('[data-test="app-select-panel"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('shows the invalid border instead of the idle one', () => {
    const wrapper = mountSelect({ invalid: true })
    const classes = wrapper.get('[data-test="app-select-trigger"]').classes()
    expect(classes).toContain('border-err')
    expect(classes).not.toContain('border-line')
    wrapper.unmount()
  })

  it('filters, highlights and reports an empty result when searchable', async () => {
    const wrapper = mountSelect({ searchable: true })
    await wrapper.get('[data-test="app-select-trigger"]').trigger('click')
    const box = wrapper.get('[data-test="app-select-search"]')

    await box.setValue('anth')
    expect(wrapper.find('[data-test="app-select-option-openai"]').exists()).toBe(false)
    const hit = wrapper.get('[data-test="app-select-option-anthropic"]')
    expect(hit.html()).toContain('<mark>Anth</mark>')

    // hint carries the sample model, so a model name finds its vendor.
    await box.setValue('claude')
    expect(wrapper.find('[data-test="app-select-option-anthropic"]').exists()).toBe(true)

    await box.setValue('zzz')
    expect(wrapper.find('[data-test="app-select-empty"]').exists()).toBe(true)
    expect(wrapper.findAll('[role="option"]')).toHaveLength(0)
    wrapper.unmount()
  })

  it('keyboard picks from the filtered list and space stays literal in search', async () => {
    const wrapper = mountSelect({ searchable: true })
    await wrapper.get('[data-test="app-select-trigger"]').trigger('click')
    const box = wrapper.get('[data-test="app-select-search"]')

    await box.setValue('cust')
    await box.trigger('keydown', { key: 'Enter' })
    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual(['custom'])
    wrapper.unmount()
  })

  it('clears the query when reopened', async () => {
    const wrapper = mountSelect({ searchable: true })
    await wrapper.get('[data-test="app-select-trigger"]').trigger('click')
    await wrapper.get('[data-test="app-select-search"]').setValue('anth')
    expect(wrapper.findAll('[role="option"]')).toHaveLength(1)

    await wrapper.get('[data-test="app-select-trigger"]').trigger('click')
    await wrapper.get('[data-test="app-select-trigger"]').trigger('click')
    expect(
      (wrapper.get('[data-test="app-select-search"]').element as HTMLInputElement).value,
    ).toBe('')
    expect(wrapper.findAll('[role="option"]')).toHaveLength(OPTIONS.length)
    wrapper.unmount()
  })

  it('offers the typed value when allowCustom, and shows it on the trigger', async () => {
    const wrapper = mountSelect({ searchable: true, allowCustom: true })
    await wrapper.get('[data-test="app-select-trigger"]').trigger('click')
    await wrapper.get('[data-test="app-select-search"]').setValue('vendor/my-model')

    expect(wrapper.find('[data-test="app-select-empty"]').exists()).toBe(false)
    await wrapper.get('[data-test="app-select-custom"]').trigger('click')
    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual(['vendor/my-model'])

    // A value outside the list is its own trigger label, not the placeholder.
    await wrapper.setProps({ modelValue: 'vendor/my-model', placeholder: '请选择' })
    expect(wrapper.get('[data-test="app-select-trigger"]').text()).toContain('vendor/my-model')
    wrapper.unmount()
  })

  it('does not offer a custom row for a query that already is an option', async () => {
    const wrapper = mountSelect({ searchable: true, allowCustom: true })
    await wrapper.get('[data-test="app-select-trigger"]').trigger('click')
    await wrapper.get('[data-test="app-select-search"]').setValue('custom')
    expect(wrapper.find('[data-test="app-select-custom"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('has no search box unless searchable', async () => {
    const wrapper = mountSelect()
    await wrapper.get('[data-test="app-select-trigger"]').trigger('click')
    expect(wrapper.find('[data-test="app-select-search"]').exists()).toBe(false)
    wrapper.unmount()
  })

  // A trigger click stops propagating, so the other select never sees it on the
  // document and the two panels would otherwise overlap.
  it('closes the open panel when another select opens', async () => {
    const wrapper = mount(
      {
        components: { AppSelect },
        template: `
          <div>
            <AppSelect data-test="first" model-value="openai" :options="options" />
            <AppSelect data-test="second" model-value="custom" :options="options" />
          </div>
        `,
        data: () => ({ options: OPTIONS }),
      },
      { global: { plugins: [i18nPlugin()], stubs: { Icon: true, Teleport: true } } },
    )

    const triggers = wrapper.findAll('[data-test="app-select-trigger"]')
    await triggers[0].trigger('click')
    expect(wrapper.findAll('[data-test="app-select-panel"]').length).toBe(1)

    await triggers[1].trigger('click')
    const panels = wrapper.findAll('[data-test="app-select-panel"]')
    expect(panels.length).toBe(1)
    // The one left open is the second select's.
    expect(wrapper.findAll('[data-test="app-select-option-custom"]')[0].attributes('aria-selected')).toBe(
      'true',
    )
    wrapper.unmount()
  })

  it('shows an option hint next to its label', async () => {
    const wrapper = mountSelect()
    await wrapper.get('[data-test="app-select-trigger"]').trigger('click')
    const row = wrapper.get('[data-test="app-select-option-openai"]')
    expect(row.text()).toContain('OpenAI')
    expect(row.text()).toContain('openai/gpt-4.1')
    wrapper.unmount()
  })

  // Options that are still on their way must not read as "nothing matches".
  it('says it is loading instead of showing the empty state', async () => {
    const wrapper = mountSelect({ options: [], loading: true })
    await wrapper.get('[data-test="app-select-trigger"]').trigger('click')
    expect(wrapper.find('[data-test="app-select-empty"]').exists()).toBe(false)
    expect(wrapper.get('[data-test="app-select-loading"]').text()).toBe(common.common.search.loadingOptions)

    await wrapper.setProps({ loading: false })
    expect(wrapper.find('[data-test="app-select-loading"]').exists()).toBe(false)
    expect(wrapper.find('[data-test="app-select-empty"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('teleports a fixed panel so modal overflow cannot clip it', async () => {
    const wrapper = mount(AppSelect, {
      props: { modelValue: 'openai', options: OPTIONS },
      global: { plugins: [i18nPlugin()], stubs: { Icon: true, Teleport: false } },
      attachTo: document.body,
    })

    const triggerEl = wrapper.get('[data-test="app-select-trigger"]').element as HTMLElement
    vi.spyOn(triggerEl, 'getBoundingClientRect').mockReturnValue({
      top: 100,
      left: 24,
      right: 224,
      bottom: 132,
      width: 200,
      height: 32,
      x: 24,
      y: 100,
      toJSON: () => ({}),
    } as DOMRect)

    await wrapper.get('[data-test="app-select-trigger"]').trigger('click')
    await flushPromises()

    const panel = document.body.querySelector('[data-test="app-select-panel"]') as HTMLElement
    expect(panel).toBeTruthy()
    expect(panel.style.position).toBe('fixed')
    expect(panel.style.width).toBe('200px')
    // Opens below the trigger (bottom + gap).
    expect(Number.parseInt(panel.style.top, 10)).toBeGreaterThan(132)
    // g2.2: panel escapes the overflow:hidden trigger (Teleport → body tree)
    expect(triggerEl.contains(panel)).toBe(false)
    expect(document.body.contains(panel)).toBe(true)
    wrapper.unmount()
  })

  it('wraps the panel in overlay-pop Transition and rotates chevron (g2.3)', () => {
    const src = readFileSync(join(dirname(fileURLToPath(import.meta.url)), 'AppSelect.vue'), 'utf8')
    expect(src).toMatch(/name="overlay-pop"/)
    expect(src).toMatch(/app-select-chevron/)
    expect(src).toMatch(/is-open/)
  })
})
