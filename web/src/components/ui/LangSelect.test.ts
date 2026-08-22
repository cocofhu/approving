// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { mount, flushPromises } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import LangSelect from './LangSelect.vue'

function mountLang(modelValue: 'zh-CN' | 'en' = 'zh-CN') {
  const i18n = createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...common } },
  })
  return mount(LangSelect, {
    props: { modelValue },
    global: { plugins: [i18n], stubs: { Icon: true } },
  })
}

describe('LangSelect', () => {
  it('shows current locale label', () => {
    const wrapper = mountLang('zh-CN')
    expect(wrapper.text()).toContain('中文')
    wrapper.unmount()
  })

  it('emits locale change when option chosen', async () => {
    const wrapper = mountLang('zh-CN')
    await wrapper.find('button').trigger('click')
    const options = wrapper.findAll('[role="option"], button')
    const en = options.find((o) => o.text().includes('English'))
    expect(en).toBeTruthy()
    await en!.trigger('click')
    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual(['en'])
    wrapper.unmount()
  })

  it('ghost variant removes border for sidebar chrome (g2.1)', () => {
    const i18n = createI18n({
      legacy: false,
      locale: 'zh-CN',
      messages: { 'zh-CN': { ...common } },
    })
    const wrapper = mount(LangSelect, {
      props: { modelValue: 'zh-CN', variant: 'ghost' },
      global: { plugins: [i18n], stubs: { Icon: true } },
    })
    expect(wrapper.find('[data-variant="ghost"]').exists()).toBe(true)
    const trigger = wrapper.find('[data-testid="lang-select-trigger"]')
    expect(trigger.classes()).toContain('border-0')
    expect(trigger.classes()).toContain('h-8')
    expect(trigger.classes()).not.toContain('border-line')
    wrapper.unmount()
  })

  it('ghost variant teleports menu above trigger (g1.2)', async () => {
    const i18n = createI18n({
      legacy: false,
      locale: 'zh-CN',
      messages: { 'zh-CN': { ...common } },
    })
    const clip = document.createElement('div')
    clip.style.overflow = 'hidden'
    clip.style.height = '80px'
    document.body.appendChild(clip)

    const wrapper = mount(LangSelect, {
      props: { modelValue: 'zh-CN', variant: 'ghost' },
      global: { plugins: [i18n], stubs: { Icon: true, Teleport: false } },
      attachTo: clip,
    })

    const triggerEl = wrapper.find('[data-testid="lang-select-trigger"]').element as HTMLElement
    vi.spyOn(triggerEl, 'getBoundingClientRect').mockReturnValue({
      top: 300,
      left: 16,
      right: 120,
      bottom: 332,
      width: 104,
      height: 32,
      x: 16,
      y: 300,
      toJSON: () => ({}),
    } as DOMRect)

    await wrapper.find('[data-testid="lang-select-trigger"]').trigger('click')
    await flushPromises()

    const menu = document.body.querySelector('[data-testid="lang-select-menu"]') as HTMLElement
    expect(menu).toBeTruthy()
    expect(menu.getAttribute('data-placement')).toBe('above')
    expect(menu.style.position).toBe('fixed')
    expect(menu.textContent).toContain('English')
    expect(menu.textContent).toContain('中文')
    expect(Number.parseInt(menu.style.top, 10)).toBeLessThan(300)

    wrapper.unmount()
    clip.remove()
    document.body.innerHTML = ''
  })
})
