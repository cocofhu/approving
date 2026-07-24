<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/ui/Icon.vue'

export type CtxTarget = {
  dir: boolean
  path: string
  name: string
  /** blank explorer area (root) */
  blank?: boolean
}

const props = defineProps<{
  open: boolean
  x: number
  y: number
  target: CtxTarget | null
}>()

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'action', action: string): void
}>()

const { t } = useI18n()

const isProtected = computed(() => props.target?.path === 'rules' || props.target?.path === 'skills')
const isBlank = computed(() => !!props.target?.blank)
const isDir = computed(() => !!props.target?.dir && !isBlank.value)
const isFile = computed(() => props.target && !props.target.dir && !props.target.blank)

const uploadEnabled = computed(() => isDir.value)
const deleteDisabled = computed(() => isProtected.value || isBlank.value)
const uploadHint = computed(() => {
  if (!props.target) return ''
  if (isDir.value) return t('common.uploadTo', { path: props.target.path })
  return t('common.foldersOnly')
})

function onAction(action: string, disabled = false) {
  if (disabled || !props.target) return
  emit('action', action)
}

function itemClass(disabled: boolean) {
  return [
    'relative flex cursor-pointer items-center gap-2 px-5 py-1 text-[12px] text-txt2 hover:bg-overlay hover:text-txt',
    disabled ? 'pointer-events-none opacity-[0.38]' : '',
  ]
}
</script>

<template>
  <Teleport to="body">
    <div
      v-if="open && target"
      class="fixed z-[9999] min-w-[210px] border border-line bg-elevated py-1 shadow-card explorer-ctx-menu"
      :style="{ left: x + 'px', top: y + 'px' }"
      @click.stop
    >
      <div :class="itemClass(false)" @click="onAction('newFile')">
        <Icon name="doc" :size="14" class="absolute left-2 text-txt3" />
        <span class="pl-4">{{ t('pages.agentStudio.explorer.newFile') }}</span>
      </div>
      <div :class="itemClass(false)" @click="onAction('newFolder')">
        <Icon name="folder" :size="14" class="absolute left-2 text-txt3" />
        <span class="pl-4">{{ t('pages.agentStudio.explorer.newFolder') }}</span>
      </div>
      <div
        :class="itemClass(!uploadEnabled)"
        @click="onAction('uploadFolder', !uploadEnabled)"
      >
        <Icon name="download" :size="14" class="absolute left-2 rotate-180 text-txt3" />
        <span class="pl-4">{{ t('common.uploadFolder') }}</span>
        <span class="ml-auto pl-2 text-[10px] text-txt3">{{ uploadHint }}</span>
      </div>
      <div class="my-1 h-px bg-line" />
      <div
        :class="itemClass(isBlank)"
        @click="onAction('rename', isBlank)"
      >
        <Icon name="edit" :size="14" class="absolute left-2 text-txt3" />
        <span class="pl-4">{{ t('pages.agentStudio.explorer.rename') }}</span>
        <span v-if="isFile" class="ml-auto pl-4 text-[11px] text-txt3">F2</span>
      </div>
      <div
        :class="itemClass(!isFile)"
        @click="onAction('copyPath', !isFile)"
      >
        <Icon name="copy" :size="14" class="absolute left-2 text-txt3" />
        <span class="pl-4">{{ t('pages.agentStudio.explorer.copyPath') }}</span>
      </div>
      <div class="my-1 h-px bg-line" />
      <div
        :class="[...itemClass(deleteDisabled), 'hover:!bg-err/10 hover:!text-err']"
        @click="onAction('delete', deleteDisabled)"
      >
        <Icon name="close" :size="14" class="absolute left-2 text-txt3" />
        <span class="pl-4">{{ t('pages.agentStudio.explorer.delete') }}</span>
      </div>
    </div>
  </Teleport>
</template>
