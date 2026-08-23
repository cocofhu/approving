<script setup lang="ts">
import { useI18n } from 'vue-i18n'

const props = withDefaults(
  defineProps<{
    url: string
    title?: string
  }>(),
  { title: 'preview' },
)

const { t } = useI18n()

function openTab() {
  const u = (props.url || '').trim()
  if (u) window.open(u, '_blank', 'noopener,noreferrer')
}
</script>

<template>
  <div class="flex h-full min-h-0 flex-col" data-testid="app-preview-external-url-frame">
    <div
      class="flex shrink-0 flex-wrap items-center gap-2 border-b border-line bg-elevated px-2 py-1.5 text-xs text-txt2"
    >
      <span class="text-txt3">{{ t('pages.appPreview.externalUrlHint') }}</span>
      <code class="max-w-full flex-1 truncate text-[11px] text-info">{{ url }}</code>
      <button
        type="button"
        class="rounded border border-line px-2 py-0.5 text-[11px] text-txt hover:bg-surface"
        @click="openTab"
      >
        {{ t('pages.appPreview.directOpenTab') }}
      </button>
    </div>
    <iframe
      :src="url"
      class="h-full w-full flex-1 border-0 bg-base"
      :title="title"
      referrerpolicy="no-referrer"
    />
  </div>
</template>
