<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/ui/Icon.vue'
import AppButton from '@/components/ui/AppButton.vue'
import AgentFilesPanel from '@/components/agent/AgentFilesPanel.vue'
import AgentMcpPanel from '@/components/agent/AgentMcpPanel.vue'
import AgentEnvPanel from '@/components/agent/AgentEnvPanel.vue'
import AgentPromptsPanel from '@/components/agent/AgentPromptsPanel.vue'
import AgentChatTester from '@/components/agent/AgentChatTester.vue'
import ProjectAgentSelect from '@/components/project/ProjectAgentSelect.vue'
import { api, type CreateAgentTestPayload, type ProjectSharedAgentConfig, type SandboxView } from '@/lib/api/api'
import {
  DEFAULT_CONFIG_ROOT,
  DEFAULT_WORKSPACE_DIR,
  defaultConfigRootFor,
  fromDraft,
  fromDraftRaw,
  kvToRec,
  normalizeDraftRegions,
  PROMPT_KEYS,
  recToKV,
  toDraft,
  type AgentStudioDraft,
} from '@/lib/agent/agentStudioDraft'
import {
  ACP_BACKENDS,
  getRegionPolicy,
  isManagedRegionKey,
  normalizeRegions,
  setRegion,
  switchBackendRegions,
  type BackendId,
} from '@/lib/shared/regionPolicy'
import { useBreakpoint } from '@/lib/composables/useBreakpoint'
import { useToast } from '@/lib/composables/useToast'
import { AGENT_SETTINGS_PATH } from '@/lib/agent/agentCreateWizard'

const props = defineProps<{ projectId: string }>()

const { t } = useI18n()
const toast = useToast()
const { isMobile } = useBreakpoint()

type SubTab = 'files' | 'mcp' | 'env' | 'prompts' | 'meta' | 'test'

const loading = ref(true)
const loadError = ref('')
const saving = ref(false)
const draft = ref<AgentStudioDraft | null>(null)
const originalJson = ref('')
const subTab = ref<SubTab>('files')
const justSaved = ref(false)
const filesPanelRef = ref<InstanceType<typeof AgentFilesPanel> | null>(null)
let configRootTouched = false

const agents = ref<{ name: string; projectId?: string }[]>([])
const testAgentName = ref('')

const projectAgents = computed(() =>
  agents.value.filter((a) => a.projectId === props.projectId),
)

function syncTestAgentSelection() {
  const candidates = projectAgents.value
  if (!candidates.length) {
    testAgentName.value = ''
    return
  }
  if (candidates.some((a) => a.name === testAgentName.value)) return
  testAgentName.value = candidates[0]!.name
}

const dirty = computed(() => {
  if (!draft.value) return false
  return JSON.stringify(fromDraftRaw(draft.value)) !== originalJson.value
})

const promptCount = computed(() =>
  draft.value ? PROMPT_KEYS.filter((k) => draft.value!.prompts[k].trim()).length : 0,
)

const subTabs = computed(() => {
  if (!draft.value) return [] as { k: SubTab; l: string }[]
  const d = draft.value
  const pc = promptCount.value
  return [
    { k: 'files' as const, l: t('pages.agentStudio.tabs.files', { n: d.files.length }) },
    { k: 'mcp' as const, l: t('pages.agentStudio.tabs.mcp', { n: d.mcp.length }) },
    {
      k: 'env' as const,
      l: t('pages.agentStudio.tabs.env', { n: d.env.filter((e) => !isManagedRegionKey(e.k)).length }),
    },
    {
      k: 'prompts' as const,
      l: pc ? t('pages.agentStudio.tabs.promptsCount', { n: pc }) : t('pages.agentStudio.tabs.prompts'),
    },
    { k: 'meta' as const, l: t('pages.agentStudio.tabs.meta') },
    { k: 'test' as const, l: t('pages.projectDetail.sharedAgent.testTab') },
  ]
})

