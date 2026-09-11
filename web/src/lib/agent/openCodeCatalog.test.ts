import { beforeEach, describe, expect, it, vi } from 'vitest'

const openCodeProviders = vi.fn()
const openCodeModels = vi.fn()

vi.mock('@/lib/api/api', () => ({
  api: {
    openCodeProviders: (...args: unknown[]) => openCodeProviders(...args),
    openCodeModels: (...args: unknown[]) => openCodeModels(...args),
  },
}))

const {
  loadOpenCodeModels,
  loadOpenCodeProviders,
  openCodeCatalogKnowsProvider,
  resetOpenCodeCatalog,
} = await import('./openCodeCatalog')
const { openCodeCustomBaseRequired } = await import('./openCodeProvider')

describe('openCodeCatalog', () => {
  beforeEach(() => {
    resetOpenCodeCatalog()
    openCodeProviders.mockReset()
    openCodeModels.mockReset()
  })

  it('reads vendors and models through the server', async () => {
    openCodeProviders.mockResolvedValue({
      providers: [{ id: 'deepseek', name: 'DeepSeek', models: 3 }],
    })
    openCodeModels.mockResolvedValue({ models: [{ id: 'deepseek-v4-pro', name: 'DeepSeek V4 Pro' }] })
    expect(await loadOpenCodeProviders()).toEqual([{ id: 'deepseek', name: 'DeepSeek', models: 3 }])
    expect(await loadOpenCodeModels('DeepSeek')).toEqual([
      { id: 'deepseek-v4-pro', name: 'DeepSeek V4 Pro' },
    ])
    expect(openCodeModels).toHaveBeenCalledWith('deepseek')
  })

  it('requires a base URL for a typed vendor absent from a readable catalog', async () => {
    openCodeProviders.mockResolvedValue({
      providers: [{ id: 'openai', name: 'OpenAI', models: 1 }],
    })
    await loadOpenCodeProviders()

    expect(openCodeCatalogKnowsProvider('OpenAI')).toBe(true)
    expect(openCodeCatalogKnowsProvider('tokenhub')).toBe(false)
    expect(openCodeCustomBaseRequired('tokenhub', '')).toBe(true)
    expect(openCodeCustomBaseRequired('tokenhub', 'https://tokenhub.example/v1')).toBe(false)
  })

  it('asks once per vendor', async () => {
    openCodeProviders.mockResolvedValue({ providers: [] })
    openCodeModels.mockResolvedValue({ models: [] })
    await Promise.all([loadOpenCodeProviders(), loadOpenCodeProviders(), loadOpenCodeProviders()])
    await Promise.all([loadOpenCodeModels('zai'), loadOpenCodeModels('zai')])
    expect(openCodeProviders).toHaveBeenCalledTimes(1)
    expect(openCodeModels).toHaveBeenCalledTimes(1)
  })

  // The catalog is a convenience: a failure must leave the form usable with a
  // hand-typed id rather than surface as an error.
  it('resolves to empty lists when the request fails', async () => {
    openCodeProviders.mockRejectedValue(new Error('offline'))
    openCodeModels.mockRejectedValue(new Error('offline'))
    expect(await loadOpenCodeProviders()).toEqual([])
    expect(await loadOpenCodeModels('deepseek')).toEqual([])
    expect(openCodeCatalogKnowsProvider('deepseek')).toBeUndefined()
    expect(openCodeCustomBaseRequired('deepseek', '')).toBe(false)
  })

  it('does not ask for an empty vendor', async () => {
    expect(await loadOpenCodeModels('  ')).toEqual([])
    expect(openCodeModels).not.toHaveBeenCalled()
  })

  it('forgets memoized answers on reset', async () => {
    openCodeProviders.mockResolvedValue({ providers: [] })
    await loadOpenCodeProviders()
    resetOpenCodeCatalog()
    await loadOpenCodeProviders()
    expect(openCodeProviders).toHaveBeenCalledTimes(2)
  })
})
