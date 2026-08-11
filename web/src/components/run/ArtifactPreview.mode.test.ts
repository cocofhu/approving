// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'

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

import ArtifactPreview from './ArtifactPreview.vue'

const clarifiedDoc = {
  title: '需求标题',
  summary: '概述',
  goals: ['g'],
  in_scope: ['in'],
  out_of_scope: ['out'],
  functional_requirements: [{ title: 'f1', detail: 'd', acceptance_criteria: ['a'] }],
  assumptions: ['a'],
  dependencies: ['d'],
  constraints: ['c'],
}

const artA = {
  id: 'art-a',
  name: 'clarified_requirement.json',
  kind: 'json' as const,
  nodeId: 'react',
  runId: 'run-1',
  workflowName: 'wf',
  sizeBytes: 100,
  createdAt: '2026-08-10T12:00:00Z',
}

const artB = {
  ...artA,
  id: 'art-b',
  name: 'plan.json',
  nodeId: 'plan',
}

function mountPreview(
  artifact: typeof artA | null,
  extra: Record<string, unknown> = {},
) {
  const i18n = createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...common, ...pages } },
  })
  return mount(ArtifactPreview, {
    props: { artifact, ...extra },
    global: {
      plugins: [i18n],
      stubs: {
        Icon: true,
        HtmlPreview: true,
        StructuredArtifactView: {
          props: ['name', 'doc'],
          template: '<div data-testid="structured-stub">{{ doc?.title }}</div>',
        },
        AppModal: {
          props: ['open', 'title'],
          template: '<div v-if="open" data-testid="zoom-modal"><slot /><slot name="footer" /></div>',
        },
        AppButton: {
          emits: ['click'],
          template: '<button v-bind="$attrs" @click="$emit(\'click\')"><slot /></button>',
        },
      },
    },
  })
}

describe('ArtifactPreview structured mode + capability trim', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    apiMocks.artifactContent.mockImplementation(async (id: string) => ({
      ...(id === 'art-b' ? artB : artA),
      content: JSON.stringify(
        id === 'art-b' ? { title: '计划 B', goals: [] } : clarifiedDoc,
      ),
    }))
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('defaults to structured UI with mode toggle (g1.2)', async () => {
    const wrapper = mountPreview(artA)
    await flushPromises()
    expect(wrapper.find('[data-testid="structured-stub"]').text()).toContain('需求标题')
    expect(wrapper.find('[data-testid="artifact-preview-mode-toggle"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="artifact-preview-raw-json"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('switches to raw JSON and back without losing selection (g1.2)', async () => {
    const wrapper = mountPreview(artA)
    await flushPromises()
    await wrapper.find('[data-testid="artifact-preview-mode-raw"]').trigger('click')
    await nextTick()
    expect(wrapper.find('[data-testid="artifact-preview-raw-json"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="structured-stub"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="artifact-preview-raw-json"]').text()).toContain('需求标题')
    await wrapper.find('[data-testid="artifact-preview-mode-structured"]').trigger('click')
    await nextTick()
    expect(wrapper.find('[data-testid="structured-stub"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('resets to structured when artifact changes while in raw (g1.3)', async () => {
    const wrapper = mountPreview(artA)
    await flushPromises()
    await wrapper.find('[data-testid="artifact-preview-mode-raw"]').trigger('click')
    await nextTick()
    expect(wrapper.find('[data-testid="artifact-preview-raw-json"]').exists()).toBe(true)
    await wrapper.setProps({ artifact: artB })
    await flushPromises()
    await nextTick()
    expect(wrapper.find('[data-testid="structured-stub"]').text()).toContain('计划 B')
    expect(wrapper.find('[data-testid="artifact-preview-raw-json"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('hides delete/copy/zoom/export when trim props set (g1.1 / g1.4)', async () => {
    const wrapper = mountPreview(artA, {
      hideDelete: true,
      hideCopy: true,
      hideZoom: true,
      hideExport: true,
    })
    await flushPromises()
    expect(wrapper.find('[data-testid="artifact-preview-delete"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="artifact-preview-copy"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="artifact-preview-zoom"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="artifact-preview-download-png"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="artifact-preview-download-pdf"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="artifact-preview-download-raw"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="artifact-preview-mode-toggle"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('keeps delete/copy visible by default for Artifacts detail', async () => {
    const wrapper = mountPreview(artA)
    await flushPromises()
    expect(wrapper.find('[data-testid="artifact-preview-delete"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="artifact-preview-copy"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="artifact-preview-zoom"]').exists()).toBe(true)
    wrapper.unmount()
  })
})
