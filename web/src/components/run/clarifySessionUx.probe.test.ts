// @vitest-environment happy-dom
/**
 * Acceptance probes: ClarifyChat non-reviewMode (需求澄清) queue / stream /
 * Cancel keep-queue / refresh resume / IME — aligned with Demo「修复后」.
 */
import { createI18n } from 'vue-i18n'
import { flushPromises, mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import { beforeEach, describe, expect, it } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'
import ClarifyChat from './ClarifyChat.vue'
import AnnotationChip from './AnnotationChip.vue'
import type { ReactAnnotation } from '@/lib/types'

function mountClarify(nodeId = 'clarify') {
  const i18n = createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...common, ...pages } },
  })
  return mount(ClarifyChat, {
    props: {
      runId: 'run-1',
      nodeId,
      iteration: 1,
      turns: [],
      done: false,
      active: true,
      reviewMode: false,
      annotateEnabled: true,
      hideFinish: true,
      sendLabel: '发送澄清回复',
    },
    global: {
      plugins: [i18n],
      stubs: { Icon: true, ClarifyDemoFrame: true },
    },
  })
}

async function sendText(wrapper: ReturnType<typeof mountClarify>, text: string) {
  await wrapper.find('textarea').setValue(text)
  await wrapper.find('[data-testid="clarify-send-label"]').trigger('click')
  await flushPromises()
}

