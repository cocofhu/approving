// @vitest-environment happy-dom
import { describe, expect, it, beforeEach, afterEach, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { PROJECT_CONTEXT_STORAGE_KEY } from '@/lib/composables/useProjectContext'

const replace = vi.fn()

vi.mock('vue-router', () => ({
  useRouter: () => ({ replace }),
}))

import BoardRedirectView from './BoardRedirectView.vue'

describe('BoardRedirectView', () => {
  beforeEach(() => {
    localStorage.clear()
    replace.mockReset()
  })

  afterEach(() => {
    localStorage.clear()
  })

  it('replaces to remembered project board when memory exists', async () => {
    localStorage.setItem(PROJECT_CONTEXT_STORAGE_KEY, 'proj-remembered')
    mount(BoardRedirectView)
    await flushPromises()
    expect(replace).toHaveBeenCalledWith({
      path: '/projects/proj-remembered',
      query: { tab: 'board' },
    })
  })

  it('replaces to projects list when no memory', async () => {
    mount(BoardRedirectView)
    await flushPromises()
    expect(replace).toHaveBeenCalledWith({ path: '/projects' })
  })
})
