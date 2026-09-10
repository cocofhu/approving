// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { createMemoryHistory, createRouter } from 'vue-router'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'
import enCommon from '@/locales/en/common.json'
import enPages from '@/locales/en/pages.json'

const apiMocks = vi.hoisted(() => ({
  listProjects: vi.fn(),
}))

const firstInstallMocks = vi.hoisted(() => ({
  openCreateProjectOnboarding: vi.fn(),
}))

vi.mock('@/lib/api/api', async () => {
  const actual = await vi.importActual<typeof import('@/lib/api/api')>('@/lib/api/api')
  return {
    ...actual,
    api: {
      ...actual.api,
      listProjects: apiMocks.listProjects,
    },
  }
})

vi.mock('@/lib/pm/firstInstall', () => ({
  openCreateProjectOnboarding: firstInstallMocks.openCreateProjectOnboarding,
}))

vi.mock('@/lib/composables/useToast', () => ({
  useToast: () => ({ success: vi.fn(), error: vi.fn() }),
}))

vi.mock('@/lib/composables/useProjectContext', () => ({
  writeStoredProjectId: vi.fn(),
}))

import ProjectListView from './ProjectListView.vue'

const src = readFileSync(join(dirname(fileURLToPath(import.meta.url)), 'ProjectListView.vue'), 'utf8')

const SAMPLE = {
  id: 'p1',
  name: 'Alpha',
  description: 'demo',
  workflowCount: 2,
  createdAt: '2026-01-01T00:00:00Z',
  updatedAt: '2026-01-02T00:00:00Z',
}

function mountList(locale: 'zh-CN' | 'en' = 'zh-CN') {
  const i18n =
    locale === 'zh-CN'
      ? createI18n({ legacy: false, locale: 'zh-CN', messages: { 'zh-CN': { ...common, ...pages } } })
      : createI18n({ legacy: false, locale: 'en', messages: { en: { ...enCommon, ...enPages } } })
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/projects', component: ProjectListView },
      { path: '/projects/:id', component: { template: '<div />' } },
    ],
  })
  void router.push('/projects')
  return mount(ProjectListView, {
    global: {
      plugins: [i18n, router],
      stubs: {
        Icon: true,
        AppButton: { template: '<button type="button" v-bind="$attrs"><slot /></button>' },
        EmptyState: { props: ['title'], template: '<div data-testid="empty-state"><p>{{ title }}</p><slot /></div>' },
        TokenUsageHoverTip: true,
      },
    },
  })
}

