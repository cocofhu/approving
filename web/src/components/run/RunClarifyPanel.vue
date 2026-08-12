<script setup lang="ts">
/**
 * Run 详情「澄清」面板壳：失败态 / ClarifyChat / BootLoader。
 */
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/ui/Icon.vue'
import StatusPill from '@/components/ui/StatusPill.vue'
import ClarifyChat from '@/components/run/ClarifyChat.vue'
import ClarifyBootLoader from '@/components/run/ClarifyBootLoader.vue'
import type {
  ClarifyImage,
  ClarifyTurn,
  NodeRunStatus,
  ReactAnnotation,
} from '@/lib/shared/types'

defineProps<{
  sandboxFailed: boolean
  nodeLabel: string
  nodeId: string
  nodeError?: string | null
  clarify: { nodeId: string; iteration?: number; turns: ClarifyTurn[]; done: boolean } | null
  runId: string
  draft: string
  attachments: ClarifyImage[]
  inputActive: boolean
  selStatus?: NodeRunStatus | string | null
}>()

const emit = defineEmits<{
  'update:draft': [v: string]
  'update:attachments': [v: ClarifyImage[]]
  send: [text: string, images: ClarifyImage[], annotations: ReactAnnotation[]]
  finish: []
  cancel: []
}>()

const { t } = useI18n()

const reviewChatRef = ref<{
  applyReviewFrame?: (frame: any) => boolean | void
  applyAcpEvents?: (events: any[] | undefined, nodeId?: string) => boolean | void
  discardLastQueued?: () => void
  isSessionBusy?: () => boolean
  isChatReady?: () => boolean
} | null>(null)

defineExpose({
  applyReviewFrame: (frame: any) => reviewChatRef.value?.applyReviewFrame?.(frame),
  applyAcpEvents: (events: any[] | undefined, nodeId?: string) =>
    reviewChatRef.value?.applyAcpEvents?.(events, nodeId),
  discardLastQueued: () => reviewChatRef.value?.discardLastQueued?.(),
  isSessionBusy: () => !!reviewChatRef.value?.isSessionBusy?.(),
  isChatReady: () => !!reviewChatRef.value?.isChatReady?.(),
})
</script>

<template>
  <div v-if="sandboxFailed" class="scroll-area flex h-full flex-col overflow-y-auto p-4">
    <div class="mb-3 flex items-center gap-2.5">
      <div class="flex h-8 w-8 shrink-0 items-center justify-center rounded-md bg-n-clarify/15 text-n-clarify">
        <Icon name="chat" :size="16" />
      </div>
      <div class="min-w-0 flex-1">
        <div class="text-[14px] font-semibold text-txt [overflow-wrap:anywhere]">{{ nodeLabel }}</div>
        <div class="text-[11px] text-txt3 [overflow-wrap:anywhere]">{{ t('pages.runDetail.clarifyFailed.subtitle', { id: nodeId }) }}</div>
      </div>
      <StatusPill status="failed" />
    </div>
    <div class="mb-3 border border-err/40 bg-err/5 p-3.5">
      <div class="mb-2 flex items-center gap-1.5 text-[11px] font-semibold uppercase tracking-wide text-err">
        <Icon name="alert" :size="14" />
        {{ t('pages.runDetail.clarifyFailed.errorTitle') }}
      </div>
      <pre class="min-w-0 max-w-full overflow-x-auto whitespace-pre font-mono text-[11px] leading-relaxed text-txt2">{{ nodeError }}</pre>
    </div>
    <p class="text-[11px] text-txt3">{{ t('pages.runDetail.clarifyFailed.hint') }}</p>
  </div>
  <ClarifyChat
    v-else-if="clarify"
    ref="reviewChatRef"
    :run-id="runId"
    :node-id="clarify.nodeId"
    :iteration="clarify.iteration ?? 1"
    :draft="draft"
    :attachments="attachments"
    :turns="clarify.turns"
    :done="clarify.done"
    :active="inputActive"
    @update:draft="emit('update:draft', $event)"
    @update:attachments="emit('update:attachments', $event)"
    @send="(text: string, images: ClarifyImage[], anns: ReactAnnotation[]) => emit('send', text, images, anns)"
    @finish="emit('finish')"
    @cancel="emit('cancel')"
  />
  <ClarifyBootLoader v-else :phase="selStatus === 'pending' ? 'pending' : 'starting'" />
</template>
