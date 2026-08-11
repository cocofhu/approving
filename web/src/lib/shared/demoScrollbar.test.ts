import { describe, expect, it } from 'vitest'
import {
  DEMO_GHOST_SCROLLBAR_SCRIPT,
  DEMO_GHOST_SCROLLBAR_STYLE,
  injectDemoScrollbarStyles,
} from './demoScrollbar'

const IDLE_COLOR_MARKER = 'scrollbar-color: transparent transparent'
const ACTIVE_COLOR_MARKER = 'scrollbar-color: rgb(38, 38, 43) transparent'
const SCROLLING_MARKER = 'is-scrolling'

function countIdleBlocks(html: string): number {
  return (html.match(/scrollbar-color: transparent transparent/g) ?? []).length
}

describe('injectDemoScrollbarStyles', () => {
  it('returns empty string unchanged', () => {
    expect(injectDemoScrollbarStyles('')).toBe('')
  })

  it('inserts style before </head> and script before </body>', () => {
    const html = '<!doctype html><html><head><title>t</title></head><body></body></html>'
    const result = injectDemoScrollbarStyles(html)
    expect(result).toContain(`${DEMO_GHOST_SCROLLBAR_STYLE}</head>`)
    expect(result).toContain(`${DEMO_GHOST_SCROLLBAR_SCRIPT}</body>`)
    expect(result.indexOf(IDLE_COLOR_MARKER)).toBeLessThan(result.toLowerCase().indexOf('</head>'))
    expect(countIdleBlocks(result)).toBeGreaterThanOrEqual(1)
  })

  it('inserts after <head> when head is open but not closed', () => {
    const html = '<!doctype html><html><head><title>t</title><body></body></html>'
    const result = injectDemoScrollbarStyles(html)
    expect(result).toMatch(/<head[^>]*>[\s\S]*scrollbar-color: transparent transparent/)
    expect(result).toContain(DEMO_GHOST_SCROLLBAR_SCRIPT)
    expect(countIdleBlocks(result)).toBeGreaterThanOrEqual(1)
  })

  it('wraps with head when html exists but head is missing', () => {
    const html = '<!doctype html><html><body>hi</body></html>'
    const result = injectDemoScrollbarStyles(html)
    expect(result).toContain(`<head>${DEMO_GHOST_SCROLLBAR_STYLE}</head>`)
    expect(result).toContain(`${DEMO_GHOST_SCROLLBAR_SCRIPT}</body>`)
    expect(countIdleBlocks(result)).toBeGreaterThanOrEqual(1)
  })

  it('prepends style and appends script when no html/head structure', () => {
    const html = '<div>fragment</div>'
    const result = injectDemoScrollbarStyles(html)
    expect(result.startsWith(DEMO_GHOST_SCROLLBAR_STYLE)).toBe(true)
    expect(result.endsWith(DEMO_GHOST_SCROLLBAR_SCRIPT)).toBe(true)
    expect(result).toContain(html)
    expect(countIdleBlocks(result)).toBeGreaterThanOrEqual(1)
  })

  it('locks idle-invisible + right-angle thin bar contract', () => {
    const result = injectDemoScrollbarStyles('<html><head></head><body></body></html>')
    expect(result).toContain('border-radius: 0')
    expect(result).toContain(IDLE_COLOR_MARKER)
    expect(result).toContain(ACTIVE_COLOR_MARKER)
    expect(result).toContain(SCROLLING_MARKER)
    expect(result).toContain('width: 4px')
    expect(result).toContain('rgb(54, 54, 62)')
    expect(result).toContain('800')
  })
})
