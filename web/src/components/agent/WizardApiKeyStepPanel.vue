<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import CodeEditor from '@/components/ui/CodeEditor.vue'
import {
  customConfigNoteKey,
  settingsFileAbsPath,
  type BackendAuthGuide,
} from '@/lib/agent/backendAuthGuide'
import type { WizardAuthMode, WizardBackendId } from '@/lib/agent/agentCreateWizard'
import { AGENT_SETTINGS_PATH } from '@/lib/agent/agentCreateWizard'

const props = defineProps<{
  acpBackend: WizardBackendId
  configRoot: string
  authMode: WizardAuthMode
  apiKeyInput: string
  customConfigContent: string
  customConfigError: boolean
  authGuide: BackendAuthGuide
  primaryAuthKey: string
  primaryAuthAlt: string
}>()

const emit = defineEmits<{
  'update:authMode': [mode: WizardAuthMode]
  'update:apiKeyInput': [value: string]
  'update:customConfigContent': [value: string]
}>()

const { t } = useI18n()

const settingsAbsPath = computed(() => settingsFileAbsPath(props.configRoot))
const configNoteKey = computed(() => customConfigNoteKey(props.acpBackend))

function setMode(mode: WizardAuthMode) {
  if (mode === props.authMode) return
  emit('update:authMode', mode)
}
</script>

<template>
  <div>
    <p class="sec-meta">{{ t('pages.agentStudio.wizard.apiKey.meta') }}</p>
    <div class="mb-3 border border-accent/30 bg-accent-dim/40 px-3 py-2.5 text-[12px] leading-5 text-txt2">
      {{
        t('pages.agentStudio.wizard.apiKey.backendBanner', {
          backend: acpBackend,
        })
      }}
      <span class="ml-1 font-mono text-accent-2">configRoot → {{ configRoot }}</span>
    </div>

    <div
      class="mb-4 grid grid-cols-1 gap-2 sm:grid-cols-2"
      role="group"
      :aria-label="t('pages.agentStudio.wizard.apiKey.modeGroup')"
    >
      <button
        type="button"
        class="border px-3 py-3 text-left transition"
        :class="
          authMode === 'apiKey'
            ? 'border-accent bg-accent-dim'
            : 'border-line bg-base hover:border-line-strong'
        "
        :aria-pressed="authMode === 'apiKey'"
        data-test="auth-mode-api-key"
        @click="setMode('apiKey')"
      >
        <strong class="block text-[13px] text-txt">{{ t('pages.agentStudio.wizard.apiKey.modeApiKey') }}</strong>
        <span class="mt-1 block text-[11px] leading-5 text-txt3">
          {{ t('pages.agentStudio.wizard.apiKey.modeApiKeyHint') }}
        </span>
      </button>
      <button
        type="button"
        class="border px-3 py-3 text-left transition"
        :class="
          authMode === 'customConfig'
            ? 'border-accent bg-accent-dim'
            : 'border-line bg-base hover:border-line-strong'
        "
        :aria-pressed="authMode === 'customConfig'"
        data-test="auth-mode-custom-config"
        @click="setMode('customConfig')"
      >
        <strong class="block text-[13px] text-txt">{{ t('pages.agentStudio.wizard.apiKey.modeCustomConfig') }}</strong>
        <span class="mt-1 block text-[11px] leading-5 text-txt3">
          {{ t('pages.agentStudio.wizard.apiKey.modeCustomConfigHint') }}
        </span>
      </button>
    </div>

    <div v-if="authMode === 'apiKey'">
      <div class="mb-4 border border-line bg-base p-3.5">
        <div class="text-[13px] font-semibold text-txt">
          <code class="text-accent-2">{{ primaryAuthKey }}</code>
        </div>
        <p v-if="primaryAuthAlt" class="mt-1 text-[11px] text-txt3">
          {{ t('pages.agentStudio.wizard.apiKey.alias') }}
          <code>{{ primaryAuthAlt }}</code>
        </p>
        <p v-if="authGuide.noteKey" class="mt-2 text-[11px] leading-5 text-txt2">
          {{ t(authGuide.noteKey) }}
        </p>
        <ol class="mt-3 list-decimal space-y-1.5 pl-5 text-[12px] leading-5 text-txt2">
          <li v-for="stepKey in authGuide.pathStepKeys" :key="stepKey">
            {{ t(stepKey) }}
          </li>
        </ol>
        <div class="mt-3 flex flex-wrap gap-2">
          <a
            v-for="link in authGuide.links"
            :key="link.url"
            :href="link.url"
            target="_blank"
            rel="noopener noreferrer"
            class="border border-accent/40 px-2 py-1 text-[11px] text-accent-2 hover:bg-accent-dim"
          >
            {{ t(link.labelKey) }}
          </a>
        </div>
      </div>
      <label class="block">
        <span class="mb-1.5 block text-[12px] font-medium text-txt2">
          {{ t('pages.agentStudio.wizard.apiKey.inputLabel') }}
        </span>
        <input
          :value="apiKeyInput"
          type="password"
          autocomplete="off"
          class="w-full border border-line bg-base px-3 py-2 font-mono text-[12px] text-txt outline-none focus:border-accent"
          :placeholder="t('pages.agentStudio.wizard.apiKey.inputPlaceholder')"
          data-test="api-key-input"
          @input="emit('update:apiKeyInput', ($event.target as HTMLInputElement).value)"
        />
        <p class="mt-1.5 text-[11px] text-txt3">
          {{ t('pages.agentStudio.wizard.apiKey.skipHint') }}
        </p>
      </label>
    </div>

    <div v-else>
      <div class="mb-4 border border-line bg-base p-3.5">
        <div class="text-[13px] font-semibold text-txt">
          {{ t('pages.agentStudio.wizard.apiKey.customConfig.cardTitle') }}
        </div>
        <p class="path-row mt-2 font-mono text-[11px] text-txt3">
          {{ t('pages.agentStudio.wizard.apiKey.customConfig.targetPath') }}
          <span class="text-accent-2">{{ settingsAbsPath }}</span>
          <span class="text-txt3">
            （{{ t('pages.agentStudio.wizard.apiKey.customConfig.agentFiles') }}
            <span class="text-accent-2">{{ AGENT_SETTINGS_PATH }}</span>）
          </span>
        </p>
        <p class="mt-2 text-[11px] leading-5 text-txt2">{{ t(configNoteKey) }}</p>
      </div>
      <label class="block">
        <span class="mb-1.5 block text-[12px] font-medium text-txt2">
          {{ AGENT_SETTINGS_PATH }}
        </span>
        <div class="min-h-[220px] border border-line">
          <CodeEditor
            :model-value="customConfigContent"
            language="json"
            data-test="custom-config-editor"
            @update:model-value="emit('update:customConfigContent', $event)"
          />
        </div>
        <p class="mt-1.5 text-[11px] text-txt3">
          {{ t('pages.agentStudio.wizard.apiKey.customConfig.editorHint') }}
        </p>
        <p v-if="customConfigError" class="mt-2 text-[11px] text-err" data-test="custom-config-error">
          {{ t('pages.agentStudio.wizard.apiKey.customConfig.invalidJson') }}
        </p>
      </label>
    </div>
  </div>
</template>
