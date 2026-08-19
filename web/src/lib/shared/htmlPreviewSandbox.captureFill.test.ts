// @vitest-environment happy-dom
import { afterEach, describe, expect, it } from 'vitest'
import { injectInlineInspectScript } from './htmlPreviewSandbox'

const TEST_ID = 'capture-fill-test'

type FillHelpers = {
  parseCssColorAlpha: (color: string) => number
  isOpaqueFillColor: (color: string) => boolean
  resolveOpaqueFillColor: (el: Element) => string
}

function loadFillHelpers(): FillHelpers {
  const src = injectInlineInspectScript('<body></body>', TEST_ID)
  const start = src.indexOf('function parseCssColorAlpha')
  const end = src.indexOf('function captureElement')
  expect(start).toBeGreaterThan(-1)
  expect(end).toBeGreaterThan(start)
  const helpers = src.slice(start, end)
  return new Function(`${helpers}; return { parseCssColorAlpha, isOpaqueFillColor, resolveOpaqueFillColor };`)() as FillHelpers
}

describe('inspect capture opaque fill (g2.1/g3.3)', () => {
  afterEach(() => {
    document.body.innerHTML = ''
    document.documentElement.removeAttribute('style')
    document.body.removeAttribute('style')
  })

  it('parses transparent vs opaque CSS colors', () => {
    const { parseCssColorAlpha, isOpaqueFillColor } = loadFillHelpers()
    expect(parseCssColorAlpha('transparent')).toBe(0)
    expect(parseCssColorAlpha('rgba(0, 0, 0, 0)')).toBe(0)
    expect(isOpaqueFillColor('rgb(10, 10, 11)')).toBe(true)
    expect(isOpaqueFillColor('#0A0A0B')).toBe(true)
    expect(isOpaqueFillColor('rgba(237, 237, 240, 0)')).toBe(false)
  })

  it('uses dark ancestor fill for a transparent light h1 (dark page, no sash/siblings)', () => {
    const { resolveOpaqueFillColor } = loadFillHelpers()
    document.documentElement.style.backgroundColor = 'rgb(10, 10, 11)'
    document.body.style.backgroundColor = 'rgb(10, 10, 11)'
    document.body.innerHTML = `
      <div style="background-color: rgb(10, 10, 11);">
        <div>
          <div>
            <h1 id="leaf-h1" style="background-color: transparent; color: rgb(237, 237, 240);">
              Run 详情：Agent交互产物舞台可拖加宽
            </h1>
            <div id="stage-sash" style="background-color: rgb(80, 80, 90);">sash</div>
          </div>
        </div>
      </div>
    `
    const h1 = document.getElementById('leaf-h1')
    expect(h1).toBeTruthy()
    const fill = resolveOpaqueFillColor(h1!)
    expect(fill).toBe('rgb(10, 10, 11)')
    expect(fill).not.toBe('rgb(80, 80, 90)')
  })

  it('uses white page fill for a transparent dark-text leaf (white page, no regression)', () => {
    const { resolveOpaqueFillColor } = loadFillHelpers()
    document.documentElement.style.backgroundColor = 'rgb(255, 255, 255)'
    document.body.style.backgroundColor = 'rgb(255, 255, 255)'
    document.body.innerHTML = `
      <div style="background-color: rgb(255, 255, 255);">
        <p id="leaf-p" style="background-color: rgba(0, 0, 0, 0); color: rgb(24, 24, 27);">深色字</p>
      </div>
    `
    const fill = resolveOpaqueFillColor(document.getElementById('leaf-p')!)
    expect(fill).toBe('rgb(255, 255, 255)')
  })

  it('skips gradient/image layers and keeps walking for a solid ancestor', () => {
    const { resolveOpaqueFillColor } = loadFillHelpers()
    document.documentElement.style.backgroundColor = 'rgb(10, 10, 11)'
    document.body.innerHTML = `
      <div id="solid" style="background-color: rgb(28, 28, 33);">
        <div id="grad" style="background-color: rgb(255, 0, 0); background-image: linear-gradient(red, blue);">
          <span id="leaf" style="background-color: transparent;">hi</span>
        </div>
      </div>
    `
    const fill = resolveOpaqueFillColor(document.getElementById('leaf')!)
    expect(fill).toBe('rgb(28, 28, 33)')
  })
})
