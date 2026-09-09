<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import Icon from '@/components/ui/Icon.vue'

withDefaults(
  defineProps<{
    mode?: 'stage' | 'sidebar'
    showConfirm?: boolean
  }>(),
  {
    mode: 'sidebar',
    showConfirm: true,
  },
)

const { t } = useI18n()
</script>

<template>
  <div
    v-if="mode === 'stage'"
    class="flex h-full min-h-0 flex-col"
    data-testid="react-connecting-stage"
    aria-busy="true"
  >
    <div class="flex shrink-0 gap-1 border-b border-line px-3 py-2">
      <span class="rounded-md bg-elevated px-2.5 py-1 text-[11px] text-txt2">
        {{ t('pages.reactArtifactStage.pipelineTab') }}
      </span>
      <span class="px-2.5 py-1 text-[11px] text-txt3">
        {{ t('pages.reactArtifactStage.previewTab') }}
      </span>
    </div>
    <div class="flex min-h-0 flex-1 flex-col items-center justify-center gap-3 p-5 text-center">
      <div class="w-full max-w-[420px] space-y-2" aria-hidden="true">
        <div class="h-28 animate-pulse rounded-md bg-elevated" />
        <div class="h-2.5 w-4/5 animate-pulse rounded bg-elevated" />
        <div class="h-2.5 w-1/2 animate-pulse rounded bg-elevated" />
      </div>
      <p class="text-[11px] text-txt3">{{ t('pages.clarify.connectingStageHint') }}</p>
    </div>
  </div>

  <div
    v-else
    class="flex h-full min-h-0 flex-col"
    data-testid="react-connecting-sidebar"
    aria-busy="true"
  >
    <div class="flex shrink-0 items-center gap-2 border-b border-line px-3 py-2.5">
      <Icon name="chat" :size="13" class="text-txt3" />
      <span class="text-[11px] text-txt3">{{ t('pages.clarify.header', { n: 0 }) }}</span>
      <span
        class="ml-auto inline-flex items-center gap-1.5 rounded-full border border-n-clarify/30 bg-n-clarify/10 px-2 py-0.5 text-[10px] text-n-clarify"
        data-testid="react-connecting-pill"
      >
        <i class="h-1.5 w-1.5 animate-pulse rounded-full bg-current" />
        {{ t('pages.clarify.connecting') }}
      </span>
    </div>
    <div class="flex min-h-0 flex-1 flex-col gap-2 overflow-hidden p-4">
      <div class="h-9 animate-pulse rounded-lg bg-elevated" />
      <div class="h-7 w-2/3 animate-pulse rounded-lg bg-elevated" />
      <p class="mt-1 text-center text-[11px] text-txt3">
        {{ t('pages.clarify.connectingHint') }}
      </p>
    </div>
    <div class="shrink-0 border-t border-line p-3">
      <textarea
        disabled
        rows="2"
        class="input min-h-[62px] w-full resize-none disabled:cursor-not-allowed disabled:bg-elevated disabled:text-txt3"
        :placeholder="t('pages.clarify.connectingInputPlaceholder')"
        data-testid="react-connecting-input"
      />
      <div class="mt-2 flex justify-end gap-2">
        <button
          v-if="showConfirm"
          type="button"
          disabled
          class="rounded-md bg-ok px-3 py-1.5 text-xs font-semibold text-white opacity-45"
          data-testid="react-connecting-confirm"
        >
          {{ t('pages.clarify.confirmFlow') }}
        </button>
        <button
          type="button"
          disabled
          class="rounded-md bg-accent px-3 py-1.5 text-xs font-semibold text-white opacity-45"
          data-testid="react-connecting-send"
        >
          {{ t('pages.reviewComposer.send') }}
        </button>
      </div>
    </div>
  </div>
</template>
