// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'
import type { Artifact } from '@/lib/shared/types'

const apiMocks = vi.hoisted(() => ({
  artifactContent: vi.fn(),
  artifactDownloadUrl: vi.fn((id: string) => `http://test/api/artifacts/${id}/download`),
  deleteArtifact: vi.fn(),
}))

const toastMocks = vi.hoisted(() => ({
  success: vi.fn(),
  error: vi.fn(),
  warn: vi.fn(),
}))

const copyMocks = vi.hoisted(() => ({
  copyToClipboard: vi.fn(),
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
  useToast: () => toastMocks,
}))

vi.mock('@/lib/shared/copyToClipboard', () => ({
  copyToClipboard: (...args: unknown[]) => copyMocks.copyToClipboard(...args),
}))

import ArtifactPreview from './ArtifactPreview.vue'

const textArtifact: Artifact = {
  id: 'art-md-1',
  name: 'notes.md',
  kind: 'markdown',
  nodeId: 'n1',
  runId: 'run-1',
  workflowName: 'wf',
  sizeBytes: 20,
  createdAt: '2026-07-18T00:00:00Z',
}

function mountPreview(artifact: Artifact | null = textArtifact) {
  const i18n = createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...common, ...pages } },
  })
  return mount(ArtifactPreview, {
    props: { artifact },
    global: {
      plugins: [i18n],
      stubs: { Icon: true, AppModal: true, AppButton: { template: '<button><slot /></button>' } },
    },
  })
}

describe('ArtifactPreview loading/pending (专项三 g6)', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    apiMocks.artifactContent.mockResolvedValue({ content: '# hello artifact' })
    copyMocks.copyToClipboard.mockResolvedValue(true)
  })

  it('keeps loaded text visible while refreshing and does not wipe on loading branch', async () => {
    const wrapper = mountPreview()
    await flushPromises()
    expect(wrapper.text()).toContain('hello artifact')
    expect(wrapper.html()).not.toMatch(/v-else-if artifact && loading/)
    wrapper.unmount()
  })

  it('discards stale loadContent when artifact switches', async () => {
    let resolveFirst!: (v: { content: string }) => void
    apiMocks.artifactContent
      .mockImplementationOnce(
        () =>
          new Promise((resolve) => {
            resolveFirst = resolve
          }),
      )
      .mockResolvedValueOnce({ content: '# second' })
    const wrapper = mountPreview({ ...textArtifact, id: 'art-a', name: 'a.md' })
    await wrapper.setProps({ artifact: { ...textArtifact, id: 'art-b', name: 'b.md' } })
    await flushPromises()
    resolveFirst({ content: '# STALE-FIRST' })
    await flushPromises()
    expect(wrapper.text()).not.toContain('STALE-FIRST')
    expect(wrapper.text()).toContain('second')
    wrapper.unmount()
  })

  it('sandboxes loadErr without concatenating e.message', async () => {
    apiMocks.artifactContent.mockRejectedValue(new Error('internal-stack-trace'))
    const wrapper = mountPreview()
    await flushPromises()
    expect(wrapper.find('[data-testid="artifact-preview-load-error"]').exists()).toBe(true)
    expect(wrapper.text()).not.toContain('internal-stack-trace')
    expect(wrapper.text()).toMatch(/产物加载失败/)
    wrapper.unmount()
  })

  it('copy button pending + success toast; double click only copies once', async () => {
    let resolveCopy!: (ok: boolean) => void
    copyMocks.copyToClipboard.mockImplementation(
      () =>
        new Promise((resolve) => {
          resolveCopy = resolve
        }),
    )
    const wrapper = mountPreview()
    await flushPromises()
    const btn = wrapper.get('[data-testid="artifact-preview-copy"]')
    await btn.trigger('click')
    await btn.trigger('click')
    expect(copyMocks.copyToClipboard).toHaveBeenCalledTimes(1)
    expect(btn.attributes('disabled')).toBeDefined()
    expect(btn.attributes('aria-busy')).toBe('true')
    resolveCopy(true)
    await flushPromises()
    expect(toastMocks.success).toHaveBeenCalled()
    wrapper.unmount()
  })
})
