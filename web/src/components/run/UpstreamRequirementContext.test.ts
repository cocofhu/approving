// @vitest-environment happy-dom
import { defineComponent, nextTick } from 'vue'
import { createI18n } from 'vue-i18n'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'
import type { Artifact } from '@/lib/types'
import UpstreamRequirementContext, {
  UPSTREAM_REQUIREMENT_ARTIFACT,
  bodyTemplateShowsRequirement,
} from './UpstreamRequirementContext.vue'

const apiMocks = vi.hoisted(() => ({
  artifactContent: vi.fn(),
}))

vi.mock('@/lib/api', async () => {
  const actual = await vi.importActual<typeof import('@/lib/api')>('@/lib/api')
  return {
    ...actual,
    api: {
      ...actual.api,
      artifactContent: apiMocks.artifactContent,
    },
  }
})

const StructuredStub = defineComponent({
  name: 'StructuredArtifactView',
  props: { name: String, doc: Object },
  template: '<div data-testid="structured-view" :data-name="name" />',
})

const AppModalStub = defineComponent({
  name: 'AppModal',
  props: {
    open: Boolean,
    title: String,
    width: Number,
    closeOnBackdrop: { type: Boolean, default: true },
  },
  emits: ['close'],
  template: `
    <div v-if="open" data-testid="upstream-enlarge-modal">
      <div
        data-testid="modal-backdrop"
        @click="closeOnBackdrop && $emit('close')"
      />
      <button data-testid="modal-close" type="button" @click="$emit('close')">close</button>
      <slot />
      <slot name="footer" />
    </div>
  `,
})

function artifact(overrides: Partial<Artifact> = {}): Artifact {
  return {
    id: 'a-req',
    name: UPSTREAM_REQUIREMENT_ARTIFACT,
    kind: 'json',
    nodeId: 'react',
    runId: 'run-1',
    workflowName: 'wf',
    sizeBytes: 20,
    createdAt: '2026-07-18T00:00:00Z',
    ...overrides,
  }
}

function mountCtx(opts: {
  artifacts?: Artifact[]
  productName?: string | null
  bodyTemplate?: string | null
} = {}) {
  const i18n = createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...common, ...pages } },
  })
  return mount(UpstreamRequirementContext, {
    props: {
      artifacts: opts.artifacts ?? [artifact()],
      runId: 'run-1',
      productName: opts.productName ?? 'page.html',
      bodyTemplate: opts.bodyTemplate,
    },
    global: {
      plugins: [i18n],
      stubs: {
        Icon: true,
        StructuredArtifactView: StructuredStub,
        AppModal: AppModalStub,
      },
    },
    attachTo: document.body,
  })
}

