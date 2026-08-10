// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'
import shell from '@/locales/zh-CN/shell.json'
import type { Artifact } from '@/lib/shared/types'

const push = vi.fn()

vi.mock('vue-router', () => ({
  useRouter: () => ({ push }),
}))

const apiMocks = vi.hoisted(() => ({
  getRun: vi.fn(),
  artifactContent: vi.fn(),
  artifactDownloadUrl: vi.fn((id: string) => `http://test/api/artifacts/${id}/download`),
}))

vi.mock('@/lib/api/api', async () => {
  const actual = await vi.importActual<typeof import('@/lib/api/api')>('@/lib/api/api')
  return {
    ...actual,
    api: {
      ...actual.api,
      getRun: apiMocks.getRun,
      artifactContent: apiMocks.artifactContent,
      artifactDownloadUrl: apiMocks.artifactDownloadUrl,
    },
  }
})

vi.mock('@/lib/composables/useToast', () => ({
  useToast: () => ({ success: vi.fn(), error: vi.fn(), warn: vi.fn() }),
}))

import RunOutputPptModal from './RunOutputPptModal.vue'

const clarifiedDoc = {
  title: '运行产出弹窗',
  summary: '主从预览',
  goals: ['看清内容'],
  in_scope: ['列表'],
  out_of_scope: ['缩略图'],
  functional_requirements: [{ title: 'f1', detail: 'd', acceptance_criteria: ['a'] }],
  assumptions: ['无额外假设'],
  dependencies: ['产物 API'],
  constraints: ['禁止删除'],
}

function artifact(partial: Partial<Artifact> & Pick<Artifact, 'id' | 'name' | 'kind'>): Artifact {
  return {
    nodeId: partial.nodeId ?? 'node-1',
    runId: partial.runId ?? 'run-1',
    workflowName: partial.workflowName ?? 'wf',
    sizeBytes: partial.sizeBytes ?? 128,
    createdAt: partial.createdAt ?? '2026-08-10T12:00:00Z',
    ...partial,
  }
}

function mountModal(props: { open?: boolean; runId?: string | null } = {}) {
  const i18n = createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...common, ...pages, ...shell } },
  })
  return mount(RunOutputPptModal, {
    props: {
      open: props.open ?? true,
      runId: props.runId ?? 'run-1',
      contextLabel: '自我迭代',
    },
    global: {
      plugins: [i18n],
      stubs: {
        Icon: true,
        HtmlPreview: { template: '<div data-testid="html-stub" />' },
        StructuredArtifactView: {
          props: ['name', 'doc'],
          template:
            '<div data-testid="structured-stub">{{ name }} · {{ doc?.title || doc?.summary || "ui" }}</div>',
        },
        AppModal: {
          props: ['open', 'title', 'width', 'bodyOverflow', 'bodyMinHeight'],
          template:
            '<div v-if="open" data-testid="run-output-modal"><div data-testid="modal-title">{{ title }}</div><slot /><div data-testid="modal-footer"><slot name="footer" /></div></div>',
        },
        AppButton: {
          emits: ['click'],
          template: '<button v-bind="$attrs" @click="$emit(\'click\')"><slot /></button>',
        },
      },
    },
    attachTo: document.body,
  })
}

