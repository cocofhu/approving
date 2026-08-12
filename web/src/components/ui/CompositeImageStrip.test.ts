// @vitest-environment happy-dom
import { mount, flushPromises } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import {
  beginAutoLoad,
  beginManualRetry,
  blobMissingCacheDebug,
  isKnownMissing,
  markLoaded,
  markMissing,
  resetBlobMissingCacheForTests,
} from '@/lib/shared/blobMissingCache'
import CompositeImageStrip from './CompositeImageStrip.vue'

/** Must match LOAD_TIMEOUT_MS in CompositeImageStrip.vue (8–15s band). */
const LOAD_TIMEOUT_MS = 12_000

beforeEach(() => {
  resetBlobMissingCacheForTests()
})

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
    expect(wrapper.find('[data-testid="composite-image-ok"] img').exists()).toBe(true)
    wrapper.unmount()
  })

  it('poll object replace keeps failed; does not remount img (g2.1 / g2.2)', async () => {
    const orphan = {
      text: '需求描述',
      images: [{ mime: 'image/png', ref: 'blob:e54381fb9ce8471dbe0765d99fc0239f', name: 'a.png' }],
    }
    const wrapper = mountStrip(orphan)
    await wrapper.find('img').trigger('error')
    expect(wrapper.find('[data-testid="composite-image-failed"]').exists()).toBe(true)
    expect(isKnownMissing('e54381fb9ce8471dbe0765d99fc0239f')).toBe(true)

    await wrapper.setProps({
      value: {
        text: '需求描述',
        images: [{ mime: 'image/png', ref: 'blob:e54381fb9ce8471dbe0765d99fc0239f', name: 'a.png' }],
      },
    })
    await flushPromises()

    expect(wrapper.find('img').exists()).toBe(false)
    expect(wrapper.find('[data-testid="composite-image-failed"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('无法显示')
    expect(wrapper.text()).toContain('附件不可用')
    wrapper.unmount()
  })

  it('knownMissing: direct placeholder, zero img src (g2.2)', () => {
    markMissing('5b32f70529a64bdebafade19ca497a35')
    const wrapper = mountStrip({
      text: 'x',
      images: [{ mime: 'image/png', ref: 'blob:5b32f70529a64bdebafade19ca497a35', name: 'b.png' }],
    })
    expect(wrapper.find('img').exists()).toBe(false)
    expect(wrapper.find('[data-testid="composite-image-failed"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('dual strips: same missing id auto begin ≤1 (g1.2)', async () => {
    const value = {
      text: 'dual',
      images: [{ mime: 'image/png', ref: 'blob:6f70eb9a67f2432983d16bc26a1bb420', name: 'c.png' }],
    }
    const a = mountStrip(value)
    const b = mountStrip(value)
    const imgs = [...a.findAll('img'), ...b.findAll('img')]
    expect(imgs.length).toBe(1)
    expect(blobMissingCacheDebug().inflight).toContain('6f70eb9a67f2432983d16bc26a1bb420')
    await imgs[0]!.trigger('error')
    expect(a.find('[data-testid="composite-image-failed"]').exists()).toBe(true)
    expect(b.find('[data-testid="composite-image-failed"]').exists()).toBe(true)
    expect(a.find('img').exists()).toBe(false)
    expect(b.find('img').exists()).toBe(false)
    a.unmount()
    b.unmount()
  })

  it('same src value replace keeps ok (poll refresh / g1.1)', async () => {
    const wrapper = mountStrip({
      text: 'feature',
      images: [blobImg('same-id')],
    })
    await wrapper.find('img').trigger('load')
    expect(wrapper.find('[data-testid="composite-image-ok"]').exists()).toBe(true)

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

    expect(wrapper.find('[data-testid="composite-image-ok"]').exists()).toBe(false)
    const loading = wrapper.find('[data-testid="composite-image-loading"]').exists()
    const failed = wrapper.find('[data-testid="composite-image-failed"]').exists()
    expect(loading || failed).toBe(true)
    wrapper.unmount()
  })

  it('ok image survives poll; new blob id still loads (g2.3)', async () => {
    const wrapper = mountStrip({
      text: 't',
      images: [{ mime: 'image/png', data: 'okbytes', name: 'ok.png' }],
    })
    await wrapper.find('img').trigger('load')
    expect(wrapper.find('[data-testid="composite-image-ok"]').exists()).toBe(true)

    await wrapper.setProps({
      value: {
        text: 't',
        images: [
          { mime: 'image/png', data: 'okbytes', name: 'ok.png' },
          { mime: 'image/png', ref: 'blob:newid001', name: 'new.png' },
        ],
      },
    })
    await flushPromises()

    expect(wrapper.find('[data-testid="composite-image-ok"]').exists()).toBe(true)
    expect(wrapper.findAll('img').length).toBe(2)
    expect(beginAutoLoad('newid001')).toBe('blocked_pending')
    wrapper.unmount()
  })

  it('chat retry in progress does not remount strip img; markLoaded syncs (g3.3)', async () => {
    const id = 'e54381fb9ce8471dbe0765d99fc0239f'
    const wrapper = mountStrip({
      text: 'x',
      images: [{ mime: 'image/png', ref: `blob:${id}`, name: 'a.png' }],
    })
    await wrapper.find('img').trigger('error')
    expect(wrapper.find('img').exists()).toBe(false)

    beginManualRetry(id)
    await wrapper.vm.$nextTick()
    expect(wrapper.find('img').exists()).toBe(false)
    expect(wrapper.find('[data-testid="composite-image-failed"]').exists()).toBe(true)

    markLoaded(id)
    await wrapper.vm.$nextTick()
    expect(wrapper.find('[data-testid="composite-image-ok"]').exists()).toBe(true)
    expect(wrapper.find('img').exists()).toBe(true)
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
