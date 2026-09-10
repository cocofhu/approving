import { describe, expect, it } from 'vitest'
import {
  applyOpenCodeFields,
  normalizeOpenCodeProvider,
  OPENCODE_FALLBACK_PROVIDERS,
  openCodeCustomBaseRequired,
  openCodeFieldsFromEnv,
  openCodeModelMatchesProvider,
  openCodeModelRequired,
  switchOpenCodeEnv,
} from './openCodeProvider'

describe('openCodeProvider', () => {
  it('keeps any catalog-shaped vendor id', () => {
    expect(normalizeOpenCodeProvider('')).toBe('openai')
    expect(normalizeOpenCodeProvider('Anthropic')).toBe('anthropic')
    // The vendor list is the catalog's, so an id we never enumerated survives.
    expect(normalizeOpenCodeProvider('ZAI')).toBe('zai')
    expect(normalizeOpenCodeProvider('my-gateway.v2')).toBe('my-gateway.v2')
    expect(normalizeOpenCodeProvider('has space')).toBe('custom')
  })

  it('requires base URL for custom', () => {
    expect(openCodeCustomBaseRequired('custom', '')).toBe(true)
    expect(openCodeCustomBaseRequired('custom', 'https://x')).toBe(false)
    expect(openCodeCustomBaseRequired('openai', '')).toBe(false)
  })

  it('requires a model id', () => {
    expect(openCodeModelRequired('')).toBe(true)
    expect(openCodeModelRequired('  ')).toBe(true)
    expect(openCodeModelRequired('anthropic/claude-sonnet-4-5')).toBe(false)
  })

  it('offers a translated shortlist for an unreachable catalog', () => {
    const ids = OPENCODE_FALLBACK_PROVIDERS.map((p) => p.id)
    expect(ids).toContain('custom')
    expect(ids).toContain('deepseek')
    for (const p of OPENCODE_FALLBACK_PROVIDERS) {
      expect(p.labelKey, p.id).toMatch(/^pages\.agentStudio\.openCode\.providers\./)
      expect(normalizeOpenCodeProvider(p.id)).toBe(p.id)
    }
  })

  it('tells whether a model still names the selected vendor', () => {
    expect(openCodeModelMatchesProvider('deepseek/deepseek-v4-pro', 'deepseek')).toBe(true)
    expect(openCodeModelMatchesProvider('deepseek/deepseek-v4-pro', 'zai')).toBe(false)
    // Empty and bare ids name no vendor, so neither contradicts one.
    expect(openCodeModelMatchesProvider('', 'zai')).toBe(true)
    expect(openCodeModelMatchesProvider('my-model', 'custom')).toBe(true)
    // OpenRouter model ids carry their own slash and still belong to the vendor.
    expect(openCodeModelMatchesProvider('openrouter/qwen/qwen3.7-max', 'openrouter')).toBe(true)
  })

  it('writes and strips managed OpenCode env', () => {
    const env = applyOpenCodeFields(
      {},
      { provider: 'custom', baseURL: 'https://llm.example/v1', model: 'foo' },
    )
    expect(openCodeFieldsFromEnv(env)).toEqual({
      provider: 'custom',
      baseURL: 'https://llm.example/v1',
      model: 'foo',
    })
    expect(switchOpenCodeEnv(env, 'cursor')).toEqual({ ACP_BRIDGE_MODEL: 'foo' })
    expect(switchOpenCodeEnv({}, 'opencode').APPROVING_OPENCODE_PROVIDER).toBe('openai')
  })
})
