// @vitest-environment happy-dom
import { beforeEach, describe, expect, it, vi } from 'vitest'

const replace = vi.fn()
const route = { query: {} as Record<string, string> }

vi.mock('vue-router', () => ({
  useRoute: () => route,
  useRouter: () => ({ replace }),
}))

import {
  PROJECT_CONTEXT_STORAGE_KEY,
  readStoredProjectId,
  useProjectContext,
  writeStoredProjectId,
} from './useProjectContext'

describe('useProjectContext', () => {
  beforeEach(() => {
    localStorage.clear()
    route.query = {}
    replace.mockReset()
  })

  it('reads/writes storage and hydrates URL', () => {
    expect(readStoredProjectId()).toBe('')
    writeStoredProjectId('p1')
    expect(localStorage.getItem(PROJECT_CONTEXT_STORAGE_KEY)).toBe('p1')
    writeStoredProjectId('')
    expect(localStorage.getItem(PROJECT_CONTEXT_STORAGE_KEY)).toBeNull()

    writeStoredProjectId('p2')
    const ctx = useProjectContext()
    expect(ctx.selected.value).toBe('')
    ctx.ensureHydrated()
    expect(replace).toHaveBeenCalledWith({ query: { projectId: 'p2' } })

    route.query = { projectId: 'p3' }
    const ctx2 = useProjectContext()
    ctx2.ensureHydrated()
    expect(localStorage.getItem(PROJECT_CONTEXT_STORAGE_KEY)).toBe('p3')
    expect(ctx2.selected.value).toBe('p3')

    ctx2.setProject('p4')
    expect(replace).toHaveBeenCalledWith({ query: { projectId: 'p4' } })
    ctx2.setProject('')
    expect(replace).toHaveBeenCalledWith({ query: {} })
  })
})
