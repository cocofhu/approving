// @vitest-environment happy-dom
import { createApp, defineComponent, nextTick, reactive, ref } from 'vue'
import { createI18n } from 'vue-i18n'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises } from '@vue/test-utils'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'

const shared = vi.hoisted(() => {
  // eslint-disable-next-line @typescript-eslint/no-require-imports
  const { ref: hoistedRef } = require('vue') as typeof import('vue')
  return { isMobile: hoistedRef(false) }
})

const mocks = vi.hoisted(() => ({
  listPmThreads: vi.fn(),
  createPmThread: vi.fn(),
  deletePmThread: vi.fn(),
  listPmMessages: vi.fn(),
  getPmDraft: vi.fn(),
  appendPmMessage: vi.fn(),
  patchPmMessage: vi.fn(),
  ensurePmSandbox: vi.fn(),
  getSandbox: vi.fn(),
  pmThreadChatWsUrl: vi.fn(() => 'ws://example.test/pm'),
  toastSuccess: vi.fn(),
  toastError: vi.fn(),
}))

vi.mock('@/lib/api/api', async () => {
  const actual = await vi.importActual<typeof import('@/lib/api/api')>('@/lib/api/api')
  return {
    ...actual,
    api: {
      ...actual.api,
      listPmThreads: mocks.listPmThreads,
      createPmThread: mocks.createPmThread,
      deletePmThread: mocks.deletePmThread,
      listPmMessages: mocks.listPmMessages,
      getPmDraft: mocks.getPmDraft,
      appendPmMessage: mocks.appendPmMessage,
      patchPmMessage: mocks.patchPmMessage,
      ensurePmSandbox: mocks.ensurePmSandbox,
      getSandbox: mocks.getSandbox,
      pmThreadChatWsUrl: mocks.pmThreadChatWsUrl,
    },
  }
})

vi.mock('@/lib/composables/useToast', () => ({
  useToast: () => ({
    success: mocks.toastSuccess,
    error: mocks.toastError,
    warn: vi.fn(),
    info: vi.fn(),
  }),
}))

vi.mock('@/lib/composables/useBreakpoint', () => ({
  useBreakpoint: () => ({ isMobile: shared.isMobile }),
}))

import { usePmLeaderChat } from './usePmLeaderChat'

const thread = (over: Record<string, unknown> = {}) => ({
  id: 'th-1',
  projectId: 'proj-1',
  userId: 'u1',
  agentName: 'pm',
  kind: 'pm',
  title: '会话',
  createdAt: '2026-01-01T00:00:00Z',
  updatedAt: '2026-01-01T00:00:00Z',
  ...over,
})

const message = (id: string, role: 'user' | 'assistant' | 'system' = 'user', over = {}) => ({
  id,
  threadId: 'th-1',
  role,
  content: id,
  createdAt: '2026-01-01T00:00:00Z',
  ...over,
})

class MockWebSocket {
  static CONNECTING = 0
  static OPEN = 1
  static CLOSING = 2
  static CLOSED = 3
  static instances: MockWebSocket[] = []
  readyState = MockWebSocket.OPEN
  onmessage: ((ev: MessageEvent) => void) | null = null
  onerror: ((ev: Event) => void) | null = null
  onclose: ((ev: CloseEvent) => void) | null = null
  sent: string[] = []
  listeners = new Map<string, Set<EventListener>>()
  constructor(public url: string) {
    MockWebSocket.instances.push(this)
  }
  send(data: string) {
    this.sent.push(data)
  }
  close() {
    this.readyState = MockWebSocket.CLOSED
    this.onclose?.(new CloseEvent('close'))
  }
  addEventListener(type: string, fn: EventListener) {
    const set = this.listeners.get(type) ?? new Set()
    set.add(fn)
    this.listeners.set(type, set)
  }
  removeEventListener(type: string, fn: EventListener) {
    this.listeners.get(type)?.delete(fn)
  }
  fire(type: string) {
    for (const fn of this.listeners.get(type) ?? []) fn(new Event(type))
  }
  frame(data: unknown) {
    this.onmessage?.(new MessageEvent('message', { data: JSON.stringify(data) }))
  }
}

