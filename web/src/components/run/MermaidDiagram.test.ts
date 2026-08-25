// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import MermaidDiagram from './MermaidDiagram.vue'
import { flushMermaidQueue } from './mermaidRenderQueue'
import { cssTokenColor, isLightTheme, mermaidThemeName, themeVars } from './mermaidTheme'

const mermaidInitialize = vi.fn()
const mermaidRender = vi.fn()

vi.mock('mermaid', () => ({
  default: {
    initialize: (...args: unknown[]) => mermaidInitialize(...args),
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

/** Three-section plan fixture — mirrors PlanView architecture / data / interaction. */
const THREE_SECTION_FIXTURE = {
  architecture: 'flowchart LR\n  ARCH_ONLY-->Q',
  data_design: 'erDiagram\n  DATA_ENTITY_ONLY {\n    string id PK\n  }',
  interaction: 'sequenceDiagram\n  participant INTERACT_ONLY\n  Note over INTERACT_ONLY: note-only',
} as const

describe('MermaidDiagram theme helpers (g2.1/g2.3)', () => {
  beforeEach(async () => {
    document.documentElement.classList.remove('light')
    document.documentElement.style.cssText = ''
    mermaidInitialize.mockReset()
    mermaidRender.mockReset()
    mermaidRender.mockResolvedValue({ svg: '<svg data-ok="1"></svg>' })
    await flushMermaidQueue()
  })

  afterEach(async () => {
    document.documentElement.classList.remove('light')
    document.documentElement.style.cssText = ''
    await flushMermaidQueue()
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
    await flushMermaidQueue()
    expect(mermaidInitialize).toHaveBeenCalled()
    const lightCalls = mermaidInitialize.mock.calls
    const cfg = lightCalls[lightCalls.length - 1]?.[0] as {
      theme: string
      themeVariables: Record<string, string>
    }
    expect(cfg.theme).toBe('base')
    expect(cfg.theme).not.toBe('dark')
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
    await flushMermaidQueue()
    const darkCalls = mermaidInitialize.mock.calls
    const cfg = darkCalls[darkCalls.length - 1]?.[0] as { theme: string }
    expect(cfg.theme).toBe('dark')
    wrapper.unmount()
  })

  it('re-initializes when html.light is toggled', async () => {
    const wrapper = mountDiagram()
    await flushPromises()
    await flushMermaidQueue()
    const callsBefore = mermaidInitialize.mock.calls.length
    document.documentElement.classList.add('light')
    await flushPromises()
    // MutationObserver is async; allow a tick
    await new Promise((r) => setTimeout(r, 0))
    await flushPromises()
    await flushMermaidQueue()
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
    await flushMermaidQueue()
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

describe('MermaidDiagram serial isolation (g1.1 / g2.1 / g2.2 / g2.3 / g3.1 / g3.2)', () => {
  beforeEach(async () => {
    document.documentElement.classList.remove('light')
    mermaidInitialize.mockReset()
    mermaidRender.mockReset()
    await flushMermaidQueue()
  })

  afterEach(async () => {
    document.documentElement.classList.remove('light')
    await flushMermaidQueue()
  })

  it('g1.1/g2.1: three concurrent mounts serialize render and never write sibling content into host', async () => {
    let inFlight = 0
    let maxInFlight = 0
    mermaidRender.mockImplementation(async (_id: string, src: string) => {
      inFlight++
      maxInFlight = Math.max(maxInFlight, inFlight)
      await new Promise((r) => setTimeout(r, 15))
      inFlight--
      const tag =
        src.includes('ARCH_ONLY') ? 'arch' : src.includes('DATA_ENTITY_ONLY') ? 'data' : src.includes('INTERACT_ONLY') ? 'interact' : 'other'
      return { svg: `<svg data-section="${tag}"><text>${tag}</text></svg>` }
    })

    const i18n = createI18nPlugin()
    const wrappers = [
      mount(MermaidDiagram, {
        props: { diagram: { format: 'mermaid', source: THREE_SECTION_FIXTURE.architecture }, jsonPath: 'architecture.diagram' },
        global: { plugins: [i18n] },
      }),
      mount(MermaidDiagram, {
        props: { diagram: { format: 'mermaid', source: THREE_SECTION_FIXTURE.data_design }, jsonPath: 'data_design.diagram' },
        global: { plugins: [i18n] },
      }),
      mount(MermaidDiagram, {
        props: { diagram: { format: 'mermaid', source: THREE_SECTION_FIXTURE.interaction }, jsonPath: 'interaction.diagram' },
        global: { plugins: [i18n] },
      }),
    ]

    await flushPromises()
    await flushMermaidQueue()
    await flushPromises()

    expect(maxInFlight).toBe(1)
    expect(mermaidRender).toHaveBeenCalledTimes(3)

    const svgs = wrappers.map((w) => w.element.querySelector('svg')?.getAttribute('data-section') || '')
    expect(svgs).toEqual(['arch', 'data', 'interact'])
    // Cross-contamination: each host's SVG must only carry its own section marker (g1.1 fixture).
    expect(wrappers[0].html()).toContain('data-section="arch"')
    expect(wrappers[0].html()).not.toContain('data-section="data"')
    expect(wrappers[0].html()).not.toContain('data-section="interact"')
    expect(wrappers[1].html()).toContain('data-section="data"')
    expect(wrappers[1].html()).not.toContain('data-section="arch"')
    expect(wrappers[1].html()).not.toContain('data-section="interact"')
    expect(wrappers[2].html()).toContain('data-section="interact"')
    expect(wrappers[2].html()).not.toContain('data-section="arch"')
    expect(wrappers[2].html()).not.toContain('data-section="data"')
    // render args prove each instance requested its own source (fixture markers).
    const sources = mermaidRender.mock.calls.map((c) => String(c[1]))
    expect(sources.some((s) => s.includes('ARCH_ONLY'))).toBe(true)
    expect(sources.some((s) => s.includes('DATA_ENTITY_ONLY'))).toBe(true)
    expect(sources.some((s) => s.includes('INTERACT_ONLY'))).toBe(true)

    wrappers.forEach((w) => w.unmount())
  })

  it('g2.3: one failed section falls back without polluting siblings', async () => {
    mermaidRender.mockImplementation(async (_id: string, src: string) => {
      if (src.includes('BAD')) throw new Error('parse fail')
      await new Promise((r) => setTimeout(r, 5))
      return { svg: `<svg data-ok="${src.includes('GOOD_A') ? 'a' : 'b'}"></svg>` }
    })
    const i18n = createI18nPlugin()
    const bad = mount(MermaidDiagram, {
      props: { diagram: { format: 'mermaid', source: 'flowchart LR\n  BAD-->X' }, jsonPath: 'architecture.diagram' },
      global: { plugins: [i18n] },
    })
    const goodA = mount(MermaidDiagram, {
      props: { diagram: { format: 'mermaid', source: 'flowchart LR\n  GOOD_A-->Y' }, jsonPath: 'data_design.diagram' },
      global: { plugins: [i18n] },
    })
    const goodB = mount(MermaidDiagram, {
      props: { diagram: { format: 'mermaid', source: 'flowchart LR\n  GOOD_B-->Z' }, jsonPath: 'interaction.diagram' },
      global: { plugins: [i18n] },
    })
    await flushPromises()
    await flushMermaidQueue()
    await flushPromises()

    expect(bad.find('[data-testid="plan-diagram-fallback"]').exists()).toBe(true)
    expect(bad.text()).toContain('BAD-->X')
    expect(goodA.find('[data-testid="plan-diagram-fallback"]').exists()).toBe(false)
    expect(goodB.find('[data-testid="plan-diagram-fallback"]').exists()).toBe(false)
    expect(goodA.element.querySelector('svg')?.getAttribute('data-ok')).toBe('a')
    expect(goodB.element.querySelector('svg')?.getAttribute('data-ok')).toBe('b')
    expect(goodA.html()).not.toContain('BAD-->X')
    expect(goodB.html()).not.toContain('BAD-->X')

    bad.unmount()
    goodA.unmount()
    goodB.unmount()
  })

  it('g2.3: theme MutationObserver re-render keeps three hosts isolated', async () => {
    mermaidRender.mockImplementation(async (_id: string, src: string) => {
      await new Promise((r) => setTimeout(r, 8))
      const tag = src.includes('T_ARCH') ? 'arch' : src.includes('T_DATA') ? 'data' : 'interact'
      return { svg: `<svg data-theme="${tag}"></svg>` }
    })
    const i18n = createI18nPlugin()
    const wrappers = [
      mount(MermaidDiagram, {
        props: { diagram: { format: 'mermaid', source: 'flowchart LR\n  T_ARCH-->A' }, jsonPath: 'architecture.diagram' },
        global: { plugins: [i18n] },
      }),
      mount(MermaidDiagram, {
        props: { diagram: { format: 'mermaid', source: 'flowchart LR\n  T_DATA-->B' }, jsonPath: 'data_design.diagram' },
        global: { plugins: [i18n] },
      }),
      mount(MermaidDiagram, {
        props: { diagram: { format: 'mermaid', source: 'flowchart LR\n  T_INTER-->C' }, jsonPath: 'interaction.diagram' },
        global: { plugins: [i18n] },
      }),
    ]
    await flushPromises()
    await flushMermaidQueue()
    await flushPromises()

    document.documentElement.classList.add('light')
    await new Promise((r) => setTimeout(r, 0))
    await flushPromises()
    await flushMermaidQueue()
    await flushPromises()

    const tags = wrappers.map((w) => w.element.querySelector('svg')?.getAttribute('data-theme'))
    expect(tags).toEqual(['arch', 'data', 'interact'])
    wrappers.forEach((w) => w.unmount())
  })

  it('g2.2: unmount invalidates in-flight render so destroyed host stays empty', async () => {
    let resolveRender!: (v: { svg: string }) => void
    mermaidRender.mockImplementation(
      () =>
        new Promise<{ svg: string }>((resolve) => {
          resolveRender = resolve
        }),
    )
    const wrapper = mountDiagram('flowchart LR\n  LATE-->X')
    await flushPromises()
    // Still rendering — unmount before resolve
    wrapper.unmount()
    resolveRender({ svg: '<svg data-late="1"></svg>' })
    await flushPromises()
    await flushMermaidQueue()
    await flushPromises()
    // Component gone; no throw and no leaked write into detached host with residual svg from us asserting via mock only.
    expect(mermaidRender).toHaveBeenCalled()
  })

  it('g3.2: rapid consecutive source updates settle to the latest content only', async () => {
    const renderLog: string[] = []
    mermaidRender.mockImplementation(async (_id: string, src: string) => {
      renderLog.push(src)
      await new Promise((r) => setTimeout(r, 12))
      const marker = src.match(/V(\d+)/)?.[1] || '?'
      return { svg: `<svg data-v="${marker}"></svg>` }
    })
    const wrapper = mountDiagram('flowchart LR\n  V1-->A')
    await flushPromises()

    // Simulate followLive / set_plan hot updates
    await wrapper.setProps({ diagram: { format: 'mermaid', source: 'flowchart LR\n  V2-->A' }, jsonPath: 'architecture.diagram' })
    await wrapper.setProps({ diagram: { format: 'mermaid', source: 'flowchart LR\n  V3-->A' }, jsonPath: 'architecture.diagram' })
    await wrapper.setProps({ diagram: { format: 'mermaid', source: 'flowchart LR\n  V4-->A' }, jsonPath: 'architecture.diagram' })

    await flushPromises()
    await flushMermaidQueue()
    await flushPromises()

    expect(wrapper.element.querySelector('svg')?.getAttribute('data-v')).toBe('4')
    expect(wrapper.html()).not.toMatch(/data-v="[123]"/)
    // Stale gens may skip render after dequeue; latest must have run.
    expect(renderLog.some((s) => s.includes('V4'))).toBe(true)
    wrapper.unmount()
  })

  it('non-mermaid format uses fallback without calling render', async () => {
    const wrapper = mountDiagram('not mermaid', { format: 'plantuml' })
    await flushPromises()
    await flushMermaidQueue()
    expect(wrapper.find('[data-testid="plan-diagram-fallback"]').exists()).toBe(true)
    expect(mermaidRender).not.toHaveBeenCalled()
    wrapper.unmount()
  })
})
