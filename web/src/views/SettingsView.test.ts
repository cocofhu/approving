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
  getSettings: vi.fn(),
  updateSettings: vi.fn(),
  listSandboxes: vi.fn(),
  dashboard: vi.fn(),
}))

vi.mock('@/lib/api', async () => {
  const actual = await vi.importActual<typeof import('@/lib/api')>('@/lib/api')
  return {
    ...actual,
    api: {
      ...actual.api,
      getSettings: apiMocks.getSettings,
      updateSettings: apiMocks.updateSettings,
      listSandboxes: apiMocks.listSandboxes,
      dashboard: apiMocks.dashboard,
    },
  }
})

vi.mock('@/lib/useAuth', async () => {
  const { ref } = await import('vue')
  return {
    useAuth: () => ({ user: ref({ username: 'admin', isAdmin: true }) }),
  }
})

import SettingsView from './SettingsView.vue'

const src = readFileSync(join(dirname(fileURLToPath(import.meta.url)), 'SettingsView.vue'), 'utf8')

const SETTINGS = {
  items: [
    { key: 'max_concurrent_runs', value: 4, min: 1, source: 'ui', locked: false },
    { key: 'run_sandbox_ttl_minutes', value: 30, min: 1, source: 'ui', locked: false },
    { key: 'test_sandbox_ttl_minutes', value: 15, min: 1, source: 'ui', locked: false },
    { key: 'max_test_sandboxes', value: 3, min: 1, source: 'ui', locked: false },
  ],
}

function mountSettings() {
  const i18n = createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...common, ...pages } },
  })
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/settings', component: SettingsView }],
  })
  void router.push('/settings')
  return mount(SettingsView, {
    global: {
      plugins: [i18n, router],
      stubs: {
        Icon: true,
        AppButton: { template: '<button type="button" v-bind="$attrs"><slot /></button>' },
      },
    },
  })
}

describe('SettingsView loading source lock', () => {
  it('first load uses grouped form skeleton, not EmptyState loadingTitle', () => {
    expect(src).toMatch(/data-testid="settings-form-skeleton"/)
    expect(src).not.toMatch(/EmptyState/)
    expect(src).not.toMatch(/pages\.settings\.loadingTitle/)
    expect(src).toMatch(/admin-list-thin-bar bg-accent/)
    expect(src).toMatch(/opacity-\[0\.55\]/)
  })
})

describe('SettingsView first skeleton vs reset keep form', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    apiMocks.listSandboxes.mockResolvedValue([])
    apiMocks.dashboard.mockResolvedValue({ running: 0 })
  })

  it('shows form skeleton before settings arrive', async () => {
    let release!: (v: unknown) => void
    apiMocks.getSettings.mockReturnValue(new Promise((resolve) => { release = resolve }))
    const w = mountSettings()
    await flushPromises()
    expect(w.find('[data-testid="settings-form-skeleton"]').exists()).toBe(true)
    release!(SETTINGS)
    await flushPromises()
    expect(w.find('[data-testid="settings-form-skeleton"]').exists()).toBe(false)
    expect(w.text()).toContain('max_concurrent_runs')
    w.unmount()
  })

  it('reset keeps form fields on screen with thin progress', async () => {
    apiMocks.getSettings.mockResolvedValue(SETTINGS)
    const w = mountSettings()
    await flushPromises()
    expect(w.find('input').exists()).toBe(true)
    let release!: (v: unknown) => void
    apiMocks.getSettings.mockReturnValue(new Promise((resolve) => { release = resolve }))
    const resetBtn = w.findAll('button').find((b) => b.text().includes('重置'))
    expect(resetBtn).toBeTruthy()
    await resetBtn!.trigger('click')
    await flushPromises()
    expect(w.find('[data-testid="settings-thin-progress"]').exists()).toBe(true)
    expect(w.find('input').exists()).toBe(true)
    expect(w.find('[data-testid="settings-form-skeleton"]').exists()).toBe(false)
    release!(SETTINGS)
    await flushPromises()
    w.unmount()
  })
})
