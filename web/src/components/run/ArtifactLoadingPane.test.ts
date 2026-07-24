// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'
import ArtifactLoadingPane from './ArtifactLoadingPane.vue'

function mountPane(props: { messageKey?: string; message?: string } = {}) {
  const i18n = createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...common, ...pages } },
  })
  return mount(ArtifactLoadingPane, {
    props,
    global: { plugins: [i18n], stubs: { Icon: true } },
  })
}

describe('ArtifactLoadingPane', () => {
  it('shows default loading message', () => {
    const wrapper = mountPane()
    expect(wrapper.text()).toMatch(/加载|loading/i)
    wrapper.unmount()
  })

  it('shows custom message prop', () => {
    const wrapper = mountPane({ message: '正在拉取产物…' })
    expect(wrapper.text()).toContain('正在拉取产物…')
    wrapper.unmount()
  })

  it('shows message from i18n key', () => {
    const wrapper = mountPane({ messageKey: 'pages.gateApproval.loadingArtifact' })
    expect(wrapper.text()).toMatch(/加载|loading/i)
    wrapper.unmount()
  })
})
