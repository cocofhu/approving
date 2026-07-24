// @vitest-environment happy-dom
import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { getEdgeStroke } from './edgeColors'

describe('getEdgeStroke', () => {
  beforeEach(() => {
    document.documentElement.style.setProperty('--c-ok', '52 211 153')
    document.documentElement.style.setProperty('--c-err', '248 113 113')
    document.documentElement.style.setProperty('--c-warn', '251 191 36')
  })

  afterEach(() => {
    document.documentElement.style.removeProperty('--c-ok')
    document.documentElement.style.removeProperty('--c-err')
    document.documentElement.style.removeProperty('--c-warn')
  })

  it('reads semantic stroke colors from CSS variables', () => {
    expect(getEdgeStroke('ok')).toBe('rgb(52 211 153)')
    expect(getEdgeStroke('err')).toBe('rgb(248 113 113)')
    expect(getEdgeStroke('warn')).toBe('rgb(251 191 36)')
  })

  it('returns empty string when token is missing', () => {
    document.documentElement.style.removeProperty('--c-ok')
    expect(getEdgeStroke('ok')).toBe('')
  })
})
