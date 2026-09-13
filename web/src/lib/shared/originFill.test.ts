// @vitest-environment happy-dom
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import {
  coverDiameter,
  installOriginFill,
  isOriginFillInstalled,
  shouldSkipOriginFill,
  uninstallOriginFill,
} from './originFill'

/** Stacking contract (review v1/v3): ::before under all content including bare text nodes. */
function originFillCssBlock(): string {
  const css = readFileSync(resolve(__dirname, '../../styles/global.css'), 'utf8')
  const start = css.indexOf('/* ---------- Button origin-fill')
  expect(start).toBeGreaterThanOrEqual(0)
  return css.slice(start)
}

function dispatchPointer(type: 'pointerenter' | 'pointerleave', target: EventTarget, init: PointerEventInit = {}) {
  const event = new PointerEvent(type, {
    bubbles: true,
    clientX: init.clientX ?? 10,
    clientY: init.clientY ?? 10,
    relatedTarget: init.relatedTarget ?? null,
    ...init,
  })
  // Capture listeners on document see the event when dispatched on the target
  // only if it goes through the path — dispatch on target with capture on document
  // requires the event to be listened in capture on the way down. Use document
  // capture by dispatching via target so capture phase from document runs.
  target.dispatchEvent(event)
}

describe('originFill', () => {
  beforeEach(() => {
    uninstallOriginFill()
    document.body.innerHTML = ''
    document.documentElement.className = ''
  })

  afterEach(() => {
    uninstallOriginFill()
    document.body.innerHTML = ''
  })

  it('installs once and can uninstall', () => {
    installOriginFill()
    expect(isOriginFillInstalled()).toBe(true)
    installOriginFill()
    expect(isOriginFillInstalled()).toBe(true)
    uninstallOriginFill()
    expect(isOriginFillInstalled()).toBe(false)
  })

  it('coverDiameter reaches farthest corner from pointer', () => {
    // pointer at top-left (0,0) of 100x40 → farthest is bottom-right
    expect(coverDiameter(100, 40, 0, 0)).toBeCloseTo(Math.hypot(100, 40) * 2 + 2)
    // center needs enough to cover corners
    const mid = coverDiameter(100, 40, 50, 20)
    expect(mid).toBeGreaterThanOrEqual(Math.hypot(50, 20) * 2)
  })

  it('pointerenter writes --ox/--oy/--or and adds is-filled (g1.2 / g2.3)', () => {
    installOriginFill()
    const btn = document.createElement('button')
    btn.type = 'button'
    btn.textContent = '保存'
    Object.defineProperty(btn, 'clientWidth', { value: 100 })
    Object.defineProperty(btn, 'clientHeight', { value: 36 })
    btn.getBoundingClientRect = () =>
      ({
        left: 0,
        top: 0,
        width: 100,
        height: 36,
        right: 100,
        bottom: 36,
        x: 0,
        y: 0,
        toJSON() {
          return {}
        },
      }) as DOMRect
    document.body.appendChild(btn)

    dispatchPointer('pointerenter', btn, { clientX: 12, clientY: 8 })

    expect(btn.classList.contains('is-filled')).toBe(true)
    expect(btn.style.getPropertyValue('--ox')).toBe('12px')
    expect(btn.style.getPropertyValue('--oy')).toBe('8px')
    const or = parseFloat(btn.style.getPropertyValue('--or'))
    expect(or).toBeGreaterThan(0)
    expect(or).toBe(coverDiameter(100, 36, 12, 8))
  })

  it('pointerleave removes is-filled so rapid pass leaves no residue (g2.3)', () => {
    installOriginFill()
    const btn = document.createElement('button')
    btn.type = 'button'
    Object.defineProperty(btn, 'clientWidth', { value: 80 })
    Object.defineProperty(btn, 'clientHeight', { value: 32 })
    btn.getBoundingClientRect = () =>
      ({
        left: 0,
        top: 0,
        width: 80,
        height: 32,
        right: 80,
        bottom: 32,
        x: 0,
        y: 0,
        toJSON() {
          return {}
        },
      }) as DOMRect
    document.body.appendChild(btn)

    dispatchPointer('pointerenter', btn, { clientX: 5, clientY: 5 })
    expect(btn.classList.contains('is-filled')).toBe(true)

    dispatchPointer('pointerleave', btn, { clientX: 5, clientY: 5, relatedTarget: document.body })
    expect(btn.classList.contains('is-filled')).toBe(false)
  })

  it('skips disabled and aria-busy loading buttons (g1.2 / g2.3)', () => {
    installOriginFill()
    const disabled = document.createElement('button')
    disabled.disabled = true
    disabled.textContent = '禁用'
    Object.defineProperty(disabled, 'clientWidth', { value: 60 })
    Object.defineProperty(disabled, 'clientHeight', { value: 28 })
    disabled.getBoundingClientRect = () =>
      ({ left: 0, top: 0, width: 60, height: 28, right: 60, bottom: 28, x: 0, y: 0, toJSON() { return {} } }) as DOMRect
    document.body.appendChild(disabled)

    const loading = document.createElement('button')
    loading.setAttribute('aria-busy', 'true')
    loading.textContent = '加载'
    Object.defineProperty(loading, 'clientWidth', { value: 60 })
    Object.defineProperty(loading, 'clientHeight', { value: 28 })
    loading.getBoundingClientRect = () =>
      ({ left: 0, top: 0, width: 60, height: 28, right: 60, bottom: 28, x: 0, y: 0, toJSON() { return {} } }) as DOMRect
    document.body.appendChild(loading)

    expect(shouldSkipOriginFill(disabled)).toBe(true)
    expect(shouldSkipOriginFill(loading)).toBe(true)

    dispatchPointer('pointerenter', disabled, { clientX: 4, clientY: 4 })
    dispatchPointer('pointerenter', loading, { clientX: 4, clientY: 4 })
    expect(disabled.classList.contains('is-filled')).toBe(false)
    expect(loading.classList.contains('is-filled')).toBe(false)
  })

  it('delegates via closest(button) for nested label/icon targets (g2.1)', () => {
    installOriginFill()
    const btn = document.createElement('button')
    btn.type = 'button'
    const span = document.createElement('span')
    span.textContent = '筛选'
    btn.appendChild(span)
    Object.defineProperty(btn, 'clientWidth', { value: 72 })
    Object.defineProperty(btn, 'clientHeight', { value: 24 })
    btn.getBoundingClientRect = () =>
      ({ left: 10, top: 10, width: 72, height: 24, right: 82, bottom: 34, x: 10, y: 10, toJSON() { return {} } }) as DOMRect
    document.body.appendChild(btn)

    dispatchPointer('pointerenter', span, { clientX: 20, clientY: 18 })
    expect(btn.classList.contains('is-filled')).toBe(true)
    expect(btn.style.getPropertyValue('--ox')).toBe('10px')
    expect(btn.style.getPropertyValue('--oy')).toBe('8px')
  })

  it('does not clear is-filled when leaving to a child inside the button', () => {
    installOriginFill()
    const btn = document.createElement('button')
    const inner = document.createElement('span')
    btn.appendChild(inner)
    Object.defineProperty(btn, 'clientWidth', { value: 50 })
    Object.defineProperty(btn, 'clientHeight', { value: 20 })
    btn.getBoundingClientRect = () =>
      ({ left: 0, top: 0, width: 50, height: 20, right: 50, bottom: 20, x: 0, y: 0, toJSON() { return {} } }) as DOMRect
    document.body.appendChild(btn)

    dispatchPointer('pointerenter', btn, { clientX: 2, clientY: 2 })
    dispatchPointer('pointerleave', inner, { clientX: 3, clientY: 3, relatedTarget: btn })
    expect(btn.classList.contains('is-filled')).toBe(true)
  })

  it('uninstall clears stuck is-filled classes', () => {
    installOriginFill()
    const btn = document.createElement('button')
    btn.classList.add('is-filled')
    document.body.appendChild(btn)
    uninstallOriginFill()
    expect(btn.classList.contains('is-filled')).toBe(false)
  })

  it('CSS stacks ::before under bare text (z-index:-1, no button>* lift) (g1.1 / FR-f1)', () => {
    const block = originFillCssBlock()
    expect(block).toMatch(/button::before\s*\{[^}]*z-index:\s*-1/s)
    expect(block).not.toMatch(/button\s*>\s*\*\s*\{/)
    expect(block).not.toMatch(/z-index:\s*0/)
  })

  it('is-filled keeps bare text node label in the accessibility name (g2.3 / review v1)', () => {
    installOriginFill()
    const btn = document.createElement('button')
    btn.type = 'button'
    btn.appendChild(document.createTextNode('Bare text'))
    Object.defineProperty(btn, 'clientWidth', { value: 96 })
    Object.defineProperty(btn, 'clientHeight', { value: 32 })
    btn.getBoundingClientRect = () =>
      ({ left: 0, top: 0, width: 96, height: 32, right: 96, bottom: 32, x: 0, y: 0, toJSON() { return {} } }) as DOMRect
    document.body.appendChild(btn)

    dispatchPointer('pointerenter', btn, { clientX: 8, clientY: 8 })
    expect(btn.classList.contains('is-filled')).toBe(true)
    // Label must remain a direct text node (not only element children) and stay exposed.
    expect([...btn.childNodes].some((n) => n.nodeType === Node.TEXT_NODE && n.textContent?.includes('Bare text'))).toBe(
      true,
    )
    expect(btn.textContent).toContain('Bare text')
    expect(btn.getAttribute('aria-hidden')).toBeNull()
  })

  it('is-filled keeps icon element + sibling text label readable (g2.3 / review v1)', () => {
    installOriginFill()
    const btn = document.createElement('button')
    btn.type = 'button'
    const icon = document.createElement('span')
    icon.setAttribute('aria-hidden', 'true')
    icon.textContent = '★'
    btn.appendChild(icon)
    btn.appendChild(document.createTextNode('编辑'))
    Object.defineProperty(btn, 'clientWidth', { value: 88 })
    Object.defineProperty(btn, 'clientHeight', { value: 32 })
    btn.getBoundingClientRect = () =>
      ({ left: 0, top: 0, width: 88, height: 32, right: 88, bottom: 32, x: 0, y: 0, toJSON() { return {} } }) as DOMRect
    document.body.appendChild(btn)

    dispatchPointer('pointerenter', btn, { clientX: 6, clientY: 6 })
    expect(btn.classList.contains('is-filled')).toBe(true)
    expect(btn.textContent).toContain('★')
    expect(btn.textContent).toContain('编辑')
    expect([...btn.childNodes].some((n) => n.nodeType === Node.TEXT_NODE && n.textContent?.includes('编辑'))).toBe(true)
  })
})
