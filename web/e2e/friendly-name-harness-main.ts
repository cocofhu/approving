import '../src/styles/global.css'
import { createApp, defineComponent, h, ref } from 'vue'
import { i18n } from '../src/lib/shared/i18n'
import { initLocale } from '../src/lib/shared/locale'
import { setTheme } from '../src/lib/shared/theme'
import { installIdleScrollbar } from '../src/lib/shared/idleScrollbar'
import ArtifactPreview from '../src/components/run/ArtifactPreview.vue'
import ArtifactList from '../src/components/run/ArtifactList.vue'
import type { Artifact } from '../src/lib/shared/types'
import { api } from '../src/lib/api/api'

const params = new URLSearchParams(location.search)
const scenario = params.get('scenario') || 'preview'

initLocale()
setTheme('light')
installIdleScrollbar()

const clarifiedDoc = {
  title: '产物预览与相关表面显示「需求澄清」友好名',
  summary: '顶栏应对 clarified_requirement.json 显示人读标签。',
  goals: ['用户能认出需求澄清'],
  in_scope: ['顶栏友好名'],
  out_of_scope: ['改存储文件名'],
  functional_requirements: [{ title: '顶栏', detail: '显示需求澄清', acceptance_criteria: ['可见'] }],
  assumptions: ['徽章保留'],
  dependencies: ['artifactFriendlyNameKey'],
  constraints: ['不改 MCP 文件名'],
}

const notesArt: Artifact = {
  id: 'art-notes',
  name: 'notes.json',
  kind: 'json',
  nodeId: 'react',
  runId: 'run-friendly-e2e',
  workflowName: 'wf',
  sizeBytes: 12,
  createdAt: '2026-09-09T00:00:00Z',
}

const clarifiedArt: Artifact = {
  id: 'art-clarify',
  name: 'clarified_requirement.json',
  kind: 'json',
  nodeId: 'react',
  runId: 'run-friendly-e2e',
  workflowName: 'wf',
  sizeBytes: 100,
  createdAt: '2026-09-09T00:00:00Z',
}

const planArt: Artifact = {
  id: 'art-plan',
  name: 'plan.json',
  kind: 'json',
  nodeId: 'approve',
  runId: 'run-friendly-e2e',
  workflowName: 'wf',
  sizeBytes: 32,
  createdAt: '2026-09-09T00:00:00Z',
}

;(api as { artifactContent: (id: string) => Promise<Artifact & { content: string }> }).artifactContent =
  async (id: string) => {
    if (id === clarifiedArt.id) {
      return { ...clarifiedArt, content: JSON.stringify(clarifiedDoc) }
    }
    if (id === planArt.id) {
      return { ...planArt, content: JSON.stringify({ title: '产物预览显示需求澄清友好名', goals: [] }) }
    }
    return { ...notesArt, content: JSON.stringify({ note: 'plain file' }) }
  }
;(api as { artifactDownloadUrl: (id: string) => string }).artifactDownloadUrl = (id: string) =>
  `http://127.0.0.1:9/api/artifacts/${id}/download`

const App = defineComponent({
  name: 'FriendlyNameHarness',
  setup() {
    const selected = ref<Artifact>(clarifiedArt)
    return () => {
      if (scenario === 'list') {
        return h(
          'div',
          {
            class: 'mx-auto flex h-[720px] max-w-4xl flex-col border border-line bg-surface',
            'data-testid': 'friendly-name-harness-root',
            'data-scenario': scenario,
          },
          [
            h(ArtifactList, {
              artifacts: [clarifiedArt, notesArt, planArt],
              scope: 'platform',
              activeId: selected.value.id,
              runSections: [
                {
                  runId: 'run-friendly-e2e',
                  runTitle: 'Run 1',
                  items: [clarifiedArt, notesArt, planArt],
                },
              ],
              onSelect: (a: Artifact) => {
                selected.value = a
              },
            }),
          ],
        )
      }
      return h(
        'div',
        {
          class: 'mx-auto flex h-[720px] max-w-4xl flex-col border border-line bg-surface',
          'data-testid': 'friendly-name-harness-root',
          'data-scenario': scenario,
        },
        [
          h(ArtifactPreview, {
            artifact: scenario === 'notes' ? notesArt : clarifiedArt,
            scope: 'run',
            annotatable: false,
            hideDelete: true,
          }),
        ],
      )
    }
  },
})

void initLocale().then(() => {
  createApp(App).use(i18n).mount('#app')
})
