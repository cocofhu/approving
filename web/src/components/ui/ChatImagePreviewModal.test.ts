// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import ChatImagePreviewModal from './ChatImagePreviewModal.vue'

function mountPreview(props: Record<string, unknown> = {}) {
  const i18n = createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...common } },
  })
  return mount(ChatImagePreviewModal, {
    props: {
      open: true,
      src: 'data:image/png;base64,AAA',
      label: '看板截图.png',
      testIdPrefix: 'chat-image-preview',
      ...props,
    },
    global: {
      plugins: [i18n],
      stubs: {
        Icon: true,
        Transition: false,
      },
    },
    attachTo: document.body,
  })
}

describe('ChatImagePreviewModal', () => {
  it('opens single-image dialog with title, no gallery, object-contain', () => {
    const wrapper = mountPreview()
    expect(document.body.textContent).toContain('图片预览 · 看板截图.png')
    const img = document.body.querySelector('[data-testid="chat-image-preview-img"]') as HTMLImageElement
    expect(img).toBeTruthy()
    expect(img.getAttribute('src')).toBe('data:image/png;base64,AAA')
    expect(img.className).toMatch(/object-contain/)
    expect(img.className).toMatch(/max-h-\[74vh\]/)
    expect(document.body.querySelector('[data-testid="chat-image-preview-prev"]')).toBeNull()
    expect(document.body.querySelector('[data-testid="chat-image-preview-next"]')).toBeNull()
    const dialog = document.body.querySelector('[role="dialog"]')
    expect(dialog).toBeTruthy()
    expect(dialog?.getAttribute('aria-modal')).toBe('true')
    wrapper.unmount()
  })

  it('closes via × and backdrop; Esc does not close', async () => {
    const wrapper = mountPreview()
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    expect(wrapper.emitted('close')).toBeUndefined()

    const closeBtn = document.body.querySelector('.fixed.inset-0.z-50 .h-14 button') as HTMLButtonElement
    closeBtn.click()
    expect(wrapper.emitted('close')).toHaveLength(1)

    wrapper.unmount()
    const again = mountPreview()
    const backdrop = document.body.querySelector('.fixed.inset-0.z-50 > .absolute.inset-0') as HTMLElement
    backdrop.click()
    expect(again.emitted('close')).toHaveLength(1)
    again.unmount()
  })

  it('shows load-failed placeholder and still closes', async () => {
    const wrapper = mountPreview()
    const img = document.body.querySelector('[data-testid="chat-image-preview-img"]') as HTMLImageElement
    expect(img).toBeTruthy()
    img.dispatchEvent(new Event('error'))
    await wrapper.vm.$nextTick()
    const failed = document.body.querySelector('[data-testid="chat-image-preview-failed"]')
    expect(failed).toBeTruthy()
    expect(failed?.textContent).toContain('图片加载失败')
    expect(document.body.querySelector('[data-testid="chat-image-preview-img"]')).toBeNull()
    const closeBtn = document.body.querySelector('.fixed.inset-0.z-50 .h-14 button') as HTMLButtonElement
    closeBtn.click()
    expect(wrapper.emitted('close')).toHaveLength(1)
    wrapper.unmount()
  })
})
