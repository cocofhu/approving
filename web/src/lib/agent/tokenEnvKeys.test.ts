import { describe, expect, it } from 'vitest'
import {
  isGitTokenEnvKey,
  isTokenEnvKey,
  stripTokenKeysFromKV,
  stripTokenKeysFromRecord,
  TOKEN_ENV_KEYS,
} from './tokenEnvKeys'

describe('tokenEnvKeys', () => {
  it('recognizes ACP + Git token keys', () => {
    expect(isTokenEnvKey('APPROVING_CURSOR_API_KEY')).toBe(true)
    expect(isTokenEnvKey('CURSOR_API_KEY')).toBe(true)
    expect(isTokenEnvKey('GITLAB_TOKEN')).toBe(true)
    expect(isTokenEnvKey('GIT_SSH_PRIVATE_KEY')).toBe(true)
    expect(TOKEN_ENV_KEYS.length).toBeGreaterThanOrEqual(12)
  })

  it('excludes repos / urls / region / known_hosts', () => {
    expect(isTokenEnvKey('GIT_REPOS')).toBe(false)
    expect(isTokenEnvKey('GITHUB_URL')).toBe(false)
    expect(isTokenEnvKey('GIT_SSH_KNOWN_HOSTS')).toBe(false)
    expect(isTokenEnvKey('APPROVING_CODEBUDDY_REGION')).toBe(false)
  })

  it('strips only token keys', () => {
    expect(
      stripTokenKeysFromRecord({
        APPROVING_CURSOR_API_KEY: 'x',
        GIT_REPOS: 'a|https://x',
        FEATURE_FLAG: '1',
      }),
    ).toEqual({ GIT_REPOS: 'a|https://x', FEATURE_FLAG: '1' })
    expect(
      stripTokenKeysFromKV([
        { k: 'GITLAB_TOKEN', v: 't' },
        { k: 'LOG_LEVEL', v: 'info' },
      ]),
    ).toEqual([{ k: 'LOG_LEVEL', v: 'info' }])
  })

  it('marks git token keys for recommend filter', () => {
    expect(isGitTokenEnvKey('GITHUB_TOKEN')).toBe(true)
    expect(isGitTokenEnvKey('GIT_SSH_KNOWN_HOSTS')).toBe(false)
  })
})
