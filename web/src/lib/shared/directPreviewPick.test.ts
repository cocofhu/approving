import { describe, expect, it } from 'vitest'
import {
  DIRECT_PREVIEW_PICKED,
  DIRECT_PREVIEW_READY,
  iframeOrigin,
  isDirectPreviewOrigin,
  parseDirectPreviewMessage,
  resolveDirectPreviewGoto,
} from './directPreviewPick'

describe('directPreviewPick', () => {
  it('iframeOrigin parses http URL', () => {
    expect(iframeOrigin('http://10.0.0.8:18081/app')).toBe('http://10.0.0.8:18081')
    expect(iframeOrigin('not a url')).toBe('')
  })

  it('isDirectPreviewOrigin requires exact origin match', () => {
    const url = 'http://127.0.0.1:18081/'
    expect(isDirectPreviewOrigin(url, 'http://127.0.0.1:18081')).toBe(true)
    expect(isDirectPreviewOrigin(url, 'http://127.0.0.1:18082')).toBe(false)
    expect(isDirectPreviewOrigin(url, 'https://127.0.0.1:18081')).toBe(false)
  })

  it('resolveDirectPreviewGoto keeps same origin', () => {
    const base = 'http://127.0.0.1:18081/'
    expect(resolveDirectPreviewGoto(base, '/dash')).toBe('http://127.0.0.1:18081/dash')
    expect(resolveDirectPreviewGoto(base, 'http://127.0.0.1:18081/x?q=1')).toBe(
      'http://127.0.0.1:18081/x?q=1',
    )
    expect(resolveDirectPreviewGoto(base, 'http://evil.example/')).toBeNull()
    expect(resolveDirectPreviewGoto(base, 'javascript:alert(1)')).toBeNull()
    expect(resolveDirectPreviewGoto(base, '')).toBeNull()
    expect(resolveDirectPreviewGoto('bad', '/x')).toBeNull()
  })

  it('parseDirectPreviewMessage accepts protocol shapes', () => {
    expect(parseDirectPreviewMessage({ type: DIRECT_PREVIEW_READY, url: 'http://x/' })).toEqual({
      type: DIRECT_PREVIEW_READY,
      url: 'http://x/',
    })
    expect(parseDirectPreviewMessage({ type: 'direct-preview-canceled' })).toEqual({
      type: 'direct-preview-canceled',
    })
    expect(
      parseDirectPreviewMessage({
        type: DIRECT_PREVIEW_PICKED,
        selector: 'button',
        tagName: 'button',
        outerHTML: '<button></button>',
        url: 'http://x/a',
      }),
    ).toMatchObject({ selector: 'button', url: 'http://x/a' })
    expect(parseDirectPreviewMessage({ type: DIRECT_PREVIEW_READY })).toBeNull()
    expect(parseDirectPreviewMessage(null)).toBeNull()
    expect(parseDirectPreviewMessage({ type: 'other' })).toBeNull()
  })
})
