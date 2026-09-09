// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { flushPromises, mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import mcp from '@/locales/zh-CN/mcp.json'
import IntegrationsPanel from './IntegrationsPanel.vue'
import { BUILTIN_MCPS } from '@/data/mcp'

function mountPanel() {
  const i18n = createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...common, ...mcp } },
  })
  return mount(IntegrationsPanel, {
    global: {
      plugins: [i18n],
      stubs: { Icon: true },
    },
  })
}

describe('IntegrationsPanel', () => {
  it('shows the complete MCP catalog without a modal', async () => {
    const w = mountPanel()
    await flushPromises()
    expect(w.find('[data-testid="integrations-panel"]').exists()).toBe(true)
    expect(w.find('[data-testid="integrations-panel-catalog"]').exists()).toBe(true)
    expect(w.findAll('[data-testid="integrations-mcp-card"]').length).toBe(BUILTIN_MCPS.length)
    expect(w.find('[role="dialog"]').exists()).toBe(false)
    w.unmount()
  })

  it('switches to detail in the same pane and can return', async () => {
    const w = mountPanel()
    await flushPromises()
    await w.findAll('[data-testid="integrations-mcp-card"]')[0]!.trigger('click')
    await flushPromises()
    expect(w.find('[data-testid="integrations-panel-detail"]').exists()).toBe(true)
    expect(w.find('[data-testid="integrations-panel-catalog"]').exists()).toBe(false)
    expect(w.find('[data-testid="integrations-panel-back"]').exists()).toBe(true)
    await w.find('[data-testid="integrations-panel-back"]').trigger('click')
    await flushPromises()
    expect(w.find('[data-testid="integrations-panel-catalog"]').exists()).toBe(true)
    w.unmount()
  })
})
