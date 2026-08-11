<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/ui/Icon.vue'
import ChatImageThumb from '@/components/ui/ChatImageThumb.vue'
import ChatImagePreviewModal from '@/components/ui/ChatImagePreviewModal.vue'
import { imgSrc } from '@/lib/shared/compositeText'
import { useChatImagePreview } from '@/lib/composables/useChatImagePreview'
import { useImageAttachments } from '@/lib/composables/useImageAttachments'
import { attachmentDisplayName, isImageAttachment } from '@/lib/shared/attachments'
import type { ClarifyImage } from '@/lib/shared/types'
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

const { attachments, fileInput, notice, onPickFiles, onPaste, removeAttachment } = useImageAttachments()
const { preview: imagePreview, openChatImagePreview, closeChatImagePreview } = useChatImagePreview()

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
    <div
      v-if="!textOnly && notice?.kind === 'error'"
      class="mb-2 border border-err/40 bg-err/10 px-2.5 py-1.5 text-[12px] text-err"
      data-testid="paragraph-attach-notice"
      role="alert"
    >
      {{ notice.text }}
    </div>
    <div v-if="!textOnly && attachments.length" class="mb-2 flex flex-wrap gap-1.5">
      <div v-for="(im, ii) in attachments" :key="ii" class="relative">
        <ChatImageThumb
          v-if="isImageAttachment(im)"
          mode="previewable"
          size="sm"
          thumb-class="rounded-md"
          :src="imgSrc(im)"
          :label="attachmentDisplayName(im, ii)"
          :alt="attachmentDisplayName(im, ii)"
          test-id="paragraph-draft-image-thumb"
          @preview="openChatImagePreview(imgSrc(im), attachmentDisplayName(im, ii))"
        />
        <div
          v-else
          class="flex h-14 max-w-[160px] items-center gap-1.5 border border-line bg-elevated px-2"
          data-testid="paragraph-pending-file-chip"
          :title="attachmentDisplayName(im, ii)"
        >
          <span class="shrink-0 text-[9px] font-medium uppercase text-info">DOC</span>
          <span class="min-w-0 truncate text-[11px] text-txt2">{{ attachmentDisplayName(im, ii) }}</span>
        </div>
        <button
          type="button"
          class="absolute -right-1.5 -top-1.5 flex h-4 w-4 items-center justify-center rounded-full bg-err text-white"
          :disabled="disabled"
          @click.stop="removeAttachment(ii)"
        >
          <Icon name="close" :size="9" />
        </button>
      </div>
    </div>
    <div class="flex items-end gap-2">
      <input v-if="!textOnly" ref="fileInput" type="file" multiple class="hidden" @change="onPickFiles" />
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
    <ChatImagePreviewModal
      :open="!!imagePreview"
      :src="imagePreview?.src || ''"
      :label="imagePreview?.label || ''"
      test-id-prefix="paragraph-image-preview"
      @close="closeChatImagePreview"
    />
  </div>
</template>
