// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { flushPromises, mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import mcp from '@/locales/zh-CN/mcp.json'
import IntegrationsModal from './IntegrationsModal.vue'
import { BUILTIN_MCPS } from '@/data/mcp'

function mountModal(open = true) {
  const i18n = createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...common, ...mcp } },
  })
  return mount(IntegrationsModal, {
    props: { open },
    global: {
      plugins: [i18n],
      stubs: {
        Icon: true,
        AppModal: {
          props: ['open', 'title'],
          template:
            '<div v-if="open" data-testid="integrations-modal"><slot name="header" /><slot /></div>',
        },
      },
    },
  })
}

describe('IntegrationsModal (plan g2.2 / g2.3)', () => {
  it('shows MCP catalog when open', async () => {
    const w = mountModal(true)
    await flushPromises()
    expect(w.find('[data-testid="integrations-modal-catalog"]').exists()).toBe(true)
    expect(w.findAll('[data-testid="integrations-mcp-card"]').length).toBe(BUILTIN_MCPS.length)
    w.unmount()
  })

  it('switches to detail in the same modal and can return', async () => {
    const w = mountModal(true)
    await flushPromises()
    await w.findAll('[data-testid="integrations-mcp-card"]')[0]!.trigger('click')
    await flushPromises()
    expect(w.find('[data-testid="integrations-modal-detail"]').exists()).toBe(true)
    expect(w.find('[data-testid="integrations-modal-catalog"]').exists()).toBe(false)
    expect(w.find('[data-testid="integrations-modal-back"]').exists()).toBe(true)
    await w.find('[data-testid="integrations-modal-back"]').trigger('click')
    await flushPromises()
    expect(w.find('[data-testid="integrations-modal-catalog"]').exists()).toBe(true)
    w.unmount()
  })
})
