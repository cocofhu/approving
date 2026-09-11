// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'
import HomePipelineSelect from './HomePipelineSelect.vue'

function mountSelect(props: {
  pipelines?: { id: string; name: string; projectName?: string }[]
  modelValue?: string
  disabled?: boolean
} = {}) {
  const i18n = createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...common, ...pages } },
  })
  return mount(HomePipelineSelect, {
    props: {
      pipelines: props.pipelines ?? [
        { id: 'wf-a', name: '自我迭代PRO' },
        { id: 'wf-b', name: 'AnimeFind迭代' },
      ],
      modelValue: props.modelValue ?? 'wf-a',
      disabled: props.disabled ?? false,
    },
    attachTo: document.body,
    global: {
      plugins: [i18n],
      stubs: { Teleport: false },
    },
  })
}

describe('HomePipelineSelect', () => {
  afterEach(() => {
    document.body.innerHTML = ''
  })

  // plan g2.2 — theme token class names
  it('uses theme token classes for panel surface and option states', async () => {
    const wrapper = mountSelect({})
    await flushPromises()
    await wrapper.get('[data-testid="home-pipeline-select-trigger"]').trigger('click')
    await flushPromises()
    const panel = document.querySelector('[data-testid="home-pipeline-select-panel"]')
    expect(panel).toBeTruthy()
    expect(panel!.classList.contains('home-pipeline-select__panel')).toBe(true)
    const opt = document.querySelector('[data-testid="home-pipeline-select-option-wf-a"]')
    expect(opt).toBeTruthy()
    expect(opt!.classList.contains('home-pipeline-select__opt--current')).toBe(true)
    wrapper.unmount()
  })

  // plan g2.1 — downward placement (top below trigger), not bottom/upward
  it('positions panel below trigger via top placement (not bottom/upward)', async () => {
    const wrapper = mountSelect({})
    const triggerEl = wrapper.get('[data-testid="home-pipeline-select-trigger"]').element as HTMLElement
    vi.spyOn(triggerEl, 'getBoundingClientRect').mockReturnValue({
      top: 200,
      bottom: 232,
      left: 40,
      right: 200,
      width: 160,
      height: 32,
      x: 40,
      y: 200,
      toJSON() {
        return {}
      },
    })
    await flushPromises()
    await wrapper.get('[data-testid="home-pipeline-select-trigger"]').trigger('click')
    await flushPromises()
    const panel = document.querySelector(
      '[data-testid="home-pipeline-select-panel"]',
    ) as HTMLElement | null
    expect(panel).toBeTruthy()
    expect(panel!.getAttribute('data-placement')).toBe('below')
    expect(panel!.style.position).toBe('fixed')
    // trigger bottom 232 + 6px gap → top 238 (downward); must not use bottom anchoring
    expect(panel!.style.top).toBe('238px')
    expect(panel!.style.bottom).toBe('')
    // Teleported to body (escapes overflow clip)
    expect(panel!.parentElement).toBe(document.body)
    wrapper.unmount()
  })

  // plan g1.2 — panel remains visible when ancestors use overflow-y-auto
  it('teleports panel to body so overflow clipping containers do not hide it', async () => {
    const clip = document.createElement('div')
    clip.className = 'home-shell__content'
    clip.style.overflowY = 'auto'
    clip.style.height = '120px'
    document.body.appendChild(clip)

    const i18n = createI18n({
      legacy: false,
      locale: 'zh-CN',
      messages: { 'zh-CN': { ...common, ...pages } },
    })
    const wrapper = mount(HomePipelineSelect, {
      props: {
        pipelines: [
          { id: 'wf-a', name: '自我迭代PRO' },
          { id: 'wf-b', name: 'AnimeFind迭代' },
        ],
        modelValue: 'wf-a',
      },
      attachTo: clip,
      global: {
        plugins: [i18n],
        stubs: { Teleport: false },
      },
    })
    await flushPromises()
    await wrapper.get('[data-testid="home-pipeline-select-trigger"]').trigger('click')
    await flushPromises()
    const panel = document.querySelector('[data-testid="home-pipeline-select-panel"]')
    expect(panel).toBeTruthy()
    expect(clip.contains(panel)).toBe(false)
    expect(document.body.contains(panel)).toBe(true)
    expect(panel!.querySelector('[data-testid="home-pipeline-select-search"]')).toBeTruthy()
    wrapper.unmount()
    clip.remove()
  })

  // plan g2.2 — search only looks at the already-filtered Home list
  it('does not surface a pipeline omitted from props even when the query matches its name', async () => {
    const wrapper = mountSelect({
      pipelines: [{ id: 'wf-a', name: '自我迭代PRO' }],
      modelValue: 'wf-a',
    })
    await flushPromises()
    await wrapper.get('[data-testid="home-pipeline-select-trigger"]').trigger('click')
    await flushPromises()
    const search = document.querySelector(
      '[data-testid="home-pipeline-select-search"]',
    ) as HTMLInputElement
    expect(search).toBeTruthy()
    search.value = '内部副本'
    search.dispatchEvent(new Event('input'))
    await flushPromises()
    expect(document.querySelector('[data-testid="home-pipeline-select-option-wf-hidden"]')).toBeNull()
    expect(document.querySelector('[data-testid="home-pipeline-select-empty"]')).toBeTruthy()
    wrapper.unmount()
  })

  // plan g2.2 — trigger "项目名 · 工作流名"; search matches projectName; no empty · prefix
  it('shows projectName · workflow name on the trigger', async () => {
    const wrapper = mountSelect({
      pipelines: [
        { id: 'wf-a', name: '默认工作流', projectName: '综合项目组' },
        { id: 'wf-b', name: '默认工作流', projectName: 'SkillHub' },
      ],
      modelValue: 'wf-a',
    })
    await flushPromises()
    expect(wrapper.get('[data-testid="home-pipeline-select-trigger"]').text()).toContain(
      '综合项目组 · 默认工作流',
    )
    wrapper.unmount()
  })

  it('does not prefix the trigger with an empty · when projectName is missing', async () => {
    const wrapper = mountSelect({
      pipelines: [{ id: 'wf-a', name: '自我迭代PRO' }],
      modelValue: 'wf-a',
    })
    await flushPromises()
    const text = wrapper.get('[data-testid="home-pipeline-select-trigger"]').text()
    expect(text).toContain('自我迭代PRO')
    expect(text).not.toContain('·')
    wrapper.unmount()
  })

  it('filters options by project name substring and highlights both fields', async () => {
    const wrapper = mountSelect({
      pipelines: [
        { id: 'wf-a', name: '默认工作流', projectName: '综合项目组' },
        { id: 'wf-b', name: '默认工作流', projectName: 'SkillHub' },
      ],
      modelValue: 'wf-a',
    })
    await flushPromises()
    await wrapper.get('[data-testid="home-pipeline-select-trigger"]').trigger('click')
    await flushPromises()
    const optA = document.querySelector('[data-testid="home-pipeline-select-option-wf-a"]')
    expect(optA?.querySelector('.home-pipeline-select__opt-name')?.textContent).toBe('默认工作流')
    expect(optA?.querySelector('.home-pipeline-select__opt-project')?.textContent).toBe('综合项目组')
    const search = document.querySelector(
      '[data-testid="home-pipeline-select-search"]',
    ) as HTMLInputElement
    search.value = 'Skill'
    search.dispatchEvent(new Event('input'))
    await flushPromises()
    expect(document.querySelector('[data-testid="home-pipeline-select-option-wf-a"]')).toBeNull()
    const optB = document.querySelector('[data-testid="home-pipeline-select-option-wf-b"]')
    expect(optB).toBeTruthy()
    expect(optB!.querySelector('mark')?.textContent).toBe('Skill')
    wrapper.unmount()
  })

  it('shows the no-match empty state when neither name nor projectName hits', async () => {
    const wrapper = mountSelect({
      pipelines: [{ id: 'wf-a', name: '默认工作流', projectName: '综合项目组' }],
      modelValue: 'wf-a',
    })
    await flushPromises()
    await wrapper.get('[data-testid="home-pipeline-select-trigger"]').trigger('click')
    await flushPromises()
    const search = document.querySelector(
      '[data-testid="home-pipeline-select-search"]',
    ) as HTMLInputElement
    search.value = 'zzzz'
    search.dispatchEvent(new Event('input'))
    await flushPromises()
    expect(document.querySelector('[data-testid="home-pipeline-select-empty"]')).toBeTruthy()
    // plan g1.3 — empty search still shows create footer
    expect(document.querySelector('[data-testid="home-pipeline-select-create"]')).toBeTruthy()
    wrapper.unmount()
  })

  // plan g1.1 / g3.1 / g3.3 — sticky create footer with addCard copy
  it('renders a pinned create footer with 新建工作流 under the option list', async () => {
    const wrapper = mountSelect({})
    await flushPromises()
    await wrapper.get('[data-testid="home-pipeline-select-trigger"]').trigger('click')
    await flushPromises()
    const create = document.querySelector(
      '[data-testid="home-pipeline-select-create"]',
    ) as HTMLElement | null
    expect(create).toBeTruthy()
    expect(create!.textContent).toContain('新建工作流')
    expect(create!.getAttribute('aria-selected')).toBeNull()
    expect(create!.classList.contains('home-pipeline-select__opt--current')).toBe(false)
    const panel = document.querySelector('[data-testid="home-pipeline-select-panel"]') as HTMLElement
    expect(panel.lastElementChild).toBe(create)
    wrapper.unmount()
  })

  // plan g2.1 / g3.1 — click create emits create and closes panel
  it('emits create and closes the panel when the footer is clicked', async () => {
    const wrapper = mountSelect({})
    await flushPromises()
    await wrapper.get('[data-testid="home-pipeline-select-trigger"]').trigger('click')
    await flushPromises()
    const create = document.querySelector(
      '[data-testid="home-pipeline-select-create"]',
    ) as HTMLButtonElement
    create.click()
    await flushPromises()
    expect(wrapper.emitted('create')).toBeTruthy()
    expect(wrapper.emitted('create')!.length).toBe(1)
    expect(document.querySelector('[data-testid="home-pipeline-select-panel"]')).toBeNull()
    wrapper.unmount()
  })

  // plan g1.3 / g3.1 — zero pipelines: trigger opens create-only panel
  it('opens a create-only panel when pipelines is empty and not disabled', async () => {
    const wrapper = mountSelect({ pipelines: [], modelValue: '' })
    await flushPromises()
    const trigger = wrapper.get('[data-testid="home-pipeline-select-trigger"]')
    expect((trigger.element as HTMLButtonElement).disabled).toBe(false)
    await trigger.trigger('click')
    await flushPromises()
    expect(document.querySelector('[data-testid="home-pipeline-select-panel"]')).toBeTruthy()
    expect(document.querySelector('[data-testid="home-pipeline-select-empty"]')).toBeTruthy()
    expect(document.querySelector('[data-testid="home-pipeline-select-create"]')).toBeTruthy()
    wrapper.unmount()
  })

  it('keeps the trigger disabled while disabled prop is true (sending)', async () => {
    const wrapper = mountSelect({ pipelines: [], modelValue: '', disabled: true })
    await flushPromises()
    expect(
      (wrapper.get('[data-testid="home-pipeline-select-trigger"]').element as HTMLButtonElement)
        .disabled,
    ).toBe(true)
    await wrapper.get('[data-testid="home-pipeline-select-trigger"]').trigger('click')
    await flushPromises()
    expect(document.querySelector('[data-testid="home-pipeline-select-panel"]')).toBeNull()
    wrapper.unmount()
  })

  // plan g1.2 / g3.1 — ArrowDown from last option lands on create; Enter emits create
  it('moves keyboard highlight onto create after the last option and Enter emits create', async () => {
    const wrapper = mountSelect({})
    await flushPromises()
    await wrapper.get('[data-testid="home-pipeline-select-trigger"]').trigger('click')
    await flushPromises()
    const search = document.querySelector(
      '[data-testid="home-pipeline-select-search"]',
    ) as HTMLInputElement
    // open sets activeIndex to selected (0); ArrowDown → 1 (wf-b); ArrowDown → create
    search.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowDown', bubbles: true }))
    search.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowDown', bubbles: true }))
    await flushPromises()
    const create = document.querySelector(
      '[data-testid="home-pipeline-select-create"]',
    ) as HTMLElement
    expect(create.classList.contains('home-pipeline-select__create--active')).toBe(true)
    search.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true }))
    await flushPromises()
    expect(wrapper.emitted('create')).toBeTruthy()
    expect(document.querySelector('[data-testid="home-pipeline-select-panel"]')).toBeNull()
    wrapper.unmount()
  })

  // plan g1.2 — no matches: Enter triggers create
  it('triggers create on Enter when search has no matches', async () => {
    const wrapper = mountSelect({})
    await flushPromises()
    await wrapper.get('[data-testid="home-pipeline-select-trigger"]').trigger('click')
    await flushPromises()
    const search = document.querySelector(
      '[data-testid="home-pipeline-select-search"]',
    ) as HTMLInputElement
    search.value = 'zzzz-no-match'
    search.dispatchEvent(new Event('input'))
    await flushPromises()
    search.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true }))
    await flushPromises()
    expect(wrapper.emitted('create')).toBeTruthy()
    wrapper.unmount()
  })

  // plan g1.2 — Escape only closes the panel
  it('closes the panel on Escape without emitting create', async () => {
    const wrapper = mountSelect({})
    await flushPromises()
    await wrapper.get('[data-testid="home-pipeline-select-trigger"]').trigger('click')
    await flushPromises()
    const search = document.querySelector(
      '[data-testid="home-pipeline-select-search"]',
    ) as HTMLInputElement
    search.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    await flushPromises()
    expect(document.querySelector('[data-testid="home-pipeline-select-panel"]')).toBeNull()
    expect(wrapper.emitted('create')).toBeFalsy()
    wrapper.unmount()
  })
})
