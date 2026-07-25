// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'
import { extractAgentMessageDelta } from '@/lib/acpUnpack'
import PmLeaderChat from './PmLeaderChat.vue'

const apiMocks = vi.hoisted(() => ({
  listPmThreads: vi.fn(),
  createPmThread: vi.fn(),
  listPmMessages: vi.fn(),
  appendPmMessage: vi.fn(),
  ensurePmSandbox: vi.fn(),
  getSandbox: vi.fn(),
  sandboxChatWsUrl: vi.fn(() => 'ws://test/sandbox/1'),
  pmThreadChatWsUrl: vi.fn(() => 'ws://test/pm/thr-new/chat'),
  getPmDraft: vi.fn(),
  patchPmMessage: vi.fn(),
  deletePmThread: vi.fn(),
}))

vi.mock('@/lib/api', async () => {
  const actual = await vi.importActual<typeof import('@/lib/api')>('@/lib/api')
  return {
    ...actual,
    api: {
      ...actual.api,
      listPmThreads: apiMocks.listPmThreads,
      createPmThread: apiMocks.createPmThread,
      listPmMessages: apiMocks.listPmMessages,
      appendPmMessage: apiMocks.appendPmMessage,
      ensurePmSandbox: apiMocks.ensurePmSandbox,
      getSandbox: apiMocks.getSandbox,
      sandboxChatWsUrl: apiMocks.sandboxChatWsUrl,
      pmThreadChatWsUrl: apiMocks.pmThreadChatWsUrl,
      getPmDraft: apiMocks.getPmDraft,
      patchPmMessage: apiMocks.patchPmMessage,
      deletePmThread: apiMocks.deletePmThread,
    },
  }
})

class MockWebSocket {
  static CONNECTING = 0
  static OPEN = 1
  static CLOSING = 2
  static CLOSED = 3
  readyState = MockWebSocket.OPEN
  onmessage: ((ev: MessageEvent) => void) | null = null
  onerror: (() => void) | null = null
  onclose: (() => void) | null = null
  send = vi.fn()
  close = vi.fn()
  addEventListener = vi.fn()
  removeEventListener = vi.fn()
}

function mountChat(
  binding: {
    enabled: boolean
    agentAvailable: boolean
    agentConfigRef: string
    aclNote: string
  } = {
    enabled: true,
    agentAvailable: true,
    agentConfigRef: 'agent-1',
    aclNote: '',
  },
) {
  const i18n = createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...common, ...pages } },
  })
  return mount(PmLeaderChat, {
    props: {
      projectId: 'proj-1',
      binding,
    },
    global: {
      plugins: [i18n],
      stubs: { Icon: true, CitationCard: true },
    },
  })
}

