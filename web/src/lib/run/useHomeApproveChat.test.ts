// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { createApp, defineComponent, nextTick } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'
import { SITE_ATTACH_MAX_BYTES } from '@/lib/shared/attachments'
import type { ClarifyImage, Workflow } from '@/lib/shared/types'

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

function tinyPng(): ClarifyImage {
  return {
    data: 'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==',
    mimeType: 'image/png',
    name: 'shot.png',
  }
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

  it('starts a run with the first message as title, replies, then opens the run', async () => {
    const chat = withSetup(() => useHomeApproveChat())
    await chat.load()
    chat.draft.value = '  把登录做清楚  '
    await chat.send()
    await nextTick()
    expect(mocks.startRun).toHaveBeenCalledWith('wf-ap', {}, 'manual', 'normal', [], {
      title: '把登录做清楚',
    })
    expect(mocks.reactReply).toHaveBeenCalledWith('run-1', 'ap', '把登录做清楚', [])
    expect(mocks.push).toHaveBeenCalledWith({ path: '/runs/run-1', query: { node: 'ap' } })
  })

  it('sends text + attachments via reactReply and clears pending list', async () => {
    const chat = withSetup(() => useHomeApproveChat())
    await chat.load()
    const img = tinyPng()
    chat.draft.value = '带图启动'
    chat.attachments.value = [img]
    await chat.send()
    await nextTick()
    expect(mocks.startRun).toHaveBeenCalledWith('wf-ap', {}, 'manual', 'normal', [], {
      title: '带图启动',
    })
    expect(mocks.reactReply).toHaveBeenCalledWith('run-1', 'ap', '带图启动', [img])
    expect(chat.attachments.value).toEqual([])
    expect(chat.draft.value).toBe('')
  })

  it('allows attachment-only send with default title 附件启动', async () => {
    const chat = withSetup(() => useHomeApproveChat())
    await chat.load()
    const img = tinyPng()
    chat.attachments.value = [img]
    await chat.send()
    await nextTick()
    expect(mocks.startRun).toHaveBeenCalledWith('wf-ap', {}, 'manual', 'normal', [], {
      title: '附件启动',
    })
    expect(mocks.reactReply).toHaveBeenCalledWith('run-1', 'ap', '', [img])
    expect(chat.attachments.value).toEqual([])
  })

  it('blocks empty send when both text and attachments are missing', async () => {
    const chat = withSetup(() => useHomeApproveChat())
    await chat.load()
    await chat.send()
    expect(mocks.startRun).not.toHaveBeenCalled()
    expect(mocks.toastWarn).toHaveBeenCalled()
  })

  it('blocks send when an oversized attachment remains', async () => {
    const chat = withSetup(() => useHomeApproveChat())
    await chat.load()
    const overB64 = 'A'.repeat(Math.ceil(((SITE_ATTACH_MAX_BYTES + 1024) * 4) / 3))
    chat.attachments.value = [{ data: overB64, mimeType: 'application/octet-stream', name: 'huge.bin' }]
    await chat.send()
    expect(mocks.startRun).not.toHaveBeenCalled()
    expect(chat.attachNotice.value?.kind).toBe('error')
    expect(chat.attachNotice.value?.text).toContain('发送已阻止')
  })

  it('keeps draft attachments when launch modal opens for missing required fields', async () => {
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
    const img = tinyPng()
    chat.draft.value = '先澄清目标'
    chat.attachments.value = [img]
    await chat.send()
    expect(mocks.startRun).not.toHaveBeenCalled()
    expect(chat.launchOpen.value).toBe(true)
    expect(chat.draft.value).toBe('先澄清目标')
    expect(chat.attachments.value).toEqual([img])
    expect(chat.launchTitle.value).toBe('先澄清目标')
    expect(mocks.toastWarn).toHaveBeenCalled()
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
