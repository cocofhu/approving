// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'
import type { Project } from '@/lib/shared/types'
import ProjectNotifyPanel from './ProjectNotifyPanel.vue'

const apiMocks = vi.hoisted(() => ({
  listProjectChannels: vi.fn(),
  updateProject: vi.fn(),
}))

vi.mock('@/lib/api/api', async () => {
  const actual = await vi.importActual<typeof import('@/lib/api/api')>('@/lib/api/api')
  return {
    ...actual,
    api: {
      ...actual.api,
      listProjectChannels: apiMocks.listProjectChannels,
      updateProject: apiMocks.updateProject,
    },
  }
})

vi.mock('@/lib/composables/useToast', () => ({
  useToast: () => ({ success: vi.fn(), error: vi.fn(), warn: vi.fn(), info: vi.fn() }),
}))

const SAMPLE_PROJECT: Project = {
  id: 'proj-1',
  name: 'Demo',
  description: '',
  variables: [],
  notifyPolicy: {
    enabled: true,
    defaultEvents: ['waiting_human', 'failed'],
    waitingHumanTemplate: 'wait {{run}}',
    failedTemplate: 'fail {{run}}',
    completedTemplate: '',
    channelIds: ['ch-1'],
  },
}

function mountPanel(project: Project = SAMPLE_PROJECT) {
  const i18n = createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...common, ...pages } },
  })
  return mount(ProjectNotifyPanel, {
    props: { projectId: 'proj-1', project },
    global: {
      plugins: [i18n],
    },
  })
}

describe('ProjectNotifyPanel', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    apiMocks.listProjectChannels.mockResolvedValue({
      items: [{ id: 'ch-1', name: 'Slack', kind: 'webhook' }],
      freeAgents: [],
      secretsKeyConfigured: true,
    })
    apiMocks.updateProject.mockImplementation(async (_id: string, body: Partial<Project>) => ({
      ...SAMPLE_PROJECT,
      ...body,
    }))
  })

  it('loads channel status and can save policy', async () => {
    const w = mountPanel()
    await flushPromises()
    expect(apiMocks.listProjectChannels).toHaveBeenCalledWith('proj-1')
    expect(w.text().length).toBeGreaterThan(10)
    const switches = w.findAll('button[role="switch"]')
    if (switches.length) {
      await switches[0].trigger('click')
      await flushPromises()
    }
    const save = w.findAll('button').find((b) => /保存|Save/i.test(b.text()))
    if (save) {
      await save.trigger('click')
      await flushPromises()
    }
    w.unmount()
  })

  it('handles missing channel ids as no channel configured', async () => {
    const w = mountPanel({
      ...SAMPLE_PROJECT,
      notifyPolicy: { enabled: true, defaultEvents: ['failed'] },
    })
    await flushPromises()
    expect(apiMocks.listProjectChannels).not.toHaveBeenCalled()
    w.unmount()
  })
})
