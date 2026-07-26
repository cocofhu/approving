// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'
import type { ClarifyTurn, ReactAnnotation } from '@/lib/types'
import ClarifyChat from './ClarifyChat.vue'

function mountChat(opts: {
  turns?: ClarifyTurn[]
  done?: boolean
  active?: boolean
  draft?: string
  reviewMode?: boolean
  confirmError?: string | null
  annotateEnabled?: boolean
  annotations?: ReactAnnotation[]
} = {}) {
  const i18n = createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...common, ...pages } },
  })
  return mount(ClarifyChat, {
    props: {
      runId: 'run-1',
      nodeId: 'react-1',
      iteration: 1,
      turns: opts.turns ?? [],
      done: opts.done ?? false,
      active: opts.active ?? true,
      draft: opts.draft ?? '',
      reviewMode: opts.reviewMode ?? false,
      confirmError: opts.confirmError ?? null,
      annotateEnabled: opts.annotateEnabled ?? false,
      annotations: opts.annotations ?? [],
    },
    global: {
      plugins: [i18n],
      stubs: { Icon: true, ClarifyDemoFrame: true },
    },
  })
}

async function clickSend(wrapper: ReturnType<typeof mountChat>) {
  const sendBtn = wrapper.find('button[class*="bg-accent"]')
  expect(sendBtn.exists()).toBe(true)
  await sendBtn.trigger('click')
  await flushPromises()
}

