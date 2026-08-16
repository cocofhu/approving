<script setup lang="ts">
/**
 * Run 详情「复审」面板壳：ReviewShell + 产物舞台 + ReviewComposer。
 */
import { ref } from 'vue'
import ReviewShell from '@/components/run/ReviewShell.vue'
import ReviewComposer from '@/components/run/ReviewComposer.vue'
import ClarifyBootLoader from '@/components/run/ClarifyBootLoader.vue'
import AppPreviewPanel from '@/components/run/AppPreviewPanel.vue'
import StructuredProductPanel from '@/components/run/StructuredProductPanel.vue'
import {
  REVIEW_SIDEBAR,
  REVIEW_SHELL_WIDTH_KEY_REVIEW,
} from '@/lib/inbox/reviewLayoutBudget'
import type {
  ClarifyImage,
  ClarifyTurn,
  NodeRun,
  NodeRunStatus,
  ReactAnnotation,
  Run,
  WFNode,
} from '@/lib/shared/types'
import type { AppPreviewPickPayload } from '@/lib/shared/previewPickUrl'

defineProps<{
  mobile: boolean
  node: WFNode
  nodeRun: NodeRun
  run: Run
  clarify: { nodeId: string; iteration?: number; turns: ClarifyTurn[]; done: boolean } | null
  draft: string
  attachments: ClarifyImage[]
  annotations: ReactAnnotation[]
  inputActive: boolean
  confirmError?: string | null
  selStatus?: NodeRunStatus | string | null
}>()

const emit = defineEmits<{
  'update:draft': [v: string]
  'update:attachments': [v: ClarifyImage[]]
  'update:annotations': [v: ReactAnnotation[]]
  send: [text: string, images: ClarifyImage[], annotations: ReactAnnotation[]]
  finish: []
  cancel: []
  pick: [payload: AppPreviewPickPayload]
  stagedPick: [payload: AppPreviewPickPayload | null]
}>()

const historicalPreview = ref(false)
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
  <!-- Left product stage + right review sidebar (page.html RUN 复审 / app_preview VNC) -->
  <ReviewShell
    class="h-full min-h-0"
    :mobile="mobile"
    :sidebar-width="REVIEW_SIDEBAR"
    :storage-key="REVIEW_SHELL_WIDTH_KEY_REVIEW"
  >
    <template #stage>
      <div v-if="node.type === 'app_preview'" class="flex h-full min-h-0 flex-col p-3">
        <AppPreviewPanel
          :run-id="run.id"
          :node-id="node.id"
          fill
          :show-feedback="false"
          @pick="emit('pick', $event)"
          @staged-pick="emit('stagedPick', $event)"
        />
      </div>
      <StructuredProductPanel
        v-else
        :node="node"
        :node-run="nodeRun"
        :run="run"
        annotatable
        @update:historical-preview="historicalPreview = $event"
      />
    </template>
    <template #sidebar>
      <ReviewComposer
        v-if="clarify"
        ref="reviewChatRef"
        mode="review"
        :run-id="run.id"
        :node-id="clarify.nodeId"
        :iteration="clarify.iteration ?? 1"
        :draft="draft"
        :attachments="attachments"
        :annotations="annotations"
        :turns="clarify.turns"
        :done="clarify.done"
        :active="inputActive && !historicalPreview"
        :confirm-error="confirmError"
        @update:draft="emit('update:draft', $event)"
        @update:attachments="emit('update:attachments', $event)"
        @update:annotations="emit('update:annotations', $event)"
        @send="(text: string, images: ClarifyImage[], anns: ReactAnnotation[]) => emit('send', text, images, anns)"
        @finish="emit('finish')"
        @cancel="emit('cancel')"
      />
      <ClarifyBootLoader v-else :phase="selStatus === 'pending' ? 'pending' : 'starting'" />
    </template>
  </ReviewShell>
</template>
