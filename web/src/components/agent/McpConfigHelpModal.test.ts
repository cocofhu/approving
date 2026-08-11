// @vitest-environment happy-dom
import { nextTick } from 'vue'
import { createI18n } from 'vue-i18n'
import { mount } from '@vue/test-utils'
import { afterEach, describe, expect, it } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'
import commonEn from '@/locales/en/common.json'
import pagesEn from '@/locales/en/pages.json'
import McpConfigHelpModal from './McpConfigHelpModal.vue'

function mountHelp(
  props: Record<string, unknown> = {},
  locale: 'zh-CN' | 'en' = 'zh-CN',
) {
  const i18n = createI18n({
    legacy: false,
    locale,
    messages: {
      'zh-CN': { ...common, ...pages },
      en: { ...commonEn, ...pagesEn },
    },
  })
  return mount(McpConfigHelpModal, {
    props: { open: true, configRoot: '/root/.codebuddy', ...props },
    global: {
      plugins: [i18n],
      stubs: { Transition: false },
    },
    attachTo: document.body,
  })
}

afterEach(() => {
  document.body.innerHTML = ''
})

describe('McpConfigHelpModal', () => {
  it('opens as 640 AppModal on run, with hint, Got-it footer and Esc close', async () => {
    const wrapper = mountHelp()
    const dialog = document.body.querySelector('.fixed.inset-0 .relative.flex.max-h-\\[88vh\\]') as HTMLElement
    expect(dialog).toBeTruthy()
    expect(dialog.style.maxWidth).toBe('640px')
    expect(document.body.textContent).toContain('MCP 配置帮助')
    expect(document.body.textContent).toContain('运行级变量')
    expect(document.body.textContent).toContain('Agent 平台 MCP')
    expect(document.body.textContent).toContain('知道了')
    expect(document.body.textContent).toContain('整份 mcp.json 由你配置')
    expect(document.body.textContent).toContain('/root/.codebuddy/mcp.json')
    expect(document.body.textContent).toContain('APPROVING_ARTIFACT_URL')
    expect(document.body.textContent).not.toContain('APPROVING_MEMORY_URL')
    expect(document.body.querySelector('[data-test="mcp-help-run"]')).toBeTruthy()
    expect(document.body.querySelector('[data-test="mcp-help-agent"]')).toBeFalsy()
    expect(document.body.querySelector('[data-help-chip="run"]')?.className).toContain('border-accent')

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    expect(wrapper.emitted('close')).toHaveLength(1)
    wrapper.unmount()
  })

  it('switches exclusively to agent docs without add buttons', async () => {
    const wrapper = mountHelp()
    await nextTick()
    const agentChip = document.body.querySelector('[data-test="mcp-help-chip-agent"]') as HTMLButtonElement
    agentChip.click()
    await nextTick()
    expect(wrapper.emitted('close')).toBeUndefined()
    expect(document.body.querySelector('[data-help-chip="agent"]')?.className).toContain('border-accent')
    expect(document.body.querySelector('[data-test="mcp-help-agent"]')).toBeTruthy()
    expect(document.body.querySelector('[data-test="mcp-help-run"]')).toBeFalsy()
    expect(document.body.textContent).toContain('APPROVING_MEMORY_URL')
    expect(document.body.textContent).toContain('pm-progress')
    expect(document.body.textContent).toContain('Agent 通用平台 MCP')
    expect(document.body.textContent).not.toContain('+ 添加长期记忆')
    expect(document.body.textContent).not.toContain('+ memory-store')
    wrapper.unmount()
  })

  it('resets to run each time it reopens', async () => {
    const wrapper = mountHelp()
    await nextTick()
    ;(document.body.querySelector('[data-test="mcp-help-chip-agent"]') as HTMLButtonElement).click()
    await nextTick()
    expect(document.body.querySelector('[data-test="mcp-help-agent"]')).toBeTruthy()

    await wrapper.setProps({ open: false })
    await nextTick()
    await wrapper.setProps({ open: true })
    await nextTick()
    expect(document.body.querySelector('[data-help-chip="run"]')?.className).toContain('border-accent')
    expect(document.body.querySelector('[data-test="mcp-help-run"]')).toBeTruthy()
    expect(document.body.querySelector('[data-test="mcp-help-agent"]')).toBeFalsy()
    wrapper.unmount()
  })

  it('English copy uses Help title / Got it / chips', async () => {
    const wrapper = mountHelp({}, 'en')
    expect(document.body.textContent).toContain('MCP config help')
    expect(document.body.textContent).toContain('Got it')
    expect(document.body.textContent).toContain('Run-scoped vars')
    expect(document.body.textContent).toContain('Agent platform MCP')
    wrapper.unmount()
  })
})
