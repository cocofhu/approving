<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/ui/Icon.vue'
import AppButton from '@/components/ui/AppButton.vue'
import CodeEditor from '@/components/ui/CodeEditor.vue'
import AgentGitGuide from '@/components/agent/AgentGitGuide.vue'
import EnvCredentialHelpModal, {
  type EnvCredentialHelpSection,
} from '@/components/agent/EnvCredentialHelpModal.vue'
import { BACKEND_AUTH_HINTS, settingsFileAbsPath } from '@/lib/agent/backendAuthGuide'
import {
  getRegionPolicy,
  isManagedRegionKey,
  normalizeRegions,
} from '@/lib/shared/regionPolicy'
import {
  kvToRec,
  recToKV,
  type AgentStudioDraft,
} from '@/lib/agent/agentStudioDraft'

const props = defineProps<{
  draft: AgentStudioDraft
  /** agent = Studio 单 Agent；shared = 项目共享配置（可继续写 Token） */
  context?: 'agent' | 'shared'
}>()
const emit = defineEmits<{ toast: [msg: string]; 'open-settings-file': [] }>()

const { t } = useI18n()

const envContext = computed(() => props.context ?? 'agent')
const preferSharedAuth = computed(() => envContext.value === 'agent')

const envRaw = ref(false)
const envRawText = ref('')
const rawError = ref('')
const envHelpOpen = ref(false)
const envHelpSection = ref<EnvCredentialHelpSection>('inject')

const currentAuthHint = computed(() => BACKEND_AUTH_HINTS[props.draft.acpBackend || 'cursor'])
const settingsPath = computed(() => settingsFileAbsPath(props.draft.layout?.configRoot || ''))
const currentRegionPolicy = computed(() => getRegionPolicy(props.draft.acpBackend))

function openEnvHelp(section: EnvCredentialHelpSection) {
  envHelpSection.value = section
  envHelpOpen.value = true
}

function upsertEnv(key: string, value: string) {
  const row = props.draft.env.find((e) => e.k === key)
  if (row) row.v = value
  else props.draft.env.push({ k: key, v: value })
}

function storedRegion(): string {
  const policy = getRegionPolicy(props.draft.acpBackend)
  if (!policy) return ''
  return props.draft.env.find((e) => e.k === policy.regionEnvKey)?.v || ''
}

function updateEnvKey(i: number, value: string) {
  if (isManagedRegionKey(value)) {
    props.draft.env[i].k = ''
    emit('toast', t('pages.agentStudio.region.managedConflict'))
    return
  }
  props.draft.env[i].k = value
}

function toggleEnvRaw() {
  envRaw.value = !envRaw.value
  rawError.value = ''
  if (envRaw.value) envRawText.value = JSON.stringify(kvToRec(props.draft.env), null, 2)
}

function onEnvRaw(text: string) {
  envRawText.value = text
  try {
    const obj = JSON.parse(text)
    if (typeof obj !== 'object' || Array.isArray(obj)) throw new Error(t('pages.agentStudio.dialogs.jsonObject'))
    const incoming = obj as Record<string, string>
    const policy = getRegionPolicy(props.draft.acpBackend)
    if (policy) incoming[policy.regionEnvKey] = storedRegion()
    props.draft.env = recToKV(
      normalizeRegions(incoming, props.draft.acpBackend, 'preserve-special').env,
    )
    rawError.value = ''
  } catch (e: any) {
    rawError.value = t('pages.agentStudio.env.parseError') + (e?.message || e)
  }
}

watch(
  () => props.draft.name,
  () => {
    envRaw.value = false
    rawError.value = ''
  },
)
</script>

