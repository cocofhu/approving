// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'
import commonZh from '@/locales/zh-CN/common.json'
import commonEn from '@/locales/en/common.json'
import Pagination from './Pagination.vue'

const isMobile = ref(false)

vi.mock('@/lib/useBreakpoint', () => ({
  useBreakpoint: () => ({ isMobile }),
}))

function mountPager(
  props: Partial<{
    page: number
    pageSize: number
    total: number
    loading: boolean
    disabled: boolean
    pageSizeOptions: number[]
    summaryTestId: string
    pageSizeTestId: string
  }> = {},
  locale: 'zh-CN' | 'en' = 'zh-CN',
) {
  const i18n = createI18n({
    legacy: false,
    locale,
    messages: {
      'zh-CN': { ...commonZh },
      en: { ...commonEn },
    },
  })
  return mount(Pagination, {
    props: {
      page: 1,
      pageSize: 10,
      total: 50,
      ...props,
    },
    global: { plugins: [i18n] },
  })
}

describe('Pagination', () => {
  beforeEach(() => {
    isMobile.value = false
  })

  it('shows range summary with en-dash', () => {
    const wrapper = mountPager({ page: 2, total: 50 })
    expect(wrapper.text()).toContain('11–20 / 50')
    wrapper.unmount()
  })

  it('shows empty summary when total is 0 (zh/en)', () => {
    const zh = mountPager({ total: 0 }, 'zh-CN')
    expect(zh.text()).toContain('共 0')
    expect(zh.text()).not.toMatch(/0–0/)
    zh.unmount()

    const en = mountPager({ total: 0 }, 'en')
    expect(en.text()).toContain('0 items')
    en.unmount()
  })

  it('highlights active page with aria-current', () => {
    const wrapper = mountPager({ page: 3, total: 50 })
    const active = wrapper.find('.page-num.active')
    expect(active.exists()).toBe(true)
    expect(active.text()).toBe('3')
    expect(active.attributes('aria-current')).toBe('page')
    wrapper.unmount()
  })

  it('emits page update when next is clicked', async () => {
    const wrapper = mountPager({ page: 1, total: 50 })
    const next = wrapper.findAll('button.pg-btn')[1]
    expect(next).toBeTruthy()
    await next!.trigger('click')
    expect(wrapper.emitted('update:page')?.[0]).toEqual([2])
    wrapper.unmount()
  })

  it('disables prev on first page and next on last page', () => {
    const first = mountPager({ page: 1, total: 50 })
    const firstBtns = first.findAll('button.pg-btn')
    expect((firstBtns[0]!.element as HTMLButtonElement).disabled).toBe(true)
    expect((firstBtns[1]!.element as HTMLButtonElement).disabled).toBe(false)
    first.unmount()

    const last = mountPager({ page: 5, total: 50 })
    const lastBtns = last.findAll('button.pg-btn')
    expect((lastBtns[0]!.element as HTMLButtonElement).disabled).toBe(false)
    expect((lastBtns[1]!.element as HTMLButtonElement).disabled).toBe(true)
    last.unmount()
  })

  it('disables nav, page nums and pageSize when loading', () => {
    const wrapper = mountPager({
      page: 2,
      total: 50,
      loading: true,
      pageSizeOptions: [5, 10, 20],
    })
    for (const btn of wrapper.findAll('button.pg-btn, button.page-num')) {
      expect((btn.element as HTMLButtonElement).disabled).toBe(true)
    }
    expect((wrapper.find('select').element as HTMLSelectElement).disabled).toBe(true)
    wrapper.unmount()
  })

  it('disables all controls when disabled', () => {
    const wrapper = mountPager({
      page: 2,
      total: 50,
      disabled: true,
      pageSizeOptions: [5, 10, 20],
    })
    for (const btn of wrapper.findAll('button')) {
      expect((btn.element as HTMLButtonElement).disabled).toBe(true)
    }
    expect((wrapper.find('select').element as HTMLSelectElement).disabled).toBe(true)
    wrapper.unmount()
  })

  it('renders pageSize options and emits update:pageSize', async () => {
    const wrapper = mountPager({
      page: 2,
      pageSize: 10,
      total: 50,
      pageSizeOptions: [5, 10, 20],
      pageSizeTestId: 'project-audit-page-size',
    })
    const size = wrapper.find('[data-testid="project-audit-page-size"]')
    expect(size.exists()).toBe(true)
    const select = wrapper.find('select')
    const options = select.findAll('option').map((o) => o.element.value)
    expect(options).toEqual(['5', '10', '20'])
    await select.setValue('20')
    expect(wrapper.emitted('update:pageSize')?.[0]).toEqual([20])
    wrapper.unmount()
  })

  it('hides pageSize selector when options are omitted', () => {
    const wrapper = mountPager({ total: 50 })
    expect(wrapper.find('select').exists()).toBe(false)
    expect(wrapper.find('.pg-size').exists()).toBe(false)
    wrapper.unmount()
  })

  it('hides page numbers on mobile breakpoint', () => {
    isMobile.value = true
    const wrapper = mountPager({ page: 2, total: 50 })
    expect(wrapper.find('.page-nums').exists()).toBe(false)
    expect(wrapper.findAll('button.pg-btn').length).toBe(2)
    expect(wrapper.classes()).toContain('is-mobile')
    wrapper.unmount()
  })

  it('forwards summary test id', () => {
    const wrapper = mountPager({
      total: 50,
      summaryTestId: 'project-audit-pager-info',
    })
    expect(wrapper.find('[data-testid="project-audit-pager-info"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('keeps prev/next as icon+label elevated buttons (not bare text links)', () => {
    const wrapper = mountPager({ page: 2, total: 50 })
    const btns = wrapper.findAll('button.pg-btn')
    const prev = btns[0]!
    const next = btns[1]!
    expect(prev.find('svg').exists()).toBe(true)
    expect(next.find('svg').exists()).toBe(true)
    expect(prev.text()).toMatch(/上一页/)
    expect(next.text()).toMatch(/下一页/)
    wrapper.unmount()
  })
})
