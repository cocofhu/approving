// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { mount, flushPromises } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import pages from '@/locales/zh-CN/pages.json'
import MarkdownSplitEditor from './MarkdownSplitEditor.vue'

vi.mock('@/lib/markdown', () => ({
  renderMarkdown: (body: string) => `<p>${body}</p>`,
}))

const layoutSpy = vi.fn()

const CodeEditorStub = {
  name: 'CodeEditor',
  props: ['modelValue', 'language', 'minimap', 'readonly'],
  emits: ['update:modelValue', 'scroll', 'ready'],
  template: '<div data-testid="code-editor-stub" />',
  methods: {
    getEditor() {
      return { layout: layoutSpy }
    },
  },
}

const SPLIT_KEY = 'agent-md-split-ratio'
const PREVIEW_COLLAPSED_KEY = 'agent-md-preview-collapsed'

const md = `---
description: demo
alwaysApply: false
---

# Hello
`

function mountEditor(props: Record<string, unknown> = {}) {
  const i18n = createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...pages } },
  })
  return mount(MarkdownSplitEditor, {
    props: {
      modelValue: md,
      filePath: 'rules/demo.md',
      variant: 'split',
      ...props,
    },
    global: {
      plugins: [i18n],
      stubs: { Icon: true, CodeEditor: CodeEditorStub },
    },
    attachTo: document.body,
  })
}

