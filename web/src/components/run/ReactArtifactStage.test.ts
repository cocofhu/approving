// @vitest-environment happy-dom
import { defineComponent } from 'vue'
import { createI18n } from 'vue-i18n'
import { flushPromises, mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'
import type { Artifact } from '@/lib/shared/types'
import ReactArtifactStage from './ReactArtifactStage.vue'

const { mockAddClarifyAnnotation } = vi.hoisted(() => ({
  mockAddClarifyAnnotation: vi.fn(() => 'added'),
}))

vi.mock('@/lib/api/api', () => ({
  api: {
    artifactContent: vi.fn(async () => ({ content: '<html>thumb</html>' })),
    getRunNodeSandbox: vi.fn(async () => ({ id: 42 })),
  },
}))

vi.mock('@/lib/inbox/useClarifyDraft', () => ({
  addClarifyAnnotation: mockAddClarifyAnnotation,
}))

function i18n() {
  return createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...common, ...pages } },
  })
}

function art(partial: Partial<Artifact> & Pick<Artifact, 'id' | 'name'>): Artifact {
  return {
    kind: 'json',
    nodeId: 'react',
    runId: 'run-1',
    workflowName: 'wf',
    sizeBytes: 12,
    createdAt: '2026-08-01T00:00:00Z',
    revision: 1,
    ...partial,
  }
}

const stubs = {
  Icon: true,
  HtmlPreview: true,
  NovncPreviewPanel: defineComponent({
    props: { sandboxId: Number, inspectable: Boolean },
    template: '<div data-testid="novnc-stub" :data-inspectable="inspectable ? \'1\' : \'0\'" />',
  }),
  ArtifactPreview: defineComponent({
    props: { artifact: Object, annotatable: Boolean },
    template: '<div data-testid="artifact-preview">{{ artifact?.name }}|{{ annotatable ? \'on\' : \'off\' }}{{ artifact?.content ? \'|\' + artifact.content : \'\' }}</div>',
  }),
  AppPreviewPanel: defineComponent({
    props: { runId: String, nodeId: String, shareEnabled: Boolean },
    emits: ['pick', 'stagedPick', 'openShare'],
    template:
      '<div data-testid="app-preview-stub" :data-share="shareEnabled ? \'1\' : \'0\'">' +
      '<button type="button" data-testid="app-preview-pick" @click="$emit(\'pick\', { selector: \'#hero\', tagName: \'DIV\', outerHTML: \'<div id=hero></div>\', url: \'http://app/\' })">pick</button>' +
      '</div>',
  }),
  PublicAppPreviewPanel: defineComponent({
    template: '<div data-testid="public-app-preview-stub" />',
  }),
}

