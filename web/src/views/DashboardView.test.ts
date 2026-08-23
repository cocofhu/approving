// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'
import type { Workflow } from '@/lib/shared/types'

const mocks = vi.hoisted(() => ({
  push: vi.fn(),
  listWorkflows: vi.fn(),
  startRun: vi.fn(),
  getRun: vi.fn(),
  reactReply: vi.fn(),
  readStoredProjectId: vi.fn(() => 'proj-1'),
}))

vi.mock('vue-router', () => ({
  useRouter: () => ({ push: mocks.push }),
}))

vi.mock('@/lib/api/api', async () => {
  const actual = await vi.importActual<typeof import('@/lib/api/api')>('@/lib/api/api')
  return {
    ...actual,
    api: {
      ...actual.api,
      listWorkflows: mocks.listWorkflows,
      startRun: mocks.startRun,
      getRun: mocks.getRun,
      reactReply: mocks.reactReply,
    },
  }
})

vi.mock('@/lib/composables/useProjectContext', () => ({
  readStoredProjectId: () => mocks.readStoredProjectId(),
}))

vi.mock('@/lib/composables/useToast', () => ({
  useToast: () => ({ warn: vi.fn(), error: vi.fn(), success: vi.fn() }),
}))

import DashboardView from './DashboardView.vue'

const HomePreviewAppModalStub = {
  props: ['open', 'title', 'width'],
  emits: ['close'],
  template: `
    <div v-if="open" data-testid="home-image-preview-modal">
      <div data-testid="home-image-preview-title">{{ title }}</div>
      <button type="button" data-testid="home-image-preview-close" @click="$emit('close')">×</button>
      <button type="button" data-testid="home-image-preview-backdrop" @click="$emit('close')">backdrop</button>
      <slot />
    </div>
  `,
}

const approveWf: Workflow = {
  id: 'wf-ap',
  name: '自我迭代PRO',
  description: '开发前澄清 + 计划',
  status: 'published',
  version: 1,
  updatedAt: '',
  needsRepo: false,
  projectId: 'proj-1',
  nodes: [
    { id: 'in', type: 'input', label: '开始', position: { x: 0, y: 0 }, config: {} },
    { id: 'ap', type: 'approve', label: '澄清', position: { x: 0, y: 0 }, config: {} },
  ],
  edges: [{ id: 'e1', source: 'in', target: 'ap' }],
}

function mountDashboard() {
  const i18n = createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...common, ...pages } },
  })
  return mount(DashboardView, {
    global: {
      plugins: [i18n],
      stubs: { Icon: true, RunLaunchModal: true, AppModal: HomePreviewAppModalStub },
    },
  })
}

function stubReducedMotion(matches: boolean) {
  vi.stubGlobal(
    'matchMedia',
    vi.fn((query: string) => ({
      matches: query.includes('prefers-reduced-motion') ? matches : false,
      media: query,
      onchange: null,
      addListener: vi.fn(),
      removeListener: vi.fn(),
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      dispatchEvent: vi.fn(),
    })),
  )
}