const currentRegionPolicy = computed(() =>
  draft.value ? getRegionPolicy(draft.value.acpBackend) : null,
)
const showMetaRegionBlock = computed(() => !!currentRegionPolicy.value)
const metaRegionOptions = computed(() => currentRegionPolicy.value?.options || [])

const displayRegion = computed(() => {
  if (!draft.value) return ''
  const normalized = normalizeRegions(
    kvToRec(draft.value.env),
    draft.value.acpBackend,
    'preserve-special',
  )
  return normalized.special ? '' : normalized.region
})

const specialRegion = computed(() => {
  if (!draft.value) return ''
  const normalized = normalizeRegions(
    kvToRec(draft.value.env),
    draft.value.acpBackend,
    'preserve-special',
  )
  return normalized.special ? normalized.region : ''
})

function joinConfigPath(root: string, sub: string): string {
  return (root || DEFAULT_CONFIG_ROOT).replace(/\/+$/, '') + '/' + sub
}

const derivedPaths = computed(() => {
  if (!draft.value) return []
  const root = draft.value.layout.configRoot || DEFAULT_CONFIG_ROOT
  return [
    {
      label: t('pages.agentStudio.configPaths.mcp'),
      path: joinConfigPath(root, 'mcp.json'),
      note: t('pages.agentStudio.configPaths.mcpNote'),
    },
    {
      label: t('pages.agentStudio.configPaths.rules'),
      path: joinConfigPath(root, 'rules/'),
      note: t('pages.agentStudio.configPaths.rulesNote'),
    },
    {
      label: t('pages.agentStudio.configPaths.skills'),
      path: joinConfigPath(root, 'skills/'),
      note: t('pages.agentStudio.configPaths.skillsNote'),
    },
    {
      label: t('pages.agentStudio.configPaths.commands'),
      path: joinConfigPath(root, 'commands/'),
      note: t('pages.agentStudio.configPaths.commandsNote'),
    },
    {
      label: t('pages.agentStudio.configPaths.env'),
      path: 'container-env',
      note: t('pages.agentStudio.configPaths.envNote'),
    },
  ]
})

function sharedToDraft(cfg: ProjectSharedAgentConfig): AgentStudioDraft {
  const d = toDraft({
    name: '__project_shared__',
    projectId: cfg.defaultProjectId || '',
    acpBackend: (cfg.acpBackend as BackendId) || 'cursor',
    gitCredentialType: cfg.gitCredentialType as AgentStudioDraft['gitCredentialType'],
    files: cfg.files || [],
    mcp: cfg.mcp || [],
    env: cfg.env || {},
    layout: cfg.layout || {},
    prompts: cfg.prompts,
  })
  normalizeDraftRegions(d)
  return d
}

function applyLoaded(cfg: ProjectSharedAgentConfig) {
  const d = sharedToDraft(cfg)
  draft.value = d
  originalJson.value = JSON.stringify(fromDraftRaw(d))
  configRootTouched = false
}

async function load() {
  loading.value = true
  loadError.value = ''
  try {
    const [cfg, agentList] = await Promise.all([
      api.getProjectSharedAgentConfig(props.projectId),
      api.listAgents().catch(() => [] as { name: string; projectId?: string }[]),
    ])
    applyLoaded(cfg)
    agents.value = agentList.map((a) => ({ name: a.name, projectId: a.projectId }))
    syncTestAgentSelection()
  } catch (e: unknown) {
    loadError.value = String((e as { message?: string })?.message || e)
    draft.value = null
  } finally {
    loading.value = false
  }
}

async function save(): Promise<boolean> {
  if (!draft.value) return false
  saving.value = true
  try {
    const payload = fromDraft(draft.value)
    const saved = await api.putProjectSharedAgentConfig(props.projectId, {
      acpBackend: payload.acpBackend || '',
      defaultProjectId: payload.projectId || '',
      gitCredentialType: payload.gitCredentialType || '',
      files: payload.files || [],
      mcp: payload.mcp || [],
      env: payload.env || {},
      layout: payload.layout || {},
      prompts: payload.prompts,
    })
    applyLoaded(saved)
    toast.success(t('pages.projectDetail.saved'))
    justSaved.value = true
    return true
  } catch (e: unknown) {
    toast.error(String((e as { message?: string })?.message || e))
    return false
  } finally {
    saving.value = false
  }
}

