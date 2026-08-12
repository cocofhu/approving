<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { AgentStudioDraft, PromptKey } from '@/lib/agent/agentStudioDraft'

defineProps<{ draft: AgentStudioDraft }>()

const { t } = useI18n()

const PROMPT_FRAGMENTS = computed(() => [
  {
    key: 'upstreamArtifactsHeader' as PromptKey,
    label: t('pages.agentStudio.promptFields.upstreamHeader.label'),
    hint: t('pages.agentStudio.promptFields.upstreamHeader.hint'),
    placeholder: t('pages.agentStudio.promptFields.upstreamHeader.placeholder'),
  },
  {
    key: 'producesContract' as PromptKey,
    label: t('pages.agentStudio.promptFields.producesContract.label'),
    hint: t('pages.agentStudio.promptFields.producesContract.hint'),
    placeholder: t('pages.agentStudio.promptFields.producesContract.placeholder'),
  },
  {
    key: 'reactOpenSuffix' as PromptKey,
    label: t('pages.agentStudio.promptFields.reactOpening.label'),
    hint: t('pages.agentStudio.promptFields.reactOpening.hint'),
    placeholder: t('pages.agentStudio.promptFields.reactOpening.placeholder'),
  },
  {
    key: 'producesRetry' as PromptKey,
    label: t('pages.agentStudio.promptFields.reactMissingArtifact.label'),
    hint: t('pages.agentStudio.promptFields.reactMissingArtifact.hint'),
    placeholder: t('pages.agentStudio.promptFields.reactMissingArtifact.placeholder'),
  },
])
</script>

<template>
  <div class="scroll-area min-h-0 flex-1 overflow-y-auto p-4">
    <div class="mb-4 max-w-3xl">
      <h3 class="text-sm font-semibold text-txt">{{ t('pages.agentStudio.prompts.title') }}</h3>
      <p class="mt-1 text-[12px] leading-6 text-txt3" v-html="t('pages.agentStudio.prompts.intro')" />
    </div>

    <div class="max-w-3xl space-y-4">
      <label v-for="f in PROMPT_FRAGMENTS" :key="f.key" class="block">
        <span class="text-[12px] font-medium text-txt2">{{ f.label }}</span>
        <p class="mb-1.5 text-[11px] text-txt3">{{ f.hint }}</p>
        <textarea
          v-model="draft.prompts[f.key]"
          rows="3"
          spellcheck="false"
          :placeholder="t('pages.agentStudio.prompts.defaultPrefix') + f.placeholder"
          class="w-full resize-y rounded-md border border-line bg-base px-3 py-2 font-mono text-[12px] leading-6 text-txt outline-none focus:border-accent"
        />
      </label>
    </div>

    <p class="mt-4 max-w-3xl text-[11px] leading-5 text-txt3">
      {{ t('pages.agentStudio.prompts.rulesNote') }}
    </p>
  </div>
</template>
