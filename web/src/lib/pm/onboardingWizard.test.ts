// @vitest-environment happy-dom
import { describe, expect, it, beforeEach, beforeAll, vi } from 'vitest'
import {
  DEFAULT_PROJECT_ID,
  ONBOARDING_STEPS,
  assembleBootstrapBody,
  detectSystemLocale,
  dismissOnboarding,
  freshOnboardingDraft,
  gitConfigured,
  isEmptyProjectForOnboarding,
  isOnboardingDismissed,
  onboardingDismissKey,
  gitIdentityConfigured,
  repoConfigured,
  repoNameFromUrl,
  shouldAutoOpenOnboarding,
} from './onboardingWizard'
import { i18n } from '@/lib/shared/i18n'
import { loadLocaleMessages } from '@/lib/shared/loadLocaleMessages'

beforeAll(async () => {
  const [zh, en] = await Promise.all([loadLocaleMessages('zh-CN'), loadLocaleMessages('en')])
  i18n.global.setLocaleMessage('zh-CN', zh)
  i18n.global.setLocaleMessage('en', en)
})

describe('onboardingWizard', () => {
  beforeEach(() => {
    localStorage.clear()
    i18n.global.locale.value = 'zh-CN'
  })

  it('starts with language and defaults it from the system locale', () => {
    expect(ONBOARDING_STEPS[0]?.id).toBe('language')
    vi.stubGlobal('navigator', { language: 'zh-CN' })
    expect(detectSystemLocale()).toBe('zh-CN')
    expect(freshOnboardingDraft().language).toBe('zh-CN')
    vi.stubGlobal('navigator', { language: 'en-US' })
    expect(detectSystemLocale()).toBe('en')
    expect(freshOnboardingDraft().language).toBe('en')
    vi.unstubAllGlobals()
  })

  it('assembles bootstrap body without heroku repos or featureHint', () => {
    const d = freshOnboardingDraft()
    d.acpBackend = 'codebuddy'
    d.region = 'public'
    d.apiKey = 'cb-key'
    d.gitCredentialType = 'github_https'
    d.githubToken = 'ghp_x'
    const body = assembleBootstrapBody(d)
    expect(body.apiKey).toBe('cb-key')
    expect(body.region).toBe('public')
    expect(body.gitCredentialType).toBe('github_https')
    expect(body.githubToken).toBe('ghp_x')
    expect(body).not.toHaveProperty('repos')
    expect(body).not.toHaveProperty('featureHint')
    expect(body.vncPreview).toBe(true)
    expect(body.browserMcp).toBe(true)
  })

  it('sends git identity and can turn preview flags off', () => {
    const d = freshOnboardingDraft()
    d.apiKey = 'k'
    expect(gitIdentityConfigured(d)).toBe(false)
    d.gitUserName = ' Ada Lovelace '
    d.gitUserEmail = ' ada@example.com '
    d.vncPreview = false
    d.browserMcp = false
    expect(gitIdentityConfigured(d)).toBe(true)
    const body = assembleBootstrapBody(d)
    expect(body.gitUserName).toBe('Ada Lovelace')
    expect(body.gitUserEmail).toBe('ada@example.com')
    expect(body.vncPreview).toBe(false)
    expect(body.browserMcp).toBe(false)
  })

  it('sends the repo only when a URL is given, branch only alongside it', () => {
    const d = freshOnboardingDraft()
    d.apiKey = 'k'
    expect(assembleBootstrapBody(d)).not.toHaveProperty('repoUrl')

    d.repoBranch = 'develop'
    expect(assembleBootstrapBody(d)).not.toHaveProperty('repoBranch')

    d.repoUrl = '  https://github.com/org/web.git  '
    const body = assembleBootstrapBody(d)
    expect(body.repoUrl).toBe('https://github.com/org/web.git')
    expect(body.repoBranch).toBe('develop')
  })

  it('derives the clone dir the same way the server does', () => {
    expect(repoNameFromUrl('https://github.com/org/web.git')).toBe('web')
    expect(repoNameFromUrl('https://git.host.cc/org/web')).toBe('web')
    expect(repoNameFromUrl('git@github.com:org/api.git')).toBe('api')
    expect(repoNameFromUrl('ssh://git@host/org/infra.git/')).toBe('infra')
    expect(repoNameFromUrl('   ')).toBe('')
  })

  it('repoConfigured tracks a non-blank URL', () => {
    const d = freshOnboardingDraft()
    expect(repoConfigured(d)).toBe(false)
    d.repoUrl = '   '
    expect(repoConfigured(d)).toBe(false)
    d.repoUrl = 'https://github.com/org/web.git'
    expect(repoConfigured(d)).toBe(true)
  })

  it('gitConfigured requires type and matching secret', () => {
    const d = freshOnboardingDraft()
    expect(gitConfigured(d)).toBe(false)
    d.gitCredentialType = 'github_https'
    expect(gitConfigured(d)).toBe(false)
    d.githubToken = 'tok'
    expect(gitConfigured(d)).toBe(true)
  })

  it('only treats the default project as empty for first-install', () => {
    expect(isEmptyProjectForOnboarding(0, [], DEFAULT_PROJECT_ID)).toBe(true)
    expect(isEmptyProjectForOnboarding(0, [], 'p1')).toBe(false)
    expect(isEmptyProjectForOnboarding(1, [], DEFAULT_PROJECT_ID)).toBe(false)
    expect(
      isEmptyProjectForOnboarding(0, [{ name: '综合研发工程师', projectId: DEFAULT_PROJECT_ID }], DEFAULT_PROJECT_ID),
    ).toBe(false)
  })

  it('treats cross-project first-install agent names as non-empty', () => {
    expect(
      isEmptyProjectForOnboarding(0, [{ name: '综合AI技术产品', projectId: 'other' }], DEFAULT_PROJECT_ID),
    ).toBe(false)
    expect(isEmptyProjectForOnboarding(0, [{ name: '综合AI技术产品', projectId: '' }], DEFAULT_PROJECT_ID)).toBe(true)
  })

  it('auto-open respects dismiss and default project', () => {
    expect(shouldAutoOpenOnboarding(DEFAULT_PROJECT_ID, 0, [])).toBe(true)
    expect(shouldAutoOpenOnboarding('p1', 0, [])).toBe(false)
    dismissOnboarding(DEFAULT_PROJECT_ID)
    expect(isOnboardingDismissed(DEFAULT_PROJECT_ID)).toBe(true)
    expect(localStorage.getItem(onboardingDismissKey(DEFAULT_PROJECT_ID))).toBe('1')
    expect(shouldAutoOpenOnboarding(DEFAULT_PROJECT_ID, 0, [])).toBe(false)
  })
})