describe('ClarifyChat', () => {
  beforeEach(() => {
    sessionStorage.clear()
  })

  it('renders turns and composer on active dialogue', () => {
    const turns: ClarifyTurn[] = [
      { role: 'agent', text: '你好，请描述需求', at: '2026-07-18T00:00:00Z' },
    ]
    const wrapper = mountChat({ turns })
    expect(wrapper.text()).toContain('你好，请描述需求')
    expect(wrapper.find('textarea').exists()).toBe(true)
    wrapper.unmount()
  })

  it('shows done banner and hides composer when done', () => {
    const wrapper = mountChat({ done: true, turns: [{ role: 'agent', text: '完成', at: '2026-07-18T00:00:00Z' }] })
    expect(wrapper.text()).toMatch(/交互已完成|已完成/)
    expect(wrapper.find('textarea').exists()).toBe(false)
    wrapper.unmount()
  })

  it('shows closed hint when inactive', () => {
    const wrapper = mountChat({ active: false })
    expect(wrapper.text()).toMatch(/已关闭|不可再回复/)
    expect(wrapper.find('textarea').exists()).toBe(false)
    wrapper.unmount()
  })

  it('emits send when user submits text', async () => {
    const wrapper = mountChat()
    await wrapper.find('textarea').setValue('我的需求说明')
    await wrapper.find('button[class*="bg-accent"]').trigger('click')
    await flushPromises()
    expect(wrapper.emitted('send')).toBeTruthy()
    const payload = wrapper.emitted('send')![0]
    expect(payload[0]).toBe('我的需求说明')
    wrapper.unmount()
  })

  it('renders structured questions and submits choice', async () => {
    const turns: ClarifyTurn[] = [
      {
        role: 'agent',
        text: '',
        at: '2026-07-18T00:00:00Z',
        questions: [
          {
            id: 'q1',
            prompt: '选择部署方式',
            options: [
              { id: 'k8s', label: 'Kubernetes', recommended: true },
              { id: 'vm', label: '虚拟机' },
            ],
          },
        ],
      },
    ]
    const wrapper = mountChat({ turns })
    expect(wrapper.text()).toContain('选择部署方式')
    const optionBtn = wrapper.findAll('button').find((b) => b.text().includes('Kubernetes'))
    expect(optionBtn).toBeTruthy()
    await optionBtn!.trigger('click')
    await flushPromises()
    const submitBtn = wrapper.findAll('button').find((b) => b.text().includes('确认选择'))
    expect(submitBtn).toBeTruthy()
    await submitBtn!.trigger('click')
    await flushPromises()
    expect(wrapper.emitted('send')).toBeTruthy()
    expect(String(wrapper.emitted('send')![0][0])).toContain('Kubernetes')
    wrapper.unmount()
  })

  it('emits finish on early finish click', async () => {
    const wrapper = mountChat()
    const finishBtn = wrapper.findAll('button').find((b) => b.text().includes('结束交互'))
    expect(finishBtn).toBeTruthy()
    await finishBtn!.trigger('click')
    await flushPromises()
    expect(wrapper.emitted('finish')).toBeTruthy()
    wrapper.unmount()
  })

  it('review mode: confirm disabled while thinking/queued (FR4 ready gate)', async () => {
    const wrapper = mountChat({ reviewMode: true })
    // Enqueue a revise: thinking/queue must block 确认并流转 until ready.
    await wrapper.find('textarea').setValue('补充修订')
    await wrapper.find('button[class*="bg-accent"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="clarify-review-queue"]').exists()).toBe(true)
    const confirmBtn = wrapper.find('[data-testid="clarify-confirm-flow"]')
    expect(confirmBtn.exists()).toBe(true)
    expect((confirmBtn.element as HTMLButtonElement).disabled).toBe(true)
    expect(wrapper.find('[data-testid="clarify-confirm-hint"]').text()).toContain(
      '接受当前已落盘产物并流转（不触发 Agent）',
    )
    expect(wrapper.find('[data-testid="clarify-review-cancel"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('review mode: confirmError shows status bar and re-enables confirm', async () => {
    const wrapper = mountChat({ reviewMode: true })
    const confirmBtn = wrapper.find('[data-testid="clarify-confirm-flow"]')
    await confirmBtn.trigger('click')
    await flushPromises()
    expect((confirmBtn.element as HTMLButtonElement).disabled).toBe(true)
    await wrapper.setProps({ confirmError: '产物契约不满足' })
    await flushPromises()
    expect(wrapper.find('[data-testid="clarify-confirm-error"]').text()).toContain('产物契约不满足')
    expect((wrapper.find('[data-testid="clarify-confirm-flow"]').element as HTMLButtonElement).disabled).toBe(
      false,
    )
    wrapper.unmount()
  })

  it('shows optimistic annotation chips with text after send', async () => {
    const anns: ReactAnnotation[] = [{ label: '提案标题', jsonPath: 'proposals[0].title' }]
    const wrapper = mountChat({
      annotateEnabled: true,
      annotations: anns,
      draft: '请看这里',
    })
    await clickSend(wrapper)

    expect(wrapper.emitted('send')).toBeTruthy()
    const payload = wrapper.emitted('send')![0]
    expect(payload[0]).toBe('请看这里')
    expect(payload[2]).toEqual(anns)

    // Thinking window: body text + chip both visible
    expect(wrapper.text()).toContain('请看这里')
    expect(wrapper.text()).toContain('提案标题')
    expect(wrapper.text()).toMatch(/思考/)
    wrapper.unmount()
  })

  it('shows only annotation chips without empty body bubble when text is empty', async () => {
    const anns: ReactAnnotation[] = [{ label: 'Hero', selector: '#hero' }]
    const wrapper = mountChat({
      annotateEnabled: true,
      annotations: anns,
    })
    await clickSend(wrapper)

    expect(wrapper.emitted('send')).toBeTruthy()
    const payload = wrapper.emitted('send')![0]
    expect(payload[0]).toBe('')
    expect(payload[2]).toEqual(anns)

    // Chip is the visual subject; no markdown body bubble for empty text
    expect(wrapper.text()).toContain('Hero')
    expect(wrapper.findAll('.md.rounded-lg').length).toBe(0)
    expect(wrapper.text()).toMatch(/思考/)
    wrapper.unmount()
  })

  it('keeps optimistic annotation chips after composer annotations are cleared', async () => {
    const anns: ReactAnnotation[] = [{ label: '字段路径', jsonPath: 'summary' }]
    const wrapper = mountChat({
      annotateEnabled: true,
      annotations: anns,
      draft: '核对引用',
    })
    await clickSend(wrapper)

    // Parent applies composer clear from update:annotations (same as RunDetail)
    const updates = wrapper.emitted('update:annotations')
    expect(updates).toBeTruthy()
    const cleared = updates![updates!.length - 1][0] as ReactAnnotation[]
    await wrapper.setProps({ annotations: cleared, draft: '' })
    await flushPromises()

    // Composer staging cleared, but optimistic bubble still shows the snapshot chip
    expect(wrapper.props('annotations')).toEqual([])
    expect(wrapper.text()).toContain('字段路径')
    expect(wrapper.text()).toContain('核对引用')
    expect(wrapper.text()).toMatch(/思考/)
    wrapper.unmount()
  })

  it('uses clarify placeholder by default', () => {
    const wrapper = mountChat()
    expect(wrapper.find('[data-testid="clarify-input"]').attributes('placeholder')).toBe('补充信息…')
    wrapper.unmount()
  })

  it('uses review placeholder in reviewMode', () => {
    const wrapper = mountChat({ reviewMode: true })
    expect(wrapper.find('[data-testid="clarify-input"]').attributes('placeholder')).toBe(
      '补充批注或修改说明…',
    )
    wrapper.unmount()
  })

  function mockScrollHeight(el: HTMLTextAreaElement, height: number) {
    Object.defineProperty(el, 'scrollHeight', { configurable: true, get: () => height })
  }

  it('autoGrow: empty/single-line keeps ~40px and hides overflow', async () => {
    const wrapper = mountChat()
    const ta = wrapper.find('[data-testid="clarify-input"]').element as HTMLTextAreaElement
    mockScrollHeight(ta, 40)
    await wrapper.find('[data-testid="clarify-input"]').setValue('短句')
    await flushPromises()
    expect(ta.style.height).toBe('40px')
    expect(ta.classList.contains('overflow-y-hidden')).toBe(true)
    expect(ta.classList.contains('overflow-y-auto')).toBe(false)
    wrapper.unmount()
  })

  it('autoGrow: grows with content up to 128px max', async () => {
    const wrapper = mountChat()
    const ta = wrapper.find('[data-testid="clarify-input"]').element as HTMLTextAreaElement
    mockScrollHeight(ta, 96)
    await wrapper.find('[data-testid="clarify-input"]').setValue('第一行\n第二行\n第三行')
    await flushPromises()
    expect(ta.style.height).toBe('96px')
    expect(ta.classList.contains('overflow-y-hidden')).toBe(true)

    mockScrollHeight(ta, 200)
    await wrapper.find('[data-testid="clarify-input"]').setValue('a\n'.repeat(20))
    await flushPromises()
    expect(ta.style.height).toBe('128px')
    expect(ta.classList.contains('overflow-y-auto')).toBe(true)
    expect(ta.classList.contains('max-h-[128px]')).toBe(true)
    wrapper.unmount()
  })

  it('autoGrow: clears draft and collapses to ~40px instantly', async () => {
    const wrapper = mountChat({ draft: '多行\n内容\n说明' })
    const ta = wrapper.find('[data-testid="clarify-input"]').element as HTMLTextAreaElement
    mockScrollHeight(ta, 160)
    await wrapper.find('[data-testid="clarify-input"]').trigger('input')
    await flushPromises()
    expect(ta.style.height).toBe('128px')
    expect(ta.classList.contains('overflow-y-auto')).toBe(true)

    mockScrollHeight(ta, 40)
    await wrapper.setProps({ draft: '' })
    await flushPromises()
    expect(ta.style.height).toBe('40px')
    expect(ta.classList.contains('overflow-y-hidden')).toBe(true)
    wrapper.unmount()
  })

  it('keeps Enter send and Shift+Enter keybinding on textarea', async () => {
    const wrapper = mountChat({ draft: '发送我' })
    const ta = wrapper.find('[data-testid="clarify-input"]')
    await ta.trigger('keydown', { key: 'Enter', shiftKey: false })
    await flushPromises()
    expect(wrapper.emitted('send')).toBeTruthy()
    expect(wrapper.emitted('send')![0][0]).toBe('发送我')
    // Shift+Enter is not bound to send (exact modifier); no extra emit
    const before = wrapper.emitted('send')!.length
    await ta.trigger('keydown', { key: 'Enter', shiftKey: true })
    await flushPromises()
    expect(wrapper.emitted('send')!.length).toBe(before)
    wrapper.unmount()
  })

  it('reviewMode confirm bar remains visible with tall input', async () => {
    const wrapper = mountChat({ reviewMode: true, draft: 'a\n'.repeat(30) })
    const ta = wrapper.find('[data-testid="clarify-input"]').element as HTMLTextAreaElement
    mockScrollHeight(ta, 240)
    await wrapper.find('[data-testid="clarify-input"]').trigger('input')
    await flushPromises()
    expect(ta.style.height).toBe('128px')
    expect(wrapper.find('[data-testid="clarify-confirm-flow"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="clarify-confirm-hint"]').exists()).toBe(true)
    wrapper.unmount()
  })

  function scrollerOf(wrapper: ReturnType<typeof mountChat>) {
    return wrapper.find('[data-testid="clarify-scroller"]').element as HTMLElement
  }

  /** Mock scroller metrics so near-bottom / leave-bottom can be simulated in happy-dom. */
  function mockScrollerMetrics(
    el: HTMLElement,
    opts: { scrollHeight: number; clientHeight: number; scrollTop: number },
  ) {
    let top = opts.scrollTop
    Object.defineProperty(el, 'scrollHeight', { configurable: true, get: () => opts.scrollHeight })
    Object.defineProperty(el, 'clientHeight', { configurable: true, get: () => opts.clientHeight })
    Object.defineProperty(el, 'scrollTop', {
      configurable: true,
      get: () => top,
      set: (v: number) => {
        top = v
      },
    })
  }

  async function leaveBottom(wrapper: ReturnType<typeof mountChat>) {
    const el = scrollerOf(wrapper)
    mockScrollerMetrics(el, { scrollHeight: 800, clientHeight: 300, scrollTop: 100 })
    await wrapper.find('[data-testid="clarify-scroller"]').trigger('scroll')
    await flushPromises()
  }

  async function scrollNearBottom(wrapper: ReturnType<typeof mountChat>) {
    const el = scrollerOf(wrapper)
    mockScrollerMetrics(el, { scrollHeight: 800, clientHeight: 300, scrollTop: 760 })
    await wrapper.find('[data-testid="clarify-scroller"]').trigger('scroll')
    await flushPromises()
  }

  function agentTurn(text: string, at: string): ClarifyTurn {
    return { role: 'agent', text, at }
  }

  it('stick-to-bottom: stays put when off-bottom and new turns arrive', async () => {
    const wrapper = mountChat({
      turns: [agentTurn('历史消息', '2026-07-18T00:00:00Z')],
    })
    await leaveBottom(wrapper)
    const el = scrollerOf(wrapper)
    const before = el.scrollTop
    await wrapper.setProps({
      turns: [
        agentTurn('历史消息', '2026-07-18T00:00:00Z'),
        agentTurn('新消息', '2026-07-18T00:00:01Z'),
      ],
    })
    await flushPromises()
    expect(el.scrollTop).toBe(before)
    wrapper.unmount()
  })

  it('stick-to-bottom: auto-follows when near bottom', async () => {
    const wrapper = mountChat({
      turns: [agentTurn('历史消息', '2026-07-18T00:00:00Z')],
    })
    const el = scrollerOf(wrapper)
    mockScrollerMetrics(el, { scrollHeight: 800, clientHeight: 300, scrollTop: 760 })
    await wrapper.find('[data-testid="clarify-scroller"]').trigger('scroll')
    await flushPromises()
    await wrapper.setProps({
      turns: [
        agentTurn('历史消息', '2026-07-18T00:00:00Z'),
        agentTurn('贴底新消息', '2026-07-18T00:00:01Z'),
      ],
    })
    await flushPromises()
    // force/stick path sets scrollTop = scrollHeight
    expect(el.scrollTop).toBe(el.scrollHeight)
    expect(wrapper.find('[data-testid="clarify-unread-fab"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('unread FAB: accumulates exact count while off-bottom', async () => {
    const wrapper = mountChat({
      turns: [agentTurn('seed', '2026-07-18T00:00:00Z')],
    })
    await leaveBottom(wrapper)
    expect(wrapper.find('[data-testid="clarify-unread-fab"]').exists()).toBe(false)

    await wrapper.setProps({
      turns: [
        agentTurn('seed', '2026-07-18T00:00:00Z'),
        agentTurn('a', '2026-07-18T00:00:01Z'),
      ],
    })
    await flushPromises()
    const fab = wrapper.find('[data-testid="clarify-unread-fab"]')
    expect(fab.exists()).toBe(true)
    expect(fab.text()).toContain('1')
    expect(fab.attributes('aria-label')).toContain('1')
    expect(fab.attributes('title')).toContain('1')

    await wrapper.setProps({
      turns: [
        agentTurn('seed', '2026-07-18T00:00:00Z'),
        agentTurn('a', '2026-07-18T00:00:01Z'),
        agentTurn('b', '2026-07-18T00:00:02Z'),
        agentTurn('c', '2026-07-18T00:00:03Z'),
      ],
    })
    await flushPromises()
    expect(wrapper.find('[data-testid="clarify-unread-fab"]').text()).toContain('3')
    wrapper.unmount()
  })

  it('unread FAB: thinking alone does not show or increment unread', async () => {
    const wrapper = mountChat({
      turns: [agentTurn('seed', '2026-07-18T00:00:00Z')],
    })
    await leaveBottom(wrapper)
    // Simulate send-triggered thinking without new props.turns
    await wrapper.find('textarea').setValue('回复')
    // Leave bottom again after send force-stick, then re-enter off-bottom with no new turns
    await wrapper.find('button[class*="bg-accent"]').trigger('click')
    await flushPromises()
    expect(wrapper.text()).toMatch(/思考/)
    // After send we force-stick — FAB must be hidden even while thinking
    expect(wrapper.find('[data-testid="clarify-unread-fab"]').exists()).toBe(false)

    await leaveBottom(wrapper)
    // Still no new props.turns → still no FAB (thinking does not count)
    expect(wrapper.find('[data-testid="clarify-unread-fab"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('unread FAB: click clears unread and scrolls to bottom', async () => {
    const wrapper = mountChat({
      turns: [agentTurn('seed', '2026-07-18T00:00:00Z')],
    })
    await leaveBottom(wrapper)
    await wrapper.setProps({
      turns: [
        agentTurn('seed', '2026-07-18T00:00:00Z'),
        agentTurn('new', '2026-07-18T00:00:01Z'),
      ],
    })
    await flushPromises()
    const fab = wrapper.find('[data-testid="clarify-unread-fab"]')
    expect(fab.exists()).toBe(true)
    await fab.trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="clarify-unread-fab"]').exists()).toBe(false)
    expect(scrollerOf(wrapper).scrollTop).toBe(scrollerOf(wrapper).scrollHeight)
    wrapper.unmount()
  })

  it('unread FAB: manual scroll near bottom clears unread', async () => {
    const wrapper = mountChat({
      turns: [agentTurn('seed', '2026-07-18T00:00:00Z')],
    })
    await leaveBottom(wrapper)
    await wrapper.setProps({
      turns: [
        agentTurn('seed', '2026-07-18T00:00:00Z'),
        agentTurn('new', '2026-07-18T00:00:01Z'),
      ],
    })
    await flushPromises()
    expect(wrapper.find('[data-testid="clarify-unread-fab"]').exists()).toBe(true)
    await scrollNearBottom(wrapper)
    expect(wrapper.find('[data-testid="clarify-unread-fab"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('unread FAB: user send force-sticks and clears unread', async () => {
    const wrapper = mountChat({
      turns: [agentTurn('seed', '2026-07-18T00:00:00Z')],
      draft: '我看完了',
    })
    await leaveBottom(wrapper)
    await wrapper.setProps({
      turns: [
        agentTurn('seed', '2026-07-18T00:00:00Z'),
        agentTurn('agent-new', '2026-07-18T00:00:01Z'),
      ],
    })
    await flushPromises()
    expect(wrapper.find('[data-testid="clarify-unread-fab"]').exists()).toBe(true)

    await clickSend(wrapper)
    expect(wrapper.find('[data-testid="clarify-unread-fab"]').exists()).toBe(false)
    expect(scrollerOf(wrapper).scrollTop).toBe(scrollerOf(wrapper).scrollHeight)

    // Backend catch-up with user turn must not re-show FAB while stuck
    await wrapper.setProps({
      turns: [
        agentTurn('seed', '2026-07-18T00:00:00Z'),
        agentTurn('agent-new', '2026-07-18T00:00:01Z'),
        { role: 'human', text: '我看完了', at: '2026-07-18T00:00:02Z' },
      ],
    })
    await flushPromises()
    expect(wrapper.find('[data-testid="clarify-unread-fab"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('unread FAB: shows exact three-digit count without capping', async () => {
    const seed = Array.from({ length: 5 }, (_, i) =>
      agentTurn(`t${i}`, `2026-07-18T00:00:0${i}Z`),
    )
    const wrapper = mountChat({ turns: seed })
    await leaveBottom(wrapper)
    const more = [
      ...seed,
      ...Array.from({ length: 128 }, (_, i) =>
        agentTurn(`n${i}`, `2026-07-18T01:${String(i % 60).padStart(2, '0')}:00Z`),
      ),
    ]
    await wrapper.setProps({ turns: more })
    await flushPromises()
    const fab = wrapper.find('[data-testid="clarify-unread-fab"]')
    expect(fab.exists()).toBe(true)
    expect(fab.text()).toContain('128')
    expect(fab.text()).not.toMatch(/\+|99\+/)
    expect(fab.attributes('aria-label')).toContain('128')
    wrapper.unmount()
  })

})
