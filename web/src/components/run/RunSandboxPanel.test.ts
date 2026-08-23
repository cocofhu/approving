// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'
import RunSandboxPanel from './RunSandboxPanel.vue'

function mountPanel(loading: boolean) {
  const i18n = createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...common, ...pages } },
  })
  return mount(RunSandboxPanel, {
    props: {
      loading,
      sbxLog: { content: 'line1\nline2', live: true, found: true },
      selStatus: 'running',
    },
    global: {
      plugins: [i18n],
      stubs: { Icon: true },
    },
  })
}

describe('RunSandboxPanel', () => {
  it('shows refresh-strip only for visible loading with existing content', () => {
    const visible = mountPanel(true)
    expect(visible.find('[data-testid="refresh-strip"]').exists()).toBe(true)
    visible.unmount()

    const silent = mountPanel(false)
    expect(silent.find('[data-testid="refresh-strip"]').exists()).toBe(false)
    silent.unmount()
  })
})
