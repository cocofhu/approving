// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import {
  beginAutoLoad,
  blobMissingCacheDebug,
  isKnownMissing,
  markMissing,
  resetBlobMissingCacheForTests,
} from '@/lib/shared/blobMissingCache'
import CompositeImageStrip from './CompositeImageStrip.vue'

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

  it('poll object replace keeps failed; does not remount img (g2.1 / g2.2)', async () => {
    const orphan = {
      text: '需求描述',
      images: [{ mime: 'image/png', ref: 'blob:e54381fb9ce8471dbe0765d99fc0239f', name: 'a.png' }],
    }
    const wrapper = mountStrip(orphan)
    await wrapper.find('img').trigger('error')
    expect(wrapper.find('[data-testid="composite-image-failed"]').exists()).toBe(true)
    expect(isKnownMissing('e54381fb9ce8471dbe0765d99fc0239f')).toBe(true)

    // Simulate RunDetailView poll: new object, same blob ids
    await wrapper.setProps({
      value: {
        text: '需求描述',
        images: [{ mime: 'image/png', ref: 'blob:e54381fb9ce8471dbe0765d99fc0239f', name: 'a.png' }],
      },
    })
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
    // Only one surface may mount the requesting img for the first auto GET
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

    const { beginManualRetry, markLoaded } = await import('@/lib/shared/blobMissingCache')
    beginManualRetry(id)
    await wrapper.vm.$nextTick()
    // Still placeholder — no automatic GET while chat retries
    expect(wrapper.find('img').exists()).toBe(false)
    expect(wrapper.find('[data-testid="composite-image-failed"]').exists()).toBe(true)

    markLoaded(id)
    await wrapper.vm.$nextTick()
    expect(wrapper.find('[data-testid="composite-image-ok"]').exists()).toBe(true)
    expect(wrapper.find('img').exists()).toBe(true)
    wrapper.unmount()
  })
})
