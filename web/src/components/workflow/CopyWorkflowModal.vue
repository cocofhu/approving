<script setup lang="ts">
import { ref, watch, computed, onBeforeUnmount } from 'vue'
import { isAbortError } from '@/lib/liveLogRehydrate'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/ui/Icon.vue'
import AppButton from '@/components/ui/AppButton.vue'
import AppModal from '@/components/ui/AppModal.vue'
import { api } from '@/lib/api'
import type { Workflow } from '@/lib/types'

const props = defineProps<{
  open: boolean
  sourceId: string
  sourceName: string
  suggestedName: string
  existingNames: string[]
}>()

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'copied', wf: Workflow): void
}>()

const { t } = useI18n()
const name = ref('')
const error = ref('')
const copying = ref(false)
let copyAbort: AbortController | null = null
let copyGen = 0

const modalTitle = computed(() => t('pages.copyWorkflow.title', { name: props.sourceName }))

watch(
  () => props.open,
  (open) => {
    if (open) {
      name.value = props.suggestedName
      error.value = ''
      copying.value = false
    } else {
      copyAbort?.abort()
      copyAbort = null
      copyGen++
      copying.value = false
    }
  },
)

onBeforeUnmount(() => {
  copyAbort?.abort()
  copyAbort = null
  copyGen++
})

function clearError() {
  error.value = ''
}

function validateLocal(): string | null {
  const n = name.value
  if (!n || /^\s+$/.test(n)) return t('pages.copyWorkflow.nameEmpty')
  if (props.existingNames.includes(n)) return t('pages.copyWorkflow.nameExists')
  return null
}

async function confirmCopy() {
  if (copying.value) return
  clearError()
  const localErr = validateLocal()
  if (localErr) {
    error.value = localErr
    return
  }
  copyAbort?.abort()
  const gen = ++copyGen
  copyAbort = new AbortController()
  copying.value = true
  try {
    const wf = await api.copyWorkflow(props.sourceId, name.value, { signal: copyAbort.signal })
    if (gen !== copyGen) return
    emit('copied', wf)
  } catch (e: any) {
    if (gen !== copyGen || isAbortError(e) || copyAbort.signal.aborted) return
    error.value = String(e?.message || e)
  } finally {
    if (gen === copyGen) copying.value = false
  }
}
</script>

<template>
  <AppModal :open="open" :title="modalTitle" @close="emit('close')">
    <div class="space-y-4">
      <div class="flex items-center gap-2.5 rounded-md border border-line bg-base/50 px-3 py-3 text-[12px] text-txt2">
        <div class="flex h-7 w-7 shrink-0 items-center justify-center rounded-md bg-accent-dim text-accent-2">
          <Icon name="workflow" :size="14" />
        </div>
        <div>{{ t('pages.copyWorkflow.intro', { name: sourceName }) }}</div>
      </div>
      <div>
        <label class="mb-2 block text-[13px] font-medium text-txt">
          {{ t('pages.copyWorkflow.nameLabel') }} <span class="text-err">*</span>
        </label>
        <input
          v-model="name"
          type="text"
          class="w-full rounded-md border bg-base px-3 py-2.5 text-sm text-txt outline-none transition focus:border-accent-2/60"
          :class="error ? 'border-err' : 'border-line'"
          :placeholder="t('pages.copyWorkflow.namePlaceholder')"
          autocomplete="off"
          @input="clearError"
          @keydown.enter.prevent="confirmCopy"
        />
        <div
          v-if="error"
          class="mt-2 flex items-start gap-1.5 rounded-md border border-err/30 bg-err/10 px-2.5 py-2 text-[12px] text-err"
        >
          <Icon name="alert" :size="13" class="mt-0.5 shrink-0" />
          {{ error }}
        </div>
        <p class="mt-2 text-[12px] leading-relaxed text-txt3">
          {{ t('pages.copyWorkflow.nameHint') }}
        </p>
      </div>
    </div>
    <template #footer>
      <AppButton variant="ghost" :disabled="copying" @click="emit('close')">{{ t('common.buttons.cancel') }}</AppButton>
      <AppButton variant="primary" icon="copy" :disabled="copying" :aria-busy="copying ? 'true' : undefined" @click="confirmCopy">
        {{ copying ? t('common.buttons.copying') : t('common.buttons.confirmCopy') }}
      </AppButton>
    </template>
  </AppModal>
</template>
