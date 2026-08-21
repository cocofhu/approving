// @vitest-environment happy-dom
import { createApp, defineComponent, nextTick } from 'vue'
import { createI18n } from 'vue-i18n'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'
import { emptyPrompts, type AgentStudioDraft } from '@/lib/agent/agentStudioDraft'
import { useAgentFilesPanel } from './useAgentFilesPanel'

const baseDraft = (): AgentStudioDraft => ({
  name: 'agent-a',
  projectId: 'proj-1',
  acpBackend: 'cursor',
  files: [
    { path: 'README.md', content: '# hello' },
    { path: 'src/main.ts', content: 'export {}' },
  ],
  mcp: [],
  env: [],
  layout: { configRoot: '/root/.cursor', workspaceDir: '/root/workspace' },
  prompts: emptyPrompts(),
})

function withFilesPanel(isMobile = false) {
  let panel!: ReturnType<typeof useAgentFilesPanel>
  const emit = vi.fn()
  const save = vi.fn(async () => true)
  const props = {
    draft: baseDraft(),
    dirty: false,
    isMobile,
    save,
  }
  const i18n = createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...common, ...pages } },
  })
  const Comp = defineComponent({
    setup() {
      panel = useAgentFilesPanel(props, emit)
      return () => null
    },
  })
  const app = createApp(Comp)
  app.use(i18n)
  app.mount(document.createElement('div'))
  return { panel, app, emit, props, save }
}

describe('useAgentFilesPanel', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('mounts explorer state and opens default file', async () => {
    const { panel, app } = withFilesPanel()
    await nextTick()

    expect(panel.rows.value.length).toBeGreaterThan(0)
    panel.selectDefaultFile()
    expect(panel.activeFile.value?.path).toBeTruthy()
    panel.toggleExplorerCollapsed()
    panel.goFilesList()
    panel.hideCtxMenu()

    document.dispatchEvent(new Event('click'))
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    window.dispatchEvent(new Event('resize'))

    app.unmount()
  })

  it('switches to edit step on mobile when a file is active', async () => {
    const { panel, app } = withFilesPanel(true)
    await nextTick()
    panel.openFile({ path: 'README.md', content: '# hello' })
    await nextTick()
    expect(panel.filesStep.value).toBe('edit')
    app.unmount()
  })
})