describe('ReactArtifactStage', () => {
  it('defaults to the pipeline grid tab and opens a new preview tab on card click', async () => {
    const wrapper = mount(ReactArtifactStage, {
      props: {
        artifacts: [art({ id: 'a1', name: 'note.md', kind: 'markdown' })],
        runId: 'run-1',
      },
      global: { plugins: [i18n()], stubs },
    })
    expect(wrapper.get('[data-testid="react-artifact-tab-grid"]').attributes('aria-selected')).toBe('true')
    expect(wrapper.get('[data-testid="react-artifact-grid"]').text()).toContain('note.md')
    expect(wrapper.find('[data-testid="react-artifact-tab-note.md"]').exists()).toBe(false)
    await wrapper.get('[data-testid="react-artifact-card-note.md"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="react-artifact-tab-grid"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="react-artifact-tab-note.md"]').attributes('aria-selected')).toBe('true')
    expect(wrapper.get('[data-testid="artifact-preview"]').text()).toBe('note.md|off')
    wrapper.unmount()
  })

  it('adds another preview tab instead of replacing the open one', async () => {
    const wrapper = mount(ReactArtifactStage, {
      props: {
        artifacts: [
          art({ id: 'a1', name: 'homepage-preview.html', kind: 'html' }),
          art({ id: 'a2', name: 'copy-variants.md', kind: 'markdown' }),
        ],
        runId: 'run-1',
      },
      global: { plugins: [i18n()], stubs },
    })
    await wrapper.get('[data-testid="react-artifact-card-homepage-preview.html"]').trigger('click')
    await wrapper.get('[data-testid="react-artifact-tab-grid"]').trigger('click')
    await wrapper.get('[data-testid="react-artifact-card-copy-variants.md"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="react-artifact-tab-homepage-preview.html"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="react-artifact-tab-copy-variants.md"]').attributes('aria-selected')).toBe('true')
    expect(wrapper.get('[data-testid="react-artifact-tab-grid"]').attributes('aria-selected')).toBe('false')
    const previews = wrapper.findAll('[data-testid="artifact-preview"]')
    expect(previews.map((n) => n.text())).toEqual(['homepage-preview.html|off', 'copy-variants.md|off'])
    wrapper.unmount()
  })

  it('enables annotate on every open preview tab', async () => {
    const wrapper = mount(ReactArtifactStage, {
      props: {
        artifacts: [
          art({ id: 'a1', name: 'homepage-preview.html', kind: 'html', nodeId: 'clarify' }),
          art({ id: 'a2', name: 'copy-variants.md', kind: 'markdown', nodeId: 'clarify' }),
        ],
        runId: 'run-1',
        nodeId: 'clarify',
        annotatable: true,
      },
      global: { plugins: [i18n()], stubs },
    })
    await wrapper.get('[data-testid="react-artifact-card-homepage-preview.html"]').trigger('click')
    await wrapper.get('[data-testid="react-artifact-tab-grid"]').trigger('click')
    await wrapper.get('[data-testid="react-artifact-card-copy-variants.md"]').trigger('click')
    await flushPromises()
    const previews = wrapper.findAll('[data-testid="artifact-preview"]')
    expect(previews.map((n) => n.text())).toEqual(['homepage-preview.html|on', 'copy-variants.md|on'])
    wrapper.unmount()
  })

  it('pins a preview tab when previewArtifact is set without dropping other open tabs', async () => {
    const first = art({ id: 'a1', name: 'page.html', kind: 'html', revision: 1, updatedAt: 't1' })
    const note = art({ id: 'a2', name: 'note.md', kind: 'markdown' })
    const wrapper = mount(ReactArtifactStage, {
      props: {
        artifacts: [first, note],
        runId: 'run-1',
      },
      global: { plugins: [i18n()], stubs },
    })
    await wrapper.get('[data-testid="react-artifact-card-note.md"]').trigger('click')
    await wrapper.setProps({
      artifacts: [first, note],
      previewArtifact: 'page.html',
    })
    await flushPromises()
    expect(wrapper.find('[data-testid="react-artifact-tab-note.md"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="react-artifact-tab-page.html"]').attributes('aria-selected')).toBe('true')
    await wrapper.setProps({
      artifacts: [{ ...first, revision: 2, updatedAt: 't2', sizeBytes: 99 }, note],
      previewArtifact: 'page.html',
    })
    await flushPromises()
    expect(wrapper.find('[data-testid="react-artifact-tab-note.md"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="react-artifact-preview-page.html"]').text()).toContain('page.html')
    wrapper.unmount()
  })

  it('keeps the tab bar when a single visual page is pinned', async () => {
    const wrapper = mount(ReactArtifactStage, {
      props: {
        artifacts: [art({ id: 'a1', name: 'page.html', kind: 'html', nodeId: 'visual_bqc5' })],
        previewArtifact: 'page.html',
        runId: 'run-1',
        nodeId: 'visual_bqc5',
        annotatable: true,
      },
      global: { plugins: [i18n()], stubs },
    })
    await flushPromises()
    expect(wrapper.get('[data-testid="react-artifact-tabs"]').isVisible()).toBe(true)
    expect(wrapper.get('[data-testid="react-artifact-tab-grid"]').text()).toContain('流水线产物')
    expect(wrapper.get('[data-testid="react-artifact-tab-page.html"]').attributes('aria-selected')).toBe('true')
    wrapper.unmount()
  })

  it('opens a pinned tab when the artifact arrives after previewArtifact', async () => {
    const note = art({ id: 'a2', name: 'note.md', kind: 'markdown' })
    const page = art({ id: 'a1', name: 'page.html', kind: 'html', revision: 1 })
    const wrapper = mount(ReactArtifactStage, {
      props: {
        artifacts: [note],
        previewArtifact: 'page.html',
        runId: 'run-1',
      },
      global: { plugins: [i18n()], stubs },
    })
    await wrapper.get('[data-testid="react-artifact-card-note.md"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="react-artifact-tab-page.html"]').exists()).toBe(false)
    await wrapper.setProps({ artifacts: [note, page], previewArtifact: 'page.html' })
    await flushPromises()
    expect(wrapper.find('[data-testid="react-artifact-tab-note.md"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="react-artifact-tab-page.html"]').attributes('aria-selected')).toBe('true')
    wrapper.unmount()
  })

  it('does not steal focus when an unrelated artifact is added under the same pin', async () => {
    const page = art({ id: 'a1', name: 'page.html', kind: 'html' })
    const note = art({ id: 'a2', name: 'note.md', kind: 'markdown' })
    const extra = art({ id: 'a3', name: 'extra.md', kind: 'markdown' })
    const wrapper = mount(ReactArtifactStage, {
      props: {
        artifacts: [page, note],
        previewArtifact: 'page.html',
        runId: 'run-1',
      },
      global: { plugins: [i18n()], stubs },
    })
    await flushPromises()
    expect(wrapper.get('[data-testid="react-artifact-tab-page.html"]').attributes('aria-selected')).toBe('true')
    await wrapper.get('[data-testid="react-artifact-tab-grid"]').trigger('click')
    await wrapper.get('[data-testid="react-artifact-card-note.md"]').trigger('click')
    await wrapper.setProps({ artifacts: [page, note, extra], previewArtifact: 'page.html' })
    await flushPromises()
    expect(wrapper.get('[data-testid="react-artifact-tab-note.md"]').attributes('aria-selected')).toBe('true')
    wrapper.unmount()
  })

  it('closes a preview tab and keeps the remaining one', async () => {
    const wrapper = mount(ReactArtifactStage, {
      props: {
        artifacts: [
          art({ id: 'a1', name: 'a.html', kind: 'html' }),
          art({ id: 'a2', name: 'b.md', kind: 'markdown' }),
        ],
        runId: 'run-1',
      },
      global: { plugins: [i18n()], stubs },
    })
    await wrapper.get('[data-testid="react-artifact-card-a.html"]').trigger('click')
    await wrapper.get('[data-testid="react-artifact-tab-grid"]').trigger('click')
    await wrapper.get('[data-testid="react-artifact-card-b.md"]').trigger('click')
    await wrapper.get('[data-testid="react-artifact-tab-close-b.md"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="react-artifact-tab-b.md"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="react-artifact-tab-a.html"]').attributes('aria-selected')).toBe('true')
    wrapper.unmount()
  })

  it('opens a noVNC tab from the pipeline card without replacing artifact tabs', async () => {
    const wrapper = mount(ReactArtifactStage, {
      props: {
        artifacts: [art({ id: 'a1', name: 'note.md', kind: 'markdown' })],
        runId: 'run-1',
        nodeId: 'clarify',
        annotatable: true,
      },
      global: { plugins: [i18n()], stubs },
    })
    await flushPromises()
    expect(wrapper.find('[data-testid="react-artifact-card-novnc"]').exists()).toBe(true)
    await wrapper.get('[data-testid="react-artifact-card-note.md"]').trigger('click')
    await wrapper.get('[data-testid="react-artifact-tab-grid"]').trigger('click')
    await wrapper.get('[data-testid="react-artifact-card-novnc"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="react-artifact-tab-note.md"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="react-artifact-tab-novnc"]').attributes('aria-selected')).toBe('true')
    expect(wrapper.get('[data-testid="novnc-stub"]').attributes('data-inspectable')).toBe('1')
    await wrapper.get('[data-testid="react-artifact-tab-close-novnc"]').trigger('click')
    expect(wrapper.find('[data-testid="react-artifact-tab-novnc"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="react-artifact-tab-note.md"]').attributes('aria-selected')).toBe('true')
    wrapper.unmount()
  })

  it('keeps foreign-node artifacts previewable but not annotatable', async () => {
    const wrapper = mount(ReactArtifactStage, {
      props: {
        artifacts: [
          art({ id: 'own', name: 'research.json', kind: 'json', nodeId: 'research' }),
          art({ id: 'other', name: 'plan.json', kind: 'json', nodeId: 'plan' }),
        ],
        runId: 'run-1',
        nodeId: 'research',
        annotatable: true,
      },
      global: { plugins: [i18n()], stubs },
    })
    expect(wrapper.findAll('[data-testid="react-artifact-card-readonly"]').length).toBe(1)
    await wrapper.get('[data-testid="react-artifact-card-research.json"]').trigger('click')
    await wrapper.get('[data-testid="react-artifact-tab-grid"]').trigger('click')
    await wrapper.get('[data-testid="react-artifact-card-plan.json"]').trigger('click')
    await flushPromises()
    const previews = wrapper.findAll('[data-testid="artifact-preview"]')
    expect(previews.map((n) => n.text())).toEqual(['research.json|on', 'plan.json|off'])
    wrapper.unmount()
  })

  it('opens app preview inside the remote tab without sandbox noVNC', async () => {
    const wrapper = mount(ReactArtifactStage, {
      props: {
        artifacts: [art({ id: 'a1', name: 'note.md', kind: 'markdown' })],
        runId: 'run-1',
        nodeId: 'preview',
        annotatable: true,
        remoteKind: 'app',
        shareEnabled: true,
      },
      global: { plugins: [i18n()], stubs },
    })
    await flushPromises()
    expect(wrapper.find('[data-testid="novnc-stub"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="app-preview-stub"]').attributes('data-share')).toBe('1')
    expect(wrapper.get('[data-testid="react-artifact-tab-novnc"]').attributes('aria-selected')).toBe('true')
    expect(wrapper.get('[data-testid="react-artifact-tab-novnc"]').text()).toContain('应用预览')
    wrapper.unmount()
  })

  it('emits remote pick once without staging a local annotation', async () => {
    mockAddClarifyAnnotation.mockClear()
    const wrapper = mount(ReactArtifactStage, {
      props: {
        artifacts: [],
        runId: 'run-1',
        nodeId: 'preview',
        annotatable: true,
        remoteKind: 'app',
      },
      global: { plugins: [i18n()], stubs },
    })
    await flushPromises()
    await wrapper.get('[data-testid="app-preview-pick"]').trigger('click')
    expect(wrapper.emitted('pick')).toEqual([
      [{ selector: '#hero', tagName: 'DIV', outerHTML: '<div id=hero></div>', url: 'http://app/' }],
    ])
    expect(mockAddClarifyAnnotation).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('merges visual page snapshots onto one page.html card with a footer version chip', async () => {
    const live = art({ id: 'live', name: 'page.html', kind: 'html', nodeId: 'visual_1', content: '<p>new</p>' })
    const alias = art({
      id: 'alias',
      name: 'visual_1.page.html',
      kind: 'html',
      nodeId: 'visual_1',
      content: '<p>new</p>',
    })
    const json = art({ id: 'json', name: 'node_complete.json', kind: 'json', nodeId: 'visual_1' })
    const other = art({
      id: 'other',
      name: 'visual_other.page.html',
      kind: 'html',
      nodeId: 'visual_other',
      content: '<p>other</p>',
    })
    const wrapper = mount(ReactArtifactStage, {
      props: {
        artifacts: [live, alias, json, other],
        runId: 'run-1',
        run: {
          id: 'run-1',
          nodes: [
            { id: 'visual_1', type: 'visual', label: '视觉', position: { x: 0, y: 0 }, config: {} },
            { id: 'visual_other', type: 'visual', label: '另一页', position: { x: 0, y: 0 }, config: {} },
          ],
          nodeExecutions: {
            visual_1: [
              { nodeId: 'visual_1', iteration: 1, status: 'completed', outputs: { page: '<p>old</p>' } },
              { nodeId: 'visual_1', iteration: 2, status: 'waiting_human', outputs: { page: '<p>new</p>' } },
            ],
          },
        } as any,
        nodeId: 'visual_1',
        annotatable: true,
        remoteKind: 'off',
      },
      global: { plugins: [i18n()], stubs },
    })
    await flushPromises()
    expect(wrapper.find('[data-testid="react-artifact-card-page.html"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="react-artifact-card-visual_1.page.html"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="react-artifact-card-page.html#iter-1"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="react-artifact-card-node_complete.json"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="react-artifact-card-visual_other.page.html"]').exists()).toBe(true)
    expect(wrapper.findAll('[data-testid="react-artifact-card-page.html"]').length).toBe(1)
    expect(wrapper.get('[data-testid="react-artifact-version-chip-btn-page.html"]').text()).toContain('v2 · 最新')
    expect(wrapper.find('[data-testid="react-artifact-card-iteration"]').exists()).toBe(false)
    await wrapper.get('[data-testid="react-artifact-version-chip-btn-page.html"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-testid="react-artifact-version-option-v1"]').text()).toBe('v1')
    expect(wrapper.get('[data-testid="react-artifact-version-option-v2"]').text()).toContain('v2 · 最新')
    await wrapper.get('[data-testid="react-artifact-version-option-v1"]').trigger('click')
    await flushPromises()
    expect(wrapper.findAll('[data-testid="react-artifact-card-page.html"]').length).toBe(1)
    expect(wrapper.get('[data-testid="artifact-preview"]').text()).toBe('page.html|off|<p>old</p>')
    await wrapper.get('[data-testid="react-artifact-tab-grid"]').trigger('click')
    await wrapper.get('[data-testid="react-artifact-version-chip-btn-page.html"]').trigger('click')
    await wrapper.get('[data-testid="react-artifact-version-option-v2"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-testid="artifact-preview"]').text()).toBe('page.html|on|<p>new</p>')
    wrapper.unmount()
  })

  it('hides the version chip for a single snapshot and still shows json cards', async () => {
    const live = art({ id: 'live', name: 'page.html', kind: 'html', nodeId: 'visual_1', content: '<p>only</p>' })
    const json = art({ id: 'json', name: 'node_complete.json', kind: 'json', nodeId: 'visual_1' })
    const wrapper = mount(ReactArtifactStage, {
      props: {
        artifacts: [live, json],
        runId: 'run-1',
        run: {
          id: 'run-1',
          nodes: [{ id: 'visual_1', type: 'visual', label: '视觉', position: { x: 0, y: 0 }, config: {} }],
          nodeExecutions: {
            visual_1: [{ nodeId: 'visual_1', iteration: 1, status: 'completed', outputs: { page: '<p>only</p>' } }],
          },
        } as any,
        nodeId: 'visual_1',
        remoteKind: 'off',
      },
      global: { plugins: [i18n()], stubs },
    })
    await flushPromises()
    expect(wrapper.find('[data-testid="react-artifact-version-chip"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="react-artifact-card-node_complete.json"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('disables a missing snapshot menu item instead of previewing latest html', async () => {
    const live = art({ id: 'live', name: 'page.html', kind: 'html', nodeId: 'visual_1', content: '<p>new</p>' })
    const wrapper = mount(ReactArtifactStage, {
      props: {
        artifacts: [live],
        runId: 'run-1',
        run: {
          id: 'run-1',
          nodes: [{ id: 'visual_1', type: 'visual', label: '视觉', position: { x: 0, y: 0 }, config: {} }],
          nodeExecutions: {
            visual_1: [
              { nodeId: 'visual_1', iteration: 1, status: 'completed', outputs: {} },
              { nodeId: 'visual_1', iteration: 2, status: 'waiting_human', outputs: { page: '<p>new</p>' } },
            ],
          },
        } as any,
        nodeId: 'visual_1',
        annotatable: true,
        remoteKind: 'off',
      },
      global: { plugins: [i18n()], stubs },
    })
    await flushPromises()
    await wrapper.get('[data-testid="react-artifact-version-chip-btn-page.html"]').trigger('click')
    const missing = wrapper.get('[data-testid="react-artifact-version-option-v1"]')
    expect(missing.attributes('disabled')).toBeDefined()
    await missing.trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="artifact-preview"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('uses a footer chip on a standalone visual_*.page.html card when page.html is absent', async () => {
    const alias = art({
      id: 'alias',
      name: 'visual_1.page.html',
      kind: 'html',
      nodeId: 'visual_1',
      content: '<p>new</p>',
    })
    const wrapper = mount(ReactArtifactStage, {
      props: {
        artifacts: [alias],
        runId: 'run-1',
        run: {
          id: 'run-1',
          nodes: [{ id: 'visual_1', type: 'visual', label: '视觉', position: { x: 0, y: 0 }, config: {} }],
          nodeExecutions: {
            visual_1: [
              { nodeId: 'visual_1', iteration: 1, status: 'completed', outputs: { page: '<p>old</p>' } },
              { nodeId: 'visual_1', iteration: 2, status: 'waiting_human', outputs: { page: '<p>new</p>' } },
            ],
          },
        } as any,
        nodeId: 'visual_1',
        remoteKind: 'off',
      },
      global: { plugins: [i18n()], stubs },
    })
    await flushPromises()
    expect(wrapper.find('[data-testid="react-artifact-card-page.html"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="react-artifact-card-visual_1.page.html"]').exists()).toBe(true)
    await wrapper.get('[data-testid="react-artifact-version-chip-btn-visual_1.page.html"]').trigger('click')
    await wrapper.get('[data-testid="react-artifact-version-option-v1"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-testid="artifact-preview"]').text()).toBe('visual_1.page.html|off|<p>old</p>')
    wrapper.unmount()
  })
})