<template>
  <div class="flex min-h-0 flex-1 flex-col">
    <div class="flex items-center gap-2 border-b border-line px-4 py-2">
      <AppButton
        type="button"
        size="sm"
        variant="outline"
        icon="help"
        data-test="env-help-inject"
        @click="openEnvHelp('inject')"
      >
        {{ t('pages.agentStudio.envHelp.link') }}
      </AppButton>
      <span class="flex-1" />
      <button class="rounded border border-line px-2 py-1 text-[11px] text-txt2 hover:border-line-strong" @click="toggleEnvRaw">{{ envRaw ? t('pages.agentStudio.mcp.formEdit') : t('pages.agentStudio.mcp.rawJson') }}</button>
    </div>

    <div v-if="envRaw" class="flex min-h-0 flex-1 flex-col">
      <div v-if="rawError" class="border-b border-err/30 bg-err/10 px-4 py-1.5 text-[11px] text-err">{{ rawError }}</div>
      <div class="min-h-0 flex-1"><CodeEditor :model-value="envRawText" language="json" @update:model-value="onEnvRaw" /></div>
    </div>
    <div v-else class="scroll-area flex-1 space-y-2 overflow-y-auto p-4">
      <div
        class="callout mb-3 border border-dashed border-accent/45 bg-accent-dim/40 px-3 py-2.5 text-[12px] leading-6 text-txt2"
        data-test="env-custom-config-callout"
      >
        {{ t('pages.agentStudio.env.customConfigCallout', { path: settingsPath }) }}
        <button
          type="button"
          class="ml-1 bg-transparent p-0 text-accent-2 underline underline-offset-[3px] hover:text-[#c4b5fd]"
          data-test="env-open-settings-file"
          @click="emit('open-settings-file')"
        >
          {{ t('pages.agentStudio.env.openSettingsFile') }}
        </button>
      </div>
      <AgentGitGuide
        :env="draft.env"
        :upsert-env="upsertEnv"
        :credential-type="draft.gitCredentialType"
        :allow-token-recommend="!preferSharedAuth"
        @update:credential-type="draft.gitCredentialType = $event"
        @help="openEnvHelp('git')"
      />
      <div class="mb-3 rounded-lg border border-line bg-base/50 p-3 text-[11px] leading-6 text-txt3">
        <div class="mb-1 flex items-center justify-between gap-2">
          <div class="font-medium text-txt2">
            {{
              preferSharedAuth
                ? t('pages.agentStudio.env.backendAuthPreferShared')
                : t('pages.agentStudio.env.backendAuthTitle')
            }}
          </div>
          <AppButton
            type="button"
            size="sm"
            variant="outline"
            icon="help"
            data-test="env-help-acp"
            @click="openEnvHelp('acp')"
          >
            {{ t('pages.agentStudio.envHelp.link') }}
          </AppButton>
        </div>
        <p v-if="preferSharedAuth" class="mb-1 text-txt2" data-test="env-auth-prefer-shared">
          {{ t('pages.agentStudio.env.backendAuthPreferSharedBody') }}
        </p>
        <p class="font-mono text-accent-2">{{ currentAuthHint.key }}<span v-if="currentAuthHint.alt"> / {{ currentAuthHint.alt }}</span></p>
      </div>
      <template v-for="(e, i) in draft.env" :key="i">
        <div v-if="!isManagedRegionKey(e.k)" class="flex items-center gap-1.5">
          <input :value="e.k" placeholder="KEY" class="w-1/3 rounded border border-line bg-surface px-2 py-1.5 font-mono text-[12px] text-txt outline-none focus:border-accent" @input="updateEnvKey(i, ($event.target as HTMLInputElement).value)" />
          <input v-model="e.v" placeholder="value" class="flex-1 rounded border border-line bg-surface px-2 py-1.5 font-mono text-[12px] text-txt2 outline-none focus:border-accent" />
          <button class="text-txt3 hover:text-err" @click="draft.env.splice(i, 1)"><Icon name="close" :size="14" /></button>
        </div>
      </template>
      <div v-if="currentRegionPolicy" class="border border-accent/30 bg-accent-dim/40 p-3">
        <div class="mb-2 text-[11px] text-txt3">{{ t('pages.agentStudio.region.managedByAcp') }}</div>
        <div class="flex items-center gap-1.5">
          <input :value="currentRegionPolicy.regionEnvKey" readonly :aria-label="t('pages.agentStudio.region.managedKey')" class="w-1/3 rounded border border-line bg-elevated px-2 py-1.5 font-mono text-[12px] text-txt3" />
          <input :value="storedRegion()" readonly :aria-label="t('pages.agentStudio.region.managedValue')" class="flex-1 rounded border border-line bg-elevated px-2 py-1.5 font-mono text-[12px] text-txt3" />
        </div>
      </div>
      <AppButton size="sm" variant="outline" icon="plus" @click="draft.env.push({ k: '', v: '' })">{{ t('pages.agentStudio.env.add') }}</AppButton>
    </div>

    <EnvCredentialHelpModal
      :open="envHelpOpen"
      :section="envHelpSection"
      :backend="draft.acpBackend || 'cursor'"
      @close="envHelpOpen = false"
    />
  </div>
</template>
