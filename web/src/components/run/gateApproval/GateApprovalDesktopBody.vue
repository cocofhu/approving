<script setup lang="ts">
import { toRefs } from 'vue'
import { useGateApprovalCtx } from './gateApprovalContext'
import Icon from '../../ui/Icon.vue'
import ParagraphInput from '../../ui/ParagraphInput.vue'
import HtmlPreview from '../../ui/HtmlPreview.vue'
import RefreshStrip from '../RefreshStrip.vue'
import UpstreamRequirementContext from '../UpstreamRequirementContext.vue'
import PreviewFeedbackChat from '../PreviewFeedbackChat.vue'
import GateProductEditor from '../GateProductEditor.vue'
import ArtifactLoadingPane from '../ArtifactLoadingPane.vue'
import AppPreviewPanel from '../AppPreviewPanel.vue'
import PlanView from '../PlanView.vue'
import ProposalSelectView from '../ProposalSelectView.vue'
import StructuredArtifactView from '../StructuredArtifactView.vue'
import GateApprovalComposer from './GateApprovalComposer.vue'
import GateApprovalColdActions from './GateApprovalColdActions.vue'

const ctx = useGateApprovalCtx()
const s = ctx.s
const productEditorRef = ctx.productEditorRef
const gateStageEl = ctx.gateStageEl
const {
  gate,
  run,
  isMobile,
  formText,
  formImages,
  resolved,
  formError,
  actionSubmitting,
  productDirty,
  savedProductContent,
  savedProductMeta,
  primaryProducts,
  excludedProduces,
  productName,
  canEditProducts,
  isVisualBody,
  isProposalSelect,
  isAppPreview,
  bodyTemplate,
  usesPreviewIssues,
  openPreviewIssueCount,
  previewIssuesLoading,
  previewIssuesError,
  pickedSelector,
  pickedElementImage,
  proposalsDoc,
  proposalsLoading,
  planDoc,
  planLoading,
  productDoc,
  productHtml,
  productLoading,
  productLoadError,
  productHasSavedContent,
  reviewingIteration,
  previewFromArtifactFallback,
  shouldFillPreview,
  shouldFitStructured,
  shouldFillAppPreview,
  useFillLayout,
  useUnifiedPreviewBudget,
  contentFitChromeOffsetPx,
  CONTENT_FIT_PREVIEW_MAX_VH,
  REVIEW_SHELL_WIDTH_KEY_APPROVAL,
  shouldHideGateForm,
  footerActions,
  isColdSession,
  helpColdText,
  helpReviseDetailNoIssuesText,
  helpReviseWithIssuesText,
  canReactRevise,
  canRecordIssue,
  canSubmitReact,
  showHotPass,
  showHotReject,
  hotRejectAllowEmpty,
  composerRejectLabel,
  composerPassDisabled,
  passAction,
  reactText,
  reactImages,
  reactAnnotations,
  reactSending,
  reactError,
  reactQueued,
  reactThinking,
  reactStreamText,
  reactStreamThought,
  reactInterrupted,
  reactStreamCompletedAt,
} = toRefs(s)
const {
  t,
  isActionDisabled,
  actionPendingLabel,
  actionButtonTitle,
  onProductSaved,
  onProductRefresh,
  retryLoadProduct,
  onHtmlPreviewPick,
  onAppPreviewPick,
  clearHtmlPreviewPick,
  loadPreviewIssues,
  choose,
  recordFeedbackIssue,
  sendHotReject,
  onComposerPass,
  onComposerReject,
  cancelReactRevise,
  onSidebarAction,
  renderMarkdown,
} = s
</script>

