// @vitest-environment happy-dom
import { describe, expect, it } from 'vitest'
import {
  applyComposerAutoGrow,
  CLARIFY_AUTO_GROW_MAX,
  CLARIFY_AUTO_GROW_MIN,
  measureWrappedTextHeight,
} from './composerAutoGrow'

function makeTextarea(opts: { value?: string; clientWidth: number; scrollHeight: number }) {
  const el = document.createElement('textarea')
  el.value = opts.value ?? ''
  el.placeholder = '请先描述目标…'
  Object.defineProperty(el, 'clientWidth', { configurable: true, get: () => opts.clientWidth })
  Object.defineProperty(el, 'scrollHeight', { configurable: true, get: () => opts.scrollHeight })
  document.body.appendChild(el)
  return el
}

function withSizerOffsetHeight(height: number, run: () => void) {
  const proto = Object.getOwnPropertyDescriptor(HTMLElement.prototype, 'offsetHeight')
  Object.defineProperty(HTMLElement.prototype, 'offsetHeight', {
    configurable: true,
    get() {
      if ((this as HTMLElement).getAttribute?.('data-composer-sizer') != null) return height
      return proto?.get ? proto.get.call(this) : 0
    },
  })
  try {
    run()
  } finally {
    if (proto) Object.defineProperty(HTMLElement.prototype, 'offsetHeight', proto)
  }
}

describe('composerAutoGrow', () => {
  it('unlaid-out empty (clientWidth 0) uses scrollHeight and stays at min, not max', () => {
    const el = makeTextarea({ clientWidth: 0, scrollHeight: 40 })
    const overflow = applyComposerAutoGrow(el, {
      min: CLARIFY_AUTO_GROW_MIN,
      max: CLARIFY_AUTO_GROW_MAX,
      emptyHint: '请先描述目标…',
    })
    expect(el.style.height).toBe('40px')
    expect(overflow).toBe(false)
    el.remove()
  })

  it('wide empty hint wrap (~one line) stays at min, not 128px cap', () => {
    const el = makeTextarea({ clientWidth: 420, scrollHeight: 128 })
    withSizerOffsetHeight(40, () => {
      applyComposerAutoGrow(el, {
        min: CLARIFY_AUTO_GROW_MIN,
        max: CLARIFY_AUTO_GROW_MAX,
        emptyHint: '请先描述目标…',
      })
      expect(el.style.height).toBe('40px')
    })
    el.remove()
  })

  it('narrow empty hint wrap grows to N lines below cap', () => {
    const el = makeTextarea({ clientWidth: 120, scrollHeight: 40 })
    withSizerOffsetHeight(88, () => {
      const overflow = applyComposerAutoGrow(el, {
        min: CLARIFY_AUTO_GROW_MIN,
        max: CLARIFY_AUTO_GROW_MAX,
        emptyHint: '请先描述目标…',
      })
      expect(el.style.height).toBe('88px')
      expect(overflow).toBe(false)
    })
    el.remove()
  })

  it('widening from wrapped hint retracts height', () => {
    const el = makeTextarea({ clientWidth: 80, scrollHeight: 40 })
    withSizerOffsetHeight(96, () => {
      applyComposerAutoGrow(el, {
        min: CLARIFY_AUTO_GROW_MIN,
        max: CLARIFY_AUTO_GROW_MAX,
        emptyHint: '请先描述目标…',
      })
      expect(el.style.height).toBe('96px')
    })
    Object.defineProperty(el, 'clientWidth', { configurable: true, get: () => 480 })
    withSizerOffsetHeight(40, () => {
      applyComposerAutoGrow(el, {
        min: CLARIFY_AUTO_GROW_MIN,
        max: CLARIFY_AUTO_GROW_MAX,
        emptyHint: '请先描述目标…',
      })
      expect(el.style.height).toBe('40px')
    })
    el.remove()
  })

  it('draft taller than max enables overflow and caps height', () => {
    const el = makeTextarea({ value: 'a\n'.repeat(20), clientWidth: 0, scrollHeight: 200 })
    const overflow = applyComposerAutoGrow(el, {
      min: CLARIFY_AUTO_GROW_MIN,
      max: CLARIFY_AUTO_GROW_MAX,
    })
    expect(el.style.height).toBe('128px')
    expect(overflow).toBe(true)
    el.remove()
  })

  it('measureWrappedTextHeight returns 0 when width is 0', () => {
    const el = makeTextarea({ clientWidth: 0, scrollHeight: 40 })
    expect(measureWrappedTextHeight(el, '请先描述目标…')).toBe(0)
    el.remove()
  })
})
