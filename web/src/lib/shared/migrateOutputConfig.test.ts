import { describe, expect, it } from 'vitest'
import {
  cleanOutputConfigForSave,
  migrateAndCleanOutputNodes,
  migrateOutputConfig,
  migrateOutputNodes,
} from './migrateOutputConfig'

describe('migrateOutputConfig', () => {
  it('converts non-empty result to results array', () => {
    const { config, migrated } = migrateOutputConfig({ result: '{{nodes.a.outputs.plan}}' })
    expect(migrated).toBe(true)
    expect(config.results).toEqual(['{{nodes.a.outputs.plan}}'])
  })

  it('empty result yields empty results', () => {
    const { config, migrated } = migrateOutputConfig({ result: '' })
    expect(migrated).toBe(true)
    expect(config.results).toEqual([])
  })

  it('skips when results already present', () => {
    const { config, migrated } = migrateOutputConfig({ results: ['a'], result: 'old' })
    expect(migrated).toBe(false)
    expect(config.results).toEqual(['a'])
  })
})

describe('migrateOutputNodes', () => {
  it('migrates output nodes in graph', () => {
    const nodes = [
      { type: 'input', config: {} },
      { type: 'output', config: { result: '{{artifact("x.md")}}' } },
    ]
    expect(migrateOutputNodes(nodes)).toBe(true)
    expect((nodes[1].config as Record<string, unknown>)?.results).toEqual(['{{artifact("x.md")}}'])
  })
})

describe('cleanOutputConfigForSave', () => {
  it('removes result and keeps results', () => {
    const out = cleanOutputConfigForSave({ result: 'old', results: ['a', 'b'] })
    expect(out.result).toBeUndefined()
    expect(out.results).toEqual(['a', 'b'])
  })
})

describe('migrateAndCleanOutputNodes', () => {
  it('migrates then strips legacy result for baseline', () => {
    const nodes = [
      { type: 'output', config: { result: '{{artifact("x.md")}}' } },
    ]
    expect(migrateAndCleanOutputNodes(nodes)).toBe(true)
    const cfg = nodes[0].config as Record<string, unknown>
    expect(cfg.result).toBeUndefined()
    expect(cfg.results).toEqual(['{{artifact("x.md")}}'])
  })
})
