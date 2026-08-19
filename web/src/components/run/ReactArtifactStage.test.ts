// @vitest-environment happy-dom
import { defineComponent } from 'vue'
import { createI18n } from 'vue-i18n'
import { flushPromises, mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'
import type { Artifact } from '@/lib/shared/types'
import ReactArtifactStage from './ReactArtifactStage.vue'

vi.mock('@/lib/api/api', () => ({
  api: {
    artifactContent: vi.fn(async () => ({ content: '<html>thumb</html>' })),
    getRunNodeSandbox: vi.fn(async () => ({ id: 42 })),
  },
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
    template: '<div data-testid="artifact-preview">{{ artifact?.name }}|{{ annotatable ? \'on\' : \'off\' }}</div>',
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
          art({ id: 'a1', name: 'homepage-preview.html', kind: 'html' }),
          art({ id: 'a2', name: 'copy-variants.md', kind: 'markdown' }),
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
})