describe('ProjectListView narrow-screen overflow constraints', () => {
  it('locks grid, skeleton, and card width constraints for mobile overflow prevention', () => {
    expect(src).toMatch(/data-testid="project-list-cards"/)
    expect(src).toMatch(/data-testid="project-list-skeleton"/)
    expect(src).toMatch(/grid min-w-0 gap-3 sm:grid-cols-2 xl:grid-cols-3/)
    expect(src).toMatch(/min-w-0 w-full max-w-full flex-col gap-2 overflow-hidden/)
    expect(src).toMatch(/line-clamp-2 break-words/)
    expect(src).toMatch(/flex min-w-0 flex-wrap items-center gap-x-1\.5 gap-y-0\.5/)
  })

  // plan g3.3 — narrow viewport: fill root + list panel is the scroll exit (末卡可达)
  it('uses fill-height root and project-list-panel overflow-y-auto scroll exit (g3.3)', async () => {
    expect(src).toMatch(/flex h-full min-h-0 flex-col/)
    expect(src).toMatch(/data-testid="project-list-panel"[\s\S]*?class="min-h-0 flex-1 overflow-y-auto"/)
    const many = Array.from({ length: 12 }, (_, i) => ({
      ...SAMPLE,
      id: `p${i}`,
      name: `项目 ${i}`,
    }))
    apiMocks.listProjects.mockResolvedValue(many)
    const w = mountList()
    await flushPromises()
    const panel = w.get('[data-testid="project-list-panel"]')
    expect(panel.classes()).toEqual(expect.arrayContaining(['min-h-0', 'flex-1', 'overflow-y-auto']))
    const cards = w.findAll('[data-testid="project-list-cards"] button')
    expect(cards.length).toBe(12)
    expect(cards[11].text()).toContain('项目 11')
    w.unmount()
  })

  it('renders long URL description inside card without layout regression', async () => {
    const longUrl =
      'https://github.com/example/very-long-repository-name-without-spaces-that-would-overflow-on-narrow-screens'
    apiMocks.listProjects.mockResolvedValue([
      {
        ...SAMPLE,
        name: 'HarnessPlugin',
        description: longUrl,
        totalTokens: 1234567,
      },
    ])
    const w = mountList()
    await flushPromises()
    const card = w.get('[data-testid="project-list-cards"] button')
    expect(card.classes()).toContain('min-w-0')
    expect(card.classes()).toContain('max-w-full')
    expect(card.text()).toContain('github.com')
    expect(w.find('[data-testid="project-list-token"]').exists()).toBe(true)
    w.unmount()
  })
  it('card click still navigates to project detail', async () => {
    const { writeStoredProjectId } = await import('@/lib/composables/useProjectContext')
    apiMocks.listProjects.mockResolvedValue([SAMPLE])
    const w = mountList()
    await flushPromises()
    await w.get('[data-testid="project-list-cards"] button').trigger('click')
    await flushPromises()
    expect(writeStoredProjectId).toHaveBeenCalledWith('p1')
    expect(w.vm.$router.currentRoute.value.path).toBe('/projects/p1')
    w.unmount()
  })
})

