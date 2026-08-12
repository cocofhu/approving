// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { mount, flushPromises } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import CompositeImageStrip from './CompositeImageStrip.vue'

/** Must match LOAD_TIMEOUT_MS in CompositeImageStrip.vue (8–15s band). */
const LOAD_TIMEOUT_MS = 12_000

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

const blobImg = (id: string, name = 'a.png') => ({
  mime: 'image/png',
  ref: `blob:${id}`,
  name,
})

describe('CompositeImageStrip', () => {
  afterEach(() => {
    vi.useRealTimers()
  })

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

  it('same src value replace keeps ok (poll refresh / g1.1)', async () => {
    const wrapper = mountStrip({
      text: 'feature',
      images: [blobImg('same-id')],
    })
    await wrapper.find('img').trigger('load')
    expect(wrapper.find('[data-testid="composite-image-ok"]').exists()).toBe(true)

    // Simulate Run detail 2s poll: new object graph, identical blob src
    await wrapper.setProps({
      value: {
        text: 'feature',
        images: [blobImg('same-id')],
      },
    })
    await flushPromises()

    expect(wrapper.find('[data-testid="composite-image-ok"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="composite-image-loading"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('src change leaves previous ok for a new loading/terminal (g1.2)', async () => {
    const wrapper = mountStrip({
      text: 'feature',
      images: [blobImg('first')],
    })
    await wrapper.find('img').trigger('load')
    expect(wrapper.find('[data-testid="composite-image-ok"]').exists()).toBe(true)

    await wrapper.setProps({
      value: {
        text: 'feature',
        images: [blobImg('second')],
      },
    })
    await flushPromises()

    // Must not keep the prior ok; happy-dom may @error the new URL immediately → failed.
    expect(wrapper.find('[data-testid="composite-image-ok"]').exists()).toBe(false)
    const loading = wrapper.find('[data-testid="composite-image-loading"]').exists()
    const failed = wrapper.find('[data-testid="composite-image-failed"]').exists()
    expect(loading || failed).toBe(true)
    wrapper.unmount()
  })

  it('one failed slot does not reset sibling ok on same-src poll (g1.2)', async () => {
    const wrapper = mountStrip({
      text: 'multi',
      images: [blobImg('ok-slot', 'ok.png'), blobImg('bad-slot', 'bad.png')],
    })
    const imgs = wrapper.findAll('img')
    await imgs[0]!.trigger('load')
    await imgs[1]!.trigger('error')
    expect(wrapper.findAll('[data-testid="composite-image-ok"]').length).toBe(1)
    expect(wrapper.findAll('[data-testid="composite-image-failed"]').length).toBe(1)

    await wrapper.setProps({
      value: {
        text: 'multi',
        images: [blobImg('ok-slot', 'ok.png'), blobImg('bad-slot', 'bad.png')],
      },
    })
    await flushPromises()

    expect(wrapper.findAll('[data-testid="composite-image-ok"]').length).toBe(1)
    expect(wrapper.findAll('[data-testid="composite-image-failed"]').length).toBe(1)
    expect(wrapper.find('[data-testid="composite-image-loading"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('complete+naturalWidth syncs to ok without @load (g2.1)', async () => {
    const completeDesc = Object.getOwnPropertyDescriptor(HTMLImageElement.prototype, 'complete')
    const widthDesc = Object.getOwnPropertyDescriptor(HTMLImageElement.prototype, 'naturalWidth')
    Object.defineProperty(HTMLImageElement.prototype, 'complete', {
      configurable: true,
      get: () => true,
    })
    Object.defineProperty(HTMLImageElement.prototype, 'naturalWidth', {
      configurable: true,
      get: () => 64,
    })
    try {
      const wrapper = mountStrip({
        text: 'cached',
        images: [blobImg('cached')],
      })
      await flushPromises()
      expect(wrapper.find('[data-testid="composite-image-ok"]').exists()).toBe(true)
      expect(wrapper.find('[data-testid="composite-image-loading"]').exists()).toBe(false)
      wrapper.unmount()
    } finally {
      if (completeDesc) Object.defineProperty(HTMLImageElement.prototype, 'complete', completeDesc)
      else delete (HTMLImageElement.prototype as { complete?: unknown }).complete
      if (widthDesc) Object.defineProperty(HTMLImageElement.prototype, 'naturalWidth', widthDesc)
      else delete (HTMLImageElement.prototype as { naturalWidth?: unknown }).naturalWidth
    }
  })

  it('loading timeout enters failed placeholder without retry (g2.2)', async () => {
    vi.useFakeTimers()
    const wrapper = mountStrip({
      text: 'slow',
      images: [blobImg('hang')],
    })
    expect(wrapper.find('[data-testid="composite-image-loading"]').exists()).toBe(true)

    await vi.advanceTimersByTimeAsync(LOAD_TIMEOUT_MS)
    await flushPromises()

    expect(wrapper.find('[data-testid="composite-image-failed"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="composite-image-loading"]').exists()).toBe(false)
    expect(wrapper.text()).toContain('无法显示')
    expect(wrapper.text()).toContain('附件不可用')
    expect(wrapper.text()).not.toContain('重试')
    expect(wrapper.find('button').exists()).toBe(false)
    wrapper.unmount()
  })
})