describe('MarkdownSplitEditor preview collapse', () => {
  beforeEach(() => {
    localStorage.clear()
    layoutSpy.mockClear()
  })

  afterEach(() => {
    localStorage.clear()
  })

  it('defaults to expanded preview on desktop split', async () => {
    const wrapper = mountEditor()
    await flushPromises()
    expect(wrapper.find('[data-testid="md-preview-panel"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="md-preview-sash"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="md-preview-collapse"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="md-preview-expand-rail"]').exists()).toBe(false)
    expect(localStorage.getItem(PREVIEW_COLLAPSED_KEY)).toBeNull()
    wrapper.unmount()
  })

  it('collapses preview: hides sash/panel and shows expand rail', async () => {
    const wrapper = mountEditor()
    await flushPromises()
    await wrapper.get('[data-testid="md-preview-collapse"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="md-preview-panel"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="md-preview-sash"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="md-preview-expand-rail"]').exists()).toBe(true)
    expect(localStorage.getItem(PREVIEW_COLLAPSED_KEY)).toBe('true')
    wrapper.unmount()
  })

  it('expands preview from the narrow rail', async () => {
    localStorage.setItem(PREVIEW_COLLAPSED_KEY, 'true')
    const wrapper = mountEditor()
    await flushPromises()
    expect(wrapper.find('[data-testid="md-preview-expand-rail"]').exists()).toBe(true)
    await wrapper.get('[data-testid="md-preview-expand-rail"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="md-preview-panel"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="md-preview-sash"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="md-preview-expand-rail"]').exists()).toBe(false)
    expect(localStorage.getItem(PREVIEW_COLLAPSED_KEY)).toBe('false')
    wrapper.unmount()
  })

  it('loads collapsed preference from localStorage and falls back when corrupted', async () => {
    localStorage.setItem(PREVIEW_COLLAPSED_KEY, 'true')
    const collapsed = mountEditor()
    await flushPromises()
    expect(collapsed.find('[data-testid="md-preview-expand-rail"]').exists()).toBe(true)
    collapsed.unmount()

    localStorage.setItem(PREVIEW_COLLAPSED_KEY, 'not-a-bool')
    const fallback = mountEditor()
    await flushPromises()
    // only exact 'true' collapses; anything else → expanded
    expect(fallback.find('[data-testid="md-preview-panel"]').exists()).toBe(true)
    fallback.unmount()
  })

  it('falls back to expanded when localStorage getItem throws', async () => {
    const spy = vi.spyOn(Storage.prototype, 'getItem').mockImplementation((key: string) => {
      if (key === PREVIEW_COLLAPSED_KEY) throw new Error('blocked')
      return null
    })
    const wrapper = mountEditor()
    await flushPromises()
    expect(wrapper.find('[data-testid="md-preview-panel"]').exists()).toBe(true)
    spy.mockRestore()
    wrapper.unmount()
  })

  it('does not reset previewCollapsed when filePath changes', async () => {
    const wrapper = mountEditor()
    await flushPromises()
    await wrapper.get('[data-testid="md-preview-collapse"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="md-preview-expand-rail"]').exists()).toBe(true)
    await wrapper.setProps({ filePath: 'rules/other.md' })
    await flushPromises()
    expect(wrapper.find('[data-testid="md-preview-expand-rail"]').exists()).toBe(true)
    expect(localStorage.getItem(PREVIEW_COLLAPSED_KEY)).toBe('true')
    wrapper.unmount()
  })

  it('restores prior split ratio after expand without rewriting ratio on collapse', async () => {
    localStorage.setItem(SPLIT_KEY, '0.4')
    const wrapper = mountEditor()
    await flushPromises()

    const editCol = () => wrapper.findAll('.flex.h-full.min-h-0.min-w-0.flex-col')[0]
    expect(editCol().attributes('style')).toContain('40%')

    await wrapper.get('[data-testid="md-preview-collapse"]').trigger('click')
    await flushPromises()
    expect(localStorage.getItem(SPLIT_KEY)).toBe('0.4')
    const collapsedStyle = editCol().attributes('style') || ''
    expect(
      /flex:\s*1\s+1\s+auto/.test(collapsedStyle)
      || (collapsedStyle.includes('flex-grow: 1') && collapsedStyle.includes('flex-basis: auto')),
    ).toBe(true)

    await wrapper.get('[data-testid="md-preview-expand-rail"]').trigger('click')
    await flushPromises()
    expect(editCol().attributes('style')).toContain('40%')
    expect(localStorage.getItem(SPLIT_KEY)).toBe('0.4')
    wrapper.unmount()
  })

  it('dragging sash to ratio extremes does not auto-collapse', async () => {
    const wrapper = mountEditor()
    await flushPromises()

    const splitBody = wrapper.find('div.flex.min-h-0.flex-1')
    const el = splitBody.element as HTMLElement
    vi.spyOn(el, 'getBoundingClientRect').mockReturnValue({
      left: 0,
      width: 1000,
      top: 0,
      right: 1000,
      bottom: 400,
      height: 400,
      x: 0,
      y: 0,
      toJSON: () => ({}),
    })

    await wrapper.get('[data-testid="md-preview-sash"]').trigger('mousedown', { clientX: 500 })
    document.dispatchEvent(new MouseEvent('mousemove', { clientX: 100 })) // → 0.10 → clamp 0.28
    document.dispatchEvent(new MouseEvent('mouseup'))
    await flushPromises()
    expect(wrapper.find('[data-testid="md-preview-panel"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="md-preview-expand-rail"]').exists()).toBe(false)
    expect(localStorage.getItem(SPLIT_KEY)).toBe('0.28')

    await wrapper.get('[data-testid="md-preview-sash"]').trigger('mousedown', { clientX: 280 })
    document.dispatchEvent(new MouseEvent('mousemove', { clientX: 900 })) // → 0.90 → clamp 0.72
    document.dispatchEvent(new MouseEvent('mouseup'))
    await flushPromises()
    expect(wrapper.find('[data-testid="md-preview-panel"]').exists()).toBe(true)
    expect(localStorage.getItem(SPLIT_KEY)).toBe('0.72')
    wrapper.unmount()
  })

  it('stack variant has no desktop collapse rail or collapse control', async () => {
    const wrapper = mountEditor({ variant: 'stack' })
    await flushPromises()
    expect(wrapper.find('[data-testid="md-preview-collapse"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="md-preview-expand-rail"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="md-preview-sash"]').exists()).toBe(false)
    expect(wrapper.text()).toContain('编辑')
    expect(wrapper.text()).toContain('预览')
    wrapper.unmount()
  })
})
