<script setup lang="ts">
import { ref, computed, onMounted, onBeforeUnmount } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from './Icon.vue'
import { api } from '@/lib/api'
import type { Project } from '@/lib/types'

const props = withDefaults(
  defineProps<{ modelValue: string; count?: number; open?: boolean }>(),
  {
    modelValue: '',
    open: undefined,
  },
)
const emit = defineEmits<{
  (e: 'update:modelValue', v: string): void
  (e: 'update:open', v: boolean): void
}>()

const { t } = useI18n()
const projects = ref<Project[]>([])
const internalOpen = ref(false)
const search = ref('')
const root = ref<HTMLElement | null>(null)

const isControlled = computed(() => props.open !== undefined)
const open = computed({
  get: () => (isControlled.value ? props.open! : internalOpen.value),
  set: (v: boolean) => {
    if (isControlled.value) emit('update:open', v)
    else internalOpen.value = v
  },
})

const selectedName = computed(() => {
  if (!props.modelValue) return t('common.projectFilter.all')
  return projects.value.find((p) => p.id === props.modelValue)?.name ?? t('common.projectFilter.all')
})

const filtered = computed(() => {
  const q = search.value.trim().toLowerCase()
  if (!q) return projects.value
  return projects.value.filter((p) => p.name.toLowerCase().includes(q))
})

function choose(id: string) {
  emit('update:modelValue', id)
  open.value = false
  search.value = ''
}

function toggle() {
  open.value = !open.value
  if (open.value) search.value = ''
}

function onDocClick(e: MouseEvent) {
  if (open.value && root.value && !root.value.contains(e.target as Node)) open.value = false
}

onMounted(async () => {
  document.addEventListener('click', onDocClick)
  try {
    projects.value = await api.listProjects()
  } catch {
    projects.value = []
  }
})
onBeforeUnmount(() => document.removeEventListener('click', onDocClick))
</script>

<template>
  <div ref="root" class="relative w-full md:w-auto">
    <button
      type="button"
      class="flex w-full min-h-[44px] items-center gap-2 border border-line bg-surface px-3 py-1.5 text-sm text-txt2 transition hover:bg-elevated md:min-h-0 md:w-auto"
      :class="{ 'border-accent/60 text-txt': modelValue || open }"
      @click.stop="toggle"
    >
      <Icon name="folder" :size="14" />
      <span class="min-w-0 flex-1 truncate text-left">{{ selectedName }}</span>
      <span v-if="count != null" class="text-xs text-txt3">{{ count }}</span>
      <Icon name="chevron-down" :size="12" class="opacity-60" />
    </button>
    <div
      v-if="open"
      class="scroll-area absolute left-0 right-0 z-40 mt-1 max-h-72 overflow-auto rounded-md border border-line-strong bg-surface p-1 shadow-lg md:left-auto md:right-0 md:w-64"
      @click.stop
    >
      <input
        v-model="search"
        type="search"
        class="mb-1 w-full rounded border border-line bg-elevated px-2 py-1.5 text-sm text-txt outline-none focus:border-accent"
        :placeholder="t('common.projectFilter.searchPlaceholder')"
      />
      <button
        type="button"
        class="flex w-full items-center rounded px-2 py-1.5 text-left text-sm transition hover:bg-elevated"
        :class="!modelValue ? 'text-accent-2' : 'text-txt2'"
        @click="choose('')"
      >
        {{ t('common.projectFilter.all') }}
      </button>
      <button
        v-for="p in filtered"
        :key="p.id"
        type="button"
        class="flex w-full items-center rounded px-2 py-1.5 text-left text-sm transition hover:bg-elevated"
        :class="modelValue === p.id ? 'text-accent-2' : 'text-txt2'"
        @click="choose(p.id)"
      >
        <span class="truncate">{{ p.name }}</span>
      </button>
      <div v-if="!filtered.length" class="px-2 py-2 text-xs text-txt3">
        {{ t('common.projectFilter.noMatch') }}
      </div>
    </div>
  </div>
</template>
