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
})
