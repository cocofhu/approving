<script setup lang="ts">
/**
 * Run 详情「复审」面板壳：ReviewShell + 产物舞台 + ReviewComposer。
 */
import { computed, ref } from 'vue'
import ReviewShell from '@/components/run/ReviewShell.vue'
import ReviewComposer from '@/components/run/ReviewComposer.vue'
import ClarifyBootLoader from '@/components/run/ClarifyBootLoader.vue'
import ReactArtifactStage from '@/components/run/ReactArtifactStage.vue'
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

const props = defineProps<{
  mobile: boolean
  node: WFNode
  nodeRun: NodeRun
  run: Run
  clarify: {
    nodeId: string
    iteration?: number
    turns: ClarifyTurn[]
    done: boolean
    previewArtifact?: string
  } | null
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

const reviewChatRef = ref<{
  applyReviewFrame?: (frame: any) => boolean | void
  applyAcpEvents?: (events: any[] | undefined, nodeId?: string) => boolean | void
  discardLastQueued?: () => void
  isSessionBusy?: () => boolean
  isChatReady?: () => boolean
} | null>(null)

const remoteKind = computed(() => (props.node.type === 'app_preview' ? 'app' : 'off'))

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
  <ReviewShell
    class="h-full min-h-0"
    :mobile="mobile"
    :sidebar-width="REVIEW_SIDEBAR"
    :storage-key="REVIEW_SHELL_WIDTH_KEY_REVIEW"
  >
    <template #stage>
      <ReactArtifactStage
        :artifacts="run.artifacts || []"
        :preview-artifact="clarify?.previewArtifact"
        :run-id="run.id"
        :run="run"
        :node-id="node.id"
        :node-type="node.type"
        :annotatable="inputActive"
        :remote-kind="remoteKind"
        @pick="emit('pick', $event)"
        @staged-pick="emit('stagedPick', $event)"
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
        :turns="clarify.turns ?? []"
        :done="clarify.done"
        :active="inputActive"
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