<template>
  <div class="contents">
      <div
        class="min-h-0 flex-1"
        :class="
          useFillLayout && !(isMobile && isVisualBody)
            ? 'flex flex-col overflow-hidden'
            : isMobile
              ? 'scroll-area overflow-y-auto overflow-x-hidden'
              : 'scroll-area overflow-y-auto p-4'
        "
      >
        <ArtifactLoadingPane
          v-if="proposalsLoading"
          message-key="pages.gateApproval.loadingArtifact"
          :class="useFillLayout ? 'min-h-0 flex-1' : ''"
        />
        <div
          v-else-if="isProposalSelect && proposalsDoc"
          :class="useFillLayout ? 'scroll-area min-h-0 flex-1 overflow-y-auto p-4' : ''"
        >
          <GateProductEditor
            v-if="canEditProducts && run"
            ref="productEditorRef"
            class="mb-4"
            :run-id="run.id"
            :gate-node-id="gate.nodeId"
            :products="primaryProducts"
            :saved-content="savedProductContent"
            :saved-meta="savedProductMeta"
            :artifacts="run.artifacts"
            :run-status="run.status"
            :can-edit="canEditProducts"
            :load-error="productLoadError"
            :excluded-names="excludedProduces"
            :enlargeable="!isMobile"
            @saved="onProductSaved"
            @dirty-change="productDirty = $event"
            @refresh-request="onProductRefresh"
            @retry-load="retryLoadProduct"
          />
          <ProposalSelectView
            :doc="proposalsDoc"
            :resolved-id="resolved"
            @select="choose"
          />
          <div v-if="canReactRevise" class="mt-4">
            <GateApprovalComposer />
          </div>
        </div>
        <div
          v-else-if="canEditProducts && run"
          ref="gateStageEl"
          data-review-annotate-stage
          :class="useFillLayout ? 'scroll-area min-h-0 flex-1 overflow-y-auto' : ''"
        >
          <div
            v-if="reviewingIteration != null"
            class="shrink-0 border-b border-line bg-elevated/60 px-3 py-1.5 text-[11px] text-txt3"
          >
            {{ t('pages.gateApproval.reviewingUpstream', { n: reviewingIteration }) }}
            <span v-if="previewFromArtifactFallback" class="ml-2 text-warn">
              {{ t('pages.gateApproval.reviewingUpstreamFallback') }}
            </span>
          </div>
          <RefreshStrip v-if="productLoading && productHasSavedContent" />
          <GateProductEditor
            ref="productEditorRef"
            :run-id="run.id"
            :gate-node-id="gate.nodeId"
            :products="primaryProducts"
            :saved-content="savedProductContent"
            :saved-meta="savedProductMeta"
            :artifacts="run.artifacts"
            :run-status="run.status"
            :can-edit="canEditProducts"
            :content-loading="productLoading"
            :load-error="productLoadError"
            :excluded-names="excludedProduces"
            :enlargeable="!isMobile"
            :inspectable="isVisualBody"
            @saved="onProductSaved"
            @dirty-change="productDirty = $event"
            @refresh-request="onProductRefresh"
            @retry-load="retryLoadProduct"
            @pick="onHtmlPreviewPick"
          />
          <div
            v-if="isVisualBody"
            class="border-t border-line px-3 pb-3"
            data-testid="editable-visual-feedback"
          >
            <PreviewFeedbackChat
              :run-id="run.id"
              :node-id="gate.nodeId"
              :selector="pickedSelector"
              :element-image="pickedElementImage"
              copy-variant="review"
              @clear-selector="clearHtmlPreviewPick"
              @issues-changed="loadPreviewIssues()"
            />
          </div>
        </div>
        <div
          v-else-if="isVisualBody && productHtml"
          class="overflow-x-hidden border border-line"
        >
          <div
            v-if="reviewingIteration != null"
            class="shrink-0 border-b border-line bg-elevated/60 px-3 py-1.5 text-[11px] text-txt3"
          >
            {{ t('pages.gateApproval.reviewingUpstream', { n: reviewingIteration }) }}
            <span v-if="previewFromArtifactFallback" class="ml-2 text-warn">
              {{ t('pages.gateApproval.reviewingUpstreamFallback') }}
            </span>
          </div>
          <HtmlPreview
            :html="productHtml"
            :mode="isMobile ? 'inline' : 'default'"
            :enlargeable="!isMobile"
            inspectable
            @pick="onHtmlPreviewPick"
          />
          <div v-if="run" class="border-t border-line px-3 pb-3">
            <PreviewFeedbackChat
              :run-id="run.id"
              :node-id="gate.nodeId"
              :selector="pickedSelector"
              :element-image="pickedElementImage"
              copy-variant="review"
              @clear-selector="clearHtmlPreviewPick"
              @issues-changed="loadPreviewIssues()"
            />
          </div>
        </div>
        <div
          v-else-if="isAppPreview && run"
          :class="shouldFillAppPreview ? 'flex min-h-0 flex-1 flex-col overflow-hidden p-4' : 'p-4'"
          data-testid="app-preview-host"
        >
          <AppPreviewPanel
            :run-id="run.id"
            :node-id="gate.nodeId"
            :fill="shouldFillAppPreview"
            :show-feedback="!canReactRevise"
            @issues-changed="loadPreviewIssues()"
            @pick="onAppPreviewPick"
          />
        </div>
        <ArtifactLoadingPane
          v-else-if="planLoading"
          message-key="pages.gateApproval.loadingArtifact"
          :class="useFillLayout ? 'min-h-0 flex-1' : ''"
        />
        <div
          v-else
          ref="gateStageEl"
          data-review-annotate-stage
          :class="[
            useFillLayout ? 'scroll-area min-h-0 flex-1 overflow-y-auto overflow-x-hidden p-4' : 'card p-4',
            isMobile && !useFillLayout ? 'overflow-x-hidden' : '',
          ]"
        >
          <template v-if="productName && productDoc">
            <div
              v-if="reviewingIteration != null"
              class="mb-2 text-[11px] text-txt3"
            >
              {{ t('pages.gateApproval.reviewingUpstream', { n: reviewingIteration }) }}
              <span v-if="previewFromArtifactFallback" class="ml-2 text-warn">
                {{ t('pages.gateApproval.reviewingUpstreamFallback') }}
              </span>
            </div>
            <StructuredArtifactView
              :name="productName"
              :doc="productDoc"
              :artifacts="run?.artifacts"
              :run-id="run?.id"
              :run-status="run?.status"
            />
          </template>
          <PlanView v-else-if="planDoc" :doc="planDoc" />
          <div v-else class="md max-md:text-[13px] max-md:leading-relaxed" v-html="renderMarkdown(gate.bodyMd)" />
        </div>
      </div>

      <UpstreamRequirementContext
        :artifacts="run?.artifacts"
        :run-id="run?.id"
        :run-status="run?.status"
        :product-name="productName"
        :body-template="bodyTemplate"
      />

      <div
        v-if="!(isProposalSelect && proposalsDoc)"
        class="shrink-0 border-t border-line p-4 safe-area-bottom"
        :class="isMobile ? 'sticky bottom-0 z-10 bg-surface/95 backdrop-blur' : ''"
      >
        <div v-if="gate.form?.length && !shouldHideGateForm" class="mb-3 space-y-3">
          <div v-for="f in gate.form" :key="f.key">
            <label class="label">{{ f.label }} <span v-if="f.required" class="text-err">*</span></label>
            <ParagraphInput
              v-model:text="formText[f.key]"
              v-model:images="formImages[f.key]"
              :text-only="isMobile"
              :placeholder="t('pages.gateApproval.formPlaceholder', { label: f.label })"
            />
          </div>
        </div>
        <div v-if="formError" class="mb-2 rounded-md border border-err/30 bg-err/10 px-3 py-2 text-xs text-err" role="alert">
          {{ formError }}
        </div>
        <div v-if="resolved && !actionSubmitting" class="rounded-md border border-ok/30 bg-ok/10 px-3 py-2 text-xs text-ok">
          {{ t('pages.gateApproval.submitted', { label: gate.actions.find((a) => a.id === resolved)?.label }) }}
        </div>
        <div
          v-else-if="usesPreviewIssues && previewIssuesLoading"
          class="flex items-center justify-center gap-2 py-2 text-xs text-txt3"
        >
          <Icon name="spinner" :size="14" class="animate-spin text-accent" />
          {{ t('pages.gateApproval.loadingPreviewIssues') }}
        </div>
        <div v-else>
          <p
            v-if="isColdSession"
            class="mb-2 text-[11px] leading-relaxed text-txt3"
            data-testid="gate-cold-help"
          >
            {{ helpColdText }}
          </p>
          <p
            v-else-if="usesPreviewIssues && openPreviewIssueCount === 0"
            class="mb-2 text-[11px] leading-relaxed text-txt3"
          >
            <b class="font-medium text-txt2">{{ t('pages.clarify.confirmFlow') }}</b>
            {{ t('pages.gateApproval.helpApproveDetail') }}
            <span class="mx-1">·</span>
            <b class="font-medium text-txt2">{{ t('pages.reviewComposer.send') }}</b>
            {{ helpReviseDetailNoIssuesText }}
          </p>
          <p
            v-else-if="usesPreviewIssues && openPreviewIssueCount >= 1"
            class="mb-2 text-[11px] leading-relaxed text-txt3"
          >
            <template v-if="canReactRevise">
              <b class="font-medium text-txt2">{{ t('pages.reviewComposer.send') }}</b>
              {{ t('pages.gateApproval.helpReviseWithIssuesDetail') }}
              <span class="mx-1">·</span>
              {{ t('pages.reviewComposer.openIssuesConfirmHint') }}
            </template>
            <template v-else>{{ helpReviseWithIssuesText }}</template>
          </p>
          <p v-else-if="canEditProducts" class="mb-2 text-[11px] leading-relaxed text-txt3">
            <b class="font-medium text-txt2">{{ t('pages.clarify.confirmFlow') }}</b>
            {{ t('pages.gateApproval.helpApproveDetail') }}
            <span class="mx-1">·</span>
            <b class="font-medium text-txt2">{{ t('pages.reviewComposer.send') }}</b>
            {{ t('pages.gateApproval.helpReviseDetail') }}
          </p>
          <GateApprovalComposer v-if="canReactRevise" class="mb-3" />
          <GateApprovalColdActions v-else layout="desktop" />
        </div>
      </div>
  </div>
</template>
