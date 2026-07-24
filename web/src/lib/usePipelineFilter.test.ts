import { describe, expect, it, vi } from 'vitest'

const replace = vi.fn()
const route = { query: {} as Record<string, string> }

vi.mock('vue-router', () => ({
  useRoute: () => route,
  useRouter: () => ({ replace }),
}))

import { PIPELINE_FILTER_KEYS, usePipelineFilter } from './usePipelineFilter'

describe('usePipelineFilter', () => {
  it('reads and writes wf query param', () => {
    expect(PIPELINE_FILTER_KEYS.all).toContain('pipelineFilter')
    route.query = {}
    const { selected } = usePipelineFilter()
    expect(selected.value).toBe('')
    selected.value = 'wf-1'
    expect(replace).toHaveBeenCalledWith({ query: { wf: 'wf-1' } })
    route.query = { wf: 'wf-1', other: '1' }
    selected.value = ''
    expect(replace).toHaveBeenCalledWith({ query: { other: '1' } })
  })
})
