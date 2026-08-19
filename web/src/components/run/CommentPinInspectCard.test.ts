// @vitest-environment happy-dom
import { nextTick } from 'vue'
import { createI18n } from 'vue-i18n'
import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'
import CommentPinInspectCard from './CommentPinInspectCard.vue'

function mountCard(props: Record<string, unknown> = {}) {
  const i18n = createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...common, ...pages } },
  })
  return mount(CommentPinInspectCard, {
    props: {
      open: true,
      selector: 'h1.display.reveal',
      screenshotMissing: true,
      initialComment: '',
      anchor: { left: 10, top: 20, width: 1211, height: 278 },
      styleInfo: {
        color: 'rgb(26, 26, 26)',
        fontSize: '118px',
        fontWeight: '700',
        fontFamily: '"Bodoni Moda", serif',
        lineHeight: '159.3px',
      },
      ...props,
    },
    global: { plugins: [i18n] },
  })
}

describe('CommentPinInspectCard', () => {
  it('disables comment and send-to-chat while empty, then emits after typing', async () => {
    const wrapper = mountCard()
    const save = wrapper.find('[data-testid="comment-pin-save"]')
    const send = wrapper.find('[data-testid="comment-pin-send-chat"]')
    expect(save.text()).toBe('评论')
    expect(send.text()).toBe('发送到聊天')
    expect(save.attributes('disabled')).toBeDefined()
    expect(send.attributes('disabled')).toBeDefined()

    await wrapper.find('[data-testid="comment-pin-input"]').setValue('  加大标题  ')
    await nextTick()
    expect(save.attributes('disabled')).toBeUndefined()
    expect(send.attributes('disabled')).toBeUndefined()

    await save.trigger('click')
    expect(wrapper.emitted('save')?.[0]).toEqual(['加大标题'])

    await send.trigger('click')
    expect(wrapper.emitted('send-chat')?.[0]).toEqual(['加大标题'])
    wrapper.unmount()
  })

  it('renders Open Design style rows from bounds + computed style', () => {
    const wrapper = mountCard()
    const rows = wrapper.find('[data-testid="comment-pin-style-rows"]')
    expect(rows.exists()).toBe(true)
    expect(rows.text()).toContain('Size')
    expect(rows.text()).toContain('1211x278')
    expect(rows.text()).toContain('Color')
    expect(rows.text()).toContain('#1A1A1A')
    expect(rows.text()).toContain('Font')
    expect(rows.text()).toContain('118px')
    expect(rows.text()).toContain('700')
    expect(rows.text()).toContain('Bodoni Moda')
    expect(rows.text()).toContain('Line')
    expect(rows.text()).toContain('159.3px')
    wrapper.unmount()
  })

  it('locks Demo dark elevated tokens even when html.light is set (g1.1/g1.3)', () => {
    document.documentElement.classList.add('light')
    const lightOverride = document.createElement('style')
    lightOverride.textContent =
      'html.light{--c-elevated:244 244 245;--c-base:250 250 251;--c-txt:24 24 27;--c-txt2:82 82 91;--c-txt3:161 161 170;}'
    document.head.appendChild(lightOverride)
    const wrapper = mountCard()
    const card = wrapper.get('[data-testid="comment-pin-inspect-card"]').element as HTMLElement
    expect(card.classList.contains('comment-pin-inspect-card')).toBe(true)
    expect(card.style.getPropertyValue('--c-elevated').trim()).toBe('28 28 33')
    expect(card.style.getPropertyValue('--c-base').trim()).toBe('10 10 11')
    expect(card.style.getPropertyValue('--c-txt').trim()).toBe('237 237 240')
    expect(card.style.getPropertyValue('--c-txt2').trim()).toBe('161 161 170')
    expect(card.style.getPropertyValue('--c-txt3').trim()).toBe('110 110 120')
    expect(card.style.getPropertyValue('--c-line').trim()).toBe('38 38 43')
    expect(card.style.getPropertyValue('--c-line-strong').trim()).toBe('54 54 62')
    expect(wrapper.find('[data-testid="comment-pin-save"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="comment-pin-send-chat"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="comment-pin-input"]').exists()).toBe(true)
    wrapper.unmount()
    lightOverride.remove()
    document.documentElement.classList.remove('light')
  })

  it('keeps dashed thumb at h-11 and allows pin without screenshot (g2.3/g3.2)', async () => {
    const wrapper = mountCard({ screenshotMissing: true, imageDataUrl: '' })
    const thumb = wrapper.get('[data-testid="comment-pin-thumb"]')
    expect(thumb.classes()).toContain('h-11')
    expect(thumb.classes()).toContain('border-dashed')
    expect(thumb.text()).toContain('无截图')
    await wrapper.find('[data-testid="comment-pin-input"]').setValue('仍可落钉')
    await wrapper.find('[data-testid="comment-pin-save"]').trigger('click')
    expect(wrapper.emitted('save')?.[0]).toEqual(['仍可落钉'])
    wrapper.unmount()
  })
})
