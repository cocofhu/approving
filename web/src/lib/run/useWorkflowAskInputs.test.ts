import { describe, expect, it } from 'vitest'
import { isAskValueBlank, missingRequiredAskField, useWorkflowAskInputs } from './useWorkflowAskInputs'
import type { Workflow } from '../shared/types'

describe('useWorkflowAskInputs', () => {
  it('maps ask variables including repos defaults', () => {
    const wf = {
      id: 'w1',
      name: 'wf',
      nodes: [
        {
          id: 'in',
          type: 'input',
          label: '输入',
          position: { x: 0, y: 0 },
          config: {
            variables: [
              { name: 'title', ask: true, type: 'string', value: 'hi', required: true },
              { name: 'repos', ask: true, type: 'repos', value: [{ url: 'u' }] },
              { name: 'hidden', ask: false, type: 'string', value: 'x' },
              null,
            ],
          },
        },
      ],
      edges: [],
    } as unknown as Workflow

    const { fields } = useWorkflowAskInputs(wf)
    expect(fields.value).toHaveLength(2)
    expect(fields.value[0]).toMatchObject({ key: 'title', type: 'text', default: 'hi' })
    expect(fields.value[1].default).toContain('url')
  })

  it('treats empty repos JSON as a missing required ask field', () => {
    const wf = {
      nodes: [
        {
          id: 'in',
          type: 'input',
          label: 'in',
          position: { x: 0, y: 0 },
          config: {
            variables: [{ name: 'repos', ask: true, required: true, type: 'repos', value: [] }],
          },
        },
      ],
    } as unknown as Workflow
    expect(missingRequiredAskField(wf)?.key).toBe('repos')
    expect(isAskValueBlank('repos', '[]')).toBe(true)
    expect(isAskValueBlank('text', 'hi')).toBe(false)
  })
})