describe('RunOutputPptModal master-detail preview', () => {
  beforeEach(() => {
    push.mockReset()
    apiMocks.getRun.mockReset()
    apiMocks.artifactContent.mockReset()
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => ({
        ok: true,
        blob: async () => new Blob(['img'], { type: 'image/png' }),
      })),
    )
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('shows empty state when run has no artifacts (g3.3)', async () => {
    apiMocks.getRun.mockResolvedValue({
      id: 'run-1',
      title: 'empty',
      workflowName: 'wf',
      artifacts: [],
    })
    const wrapper = mountModal()
    await flushPromises()
    expect(wrapper.find('[data-testid="run-output-empty"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="run-output-master-detail"]').exists()).toBe(false)
    expect(wrapper.text()).toContain('本次运行暂无产出')
    expect(wrapper.text()).not.toMatch(/PPT|幻灯片|16:9/)
    wrapper.unmount()
  })

  it('shows load error without fake preview (g3.3)', async () => {
    apiMocks.getRun.mockRejectedValue(new Error('network down'))
    const wrapper = mountModal()
    await flushPromises()
    expect(wrapper.find('[data-testid="run-output-load-error"]').text()).toContain('network down')
    expect(wrapper.find('[data-testid="run-output-deck"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('defaults to first artifact structured UI and allows raw toggle + reset (g4.3)', async () => {
    const items = [
      artifact({
        id: 'a-req',
        name: 'clarified_requirement.json',
        kind: 'json',
        nodeId: 'react',
        sizeBytes: 2048,
      }),
      artifact({
        id: 'a-plan',
        name: 'plan.json',
        kind: 'json',
        nodeId: 'plan',
        sizeBytes: 1024,
      }),
    ]
    apiMocks.getRun.mockResolvedValue({
      id: 'run-1',
      title: 'done',
      workflowName: 'wf',
      artifacts: items,
    })
    apiMocks.artifactContent.mockImplementation(async (id: string) => {
      const a = items.find((x) => x.id === id)!
      return {
        ...a,
        content: JSON.stringify(
          id === 'a-req'
            ? clarifiedDoc
            : { title: '计划标题', goals: [{ title: 'g1', subgoals: [] }] },
        ),
      }
    })

    const wrapper = mountModal()
    await flushPromises()
    await nextTick()

    expect(wrapper.find('[data-testid="run-output-master-detail"]').exists()).toBe(true)
    const rows = wrapper.findAll('[data-testid="run-output-row"]')
    expect(rows).toHaveLength(2)
    expect(rows[0].attributes('aria-selected')).toBe('true')
    expect(rows[0].text()).toContain('json')
    expect(rows[0].text()).toContain('clarified_requirement.json')
    expect(rows[0].text()).toContain('react')
    expect(rows[0].text()).toContain('2.0 KB')

    // Default structured UI (f3 / g3.1)
    expect(wrapper.find('[data-testid="structured-stub"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="structured-stub"]').text()).toContain('运行产出弹窗')
    expect(wrapper.find('[data-testid="artifact-preview-mode-structured"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="artifact-preview-raw-json"]').exists()).toBe(false)

    // Switch to raw JSON then back (f4)
    await wrapper.find('[data-testid="artifact-preview-mode-raw"]').trigger('click')
    await nextTick()
    expect(wrapper.find('[data-testid="artifact-preview-raw-json"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="structured-stub"]').exists()).toBe(false)
    await wrapper.find('[data-testid="artifact-preview-mode-structured"]').trigger('click')
    await nextTick()
    expect(wrapper.find('[data-testid="structured-stub"]').exists()).toBe(true)

    // Enter raw again, then select another structured artifact → resets to structured (g1.3)
    await wrapper.find('[data-testid="artifact-preview-mode-raw"]').trigger('click')
    await nextTick()
    expect(wrapper.find('[data-testid="artifact-preview-raw-json"]').exists()).toBe(true)
    await rows[1].trigger('click')
    await flushPromises()
    await nextTick()
    expect(rows[1].attributes('aria-selected')).toBe('true')
    expect(wrapper.find('[data-testid="structured-stub"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="structured-stub"]').text()).toContain('计划标题')
    expect(wrapper.find('[data-testid="artifact-preview-raw-json"]').exists()).toBe(false)

    // Download present; delete/copy/export hidden (g1.1 / g1.4 / g3.2)
    expect(wrapper.find('[data-testid="artifact-preview-download-raw"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="artifact-preview-delete"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="artifact-preview-copy"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="artifact-preview-download-png"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="artifact-preview-zoom"]').exists()).toBe(false)

    wrapper.unmount()
  })

  it('loads real image preview for screenshot artifacts (g4.3)', async () => {
    const shot = artifact({
      id: 'a-img',
      name: 'screenshot-abc.png',
      kind: 'image',
      nodeId: 'test',
      sizeBytes: 4096,
    })
    apiMocks.getRun.mockResolvedValue({
      id: 'run-1',
      title: 'shots',
      workflowName: 'wf',
      artifacts: [shot],
    })
    apiMocks.artifactContent.mockResolvedValue({ ...shot, content: '' })
    const createObjectURL = vi.fn(() => 'blob:test-image')
    vi.stubGlobal('URL', {
      ...URL,
      createObjectURL,
      revokeObjectURL: vi.fn(),
    })

    const wrapper = mountModal()
    await flushPromises()
    await vi.waitFor(() => {
      expect(wrapper.find('[data-testid="artifact-preview-image-wrap"]').exists()).toBe(true)
    })
    const img = wrapper.find('[data-testid="artifact-preview-image-wrap"] img')
    expect(img.exists()).toBe(true)
    expect(img.attributes('src')).toBe('blob:test-image')
    expect(img.attributes('alt')).toBe('screenshot-abc.png')
    wrapper.unmount()
  })

  it('keeps footer open-run and done actions (g2.5)', async () => {
    apiMocks.getRun.mockResolvedValue({
      id: 'run-1',
      title: 't',
      workflowName: 'wf',
      artifacts: [],
    })
    const wrapper = mountModal()
    await flushPromises()
    expect(wrapper.find('[data-testid="run-output-open-run"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="run-output-done"]').exists()).toBe(true)
    await wrapper.find('[data-testid="run-output-open-run"]').trigger('click')
    expect(push).toHaveBeenCalledWith('/runs/run-1')
    wrapper.unmount()
  })
})
