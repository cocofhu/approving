<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/ui/Icon.vue'
import { imgSrc } from '@/lib/compositeText'
import { useImageAttachments } from '@/lib/useImageAttachments'
import type { ClarifyImage } from '@/lib/types'
import { nextTick, onMounted, ref, watch } from 'vue'

const props = defineProps<{
  disabled?: boolean
  placeholder?: string
  /** When true, hide image paste/upload UI and only expose textarea. */
  textOnly?: boolean
}>()

const { t } = useI18n()
const text = defineModel<string>('text', { default: '' })
const images = defineModel<ClarifyImage[]>('images', { default: () => [] })

const { attachments, fileInput, onPickFiles, onPaste, removeAttachment } = useImageAttachments()

const textareaRef = ref<HTMLTextAreaElement | null>(null)
const overflowScroll = ref(false)

const defaultPlaceholder = computed(() =>
  props.textOnly ? t('common.paragraphInput.placeholderTextOnly') : t('common.paragraphInput.placeholderWithImages'),
)

function autoGrow() {
  const el = textareaRef.value
  if (!el) return
  el.style.height = 'auto'
  const h = Math.min(Math.max(el.scrollHeight, 72), 320)
  el.style.height = `${h}px`
  overflowScroll.value = el.scrollHeight > 320
}

function onTextInput() {
  autoGrow()
}

function onTextPaste(e: ClipboardEvent) {
  if (!props.textOnly) onPaste(e)
  nextTick(autoGrow)
}

watch(
  attachments,
  (v) => {
    if (!props.textOnly) images.value = v
  },
  { deep: true },
)
watch(
  images,
  (v) => {
    if (!props.textOnly && v !== attachments.value) attachments.value = v ? [...v] : []
  },
  { immediate: true, deep: true },
)
watch(text, () => nextTick(autoGrow), { immediate: true })
watch(attachments, () => nextTick(autoGrow), { deep: true })
onMounted(() => nextTick(autoGrow))
</script>

<template>
  <div data-testid="paragraph-input-root" :data-text-only="textOnly ? '1' : '0'">
    <div v-if="!textOnly && attachments.length" class="mb-2 flex flex-wrap gap-1.5">
      <div v-for="(im, ii) in attachments" :key="ii" class="relative">
        <img :src="imgSrc(im)" class="h-14 w-14 rounded-md border border-line object-cover" />
        <button
          type="button"
          class="absolute -right-1.5 -top-1.5 flex h-4 w-4 items-center justify-center rounded-full bg-err text-white"
          :disabled="disabled"
          @click="removeAttachment(ii)"
        >
          <Icon name="close" :size="9" />
        </button>
      </div>
    </div>
    <div class="flex items-end gap-2">
      <input v-if="!textOnly" ref="fileInput" type="file" accept="image/*" multiple class="hidden" @change="onPickFiles" />
      <button
        v-if="!textOnly"
        type="button"
        class="flex h-10 w-10 shrink-0 items-center justify-center rounded-md border border-line text-txt2 hover:border-line-strong disabled:opacity-50"
        data-testid="paragraph-input-attach"
        :disabled="disabled"
        :title="t('common.paragraphInput.addImage')"
        @click="fileInput?.click()"
      >
        <Icon name="paperclip" :size="16" />
      </button>
      <textarea
        ref="textareaRef"
        v-model="text"
        data-testid="paragraph-input"
        class="input min-h-[72px] flex-1 resize-none disabled:opacity-60"
        :class="overflowScroll ? 'scroll-area max-h-[320px] overflow-y-auto' : 'overflow-y-hidden'"
        :disabled="disabled"
        :placeholder="placeholder || defaultPlaceholder"
        @input="onTextInput"
        @paste="onTextPaste"
      />
    </div>
  </div>
</template>
