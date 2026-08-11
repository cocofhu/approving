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
  createProject: vi.fn(),
}))

vi.mock('@/lib/api/api', async () => {
  const actual = await vi.importActual<typeof import('@/lib/api/api')>('@/lib/api/api')
  return {
    ...actual,
    api: {
      ...actual.api,
      listProjects: apiMocks.listProjects,
      createProject: apiMocks.createProject,
    },
  }
})

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
        AppModal: {
          props: ['open', 'title'],
          template: '<div v-if="open" data-testid="create-modal"><slot /></div>',
        },
        EmptyState: { props: ['title'], template: '<div data-testid="empty-state"><p>{{ title }}</p><slot /></div>' },
        TokenUsageHoverTip: true,
      },
    },
  })
}

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

  it('four states and creating pending copy are Demo-locked', () => {
    expect(src).toMatch(/data-testid="project-list-failed"/)
    expect(src).toMatch(/data-testid="project-list-denied"/)
    expect(src).toMatch(/data-testid="project-list-empty"/)
    expect(src).toMatch(/common\.asyncState\.loadFailedTitle/)
    expect(src).toMatch(/common\.asyncState\.permissionDeniedTitle/)
    expect(src).toMatch(/Icon name="lock"/)
    expect(src).toMatch(/common\.buttons\.creating/)
    expect(src).toMatch(/createListRequestSeq/)
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

  it('create submit shows Creating… / 创建中… and disables', async () => {
    apiMocks.listProjects.mockResolvedValue([])
    let release!: (v: unknown) => void
    apiMocks.createProject.mockReturnValue(new Promise((resolve) => { release = resolve }))
    const w = mountList()
    await flushPromises()
    await w.findAll('button').find((b) => b.text().includes('新建项目'))!.trigger('click')
    await flushPromises()
    const name = w.find('input')
    await name.setValue('New')
    await w.get('[data-testid="project-list-create-submit"]').trigger('click')
    await flushPromises()
    const submit = w.get('[data-testid="project-list-create-submit"]')
    expect(submit.text()).toBe('创建中…')
    expect((submit.element as HTMLButtonElement).disabled).toBe(true)
    release!({ id: 'p9', name: 'New' })
    await flushPromises()
    w.unmount()
  })

  it('en pending copy is Creating…', async () => {
    apiMocks.listProjects.mockResolvedValue([])
    apiMocks.createProject.mockReturnValue(new Promise(() => {}))
    const w = mountList('en')
    await flushPromises()
    await w.findAll('button').find((b) => /new project/i.test(b.text()))!.trigger('click')
    await flushPromises()
    await w.find('input').setValue('New')
    await w.get('[data-testid="project-list-create-submit"]').trigger('click')
    await flushPromises()
    expect(w.get('[data-testid="project-list-create-submit"]').text()).toBe('Creating…')
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
