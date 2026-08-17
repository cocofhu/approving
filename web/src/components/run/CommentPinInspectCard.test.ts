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
})
