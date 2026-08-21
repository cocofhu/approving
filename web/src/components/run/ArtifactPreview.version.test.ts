// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'
import type { Artifact, Run } from '@/lib/shared/types'
import ArtifactPreview from './ArtifactPreview.vue'

const apiMocks = vi.hoisted(() => ({
  artifactContent: vi.fn(),
  artifactDownloadUrl: vi.fn((id: string) => `http://test/api/artifacts/${id}/download`),
  deleteArtifact: vi.fn(),
}))

vi.mock('@/lib/api/api', async () => {
  const actual = await vi.importActual<typeof import('@/lib/api/api')>('@/lib/api/api')
  return {
    ...actual,
    api: {
      ...actual.api,
      artifactContent: apiMocks.artifactContent,
      artifactDownloadUrl: apiMocks.artifactDownloadUrl,
      deleteArtifact: apiMocks.deleteArtifact,
    },
  }
})

vi.mock('@/lib/composables/useToast', () => ({
  useToast: () => ({ success: vi.fn(), error: vi.fn(), warn: vi.fn() }),
}))

function art(partial: Partial<Artifact> & Pick<Artifact, 'id' | 'name'>): Artifact {
  return {
    kind: 'html',
    nodeId: 'visual_1',
    runId: 'run-1',
    sizeBytes: 10,
    createdAt: '2026-08-10T12:00:00Z',
    ...partial,
  } as Artifact
}

function multiVersionRun(): Run {
  return {
    id: 'run-1',
    nodes: [{ id: 'visual_1', type: 'visual', label: '视觉', position: { x: 0, y: 0 }, config: {} }],
    nodeExecutions: {
      visual_1: [
        { nodeId: 'visual_1', iteration: 1, status: 'completed', outputs: { page: '<p>old</p>' } },
        { nodeId: 'visual_1', iteration: 2, status: 'waiting_human', outputs: { page: '<p>new</p>' } },
      ],
    },
  } as unknown as Run
}

function pageHistoryRun(): Run {
  return {
    id: 'run-1',
    nodes: [{ id: 'visual_1', type: 'visual', label: '视觉', position: { x: 0, y: 0 }, config: {} }],
    nodeExecutions: {
      visual_1: [
        {
          nodeId: 'visual_1',
          iteration: 1,
          status: 'waiting_human',
          outputs: { page: '<p>v2</p>', page_history: ['<p>v1</p>'] },
        },
      ],
    },
  } as unknown as Run
}

function mountPreview(artifact: Artifact | null, extra: Record<string, unknown> = {}) {
  const i18n = createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...common, ...pages } },
  })
  return mount(ArtifactPreview, {
    props: { artifact, scope: 'platform', ...extra },
    global: {
      plugins: [i18n],
      stubs: {
        Icon: true,
        HtmlPreview: {
          props: ['html', 'inspectable'],
          template:
            '<div data-testid="html-preview-stub" :data-inspectable="inspectable ? \'1\' : \'0\'">{{ html }}</div>',
        },
        StructuredArtifactView: true,
        AppModal: {
          props: ['open', 'title'],
          template: '<div v-if="open" data-testid="zoom-modal"><slot /><slot name="footer" /></div>',
        },
        AppButton: {
          emits: ['click'],
          template: '<button v-bind="$attrs" @click="$emit(\'click\')"><slot /></button>',
        },
        SelectionAddToChat: true,
        RefreshStrip: true,
        HardLoadLayer: true,
      },
    },
  })
}

