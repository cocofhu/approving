// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { flushPromises, mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'
import ReviewComposer from './ReviewComposer.vue'

function mountGate(opts: {
  thinking?: boolean
  streamText?: string
  streamThought?: string
  interrupted?: boolean
  streamCompletedAt?: string | null
} = {}) {
  const i18n = createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...common, ...pages } },
  })
  return mount(ReviewComposer, {
    props: {
      mode: 'gate',
      thinking: opts.thinking ?? false,
      streamText: opts.streamText ?? '',
      streamThought: opts.streamThought ?? '',
      interrupted: opts.interrupted ?? false,
      streamCompletedAt: opts.streamCompletedAt ?? null,
      canReject: true,
      canPass: true,
    },
    global: {
      plugins: [i18n],
      stubs: { Icon: true, ParagraphInput: true, AnnotationChip: true, ClarifyChat: true },
    },
  })
}

describe('ReviewComposer gate busy status (C-tier)', () => {
  it('thinking with empty stream shows 思考中… placeholder (no air bubble)', async () => {
    const wrapper = mountGate({ thinking: true })
    await flushPromises()
    const placeholder = wrapper.find('[data-testid="gate-busy-placeholder"]')
    expect(wrapper.find('[data-testid="gate-react-stream"]').exists()).toBe(true)
    expect(placeholder.exists()).toBe(true)
    expect(placeholder.text()).toContain('思考中')
    expect(placeholder.find('.typing-dots').exists()).toBe(true)
    wrapper.unmount()
  })

  it('thought is visible and default-open', async () => {
    const wrapper = mountGate({
      thinking: true,
      streamThought: '旁路思考过程',
    })
    await flushPromises()
    const thought = wrapper.find('[data-testid="gate-react-thought"]')
    expect(thought.exists()).toBe(true)
    expect(thought.attributes('open')).toBeDefined()
    expect(thought.text()).toContain('旁路思考过程')
    expect(wrapper.find('[data-testid="thought-summary-state"]').attributes('data-state')).toBe(
      'streaming',
    )
    expect(wrapper.find('[data-testid="thought-summary-state"]').text()).toContain('生成中')
    expect(wrapper.find('[data-testid="gate-busy-status"]').text()).toContain('思考中')
    wrapper.unmount()
  })

  it('message switches status to 输出中, collapses thought, shows caret', async () => {
    const wrapper = mountGate({
      thinking: true,
      streamThought: '旁路思考',
      streamText: '旁路正文流',
    })
    await flushPromises()
    expect(wrapper.find('[data-testid="gate-busy-status"]').text()).toContain('输出中')
    expect(wrapper.find('[data-testid="gate-react-thought"]').text()).toContain('旁路思考')
    expect(wrapper.find('[data-testid="gate-react-thought"]').attributes('open')).toBeUndefined()
    // Message outputting — summary stays generating (not done).
    expect(wrapper.find('[data-testid="thought-summary-state"]').attributes('data-state')).toBe(
      'streaming',
    )
    expect(wrapper.find('[data-testid="thought-summary-state"]').text()).toContain('生成中')
    expect(wrapper.find('[data-testid="gate-stream-caret"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="gate-react-stream"]').text()).toContain('旁路正文流')
    wrapper.unmount()
  })

  it('thinking with neither text nor thought still shows placeholder (tool-dense safe)', async () => {
    const wrapper = mountGate({ thinking: true, streamText: '', streamThought: '' })
    await flushPromises()
    expect(wrapper.find('[data-testid="gate-busy-placeholder"]').exists()).toBe(true)
    expect(wrapper.text()).not.toMatch(/正在调用工具|读文件/)
    wrapper.unmount()
  })

  it('completed footnote shows without caret; interrupted never shows 已完成', async () => {
    const done = mountGate({
      thinking: false,
      streamThought: '思考',
      streamText: '正文',
      streamCompletedAt: new Date().toISOString(),
    })
    await flushPromises()
    expect(done.find('[data-testid="gate-turn-completed"]').exists()).toBe(true)
    expect(done.find('[data-testid="gate-turn-completed"]').text()).toContain('已完成')
    expect(done.find('[data-testid="gate-stream-caret"]').exists()).toBe(false)
    expect(done.find('[data-testid="thought-summary-state"]').attributes('data-state')).toBe('done')
    expect(done.find('[data-testid="thought-summary-state"]').text()).toContain('已完成')
    done.unmount()

    const bad = mountGate({
      thinking: false,
      streamThought: '半截思考',
      streamText: '半截',
      interrupted: true,
      streamCompletedAt: null,
    })
    await flushPromises()
    expect(bad.find('[data-testid="gate-turn-completed"]').exists()).toBe(false)
    expect(bad.text()).toContain('interrupted')
    expect(bad.find('[data-testid="thought-summary-state"]').attributes('data-state')).toBe(
      'interrupted',
    )
    expect(bad.find('[data-testid="thought-summary-state"]').text()).toContain('已中断')
    expect(bad.find('[data-testid="thought-summary-state"]').text()).not.toContain('已完成')
    bad.unmount()
  })

  it('idle (not thinking, no completed) hides stream panel', async () => {
    const wrapper = mountGate({
      thinking: false,
      streamText: '残留',
      streamThought: '残留思考',
    })
    await flushPromises()
    expect(wrapper.find('[data-testid="gate-react-stream"]').exists()).toBe(false)
    wrapper.unmount()
  })
})

describe('ReviewComposer nested ClarifyChat delivery', () => {
  it('applyAcpEvents returns false when ClarifyChat not mounted (gate mode)', async () => {
    const wrapper = mountGate({ thinking: true })
    await flushPromises()
    const vm = wrapper.vm as any
    expect(vm.applyAcpEvents?.([{ kind: 'message', text: 'x' }])).toBe(false)
    expect(vm.isChatReady?.()).toBe(false)
    wrapper.unmount()
  })
})

describe('ReviewComposer gate review semantics (send + confirm)', () => {
  it('shows 发送 + 确认并流转, not 打回/通过', async () => {
    const wrapper = mountGate()
    await flushPromises()
    expect(wrapper.find('[data-testid="review-composer-send"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="review-composer-pass"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="review-composer-send"]').text()).toContain('发送')
    expect(wrapper.find('[data-testid="review-composer-pass"]').text()).toContain('确认并流转')
    expect(wrapper.text()).not.toContain('打回修改')
    expect(wrapper.text()).not.toContain('通过并流转')
    wrapper.unmount()
  })

  it('cold session hides send and shows cold note; confirm remains', async () => {
    const i18n = createI18n({
      legacy: false,
      locale: 'zh-CN',
      messages: { 'zh-CN': { ...common, ...pages } },
    })
    const wrapper = mount(ReviewComposer, {
      props: {
        mode: 'gate',
        canReject: false,
        canPass: true,
        coldSession: true,
      },
      global: {
        plugins: [i18n],
        stubs: { Icon: true, ParagraphInput: true, AnnotationChip: true, ClarifyChat: true },
      },
    })
    await flushPromises()
    expect(wrapper.find('[data-testid="review-composer-send"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="review-composer-pass"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="review-composer-cold-note"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="review-composer-cold-note"]').text()).toContain('无法继续就地改码')
    wrapper.unmount()
  })
})
