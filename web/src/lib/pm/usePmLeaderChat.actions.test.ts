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

  it('resumes a live server draft from its sequence watermark', async () => {
    mocks.listPmMessages.mockResolvedValueOnce({
      items: [message('live', 'user', { status: 'sending' })],
      hasMore: false,
    })
    mocks.getPmDraft.mockResolvedValueOnce({
      draft: { status: 'streaming', userMsgId: 'live', partialText: 'partial', eventSeq: 4 },
      live: true,
      hasFinal: false,
    })
    const { chat, app } = withChat()
    await flushPromises()
    expect(chat.resuming.value).toBe(true)
    expect(chat.streamText.value).toBe('partial')
    expect(chat.lastEventSeq.value).toBe(4)
    expect(JSON.parse(MockWebSocket.instances[0]!.sent[0]!)).toEqual({ type: 'resume', afterSeq: 4 })
    app.unmount()
  })

  it('classifies resume failures and dead streaming drafts', async () => {
    mocks.listPmMessages.mockResolvedValueOnce({
      items: [message('dead', 'user', { status: 'sending' })],
      hasMore: false,
    })
    mocks.getPmDraft.mockResolvedValueOnce({
      draft: { status: 'streaming', userMsgId: 'dead', partialText: '', eventSeq: 2 },
      live: false,
      hasFinal: false,
    })
    const { chat, app } = withChat()
    await flushPromises()
    expect(chat.messages.value[0]?.failKind).toBe('connection')

    mocks.ensurePmSandbox.mockRejectedValueOnce(new Error('resume unavailable'))
    await chat.beginResume('th-1', 'old', 1, 'dead')
    expect(chat.messages.value[0]?.status).toBe('failed')
    expect(mocks.toastError).toHaveBeenCalledWith('resume unavailable')
    app.unmount()
  })

  it('handles failed message loads and retries the active thread', async () => {
    mocks.listPmMessages.mockRejectedValueOnce(new Error('messages down'))
    const { chat, app } = withChat()
    await flushPromises()
    expect(chat.messagesLoadFailed.value).toBe(true)
    expect(mocks.toastError).toHaveBeenCalled()

    mocks.listPmMessages.mockResolvedValueOnce({ items: [message('ok')], hasMore: false })
    await chat.retryLoadMessages()
    expect(chat.messagesLoadFailed.value).toBe(false)
    expect(chat.messages.value[0]?.id).toBe('ok')
    chat.messagesLoading.value = true
    await chat.retryLoadMessages()
    app.unmount()
  })

  it('selects threads on mobile and honors busy navigation guards', async () => {
    shared.isMobile.value = true
    mocks.listPmThreads.mockResolvedValue({ items: [thread(), thread({ id: 'th-2' })] })
    const { chat, app, emit, props } = withChat({ restoreMobileChat: true })
    await flushPromises()
    expect(chat.mobileView.value).toBe('chat')
    expect(emit).toHaveBeenCalledWith('restoredMobileChat')
    chat.backToThreads()
    expect(chat.mobileView.value).toBe('threads')
    await chat.selectThread('th-1')
    expect(chat.mobileView.value).toBe('chat')
    await chat.selectThread('th-2')
    expect(chat.activeId.value).toBe('th-2')

    chat.sending.value = true
    chat.backToThreads()
    await chat.selectThread('th-1')
    expect(chat.activeId.value).toBe('th-2')
    chat.sending.value = false
    props.projectId = 'proj-2'
    await flushPromises()
    expect(mocks.listPmThreads).toHaveBeenCalledWith('proj-2')
    app.unmount()
  })

  it('restores send state when append fails and blocks unavailable sends', async () => {
    mocks.appendPmMessage.mockRejectedValueOnce(new Error('append down'))
    const { chat, app, emit } = withChat()
    await flushPromises()
    chat.input.value = 'keep me'
    chat.attachments.value = [imageAttachment()]
    await chat.send()
    expect(chat.input.value).toBe('keep me')
    expect(chat.attachments.value).toHaveLength(1)
    expect(mocks.toastError).toHaveBeenCalledWith('append down')

    chat.input.value = 'disabled'
    ;(chat.enabled as { value: boolean }).value
    const disabled = withChat({ binding: { enabled: false, agentAvailable: true } })
    await flushPromises()
    disabled.chat.input.value = 'hello'
    await disabled.chat.send()
    expect(disabled.emit).toHaveBeenCalledWith('openSettings')
    emit.mockClear()
    disabled.app.unmount()
    app.unmount()
  })

  it('falls back locally when failure persistence and clearing fail', async () => {
    const { chat, app } = withChat()
    await flushPromises()
    chat.messages.value = [message('u', 'user', { status: 'ok' }) as never]
    mocks.patchPmMessage.mockRejectedValueOnce(new Error('persist down'))
    await chat.persistFailure('u', 'unknown')
    expect(chat.messages.value[0]).toMatchObject({ status: 'failed', failKind: 'unknown' })
    mocks.patchPmMessage.mockRejectedValueOnce(new Error('clear down'))
    await chat.clearFailure('u')
    expect(chat.messages.value[0]).toMatchObject({ status: 'ok', failKind: '' })
    expect(mocks.toastError).toHaveBeenCalledWith('clear down')
    app.unmount()
  })

  it('reports final refetch failures and clears cancelled turn completion', async () => {
    const { chat, app } = withChat()
    await flushPromises()
    chat.input.value = 'turn'
    await chat.send()
    mocks.listPmMessages.mockRejectedValueOnce(new Error('refresh down'))
    MockWebSocket.instances.at(-1)!.frame({ type: 'turn_done' })
    await flushPromises()
    expect(chat.finalizingRefetchFailed.value).toBe(true)
    expect(chat.finalizing.value).toBe(true)

    await chat.onTurnDone()
    expect(chat.finalizing.value).toBe(false)
    expect(chat.streamText.value).toBe('')
    app.unmount()
  })

  it('handles malformed frames, ACP deltas, and channel context guards', async () => {
    const { chat, app } = withChat()
    await flushPromises()
    chat.input.value = 'start'
    await chat.send()
    const socket = MockWebSocket.instances.at(-1)!
    socket.onmessage?.(new MessageEvent('message', { data: '{bad' }))
    chat.handleAcp({
      type: 'session_update',
      update: {
        sessionUpdate: 'agent_message_chunk',
        content: { text: 'delta' },
      },
    })
    chat.handleAcp({ type: 'tool_call', content: 'ignored' })
    expect(chat.streamText.value).toContain('delta')
    chat.openChannelCtx(new MouseEvent('contextmenu'), thread() as never)
    expect(chat.channelCtx.value).toBeNull()
    chat.openChannelDetail()
    chat.closeChannelCtx()
    chat.onChannelCtxAction()
    app.unmount()
  })

  it('covers computed view states and message merge helpers', async () => {
    const { chat, app } = withChat()
    await flushPromises()
    expect(chat.mergeMessagesKeepPrefix([], [message('a') as never])).toHaveLength(1)
    expect(chat.mergeMessagesKeepPrefix([message('a') as never], [])).toHaveLength(1)
    expect(
      chat.mergeMessagesKeepPrefix(
        [message('a', 'user', { content: 'old' }) as never],
        [message('a', 'user', { content: 'new' }) as never, message('b') as never],
      ),
    ).toEqual(expect.arrayContaining([expect.objectContaining({ id: 'a', content: 'new' })]))
    chat.finalizing.value = true
    expect(chat.mainViewState.value).toBe('finalizing')
    expect(chat.busyHint.value).toBeTruthy()
    chat.finalizing.value = false
    chat.resuming.value = true
    expect(chat.mainViewState.value).toBe('resuming')
    chat.resuming.value = false
    chat.messagesLoading.value = true
    expect(chat.mainViewState.value).toBe('messagesLoading')
    chat.messagesLoading.value = false
    chat.messagesLoadFailed.value = true
    expect(chat.mainViewState.value).toBe('errorEmpty')
    expect(chat.failMeta('not-real').kind).toBe('unknown')

    for (const [kind, classPart] of [
      ['wecom', 'accent'],
      ['feishu', 'cyan'],
      ['dingtalk', 'blue'],
      ['qq', 'accent'],
    ]) {
      const th = thread({ userId: `${kind}:c2c:peer` }) as never
      expect(chat.channelBadgeLabel(th)).toBeTruthy()
      expect(chat.channelBadgeClass(th)).toContain(classPart)
      expect(chat.channelReadonlyTitle(th)).toBeTruthy()
      expect(chat.channelReadonlyHint(th)).toBeTruthy()
    }
    expect(chat.threadDisplayTitle(thread({ title: '  ' }) as never)).toBeTruthy()
    expect(chat.channelSourceLine(thread({ userId: 'qq:c2c:peer' }) as never)).toContain('peer')
    chat.messagesLoadFailed.value = false
    chat.messages.value = [message('u', 'user') as never, message('a', 'assistant') as never]
    expect(chat.showIdleSuggestions.value).toBe(true)
    chat.historyLoading.value = true
    expect(chat.historyTipText.value).toBeTruthy()
    expect(chat.historyTipClass.value).toContain('accent')
    chat.historyLoading.value = false
    chat.historyLoadFailed.value = true
    expect(chat.historyTipClass.value).toContain('err')
    app.unmount()
  })

  it('fails a fresh turn when sandbox preparation rejects', async () => {
    mocks.ensurePmSandbox.mockRejectedValueOnce(new Error('sandbox boot exploded'))
    const { chat, app } = withChat()
    await flushPromises()
    chat.input.value = 'question'
    await chat.send()
    expect(chat.messages.value[0]).toMatchObject({ id: 'u-new', status: 'failed' })
    expect(chat.sending.value).toBe(false)
    expect(mocks.toastError).toHaveBeenCalledWith('sandbox boot exploded')
    app.unmount()
  })

  it('persists stopped when cancellation happens during thread creation', async () => {
    let resolveCreate!: (value: ReturnType<typeof thread>) => void
    mocks.listPmThreads.mockResolvedValueOnce({ items: [] })
    mocks.createPmThread.mockReturnValueOnce(
      new Promise((resolve) => {
        resolveCreate = resolve
      }),
    )
    const { chat, app } = withChat()
    await flushPromises()
    chat.input.value = 'slow create'
    const sending = chat.send()
    chat.stop()
    resolveCreate(thread({ id: 'created' }))
    await sending
    expect(chat.sending.value).toBe(false)
    expect(mocks.appendPmMessage).not.toHaveBeenCalled()
    app.unmount()
  })

  it('persists stopped when cancellation happens during message append', async () => {
    let resolveAppend!: (value: ReturnType<typeof message>) => void
    mocks.appendPmMessage.mockReturnValueOnce(
      new Promise((resolve) => {
        resolveAppend = resolve
      }),
    )
    const { chat, app } = withChat()
    await flushPromises()
    chat.input.value = 'slow append'
    const sending = chat.send()
    chat.stop()
    resolveAppend(message('slow', 'user', { content: 'slow append' }))
    await sending
    expect(mocks.patchPmMessage).toHaveBeenCalledWith(
      'proj-1',
      'th-1',
      'slow',
      expect.objectContaining({ failKind: 'stopped' }),
    )
    app.unmount()
  })

  it('aborts readiness and times out websocket opening', async () => {
    const { chat, app } = withChat()
    await flushPromises()
    const controller = new AbortController()
    controller.abort()
    await expect(chat.ensureSandbox(true, controller.signal)).rejects.toThrow('Aborted')
    await expect(chat.waitReady(7, controller.signal)).rejects.toThrow('Aborted')

    vi.useFakeTimers()
    const connecting = new MockWebSocket('ws://timeout')
    connecting.readyState = MockWebSocket.CONNECTING
    const pending = chat.waitWsOpen(connecting as never, 10)
    vi.advanceTimersByTime(10)
    await expect(pending).rejects.toThrow('ws open timeout')
    app.unmount()
  })

  it('handles turn errors when message refresh itself fails', async () => {
    const { chat, app } = withChat()
    await flushPromises()
    chat.input.value = 'unknown'
    await chat.send()
    mocks.listPmMessages.mockRejectedValueOnce(new Error('refresh failed'))
    MockWebSocket.instances.at(-1)!.frame({ type: 'error', failKind: 'bogus', error: 'server detail' })
    await flushPromises()
    expect(mocks.patchPmMessage).toHaveBeenCalledWith(
      'proj-1',
      'th-1',
      'u-new',
      expect.objectContaining({ failKind: 'unknown' }),
    )
    expect(mocks.toastError).toHaveBeenCalledWith('server detail')
    app.unmount()
  })
})

function imageAttachment() {
  return { data: 'YQ==', mimeType: 'image/png', name: 'a.png' }
}
