// @vitest-environment happy-dom
import { mount, flushPromises } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import { describe, expect, it, vi, beforeEach } from 'vitest'
import zhPages from '@/locales/zh-CN/pages.json'
import zhCommon from '@/locales/zh-CN/common.json'
import ProjectExternalMcpPanel from './ProjectExternalMcpPanel.vue'

const apiMocks = vi.hoisted(() => ({
  getProjectExternalMcp: vi.fn(),
  updateProjectExternalMcp: vi.fn(),
  listProjectMcpKeys: vi.fn(),
  createProjectMcpKey: vi.fn(),
  revokeProjectMcpKey: vi.fn(),
}))

vi.mock('@/lib/api/api', () => ({
  api: apiMocks,
}))

vi.mock('@/lib/composables/useToast', () => ({
  useToast: () => ({ success: vi.fn(), error: vi.fn() }),
}))

function mountPanel() {
  const i18n = createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { pages: zhPages.pages, common: zhCommon.common } },
  })
  return mount(ProjectExternalMcpPanel, {
    props: { projectId: 'proj-1' },
    attachTo: document.body,
    global: { plugins: [i18n] },
  })
}

describe('ProjectExternalMcpPanel', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    apiMocks.getProjectExternalMcp.mockResolvedValue({
      enabled: false,
      enabledPacks: [],
      mcpBaseUrl: 'http://api.example.com/mcp/external/proj-1',
    })
    apiMocks.listProjectMcpKeys.mockResolvedValue([])
    apiMocks.updateProjectExternalMcp.mockResolvedValue({
      enabled: true,
      enabledPacks: ['pm-progress'],
      mcpBaseUrl: 'http://api.example.com/mcp/external/proj-1',
    })
    apiMocks.createProjectMcpKey.mockResolvedValue({
      id: 'k1',
      name: 'cursor',
      key: 'cf_proj_abcd',
      key_prefix: 'cf_proj_••••abcd',
      created_at: '2026-01-01T00:00:00Z',
    })
  })

  it('loads settings and saves enabled packs', async () => {
    const wrapper = mountPanel()
    await flushPromises()
    expect(apiMocks.getProjectExternalMcp).toHaveBeenCalledWith('proj-1')
    expect(wrapper.get('[data-testid="external-mcp-enabled"]').exists()).toBe(true)

    await wrapper.get('[data-testid="external-mcp-save"]').trigger('click')
    await flushPromises()
    expect(apiMocks.updateProjectExternalMcp).toHaveBeenCalled()
  })

  it('opens create-key modal', async () => {
    const wrapper = mountPanel()
    await flushPromises()
    await wrapper.get('[data-testid="external-mcp-create-key"]').trigger('click')
    await flushPromises()
    expect(document.body.textContent).toContain('新建项目 MCP 密钥')
  })
})
