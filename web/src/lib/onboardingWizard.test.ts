// @vitest-environment happy-dom
import { describe, expect, it, beforeEach } from 'vitest'
import {
  DEFAULT_ONBOARDING_REPOS_LITERAL,
  assembleBootstrapBody,
  dismissOnboarding,
  encodeReposLiteral,
  freshOnboardingDraft,
  isEmptyProjectForOnboarding,
  isOnboardingDismissed,
  onboardingDismissKey,
  parseReposLiteral,
  shouldAutoOpenOnboarding,
} from './onboardingWizard'

describe('onboardingWizard', () => {
  beforeEach(() => {
    localStorage.clear()
  })

  it('encodes default heroku well-known repos literal', () => {
    const d = freshOnboardingDraft()
    expect(encodeReposLiteral(d.repo)).toBe(DEFAULT_ONBOARDING_REPOS_LITERAL)
    expect(DEFAULT_ONBOARDING_REPOS_LITERAL).toContain('heroku/nodejs-getting-started')
    expect(DEFAULT_ONBOARDING_REPOS_LITERAL.toLowerCase()).not.toContain('approving-demo')
  })

  it('round-trips repos literal', () => {
    const lit = 'demo|https://github.com/heroku/nodejs-getting-started.git|main'
    expect(encodeReposLiteral(parseReposLiteral(lit))).toBe(lit)
  })

  it('assembles bootstrap body with optional region', () => {
    const d = freshOnboardingDraft()
    d.acpBackend = 'codebuddy'
    d.region = 'public'
    d.apiKey = 'cb-key'
    const body = assembleBootstrapBody(d)
    expect(body.apiKey).toBe('cb-key')
    expect(body.region).toBe('public')
    expect(body.repos).toContain('heroku/nodejs-getting-started')
  })

  it('detects empty project by 0 workflows and 0 bound agents', () => {
    expect(isEmptyProjectForOnboarding(0, [], 'p1')).toBe(true)
    expect(isEmptyProjectForOnboarding(1, [], 'p1')).toBe(false)
    expect(isEmptyProjectForOnboarding(0, [{ projectId: 'p1' }], 'p1')).toBe(false)
    expect(isEmptyProjectForOnboarding(0, [{ projectId: 'other' }], 'p1')).toBe(true)
  })

  it('auto-open respects dismiss and emptiness', () => {
    expect(shouldAutoOpenOnboarding('p1', 0, [])).toBe(true)
    dismissOnboarding('p1')
    expect(isOnboardingDismissed('p1')).toBe(true)
    expect(localStorage.getItem(onboardingDismissKey('p1'))).toBe('1')
    expect(shouldAutoOpenOnboarding('p1', 0, [])).toBe(false)
    expect(shouldAutoOpenOnboarding('p1', 0, [])).toBe(false) // CTA may still open manually
  })
})
