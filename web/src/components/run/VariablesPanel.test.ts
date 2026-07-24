// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'
import type { RunVar } from '@/lib/types'
import VariablesPanel from './VariablesPanel.vue'

function mountPanel(vars: RunVar[] = []) {
  const i18n = createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...common, ...pages } },
  })
  return mount(VariablesPanel, {
    props: { vars },
    global: {
      plugins: [i18n],
      stubs: { Icon: true, VarValueDisplay: true, CompositeImageStrip: true },
    },
  })
}

describe('VariablesPanel', () => {
  it('shows empty state', () => {
    const wrapper = mountPanel([])
    expect(wrapper.text()).toContain('未定义全局变量')
    wrapper.unmount()
  })

  it('renders variable names and types', () => {
    const vars: RunVar[] = [
      { name: 'topic', type: 'string', value: 'hello' },
      { name: 'retry', type: 'int', value: 3 },
      { name: 'enabled', type: 'bool', value: true },
    ]
    const wrapper = mountPanel(vars)
    expect(wrapper.text()).toContain('topic')
    expect(wrapper.text()).toContain('string')
    expect(wrapper.text()).toContain('retry')
    expect(wrapper.text()).toContain('enabled')
    wrapper.unmount()
  })
})