function withChat(over: Record<string, unknown> = {}) {
  let chat!: ReturnType<typeof usePmLeaderChat>
  const emit = vi.fn()
  const props = reactive({
    projectId: 'proj-1',
    binding: { enabled: true, agentConfigRef: 'pm', agentAvailable: true, aclNote: '' },
    ...over,
  })
  const i18n = createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...common, ...pages } },
  })
  const Comp = defineComponent({
    setup() {
      chat = usePmLeaderChat(props as never, emit as never)
      return () => null
    },
  })
  const app = createApp(Comp)
  app.use(i18n)
  app.mount(document.createElement('div'))
  return { chat, app, emit, props }
}

describe('usePmLeaderChat actions', () => {
  beforeEach(() => {
    for (const fn of Object.values(mocks)) (fn as ReturnType<typeof vi.fn>).mockReset()
    shared.isMobile.value = false
    localStorage.clear()
    MockWebSocket.instances = []
    vi.stubGlobal('WebSocket', MockWebSocket)
    vi.stubGlobal('requestAnimationFrame', (cb: FrameRequestCallback) => {
      cb(0)
      return 1
    })
    mocks.listPmThreads.mockResolvedValue({ items: [thread()] })
    mocks.listPmMessages.mockResolvedValue({ items: [], hasMore: false })
    mocks.getPmDraft.mockResolvedValue({ draft: null, live: false, hasFinal: false })
    mocks.createPmThread.mockResolvedValue(thread({ id: 'th-2', title: '新会话' }))
    mocks.deletePmThread.mockResolvedValue({ status: 'ok' })
    mocks.appendPmMessage.mockImplementation(async (_p, _t, body) =>
      message('u-new', 'user', { content: body.content, images: body.images }),
    )
    mocks.patchPmMessage.mockImplementation(async (_p, _t, id, patch) => message(id, 'user', patch))
    mocks.ensurePmSandbox.mockResolvedValue({ sandbox: { id: 7 }, preamble: 'context' })
    mocks.getSandbox.mockResolvedValue({ status: 'running' })
  })

  afterEach(() => {
    vi.useRealTimers()
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  })

  it('restores the stored thread and derives channel presentation', async () => {
    localStorage.setItem('pm-leader:active-thread:proj-1', 'ch')
    mocks.listPmThreads.mockResolvedValue({
      items: [
        thread(),
        thread({ id: 'ch', userId: 'feishu:c2c:peer-9', title: '', unspoken: true }),
      ],
    })
    const { chat, app } = withChat()
    await flushPromises()

    expect(chat.activeId.value).toBe('ch')
    expect(chat.activeIsChannel.value).toBe(true)
    expect(chat.channelTypeOf(chat.activeThread.value)).toBe('feishu')
    expect(chat.channelBadgeLabel(chat.activeThread.value)).toBeTruthy()
    expect(chat.channelBadgeClass(chat.activeThread.value)).toContain('cyan')
    expect(chat.channelReadonlyTitle(chat.activeThread.value)).toBeTruthy()
    expect(chat.channelReadonlyHint(chat.activeThread.value)).toBeTruthy()
    expect(chat.threadDisplayTitle(chat.activeThread.value)).toBe('feishu:c2c:peer-9')
    expect(chat.channelSourceLine(chat.activeThread.value)).toContain('peer-9')
    expect(chat.isChannelHint(message('h', 'system', { source: 'channel' }) as never)).toBe(true)

    const ev = { preventDefault: vi.fn(), stopPropagation: vi.fn(), clientX: 4, clientY: 8 }
    chat.openChannelCtx(ev as unknown as MouseEvent, chat.activeThread.value!)
    expect(chat.channelCtx.value?.threadId).toBe('ch')
    chat.openChannelDetail()
    expect(chat.channelDetailOpen.value).toBe(true)
    chat.closeChannelDetail()
    app.unmount()
  })

  it('loads earlier messages, preserves scroll position and exposes retry failure', async () => {
    mocks.listPmMessages
      .mockResolvedValueOnce({ items: [message('m2'), message('m3')], hasMore: true })
      .mockResolvedValueOnce({ items: [message('m1'), message('m2')], hasMore: false })
      .mockRejectedValueOnce(new Error('older down'))
    const { chat, app } = withChat()
    await flushPromises()
    const el = document.createElement('div')
    Object.defineProperties(el, {
      scrollTop: { value: 10, writable: true },
      scrollHeight: { value: 200, configurable: true },
      clientHeight: { value: 100, configurable: true },
    })
    chat.scroller.value = el
    await chat.loadEarlier()
    expect(chat.messages.value.map((m) => m.id)).toEqual(['m1', 'm2', 'm3'])
    expect(chat.hasMoreEarlier.value).toBe(false)

    chat.hasMoreEarlier.value = true
    await chat.loadEarlier()
    expect(chat.historyLoadFailed.value).toBe(true)
    expect(chat.historyLoading.value).toBe(false)
    chat.onScrollerScroll()
    expect(chat.stickToBottom.value).toBe(false)
    chat.scrollBottom(true)
    expect(el.scrollTop).toBe(200)
    app.unmount()
  })

  it('handles load, create, delete and localStorage failures', async () => {
    mocks.listPmThreads.mockRejectedValueOnce(new Error('threads down'))
    const { chat, app, emit } = withChat()
    await flushPromises()
    expect(mocks.toastError).toHaveBeenCalledWith('threads down')

    await chat.newThread()
    expect(chat.activeId.value).toBe('th-2')
    mocks.createPmThread.mockRejectedValueOnce(new Error('create down'))
    await chat.newThread()
    expect(mocks.toastError).toHaveBeenCalledWith('create down')

    mocks.deletePmThread.mockRejectedValueOnce(new Error('delete down'))
    await chat.removeThread('th-2')
    expect(mocks.toastError).toHaveBeenCalledWith('delete down')
    mocks.deletePmThread.mockResolvedValueOnce({})
    await chat.removeThread('th-2')
    expect(chat.activeId.value).toBe('')

    const disabled = withChat({ binding: null })
    await flushPromises()
    await disabled.chat.newThread()
    expect(disabled.emit).toHaveBeenCalledWith('openSettings')
    emit.mockClear()
    disabled.app.unmount()
    app.unmount()
  })

  it('sends a turn, consumes websocket frames and finalizes from the server', async () => {
    const { chat, app } = withChat()
    await flushPromises()
    chat.input.value = ' hello '
    await chat.send()
    const socket = MockWebSocket.instances.at(-1)!
    expect(mocks.appendPmMessage).toHaveBeenCalled()
    expect(JSON.parse(socket.sent.at(-1)!)).toMatchObject({
      type: 'chat',
      content: 'context\n\n用户问题：hello',
      userMsgId: 'u-new',
    })

    socket.frame({ seq: 1, type: 'resume_hint', partialText: 'seed', eventSeq: 5, userMsgId: 'u-new' })
    expect(chat.streamText.value).toBe('seed')
    socket.frame({ seq: 5, type: 'acp', data: { type: 'agent_message_chunk', content: 'ignored' } })
    socket.frame({ type: 'acp', data: { type: 'agent_message_chunk', content: '!' } })
    socket.frame('bad json')

    mocks.listPmMessages.mockResolvedValueOnce({
      items: [message('u-new'), message('a-new', 'assistant')],
      hasMore: false,
    })
    mocks.listPmThreads.mockResolvedValueOnce({ items: [thread({ title: '更新' })] })
    socket.frame({ seq: 6, type: 'turn_done' })
    await flushPromises()
    expect(chat.finalizing.value).toBe(false)
    expect(chat.messages.value.some((m) => m.id === 'a-new')).toBe(true)
    expect(chat.streamText.value).toBe('')
    app.unmount()
  })

  it('marks websocket and server errors as failed', async () => {
    const { chat, app } = withChat()
    await flushPromises()
    chat.input.value = 'fail me'
    await chat.send()
    const socket = MockWebSocket.instances.at(-1)!
    mocks.listPmMessages.mockResolvedValueOnce({
      items: [message('u-new', 'user', { status: 'ok' })],
      hasMore: false,
    })
    socket.frame({ type: 'error', failKind: 'sandbox', error: 'runner failed' })
    await flushPromises()
    expect(mocks.patchPmMessage).toHaveBeenCalledWith(
      'proj-1',
      'th-1',
      'u-new',
      expect.objectContaining({ status: 'failed', failKind: 'sandbox' }),
    )

    chat.input.value = 'disconnect'
    await chat.send()
    MockWebSocket.instances.at(-1)!.onerror?.(new Event('error'))
    await flushPromises()
    expect(chat.sending.value).toBe(false)
    app.unmount()
  })

  it('hydrates failed drafts and converges orphan messages even when persistence fails', async () => {
    mocks.listPmMessages.mockResolvedValueOnce({
      items: [
        message('draft', 'user', { status: 'ok' }),
        message('orphan', 'user', { status: 'sending' }),
      ],
      hasMore: false,
    })
    mocks.getPmDraft.mockResolvedValueOnce({
      draft: { status: 'failed', userMsgId: 'draft', failKind: 'unknown', partialText: 'partial' },
      live: false,
      hasFinal: false,
    })
    mocks.patchPmMessage
      .mockImplementationOnce(async (_p, _t, id, patch) => message(id, 'user', patch))
      .mockRejectedValueOnce(new Error('patch down'))
    const { chat, app } = withChat()
    await flushPromises()
    expect(chat.failedPartialByUserMsgId.value.draft).toBe('partial')
    expect(chat.messages.value.find((m) => m.id === 'draft')?.failKind).toBe('connection')
    expect(chat.messages.value.find((m) => m.id === 'orphan')?.status).toBe('failed')
    expect(mocks.toastError).toHaveBeenCalledWith('patch down')
    app.unmount()
  })

  it('stops and retries a failed turn without appending another user message', async () => {
    const { chat, app } = withChat()
    await flushPromises()
    chat.messages.value = [message('retry', 'user', { content: 'again', status: 'failed' }) as never]
    await chat.retryTurn('missing')
    await chat.retryTurn('retry')
    expect(mocks.patchPmMessage).toHaveBeenCalledWith('proj-1', 'th-1', 'retry', { status: 'ok' })
    expect(mocks.appendPmMessage).not.toHaveBeenCalled()
    const socket = MockWebSocket.instances.at(-1)!
    expect(JSON.parse(socket.sent.at(-1)!)).toMatchObject({ type: 'chat', userMsgId: 'retry' })
    chat.stop()
    await flushPromises()
    expect(socket.sent.some((raw) => JSON.parse(raw).type === 'cancel')).toBe(true)
    expect(chat.messages.value[0]?.status).toBe('failed')
    app.unmount()
  })

  it('covers sandbox readiness and websocket opening outcomes', async () => {
    const { chat, app } = withChat()
    await flushPromises()
    mocks.getSandbox.mockResolvedValueOnce({ status: 'error', error: 'boot failed' })
    await expect(chat.waitReady(7)).rejects.toThrow('boot failed')

    const open = new MockWebSocket('ws://open')
    expect(await chat.waitWsOpen(open as never, 10)).toBeUndefined()
    const closed = new MockWebSocket('ws://closed')
    closed.readyState = MockWebSocket.CLOSED
    await expect(chat.waitWsOpen(closed as never, 10)).rejects.toThrow('ws closed')
    const connecting = new MockWebSocket('ws://connecting')
    connecting.readyState = MockWebSocket.CONNECTING
    const pending = chat.waitWsOpen(connecting as never, 100)
    connecting.fire('error')
    await expect(pending).rejects.toThrow('ws error')
    app.unmount()
  })

  it('copies assistant text and reports clipboard failures', async () => {
    const { chat, app } = withChat()
    await flushPromises()
    const root = document.createElement('div')
    root.dataset.assistantBubble = ''
    const md = document.createElement('div')
    md.className = 'md'
    md.innerText = 'answer'
    root.appendChild(md)
    const button = document.createElement('button')
    root.appendChild(button)
    const writeText = vi.fn().mockResolvedValue(undefined)
    vi.stubGlobal('navigator', { clipboard: { writeText } })
    await chat.copyAssistantText({ currentTarget: button } as unknown as Event)
    expect(writeText).toHaveBeenCalledWith('answer')
    expect(mocks.toastSuccess).toHaveBeenCalled()
    writeText.mockRejectedValueOnce(new Error('denied'))
    await chat.copyAssistantText({ currentTarget: button } as unknown as Event)
    expect(mocks.toastError).toHaveBeenCalled()
    app.unmount()
  })
})
