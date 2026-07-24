// @vitest-environment happy-dom
import { defineComponent } from 'vue'
import { createI18n } from 'vue-i18n'
import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'
import ClarifyDemoFrame from './ClarifyDemoFrame.vue'

const HtmlPreviewStub = defineComponent({
  name: 'HtmlPreview',
  props: { html: String, mode: String, enlargeable: Boolean, modalTitle: String },
  methods: {
    openEnlarge() {
      this.$emit('enlarge')
    },
  },
  template: '<div data-testid="html-preview">{{ html }}</div>',
})

function mountFrame(props: { label: string; html: string; highlighted?: boolean; selected?: boolean }) {
  const i18n = createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...common, ...pages } },
  })
  return mount(ClarifyDemoFrame, {
    props,
    global: {
      plugins: [i18n],
      stubs: { Icon: true, HtmlPreview: HtmlPreviewStub },
    },
  })
}

describe('ClarifyDemoFrame', () => {
  it('renders label and html preview', () => {
    const wrapper = mountFrame({
      label: '方案 A',
      html: '<!doctype html><html><body>demo</body></html>',
    })
    expect(wrapper.text()).toContain('方案 A')
    expect(wrapper.find('[data-testid="html-preview"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('shows selected marker and highlighted border', () => {
    const wrapper = mountFrame({
      label: '方案 B',
      html: '<html></html>',
      highlighted: true,
      selected: true,
    })
    expect(wrapper.text()).toMatch(/已选|✓/)
    expect(wrapper.find('div.border-accent').exists() || wrapper.html().includes('border-accent')).toBe(true)
    wrapper.unmount()
  })

  it('calls enlarge on preview via button', async () => {
    const wrapper = mountFrame({ label: 'Demo', html: '<html></html>' })
    const enlargeBtn = wrapper.findAll('button').find((b) => b.text().includes('放大') || b.text().includes('Enlarge'))
    expect(enlargeBtn).toBeTruthy()
    await enlargeBtn!.trigger('click')
    wrapper.unmount()
  })
})
