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

const approveWf: Workflow = {
  id: 'wf-ap',
  name: '自我迭代PRO',
  description: '开发前澄清 + 计划',
  status: 'published',
  version: 1,
  updatedAt: '',
  needsRepo: false,
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
      stubs: { Icon: true, RunLaunchModal: true },
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

  it('renders composer and approve-first cards when a project is selected', async () => {
    const wrapper = mountDashboard()
    await flushPromises()
    expect(wrapper.get('[data-testid="home-title"]').text()).toContain('从一句话开始一次开发前澄清')
    expect(wrapper.find('[data-testid="home-composer"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="home-pipeline-card-wf-ap"]').text()).toContain('自我迭代PRO')
    wrapper.unmount()
  })

  // plan g1.1 — no full-bleed purple stage layer
  it('does not render full-screen purple stage atmosphere', async () => {
    const wrapper = mountDashboard()
    await flushPromises()
    expect(wrapper.find('[data-testid="home-stage-bg"]').exists()).toBe(false)
    expect(wrapper.find('.home-stage__wash').exists()).toBe(false)
    expect(wrapper.find('.home-stage__glow').exists()).toBe(false)
    wrapper.unmount()
  })

  // plan g1.2 / g1.3 — Approving mono brand + Chinese hint（无 subtitle，对齐第 5 轮）
  it('renders monospace Approving brand and Chinese hint', async () => {
    const wrapper = mountDashboard()
    await flushPromises()
    expect(wrapper.get('[data-testid="home-brand"]').classes()).toContain('home-brand')
    expect(wrapper.get('[data-testid="home-title"]').text()).toBe('从一句话开始一次开发前澄清')
    expect(wrapper.find('[data-testid="home-subtitle"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="home-filter-hint"]').text()).toContain('Approve')
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

  // plan g1.3 — one-shot typewriter then settle
  it('types Approving once then settles without looping', async () => {
    const wrapper = mountDashboard()
    await flushPromises()
    expect(wrapper.get('[data-testid="home-brand-text"]').text()).toBe('')
    await vi.advanceTimersByTimeAsync(120 + 72 * 9 + 50)
    expect(wrapper.get('[data-testid="home-brand-text"]').text()).toBe('Approving')
    expect(wrapper.find('[data-testid="home-brand-cursor"]').exists()).toBe(true)
    await vi.advanceTimersByTimeAsync(420 + 380 * 6 + 50)
    expect(wrapper.get('[data-testid="home-brand-text"]').text()).toBe('Approving')
    expect(wrapper.find('[data-testid="home-brand-cursor"]').exists()).toBe(false)
    await vi.advanceTimersByTimeAsync(5000)
    expect(wrapper.get('[data-testid="home-brand-text"]').text()).toBe('Approving')
    expect(wrapper.find('[data-testid="home-brand-cursor"]').exists()).toBe(false)
    wrapper.unmount()
  })

  // plan g3.3 — reduced-motion shows static brand
  it('shows full Approving immediately under reduced-motion', async () => {
    stubReducedMotion(true)
    const wrapper = mountDashboard()
    await flushPromises()
    expect(wrapper.get('[data-testid="home-brand-text"]').text()).toBe('Approving')
    expect(wrapper.find('[data-testid="home-brand-cursor"]').exists()).toBe(false)
    wrapper.unmount()
  })

  // plan g2.1 — placeholder typewriter when idle/empty
  it('shows placeholder typewriter when empty and unfocused', async () => {
    const wrapper = mountDashboard()
    await flushPromises()
    expect(wrapper.find('[data-testid="home-composer-placeholder"]').exists()).toBe(true)
    await vi.advanceTimersByTimeAsync(80 + 64 * 9 + 50)
    expect(wrapper.get('[data-testid="home-composer-placeholder"]').text()).toContain('快速开启你的迭代')
    await wrapper.get('[data-testid="home-composer-input"]').trigger('focus')
    await flushPromises()
    expect(wrapper.find('[data-testid="home-composer-placeholder"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('shows project empty state when no project is stored', async () => {
    mocks.readStoredProjectId.mockReturnValue('')
    const wrapper = mountDashboard()
    await flushPromises()
    expect(wrapper.find('[data-testid="home-no-project"]').exists()).toBe(true)
    await wrapper.get('[data-testid="dashboard-select-project"]').trigger('click')
    expect(mocks.push).toHaveBeenCalledWith('/projects')
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
})
