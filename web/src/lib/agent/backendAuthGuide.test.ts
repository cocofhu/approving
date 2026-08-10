import { describe, expect, it } from 'vitest'
import { authGuideFor, hasAuthKeyConfigured } from './backendAuthGuide'

describe('backendAuthGuide', () => {
  it('returns Cursor console path + official entry links', () => {
    const guide = authGuideFor('cursor')
    expect(guide.keys[0]).toMatchObject({
      key: 'APPROVING_CURSOR_API_KEY',
      alt: 'CURSOR_API_KEY',
    })
    expect(guide.pathStepKeys.length).toBeGreaterThanOrEqual(2)
    expect(guide.links.some((l) => l.url.includes('cursor.com/dashboard'))).toBe(true)
    expect(guide.links.some((l) => l.url.includes('cursor.com/docs'))).toBe(true)
  })

  it('picks CodeBuddy deep link for the current site', () => {
    const publicGuide = authGuideFor('codebuddy', 'public')
    expect(publicGuide.links[0].url).toContain('codebuddy.ai/profile/keys')
    const internalGuide = authGuideFor('codebuddy', 'internal')
    expect(internalGuide.links[0].url).toContain('copilot.tencent.com/profile')
  })

  it('includes Trae CLI login-token documentation link', () => {
    const guide = authGuideFor('trae')
    expect(guide.links.some((l) => l.url.includes('cli_login-token'))).toBe(true)
  })

  it('detects configured auth keys including aliases', () => {
    expect(hasAuthKeyConfigured({}, 'cursor')).toBe(false)
    expect(
      hasAuthKeyConfigured([{ k: 'CURSOR_API_KEY', v: 'x' }], 'cursor'),
    ).toBe(true)
    expect(
      hasAuthKeyConfigured({ TRAE_API_KEY: 'trae-lt-x' }, 'trae'),
    ).toBe(true)
  })
})
