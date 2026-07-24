<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '../ui/Icon.vue'
import HtmlPreview from '../ui/HtmlPreview.vue'

const props = defineProps<{
  label: string
  html: string
  highlighted?: boolean
  selected?: boolean
}>()

const { t } = useI18n()
const previewRef = ref<InstanceType<typeof HtmlPreview> | null>(null)

function enlarge() {
  previewRef.value?.openEnlarge()
}
</script>

<template>
  <div
    class="overflow-hidden border bg-elevated transition-colors"
    :class="highlighted ? 'border-accent shadow-[0_0_0_1px_rgba(123,97,255,0.3)]' : 'border-line'"
  >
    <div class="flex items-center gap-1.5 border-b border-line px-2 py-1 text-[10px] text-txt3">
      <span class="min-w-0 flex-1 truncate font-medium text-txt2">
        {{ label }}<span v-if="selected" class="text-accent"> ✓ {{ t('pages.clarify.demoSelected') }}</span>
      </span>
      <button
        type="button"
        class="inline-flex shrink-0 items-center gap-0.5 rounded border border-line bg-surface px-1.5 py-0.5 text-[10px] text-txt2 transition-colors hover:text-txt"
        @click="enlarge"
      >
        <Icon name="expand" :size="11" />{{ t('pages.clarify.demoEnlarge') }}
      </button>
    </div>
    <HtmlPreview ref="previewRef" :html="html" mode="demo" :enlargeable="true" :modal-title="label" />
  </div>
</template>
