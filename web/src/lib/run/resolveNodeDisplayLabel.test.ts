import { describe, expect, it } from 'vitest'
import { resolveNodeDisplayLabel, resolveNodeDisplayLabelFromNode } from './resolveNodeDisplayLabel'

const t = (key: string) => {
  const map: Record<string, string> = {
    'nodes.test.label': '测试',
    'nodes.plan.label': '计划',
  }
  return map[key] ?? key
}

describe('resolveNodeDisplayLabel', () => {
  it('translates when label equals the raw registry key', () => {
    expect(resolveNodeDisplayLabel('nodes.test.label', 'test', t)).toBe('测试')
  })

  it('returns custom labels unchanged', () => {
    expect(resolveNodeDisplayLabel('测试节点', 'test', t)).toBe('测试节点')
  })

  it('falls back to nodeId when label is empty', () => {
    expect(resolveNodeDisplayLabel('', 'test', t, { nodeId: 'test_abc' })).toBe('test_abc')
  })

  it('falls back to translated type name when label is empty and no nodeId', () => {
    expect(resolveNodeDisplayLabel(undefined, 'plan', t, { typeLabel: '计划' })).toBe('计划')
  })
})

describe('resolveNodeDisplayLabelFromNode', () => {
  it('delegates to resolveNodeDisplayLabel with node fields', () => {
    const node = { id: 'test_1', type: 'test' as const, label: 'nodes.test.label' }
    expect(resolveNodeDisplayLabelFromNode(node, t)).toBe('测试')
  })
})
