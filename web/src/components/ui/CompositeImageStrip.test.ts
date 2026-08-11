// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import CompositeImageStrip from './CompositeImageStrip.vue'

function mountStrip(value: unknown, size?: 'sm' | 'md' | 'lg') {
  const i18n = createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...common } },
  })
  return mount(CompositeImageStrip, {
    props: { value, ...(size ? { size } : {}) },
    global: {
      plugins: [i18n],
      stubs: {
        AppSpinner: { template: '<span data-testid="spinner-stub" />' },
      },
    },
  })
}

describe('CompositeImageStrip', () => {
  it('renders images from composite value (g1.1 success path)', async () => {
    const wrapper = mountStrip({
      text: 'hi',
      images: [{ mime: 'image/png', data: 'abc', name: 'a.png' }],
    })
    const img = wrapper.find('img')
    expect(img.exists()).toBe(true)
    expect(wrapper.find('[data-testid="composite-image-loading"]').exists()).toBe(true)
    await img.trigger('load')
    expect(wrapper.find('[data-testid="composite-image-ok"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="composite-image-failed"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="composite-image-loading"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('renders nothing when no images', () => {
    const wrapper = mountStrip('plain text')
    expect(wrapper.find('img').exists()).toBe(false)
    expect(wrapper.find('[data-testid="composite-image-strip"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('empty src: immediate permanent fail placeholder, not bare broken img (g2.1 / g1.2)', () => {
    const wrapper = mountStrip({
      text: '需求描述',
      images: [{ mime: 'image/png', name: 'missing.png' }],
    })
    expect(wrapper.find('[data-testid="composite-image-failed"]').exists()).toBe(true)
    expect(wrapper.find('img').exists()).toBe(false)
    expect(wrapper.text()).toContain('无法显示')
    expect(wrapper.text()).toContain('附件不可用')
    expect(wrapper.text()).not.toContain('重试')
    expect(wrapper.text()).not.toContain('404')
    expect(wrapper.find('button').exists()).toBe(false)
    wrapper.unmount()
  })

  it('@error / blob load failure → permanent fail without retry (g2.2 / g1.3)', async () => {
    const wrapper = mountStrip({
      text: 'feature',
      images: [{ mime: 'image/png', ref: 'blob:deadbeef', name: 'orphan.png' }],
    })
    expect(wrapper.find('[data-testid="composite-image-loading"]').exists()).toBe(true)
    const img = wrapper.find('img')
    expect(img.exists()).toBe(true)
    await img.trigger('error')
    expect(wrapper.find('[data-testid="composite-image-failed"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('无法显示')
    expect(wrapper.text()).toContain('附件不可用')
    expect(wrapper.text()).not.toContain('重试')
    expect(wrapper.text()).not.toContain('重新上传')
    expect(wrapper.find('button').exists()).toBe(false)
    expect(wrapper.find('[data-image-failed="1"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('mixed: success and permanent fail coexist per image (g2.3 / g1.1)', async () => {
    const wrapper = mountStrip({
      text: 'multi',
      images: [
        { mime: 'image/png', data: 'okbytes', name: 'ok.png' },
        { mime: 'image/png', name: 'empty.png' },
        { mime: 'image/png', ref: 'blob:orphan', name: 'gone.png' },
      ],
    })
    const imgs = wrapper.findAll('img')
    expect(imgs.length).toBe(2)
    await imgs[0]!.trigger('load')
    await imgs[1]!.trigger('error')

    expect(wrapper.findAll('[data-testid="composite-image-ok"]').length).toBe(1)
    expect(wrapper.findAll('[data-testid="composite-image-failed"]').length).toBe(2)
    expect(wrapper.text()).toContain('无法显示')
    expect(wrapper.text()).toContain('附件不可用')
    // Successful thumb still present; failures are independent placeholders
    expect(wrapper.find('[data-testid="composite-image-ok"] img').exists()).toBe(true)
    wrapper.unmount()
  })
})