function discard() {
  if (!draft.value) return
  void load()
}

function selectAcpBackend(id: BackendId) {
  if (!draft.value) return
  const prev = draft.value.acpBackend
  draft.value.acpBackend = id
  if (!configRootTouched) {
    draft.value.layout.configRoot = defaultConfigRootFor(id)
  }
  if (prev !== id) {
    draft.value.env = recToKV(switchBackendRegions(kvToRec(draft.value.env), id))
  }
}

function selectRegion(id: string) {
  if (!draft.value) return
  draft.value.env = recToKV(setRegion(kvToRec(draft.value.env), draft.value.acpBackend, id))
}

function openSettingsInFiles() {
  if (!draft.value) return
  subTab.value = 'files'
  void Promise.resolve().then(() => {
    filesPanelRef.value?.openPathOrCreate?.(AGENT_SETTINGS_PATH)
  })
}

async function createProjectContextTest(
  profile: string,
  payload: CreateAgentTestPayload,
): Promise<SandboxView> {
  return api.createProjectSharedAgentTest(props.projectId, {
    agentName: profile,
    ...(payload.repos ? { repos: payload.repos } : {}),
    ...(payload.repoUrl ? { repoUrl: payload.repoUrl } : {}),
  })
}

watch(
  () => props.projectId,
  () => {
    testAgentName.value = ''
    subTab.value = 'files'
    void load()
  },
)

watch(projectAgents, () => {
  syncTestAgentSelection()
})

onMounted(() => {
  void load()
})
</script>

