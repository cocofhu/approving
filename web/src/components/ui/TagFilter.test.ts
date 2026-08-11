// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import common from '@/locales/zh-CN/common.json'

const apiMocks = vi.hoisted(() => ({
  listProjectRunTags: vi.fn(),
}))

vi.mock('@/lib/api/api', async () => {
  const actual = await vi.importActual<typeof import('@/lib/api/api')>('@/lib/api/api')
  return {
    ...actual,
    api: {
      ...actual.api,
      listProjectRunTags: apiMocks.listProjectRunTags,
    },
  }
})

import TagFilter from './TagFilter.vue'

function mountFilter(
  props: { modelValue?: string[]; projectId?: string; open?: boolean } = {},
) {
  const i18n = createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...common } },
  })
  return mount(TagFilter, {
    props: {
      modelValue: props.modelValue ?? [],
      projectId: props.projectId ?? '',
      open: props.open,
    },
    global: { plugins: [i18n], stubs: { Icon: true } },
  })
}

beforeEach(() => {
  vi.clearAllMocks()
  apiMocks.listProjectRunTags.mockResolvedValue({ tags: ['prod', 'canary', 'hotfix'] })
})

describe('TagFilter', () => {
  it('shows label trigger without count when empty', () => {
    const wrapper = mountFilter()
    expect(wrapper.get('[data-testid="tag-filter-trigger"]').text()).toContain('标签')
    expect(wrapper.find('[data-testid="tag-filter-count"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('shows selected count on trigger', () => {
    const wrapper = mountFilter({ modelValue: ['prod', 'canary'] })
    expect(wrapper.get('[data-testid="tag-filter-count"]').text()).toBe('2')
    wrapper.unmount()
  })

  it('loads stock tags and toggles selection', async () => {
    const wrapper = mountFilter({ projectId: 'p1', open: true })
    await flushPromises()
    expect(apiMocks.listProjectRunTags).toHaveBeenCalledWith('p1')

    const suggestions = wrapper.get('[data-testid="tag-filter-suggestions"]').findAll('button')
    expect(suggestions.length).toBe(3)
    await suggestions[0]!.trigger('click')
    expect(wrapper.emitted('update:modelValue')?.[0]?.[0]).toEqual(['prod'])

    await wrapper.setProps({ modelValue: ['prod'] })
    await suggestions[0]!.trigger('click')
    const emitted = wrapper.emitted('update:modelValue') ?? []
    expect(emitted[emitted.length - 1]?.[0]).toEqual([])
    wrapper.unmount()
  })

  it('adds valid tag on Enter and shows selected chip', async () => {
    const wrapper = mountFilter({ projectId: 'p1', open: true, modelValue: [] })
    await flushPromises()
    const input = wrapper.get('[data-testid="tag-filter-input"]')
    await input.setValue('nightly')
    await input.trigger('keydown', { key: 'Enter' })
    expect(wrapper.emitted('update:modelValue')?.[0]?.[0]).toEqual(['nightly'])
    wrapper.unmount()
  })

  it('blocks illegal tag with red error and does not emit', async () => {
    const wrapper = mountFilter({ open: true })
    const input = wrapper.get('[data-testid="tag-filter-input"]')
    await input.setValue('owner:alice')
    await input.trigger('keydown', { key: 'Enter' })
    expect(wrapper.find('[data-testid="tag-filter-error"]').exists()).toBe(true)
    expect(wrapper.emitted('update:modelValue')).toBeFalsy()
    wrapper.unmount()
  })

  it('shows need-project empty state without calling API', async () => {
    const wrapper = mountFilter({ open: true, projectId: '' })
    await flushPromises()
    expect(apiMocks.listProjectRunTags).not.toHaveBeenCalled()
    expect(wrapper.get('[data-testid="tag-filter-empty"]').text()).toMatch(/选择项目/)
    wrapper.unmount()
  })

  it('shows no-stock empty state when project has no tags', async () => {
    apiMocks.listProjectRunTags.mockResolvedValue({ tags: [] })
    const wrapper = mountFilter({ open: true, projectId: 'p1' })
    await flushPromises()
    expect(wrapper.get('[data-testid="tag-filter-empty"]').text()).toMatch(/暂无存量/)
    wrapper.unmount()
  })

  it('removes selected chip on click', async () => {
    const wrapper = mountFilter({ open: true, modelValue: ['prod'] })
    await wrapper.get('[data-testid="tag-filter-selected"] button').trigger('click')
    expect(wrapper.emitted('update:modelValue')?.[0]?.[0]).toEqual([])
    wrapper.unmount()
  })

  it('emits update:open when controlled', async () => {
    const wrapper = mountFilter({ open: false })
    await wrapper.get('[data-testid="tag-filter-trigger"]').trigger('click')
    expect(wrapper.emitted('update:open')?.[0]?.[0]).toBe(true)
    wrapper.unmount()
  })

  it('filters suggestions by draft input', async () => {
    const wrapper = mountFilter({ projectId: 'p1', open: true })
    await flushPromises()
    await wrapper.get('[data-testid="tag-filter-input"]').setValue('can')
    const suggestions = wrapper.get('[data-testid="tag-filter-suggestions"]').findAll('button')
    expect(suggestions).toHaveLength(1)
    expect(suggestions[0]!.text()).toContain('canary')
    wrapper.unmount()
  })
})
