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

const props = withDefaults(
  defineProps<{
    modelValue: AppLocale
    /** default = bordered trigger; ghost = borderless compact (sidebar chrome) */
    variant?: 'default' | 'ghost'
  }>(),
  { variant: 'default' },
)
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
  <div ref="root" class="relative" :data-variant="variant">
    <button
      type="button"
      class="flex items-center text-txt2 transition hover:bg-elevated hover:text-txt"
      :class="
        variant === 'ghost'
          ? ['h-8 gap-1.5 border-0 bg-transparent px-2 text-xs', open ? 'bg-elevated text-txt' : '']
          : ['gap-2 border border-line bg-surface px-3 py-1.5 text-sm', open ? 'border-accent/60 text-txt' : '']
      "
      :aria-label="t('shell.langSelect')"
      aria-haspopup="listbox"
      :aria-expanded="open"
      data-testid="lang-select-trigger"
      @click.stop="toggle"
    >
      <Icon name="globe" :size="variant === 'ghost' ? 16 : 15" class="text-txt3" />
      <span>{{ currentLabel }}</span>
      <Icon name="chevron-down" :size="variant === 'ghost' ? 12 : 14" class="text-txt3" />
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
