// @vitest-environment happy-dom
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createI18n } from 'vue-i18n'
import { createApp, defineComponent, nextTick } from 'vue'
import common from '@/locales/zh-CN/common.json'
import {
  WORKFLOW_FAVORITES_MAX,
  favoritesKeyForUser,
  hydrateFromStorage,
  loadFavoriteEntries,
  useWorkflowFavorites,
} from './useWorkflowFavorites'
import { useAuth } from '@/lib/composables/useAuth'

vi.mock('@/lib/api/api', () => ({
  api: {
    listProjects: vi.fn(async () => [{ id: 'p1', name: 'Proj A' }]),
    getWorkflow: vi.fn(async (id: string) => {
      if (id === 'gone') {
        const err = Object.assign(new Error('missing'), { status: 404 })
        throw err
      }
      return {
        id,
        projectId: 'p1',
        name: `WF ${id}`,
        description: '',
        status: id === 'draft-1' ? 'draft' : 'published',
        version: 1,
        updatedAt: '',
        needsRepo: false,
        nodes: [],
        edges: [],
      }
    }),
  },
}))

function withSetup<T>(fn: () => T): T {
  let result!: T
  const i18n = createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': common },
  })
  const Comp = defineComponent({
    setup() {
      result = fn()
      return () => null
    },
  })
  const app = createApp(Comp)
  app.use(i18n)
  app.mount(document.createElement('div'))
  return result
}

beforeEach(() => {
  localStorage.clear()
  const auth = useAuth()
  auth.setUser({ username: 'dev.li', expiresAt: '' })
  hydrateFromStorage()
})

describe('useWorkflowFavorites', () => {
  it('keys storage by username and isolates accounts', () => {
    expect(favoritesKeyForUser('dev.li')).toBe('approving.workflowFavorites.dev.li')
    const fav = withSetup(() => useWorkflowFavorites())
    fav.toggleFavorite('wf-a', { name: 'A', silent: true })
    expect(loadFavoriteEntries('dev.li')).toHaveLength(1)

    useAuth().setUser({ username: 'other', expiresAt: '' })
    hydrateFromStorage()
    const fav2 = withSetup(() => useWorkflowFavorites())
    expect(fav2.listSorted.value).toHaveLength(0)
    expect(fav2.isFavorite('wf-a')).toBe(false)
  })

  it('sorts by favoritedAt desc and refreshes on re-favorite', async () => {
    vi.useFakeTimers()
    const fav = withSetup(() => useWorkflowFavorites())
    vi.setSystemTime(1000)
    fav.toggleFavorite('wf-a', { silent: true })
    vi.setSystemTime(2000)
    fav.toggleFavorite('wf-b', { silent: true })
    expect(fav.listSorted.value.map((e) => e.workflowId)).toEqual(['wf-b', 'wf-a'])
    fav.toggleFavorite('wf-a', { silent: true }) // unfavorite
    vi.setSystemTime(3000)
    fav.toggleFavorite('wf-a', { silent: true }) // re-favorite → top
    expect(fav.listSorted.value.map((e) => e.workflowId)).toEqual(['wf-a', 'wf-b'])
    vi.useRealTimers()
  })

  it('rejects the 9th favorite without LRU eviction', () => {
    const fav = withSetup(() => useWorkflowFavorites())
    for (let i = 0; i < WORKFLOW_FAVORITES_MAX; i++) {
      expect(fav.toggleFavorite(`wf-${i}`, { silent: true })).toBe(true)
    }
    expect(fav.toggleFavorite('wf-extra', { silent: true })).toBe(false)
    expect(fav.listSorted.value).toHaveLength(WORKFLOW_FAVORITES_MAX)
    expect(fav.isFavorite('wf-extra')).toBe(false)
    // Unfavorite still works when full
    expect(fav.toggleFavorite('wf-0', { silent: true })).toBe(false)
    expect(fav.listSorted.value).toHaveLength(WORKFLOW_FAVORITES_MAX - 1)
  })

  it('does not duplicate on repeated favorite', () => {
    const fav = withSetup(() => useWorkflowFavorites())
    fav.toggleFavorite('wf-a', { silent: true })
    fav.toggleFavorite('wf-a', { silent: true }) // unfavorite
    fav.toggleFavorite('wf-a', { silent: true })
    expect(fav.listSorted.value.filter((e) => e.workflowId === 'wf-a')).toHaveLength(1)
  })

  it('hydrates display names and silently strips 404 favorites', async () => {
    const fav = withSetup(() => useWorkflowFavorites())
    fav.toggleFavorite('wf-ok', { silent: true })
    fav.toggleFavorite('gone', { silent: true })
    await fav.hydrateDisplay()
    await nextTick()
    expect(fav.displayItems.value.map((d) => d.workflowId)).toEqual(['gone', 'wf-ok'].filter((id) => id !== 'gone'))
    // After strip, only wf-ok remains in storage (gone was favorited later so would be first before strip)
    expect(fav.isFavorite('gone')).toBe(false)
    expect(fav.isFavorite('wf-ok')).toBe(true)
    expect(fav.displayItems.value[0]?.name).toBe('WF wf-ok')
    expect(fav.displayItems.value[0]?.projectName).toBe('Proj A')
  })

  it('clears to empty after localStorage wipe', () => {
    const fav = withSetup(() => useWorkflowFavorites())
    fav.toggleFavorite('wf-a', { silent: true })
    localStorage.clear()
    hydrateFromStorage()
    expect(fav.listSorted.value).toHaveLength(0)
  })
})