<template>
  <div
    class="flex min-h-0 flex-1 flex-col overflow-hidden border border-b-0 border-line bg-surface shadow-[var(--shadow-card)]"
    data-testid="project-shared-agent-panel"
  >
    <div
      class="shrink-0 border-b border-line bg-elevated/55 px-3 py-2.5"
      data-testid="shared-agent-hint"
    >
      <p class="text-[12px] leading-5 text-txt2">
        {{ t('pages.projectDetail.sharedAgent.extendHint') }}
      </p>
    </div>

    <div
      v-if="loading"
      class="flex flex-1 items-center justify-center text-[13px] text-txt3"
    >
      {{ t('common.loading.inProgress') }}
    </div>

    <div
      v-else-if="loadError"
      class="flex flex-1 flex-col items-center justify-center gap-2 px-4 text-center"
    >
      <p class="text-[13px] text-err">{{ loadError }}</p>
      <AppButton size="sm" variant="outline" @click="load">{{ t('common.buttons.retry') }}</AppButton>
    </div>

    <template v-else-if="draft">
      <div class="flex shrink-0 items-center gap-2 border-b border-line px-3 py-2">
        <div class="scroll-area flex min-w-0 flex-1 gap-1 overflow-x-auto">
          <button
            v-for="tabItem in subTabs"
            :key="tabItem.k"
            type="button"
            class="shrink-0 whitespace-nowrap px-2.5 py-1.5 text-[12px] transition"
            :class="
              subTab === tabItem.k
                ? 'border-b-2 border-accent text-txt'
                : 'text-txt3 hover:text-txt2'
            "
            :data-testid="`shared-agent-subtab-${tabItem.k}`"
            @click="subTab = tabItem.k"
          >
            {{ tabItem.l }}
          </button>
        </div>
        <AppButton
          size="sm"
          variant="outline"
          :disabled="!dirty || saving"
          @click="discard"
        >
          {{ t('pages.projectDetail.sharedAgent.discard') }}
        </AppButton>
        <AppButton
          size="sm"
          variant="primary"
          :disabled="!dirty || saving"
          data-testid="shared-agent-save"
          @click="save"
        >
          {{ saving ? t('common.buttons.saving') : t('common.buttons.save') }}
        </AppButton>
      </div>

      <AgentFilesPanel
        v-show="subTab === 'files'"
        ref="filesPanelRef"
        :draft="draft"
        :dirty="dirty"
        :is-mobile="isMobile"
        :save="save"
        @toast="toast.success($event)"
        @error="toast.error($event)"
        @update:just-saved="justSaved = $event"
        @discard="discard"
      />

      <AgentMcpPanel
        v-if="subTab === 'mcp'"
        :draft="draft"
        :is-project-bound="true"
        @toast="toast.success($event)"
      />

      <AgentEnvPanel
        v-if="subTab === 'env'"
        :draft="draft"
        @toast="toast.success($event)"
        @open-settings-file="openSettingsInFiles"
      />

      <AgentPromptsPanel
        v-if="subTab === 'prompts'"
        :draft="draft"
      />

      <div
        v-if="subTab === 'meta'"
        class="scroll-area min-h-0 flex-1 overflow-y-auto p-4"
        data-testid="shared-agent-meta"
      >
        <div class="mb-4 max-w-3xl">
          <h3 class="text-sm font-semibold text-txt">{{ t('pages.agentStudio.meta.layoutTitle') }}</h3>
          <p class="mt-1 text-[12px] leading-6 text-txt3" v-html="t('pages.agentStudio.meta.layoutIntro')" />
        </div>

        <div class="max-w-3xl space-y-4">
          <div>
            <div class="text-[12px] font-medium text-txt2">{{ t('pages.agentStudio.meta.acpBackend') }}</div>
            <p class="mb-2 text-[11px] text-txt3">{{ t('pages.agentStudio.meta.acpBackendDesc') }}</p>
            <div class="grid grid-cols-2 gap-2 sm:grid-cols-4">
              <button
                v-for="b in ACP_BACKENDS"
                :key="b.id"
                type="button"
                class="border px-2 py-3 text-center transition"
                :class="
                  draft.acpBackend === b.id
                    ? 'border-accent bg-accent-dim text-txt'
                    : 'border-line bg-base text-txt2 hover:border-line-strong'
                "
                @click="selectAcpBackend(b.id)"
              >
                <div class="text-[12px] font-semibold">{{ b.label }}</div>
                <div class="mt-0.5 font-mono text-[10px] text-txt3">{{ b.id }}</div>
              </button>
            </div>
          </div>

          <div v-if="showMetaRegionBlock" class="border-t border-dashed border-line pt-4">
            <div class="text-[12px] font-medium text-txt2">
              {{ t('pages.agentStudio.meta.regionTitle') }}
            </div>
            <p class="mb-2 text-[11px] text-txt3">{{ t('pages.agentStudio.meta.regionDesc') }}</p>
            <p
              v-if="specialRegion"
              class="mb-2 border border-warn/35 bg-warn/10 px-2.5 py-2 font-mono text-[11px] text-warn"
            >
              {{ t('pages.agentStudio.region.special', { value: specialRegion }) }}
            </p>
            <div class="grid max-w-md grid-cols-2 gap-2" role="radiogroup">
              <button
                v-for="r in metaRegionOptions"
                :key="r.id"
                type="button"
                class="border px-2 py-3 text-center transition"
                :class="
                  displayRegion === r.id
                    ? 'border-accent bg-accent-dim text-txt'
                    : 'border-line bg-base text-txt2 hover:border-line-strong'
                "
                role="radio"
                :aria-checked="displayRegion === r.id"
                @click="selectRegion(r.id)"
              >
                <div class="text-[12px] font-semibold">{{ t(r.labelKey) }}</div>
                <div class="mt-0.5 font-mono text-[10px] text-txt3">{{ r.id }}</div>
              </button>
            </div>
          </div>

          <label class="block">
            <span class="text-[12px] font-medium text-txt2">
              {{ t('pages.projectDetail.sharedAgent.defaultProjectId') }}
            </span>
            <p class="mb-1.5 text-[11px] text-txt3">
              {{ t('pages.projectDetail.sharedAgent.defaultProjectIdDesc') }}
            </p>
            <input
              v-model="draft.projectId"
              spellcheck="false"
              class="w-full border border-line bg-base px-3 py-2 font-mono text-[12px] text-txt outline-none focus:border-accent"
              :placeholder="projectId"
            />
          </label>

          <label class="block">
            <span class="text-[12px] font-medium text-txt2">{{ t('pages.agentStudio.meta.configRoot') }}</span>
            <p class="mb-1.5 text-[11px] text-txt3">{{ t('pages.agentStudio.meta.configRootDesc') }}</p>
            <input
              v-model="draft.layout.configRoot"
              :placeholder="defaultConfigRootFor(draft.acpBackend)"
              spellcheck="false"
              class="w-full border border-line bg-base px-3 py-2 font-mono text-[12px] text-txt outline-none focus:border-accent"
              @input="configRootTouched = true"
            />
          </label>
          <label class="block">
            <span class="text-[12px] font-medium text-txt2">{{ t('pages.agentStudio.meta.workspaceDir') }}</span>
            <p class="mb-1.5 text-[11px] text-txt3">{{ t('pages.agentStudio.meta.workspaceDirDesc') }}</p>
            <input
              v-model="draft.layout.workspaceDir"
              :placeholder="DEFAULT_WORKSPACE_DIR"
              spellcheck="false"
              class="w-full border border-line bg-base px-3 py-2 font-mono text-[12px] text-txt outline-none focus:border-accent"
            />
          </label>
        </div>

        <div class="mt-5 max-w-3xl">
          <div class="mb-1.5 text-[11px] uppercase tracking-wider text-txt3">
            {{ t('pages.agentStudio.meta.derivedPaths') }}
          </div>
          <div class="overflow-hidden border border-line">
            <table class="w-full text-left text-[12px]">
              <tbody>
                <tr
                  v-for="(e, i) in derivedPaths"
                  :key="e.label"
                  :class="i % 2 ? 'bg-base/40' : ''"
                >
                  <td class="px-3 py-2 text-txt2">{{ e.label }}</td>
                  <td class="px-3 py-2">
                    <code class="bg-base px-1.5 py-0.5 font-mono text-accent-2">{{ e.path }}</code>
                  </td>
                  <td class="px-3 py-2 text-txt3">{{ e.note }}</td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
      </div>

      <div
        v-if="subTab === 'test'"
        class="flex min-h-0 flex-1 flex-col overflow-hidden"
        data-testid="shared-agent-test"
      >
        <div
          class="flex shrink-0 flex-wrap items-start gap-x-3 gap-y-2 border-b border-line px-3 py-2"
          data-testid="shared-agent-test-toolbar"
        >
          <div class="flex min-w-0 items-center gap-2">
            <span class="shrink-0 text-[12px] text-txt2">
              {{ t('pages.projectDetail.sharedAgent.pickAgent') }}
            </span>
            <ProjectAgentSelect
              v-model="testAgentName"
              :agents="projectAgents"
              data-testid="shared-agent-test-pick"
            />
          </div>
          <p class="w-full text-[11px] leading-5 text-txt3">
            {{ t('pages.projectDetail.sharedAgent.testHint') }}
          </p>
        </div>
        <div
          v-if="!projectAgents.length"
          class="flex flex-1 flex-col items-center justify-center gap-2 px-4 text-center text-[13px] text-txt3"
          data-testid="shared-agent-test-empty"
        >
          <Icon name="robot" :size="20" />
          <p>{{ t('pages.projectDetail.sharedAgent.noProjectAgents') }}</p>
        </div>
        <AgentChatTester
          v-else-if="testAgentName"
          :key="testAgentName"
          :profile="testAgentName"
          :home-project-id="projectId"
          :create-test="createProjectContextTest"
        />
      </div>
    </template>
  </div>
</template>
