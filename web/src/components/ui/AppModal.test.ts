// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import AppModal from './AppModal.vue'

function mountModal(props: Record<string, unknown> = {}) {
  const i18n = createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...common } },
  })
  return mount(AppModal, {
    props: { open: true, title: '标题', ...props },
    slots: { default: '<p data-testid="body">内容</p>' },
    global: {
      plugins: [i18n],
      stubs: {
        Icon: { template: '<span data-testid="icon-close" />' },
        Transition: false,
      },
    },
    attachTo: document.body,
  })
}

describe('AppModal', () => {
  it('shows title and body when open', () => {
    const wrapper = mountModal({ open: true })
    expect(document.body.textContent).toContain('标题')
    expect(document.body.querySelector('[data-testid="body"]')).toBeTruthy()
    wrapper.unmount()
  })

  it('emits close on backdrop click by default', async () => {
    const wrapper = mountModal({ open: true })
    const overlay = document.body.querySelector('.bg-black\\/60') as HTMLElement
    expect(overlay).toBeTruthy()
    overlay.click()
    expect(wrapper.emitted('close')).toHaveLength(1)
    wrapper.unmount()
  })

  it('does not emit close on backdrop click when closeOnBackdrop is false', async () => {
    const wrapper = mountModal({ open: true, closeOnBackdrop: false })
    const overlay = document.body.querySelector('.bg-black\\/60') as HTMLElement
    expect(overlay).toBeTruthy()
    overlay.click()
    expect(wrapper.emitted('close')).toBeUndefined()
    wrapper.unmount()
  })

  it('still emits close from the header close button when closeOnBackdrop is false', async () => {
    const wrapper = mountModal({ open: true, closeOnBackdrop: false })
    const closeBtn = document.body.querySelector(
      '.relative.flex.max-h-\\[88vh\\] button',
    ) as HTMLButtonElement | null
    expect(closeBtn).toBeTruthy()
    closeBtn!.click()
    expect(wrapper.emitted('close')).toHaveLength(1)
    wrapper.unmount()
  })

  it('does not emit close on Escape by default', async () => {
    const wrapper = mountModal({ open: true })
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    expect(wrapper.emitted('close')).toBeUndefined()
    wrapper.unmount()
  })

  it('emits close on Escape when closeOnEsc is true', async () => {
    const wrapper = mountModal({ open: true, closeOnEsc: true })
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    expect(wrapper.emitted('close')).toHaveLength(1)
    wrapper.unmount()
  })

  it('exposes dialog semantics with accessible name from title', () => {
    const wrapper = mountModal({ open: true, title: '图片预览 · 看板.png' })
    const dialog = document.body.querySelector('[role="dialog"]')
    expect(dialog).toBeTruthy()
    expect(dialog?.getAttribute('aria-modal')).toBe('true')
    const labelledBy = dialog?.getAttribute('aria-labelledby')
    expect(labelledBy).toBeTruthy()
    const titleEl = document.getElementById(labelledBy!)
    expect(titleEl?.textContent).toContain('图片预览 · 看板.png')
    wrapper.unmount()
  })
})

