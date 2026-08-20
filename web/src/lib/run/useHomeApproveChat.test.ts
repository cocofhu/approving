// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { createApp, defineComponent, nextTick } from 'vue'
import { flushPromises } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'
import type { Workflow } from '@/lib/shared/types'

const mocks = vi.hoisted(() => ({
  push: vi.fn(),
  toastWarn: vi.fn(),
  toastError: vi.fn(),
  listWorkflows: vi.fn(),
  startRun: vi.fn(),
  getRun: vi.fn(),
  reactReply: vi.fn(),
  readStoredProjectId: vi.fn(() => 'proj-1'),
}))

vi.mock('vue-router', () => ({
  useRouter: () => ({ push: mocks.push }),
}))

vi.mock('@/lib/composables/useToast', () => ({
  useToast: () => ({ warn: mocks.toastWarn, error: mocks.toastError, success: vi.fn() }),
}))

vi.mock('@/lib/composables/useProjectContext', () => ({
  readStoredProjectId: () => mocks.readStoredProjectId(),
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

import { useHomeApproveChat } from './useHomeApproveChat'

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
    { id: 'out', type: 'output', label: '结束', position: { x: 0, y: 0 }, config: {} },
  ],
  edges: [
    { id: 'e1', source: 'in', target: 'ap' },
    { id: 'e2', source: 'ap', target: 'out' },
  ],
}

const reactWf: Workflow = {
  ...approveWf,
  id: 'wf-react',
  name: '实现流',
  nodes: [
    { id: 'in', type: 'input', label: '开始', position: { x: 0, y: 0 }, config: {} },
    { id: 'r', type: 'react', label: '实现', position: { x: 0, y: 0 }, config: {} },
  ],
  edges: [{ id: 'e1', source: 'in', target: 'r' }],
}

function withSetup<T>(fn: () => T): T {
  let result!: T
  const i18n = createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...common, ...pages } },
  })
  const Comp = defineComponent({
    setup() {
      result = fn()
      return () => null
    },
  })
  const app = createApp(Comp)
  app.use(i18n)
  app.mount(document.createElement('div'))
  return result
}

describe('useHomeApproveChat', () => {
  beforeEach(() => {
    mocks.push.mockReset()
    mocks.toastWarn.mockReset()
    mocks.toastError.mockReset()
    mocks.listWorkflows.mockReset()
    mocks.startRun.mockReset()
    mocks.getRun.mockReset()
    mocks.reactReply.mockReset()
    mocks.readStoredProjectId.mockReturnValue('proj-1')
    mocks.listWorkflows.mockResolvedValue([approveWf, reactWf])
    mocks.startRun.mockResolvedValue({ id: 'run-1', status: 'queued' })
    mocks.getRun.mockResolvedValue({
      id: 'run-1',
      status: 'waiting_human',
      nodes: approveWf.nodes,
      nodeRuns: { ap: { nodeId: 'ap', status: 'waiting_human' } },
    })
    mocks.reactReply.mockResolvedValue({ status: 'ok' })
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('filters to published approve-first pipelines', async () => {
    const chat = withSetup(() => useHomeApproveChat())
    await chat.load()
    expect(chat.pipelines.value.map((w) => w.id)).toEqual(['wf-ap'])
    expect(chat.selected.value?.id).toBe('wf-ap')
  })

  it('starts a run carrying the first message, then opens inbox', async () => {
    const chat = withSetup(() => useHomeApproveChat())
    await chat.load()
    chat.draft.value = '  把登录做清楚  '
    await chat.send()
    await nextTick()
    expect(mocks.startRun).toHaveBeenCalledWith('wf-ap', {}, 'manual', 'normal', [], {
      title: '把登录做清楚',
      firstMessage: { text: '把登录做清楚', images: [] },
    })
    // The engine delivers the message once the approve node parks.
    expect(mocks.reactReply).not.toHaveBeenCalled()
    expect(mocks.push).toHaveBeenCalledWith({ path: '/gates', query: { run: 'run-1', node: 'ap' } })
  })

  it('carries attachments on the first message and navigates to inbox', async () => {
    const chat = withSetup(() => useHomeApproveChat())
    await chat.load()
    chat.attachments.value = [{ data: 'abc', mimeType: 'image/png', name: 'shot.png' }]
    await chat.send()
    await nextTick()
    expect(mocks.startRun).toHaveBeenCalledWith('wf-ap', {}, 'manual', 'normal', [], {
      title: 'shot.png',
      firstMessage: {
        text: '',
        images: [{ data: 'abc', mimeType: 'image/png', name: 'shot.png' }],
      },
    })
    expect(mocks.reactReply).not.toHaveBeenCalled()
    expect(mocks.push).toHaveBeenCalledWith({ path: '/gates', query: { run: 'run-1', node: 'ap' } })
  })

  it('opens inbox immediately without polling for the Approve park', async () => {
    mocks.getRun.mockImplementation(() => new Promise(() => {}))
    const chat = withSetup(() => useHomeApproveChat())
    await chat.load()
    chat.attachments.value = [{ data: 'abc', mimeType: 'image/png', name: 'shot.png' }]
    chat.draft.value = '附图说明'
    void chat.send()
    await flushPromises()
    expect(mocks.push).toHaveBeenCalledWith({ path: '/gates', query: { run: 'run-1', node: 'ap' } })
    expect(mocks.getRun).not.toHaveBeenCalled()
    expect(mocks.reactReply).not.toHaveBeenCalled()
  })

  it('navigates even when the home view unmounts mid-send', async () => {
    let result!: ReturnType<typeof useHomeApproveChat>
    const i18n = createI18n({
      legacy: false,
      locale: 'zh-CN',
      messages: { 'zh-CN': { ...common, ...pages } },
    })
    const Comp = defineComponent({
      setup() {
        result = useHomeApproveChat()
        return () => null
      },
    })
    const app = createApp(Comp)
    app.use(i18n)
    app.mount(document.createElement('div'))
    await result.load()
    result.draft.value = '卸载后仍要跳转'
    const sendPromise = result.send()
    app.unmount()
    await sendPromise
    expect(mocks.startRun).toHaveBeenCalledWith('wf-ap', {}, 'manual', 'normal', [], {
      title: '卸载后仍要跳转',
      firstMessage: { text: '卸载后仍要跳转', images: [] },
    })
    expect(mocks.push).toHaveBeenCalledWith({ path: '/gates', query: { run: 'run-1', node: 'ap' } })
  })

  it('opens the launch modal when required ask fields are empty', async () => {
    const missing = {
      ...approveWf,
      nodes: [
        {
          id: 'in',
          type: 'input' as const,
          label: '开始',
          position: { x: 0, y: 0 },
          config: {
            variables: [{ name: 'repos', ask: true, required: true, type: 'repos', value: [] }],
          },
        },
        approveWf.nodes[1],
        approveWf.nodes[2],
      ],
    }
    mocks.listWorkflows.mockResolvedValue([missing])
    const chat = withSetup(() => useHomeApproveChat())
    await chat.load()
    chat.draft.value = '先澄清目标'
    await chat.send()
    expect(mocks.startRun).not.toHaveBeenCalled()
    expect(chat.launchOpen.value).toBe(true)
    expect(mocks.toastWarn).toHaveBeenCalled()
  })
})
