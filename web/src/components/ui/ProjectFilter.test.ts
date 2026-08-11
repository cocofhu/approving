// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import common from '@/locales/zh-CN/common.json'

const apiMocks = vi.hoisted(() => ({
  listProjects: vi.fn(),
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

import ProjectFilter from './ProjectFilter.vue'

beforeEach(() => {
  vi.clearAllMocks()
  apiMocks.listProjects.mockResolvedValue([{ id: 'p1', name: '项目 A' }])
})

describe('ProjectFilter', () => {
  it('loads projects on mount', async () => {
    const i18n = createI18n({
      legacy: false,
      locale: 'zh-CN',
      messages: { 'zh-CN': { ...common } },
    })
    const wrapper = mount(ProjectFilter, {
      props: { modelValue: '' },
      global: { plugins: [i18n], stubs: { Icon: true } },
    })
    await flushPromises()
    expect(apiMocks.listProjects).toHaveBeenCalled()
    wrapper.unmount()
  })

  it('shows inline error instead of empty list when load fails', async () => {
    apiMocks.listProjects.mockRejectedValueOnce(new Error('network down'))
    const i18n = createI18n({
      legacy: false,
      locale: 'zh-CN',
      messages: { 'zh-CN': { ...common } },
    })
    const wrapper = mount(ProjectFilter, {
      props: { modelValue: '', open: true },
      global: { plugins: [i18n], stubs: { Icon: true } },
    })
    await flushPromises()
    expect(wrapper.find('[data-testid="app-inline-error"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('项目列表加载失败')
    expect(wrapper.text()).not.toContain('无匹配项目')
    wrapper.unmount()
  })

  it('shows EmptyState without retry when project list is empty', async () => {
    apiMocks.listProjects.mockResolvedValueOnce([])
    const i18n = createI18n({
      legacy: false,
      locale: 'zh-CN',
      messages: { 'zh-CN': { ...common } },
    })
    const wrapper = mount(ProjectFilter, {
      props: { modelValue: '', open: true },
      global: { plugins: [i18n], stubs: { Icon: true } },
    })
    await flushPromises()
    expect(wrapper.find('[data-testid="app-inline-error"]').exists()).toBe(false)
    expect(wrapper.text()).toContain('暂无项目')
    wrapper.unmount()
  })

  it('emits update:open when controlled open toggles', async () => {
    const i18n = createI18n({
      legacy: false,
      locale: 'zh-CN',
      messages: { 'zh-CN': { ...common } },
    })
    const wrapper = mount(ProjectFilter, {
      props: { modelValue: '', open: false },
      global: { plugins: [i18n], stubs: { Icon: true } },
    })
    await flushPromises()
    await wrapper.find('button').trigger('click')
    expect(wrapper.emitted('update:open')?.[0]?.[0]).toBe(true)
    wrapper.unmount()
  })
})
