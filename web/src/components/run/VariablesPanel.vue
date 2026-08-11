<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '../ui/Icon.vue'
import VarValueDisplay from '../ui/VarValueDisplay.vue'
import CompositeImageStrip from '../ui/CompositeImageStrip.vue'
import { compositeImages } from '@/lib/shared/compositeText'
import type { RunVar } from '@/lib/shared/types'

const props = defineProps<{ vars: RunVar[] }>()

const { t } = useI18n()

const TYPE_CLS: Record<string, string> = {
  int: 'text-info',
  string: 'text-ok',
  paragraph: 'text-ok',
  bool: 'text-warn',
}
const items = computed(() => props.vars || [])

function imageCount(v: RunVar): number {
  return compositeImages(v.value).length
}
</script>

<template>
  <div class="flex h-full flex-col">
    <div class="border-b border-line px-4 py-2.5 text-[12px] text-txt3">
      {{ t('pages.variablesPanel.header') }}
    </div>
    <div v-if="!items.length" class="flex flex-1 items-center justify-center text-[12px] text-txt3">{{ t('pages.variablesPanel.empty') }}</div>
    <div v-else class="scroll-area min-h-0 flex-1 overflow-y-auto p-3">
      <div
        v-for="v in items"
        :key="v.name"
        class="mb-1.5 rounded-md border border-line bg-elevated px-3 py-2"
      >
        <div class="flex items-center gap-2">
          <Icon name="variable" :size="14" class="text-txt3" />
          <code class="font-mono text-[12px] font-medium text-txt">{{ v.name }}</code>
          <span class="rounded bg-base px-1.5 text-[10px]" :class="TYPE_CLS[v.type]">{{ v.type }}</span>
          <span class="ml-auto font-mono text-[12px]" :class="TYPE_CLS[v.type]">
            <VarValueDisplay
              :value="v.value"
              :quote="v.type === 'string'"
              :hide-image-badge="imageCount(v) > 0"
            />
          </span>
        </div>
        <CompositeImageStrip :value="v.value" size="lg" class="mt-2 pl-6" />
      </div>
    </div>
  </div>
</template>
