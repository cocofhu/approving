// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import MermaidDiagram from './MermaidDiagram.vue'
import { cssTokenColor, isLightTheme, mermaidThemeName, themeVars } from './mermaidTheme'

const mermaidInitialize = vi.fn()
const mermaidRender = vi.fn()

vi.mock('mermaid', () => ({
  default: {
    initialize: (...args: unknown[]) => mermaidInitialize(...args),
    render: (...args: unknown[]) => mermaidRender(...args),
  },
}))

function mountDiagram(source = 'flowchart LR\n  A-->B') {
  const i18n = createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: {
      'zh-CN': {
        pages: { plan: { diagramFallback: '图渲染失败,显示源码' } },
      },
    },
  })
  return mount(MermaidDiagram, {
    props: {
      diagram: { format: 'mermaid', source },
      jsonPath: 'architecture.diagram',
    },
    global: { plugins: [i18n] },
  })
}

describe('MermaidDiagram theme helpers (g2.1/g2.3)', () => {
  beforeEach(() => {
    document.documentElement.classList.remove('light')
    document.documentElement.style.cssText = ''
    mermaidInitialize.mockReset()
    mermaidRender.mockReset()
    mermaidRender.mockResolvedValue({ svg: '<svg data-ok="1"></svg>' })
  })

  afterEach(() => {
    document.documentElement.classList.remove('light')
    document.documentElement.style.cssText = ''
  })

  it('isLightTheme follows html.light', () => {
    expect(isLightTheme()).toBe(false)
    document.documentElement.classList.add('light')
    expect(isLightTheme()).toBe(true)
  })

  it('cssTokenColor converts --c-* channel tokens to rgb()', () => {
    document.documentElement.style.setProperty('--c-base', '250 250 251')
    expect(cssTokenColor('--c-base', 'rgb(0, 0, 0)')).toBe('rgb(250, 250, 251)')
  })

  it('themeVars under light uses shallow --c-* tokens not dark fallbacks', () => {
    document.documentElement.classList.add('light')
    document.documentElement.style.setProperty('--c-base', '250 250 251')
    document.documentElement.style.setProperty('--c-txt', '24 24 27')
    document.documentElement.style.setProperty('--c-elevated', '244 244 245')
    document.documentElement.style.setProperty('--c-line', '229 229 232')
    document.documentElement.style.setProperty('--c-line-strong', '212 212 216')
    const vars = themeVars()
    expect(vars.background).toBe('rgb(250, 250, 251)')
    expect(vars.primaryTextColor).toBe('rgb(24, 24, 27)')
    expect(vars.background).not.toMatch(/10,\s*10,\s*11/)
    expect(mermaidThemeName()).toBe('base')
  })

  it('themeVars under dark keeps dark tokens', () => {
    document.documentElement.style.setProperty('--c-base', '10 10 11')
    document.documentElement.style.setProperty('--c-txt', '237 237 240')
    const vars = themeVars()
    expect(vars.background).toBe('rgb(10, 10, 11)')
    expect(vars.primaryTextColor).toBe('rgb(237, 237, 240)')
    expect(mermaidThemeName()).toBe('dark')
  })

  it('initialize uses base theme under html.light (not hardcoded dark)', async () => {
    document.documentElement.classList.add('light')
    document.documentElement.style.setProperty('--c-base', '250 250 251')
    document.documentElement.style.setProperty('--c-txt', '24 24 27')
    document.documentElement.style.setProperty('--c-elevated', '244 244 245')
    document.documentElement.style.setProperty('--c-line', '229 229 232')
    document.documentElement.style.setProperty('--c-line-strong', '212 212 216')
    const wrapper = mountDiagram()
    await flushPromises()
    expect(mermaidInitialize).toHaveBeenCalled()
    const cfg = mermaidInitialize.mock.calls.at(-1)?.[0] as {
      theme: string
      themeVariables: Record<string, string>
    }
    expect(cfg.theme).toBe('base')
    expect(cfg.theme).not.toBe('dark')
    expect(cfg.themeVariables.background).toBe('rgb(250, 250, 251)')
    wrapper.unmount()
  })

  it('initialize uses dark theme without html.light', async () => {
    document.documentElement.style.setProperty('--c-base', '10 10 11')
    document.documentElement.style.setProperty('--c-txt', '237 237 240')
    document.documentElement.style.setProperty('--c-elevated', '28 28 33')
    document.documentElement.style.setProperty('--c-line', '38 38 43')
    document.documentElement.style.setProperty('--c-line-strong', '54 54 62')
    const wrapper = mountDiagram()
    await flushPromises()
    const cfg = mermaidInitialize.mock.calls.at(-1)?.[0] as { theme: string }
    expect(cfg.theme).toBe('dark')
    wrapper.unmount()
  })

  it('re-initializes when html.light is toggled', async () => {
    const wrapper = mountDiagram()
    await flushPromises()
    const callsBefore = mermaidInitialize.mock.calls.length
    document.documentElement.classList.add('light')
    await flushPromises()
    // MutationObserver is async; allow a tick
    await new Promise((r) => setTimeout(r, 0))
    await flushPromises()
    expect(mermaidInitialize.mock.calls.length).toBeGreaterThan(callsBefore)
    const cfg = mermaidInitialize.mock.calls.at(-1)?.[0] as { theme: string }
    expect(cfg.theme).toBe('base')
    wrapper.unmount()
  })

  it('falls back to source when render fails', async () => {
    mermaidRender.mockRejectedValueOnce(new Error('boom'))
    const wrapper = mountDiagram('flowchart LR\n  FAIL-->HERE')
    await flushPromises()
    expect(wrapper.find('[data-testid="plan-diagram-fallback"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('FAIL-->HERE')
    wrapper.unmount()
  })
})