describe('DashboardView home composer', () => {
  beforeEach(() => {
    mocks.push.mockReset()
    mocks.listWorkflows.mockReset()
    mocks.startRun.mockReset()
    mocks.getRun.mockReset()
    mocks.reactReply.mockReset()
    mocks.readStoredProjectId.mockReturnValue('proj-1')
    mocks.listWorkflows.mockResolvedValue([approveWf])
    mocks.startRun.mockResolvedValue({ id: 'run-9', status: 'queued' })
    mocks.getRun.mockResolvedValue({
      id: 'run-9',
      status: 'waiting_human',
      nodes: [{ id: 'ap', type: 'approve', label: '', position: { x: 0, y: 0 }, config: {} }],
      nodeRuns: { ap: { nodeId: 'ap', status: 'waiting_human' } },
    })
    mocks.reactReply.mockResolvedValue({ status: 'ok' })
    stubReducedMotion(false)
    vi.useFakeTimers()
  })

  afterEach(() => {
    vi.useRealTimers()
    vi.unstubAllGlobals()
  })

  it('renders composer and approve-first cards without a project gate', async () => {
    mocks.readStoredProjectId.mockReturnValue('')
    const wrapper = mountDashboard()
    await flushPromises()
    expect(wrapper.get('[data-testid="home-title"]').text()).toContain('从一句话开始一次开发前澄清')
    expect(wrapper.find('[data-testid="home-composer"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="home-no-project"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="home-pipeline-card-wf-ap"]').text()).toContain('自我迭代PRO')
    expect(mocks.listWorkflows).toHaveBeenCalledWith(expect.objectContaining({ signal: expect.any(AbortSignal) }))
    const call = mocks.listWorkflows.mock.calls[0]?.[0] || {}
    expect(call).not.toHaveProperty('projectId')
    wrapper.unmount()
  })

  // plan g1.1 — no full-bleed purple stage layer; particle mesh bg instead
  it('does not render full-screen purple stage atmosphere', async () => {
    const wrapper = mountDashboard()
    await flushPromises()
    expect(wrapper.find('[data-testid="home-stage-bg"]').exists()).toBe(false)
    expect(wrapper.find('.home-stage__wash').exists()).toBe(false)
    expect(wrapper.find('.home-stage__glow').exists()).toBe(false)
    wrapper.unmount()
  })

  it('renders particle mesh background layer behind content', async () => {
    const wrapper = mountDashboard()
    await flushPromises()
    const bg = wrapper.find('[data-testid="home-particle-mesh-bg"]')
    expect(bg.exists()).toBe(true)
    expect(bg.classes()).toContain('home-particle-mesh')
    expect(wrapper.find('[data-testid="home-composer"]').exists()).toBe(true)
    wrapper.unmount()
  })

  // plan g2 — Approving mono brand + Chinese hint；无筛选说明句
  it('renders monospace Approving brand and Chinese hint', async () => {
    const wrapper = mountDashboard()
    await flushPromises()
    expect(wrapper.get('[data-testid="home-brand"]').classes()).toContain('home-brand')
    expect(wrapper.get('[data-testid="home-title"]').text()).toBe('从一句话开始一次开发前澄清')
    expect(wrapper.find('[data-testid="home-subtitle"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="home-filter-hint"]').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('仅显示开始后是 Approve 的已发布流水线')
    wrapper.unmount()
  })

  // plan g1.4 — pipeline cards are square (no global .card / rounded-lg)
  it('renders right-angle pipeline cards', async () => {
    const wrapper = mountDashboard()
    await flushPromises()
    const card = wrapper.get('[data-testid="home-pipeline-card-wf-ap"]')
    expect(card.classes()).toContain('home-shell__card')
    expect(card.classes()).not.toContain('card')
    expect(card.classes()).not.toContain('rounded-lg')
    expect(card.classes()).toContain('border')
    wrapper.unmount()
  })

  // plan g3 — one-shot typewriter then opacity-hide caret (keep layout box)
  it('types Approving once then settles without looping', async () => {
    const wrapper = mountDashboard()
    await flushPromises()
    expect(wrapper.get('[data-testid="home-brand-text"]').text()).toBe('')
    await vi.advanceTimersByTimeAsync(220 + 78 * 9 + 50)
    expect(wrapper.get('[data-testid="home-brand-text"]').text()).toBe('Approving')
    const caret = wrapper.get('[data-testid="home-brand-cursor"]')
    expect(caret.classes()).not.toContain('home-brand__cursor--gone')
    await vi.advanceTimersByTimeAsync(850 * 3 + 50)
    expect(wrapper.get('[data-testid="home-brand-text"]').text()).toBe('Approving')
    expect(wrapper.get('[data-testid="home-brand-cursor"]').classes()).toContain('home-brand__cursor--gone')
    expect(wrapper.get('[data-testid="home-brand-cursor"]').classes()).not.toContain('home-brand__cursor--blink')
    await vi.advanceTimersByTimeAsync(5000)
    expect(wrapper.get('[data-testid="home-brand-text"]').text()).toBe('Approving')
    expect(wrapper.get('[data-testid="home-brand-cursor"]').classes()).toContain('home-brand__cursor--gone')
    wrapper.unmount()
  })

  // plan g3 — reduced-motion shows static brand; caret stays in layout but gone
  it('shows full Approving immediately under reduced-motion', async () => {
    stubReducedMotion(true)
    const wrapper = mountDashboard()
    await flushPromises()
    expect(wrapper.get('[data-testid="home-brand-text"]').text()).toBe('Approving')
    expect(wrapper.get('[data-testid="home-brand-cursor"]').classes()).toContain('home-brand__cursor--gone')
    wrapper.unmount()
  })

  // plan g1.2 — placeholder typewriter when idle/empty
  it('shows placeholder typewriter when empty and unfocused', async () => {
    const wrapper = mountDashboard()
    await flushPromises()
    expect(wrapper.find('[data-testid="home-composer-placeholder"]').exists()).toBe(true)
    await vi.advanceTimersByTimeAsync(80 + 70 * 9 + 50)
    expect(wrapper.get('[data-testid="home-composer-placeholder"]').text()).toContain('快速开启你的迭代')
    await wrapper.get('[data-testid="home-composer-input"]').trigger('focus')
    await flushPromises()
    expect(wrapper.find('[data-testid="home-composer-placeholder"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('configures multiple dashboard placeholder lines in i18n', () => {
    const lines = pages.pages.dashboard.placeholders as string[]
    expect(lines.length).toBe(5)
    expect(lines[0]).toBe('快速开启你的迭代')
    expect(lines[1]).toContain('说一个功能')
  })

  it('cycles placeholder typewriter to the next configured line', async () => {
    const lines = pages.pages.dashboard.placeholders as string[]
    const wrapper = mountDashboard()
    await flushPromises()
    const firstLen = lines[0].length
    const advanceMs = 80 + 70 * firstLen + 1800 + 32 * firstLen + 400 + 70 * 5
    await vi.advanceTimersByTimeAsync(advanceMs)
    expect(wrapper.get('[data-testid="home-composer-placeholder"]').text()).toContain(lines[1].slice(0, 4))
    wrapper.unmount()
  })

  it('shows static first placeholder under reduced-motion', async () => {
    stubReducedMotion(true)
    const wrapper = mountDashboard()
    await flushPromises()
    expect(wrapper.get('[data-testid="home-composer-placeholder"]').text()).toContain('快速开启你的迭代')
    expect(wrapper.get('label.sr-only').text()).toContain('快速开启你的迭代')
    wrapper.unmount()
  })

  // plan g2.1 — no project gate; still loads cross-project pipelines
  it('loads pipelines without a stored project and does not show project empty state', async () => {
    mocks.readStoredProjectId.mockReturnValue('')
    const wrapper = mountDashboard()
    await flushPromises()
    expect(wrapper.find('[data-testid="home-no-project"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="home-pipeline-cards"]').exists()).toBe(true)
    expect(mocks.listWorkflows).toHaveBeenCalled()
    wrapper.unmount()
  })

  // plan g2.2 — dropdown and card selection stay in sync
  it('keeps pipeline select and card selection in sync', async () => {
    const second: Workflow = {
      ...approveWf,
      id: 'wf-lite',
      name: '快速澄清 Lite',
      description: '轻量 Approve 入口',
    }
    mocks.listWorkflows.mockResolvedValue([approveWf, second])
    const wrapper = mountDashboard()
    await flushPromises()
    const select = wrapper.get('[data-testid="home-pipeline-select"]')
    expect((select.element as HTMLSelectElement).value).toBe('wf-ap')
    expect(wrapper.get('[data-testid="home-pipeline-card-wf-ap"]').classes()).toContain(
      'home-shell__card--selected',
    )
    await wrapper.get('[data-testid="home-pipeline-card-wf-lite"]').trigger('click')
    await flushPromises()
    expect((select.element as HTMLSelectElement).value).toBe('wf-lite')
    expect(wrapper.get('[data-testid="home-pipeline-card-wf-lite"]').classes()).toContain(
      'home-shell__card--selected',
    )
    await select.setValue('wf-ap')
    await flushPromises()
    expect(wrapper.get('[data-testid="home-pipeline-card-wf-ap"]').classes()).toContain(
      'home-shell__card--selected',
    )
    wrapper.unmount()
  })

  it('shows pipeline empty state when none are approve-first', async () => {
    mocks.listWorkflows.mockResolvedValue([
      {
        ...approveWf,
        id: 'wf-react',
        name: '实现',
        nodes: [
          { id: 'in', type: 'input', label: '开始', position: { x: 0, y: 0 }, config: {} },
          { id: 'r', type: 'react', label: '实现', position: { x: 0, y: 0 }, config: {} },
        ],
        edges: [{ id: 'e1', source: 'in', target: 'r' }],
      },
    ])
    const wrapper = mountDashboard()
    await flushPromises()
    expect(wrapper.find('[data-testid="home-pipelines-empty"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="home-pipelines-empty"]').text()).not.toContain('选择项目')
    await wrapper.get('[data-testid="home-go-projects"]').trigger('click')
    expect(mocks.push).toHaveBeenCalledWith('/projects')
    wrapper.unmount()
  })

  it('sending the first message starts the run and opens inbox', async () => {
    const wrapper = mountDashboard()
    await flushPromises()
    await wrapper.get('[data-testid="home-composer-input"]').setValue('把登录做清楚')
    await wrapper.get('[data-testid="home-composer"]').trigger('submit')
    await flushPromises()
    expect(mocks.startRun).toHaveBeenCalledWith('wf-ap', {}, 'manual', 'normal', [], {
      title: '把登录做清楚',
      firstMessage: { text: '把登录做清楚', images: [] },
    })
    expect(mocks.reactReply).not.toHaveBeenCalled()
    expect(mocks.push).toHaveBeenCalledWith({ path: '/gates', query: { run: 'run-9', node: 'ap' } })
    wrapper.unmount()
  })

  // plan g2.3 — Enter submits; Shift+Enter does not
  it('Enter submits and Shift+Enter keeps the draft for a new line', async () => {
    const wrapper = mountDashboard()
    await flushPromises()
    const input = wrapper.get('[data-testid="home-composer-input"]')
    await input.setValue('一行需求')
    await input.trigger('keydown', { key: 'Enter', shiftKey: true })
    await flushPromises()
    expect(mocks.startRun).not.toHaveBeenCalled()
    await input.trigger('keydown', { key: 'Enter', shiftKey: false })
    await flushPromises()
    expect(mocks.startRun).toHaveBeenCalled()
    wrapper.unmount()
  })

  it('plus opens the file picker; paste and attach-only send work', async () => {
    class FakeReader {
      result: string | ArrayBuffer | null = null
      onload: null | (() => void) = null
      readAsDataURL(file: File) {
        this.result = `data:${file.type || 'application/octet-stream'};base64,QUJD`
        queueMicrotask(() => this.onload?.())
      }
    }
    vi.stubGlobal('FileReader', FakeReader as unknown as typeof FileReader)

    const wrapper = mountDashboard()
    await flushPromises()
    const plus = wrapper.get('[data-testid="home-composer-plus"]')
    expect((plus.element as HTMLButtonElement).disabled).toBe(false)
    const fileInput = wrapper.get('[data-testid="home-composer-file"]').element as HTMLInputElement
    expect(fileInput.multiple).toBe(true)
    expect(fileInput.getAttribute('accept')).toBeNull()
    const clickSpy = vi.spyOn(fileInput, 'click')
    await plus.trigger('click')
    expect(clickSpy).toHaveBeenCalled()

    const note = new File(['ABC'], 'note.txt', { type: 'text/plain' })
    const dt = new DataTransfer()
    dt.items.add(note)
    Object.defineProperty(fileInput, 'files', { configurable: true, value: dt.files })
    await wrapper.get('[data-testid="home-composer-file"]').trigger('change')
    await flushPromises()
    expect(wrapper.get('[data-testid="home-pending-file-chip"]').text()).toContain('note.txt')

    const img = new File(['ABC'], 'clip.png', { type: 'image/png' })
    const pasteDt = new DataTransfer()
    pasteDt.items.add(img)
    const pasteEv = new Event('paste', { bubbles: true, cancelable: true })
    Object.defineProperty(pasteEv, 'clipboardData', { value: pasteDt })
    wrapper.get('[data-testid="home-composer-input"]').element.dispatchEvent(pasteEv)
    await flushPromises()
    expect(wrapper.find('[data-testid="home-draft-image-thumb"]').exists()).toBe(true)

    wrapper.unmount()
    vi.unstubAllGlobals()
  })

  it('opens draft image preview, close keeps attachment, no unavailable overlay (g1.3)', async () => {
    class FakeReader {
      result: string | ArrayBuffer | null = null
      onload: null | (() => void) = null
      readAsDataURL(file: File) {
        this.result = `data:${file.type || 'application/octet-stream'};base64,QUJD`
        queueMicrotask(() => this.onload?.())
      }
    }
    vi.stubGlobal('FileReader', FakeReader as unknown as typeof FileReader)

    const wrapper = mountDashboard()
    await flushPromises()
    const img = new File(['ABC'], '首页截图.png', { type: 'image/png' })
    const pasteDt = new DataTransfer()
    pasteDt.items.add(img)
    const pasteEv = new Event('paste', { bubbles: true, cancelable: true })
    Object.defineProperty(pasteEv, 'clipboardData', { value: pasteDt })
    wrapper.get('[data-testid="home-composer-input"]').element.dispatchEvent(pasteEv)
    await flushPromises()

    const thumb = wrapper.find('[data-testid="home-draft-image-thumb"]')
    expect(thumb.exists()).toBe(true)
    expect(thumb.text()).toContain('点击放大')
    expect(thumb.text()).not.toContain('不可预览')

    await thumb.trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="home-image-preview-title"]').text()).toBe('图片预览 · 首页截图.png')
    expect(wrapper.find('[data-testid="home-image-preview-img"]').attributes('src')).toContain('base64')

    await wrapper.find('[data-testid="home-image-preview-img"]').trigger('error')
    await flushPromises()
    expect(wrapper.find('[data-testid="home-image-preview-failed"]').text()).toContain('图片加载失败')
    await wrapper.find('[data-testid="home-image-preview-close"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="home-image-preview-modal"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="home-draft-image-thumb"]').exists()).toBe(true)

    wrapper.unmount()
    vi.unstubAllGlobals()
  })

  it('sends with attachments only and uses the first filename as title', async () => {
    class FakeReader {
      result: string | ArrayBuffer | null = null
      onload: null | (() => void) = null
      readAsDataURL(file: File) {
        this.result = `data:${file.type || 'application/octet-stream'};base64,QUJD`
        queueMicrotask(() => this.onload?.())
      }
    }
    vi.stubGlobal('FileReader', FakeReader as unknown as typeof FileReader)

    const wrapper = mountDashboard()
    await flushPromises()
    const fileInput = wrapper.get('[data-testid="home-composer-file"]').element as HTMLInputElement
    const note = new File(['ABC'], 'brief.pdf', { type: 'application/pdf' })
    const dt = new DataTransfer()
    dt.items.add(note)
    Object.defineProperty(fileInput, 'files', { configurable: true, value: dt.files })
    await wrapper.get('[data-testid="home-composer-file"]').trigger('change')
    await flushPromises()
    await wrapper.get('[data-testid="home-composer"]').trigger('submit')
    await flushPromises()
    expect(mocks.startRun).toHaveBeenCalledWith('wf-ap', {}, 'manual', 'normal', [], {
      title: 'brief.pdf',
      firstMessage: {
        text: '',
        images: expect.arrayContaining([
          expect.objectContaining({ name: 'brief.pdf', mimeType: 'application/pdf' }),
        ]),
      },
    })
    expect(mocks.reactReply).not.toHaveBeenCalled()
    expect(mocks.push).toHaveBeenCalledWith({ path: '/gates', query: { run: 'run-9', node: 'ap' } })
    wrapper.unmount()
    vi.unstubAllGlobals()
  })

  // plan g1 — pipeline rail scroll: hidden scrollbar + edge arrows
  it('renders pipeline scroll arrows and hides horizontal scrollbar on cards rail', async () => {
    const wrapper = mountDashboard()
    await flushPromises()
    expect(wrapper.find('[data-testid="home-pipeline-rail-wrap"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="home-pipeline-scroll-prev"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="home-pipeline-scroll-next"]').exists()).toBe(true)
    const rail = wrapper.get('[data-testid="home-pipeline-cards"]')
    expect(rail.classes()).toContain('home-pipeline-rail')
    expect(rail.classes()).not.toContain('overflow-x-auto')
    wrapper.unmount()
  })

  it('disables scroll arrows when pipeline list does not overflow', async () => {
    const wrapper = mountDashboard()
    await flushPromises()
    const prev = wrapper.get('[data-testid="home-pipeline-scroll-prev"]').element as HTMLButtonElement
    const next = wrapper.get('[data-testid="home-pipeline-scroll-next"]').element as HTMLButtonElement
    expect(prev.disabled).toBe(true)
    expect(next.disabled).toBe(true)
    wrapper.unmount()
  })

  it('syncs scroll arrow disabled state and edge fades when rail overflows', async () => {
    const many = Array.from({ length: 8 }, (_, i) => ({
      ...approveWf,
      id: `wf-${i}`,
      name: `流水线 ${i}`,
    }))
    mocks.listWorkflows.mockResolvedValue(many)
    const wrapper = mountDashboard()
    await flushPromises()

    const rail = wrapper.get('[data-testid="home-pipeline-cards"]').element as HTMLDivElement
    Object.defineProperty(rail, 'clientWidth', { configurable: true, value: 400 })
    Object.defineProperty(rail, 'scrollWidth', { configurable: true, value: 1600 })
    let scrollLeft = 0
    Object.defineProperty(rail, 'scrollLeft', {
      configurable: true,
      get: () => scrollLeft,
      set: (v: number) => {
        scrollLeft = v
      },
    })

    await rail.dispatchEvent(new Event('scroll'))
    await flushPromises()
    expect(wrapper.get('[data-testid="home-pipeline-cards"]').classes()).toContain(
      'home-pipeline-rail--overflow',
    )
    const prev = wrapper.get('[data-testid="home-pipeline-scroll-prev"]').element as HTMLButtonElement
    const next = wrapper.get('[data-testid="home-pipeline-scroll-next"]').element as HTMLButtonElement
    expect(prev.disabled).toBe(true)
    expect(next.disabled).toBe(false)
    expect(wrapper.find('.home-pipeline-rail-wrap--has-right').exists()).toBe(true)
    expect(wrapper.find('.home-pipeline-rail-wrap--has-left').exists()).toBe(false)

    scrollLeft = 1200
    await rail.dispatchEvent(new Event('scroll'))
    await flushPromises()
    expect(prev.disabled).toBe(false)
    expect(next.disabled).toBe(true)
    expect(wrapper.find('.home-pipeline-rail-wrap--has-left').exists()).toBe(true)
    expect(wrapper.find('.home-pipeline-rail-wrap--has-right').exists()).toBe(false)

    scrollLeft = 0
    await rail.dispatchEvent(new Event('scroll'))
    await flushPromises()
    expect(prev.disabled).toBe(true)
    expect(wrapper.find('.home-pipeline-rail-wrap--has-left').exists()).toBe(false)

    wrapper.unmount()
  })

  it('left-aligns pipeline rail when overflowing so first card is not clipped', async () => {
    const many = Array.from({ length: 6 }, (_, i) => ({
      ...approveWf,
      id: `wf-${i}`,
      name: `流水线 ${i}`,
    }))
    mocks.listWorkflows.mockResolvedValue(many)
    const wrapper = mountDashboard()
    await flushPromises()

    const rail = wrapper.get('[data-testid="home-pipeline-cards"]')
    const railEl = rail.element as HTMLDivElement
    Object.defineProperty(railEl, 'clientWidth', { configurable: true, value: 400 })
    Object.defineProperty(railEl, 'scrollWidth', { configurable: true, value: 1200 })
    Object.defineProperty(railEl, 'scrollLeft', { configurable: true, value: 0, writable: true })

    await railEl.dispatchEvent(new Event('scroll'))
    await flushPromises()

    expect(rail.classes()).toContain('home-pipeline-rail--overflow')
    expect(rail.classes()).not.toContain('justify-center')
    wrapper.unmount()
  })

  it('keeps pipeline rail centered when cards do not overflow', async () => {
    const wrapper = mountDashboard()
    await flushPromises()

    const rail = wrapper.get('[data-testid="home-pipeline-cards"]')
    const railEl = rail.element as HTMLDivElement
    Object.defineProperty(railEl, 'clientWidth', { configurable: true, value: 800 })
    Object.defineProperty(railEl, 'scrollWidth', { configurable: true, value: 200 })
    Object.defineProperty(railEl, 'scrollLeft', { configurable: true, value: 0, writable: true })

    await railEl.dispatchEvent(new Event('scroll'))
    await flushPromises()

    expect(rail.classes()).not.toContain('home-pipeline-rail--overflow')
    wrapper.unmount()
  })

  it('arrow click does not change pipeline card selection', async () => {
    const second: Workflow = {
      ...approveWf,
      id: 'wf-lite',
      name: '快速澄清 Lite',
      description: '轻量 Approve 入口',
    }
    mocks.listWorkflows.mockResolvedValue([approveWf, second])
    const wrapper = mountDashboard()
    await flushPromises()

    const rail = wrapper.get('[data-testid="home-pipeline-cards"]').element as HTMLDivElement
    Object.defineProperty(rail, 'clientWidth', { configurable: true, value: 200 })
    Object.defineProperty(rail, 'scrollWidth', { configurable: true, value: 800 })
    Object.defineProperty(rail, 'scrollLeft', {
      configurable: true,
      get: () => 0,
      set: () => {},
    })
    await rail.dispatchEvent(new Event('scroll'))
    await flushPromises()

    await wrapper.get('[data-testid="home-pipeline-scroll-next"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-testid="home-pipeline-card-wf-ap"]').classes()).toContain(
      'home-shell__card--selected',
    )
    wrapper.unmount()
  })
})
