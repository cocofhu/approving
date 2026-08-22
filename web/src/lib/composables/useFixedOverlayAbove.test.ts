// @vitest-environment happy-dom
import { afterEach, describe, expect, it, vi } from 'vitest'
import { placeFixedOverlayAbove } from './useFixedOverlayAbove'

describe('placeFixedOverlayAbove', () => {
  afterEach(() => {
    document.body.innerHTML = ''
    vi.restoreAllMocks()
  })

  it('positions overlay above anchor (left align)', async () => {
    const anchor = document.createElement('button')
    anchor.getBoundingClientRect = () =>
      ({
        top: 300,
        left: 40,
        right: 120,
        bottom: 332,
        width: 80,
        height: 32,
      }) as DOMRect
    const overlay = document.createElement('div')
    overlay.getBoundingClientRect = () =>
      ({
        top: 0,
        left: 0,
        right: 160,
        bottom: 78,
        width: 160,
        height: 78,
      }) as DOMRect
    document.body.append(overlay)

    const style = await placeFixedOverlayAbove(anchor, overlay, { align: 'left', gap: 6, width: 160 })
    expect(style).toEqual({
      position: 'fixed',
      top: '216px',
      left: '40px',
      width: '160px',
    })
  })

  it('clamps top to viewport margin without flipping below', async () => {
    const anchor = document.createElement('button')
    anchor.getBoundingClientRect = () =>
      ({
        top: 20,
        left: 40,
        right: 120,
        bottom: 52,
        width: 80,
        height: 32,
      }) as DOMRect
    const overlay = document.createElement('div')
    overlay.getBoundingClientRect = () =>
      ({
        top: 0,
        left: 0,
        right: 160,
        bottom: 100,
        width: 160,
        height: 100,
      }) as DOMRect
    document.body.append(overlay)

    const style = await placeFixedOverlayAbove(anchor, overlay, { align: 'left' })
    expect(style?.top).toBe('8px')
  })
})
