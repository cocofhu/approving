// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import MermaidDiagram from './MermaidDiagram.vue'
import { cssTokenColor, isLightTheme, mermaidThemeName, themeVars } from './mermaidTheme'

const mermaidInitialize = vi.fn()
const mermaidParse = vi.fn()
const mermaidRender = vi.fn()

vi.mock('mermaid', () => ({
  default: {
    initialize: (...args: unknown[]) => mermaidInitialize(...args),
    parse: (...args: unknown[]) => mermaidParse(...args),
    render: (...args: unknown[]) => mermaidRender(...args),
  },
}))

function createI18nPlugin() {
  return createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: {
      'zh-CN': {
        pages: { plan: { diagramFallback: '图渲染失败,显示源码' } },
      },
    },
  })
}

function mountDiagram(source = 'flowchart LR\n  A-->B', extra?: { format?: string }) {
  return mount(MermaidDiagram, {
    props: {
      diagram: { format: extra?.format ?? 'mermaid', source },
      jsonPath: 'architecture.diagram',
    },
    global: { plugins: [createI18nPlugin()] },
  })
}

describe('MermaidDiagram theme helpers (g2.1/g2.3)', () => {
  beforeEach(() => {
    document.documentElement.classList.remove('light')
    document.documentElement.style.cssText = ''
    mermaidInitialize.mockReset()
    mermaidParse.mockReset()
    mermaidRender.mockReset()
    mermaidParse.mockResolvedValue(true)
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
    document.documentElement.style.setProperty('--c-txt2', '82 82 91')
    document.documentElement.style.setProperty('--c-elevated', '244 244 245')
    document.documentElement.style.setProperty('--c-line', '229 229 232')
    document.documentElement.style.setProperty('--c-line-strong', '212 212 216')
    const vars = themeVars()
    expect(vars.background).toBe('rgb(250, 250, 251)')
    expect(vars.primaryTextColor).toBe('rgb(24, 24, 27)')
    expect(vars.secondaryTextColor).toBe('rgb(24, 24, 27)')
    expect(vars.nodeTextColor).toBe('rgb(24, 24, 27)')
    expect(vars.tertiaryTextColor).toBe('rgb(82, 82, 91)')
    expect(vars.textColor).toBe('rgb(24, 24, 27)')
    expect(vars.background).not.toMatch(/10,\s*10,\s*11/)
    expect(mermaidThemeName()).toBe('base')
  })

  it('themeVars under dark keeps dark tokens', () => {
    document.documentElement.style.setProperty('--c-base', '10 10 11')
    document.documentElement.style.setProperty('--c-txt', '237 237 240')
    document.documentElement.style.setProperty('--c-txt2', '161 161 170')
    const vars = themeVars()
    expect(vars.background).toBe('rgb(10, 10, 11)')
    expect(vars.primaryTextColor).toBe('rgb(237, 237, 240)')
    expect(vars.secondaryTextColor).toBe('rgb(237, 237, 240)')
    expect(vars.nodeTextColor).toBe('rgb(237, 237, 240)')
    expect(mermaidThemeName()).toBe('dark')
  })

  it('initialize uses base theme under html.light (not hardcoded dark)', async () => {
    document.documentElement.classList.add('light')
    document.documentElement.style.setProperty('--c-base', '250 250 251')
    document.documentElement.style.setProperty('--c-txt', '24 24 27')
    document.documentElement.style.setProperty('--c-txt2', '82 82 91')
    document.documentElement.style.setProperty('--c-elevated', '244 244 245')
    document.documentElement.style.setProperty('--c-line', '229 229 232')
    document.documentElement.style.setProperty('--c-line-strong', '212 212 216')
    const wrapper = mountDiagram()
    await flushPromises()
    expect(mermaidInitialize).toHaveBeenCalled()
    const lightCalls = mermaidInitialize.mock.calls
    const cfg = lightCalls[lightCalls.length - 1]?.[0] as {
      theme: string
      themeVariables: Record<string, string>
      suppressErrorRendering?: boolean
    }
    expect(cfg.theme).toBe('base')
    expect(cfg.theme).not.toBe('dark')
    expect(cfg.suppressErrorRendering).toBe(true)
    expect(cfg.themeVariables.background).toBe('rgb(250, 250, 251)')
    expect(cfg.themeVariables.primaryTextColor).toBe('rgb(24, 24, 27)')
    expect(cfg.themeVariables.nodeTextColor).toBe('rgb(24, 24, 27)')
    expect(cfg.themeVariables.secondaryTextColor).toBe('rgb(24, 24, 27)')
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
    const darkCalls = mermaidInitialize.mock.calls
    const cfg = darkCalls[darkCalls.length - 1]?.[0] as { theme: string }
    expect(cfg.theme).toBe('dark')
    wrapper.unmount()
  })

  it('re-initializes when html.light is toggled', async () => {
    const wrapper = mountDiagram()
    await flushPromises()
    const callsBefore = mermaidInitialize.mock.calls.length
    document.documentElement.classList.add('light')
    await flushPromises()
    await new Promise((r) => setTimeout(r, 0))
    await flushPromises()
    expect(mermaidInitialize.mock.calls.length).toBeGreaterThan(callsBefore)
    const toggleCalls = mermaidInitialize.mock.calls
    const cfg = toggleCalls[toggleCalls.length - 1]?.[0] as { theme: string }
    expect(cfg.theme).toBe('base')
    wrapper.unmount()
  })

  it('falls back to source when render fails', async () => {
    mermaidRender.mockRejectedValueOnce(new Error('boom'))
    const wrapper = mountDiagram('flowchart LR\n  FAIL-->HERE')
    await flushPromises()
    expect(wrapper.find('[data-testid="plan-diagram-fallback"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('FAIL-->HERE')
    const hint = wrapper.find('[data-testid="plan-diagram-fallback-hint"]')
    const source = wrapper.find('[data-testid="plan-diagram-fallback-source"]')
    expect(hint.classes()).toContain('text-txt2')
    expect(hint.classes()).not.toContain('text-txt3')
    expect(source.classes()).toContain('text-txt2')
    wrapper.unmount()
  })
})

describe('MermaidDiagram parse-first and single fallback (g2.1 / g2.2 / g2.3 / g3.1)', () => {
  beforeEach(() => {
    document.documentElement.classList.remove('light')
    document.body.innerHTML = ''
    mermaidInitialize.mockReset()
    mermaidParse.mockReset()
    mermaidRender.mockReset()
    mermaidParse.mockResolvedValue(true)
    mermaidRender.mockResolvedValue({ svg: '<svg data-ok="1"></svg>' })
  })

  afterEach(() => {
    document.documentElement.classList.remove('light')
    document.body.innerHTML = ''
  })

  it('g2.1: enables suppressErrorRendering and parses before render', async () => {
    const wrapper = mountDiagram()
    await flushPromises()
    expect(mermaidParse).toHaveBeenCalledWith('flowchart LR\n  A-->B')
    expect(mermaidRender).toHaveBeenCalled()
    const g21Calls = mermaidInitialize.mock.calls
    const cfg = g21Calls[g21Calls.length - 1]?.[0] as { suppressErrorRendering?: boolean }
    expect(cfg.suppressErrorRendering).toBe(true)
    expect(wrapper.find('svg[data-ok="1"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('g2.1/g2.2: illegal source falls back once and never calls render', async () => {
    mermaidParse.mockRejectedValue(new Error('Parse error on line 2'))
    const wrapper = mountDiagram('flowchart LR\n  A-->[')
    await flushPromises()
    expect(wrapper.find('[data-testid="plan-diagram-fallback"]').exists()).toBe(true)
    expect(wrapper.findAll('[data-testid="plan-diagram-fallback"]')).toHaveLength(1)
    expect(wrapper.text()).toContain('A-->[')
    expect(mermaidRender).not.toHaveBeenCalled()
    expect(document.body.textContent || '').not.toContain('Syntax error in text')
    // Theme toggle must not re-enter parse/render for the same failed source.
    const parseCalls = mermaidParse.mock.calls.length
    document.documentElement.classList.add('light')
    await flushPromises()
    await new Promise((r) => setTimeout(r, 0))
    await flushPromises()
    expect(mermaidParse.mock.calls.length).toBe(parseCalls)
    expect(mermaidRender).not.toHaveBeenCalled()
    expect(wrapper.findAll('[data-testid="plan-diagram-fallback"]')).toHaveLength(1)
    wrapper.unmount()
  })

  it('g2.2: cleans temporary #d{id} nodes after failure', async () => {
    mermaidParse.mockResolvedValue(true)
    mermaidRender.mockImplementation(async (id: string) => {
      const tmp = document.createElement('div')
      tmp.id = `d${id}`
      tmp.textContent = 'Syntax error in text'
      document.body.appendChild(tmp)
      throw new Error('render failed')
    })
    const wrapper = mountDiagram('flowchart LR\n  BAD-->X')
    await flushPromises()
    expect(wrapper.find('[data-testid="plan-diagram-fallback"]').exists()).toBe(true)
    expect(document.querySelectorAll('[id^="dplan-mmd-"]').length).toBe(0)
    expect(document.body.textContent || '').not.toMatch(/Syntax error in text/)
    wrapper.unmount()
  })

  it('g2.3: legal diagram still renders SVG; theme toggle re-draws', async () => {
    const wrapper = mountDiagram('flowchart LR\n  OK-->YES')
    await flushPromises()
    expect(wrapper.find('svg[data-ok="1"]').exists()).toBe(true)
    const rendersBefore = mermaidRender.mock.calls.length
    document.documentElement.classList.add('light')
    await flushPromises()
    await new Promise((r) => setTimeout(r, 0))
    await flushPromises()
    expect(mermaidRender.mock.calls.length).toBeGreaterThan(rendersBefore)
    wrapper.unmount()
  })

  it('g3.1: illegal diagram falls back while a subsequent legal diagram still renders', async () => {
    mermaidParse.mockImplementation(async (src: string) => {
      if (String(src).includes('BAD')) throw new Error('bad')
      return true
    })
    mermaidRender.mockImplementation(async (_id: string, src: string) => ({
      svg: `<svg data-src="${String(src).includes('GOOD') ? 'good' : 'other'}"></svg>`,
    }))
    const bad = mountDiagram('flowchart LR\n  BAD-->X')
    await flushPromises()
    expect(bad.find('[data-testid="plan-diagram-fallback"]').exists()).toBe(true)
    bad.unmount()

    const good = mountDiagram('flowchart LR\n  GOOD-->Y')
    await flushPromises()
    expect(good.find('[data-testid="plan-diagram-fallback"]').exists()).toBe(false)
    expect(good.html()).toContain('data-src="good"')
    good.unmount()
  })
})
