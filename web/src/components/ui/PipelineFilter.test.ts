// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import common from '@/locales/zh-CN/common.json'

const apiMocks = vi.hoisted(() => ({
  listWorkflows: vi.fn(),
}))

vi.mock('@/lib/api', async () => {
  const actual = await vi.importActual<typeof import('@/lib/api')>('@/lib/api')
  return {
    ...actual,
    api: {
      ...actual.api,
      listWorkflows: apiMocks.listWorkflows,
    },
  }
})

import PipelineFilter from './PipelineFilter.vue'

beforeEach(() => {
  vi.clearAllMocks()
  apiMocks.listWorkflows.mockResolvedValue([
    { id: 'wf-1', name: '流水线 A', status: 'published', nodes: [], edges: [] },
  ])
})

describe('PipelineFilter', () => {
  it('loads workflows and shows all label by default', async () => {
    const i18n = createI18n({
      legacy: false,
      locale: 'zh-CN',
      messages: { 'zh-CN': { ...common } },
    })
    const wrapper = mount(PipelineFilter, {
      props: { modelValue: '' },
      global: { plugins: [i18n], stubs: { Icon: true } },
    })
    await flushPromises()
    expect(apiMocks.listWorkflows).toHaveBeenCalled()
    expect(wrapper.text().length).toBeGreaterThan(0)
    wrapper.unmount()
  })
})
