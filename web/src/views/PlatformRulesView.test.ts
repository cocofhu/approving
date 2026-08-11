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

const apiMocks = vi.hoisted(() => ({
  listPlatformRules: vi.fn(),
  getPlatformRule: vi.fn(),
  savePlatformRule: vi.fn(),
  resetPlatformRule: vi.fn(),
}))

vi.mock('@/lib/api/api', async () => {
  const actual = await vi.importActual<typeof import('@/lib/api/api')>('@/lib/api/api')
  return {
    ...actual,
    api: {
      ...actual.api,
      listPlatformRules: apiMocks.listPlatformRules,
      getPlatformRule: apiMocks.getPlatformRule,
      savePlatformRule: apiMocks.savePlatformRule,
      resetPlatformRule: apiMocks.resetPlatformRule,
    },
  }
})

vi.mock('@/lib/composables/useAuth', async () => {
  const { ref } = await import('vue')
  return {
    useAuth: () => ({ user: ref({ username: 'admin', isAdmin: true }) }),
  }
})

import PlatformRulesView from './PlatformRulesView.vue'

const src = readFileSync(join(dirname(fileURLToPath(import.meta.url)), 'PlatformRulesView.vue'), 'utf8')

const FILE_A = { file: 'a.md', source: 'global' as const }
const FILE_B = { file: 'b.md', source: 'global' as const }

function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>((r) => { resolve = r })
  return { promise, resolve }
}

function mountRules() {
  const i18n = createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...common, ...pages } },
  })
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/settings/platform-rules', component: PlatformRulesView },
      { path: '/settings', component: { template: '<div />' } },
    ],
  })
  void router.push('/settings/platform-rules')
  return mount(PlatformRulesView, {
    global: {
      plugins: [i18n, router],
      stubs: {
        Icon: true,
        AppButton: { template: '<button type="button" v-bind="$attrs"><slot /></button>' },
        MarkdownSplitEditor: {
          props: ['modelValue'],
          template: '<div data-testid="platform-rules-editor">{{ modelValue }}</div>',
        },
      },
    },
  })
}

describe('PlatformRulesView loading source lock', () => {
  it('uses grouped form skeleton, Demo thin progress, and shared requestSeq', () => {
    expect(src).toMatch(/data-testid="platform-rules-skeleton"/)
    expect(src).toMatch(/admin-list-thin-bar bg-accent/)
    expect(src).toMatch(/opacity-\[0\.55\]/)
    expect(src).toMatch(/createListRequestSeq/)
    expect(src).toMatch(/rulesSeq\.beginListRequest/)
    expect(src).toMatch(/rulesSeq\.isCurrentSeq\(localSeq\)/)
    expect(src).not.toMatch(/fileSeq/)
    expect(src).not.toMatch(/EmptyState/)
    expect(src).not.toMatch(/#7B61FF/)
  })

  it('writes editor content only after isCurrentSeq', () => {
    expect(src).toMatch(/async function fetchRuleFile/)
    expect(src).toMatch(/const data = await fetchRuleFile\(file\)\s+if \(!rulesSeq\.isCurrentSeq\(localSeq\)\) return\s+content\.value = data\.content/)
  })
})

describe('PlatformRulesView selectFile race + four-state', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('shows grouped skeleton before rules arrive', async () => {
    let releaseList!: (v: unknown) => void
    apiMocks.listPlatformRules.mockReturnValue(new Promise((resolve) => { releaseList = resolve }))
    apiMocks.getPlatformRule.mockResolvedValue({ ...FILE_A, content: 'CONTENT-A' })
    const w = mountRules()
    await flushPromises()
    expect(w.find('[data-testid="platform-rules-skeleton"]').exists()).toBe(true)
    releaseList!({ items: [FILE_A] })
    await flushPromises()
    expect(w.find('[data-testid="platform-rules-skeleton"]').exists()).toBe(false)
    expect(w.find('[data-testid="platform-rules-editor"]').text()).toContain('CONTENT-A')
    w.unmount()
  })

  it('fast click fileA then fileB keeps last file; stale A does not overwrite editor', async () => {
    apiMocks.listPlatformRules.mockResolvedValue({ items: [FILE_A, FILE_B] })
    apiMocks.getPlatformRule.mockResolvedValueOnce({ ...FILE_A, content: 'CONTENT-A' })
    const w = mountRules()
    await flushPromises()
    expect(w.find('[data-testid="platform-rules-editor"]').text()).toContain('CONTENT-A')

    const pendingA = deferred<{ file: string; content: string; source: 'global' }>()
    const pendingB = deferred<{ file: string; content: string; source: 'global' }>()
    apiMocks.getPlatformRule.mockImplementation((file: string) => {
      if (file === 'a.md') return pendingA.promise
      if (file === 'b.md') return pendingB.promise
      return Promise.resolve({ file, content: '', source: 'global' as const })
    })

    const btnA = w.findAll('button').find((b) => b.text().includes('a.md'))
    const btnB = w.findAll('button').find((b) => b.text().includes('b.md'))
    expect(btnA).toBeTruthy()
    expect(btnB).toBeTruthy()
    await btnA!.trigger('click')
    await btnB!.trigger('click')
    await flushPromises()

    pendingA.resolve({ ...FILE_A, content: 'STALE-A' })
    await flushPromises()
    expect(w.find('[data-testid="platform-rules-editor"]').text()).not.toContain('STALE-A')
    expect(w.find('[data-testid="platform-rules-editor"]').text()).toContain('CONTENT-A')

    pendingB.resolve({ ...FILE_B, content: 'CONTENT-B' })
    await flushPromises()
    expect(w.find('[data-testid="platform-rules-editor"]').text()).toContain('CONTENT-B')
    expect(w.find('[data-testid="platform-rules-editor"]').text()).not.toContain('STALE-A')
    w.unmount()
  })

  it('failure shows red card + retry, not empty editor', async () => {
    apiMocks.listPlatformRules.mockRejectedValue(Object.assign(new Error('down'), { status: 500 }))
    const w = mountRules()
    await flushPromises()
    expect(w.find('[data-testid="platform-rules-failed"]').exists()).toBe(true)
    expect(w.find('[data-testid="platform-rules-skeleton"]').exists()).toBe(false)
    expect(w.text()).toContain('加载失败')
    expect(w.text()).toContain('重试')
    w.unmount()
  })

  it('403 shows amber denied surface with retry', async () => {
    apiMocks.listPlatformRules.mockRejectedValue(Object.assign(new Error('denied'), { status: 403 }))
    const w = mountRules()
    await flushPromises()
    expect(w.find('[data-testid="platform-rules-denied"]').exists()).toBe(true)
    expect(w.find('[data-testid="platform-rules-failed"]').exists()).toBe(false)
    expect(w.text()).toContain('权限不足')
    expect(w.text()).toContain('重试')
    w.unmount()
  })
})
