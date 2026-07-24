<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import Icon from '../ui/Icon.vue'
import LangSelect from '../ui/LangSelect.vue'
import { theme, toggleTheme } from '@/lib/theme'
import { isDraining } from '@/lib/useShutdownState'
import { locale, setLocale, type AppLocale } from '@/lib/locale'

const emit = defineEmits<{ (e: 'toggle-menu'): void }>()

const { t } = useI18n()
const route = useRoute()
const menuOpen = ref(false)

const themeTitle = computed(() =>
  t(theme.value === 'dark' ? 'shell.theme.toLight' : 'shell.theme.toDark'),
)

watch(
  () => route.path,
  () => {
    menuOpen.value = false
  },
)

function toggleMenu() {
  menuOpen.value = !menuOpen.value
  emit('toggle-menu')
}

function onLocaleSelect(v: AppLocale) {
  void setLocale(v)
}

defineExpose({ menuOpen, toggleMenu })
</script>

<template>
  <header class="safe-area-top relative z-20 flex h-14 shrink-0 items-center gap-3 border-b border-line bg-surface/80 px-4 backdrop-blur md:px-6">
    <button
      class="flex h-11 w-11 shrink-0 items-center justify-center rounded-md text-txt2 hover:bg-elevated hover:text-txt md:hidden"
      :aria-label="t('shell.aria.openNav')"
      @click="toggleMenu"
    >
      <Icon name="menu" :size="20" />
    </button>
    <div class="flex-1" />
    <span
      v-if="isDraining()"
      class="inline-flex items-center gap-1.5 rounded-md border border-warn/45 bg-warn/10 px-2.5 py-1 text-xs font-medium text-warn"
      :title="t('shell.shutdown.drainingTitle')"
    >
      <span class="inline-flex h-1.5 w-1.5 animate-pulse rounded-full bg-warn" />
      {{ t('shell.shutdown.draining') }}
    </span>
    <LangSelect :model-value="locale" @update:model-value="onLocaleSelect" />
    <button
      class="flex h-9 w-9 items-center justify-center rounded-md text-txt2 hover:bg-elevated hover:text-txt"
      :title="themeTitle"
      @click="toggleTheme"
    >
      <Icon :name="theme === 'dark' ? 'sun' : 'moon'" :size="18" />
    </button>
    <button class="relative flex h-9 w-9 items-center justify-center rounded-md text-txt2 hover:bg-elevated hover:text-txt">
      <Icon name="bell" :size="18" />
    </button>
  </header>
</template>