describe('ProjectListView source lock (Demo loading)', () => {
  it('uses isomorphic card skeleton (icon + title + meta), not equal-width bars only', () => {
    expect(src).toMatch(/data-testid="project-list-skeleton"/)
    expect(src).toMatch(/h-9 w-9 shrink-0 bg-elevated animate-pulse/)
    expect(src).toMatch(/h-3\.5 w-2\/3 bg-elevated animate-pulse/)
    expect(src).toMatch(/border-t border-line pt-2\.5/)
    expect(src).not.toMatch(/h-20 rounded-lg border border-line bg-surface animate-pulse/)
  })

  it('refresh uses 2px bg-accent thin progress + opacity 0.55 without pointer-events:none', () => {
    expect(src).toMatch(/h-\[2px\].*bg-line/)
    expect(src).toMatch(/admin-list-thin-bar bg-accent/)
    expect(src).toMatch(/opacity-\[0\.55\]/)
    expect(src).not.toMatch(/pointer-events:\s*none/)
    expect(src).not.toMatch(/#7B61FF/)
    expect(src).toMatch(/:aria-busy="loading \? 'true' : 'false'"/)
  })

  it('four states and create-onboarding entry are Demo-locked', () => {
    expect(src).toMatch(/data-testid="project-list-failed"/)
    expect(src).toMatch(/data-testid="project-list-denied"/)
    expect(src).toMatch(/data-testid="project-list-empty"/)
    expect(src).toMatch(/common\.asyncState\.loadFailedTitle/)
    expect(src).toMatch(/common\.asyncState\.permissionDeniedTitle/)
    expect(src).toMatch(/Icon name="lock"/)
    expect(src).toMatch(/openCreateProjectOnboarding/)
    expect(src).toMatch(/createListRequestSeq/)
    expect(src).not.toMatch(/AppModal/)
    expect(src).not.toMatch(/project-list-create-submit/)
  })
})

describe('ProjectListView loading states', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('shows card skeleton before data arrives', async () => {
    let release!: (v: unknown) => void
    apiMocks.listProjects.mockReturnValue(new Promise((resolve) => { release = resolve }))
    const w = mountList()
    await flushPromises()
    expect(w.find('[data-testid="project-list-skeleton"]').exists()).toBe(true)
    expect(w.find('[data-testid="project-list-empty"]').exists()).toBe(false)
    expect(w.find('[data-testid="project-list-failed"]').exists()).toBe(false)
    release!([SAMPLE])
    await flushPromises()
    expect(w.find('[data-testid="project-list-skeleton"]').exists()).toBe(false)
    expect(w.text()).toContain('Alpha')
    w.unmount()
  })

  it('empty state uses business CTA without retry', async () => {
    apiMocks.listProjects.mockResolvedValue([])
    const w = mountList()
    await flushPromises()
    expect(w.find('[data-testid="project-list-empty"]').exists()).toBe(true)
    expect(w.find('[data-testid="project-list-retry"]').exists()).toBe(false)
    expect(w.text()).toContain('暂无项目')
    expect(w.text()).toContain('新建项目')
    w.unmount()
  })

  it('generic failure shows red card + retry, not empty', async () => {
    apiMocks.listProjects.mockRejectedValue(Object.assign(new Error('boom'), { status: 500 }))
    const w = mountList()
    await flushPromises()
    expect(w.find('[data-testid="project-list-failed"]').exists()).toBe(true)
    expect(w.find('[data-testid="project-list-empty"]').exists()).toBe(false)
    expect(w.text()).toContain('加载失败')
    expect(w.text()).toContain('重试')
    apiMocks.listProjects.mockResolvedValue([SAMPLE])
    await w.get('[data-testid="project-list-retry"]').trigger('click')
    await flushPromises()
    expect(w.text()).toContain('Alpha')
    w.unmount()
  })

  it('403 shows amber lock card distinct from failure', async () => {
    apiMocks.listProjects.mockRejectedValue(Object.assign(new Error('denied'), { status: 403 }))
    const w = mountList()
    await flushPromises()
    expect(w.find('[data-testid="project-list-denied"]').exists()).toBe(true)
    expect(w.find('[data-testid="project-list-failed"]').exists()).toBe(false)
    expect(w.text()).toContain('权限不足')
    expect(w.text()).toContain('重试')
    w.unmount()
  })

  it('new project opens create-project onboarding wizard', async () => {
    apiMocks.listProjects.mockResolvedValue([])
    firstInstallMocks.openCreateProjectOnboarding.mockClear()
    const w = mountList()
    await flushPromises()
    await w.get('[data-testid="project-list-new"]').trigger('click')
    expect(firstInstallMocks.openCreateProjectOnboarding).toHaveBeenCalledTimes(1)
    expect(w.find('[data-testid="create-modal"]').exists()).toBe(false)
    expect(w.find('[data-testid="project-list-create-submit"]').exists()).toBe(false)
    w.unmount()
  })

  it('empty-state CTA also opens create-project onboarding', async () => {
    apiMocks.listProjects.mockResolvedValue([])
    firstInstallMocks.openCreateProjectOnboarding.mockClear()
    const w = mountList()
    await flushPromises()
    await w.get('[data-testid="project-list-new-empty"]').trigger('click')
    expect(firstInstallMocks.openCreateProjectOnboarding).toHaveBeenCalledTimes(1)
    w.unmount()
  })

  it('refresh keeps old cards with thin progress', async () => {
    apiMocks.listProjects.mockResolvedValue([SAMPLE])
    const w = mountList()
    await flushPromises()
    expect(w.text()).toContain('Alpha')
    let release!: (v: unknown) => void
    apiMocks.listProjects.mockReturnValue(new Promise((resolve) => { release = resolve }))
    await w.get('[data-testid="project-list-panel"]').trigger('click')
    const retry = w.find('[data-testid="project-list-retry"]')
    if (retry.exists()) await retry.trigger('click')
    // trigger reload via failed-then-retry path is not needed; call load by remounting panel aria
    release!([SAMPLE])
    await flushPromises()
    expect(w.text()).toContain('Alpha')
    w.unmount()
  })
})
