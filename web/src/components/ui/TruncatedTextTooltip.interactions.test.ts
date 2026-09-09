// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import TruncatedTextTooltip from './TruncatedTextTooltip.vue'

let resizeCallback: (() => void) | undefined
const disconnect = vi.fn()

class ResizeObserverStub {
  constructor(cb: () => void) {
    resizeCallback = cb
  }
  observe = vi.fn()
  disconnect = disconnect
  unobserve = vi.fn()
}

function mountTooltip(props: Record<string, unknown> = {}) {
  const i18n = createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...common } },
  })
  return mount(TruncatedTextTooltip, {
    props: { text: 'A very long tooltip value', ...props },
    global: { plugins: [i18n] },
    attachTo: document.body,
  })
}

describe('TruncatedTextTooltip interactions', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.stubGlobal('ResizeObserver', ResizeObserverStub)
    vi.spyOn(HTMLElement.prototype, 'scrollWidth', 'get').mockReturnValue(200)
    vi.spyOn(HTMLElement.prototype, 'clientWidth', 'get').mockReturnValue(80)
    Object.defineProperty(window, 'innerWidth', { configurable: true, value: 300 })
    Object.defineProperty(window, 'innerHeight', { configurable: true, value: 200 })
    disconnect.mockClear()
  })

  afterEach(() => {
    document.body.innerHTML = ''
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
    vi.useRealTimers()
  })

  it('opens on hover, places above near the viewport edge, and delays hide', async () => {
    const w = mountTooltip()
    const trigger = w.get('span')
    vi.spyOn(trigger.element, 'getBoundingClientRect').mockReturnValue({
      left: 260, right: 290, top: 160, bottom: 180, width: 30, height: 20, x: 260, y: 160, toJSON: () => {},
    })
    await trigger.trigger('mouseenter')
    await flushPromises()
    const tip = document.querySelector('[data-testid="truncated-text-tooltip"]') as HTMLElement
    expect(tip).toBeTruthy()
    vi.spyOn(tip, 'getBoundingClientRect').mockReturnValue({
      left: 0, right: 180, top: 0, bottom: 50, width: 180, height: 50, x: 0, y: 0, toJSON: () => {},
    })
    await (w.vm as any).placeTooltip()
    expect(tip.style.left).toBe('112px')
    expect(tip.style.top).toBe('104px')
    expect(trigger.attributes('aria-describedby')).toContain('truncated-tooltip')

    await trigger.trigger('mouseleave')
    vi.advanceTimersByTime(39)
    expect(document.querySelector('[role="tooltip"]')).toBeTruthy()
    vi.advanceTimersByTime(1)
    await flushPromises()
    expect(document.querySelector('[role="tooltip"]')).toBeNull()
    w.unmount()
    expect(disconnect).toHaveBeenCalled()
  })

  it('supports keyboard toggling, escape, focus, and outside pointer dismissal', async () => {
    const w = mountTooltip()
    const trigger = w.get('span')
    Object.defineProperty(trigger.element, 'scrollWidth', { configurable: true, value: 200 })
    Object.defineProperty(trigger.element, 'clientWidth', { configurable: true, value: 80 })
    ;(w.vm as any).measureOverflow()
    await flushPromises()
    expect(trigger.attributes('tabindex')).toBe('0')
    expect(trigger.attributes('role')).toBe('button')

    await trigger.trigger('keydown', { key: 'Enter' })
    await flushPromises()
    expect(document.querySelector('[role="tooltip"]')).toBeTruthy()
    await trigger.trigger('keydown', { key: 'Escape' })
    await flushPromises()
    expect(document.querySelector('[role="tooltip"]')).toBeNull()

    await trigger.trigger('keydown', { key: ' ' })
    await flushPromises()
    expect(document.querySelector('[role="tooltip"]')).toBeTruthy()
    document.body.dispatchEvent(new Event('pointerdown', { bubbles: true }))
    await flushPromises()
    expect(document.querySelector('[role="tooltip"]')).toBeNull()

    await trigger.trigger('focus')
    await flushPromises()
    expect(document.querySelector('[role="tooltip"]')).toBeTruthy()
    await trigger.trigger('blur')
    vi.advanceTimersByTime(40)
    await flushPromises()
    expect(document.querySelector('[role="tooltip"]')).toBeNull()
    w.unmount()
  })

  it('toggles on touch click while ordinary clicks are ignored', async () => {
    const w = mountTooltip()
    const trigger = w.get('span')
    await trigger.trigger('click')
    expect(document.querySelector('[role="tooltip"]')).toBeNull()

    const down = new Event('pointerdown', { bubbles: true }) as PointerEvent
    Object.defineProperty(down, 'pointerType', { value: 'touch' })
    trigger.element.dispatchEvent(down)
    await trigger.trigger('click')
    await flushPromises()
    expect(document.querySelector('[role="tooltip"]')).toBeTruthy()
    await trigger.trigger('click')
    await flushPromises()
    expect(document.querySelector('[role="tooltip"]')).toBeNull()
    w.unmount()
  })

  it('measures children, reacts to observer/viewport changes, and closes when overflow ends', async () => {
    const w = mount(TruncatedTextTooltip, {
      props: { text: 'nested', measureChild: true },
      slots: { default: '<span class="small">one</span><span class="large">two</span>' },
      attachTo: document.body,
    })
    const children = w.findAll('span span')
    Object.defineProperty(children[0].element, 'scrollWidth', { configurable: true, value: 50 })
    Object.defineProperty(children[0].element, 'clientWidth', { configurable: true, value: 50 })
    Object.defineProperty(children[1].element, 'scrollWidth', { configurable: true, value: 100 })
    Object.defineProperty(children[1].element, 'clientWidth', { configurable: true, value: 20 })
    ;(w.vm as any).measureOverflow()
    await w.get('span').trigger('mouseenter')
    await flushPromises()
    expect(document.querySelector('[role="tooltip"]')).toBeTruthy()

    Object.defineProperty(children[1].element, 'scrollWidth', { configurable: true, value: 20 })
    resizeCallback?.()
    window.dispatchEvent(new Event('resize'))
    window.dispatchEvent(new Event('scroll'))
    await flushPromises()
    expect(document.querySelector('[role="tooltip"]')).toBeNull()
    w.unmount()
  })

  it('mirrors aria-describedby to a focus parent and cleans it up', async () => {
    const w = mountTooltip({ focusParent: true, focusable: false })
    const parent = w.get('span').element.parentElement!
    parent.dispatchEvent(new FocusEvent('focus'))
    await flushPromises()
    expect(parent.getAttribute('aria-describedby')).toContain('truncated-tooltip')
    parent.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await flushPromises()
    expect(parent.hasAttribute('aria-describedby')).toBe(false)
    parent.dispatchEvent(new FocusEvent('blur'))
    vi.advanceTimersByTime(40)
    w.unmount()
  })

  it('stays non-interactive when content is not truncated', async () => {
    vi.mocked(HTMLElement.prototype.scrollWidth, true)
    vi.restoreAllMocks()
    vi.spyOn(HTMLElement.prototype, 'scrollWidth', 'get').mockReturnValue(80)
    vi.spyOn(HTMLElement.prototype, 'clientWidth', 'get').mockReturnValue(80)
    const w = mountTooltip()
    const trigger = w.get('span')
    expect(trigger.attributes('tabindex')).toBeUndefined()
    await trigger.trigger('mouseenter')
    await trigger.trigger('keydown', { key: 'Enter' })
    await flushPromises()
    expect(document.querySelector('[role="tooltip"]')).toBeNull()
    w.unmount()
  })
})