describe('[approving] ClarifyChat clarify-path UX (non-reviewMode)', () => {
  beforeEach(() => {
    sessionStorage.clear()
  })

  it('enqueue shows queue; Cancel keeps queue (Demo keep-queue)', async () => {
    const w = mountClarify()
    await sendText(w, '澄清意见甲')
    await sendText(w, '澄清意见乙')
    const queue = w.find('[data-testid="clarify-review-queue"]')
    expect(queue.exists()).toBe(true)
    expect(queue.text()).toContain('澄清意见甲')
    expect(queue.text()).toContain('澄清意见乙')

    const vm = w.vm as unknown as {
      applyReviewFrame: (f: Record<string, unknown>) => void
      applyAcpEvents: (e: { kind: string; text: string }[]) => void
    }
    vm.applyReviewFrame({ event: 'turn_begin', item: { text: '澄清意见甲' }, nodeId: 'clarify' })
    await nextTick()
    vm.applyAcpEvents([{ kind: 'message', text: '流式正文增量' }])
    await nextTick()
    expect(w.text()).toContain('流式正文增量')

    await w.find('[data-testid="clarify-review-cancel"]').trigger('click')
    await flushPromises()
    // Demo: queue kept (乙 still pending); interrupted marker on current turn.
    expect(w.find('[data-testid="clarify-review-queue"]').text()).toContain('澄清意见乙')
    expect(w.find('[data-testid="clarify-interrupted"]').exists()).toBe(true)
    expect(w.emitted('cancel')).toBeTruthy()
    w.unmount()
  })

  it('real pump order: queue_state(remaining) then turn_begin(active) binds 甲 not 乙', async () => {
    // Server pump: queue_state items=剩余(不含 active) → turn_begin item=当前轮.
    // Blind shift after trim would wrongly show 乙 as live human (review v1).
    const w = mountClarify()
    await sendText(w, '澄清意见甲')
    await sendText(w, '澄清意见乙')
    const vm = w.vm as unknown as {
      applyReviewFrame: (f: Record<string, unknown>) => void
    }
    vm.applyReviewFrame({
      event: 'queue_state',
      nodeId: 'clarify',
      waiting: 1,
      items: [{ text: '澄清意见乙' }],
      busy: true,
    })
    await nextTick()
    vm.applyReviewFrame({
      event: 'turn_begin',
      nodeId: 'clarify',
      item: { text: '澄清意见甲' },
    })
    await nextTick()
    const scroller = w.find('[data-testid="clarify-scroller"]').text()
    const humanMatches = scroller.match(/澄清意见甲/g) || []
    expect(humanMatches.length).toBe(1)
    expect(scroller).not.toMatch(/澄清意见乙/)
    expect(w.find('[data-testid="clarify-review-queue"]').text()).toContain('澄清意见乙')
    expect(w.find('[data-testid="clarify-review-queue"]').text()).not.toContain('澄清意见甲')
    w.unmount()
  })

  it('refresh resume via queue_state busy+activeItem recreates live stream bubble', async () => {
    const w = mountClarify()
    const vm = w.vm as unknown as {
      applyReviewFrame: (f: Record<string, unknown>) => void
      applyAcpEvents: (e: { kind: string; text: string }[]) => void
    }
    vm.applyReviewFrame({
      event: 'queue_state',
      nodeId: 'clarify',
      waiting: 1,
      items: [{ text: '排队中的下一条' }],
      busy: true,
      activeItem: { text: '刷新前进行中的提问' },
    })
    await nextTick()
    expect(w.text()).toContain('刷新前进行中的提问')
    expect(w.find('[data-testid="clarify-review-queue"]').text()).toContain('排队中的下一条')
    vm.applyAcpEvents([{ kind: 'message', text: '续上的流式正文' }])
    await nextTick()
    expect(w.text()).toContain('续上的流式正文')
    w.unmount()
  })

  it('hard-refresh seed: thought-only then message after remount (g4.1)', async () => {
    // Simulate host seed-then-live: rebuild slot → seed ACP after (re)mount.
    const w = mountClarify()
    const vm = w.vm as unknown as {
      applyReviewFrame: (f: Record<string, unknown>) => void
      applyAcpEvents: (e: { kind: string; text: string }[]) => void
    }
    vm.applyReviewFrame({
      event: 'queue_state',
      nodeId: 'clarify',
      waiting: 0,
      items: [],
      busy: true,
      activeItem: { text: '硬刷新前的提问' },
    })
    await nextTick()
    expect(w.find('[data-testid="clarify-busy-placeholder"]').exists()).toBe(true)

    // Seed thought recovered from LiveNodeEvents / pending ACP buffer.
    vm.applyAcpEvents([{ kind: 'thought', text: '已恢复的思考增量' }])
    await nextTick()
    expect(w.find('[data-testid="clarify-busy-placeholder"]').exists()).toBe(false)
    expect(w.find('[data-testid="clarify-thought"]').text()).toContain('已恢复的思考增量')
    expect(w.find('[data-testid="clarify-busy-status"]').text()).toContain('思考中')

    vm.applyAcpEvents([
      { kind: 'thought', text: '已恢复的思考增量' },
      { kind: 'message', text: '续流正文' },
    ])
    await nextTick()
    expect(w.text()).toContain('续流正文')
    expect(w.find('[data-testid="clarify-busy-status"]').text()).toContain('输出中')
    expect(w.find('[data-testid="clarify-stream-caret"]').exists()).toBe(true)
    w.unmount()
  })

  it('IME composing Enter does not send', async () => {
    const w = mountClarify()
    const ta = w.find('[data-testid="clarify-input"]')
    await ta.setValue('中文输入')
    await ta.trigger('compositionstart')
    await ta.trigger('keydown', { key: 'Enter', keyCode: 229, isComposing: true })
    await flushPromises()
    expect(w.emitted('send')).toBeFalsy()
    await ta.trigger('compositionend')
    await ta.trigger('keydown', { key: 'Enter', keyCode: 13, isComposing: false })
    await flushPromises()
    expect(w.emitted('send')).toBeTruthy()
    w.unmount()
  })

  it('turn_begin prefers queue item id over duplicate text', async () => {
    const w = mountClarify()
    // Two identical texts — text-only match would be ambiguous.
    await sendText(w, '同一文案')
    await sendText(w, '同一文案')
    const vm = w.vm as unknown as {
      applyReviewFrame: (f: Record<string, unknown>) => void
    }
    // Authoritative reconcile assigns server ids before turn_begin.
    vm.applyReviewFrame({
      event: 'queue_state',
      nodeId: 'clarify',
      waiting: 2,
      items: [
        { id: 'id-a', text: '同一文案' },
        { id: 'id-b', text: '同一文案' },
      ],
      busy: false,
    })
    await nextTick()
    // Pump: remaining = id-b only, then turn_begin starts id-a (not in queue).
    vm.applyReviewFrame({
      event: 'queue_state',
      nodeId: 'clarify',
      waiting: 1,
      items: [{ id: 'id-b', text: '同一文案' }],
      busy: true,
    })
    await nextTick()
    vm.applyReviewFrame({
      event: 'turn_begin',
      nodeId: 'clarify',
      item: { id: 'id-a', text: '同一文案' },
    })
    await nextTick()
    const queue = w.find('[data-testid="clarify-review-queue"]')
    expect(queue.exists()).toBe(true)
    // id-a was already trimmed; must NOT steal id-b via text fallback.
    expect(queue.findAll('[data-testid="clarify-queue-item"]').length).toBe(1)
    expect(queue.text()).toContain('同一文案')
    w.unmount()
  })

  it('turn_begin removes matching id from local queue when still present', async () => {
    const w = mountClarify()
    await sendText(w, '同一文案')
    await sendText(w, '同一文案')
    const vm = w.vm as unknown as {
      applyReviewFrame: (f: Record<string, unknown>) => void
    }
    vm.applyReviewFrame({
      event: 'queue_state',
      nodeId: 'clarify',
      waiting: 2,
      items: [
        { id: 'id-a', text: '同一文案' },
        { id: 'id-b', text: '同一文案' },
      ],
      busy: false,
    })
    await nextTick()
    // turn_begin before trim (id still in local queue) — remove by id.
    vm.applyReviewFrame({
      event: 'turn_begin',
      nodeId: 'clarify',
      item: { id: 'id-a', text: '同一文案' },
    })
    await nextTick()
    expect(w.findAll('[data-testid="clarify-queue-item"]').length).toBe(1)
    w.unmount()
  })

  it('AnnotationChip path===label shows once', () => {
    const i18n = createI18n({
      legacy: false,
      locale: 'zh-CN',
      messages: { 'zh-CN': { ...common, ...pages } },
    })
    const ann: ReactAnnotation = { label: '#launchInput', selector: '#launchInput' }
    const w = mount(AnnotationChip, {
      props: { ann, testId: 'chip-dup' },
      global: { plugins: [i18n], stubs: { Icon: true } },
    })
    const text = w.text()
    const matches = text.match(/#launchInput/g) || []
    expect(matches.length).toBe(1)
    w.unmount()
  })

  it('AnnotationChip path!==label shows both', () => {
    const i18n = createI18n({
      legacy: false,
      locale: 'zh-CN',
      messages: { 'zh-CN': { ...common, ...pages } },
    })
    const ann: ReactAnnotation = { label: '启动输入', jsonPath: '#launchInput' }
    const w = mount(AnnotationChip, {
      props: { ann },
      global: { plugins: [i18n], stubs: { Icon: true } },
    })
    expect(w.text()).toContain('#launchInput')
    expect(w.text()).toContain('启动输入')
    w.unmount()
  })

  it('props.turns catching human while live streaming does not double human bubble', async () => {
    const i18n = createI18n({
      legacy: false,
      locale: 'zh-CN',
      messages: { 'zh-CN': { ...common, ...pages } },
    })
    const w = mount(ClarifyChat, {
      props: {
        runId: 'run-1',
        nodeId: 'clarify',
        iteration: 1,
        turns: [],
        done: false,
        active: true,
        reviewMode: false,
        annotateEnabled: true,
        hideFinish: true,
        sendLabel: '发送澄清回复',
      },
      global: {
        plugins: [i18n],
        stubs: { Icon: true, ClarifyDemoFrame: true },
      },
    })
    const vm = w.vm as unknown as {
      applyReviewFrame: (f: Record<string, unknown>) => void
      applyAcpEvents: (e: { kind: string; text: string }[]) => void
    }
    vm.applyReviewFrame({
      event: 'turn_begin',
      item: { text: '澄清意见甲' },
      nodeId: 'clarify',
    })
    await nextTick()
    vm.applyAcpEvents([{ kind: 'message', text: '流式正文' }])
    await nextTick()
    // Host softRefresh/loadRun caught up with persisted human mid-stream.
    await w.setProps({
      turns: [{ role: 'human', text: '澄清意见甲', at: '2026-07-27T00:00:00Z' }],
    })
    await nextTick()
    const body = w.find('[data-testid="clarify-scroller"]').text()
    const matches = body.match(/澄清意见甲/g) || []
    expect(matches.length).toBe(1)
    expect(body).toContain('流式正文')
    w.unmount()
  })
})
