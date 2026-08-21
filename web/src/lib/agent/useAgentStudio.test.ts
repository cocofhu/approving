// @vitest-environment happy-dom
import { createApp, defineComponent, nextTick } from 'vue'
import { createI18n } from 'vue-i18n'
import { createMemoryHistory, createRouter } from 'vue-router'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises } from '@vue/test-utils'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'

const mocks = vi.hoisted(() => ({
  listAgents: vi.fn(),
  getAgentsOrg: vi.fn(),
  listProjects: vi.fn(),
}))

vi.mock('@/lib/api/api', async () => {
  const actual = await vi.importActual<typeof import('@/lib/api/api')>('@/lib/api/api')
  return {
    ...actual,
    api: {
      ...actual.api,
      listAgents: mocks.listAgents,
      getAgentsOrg: mocks.getAgentsOrg,
      listProjects: mocks.listProjects,
    },
  }
})

vi.mock('@/lib/composables/useBreakpoint', async () => {
  const { ref } = await import('vue')
  return { useBreakpoint: () => ({ isMobile: ref(false) }) }
})

vi.mock('@/lib/agent/useAgentImport', () => ({
  useAgentImport: () => ({
    importFileInput: { value: null },
    showImportDiscardConfirm: { value: false },
    triggerImport: vi.fn(),
    onImportFile: vi.fn(),
    confirmImportDiscard: vi.fn(),
    cancelImportDiscard: vi.fn(),
  }),
}))

import { useAgentStudio } from './useAgentStudio'

class MockResizeObserver {
  observe = vi.fn()
  disconnect = vi.fn()
  unobserve = vi.fn()
  constructor(_cb: ResizeObserverCallback) {}
}

async function withAgentStudio(path = '/agents?agent=agent-a&tab=files') {
  let studio!: ReturnType<typeof useAgentStudio>
  const i18n = createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...common, ...pages } },
  })
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/', component: { template: '<div />' } },
      { path: '/agents', component: { template: '<div />' } },
    ],
  })
  await router.push(path)
  await router.isReady()
  const Comp = defineComponent({
    setup() {
      studio = useAgentStudio()
      return () => null
    },
  })
  const app = createApp(Comp)
  app.use(i18n)
  app.use(router)
  app.mount(document.createElement('div'))
  return { studio, app, router }
}

describe('useAgentStudio', () => {
  beforeEach(() => {
    vi.stubGlobal('ResizeObserver', MockResizeObserver)
    mocks.listAgents.mockReset()
    mocks.getAgentsOrg.mockReset()
    mocks.listProjects.mockReset()

    mocks.listAgents.mockResolvedValue([
      { name: 'agent-a', projectId: 'proj-1', acpBackend: 'cursor' },
      { name: 'agent-b', projectId: '', acpBackend: 'cursor' },
    ])
    mocks.getAgentsOrg.mockResolvedValue({
      revision: 1,
      groups: [{ id: 'g1', name: 'Default', parentId: '', agentNames: ['agent-a'] }],
      agents: { 'agent-a': { groupId: 'g1' } },
    })
    mocks.listProjects.mockResolvedValue([{ id: 'proj-1', name: 'Proj 1' }])
  })

  afterEach(() => {
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  })

  it('loads agents/org on mount and selects query agent', async () => {
    const { studio, app } = await withAgentStudio()
    await flushPromises()
    await nextTick()

    expect(studio.hasInitialLoaded.value).toBe(true)
    expect(studio.agents.value.length).toBe(2)
    expect(studio.activeName.value).toBe('agent-a')
    expect(studio.tab.value).toBe('files')

    studio.toggleAgentListCollapsed()
    studio.closeFullNameTip()
    studio.closeMobileChromeOverlays()

    window.dispatchEvent(new Event('resize'))
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))

    app.unmount()
  })
})
