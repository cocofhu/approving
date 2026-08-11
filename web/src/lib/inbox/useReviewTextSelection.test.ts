// @vitest-environment happy-dom
import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest'
import { defineComponent, h, nextTick, ref } from 'vue'
import { mount } from '@vue/test-utils'
import { useReviewTextSelection } from './useReviewTextSelection'

function selectTextIn(el: HTMLElement) {
  const range = document.createRange()
  range.selectNodeContents(el)
  const sel = window.getSelection()
  sel?.removeAllRanges()
  sel?.addRange(range)
  document.dispatchEvent(new Event('selectionchange'))
}

describe('useReviewTextSelection', () => {
  beforeEach(() => {
    vi.useFakeTimers()
  })
  afterEach(() => {
    vi.useRealTimers()
    window.getSelection()?.removeAllRanges()
  })

  it('caches same-field selection and preserves it on toolbar mousedown', async () => {
    const root = ref<HTMLElement | null>(null)
    const enabled = ref(true)
    let api: ReturnType<typeof useReviewTextSelection> | null = null

    const Comp = defineComponent({
      setup() {
        api = useReviewTextSelection({ enabled, root })
        return () =>
          h('div', { ref: (el) => { root.value = el as HTMLElement } }, [
            h('span', { 'data-json-path': 'summary', 'data-label': '概述' }, 'hello quote text'),
          ])
      },
    })
    const w = mount(Comp, { attachTo: document.body })
    await nextTick()
    const field = w.element.querySelector('[data-json-path="summary"]') as HTMLElement
    const range = document.createRange()
    range.setStart(field.firstChild!, 0)
    range.setEnd(field.firstChild!, field.textContent!.length)
    const sel = window.getSelection()
    sel?.removeAllRanges()
    sel?.addRange(range)
    document.dispatchEvent(new Event('selectionchange'))
    vi.advanceTimersByTime(20)
    await nextTick()

    expect(api!.cached.value?.quote).toContain('hello quote text')
    expect(api!.cached.value?.jsonPath).toBe('summary')
    expect(api!.visible.value).toBe(true)

    const ev = new MouseEvent('mousedown', { bubbles: true, cancelable: true })
    api!.preserveOnMouseDown(ev)
    expect(ev.defaultPrevented).toBe(true)
    expect(api!.takeSelection()?.quote).toContain('hello quote text')

    w.unmount()
  })

  it('rejects cross-field selection with callback and no toolbar', async () => {
    const root = ref<HTMLElement | null>(null)
    const enabled = ref(true)
    const onCrossField = vi.fn()
    let api: ReturnType<typeof useReviewTextSelection> | null = null

    const Comp = defineComponent({
      setup() {
        api = useReviewTextSelection({ enabled, root, onCrossField })
        return () =>
          h('div', { ref: (el) => { root.value = el as HTMLElement } }, [
            h('span', { 'data-json-path': 'a', id: 'fa' }, 'alpha field'),
            h('span', { 'data-json-path': 'b', id: 'fb' }, 'beta field'),
          ])
      },
    })
    const w = mount(Comp, { attachTo: document.body })
    await nextTick()

    const a = w.element.querySelector('#fa') as HTMLElement
    const b = w.element.querySelector('#fb') as HTMLElement
    const range = document.createRange()
    range.setStart(a.firstChild!, 0)
    range.setEnd(b.firstChild!, 4)
    const sel = window.getSelection()
    sel?.removeAllRanges()
    sel?.addRange(range)
    document.dispatchEvent(new Event('selectionchange'))
    vi.advanceTimersByTime(20)
    await nextTick()

    expect(api!.visible.value).toBe(false)
    expect(onCrossField).toHaveBeenCalled()
    expect(api!.takeSelection()).toBeNull()

    w.unmount()
  })

  it('does not show toolbar when disabled', async () => {
    const root = ref<HTMLElement | null>(null)
    const enabled = ref(false)
    let api: ReturnType<typeof useReviewTextSelection> | null = null

    const Comp = defineComponent({
      setup() {
        api = useReviewTextSelection({ enabled, root })
        return () =>
          h('div', { ref: (el) => { root.value = el as HTMLElement } }, [
            h('span', { 'data-json-path': 'summary' }, 'readonly text'),
          ])
      },
    })
    const w = mount(Comp, { attachTo: document.body })
    await nextTick()
    selectTextIn(w.element.querySelector('[data-json-path]') as HTMLElement)
    vi.advanceTimersByTime(20)
    await nextTick()
    expect(api!.visible.value).toBe(false)
    w.unmount()
  })

  it('hides toolbar shortly after selection is cleared', async () => {
    const root = ref<HTMLElement | null>(null)
    const enabled = ref(true)
    let api: ReturnType<typeof useReviewTextSelection> | null = null

    const Comp = defineComponent({
      setup() {
        api = useReviewTextSelection({ enabled, root })
        return () =>
          h('div', { ref: (el) => { root.value = el as HTMLElement } }, [
            h('span', { 'data-json-path': 'summary' }, 'clear me next'),
          ])
      },
    })
    const w = mount(Comp, { attachTo: document.body })
    await nextTick()
    selectTextIn(w.element.querySelector('[data-json-path]') as HTMLElement)
    vi.advanceTimersByTime(20)
    await nextTick()
    expect(api!.visible.value).toBe(true)

    window.getSelection()?.removeAllRanges()
    document.dispatchEvent(new Event('selectionchange'))
    vi.advanceTimersByTime(20)
    await nextTick()
    // Still visible during the short grace window (toolbar click).
    expect(api!.visible.value).toBe(true)
    vi.advanceTimersByTime(150)
    await nextTick()
    expect(api!.visible.value).toBe(false)
    w.unmount()
  })

  it('ignores selections inside textarea / composer hosts', async () => {
    const root = ref<HTMLElement | null>(null)
    const enabled = ref(true)
    let api: ReturnType<typeof useReviewTextSelection> | null = null

    const Comp = defineComponent({
      setup() {
        api = useReviewTextSelection({ enabled, root })
        return () =>
          h('div', { ref: (el) => { root.value = el as HTMLElement } }, [
            h('span', { 'data-json-path': 'summary' }, 'field text'),
            h('div', { 'data-review-composer': '' }, [
              h('textarea', { id: 'draft' }, 'composer draft text for quote'),
            ]),
          ])
      },
    })
    const w = mount(Comp, { attachTo: document.body })
    await nextTick()
    const ta = w.element.querySelector('#draft') as HTMLTextAreaElement
    ta.focus()
    ta.setSelectionRange(0, ta.value.length)
    // happy-dom may not sync textarea selection to window.getSelection; emulate
    // a range rooted in the textarea host when available, else skip DOM range.
    const range = document.createRange()
    if (ta.firstChild) {
      range.selectNodeContents(ta)
    } else {
      range.selectNode(ta)
    }
    const sel = window.getSelection()
    sel?.removeAllRanges()
    sel?.addRange(range)
    document.dispatchEvent(new Event('selectionchange'))
    vi.advanceTimersByTime(20)
    await nextTick()
    expect(api!.visible.value).toBe(false)
    expect(api!.cached.value).toBeNull()
    w.unmount()
  })
})
