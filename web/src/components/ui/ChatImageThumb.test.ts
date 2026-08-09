// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import ChatImageThumb from './ChatImageThumb.vue'

function mountThumb(props: Record<string, unknown> = {}) {
  const i18n = createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...common } },
  })
  return mount(ChatImageThumb, {
    props: {
      src: 'data:image/png;base64,AAA',
      label: '看板截图.png',
      ...props,
    },
    global: { plugins: [i18n] },
  })
}

describe('ChatImageThumb', () => {
  it('previewable: hover hint, size md, emits preview', async () => {
    const wrapper = mountThumb({ mode: 'previewable', size: 'md', testId: 'thumb-md' })
    const btn = wrapper.find('[data-testid="thumb-md"]')
    expect(btn.element.tagName).toBe('BUTTON')
    expect(btn.classes().join(' ')).toMatch(/h-20/)
    expect(btn.classes().join(' ')).toMatch(/w-20/)
    expect(btn.classes().join(' ')).toMatch(/hover:border-accent/)
    expect(btn.classes().join(' ')).toMatch(/cursor-pointer/)
    expect(btn.text()).toContain('点击放大')
    expect(btn.text()).not.toContain('不可预览')
    expect(btn.attributes('aria-label')).toBe('预览看板截图.png')
    await btn.trigger('click')
    expect(wrapper.emitted('preview')).toHaveLength(1)
    wrapper.unmount()
  })

  it('locked: unavailable overlay, no preview emit, sm/xs sizes', async () => {
    const locked = mountThumb({ mode: 'locked', size: 'sm', testId: 'thumb-locked' })
    const el = locked.find('[data-testid="thumb-locked"]')
    expect(el.element.tagName).toBe('DIV')
    expect(el.classes().join(' ')).toMatch(/h-14/)
    expect(el.text()).toContain('不可预览')
    expect(el.text()).not.toContain('点击放大')
    await el.trigger('click')
    expect(locked.emitted('preview')).toBeUndefined()
    locked.unmount()

    const xs = mountThumb({ mode: 'previewable', size: 'xs', testId: 'thumb-xs' })
    expect(xs.find('[data-testid="thumb-xs"]').classes().join(' ')).toMatch(/h-8/)
    expect(xs.find('[data-testid="thumb-xs"]').classes().join(' ')).toMatch(/w-8/)
    xs.unmount()
  })

  it('forwards thumbClass for surface corner rules', () => {
    const wrapper = mountThumb({ thumbClass: 'rounded-md', testId: 'thumb-round' })
    expect(wrapper.find('[data-testid="thumb-round"]').classes()).toContain('rounded-md')
    wrapper.unmount()
  })
})