describe('UpstreamRequirementContext', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    apiMocks.artifactContent.mockResolvedValue({
      content: JSON.stringify({
        title: '需求',
        summary: '摘要',
        functional_requirements: [{ title: 'f1' }],
      }),
    })
  })

  it('does not render when clarified_requirement.json is absent', () => {
    const wrapper = mountCtx({ artifacts: [] })
    expect(wrapper.find('[data-testid="upstream-context"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="upstream-enlarge"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('skips when main product is already clarified_requirement.json', () => {
    const wrapper = mountCtx({ productName: UPSTREAM_REQUIREMENT_ARTIFACT })
    expect(wrapper.find('[data-testid="upstream-context"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="upstream-enlarge"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('skips when body_template uses artifact("clarified_requirement.json")', () => {
    expect(
      bodyTemplateShowsRequirement('{{artifact("clarified_requirement.json")}}'),
    ).toBe(true)
    const wrapper = mountCtx({
      productName: null,
      bodyTemplate: '{{artifact("clarified_requirement.json")}}\n\n附: MR',
    })
    expect(wrapper.find('[data-testid="upstream-context"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('skips when body_template references nodes.*.outputs.clarified_requirement', () => {
    const wrapper = mountCtx({
      productName: null,
      bodyTemplate: '{{nodes.react.outputs.clarified_requirement}}',
    })
    expect(wrapper.find('[data-testid="upstream-context"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('shows enlarge entry while collapsed and lazy-loads on expand', async () => {
    const wrapper = mountCtx()
    const root = wrapper.find('[data-testid="upstream-context"]')
    expect(root.exists()).toBe(true)
    expect(wrapper.find('[data-testid="upstream-enlarge"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="upstream-enlarge"]').attributes('title')).toBe(
      '放大上游上下文',
    )
    expect(wrapper.find('[data-testid="upstream-enlarge"]').text()).toContain(
      '放大上游上下文',
    )
    expect(wrapper.find('[data-testid="upstream-context-body"]').exists()).toBe(false)
    expect(apiMocks.artifactContent).not.toHaveBeenCalled()

    await wrapper.find('[data-testid="upstream-context-toggle"]').trigger('click')
    await flushPromises()

    expect(wrapper.find('[data-testid="upstream-context-body"]').exists()).toBe(true)
    expect(apiMocks.artifactContent).toHaveBeenCalledWith('a-req')
    expect(wrapper.find('[data-testid="structured-view"]').attributes('data-name')).toBe(
      UPSTREAM_REQUIREMENT_ARTIFACT,
    )
    wrapper.unmount()
  })

  it('opens modal without expanding and triggers lazy load', async () => {
    const wrapper = mountCtx()
    expect(wrapper.find('[data-testid="upstream-context-body"]').exists()).toBe(false)

    await wrapper.find('[data-testid="upstream-enlarge"]').trigger('click')
    await flushPromises()

    expect(wrapper.find('[data-testid="upstream-enlarge-modal"]').exists()).toBe(true)
    // Modal title stays 「上游上下文」; only the trigger button uses the new enlarge label.
    expect(wrapper.find('[data-testid="upstream-enlarge-modal"]').text()).toContain('上游上下文')
    expect(wrapper.find('[data-testid="upstream-context-body"]').exists()).toBe(false)
    expect(apiMocks.artifactContent).toHaveBeenCalledWith('a-req')
    expect(wrapper.find('[data-testid="upstream-modal-callout"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="upstream-modal-callout"]').text()).toContain(
      '放大上游上下文',
    )
    expect(wrapper.find('[data-testid="upstream-modal-readonly-footer"]').text()).toContain(
      '只读对照',
    )
    wrapper.unmount()
  })

  it('keeps inline and modal view modes independent', async () => {
    const wrapper = mountCtx()
    await wrapper.find('[data-testid="upstream-context-toggle"]').trigger('click')
    await flushPromises()

    await wrapper.find('[data-testid="upstream-mode-raw"]').trigger('click')
    expect(wrapper.find('[data-testid="upstream-context-body"] .json-code-view').exists()).toBe(
      true,
    )

    await wrapper.find('[data-testid="upstream-enlarge"]').trigger('click')
    await nextTick()

    // Modal defaults to structured; inline stays on raw.
    expect(wrapper.find('[data-testid="structured-view"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="upstream-context-body"] .json-code-view').exists()).toBe(
      true,
    )

    await wrapper.find('[data-testid="upstream-modal-mode-raw"]').trigger('click')
    await nextTick()
    expect(wrapper.findAll('.json-code-view').length).toBeGreaterThanOrEqual(2)

    await wrapper.find('[data-testid="modal-close"]').trigger('click')
    await nextTick()
    expect(wrapper.find('[data-testid="upstream-enlarge-modal"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="upstream-context-body"] .json-code-view').exists()).toBe(
      true,
    )
    wrapper.unmount()
  })

  it('does not close modal on backdrop click when closeOnBackdrop is false', async () => {
    const wrapper = mountCtx()
    await wrapper.find('[data-testid="upstream-enlarge"]').trigger('click')
    await flushPromises()

    const modal = wrapper.findComponent(AppModalStub)
    expect(modal.props('closeOnBackdrop')).toBe(false)

    await wrapper.find('[data-testid="modal-backdrop"]').trigger('click')
    await nextTick()
    expect(wrapper.find('[data-testid="upstream-enlarge-modal"]').exists()).toBe(true)

    await wrapper.find('[data-testid="modal-close"]').trigger('click')
    await nextTick()
    expect(wrapper.find('[data-testid="upstream-enlarge-modal"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('does not close modal on Escape', async () => {
    const wrapper = mountCtx()
    await wrapper.find('[data-testid="upstream-enlarge"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="upstream-enlarge-modal"]').exists()).toBe(true)

    await wrapper.trigger('keydown', { key: 'Escape' })
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    await nextTick()

    expect(wrapper.find('[data-testid="upstream-enlarge-modal"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('retries failed load from modal without blocking re-open', async () => {
    apiMocks.artifactContent
      .mockRejectedValueOnce(new Error('network down'))
      .mockResolvedValueOnce({
        content: JSON.stringify({ title: '需求', summary: '摘要' }),
      })

    const wrapper = mountCtx()
    await wrapper.find('[data-testid="upstream-enlarge"]').trigger('click')
    await flushPromises()

    expect(wrapper.find('[data-testid="upstream-modal-retry"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('上游上下文读取失败')

    await wrapper.find('[data-testid="upstream-modal-retry"]').trigger('click')
    await flushPromises()

    expect(apiMocks.artifactContent).toHaveBeenCalledTimes(2)
    expect(wrapper.find('[data-testid="structured-view"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('switches between structured and raw JSON views inline', async () => {
    const wrapper = mountCtx()
    await wrapper.find('[data-testid="upstream-context-toggle"]').trigger('click')
    await flushPromises()

    expect(wrapper.find('[data-testid="structured-view"]').exists()).toBe(true)
    await wrapper.find('[data-testid="upstream-mode-raw"]').trigger('click')
    expect(wrapper.find('[data-testid="structured-view"]').exists()).toBe(false)
    expect(wrapper.find('.json-code-view').exists()).toBe(true)

    await wrapper.find('[data-testid="upstream-mode-structured"]').trigger('click')
    expect(wrapper.find('[data-testid="structured-view"]').exists()).toBe(true)
    wrapper.unmount()
  })
})