/** g3.1: ArtifactPreview page.html version chip (plan leaf evidence). */
describe('ArtifactPreview page.html version switch (g2 / g3.1)', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    apiMocks.artifactContent.mockImplementation(async (id: string) => ({
      id,
      name: 'page.html',
      kind: 'html',
      content: '<p>new</p>',
    }))
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('shows version chip for ≥2 choices and switches historical HTML (g2.1)', async () => {
    const live = art({ id: 'live', name: 'page.html', content: '<p>new</p>' })
    const wrapper = mountPreview(live, { run: multiVersionRun() })
    await flushPromises()

    expect(wrapper.get('[data-testid="artifact-preview-version-chip-btn"]').text()).toContain('v2 · 最新')
    expect(wrapper.get('[data-testid="html-preview-stub"]').text()).toBe('<p>new</p>')

    await wrapper.get('[data-testid="artifact-preview-version-chip-btn"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-testid="artifact-preview-version-option-v1"]').text()).toBe('v1')
    expect(wrapper.get('[data-testid="artifact-preview-version-option-v2"]').text()).toContain('v2 · 最新')

    await wrapper.get('[data-testid="artifact-preview-version-option-v1"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-testid="html-preview-stub"]').text()).toBe('<p>old</p>')
    expect(wrapper.find('[data-testid="artifact-preview-historical-readonly"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="artifact-preview-delete"]').exists()).toBe(false)

    await wrapper.get('[data-testid="artifact-preview-version-chip-btn"]').trigger('click')
    await wrapper.get('[data-testid="artifact-preview-version-option-v2"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-testid="html-preview-stub"]').text()).toBe('<p>new</p>')
    expect(wrapper.find('[data-testid="artifact-preview-historical-readonly"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="artifact-preview-delete"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('hides chip for single version (g2.2)', async () => {
    const live = art({ id: 'live', name: 'page.html', content: '<p>only</p>' })
    const run = {
      id: 'run-1',
      nodes: [{ id: 'visual_1', type: 'visual', label: '视觉', position: { x: 0, y: 0 }, config: {} }],
      nodeExecutions: {
        visual_1: [{ nodeId: 'visual_1', iteration: 1, status: 'completed', outputs: { page: '<p>only</p>' } }],
      },
    } as unknown as Run
    const wrapper = mountPreview(live, { run })
    await flushPromises()
    expect(wrapper.find('[data-testid="artifact-preview-version-chip"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="html-preview-stub"]').text()).toBe('<p>only</p>')
    wrapper.unmount()
  })

  it('does not show chip for non-page.html artifacts (g3.2)', async () => {
    const json = art({
      id: 'j1',
      name: 'plan.json',
      kind: 'json',
      content: JSON.stringify({ title: '计划', goals: [] }),
    })
    const wrapper = mountPreview(json, { run: multiVersionRun() })
    await flushPromises()
    expect(wrapper.find('[data-testid="artifact-preview-version-chip"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('degrades without chip when run is null (g1.1 load failure)', async () => {
    const live = art({ id: 'live', name: 'page.html', content: '<p>fallback</p>' })
    const wrapper = mountPreview(live, { run: null })
    await flushPromises()
    expect(wrapper.find('[data-testid="artifact-preview-version-chip"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="html-preview-stub"]').text()).toBe('<p>fallback</p>')
    wrapper.unmount()
  })

  it('supports page_history versions and keeps historical inspectable=off (g2.2)', async () => {
    const live = art({ id: 'live', name: 'page.html', content: '<p>v2</p>' })
    const wrapper = mountPreview(live, { run: pageHistoryRun(), annotatable: true })
    await flushPromises()
    expect(wrapper.get('[data-testid="artifact-preview-version-chip-btn"]').text()).toContain('v2 · 最新')
    expect(wrapper.get('[data-testid="html-preview-stub"]').attributes('data-inspectable')).toBe('0')

    await wrapper.get('[data-testid="artifact-preview-version-chip-btn"]').trigger('click')
    await wrapper.get('[data-testid="artifact-preview-version-option-v1"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-testid="html-preview-stub"]').text()).toBe('<p>v1</p>')
    expect(wrapper.get('[data-testid="html-preview-stub"]').attributes('data-inspectable')).toBe('0')
    expect(wrapper.find('[data-testid="artifact-preview-delete"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('disables unavailable historical option (g2.2)', async () => {
    const live = art({ id: 'live', name: 'page.html', content: '<p>new</p>' })
    const run = {
      id: 'run-1',
      nodes: [{ id: 'visual_1', type: 'visual', label: '视觉', position: { x: 0, y: 0 }, config: {} }],
      nodeExecutions: {
        visual_1: [
          { nodeId: 'visual_1', iteration: 1, status: 'completed', outputs: {} },
          { nodeId: 'visual_1', iteration: 2, status: 'waiting_human', outputs: { page: '<p>new</p>' } },
        ],
      },
    } as unknown as Run
    const wrapper = mountPreview(live, { run })
    await flushPromises()
    await wrapper.get('[data-testid="artifact-preview-version-chip-btn"]').trigger('click')
    const missing = wrapper.get('[data-testid="artifact-preview-version-option-v1"]')
    expect(missing.attributes('disabled')).toBeDefined()
    await missing.trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-testid="html-preview-stub"]').text()).toBe('<p>new</p>')
    wrapper.unmount()
  })
})
