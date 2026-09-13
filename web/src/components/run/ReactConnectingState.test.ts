// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { nextTick } from 'vue'
import { mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'
import enCommon from '@/locales/en/common.json'
import enPages from '@/locales/en/pages.json'
import ReactConnectingState from './ReactConnectingState.vue'

function mountSidebar(locale: 'zh-CN' | 'en') {
  const i18n = locale === 'zh-CN'
    ? createI18n({
        legacy: false,
        locale,
        messages: { 'zh-CN': { ...common, ...pages } },
      })
    : createI18n({
        legacy: false,
        locale,
        messages: { en: { ...enCommon, ...enPages } },
      })
  return mount(ReactConnectingState, {
    global: {
      plugins: [i18n],
      stubs: { Icon: true },
    },
  })
}

function mountStage(locale: 'zh-CN' | 'en' = 'zh-CN') {
  const i18n = locale === 'zh-CN'
    ? createI18n({
        legacy: false,
        locale,
        messages: { 'zh-CN': { ...common, ...pages } },
      })
    : createI18n({
        legacy: false,
        locale,
        messages: { en: { ...enCommon, ...enPages } },
      })
  return mount(ReactConnectingState, {
    props: { mode: 'stage' },
    global: { plugins: [i18n], stubs: { Icon: true } },
  })
}

describe('ReactConnectingState', () => {
  afterEach(() => {
    vi.useRealTimers()
    vi.restoreAllMocks()
  })

  it('shows the Chinese staged loader with all actions disabled', async () => {
    vi.useFakeTimers()
    const wrapper = mountSidebar('zh-CN')
    expect(wrapper.get('[data-testid="react-connecting-pill"]').text()).toContain('连接中')
    expect(wrapper.get('icon-stub[name="spinner"]').exists()).toBe(true)
    const titles = [
      '正在准备工作环境…',
      '正在启动 Agent…',
      '正在连接 Agent…',
      '正在整理第一轮问题…',
    ]
    expect(wrapper.text()).toContain(titles[0])
    for (const title of titles.slice(1)) {
      vi.advanceTimersByTime(2600)
      await nextTick()
      expect(wrapper.text()).toContain(title)
    }
    vi.advanceTimersByTime(2600)
    await nextTick()
    expect(wrapper.text()).toContain(titles.at(-1))
    expect(wrapper.get('[data-testid="clarify-boot-progress"]').findAll('span')).toHaveLength(5)
    const input = wrapper.get('[data-testid="react-connecting-input"]')
    expect(input.attributes('placeholder')).toBe('连接中，暂不可输入')
    expect((input.element as HTMLTextAreaElement).disabled).toBe(true)
    expect((wrapper.get('[data-testid="react-connecting-send"]').element as HTMLButtonElement).disabled).toBe(true)
    expect((wrapper.get('[data-testid="react-connecting-confirm"]').element as HTMLButtonElement).disabled).toBe(true)
  })

  it('shows the English connecting copy', () => {
    const wrapper = mountSidebar('en')
    expect(wrapper.get('[data-testid="react-connecting-pill"]').text()).toContain('Connecting')
    expect(wrapper.get('[data-testid="react-connecting-input"]').attributes('placeholder')).toContain('Connecting')
  })

  it('keeps the first step static when reduced motion is preferred', async () => {
    vi.useFakeTimers()
    vi.spyOn(window, 'matchMedia').mockReturnValue({
      matches: true,
      media: '(prefers-reduced-motion: reduce)',
      onchange: null,
      addListener: vi.fn(),
      removeListener: vi.fn(),
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      dispatchEvent: vi.fn(),
    })
    const wrapper = mountSidebar('zh-CN')
    vi.advanceTimersByTime(10_400)
    await nextTick()
    expect(wrapper.text()).toContain('正在准备工作环境…')
    expect(wrapper.text()).not.toContain('正在启动 Agent…')
  })

  it('pins pull-image copy when sandboxPhase is pulling (g3.1)', async () => {
    vi.useFakeTimers()
    const wrapper = mount(ReactConnectingState, {
      props: { sandboxPhase: 'pulling' },
      global: {
        plugins: [createI18n({
          legacy: false,
          locale: 'zh-CN',
          messages: { 'zh-CN': { ...common, ...pages } },
        })],
        stubs: { Icon: true },
      },
    })
    expect(wrapper.get('[data-testid="react-connecting-pill"]').text()).toContain('正在拉取镜像')
    expect(wrapper.text()).toContain('正在拉取镜像…')
    vi.advanceTimersByTime(10_400)
    await nextTick()
    expect(wrapper.text()).toContain('正在拉取镜像…')
    expect(wrapper.text()).not.toContain('正在启动 Agent…')
  })

  it('stage chrome defaults to clickable preview tab with HardLoadLayer (g1.1 g1.2 g1.3)', () => {
    const wrapper = mountStage('zh-CN')
    expect(wrapper.get('[data-testid="react-connecting-stage"]').attributes('aria-busy')).toBe('true')
    const pipeline = wrapper.get('[data-testid="react-connecting-tab-pipeline"]')
    const preview = wrapper.get('[data-testid="react-connecting-tab-preview"]')
    expect(pipeline.element.tagName).toBe('BUTTON')
    expect(preview.element.tagName).toBe('BUTTON')
    expect(pipeline.attributes('role')).toBe('tab')
    expect(preview.attributes('role')).toBe('tab')
    expect(pipeline.attributes('aria-selected')).toBe('false')
    expect(preview.attributes('aria-selected')).toBe('true')
    expect(preview.text()).toContain('产物预览')
    expect(wrapper.get('[data-testid="hard-load-layer"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="hard-load-stage"]').text()).toContain('加载产物内容')
    expect(wrapper.text()).not.toContain('正在连接 Agent…')
    expect(wrapper.find('[data-testid="hard-load-retry"]').exists()).toBe(false)
  })

  it('stage chrome English defaults to Artifact preview HardLoadLayer (g1.3)', () => {
    const wrapper = mountStage('en')
    expect(wrapper.get('[data-testid="react-connecting-tab-preview"]').attributes('aria-selected')).toBe('true')
    expect(wrapper.get('[data-testid="react-connecting-tab-preview"]').text()).toContain('Artifact preview')
    expect(wrapper.get('[data-testid="hard-load-stage"]').text()).toMatch(/Loading artifact/i)
  })

  it('pipeline tab keeps content skeleton without HardLoadLayer (g1.1 f2)', async () => {
    const wrapper = mountStage('zh-CN')
    await wrapper.get('[data-testid="react-connecting-tab-pipeline"]').trigger('click')
    await nextTick()
    expect(wrapper.get('[data-testid="react-connecting-tab-pipeline"]').attributes('aria-selected')).toBe('true')
    expect(wrapper.get('[data-testid="react-connecting-tab-preview"]').attributes('aria-selected')).toBe('false')
    expect(wrapper.find('[data-testid="hard-load-layer"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="react-connecting-pipeline-skeleton"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="react-connecting-pipeline-skeleton"] .animate-pulse').exists()).toBe(true)
  })

  it('keeps pipeline selection until remount (g1 f3)', async () => {
    const wrapper = mountStage('zh-CN')
    await wrapper.get('[data-testid="react-connecting-tab-pipeline"]').trigger('click')
    await nextTick()
    expect(wrapper.get('[data-testid="react-connecting-tab-pipeline"]').attributes('aria-selected')).toBe('true')
    await wrapper.vm.$forceUpdate()
    await nextTick()
    expect(wrapper.get('[data-testid="react-connecting-tab-pipeline"]').attributes('aria-selected')).toBe('true')
  })

  it('does not show stuck or retry within 10s on connecting HardLoadLayer (g1.2 f5)', async () => {
    vi.useFakeTimers()
    const wrapper = mountStage('zh-CN')
    expect(wrapper.find('[data-testid="hard-load-layer"]').exists()).toBe(true)
    vi.advanceTimersByTime(10_000)
    await nextTick()
    expect(wrapper.find('[data-testid="hard-load-stuck"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="hard-load-retry"]').exists()).toBe(false)
  })

  it('sidebar connecting stays ClarifyBootLoader without preview tabs (f6)', () => {
    const wrapper = mountSidebar('zh-CN')
    expect(wrapper.find('[data-testid="react-connecting-sidebar"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="react-connecting-tab-preview"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="hard-load-layer"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="clarify-boot-progress"]').exists()).toBe(true)
  })
})