describe('PmLeaderChat first-send without thread', () => {
  const originalWS = globalThis.WebSocket

  beforeEach(() => {
    vi.clearAllMocks()
    localStorage.clear()
    // @ts-expect-error test stub
    globalThis.WebSocket = MockWebSocket
    apiMocks.listPmThreads.mockResolvedValue({ items: [] })
    apiMocks.createPmThread.mockResolvedValue({ id: 'thr-new', title: '新会话' })
    apiMocks.listPmMessages.mockResolvedValue({ items: [] })
    apiMocks.getPmDraft.mockResolvedValue({ draft: null, live: false, hasFinal: false })
    apiMocks.appendPmMessage.mockImplementation(async (_pid: string, _tid: string, body: { content?: string; role?: string }) => ({
      id: 'msg-u1',
      role: body.role || 'user',
      content: body.content || '',
      status: 'ok',
    }))
    apiMocks.ensurePmSandbox.mockResolvedValue({
      sandbox: { id: 1, status: 'running' },
      preamble: '',
    })
    apiMocks.getSandbox.mockResolvedValue({ id: 1, status: 'running' })
  })

  afterEach(() => {
    globalThis.WebSocket = originalWS
  })

  it('creates a thread and appends the first user message (no silent abort)', async () => {
    const wrapper = mountChat()
    await flushPromises()

    const suggestion = wrapper.findAll('button').find((b) => b.text().includes('项目整体进度如何？'))
    expect(suggestion).toBeTruthy()
    await suggestion!.trigger('click')
    await flushPromises()

    expect(apiMocks.createPmThread).toHaveBeenCalledWith('proj-1')
    expect(apiMocks.appendPmMessage).toHaveBeenCalledWith(
      'proj-1',
      'thr-new',
      expect.objectContaining({
        role: 'user',
        content: '项目整体进度如何？',
      }),
    )
    expect(wrapper.find('[data-testid="pm-stream-bubble"]').exists()).toBe(true)
    expect(wrapper.find('.typing-dots').exists()).toBe(true)
    expect(wrapper.text()).not.toContain('正在生成')
    expect(wrapper.text()).toContain('项目整体进度如何？')
    expect(wrapper.find('[data-testid="pm-idle-suggestions"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('first send from the composer also works with no active thread', async () => {
    const wrapper = mountChat()
    await flushPromises()

    const textarea = wrapper.find('textarea')
    expect(textarea.exists()).toBe(true)
    await textarea.setValue('本周有哪些风险？')
    await flushPromises()

    const sendBtn = wrapper.findAll('button').find((b) => b.text() === '发送')
    expect(sendBtn).toBeTruthy()
    await sendBtn!.trigger('click')
    await flushPromises()

    expect(apiMocks.createPmThread).toHaveBeenCalledTimes(1)
    expect(apiMocks.appendPmMessage).toHaveBeenCalledWith(
      'proj-1',
      'thr-new',
      expect.objectContaining({
        role: 'user',
        content: '本周有哪些风险？',
      }),
    )
    expect(wrapper.find('[data-testid="pm-stream-bubble"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('本周有哪些风险？')
    expect((wrapper.find('textarea').element as HTMLTextAreaElement).value).toBe('')
    wrapper.unmount()
  })

  it('disables new-thread and other-thread switch while busy after first send', async () => {
    apiMocks.listPmThreads.mockResolvedValue({
      items: [
        { id: 'thr-a', title: '会话A' },
        { id: 'thr-b', title: '会话B' },
      ],
    })
    apiMocks.listPmMessages.mockResolvedValue({ items: [] })

    const wrapper = mountChat()
    await flushPromises()

    const textarea = wrapper.find('textarea')
    await textarea.setValue('忙态禁切换')
    const sendBtn = wrapper.findAll('button').find((b) => b.text() === '发送')
    await sendBtn!.trigger('click')
    await flushPromises()

    const newBtn = wrapper.findAll('button').find((b) => b.text() === '新建')
    expect(newBtn).toBeTruthy()
    expect(newBtn!.attributes('disabled')).toBeDefined()

    const btnA = wrapper.findAll('aside button').find((b) => b.text().includes('会话A'))
    const btnB = wrapper.findAll('aside button').find((b) => b.text().includes('会话B'))
    expect(btnA).toBeTruthy()
    expect(btnB).toBeTruthy()
    expect(btnA!.attributes('disabled')).toBeUndefined()
    expect(btnB!.attributes('disabled')).toBeDefined()

    wrapper.unmount()
  })
})

describe('PmLeaderChat ACP + hydrate', () => {
  const originalWS = globalThis.WebSocket

  beforeEach(() => {
    vi.clearAllMocks()
    localStorage.clear()
    // @ts-expect-error test stub
    globalThis.WebSocket = MockWebSocket
    apiMocks.listPmThreads.mockResolvedValue({ items: [] })
    apiMocks.createPmThread.mockResolvedValue({ id: 'thr-new', title: '新会话' })
    apiMocks.listPmMessages.mockResolvedValue({ items: [] })
    apiMocks.getPmDraft.mockResolvedValue({ draft: null, live: false, hasFinal: false })
    apiMocks.appendPmMessage.mockImplementation(async (_pid: string, _tid: string, body: { content?: string; role?: string }) => ({
      id: 'msg-u1',
      role: body.role || 'user',
      content: body.content || '',
      status: 'ok',
    }))
    apiMocks.ensurePmSandbox.mockResolvedValue({
      sandbox: { id: 1, status: 'running' },
      preamble: '',
    })
    apiMocks.getSandbox.mockResolvedValue({ id: 1, status: 'running' })
    apiMocks.patchPmMessage.mockResolvedValue({ id: 'u1', status: 'failed', failKind: 'unknown' })
  })

  afterEach(() => {
    globalThis.WebSocket = originalWS
  })

  it('extractAgentMessageDelta accumulates nested op:event chunks (false-empty fix)', () => {
    const nested = {
      op: 'event',
      data: {
        type: 'session_update',
        update: {
          sessionUpdate: 'agent_message_chunk',
          content: { type: 'text', text: '进度正常' },
        },
      },
    }
    expect(extractAgentMessageDelta(nested)?.text).toBe('进度正常')
  })

  it('turn_done with no chunk maps to empty failKind (vacuum empty)', async () => {
    apiMocks.listPmThreads.mockResolvedValue({ items: [{ id: 'thr-1', title: 't' }] })
    apiMocks.listPmMessages.mockResolvedValue({
      items: [{ id: 'u1', role: 'user', content: '?', status: 'ok' }],
    })
    apiMocks.getPmDraft.mockResolvedValue({ draft: null, live: false, hasFinal: false })
    apiMocks.appendPmMessage.mockResolvedValue({
      id: 'u1',
      role: 'user',
      content: '?',
      status: 'ok',
    })
    apiMocks.patchPmMessage.mockResolvedValue({ id: 'u1', status: 'failed', failKind: 'empty' })

    let socket: MockWebSocket | null = null
    class CaptureWS extends MockWebSocket {
      constructor() {
        super()
        socket = this
      }
    }
    // @ts-expect-error test stub
    globalThis.WebSocket = CaptureWS

    const wrapper = mountChat()
    await flushPromises()

    const textarea = wrapper.find('textarea')
    await textarea.setValue('again')
    const sendBtn = wrapper.findAll('button').find((b) => b.text() === '发送')
    await sendBtn!.trigger('click')
    await flushPromises()

    expect(socket).toBeTruthy()
    // Server empty path: error frame with failKind=empty (turn produced no agent text).
    socket!.onmessage?.(
      new MessageEvent('message', {
        data: JSON.stringify({ type: 'error', failKind: 'empty', message: 'empty reply', seq: 0 }),
      }),
    )
    await flushPromises()

    expect(wrapper.text()).toContain('空回复')
    expect(wrapper.text()).toContain('重试')
    expect(apiMocks.patchPmMessage).toHaveBeenCalledWith(
      'proj-1',
      'thr-1',
      'u1',
      expect.objectContaining({ status: 'failed', failKind: 'empty' }),
    )
    wrapper.unmount()
  })

  it('duplicate acp seq does not double-append stream text', async () => {
    apiMocks.listPmThreads.mockResolvedValue({ items: [{ id: 'thr-1', title: 't' }] })
    apiMocks.listPmMessages.mockResolvedValue({ items: [] })
    apiMocks.getPmDraft.mockResolvedValue({ draft: null, live: false, hasFinal: false })

    let socket: MockWebSocket | null = null
    class CaptureWS extends MockWebSocket {
      constructor() {
        super()
        socket = this
      }
    }
    // @ts-expect-error test stub
    globalThis.WebSocket = CaptureWS

    const wrapper = mountChat()
    await flushPromises()

    const textarea = wrapper.find('textarea')
    await textarea.setValue('进度？')
    const sendBtn = wrapper.findAll('button').find((b) => b.text() === '发送')
    await sendBtn!.trigger('click')
    await flushPromises()

    const chunk = (text: string) => ({
      op: 'event',
      data: {
        type: 'session_update',
        update: {
          sessionUpdate: 'agent_message_chunk',
          content: { type: 'text', text },
        },
      },
    })

    expect(socket).toBeTruthy()
    socket!.onmessage?.(
      new MessageEvent('message', {
        data: JSON.stringify({ type: 'acp', data: chunk('Hello'), seq: 0 }),
      }),
    )
    // Same seq again — must not double-append (double fan-out regression).
    socket!.onmessage?.(
      new MessageEvent('message', {
        data: JSON.stringify({ type: 'acp', data: chunk('Hello'), seq: 0 }),
      }),
    )
    socket!.onmessage?.(
      new MessageEvent('message', {
        data: JSON.stringify({ type: 'acp', data: chunk('!'), seq: 1 }),
      }),
    )
    await flushPromises()

    expect(wrapper.text()).toContain('Hello!')
    expect(wrapper.text()).not.toMatch(/HelloHello/)
    wrapper.unmount()
  })

  it('failed draft persists failure card without skipAll blocking Retry', async () => {
    apiMocks.listPmThreads.mockResolvedValue({ items: [{ id: 'thr-1', title: 't' }] })
    apiMocks.listPmMessages.mockResolvedValue({
      items: [{ id: 'u1', role: 'user', content: 'q', status: 'ok' }],
    })
    apiMocks.getPmDraft.mockResolvedValue({
      draft: {
        id: 'd1',
        threadId: 'thr-1',
        userMsgId: 'u1',
        partialText: '',
        chunkIndex: 0,
        eventSeq: 0,
        status: 'failed',
        failKind: 'empty',
      },
      live: false,
      hasFinal: false,
    })
    apiMocks.patchPmMessage.mockResolvedValue({ id: 'u1', status: 'failed', failKind: 'empty' })

    const wrapper = mountChat()
    await flushPromises()

    expect(apiMocks.patchPmMessage).toHaveBeenCalledWith(
      'proj-1',
      'thr-1',
      'u1',
      expect.objectContaining({ status: 'failed', failKind: 'empty' }),
    )
    expect(wrapper.text()).toContain('空回复')
    expect(wrapper.text()).toContain('重试')
    wrapper.unmount()
  })

  it('hydrate prefers final over draft (s4) — hasFinal skips resume', async () => {
    apiMocks.listPmThreads.mockResolvedValue({ items: [{ id: 'thr-1', title: 't' }] })
    apiMocks.listPmMessages.mockResolvedValue({
      items: [
        { id: 'u1', role: 'user', content: 'q', status: 'ok' },
        { id: 'a1', role: 'assistant', content: 'final answer' },
      ],
    })
    apiMocks.getPmDraft.mockResolvedValue({
      draft: {
        id: 'd1',
        threadId: 'thr-1',
        userMsgId: 'u1',
        partialText: 'stale',
        chunkIndex: 1,
        eventSeq: 0,
        status: 'streaming',
      },
      live: false,
      hasFinal: true,
    })

    const wrapper = mountChat()
    await flushPromises()

    expect(wrapper.text()).toContain('final answer')
    expect(wrapper.text()).not.toContain('正在续接回复')
    expect(apiMocks.ensurePmSandbox).not.toHaveBeenCalled()
    expect(apiMocks.pmThreadChatWsUrl).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('streaming draft auto-resumes without orphan→unknown', async () => {
    apiMocks.listPmThreads.mockResolvedValue({ items: [{ id: 'thr-1', title: 't' }] })
    apiMocks.listPmMessages.mockResolvedValue({
      items: [{ id: 'u1', role: 'user', content: 'q', status: 'ok' }],
    })
    apiMocks.getPmDraft.mockResolvedValue({
      draft: {
        id: 'd1',
        threadId: 'thr-1',
        userMsgId: 'u1',
        partialText: 'partial…',
        chunkIndex: 2,
        eventSeq: 1,
        status: 'streaming',
      },
      live: true,
      hasFinal: false,
    })

    const wrapper = mountChat()
    await flushPromises()

    expect(wrapper.text()).toContain('partial…')
    expect(apiMocks.pmThreadChatWsUrl).toHaveBeenCalledWith('proj-1', 'thr-1')
    expect(apiMocks.patchPmMessage).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('streaming + !live does not resume and shows connection + failed partial (S2)', async () => {
    apiMocks.listPmThreads.mockResolvedValue({ items: [{ id: 'thr-1', title: 't' }] })
    apiMocks.listPmMessages.mockResolvedValue({
      items: [{ id: 'u1', role: 'user', content: 'q', status: 'ok' }],
    })
    apiMocks.getPmDraft.mockResolvedValue({
      draft: {
        id: 'd1',
        threadId: 'thr-1',
        userMsgId: 'u1',
        partialText: '半成品内容',
        chunkIndex: 2,
        eventSeq: 1,
        status: 'streaming',
      },
      live: false,
      hasFinal: false,
    })
    apiMocks.patchPmMessage.mockResolvedValue({ id: 'u1', status: 'failed', failKind: 'connection' })

    const wrapper = mountChat()
    await flushPromises()

    expect(apiMocks.pmThreadChatWsUrl).not.toHaveBeenCalled()
    expect(apiMocks.ensurePmSandbox).not.toHaveBeenCalled()
    expect(apiMocks.patchPmMessage).toHaveBeenCalledWith(
      'proj-1',
      'thr-1',
      'u1',
      expect.objectContaining({ status: 'failed', failKind: 'connection' }),
    )
    expect(wrapper.text()).toContain('半成品内容')
    expect(wrapper.text()).toContain('已停止 · 半成品已保留')
    expect(wrapper.text()).toContain('连接中断')
    expect(wrapper.text()).not.toContain('未知错误')
    expect(wrapper.find('[data-testid="pm-failed-partial"]').exists()).toBe(true)
    // Composer stays usable (busy=false).
    expect(wrapper.find('textarea').attributes('disabled')).toBeUndefined()
    wrapper.unmount()
  })

  it('streaming + !live without partial shows only connection fail card (S3)', async () => {
    apiMocks.listPmThreads.mockResolvedValue({ items: [{ id: 'thr-1', title: 't' }] })
    apiMocks.listPmMessages.mockResolvedValue({
      items: [{ id: 'u1', role: 'user', content: 'q', status: 'ok' }],
    })
    apiMocks.getPmDraft.mockResolvedValue({
      draft: {
        id: 'd1',
        threadId: 'thr-1',
        userMsgId: 'u1',
        partialText: '',
        chunkIndex: 0,
        eventSeq: 0,
        status: 'streaming',
      },
      live: false,
      hasFinal: false,
    })
    apiMocks.patchPmMessage.mockResolvedValue({ id: 'u1', status: 'failed', failKind: 'connection' })

    const wrapper = mountChat()
    await flushPromises()

    expect(wrapper.find('[data-testid="pm-failed-partial"]').exists()).toBe(false)
    expect(wrapper.text()).toContain('连接中断')
    expect(wrapper.text()).toContain('重试')
    expect(wrapper.text()).not.toContain('未知错误')
    wrapper.unmount()
  })

  it('orphan / getPmDraft failure converges to connection (not unknown)', async () => {
    apiMocks.listPmThreads.mockResolvedValue({ items: [{ id: 'thr-1', title: 't' }] })
    apiMocks.listPmMessages.mockResolvedValue({
      items: [{ id: 'u1', role: 'user', content: 'q', status: 'ok' }],
    })
    apiMocks.getPmDraft.mockRejectedValue(new Error('draft unavailable'))
    apiMocks.patchPmMessage.mockResolvedValue({ id: 'u1', status: 'failed', failKind: 'connection' })

    const wrapper = mountChat()
    await flushPromises()

    expect(apiMocks.patchPmMessage).toHaveBeenCalledWith(
      'proj-1',
      'thr-1',
      'u1',
      expect.objectContaining({ status: 'failed', failKind: 'connection' }),
    )
    expect(wrapper.text()).toContain('连接中断')
    expect(wrapper.text()).not.toContain('未知错误')
    wrapper.unmount()
  })

  it('Retry clears failed partial bubble and starts a new turn (S4)', async () => {
    apiMocks.listPmThreads.mockResolvedValue({ items: [{ id: 'thr-1', title: 't' }] })
    apiMocks.listPmMessages.mockResolvedValue({
      items: [{ id: 'u1', role: 'user', content: '项目整体进度如何？', status: 'ok' }],
    })
    apiMocks.getPmDraft.mockResolvedValue({
      draft: {
        id: 'd1',
        threadId: 'thr-1',
        userMsgId: 'u1',
        partialText: '半成品内容',
        chunkIndex: 1,
        eventSeq: 0,
        status: 'failed',
        failKind: 'connection',
      },
      live: false,
      hasFinal: false,
    })
    apiMocks.patchPmMessage.mockImplementation(async (_p, _t, _m, body: { status?: string; failKind?: string }) => ({
      id: 'u1',
      status: body.status || 'ok',
      failKind: body.failKind || '',
    }))
    apiMocks.ensurePmSandbox.mockResolvedValue({
      sandbox: { id: 1, status: 'running' },
      preamble: '',
    })
    apiMocks.getSandbox.mockResolvedValue({ id: 1, status: 'running' })

    const wrapper = mountChat()
    await flushPromises()

    expect(wrapper.find('[data-testid="pm-failed-partial"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('半成品内容')

    const retry = wrapper.find('[data-testid="pm-fail-retry"]')
    expect(retry.exists()).toBe(true)
    await retry.trigger('click')
    await flushPromises()

    expect(wrapper.find('[data-testid="pm-failed-partial"]').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('半成品内容')
    expect(wrapper.text()).not.toContain('连接中断')
    expect(apiMocks.pmThreadChatWsUrl).toHaveBeenCalled()
    expect(wrapper.find('[data-testid="pm-stream-bubble"]').exists()).toBe(true)
    wrapper.unmount()
  })
})

describe('PmLeaderChat loading states S1–S5', () => {
  const originalWS = globalThis.WebSocket

  beforeEach(() => {
    vi.clearAllMocks()
    localStorage.clear()
    // @ts-expect-error test stub
    globalThis.WebSocket = MockWebSocket
    apiMocks.ensurePmSandbox.mockResolvedValue({
      sandbox: { id: 1, status: 'running' },
      preamble: '',
    })
    apiMocks.getSandbox.mockResolvedValue({ id: 1, status: 'running' })
    apiMocks.getPmDraft.mockResolvedValue({ draft: null, live: false, hasFinal: false })
    apiMocks.patchPmMessage.mockResolvedValue({ id: 'u1', status: 'failed', failKind: 'connection' })
  })

  afterEach(() => {
    globalThis.WebSocket = originalWS
  })

  it('S1: mount with history does not flash emptyHint or suggestions', async () => {
    let resolveMessages: (v: { items: unknown[] }) => void = () => {}
    const messagesPromise = new Promise<{ items: unknown[] }>((r) => {
      resolveMessages = r
    })
    apiMocks.listPmThreads.mockResolvedValue({ items: [{ id: 'thr-1', title: '历史会话' }] })
    apiMocks.listPmMessages.mockReturnValue(messagesPromise)
    apiMocks.getPmDraft.mockResolvedValue({ draft: null, live: false, hasFinal: false })

    const wrapper = mountChat()
    await flushPromises()

    expect(wrapper.find('[data-testid="pm-messages-loading"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('正在加载历史对话')
    expect(wrapper.text()).not.toContain('询问项目整体进度')
    expect(wrapper.text()).not.toContain('项目整体进度如何？')

    resolveMessages({
      items: [
        { id: 'u1', role: 'user', content: 'hello', status: 'ok', createdAt: '2026-01-01T00:00:00Z' },
        { id: 'a1', role: 'assistant', content: 'world', createdAt: '2026-01-01T00:01:00Z' },
      ],
    })
    await flushPromises()

    expect(wrapper.find('[data-testid="pm-messages-loading"]').exists()).toBe(false)
    expect(wrapper.text()).toContain('hello')
    expect(wrapper.text()).toContain('world')
    expect(wrapper.find('[data-testid="pm-idle-suggestions"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('项目整体进度如何？')
    // chips are column-level siblings (pl-[38px]), not nested inside bubble-wrap
    const chips = wrapper.find('[data-testid="pm-idle-suggestions"]')
    expect(chips.classes()).toContain('pl-[38px]')
    expect(chips.element.parentElement?.classList.contains('max-w-[85%]')).toBe(false)
    wrapper.unmount()
  })

  it('idle suggestions hidden when last message is failed user (not assistant)', async () => {
    apiMocks.listPmThreads.mockResolvedValue({ items: [{ id: 'thr-1', title: 't' }] })
    apiMocks.listPmMessages.mockResolvedValue({
      items: [
        { id: 'u0', role: 'user', content: 'prev', status: 'ok', createdAt: '2026-01-01T00:00:00Z' },
        { id: 'a0', role: 'assistant', content: 'earlier reply', createdAt: '2026-01-01T00:01:00Z' },
        { id: 'u1', role: 'user', content: 'q', status: 'failed', failKind: 'connection', createdAt: '2026-01-01T00:02:00Z' },
      ],
    })
    apiMocks.getPmDraft.mockResolvedValue({ draft: null, live: false, hasFinal: false })

    const wrapper = mountChat()
    await flushPromises()

    expect(wrapper.text()).toContain('earlier reply')
    expect(wrapper.find('[data-testid="pm-fail-retry"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="pm-idle-suggestions"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('copy full text copies assistant .md innerText and toasts success', async () => {
    apiMocks.listPmThreads.mockResolvedValue({ items: [{ id: 'thr-1', title: 't' }] })
    apiMocks.listPmMessages.mockResolvedValue({
      items: [
        { id: 'u1', role: 'user', content: 'hello', status: 'ok', createdAt: '2026-01-01T00:00:00Z' },
        { id: 'a1', role: 'assistant', content: '**bold** reply', createdAt: '2026-01-01T00:01:00Z' },
      ],
    })
    apiMocks.getPmDraft.mockResolvedValue({ draft: null, live: false, hasFinal: false })
    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.defineProperty(navigator, 'clipboard', {
      configurable: true,
      value: { writeText },
    })

    const wrapper = mountChat()
    await flushPromises()

    const copyBtn = wrapper.findAll('button').find((b) => b.text().includes('复制全文'))
    expect(copyBtn).toBeTruthy()
    await copyBtn!.trigger('click')
    await flushPromises()

    expect(writeText).toHaveBeenCalled()
    const copied = writeText.mock.calls[0][0] as string
    expect(copied.length).toBeGreaterThan(0)
    wrapper.unmount()
  })

  it('S2: switching threads clears old messages and shows loadingHistory', async () => {
    apiMocks.listPmThreads.mockResolvedValue({
      items: [
        { id: 'thr-a', title: '会话A' },
        { id: 'thr-b', title: '会话B' },
      ],
    })
    apiMocks.listPmMessages.mockImplementation(async (_p, tid) => {
      if (tid === 'thr-a') {
        return { items: [{ id: 'u1', role: 'user', content: 'A-only', status: 'ok' }] }
      }
      await new Promise((r) => setTimeout(r, 30))
      return { items: [{ id: 'u2', role: 'user', content: 'B-only', status: 'ok' }] }
    })

    const wrapper = mountChat()
    await flushPromises()
    expect(wrapper.text()).toContain('A-only')

    const btnB = wrapper.findAll('aside button').find((b) => b.text().includes('会话B'))
    expect(btnB).toBeTruthy()
    await btnB!.trigger('click')
    await flushPromises()

    expect(wrapper.text()).not.toContain('A-only')
    expect(wrapper.find('[data-testid="pm-messages-loading"]').exists()).toBe(true)

    await vi.waitFor(() => {
      expect(wrapper.text()).toContain('B-only')
    })
    expect(wrapper.find('[data-testid="pm-messages-loading"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('S4: listPmMessages reject shows EmptyState and retry succeeds', async () => {
    apiMocks.listPmThreads.mockResolvedValue({ items: [{ id: 'thr-1', title: 't' }] })
    apiMocks.listPmMessages
      .mockRejectedValueOnce(new Error('network down'))
      .mockResolvedValueOnce({
        items: [{ id: 'u1', role: 'user', content: 'recovered', status: 'ok' }],
      })

    const wrapper = mountChat()
    await flushPromises()

    expect(wrapper.find('[data-testid="pm-messages-load-failed"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('加载失败')
    expect(wrapper.text()).not.toContain('询问项目整体进度')

    const retryBtn = wrapper.find('[data-testid="pm-messages-load-failed"] button')
    expect(retryBtn.exists()).toBe(true)
    await retryBtn.trigger('click')
    await flushPromises()

    expect(wrapper.find('[data-testid="pm-messages-load-failed"]').exists()).toBe(false)
    expect(wrapper.text()).toContain('recovered')
    wrapper.unmount()
  })
})

describe('PmLeaderChat loading states S3/S5 and race boundaries', () => {
  const originalWS = globalThis.WebSocket

  beforeEach(() => {
    vi.clearAllMocks()
    localStorage.clear()
    // @ts-expect-error test stub
    globalThis.WebSocket = MockWebSocket
    apiMocks.listPmThreads.mockResolvedValue({ items: [{ id: 'thr-1', title: 't' }] })
    apiMocks.listPmMessages.mockResolvedValue({ items: [] })
    apiMocks.getPmDraft.mockResolvedValue({ draft: null, live: false, hasFinal: false })
    apiMocks.appendPmMessage.mockImplementation(async (_pid: string, _tid: string, body: { content?: string; role?: string }) => ({
      id: 'msg-u1',
      role: body.role || 'user',
      content: body.content || '',
      status: 'ok',
    }))
    apiMocks.ensurePmSandbox.mockResolvedValue({
      sandbox: { id: 1, status: 'running' },
      preamble: '',
    })
    apiMocks.getSandbox.mockResolvedValue({ id: 1, status: 'running' })
    apiMocks.patchPmMessage.mockResolvedValue({ id: 'u1', status: 'failed', failKind: 'unknown' })
  })

  afterEach(() => {
    globalThis.WebSocket = originalWS
  })

  it('S3: turn_done keeps partial and finalizing meta until refetch completes', async () => {
    apiMocks.listPmMessages.mockResolvedValue({
      items: [{ id: 'u1', role: 'user', content: '?', status: 'ok' }],
    })

    let resolveRefetch: (v: { items: unknown[] }) => void = () => {}
    const refetchPromise = new Promise<{ items: unknown[] }>((r) => {
      resolveRefetch = r
    })

    let socket: MockWebSocket | null = null
    class CaptureWS extends MockWebSocket {
      constructor() {
        super()
        socket = this
      }
    }
    // @ts-expect-error test stub
    globalThis.WebSocket = CaptureWS

    const wrapper = mountChat()
    await flushPromises()

    const textarea = wrapper.find('textarea')
    await textarea.setValue('进度？')
    const sendBtn = wrapper.findAll('button').find((b) => b.text() === '发送')
    await sendBtn!.trigger('click')
    await flushPromises()

    const chunk = (text: string) => ({
      op: 'event',
      data: {
        type: 'session_update',
        update: {
          sessionUpdate: 'agent_message_chunk',
          content: { type: 'text', text },
        },
      },
    })

    socket!.onmessage?.(
      new MessageEvent('message', {
        data: JSON.stringify({ type: 'acp', data: chunk('Partial '), seq: 0 }),
      }),
    )
    await flushPromises()
    expect(wrapper.text()).toContain('Partial ')

    apiMocks.listPmMessages.mockReturnValueOnce(refetchPromise)
    socket!.onmessage?.(
      new MessageEvent('message', { data: JSON.stringify({ type: 'turn_done', seq: 1 }) }),
    )
    await flushPromises()

    expect(wrapper.text()).toContain('Partial ')
    expect(wrapper.find('[data-testid="pm-stream-finalizing-meta"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('正在保存回复')

    resolveRefetch({
      items: [
        { id: 'u1', role: 'user', content: '进度？', status: 'ok' },
        { id: 'a1', role: 'assistant', content: 'Partial answer done' },
      ],
    })
    await flushPromises()

    expect(wrapper.find('[data-testid="pm-stream-bubble"]').exists()).toBe(false)
    expect(wrapper.text()).toContain('Partial answer done')
    wrapper.unmount()
  })

  it('S1: messagesLoading does not show Stop button or turn busy hint', async () => {
    let resolveMessages: (v: { items: unknown[] }) => void = () => {}
    const messagesPromise = new Promise<{ items: unknown[] }>((r) => {
      resolveMessages = r
    })
    apiMocks.listPmThreads.mockResolvedValue({ items: [{ id: 'thr-1', title: '历史会话' }] })
    apiMocks.listPmMessages.mockReturnValue(messagesPromise)

    const wrapper = mountChat()
    await flushPromises()

    expect(wrapper.find('[data-testid="pm-messages-loading"]').exists()).toBe(true)
    expect(wrapper.text()).not.toContain('停止')
    expect(wrapper.text()).not.toContain('正在处理本轮')

    resolveMessages({ items: [{ id: 'u1', role: 'user', content: 'hello', status: 'ok' }] })
    await flushPromises()
    wrapper.unmount()
  })

  it('S3: turn_done refetch failure keeps partial and allows retry', async () => {
    apiMocks.listPmMessages.mockResolvedValue({
      items: [{ id: 'u1', role: 'user', content: '?', status: 'ok' }],
    })

    let socket: MockWebSocket | null = null
    class CaptureWS extends MockWebSocket {
      constructor() {
        super()
        socket = this
      }
    }
    // @ts-expect-error test stub
    globalThis.WebSocket = CaptureWS

    const wrapper = mountChat()
    await flushPromises()

    const textarea = wrapper.find('textarea')
    await textarea.setValue('进度？')
    const sendBtn = wrapper.findAll('button').find((b) => b.text() === '发送')
    await sendBtn!.trigger('click')
    await flushPromises()

    socket!.onmessage?.(
      new MessageEvent('message', {
        data: JSON.stringify({
          type: 'acp',
          data: {
            op: 'event',
            data: {
              type: 'session_update',
              update: {
                sessionUpdate: 'agent_message_chunk',
                content: { type: 'text', text: 'Partial ' },
              },
            },
          },
          seq: 0,
        }),
      }),
    )
    await flushPromises()

    apiMocks.listPmMessages.mockRejectedValueOnce(new Error('network down'))
    socket!.onmessage?.(
      new MessageEvent('message', { data: JSON.stringify({ type: 'turn_done', seq: 1 }) }),
    )
    await flushPromises()

    expect(wrapper.text()).toContain('Partial ')
    expect(wrapper.find('[data-testid="pm-stream-finalizing-meta"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="pm-finalizing-retry"]').exists()).toBe(true)

    apiMocks.listPmMessages.mockResolvedValueOnce({
      items: [
        { id: 'u1', role: 'user', content: '进度？', status: 'ok' },
        { id: 'a1', role: 'assistant', content: 'Partial answer done' },
      ],
    })
    await wrapper.find('[data-testid="pm-finalizing-retry"]').trigger('click')
    await flushPromises()

    expect(wrapper.find('[data-testid="pm-stream-bubble"]').exists()).toBe(false)
    expect(wrapper.text()).toContain('Partial answer done')
    wrapper.unmount()
  })

  it('S5: live draft hydrate shows loading then partial with resuming meta', async () => {
    let resolveMessages: (v: { items: unknown[] }) => void = () => {}
    const messagesPromise = new Promise<{ items: unknown[] }>((r) => {
      resolveMessages = r
    })
    apiMocks.listPmMessages.mockReturnValue(messagesPromise)
    apiMocks.getPmDraft.mockResolvedValue({
      draft: {
        id: 'd1',
        threadId: 'thr-1',
        userMsgId: 'u1',
        partialText: 'partial…',
        chunkIndex: 2,
        eventSeq: 1,
        status: 'streaming',
      },
      live: true,
      hasFinal: false,
    })

    const wrapper = mountChat()
    await flushPromises()

    expect(wrapper.find('[data-testid="pm-messages-loading"]').exists()).toBe(true)
    expect(wrapper.text()).not.toContain('partial…')

    resolveMessages({ items: [{ id: 'u1', role: 'user', content: 'q', status: 'ok' }] })
    await flushPromises()

    expect(wrapper.find('[data-testid="pm-messages-loading"]').exists()).toBe(false)
    expect(wrapper.text()).toContain('partial…')
    expect(wrapper.find('[data-testid="pm-stream-resuming-meta"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('正在续接回复')
    wrapper.unmount()
  })

  it('finalizing takes priority over messagesLoading pane', async () => {
    apiMocks.listPmMessages.mockResolvedValue({
      items: [{ id: 'u1', role: 'user', content: '?', status: 'ok' }],
    })

    let socket: MockWebSocket | null = null
    class CaptureWS extends MockWebSocket {
      constructor() {
        super()
        socket = this
      }
    }
    // @ts-expect-error test stub
    globalThis.WebSocket = CaptureWS

    const wrapper = mountChat()
    await flushPromises()

    const textarea = wrapper.find('textarea')
    await textarea.setValue('x')
    await wrapper.findAll('button').find((b) => b.text() === '发送')!.trigger('click')
    await flushPromises()

    socket!.onmessage?.(
      new MessageEvent('message', {
        data: JSON.stringify({
          type: 'acp',
          data: {
            op: 'event',
            data: {
              type: 'session_update',
              update: {
                sessionUpdate: 'agent_message_chunk',
                content: { type: 'text', text: 'keep' },
              },
            },
          },
          seq: 0,
        }),
      }),
    )
    await flushPromises()

    let resolveSlowRefetch: (v: { items: unknown[] }) => void = () => {}
    apiMocks.listPmMessages.mockReturnValueOnce(
      new Promise((r) => {
        resolveSlowRefetch = r
      }),
    )
    socket!.onmessage?.(
      new MessageEvent('message', { data: JSON.stringify({ type: 'turn_done', seq: 1 }) }),
    )
    await flushPromises()

    expect(wrapper.find('[data-testid="pm-messages-loading"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="pm-stream-finalizing-meta"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('keep')

    resolveSlowRefetch({
      items: [
        { id: 'u1', role: 'user', content: 'x', status: 'ok' },
        { id: 'a1', role: 'assistant', content: 'done' },
      ],
    })
    await flushPromises()
    wrapper.unmount()
  })

  it('rapid thread switch drops stale listPmMessages response', async () => {
    apiMocks.listPmThreads.mockResolvedValue({
      items: [
        { id: 'thr-a', title: 'A' },
        { id: 'thr-b', title: 'B' },
      ],
    })

    const pending: Record<string, { resolve: (v: { items: unknown[] }) => void }> = {}
    apiMocks.listPmMessages.mockImplementation((_p, tid) => {
      return new Promise((resolve) => {
        pending[tid] = { resolve }
      })
    })

    const wrapper = mountChat()
    await flushPromises()

    const btnB = wrapper.findAll('aside button').find((b) => b.text().includes('B'))
    await btnB!.trigger('click')
    await flushPromises()

    pending['thr-a']?.resolve({ items: [{ id: 'stale', role: 'user', content: 'STALE', status: 'ok' }] })
    await flushPromises()
    expect(wrapper.text()).not.toContain('STALE')

    pending['thr-b']?.resolve({ items: [{ id: 'fresh', role: 'user', content: 'FRESH', status: 'ok' }] })
    await flushPromises()
    expect(wrapper.text()).toContain('FRESH')
    wrapper.unmount()
  })
})

describe('PmLeaderChat i18n locale switch', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    localStorage.clear()
    apiMocks.listPmThreads.mockResolvedValue({ items: [{ id: 'thr-1', title: 't' }] })
    apiMocks.listPmMessages.mockRejectedValue(new Error('fail'))
    apiMocks.getPmDraft.mockResolvedValue({ draft: null, live: false, hasFinal: false })
  })

  it('shows en loading/error strings without raw keys', async () => {
    const enPages = await import('@/locales/en/pages.json')
    const enCommon = await import('@/locales/en/common.json')
    const i18n = createI18n({
      legacy: false,
      locale: 'en',
      messages: { en: { ...enCommon.default, ...enPages.default } },
    })
    const wrapper = mount(PmLeaderChat, {
      props: { projectId: 'proj-1', binding: { enabled: true, agentAvailable: true, agentConfigRef: 'a', aclNote: '' } },
      global: { plugins: [i18n], stubs: { Icon: true, CitationCard: true } },
    })
    await flushPromises()

    expect(wrapper.text()).toContain('Failed to load messages')
    expect(wrapper.text()).not.toContain('pages.projectDetail.pm.loadFailed')
    wrapper.unmount()
  })
})

describe('PmLeaderChat settings entry', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    apiMocks.listPmThreads.mockResolvedValue({ items: [] })
    apiMocks.listPmMessages.mockResolvedValue({ items: [] })
    apiMocks.getPmDraft.mockResolvedValue({ draft: null, live: false, hasFinal: false })
  })

  it('emits openSettings from enabled gear+settings button', async () => {
    const wrapper = mountChat()
    await flushPromises()
    const btn = wrapper.find('[data-testid="pm-chat-open-settings"]')
    expect(btn.exists()).toBe(true)
    expect(btn.text()).toContain('设置')
    await btn.trigger('click')
    expect(wrapper.emitted('openSettings')).toHaveLength(1)
    wrapper.unmount()
  })

  it('emits openSettings from disabled empty-state CTA', async () => {
    const wrapper = mountChat({
      enabled: false,
      agentAvailable: false,
      agentConfigRef: '',
      aclNote: '',
    })
    await flushPromises()
    expect(wrapper.text()).toContain('需先在本页设置中绑定')
    const go = wrapper.findAll('button').find((b) => b.text().includes('前往设置'))
    expect(go).toBeTruthy()
    await go!.trigger('click')
    expect(wrapper.emitted('openSettings')).toHaveLength(1)
    wrapper.unmount()
  })
})

describe('PmLeaderChat stick-to-bottom', () => {
  const originalWS = globalThis.WebSocket

  function mockScrollerMetrics(
    el: HTMLElement,
    scrollHeight: number,
    clientHeight: number,
    scrollTop?: number,
  ) {
    let top = scrollTop ?? scrollHeight - clientHeight
    Object.defineProperty(el, 'scrollHeight', { configurable: true, get: () => scrollHeight })
    Object.defineProperty(el, 'clientHeight', { configurable: true, get: () => clientHeight })
    Object.defineProperty(el, 'scrollTop', {
      configurable: true,
      get: () => top,
      set: (v: number) => {
        top = v
      },
    })
    return {
      getScrollTop: () => top,
      setScrollTop: (v: number) => {
        top = v
      },
      syncStickFromScroll: () => {
        el.dispatchEvent(new Event('scroll'))
      },
    }
  }

  beforeEach(() => {
    vi.clearAllMocks()
    localStorage.clear()
    // @ts-expect-error test stub
    globalThis.WebSocket = MockWebSocket
    apiMocks.listPmThreads.mockResolvedValue({ items: [{ id: 'thr-1', title: 't' }] })
    apiMocks.listPmMessages.mockResolvedValue({ items: [] })
    apiMocks.getPmDraft.mockResolvedValue({ draft: null, live: false, hasFinal: false })
    apiMocks.appendPmMessage.mockImplementation(async (_pid: string, _tid: string, body: { content?: string; role?: string }) => ({
      id: 'msg-u1',
      role: body.role || 'user',
      content: body.content || '',
      status: 'ok',
    }))
    apiMocks.ensurePmSandbox.mockResolvedValue({
      sandbox: { id: 1, status: 'running' },
      preamble: '',
    })
    apiMocks.getSandbox.mockResolvedValue({ id: 1, status: 'running' })
  })

  afterEach(() => {
    globalThis.WebSocket = originalWS
  })

  it('handleAcp and refetchAfterTurnDone do not scroll when user is away from bottom', async () => {
    let socket: MockWebSocket | null = null
    class CaptureWS extends MockWebSocket {
      constructor() {
        super()
        socket = this
      }
    }
    // @ts-expect-error test stub
    globalThis.WebSocket = CaptureWS

    const wrapper = mountChat()
    await flushPromises()

    const scrollerEl = wrapper.find('[data-testid="pm-message-scroller"]').element as HTMLElement
    const metrics = mockScrollerMetrics(scrollerEl, 1000, 200, 100)

    const textarea = wrapper.find('textarea')
    await textarea.setValue('进度？')
    const sendBtn = wrapper.findAll('button').find((b) => b.text() === '发送')
    await sendBtn!.trigger('click')
    await flushPromises()

    metrics.setScrollTop(100)
    metrics.syncStickFromScroll()
    await flushPromises()
    const awayTop = metrics.getScrollTop()

    const chunk = (text: string) => ({
      op: 'event',
      data: {
        type: 'session_update',
        update: {
          sessionUpdate: 'agent_message_chunk',
          content: { type: 'text', text },
        },
      },
    })

    socket!.onmessage?.(
      new MessageEvent('message', {
        data: JSON.stringify({ type: 'acp', data: chunk('Hello'), seq: 0 }),
      }),
    )
    await flushPromises()
    expect(metrics.getScrollTop()).toBe(awayTop)

    apiMocks.listPmMessages.mockResolvedValue({
      items: [
        { id: 'u1', role: 'user', content: '进度？', status: 'ok' },
        { id: 'a1', role: 'assistant', content: 'Hello', status: 'ok' },
      ],
    })
    socket!.onmessage?.(
      new MessageEvent('message', {
        data: JSON.stringify({ type: 'turn_done', seq: 1 }),
      }),
    )
    await flushPromises()
    expect(metrics.getScrollTop()).toBe(awayTop)
    wrapper.unmount()
  })

  it('send and loadMessages force scroll to bottom', async () => {
    apiMocks.listPmThreads.mockResolvedValue({
      items: [
        { id: 'thr-1', title: '会话1' },
        { id: 'thr-2', title: '会话2' },
      ],
    })
    apiMocks.listPmMessages.mockImplementation(async (_pid: string, tid: string) => ({
      items: [{ id: `u-${tid}`, role: 'user', content: `msg-${tid}`, status: 'ok' }],
    }))

    const wrapper = mountChat()
    await flushPromises()

    const scrollerEl = wrapper.find('[data-testid="pm-message-scroller"]').element as HTMLElement
    mockScrollerMetrics(scrollerEl, 1000, 200, 100)

    const btn2 = wrapper.findAll('aside button').find((b) => b.text().includes('会话2'))
    expect(btn2).toBeTruthy()
    await btn2!.trigger('click')
    await flushPromises()
    expect(scrollerEl.scrollTop).toBe(scrollerEl.scrollHeight)

    mockScrollerMetrics(scrollerEl, 1000, 200, 100)
    const textarea = wrapper.find('textarea')
    await textarea.setValue('强制滚底')
    const sendBtn = wrapper.findAll('button').find((b) => b.text() === '发送')
    await sendBtn!.trigger('click')
    await flushPromises()
    expect(scrollerEl.scrollTop).toBe(scrollerEl.scrollHeight)
    wrapper.unmount()
  })

  it('QQ channel threads show dual QQ tags, readonly bar, no delete/send', async () => {
    apiMocks.listPmThreads.mockResolvedValue({
      items: [
        {
          id: 'thr-qq',
          title: '产品评审同步',
          userId: 'qq:guild:ch1',
          projectId: 'proj-1',
          createdAt: '2026-01-02T00:00:00Z',
          updatedAt: '2026-01-02T12:00:00Z',
        },
        {
          id: 'thr-web',
          title: '对接发布流程',
          userId: 'alice',
          projectId: 'proj-1',
          createdAt: '2026-01-01T00:00:00Z',
          updatedAt: '2026-01-01T12:00:00Z',
        },
      ],
    })
    apiMocks.listPmMessages.mockResolvedValue({
      items: [{ id: 'm1', role: 'user', content: '[来自 Alice] 下周评审还开吗？', status: 'ok' }],
    })

    const wrapper = mountChat()
    await flushPromises()

    expect(wrapper.findAll('[data-testid="pm-qq-tag"]').length).toBe(1)
    const channelBtn = wrapper.find('[data-channel="1"]')
    expect(channelBtn.exists()).toBe(true)
    expect(channelBtn.find('[data-testid="pm-thread-delete"]').exists()).toBe(false)
    expect(wrapper.find('[data-channel="0"] [data-testid="pm-thread-delete"]').exists()).toBe(true)

    // Active defaults to first (channel): header tag + readonly composer.
    expect(wrapper.find('[data-testid="pm-qq-tag-header"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="pm-channel-readonly"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="pm-chat-send"]').exists()).toBe(false)
    expect(wrapper.text()).toContain('来自 QQ，请在 QQ 侧回复')
    expect(wrapper.text()).toContain('渠道会话在 Web 只读：不可发送、不可删除')
    expect(apiMocks.appendPmMessage).not.toHaveBeenCalled()

    // Web thread has send composer and no QQ header tag.
    await wrapper.find('[data-channel="0"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="pm-qq-tag-header"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="pm-channel-readonly"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="pm-chat-send"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('channel context menu opens detail with untitled placeholder', async () => {
    apiMocks.listPmThreads.mockResolvedValue({
      items: [
        {
          id: 'thr-qq-empty',
          title: '   ',
          userId: 'qq:c2c:u1',
          projectId: 'proj-1',
          createdAt: '2026-01-02T00:00:00Z',
          updatedAt: '2026-01-02T12:00:00Z',
        },
        {
          id: 'thr-web',
          title: 'Web',
          userId: 'alice',
          projectId: 'proj-1',
          createdAt: '2026-01-01T00:00:00Z',
          updatedAt: '2026-01-01T12:00:00Z',
        },
      ],
    })
    apiMocks.listPmMessages.mockResolvedValue({ items: [] })

    const wrapper = mountChat()
    await flushPromises()

    expect(wrapper.find('[data-channel="1"]').text()).toContain('未命名会话')
    expect(wrapper.find('[data-testid="pm-chat-title"]').text()).toContain('未命名会话')

    await wrapper.find('[data-channel="0"]').trigger('contextmenu')
    await flushPromises()
    expect(wrapper.find('[data-testid="pm-channel-ctx-menu"]').exists()).toBe(false)

    await wrapper.find('[data-channel="1"]').trigger('contextmenu')
    await flushPromises()
    const menu = wrapper.find('[data-testid="pm-channel-ctx-menu"]')
    expect(menu.exists()).toBe(true)
    expect(menu.text()).toContain('查看详情')
    expect(menu.text()).not.toContain('删除')

    await wrapper.find('[data-testid="pm-channel-ctx-detail"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="pm-channel-detail-title"]').text()).toBe('未命名会话')
    expect(wrapper.find('[data-testid="pm-channel-detail-source"]').text()).toBe('来自 QQ Channel')
    wrapper.unmount()
  })

  it('scrolling back within 48px restores stick and following acp scrolls', async () => {
    let socket: MockWebSocket | null = null
    class CaptureWS extends MockWebSocket {
      constructor() {
        super()
        socket = this
      }
    }
    // @ts-expect-error test stub
    globalThis.WebSocket = CaptureWS

    const wrapper = mountChat()
    await flushPromises()

    const scrollerEl = wrapper.find('[data-testid="pm-message-scroller"]').element as HTMLElement
    mockScrollerMetrics(scrollerEl, 1000, 200, 100)

    const textarea = wrapper.find('textarea')
    await textarea.setValue('恢复跟随')
    const sendBtn = wrapper.findAll('button').find((b) => b.text() === '发送')
    await sendBtn!.trigger('click')
    await flushPromises()

    mockScrollerMetrics(scrollerEl, 1000, 200, 100)
    scrollerEl.dispatchEvent(new Event('scroll'))
    await flushPromises()

    const metrics = mockScrollerMetrics(scrollerEl, 1000, 200, 760)
    metrics.syncStickFromScroll()
    await flushPromises()

    mockScrollerMetrics(scrollerEl, 1200, 200, 760)

    const chunk = (text: string) => ({
      op: 'event',
      data: {
        type: 'session_update',
        update: {
          sessionUpdate: 'agent_message_chunk',
          content: { type: 'text', text },
        },
      },
    })

    socket!.onmessage?.(
      new MessageEvent('message', {
        data: JSON.stringify({ type: 'acp', data: chunk('More'), seq: 0 }),
      }),
    )
    await flushPromises()

    expect(scrollerEl.scrollTop).toBe(scrollerEl.scrollHeight)
    wrapper.unmount()
  })
})

describe('PmLeaderChat tail window + lazyload', () => {
  const originalWS = globalThis.WebSocket

  function mockScrollerMetrics(
    el: HTMLElement,
    scrollHeight: number,
    clientHeight: number,
    scrollTop?: number,
  ) {
    let top = scrollTop ?? scrollHeight - clientHeight
    let height = scrollHeight
    Object.defineProperty(el, 'scrollHeight', {
      configurable: true,
      get: () => height,
      set: (v: number) => {
        height = v
      },
    })
    Object.defineProperty(el, 'clientHeight', { configurable: true, get: () => clientHeight })
    Object.defineProperty(el, 'scrollTop', {
      configurable: true,
      get: () => top,
      set: (v: number) => {
        top = v
      },
    })
    return {
      getScrollTop: () => top,
      setScrollTop: (v: number) => {
        top = v
      },
      setScrollHeight: (v: number) => {
        height = v
      },
      syncStickFromScroll: () => {
        el.dispatchEvent(new Event('scroll'))
      },
    }
  }

  beforeEach(() => {
    vi.clearAllMocks()
    localStorage.clear()
    // @ts-expect-error test stub
    globalThis.WebSocket = MockWebSocket
    apiMocks.getPmDraft.mockResolvedValue({ draft: null, live: false, hasFinal: false })
    apiMocks.appendPmMessage.mockImplementation(async (_pid: string, _tid: string, body: { content?: string; role?: string }) => ({
      id: 'msg-u1',
      role: body.role || 'user',
      content: body.content || '',
      status: 'ok',
    }))
    apiMocks.ensurePmSandbox.mockResolvedValue({
      sandbox: { id: 1, status: 'running' },
      preamble: '',
    })
    apiMocks.getSandbox.mockResolvedValue({ id: 1, status: 'running' })
  })

  afterEach(() => {
    globalThis.WebSocket = originalWS
  })

  it('non-Channel first load requests limit=20 tail window and shows scroll-up tip', async () => {
    const tail = Array.from({ length: 20 }, (_, i) => ({
      id: `m-${i + 5}`,
      role: i % 2 === 0 ? 'user' : 'assistant',
      content: `msg-${i + 5}`,
      status: 'ok',
    }))
    apiMocks.listPmThreads.mockResolvedValue({
      items: [{ id: 'thr-long', title: '长会话', userId: 'alice' }],
    })
    apiMocks.listPmMessages.mockResolvedValue({ items: tail, hasMore: true })

    const wrapper = mountChat()
    await flushPromises()

    expect(apiMocks.listPmMessages).toHaveBeenCalledWith('proj-1', 'thr-long', { limit: 20 })
    expect(wrapper.findAll('[data-msg-id]').length).toBe(20)
    const tip = wrapper.find('[data-testid="pm-history-tip"]')
    expect(tip.exists()).toBe(true)
    expect(tip.text()).toContain('上滑加载更早')
    wrapper.unmount()
  })

  it('Channel first load keeps full list without limit/lazyload tip', async () => {
    const items = Array.from({ length: 30 }, (_, i) => ({
      id: `c-${i}`,
      role: 'user',
      content: `qq-${i}`,
      status: 'ok',
    }))
    apiMocks.listPmThreads.mockResolvedValue({
      items: [
        {
          id: 'thr-qq',
          title: 'QQ会话',
          userId: 'qq:guild:1',
          projectId: 'proj-1',
        },
      ],
    })
    apiMocks.listPmMessages.mockResolvedValue({ items })

    const wrapper = mountChat()
    await flushPromises()

    expect(apiMocks.listPmMessages).toHaveBeenCalledWith('proj-1', 'thr-qq')
    expect(apiMocks.listPmMessages.mock.calls[0].length).toBe(2)
    expect(wrapper.find('[data-testid="pm-history-tip"]').exists()).toBe(false)
    expect(wrapper.findAll('[data-msg-id]').length).toBe(30)
    wrapper.unmount()
  })

  it('scroll near top lazyloads earlier page with before=oldest id', async () => {
    const tail = Array.from({ length: 20 }, (_, i) => ({
      id: `m-${i + 20}`,
      role: 'user',
      content: `t-${i + 20}`,
      status: 'ok',
    }))
    const earlier = Array.from({ length: 20 }, (_, i) => ({
      id: `m-${i}`,
      role: 'user',
      content: `e-${i}`,
      status: 'ok',
    }))
    apiMocks.listPmThreads.mockResolvedValue({
      items: [{ id: 'thr-long', title: '长会话', userId: 'alice' }],
    })
    apiMocks.listPmMessages
      .mockResolvedValueOnce({ items: tail, hasMore: true })
      .mockResolvedValueOnce({ items: earlier, hasMore: false })

    const wrapper = mountChat()
    await flushPromises()

    const scrollerEl = wrapper.find('[data-testid="pm-message-scroller"]').element as HTMLElement
    const metrics = mockScrollerMetrics(scrollerEl, 2000, 400, 40)
    metrics.syncStickFromScroll()
    await flushPromises()

    expect(apiMocks.listPmMessages).toHaveBeenLastCalledWith('proj-1', 'thr-long', {
      limit: 20,
      before: 'm-20',
    })
    expect(wrapper.findAll('[data-msg-id]').length).toBe(40)
    expect(wrapper.find('[data-testid="pm-history-tip"]').text()).toContain('已到最早')
    wrapper.unmount()
  })

  it('short session hasMore=false does not request earlier on scroll-top', async () => {
    const items = Array.from({ length: 8 }, (_, i) => ({
      id: `s-${i}`,
      role: 'user',
      content: `s-${i}`,
      status: 'ok',
    }))
    apiMocks.listPmThreads.mockResolvedValue({
      items: [{ id: 'thr-short', title: '短会话', userId: 'alice' }],
    })
    apiMocks.listPmMessages.mockResolvedValue({ items, hasMore: false })

    const wrapper = mountChat()
    await flushPromises()
    expect(apiMocks.listPmMessages).toHaveBeenCalledTimes(1)

    const scrollerEl = wrapper.find('[data-testid="pm-message-scroller"]').element as HTMLElement
    const metrics = mockScrollerMetrics(scrollerEl, 800, 400, 0)
    metrics.syncStickFromScroll()
    await flushPromises()

    expect(apiMocks.listPmMessages).toHaveBeenCalledTimes(1)
    expect(wrapper.find('[data-testid="pm-history-tip"]').text()).toContain('已到最早')
    wrapper.unmount()
  })

  it('refetchAfterTurnDone merges by id and keeps prepended prefix', async () => {
    let socket: MockWebSocket | null = null
    class CaptureWS extends MockWebSocket {
      constructor() {
        super()
        socket = this
      }
    }
    // @ts-expect-error test stub
    globalThis.WebSocket = CaptureWS

    const older = Array.from({ length: 20 }, (_, i) => ({
      id: `m-${i}`,
      role: 'user',
      content: `old-${i}`,
      status: 'ok',
    }))
    const tail = Array.from({ length: 20 }, (_, i) => ({
      id: `m-${i + 20}`,
      role: i % 2 === 0 ? 'user' : 'assistant',
      content: `tail-${i + 20}`,
      status: 'ok',
    }))
    apiMocks.listPmThreads.mockResolvedValue({
      items: [{ id: 'thr-long', title: '长会话', userId: 'alice' }],
    })
    apiMocks.listPmMessages
      .mockResolvedValueOnce({ items: tail, hasMore: true })
      .mockResolvedValueOnce({ items: older, hasMore: false })
      .mockResolvedValueOnce({
        items: [
          ...tail.slice(-19),
          { id: 'm-39', role: 'assistant', content: 'updated-tail', status: 'ok' },
          { id: 'm-40', role: 'assistant', content: 'turn-result', status: 'ok' },
        ],
        hasMore: true,
      })

    const wrapper = mountChat()
    await flushPromises()

    const scrollerEl = wrapper.find('[data-testid="pm-message-scroller"]').element as HTMLElement
    mockScrollerMetrics(scrollerEl, 2000, 400, 30).syncStickFromScroll()
    await flushPromises()
    expect(wrapper.findAll('[data-msg-id]').length).toBe(40)

    const textarea = wrapper.find('textarea')
    await textarea.setValue('继续')
    const sendBtn = wrapper.findAll('button').find((b) => b.text() === '发送')
    await sendBtn!.trigger('click')
    await flushPromises()

    socket!.onmessage?.(
      new MessageEvent('message', {
        data: JSON.stringify({ type: 'turn_done', seq: 0 }),
      }),
    )
    await flushPromises()

    // Prefix m-0..m-19 must remain; new turn message appears; no collapse to 20.
    expect(wrapper.find('[data-msg-id="m-0"]').exists()).toBe(true)
    expect(wrapper.find('[data-msg-id="m-40"]').exists()).toBe(true)
    expect(wrapper.findAll('[data-msg-id]').length).toBeGreaterThan(20)
    const mergeCall = apiMocks.listPmMessages.mock.calls.at(-1)
    expect(mergeCall?.[2]).toEqual({ limit: 20 })
    wrapper.unmount()
  })

  it('lazyload failure keeps messages and retries on next top scroll', async () => {
    const tail = Array.from({ length: 20 }, (_, i) => ({
      id: `m-${i + 20}`,
      role: 'user',
      content: `t-${i + 20}`,
      status: 'ok',
    }))
    const earlier = Array.from({ length: 10 }, (_, i) => ({
      id: `m-${i + 10}`,
      role: 'user',
      content: `e-${i + 10}`,
      status: 'ok',
    }))
    apiMocks.listPmThreads.mockResolvedValue({
      items: [{ id: 'thr-long', title: '长会话', userId: 'alice' }],
    })
    apiMocks.listPmMessages
      .mockResolvedValueOnce({ items: tail, hasMore: true })
      .mockRejectedValueOnce(new Error('network'))
      .mockResolvedValueOnce({ items: earlier, hasMore: true })

    const wrapper = mountChat()
    await flushPromises()

    const scrollerEl = wrapper.find('[data-testid="pm-message-scroller"]').element as HTMLElement
    const metrics = mockScrollerMetrics(scrollerEl, 2000, 400, 20)
    metrics.syncStickFromScroll()
    await flushPromises()

    expect(wrapper.findAll('[data-msg-id]').length).toBe(20)
    expect(wrapper.find('[data-testid="pm-history-tip"]').text()).toContain('再次滚到顶部可重试')

    // Scroll away then back to top to retry.
    metrics.setScrollTop(200)
    metrics.syncStickFromScroll()
    await flushPromises()
    metrics.setScrollTop(10)
    metrics.syncStickFromScroll()
    await flushPromises()

    expect(wrapper.findAll('[data-msg-id]').length).toBe(30)
    expect(wrapper.find('[data-msg-id="m-10"]').exists()).toBe(true)
    wrapper.unmount()
  })
})
