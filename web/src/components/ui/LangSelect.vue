<script setup lang="ts">
import { ref, computed, onMounted, onBeforeUnmount, watch, nextTick } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from './Icon.vue'
import type { AppLocale } from '@/lib/shared/locale'
import {
  placeFixedOverlayAbove,
  useFixedOverlayAboveListeners,
  type FixedOverlayAboveStyle,
} from '@/lib/composables/useFixedOverlayAbove'

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
const trigger = ref<HTMLElement | null>(null)
const menu = ref<HTMLElement | null>(null)
const menuStyle = ref<FixedOverlayAboveStyle | null>(null)

const usePortaledMenu = computed(() => props.variant === 'ghost')

const currentLabel = computed(() => LOCALE_LABELS[props.modelValue])

async function repositionMenu() {
  if (!open.value || !usePortaledMenu.value) return
  await nextTick()
  menuStyle.value = await placeFixedOverlayAbove(trigger.value, menu.value, {
    align: 'left',
    gap: 6,
    width: 160,
  })
}

const { start: startMenuListeners, stop: stopMenuListeners } = useFixedOverlayAboveListeners(
  open,
  repositionMenu,
)

watch(open, async (isOpen) => {
  if (isOpen && usePortaledMenu.value) {
    startMenuListeners()
    await repositionMenu()
  } else {
    stopMenuListeners()
    menuStyle.value = null
  }
})

function toggle() {
  open.value = !open.value
}

function choose(loc: AppLocale) {
  emit('update:modelValue', loc)
  open.value = false
}

function onDocClick(e: MouseEvent) {
  const target = e.target as Node
  if (!open.value) return
  if (root.value?.contains(target) || menu.value?.contains(target)) return
  open.value = false
}

onMounted(() => document.addEventListener('click', onDocClick))
onBeforeUnmount(() => document.removeEventListener('click', onDocClick))
</script>

<template>
  <div ref="root" class="relative" :data-variant="variant">
    <button
      ref="trigger"
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

    <div
      v-if="open && !usePortaledMenu"
      role="listbox"
      class="card absolute right-0 z-30 mt-1.5 w-40 overflow-hidden"
    >
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

    <Teleport v-if="usePortaledMenu" to="body">
      <div
        v-if="open"
        ref="menu"
        role="listbox"
        class="card z-[60] w-40 overflow-hidden"
        data-testid="lang-select-menu"
        data-placement="above"
        :style="menuStyle ?? undefined"
      >
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
    </Teleport>
  </div>
</template>
