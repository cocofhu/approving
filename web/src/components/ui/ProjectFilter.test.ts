// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import common from '@/locales/zh-CN/common.json'

const apiMocks = vi.hoisted(() => ({
  listProjects: vi.fn(),
}))

vi.mock('@/lib/api', async () => {
  const actual = await vi.importActual<typeof import('@/lib/api')>('@/lib/api')
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
})
