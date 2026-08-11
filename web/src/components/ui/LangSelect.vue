<script setup lang="ts">
import { ref, computed, onMounted, onBeforeUnmount } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from './Icon.vue'
import type { AppLocale } from '@/lib/shared/locale'

const LOCALE_LABELS: Record<AppLocale, string> = {
  en: 'English',
  'zh-CN': '中文',
}

const LANG_OPTIONS: AppLocale[] = ['en', 'zh-CN']

const props = defineProps<{ modelValue: AppLocale }>()
const emit = defineEmits<{
  (e: 'update:modelValue', v: AppLocale): void
}>()

const { t } = useI18n()
const open = ref(false)
const root = ref<HTMLElement | null>(null)

const currentLabel = computed(() => LOCALE_LABELS[props.modelValue])

function toggle() {
  open.value = !open.value
}

function choose(loc: AppLocale) {
  emit('update:modelValue', loc)
  open.value = false
}

function onDocClick(e: MouseEvent) {
  if (open.value && root.value && !root.value.contains(e.target as Node)) open.value = false
}

onMounted(() => document.addEventListener('click', onDocClick))
onBeforeUnmount(() => document.removeEventListener('click', onDocClick))
</script>

<template>
  <div ref="root" class="relative">
    <button
      type="button"
      class="flex items-center gap-2 border border-line bg-surface px-3 py-1.5 text-sm text-txt2 transition hover:bg-elevated"
      :class="{ 'border-accent/60 text-txt': open }"
      :aria-label="t('shell.langSelect')"
      aria-haspopup="listbox"
      :aria-expanded="open"
      @click.stop="toggle"
    >
      <Icon name="globe" :size="15" class="text-txt3" />
      <span>{{ currentLabel }}</span>
      <Icon name="chevron-down" :size="14" class="text-txt3" />
    </button>

    <div v-if="open" role="listbox" class="card absolute right-0 z-30 mt-1.5 w-40 overflow-hidden">
      <div class="p-1">
        <button
          v-for="loc in LANG_OPTIONS"
          :key="loc"
          type="button"
          role="option"
          class="flex w-full items-center gap-2 px-2.5 py-2 text-left text-sm transition hover:bg-elevated"
          :class="modelValue === loc ? 'bg-accent-dim text-txt' : 'text-txt2'"
          :aria-selected="modelValue === loc"
          @click.stop="choose(loc)"
        >
          <span class="flex-1">{{ LOCALE_LABELS[loc] }}</span>
          <Icon v-if="modelValue === loc" name="check" :size="14" class="text-accent-2" />
        </button>
      </div>
    </div>
  </div>
</template>
