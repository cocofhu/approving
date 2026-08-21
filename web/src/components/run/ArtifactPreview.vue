<script setup lang="ts">
import Icon from '../ui/Icon.vue'
import AppModal from '../ui/AppModal.vue'
import AppButton from '../ui/AppButton.vue'
import RefreshStrip from './RefreshStrip.vue'
import HardLoadLayer from './HardLoadLayer.vue'
import HtmlPreview from '../ui/HtmlPreview.vue'
import StructuredArtifactView from './StructuredArtifactView.vue'
import SelectionAddToChat from './SelectionAddToChat.vue'

import { useArtifactPreview } from '@/lib/run/useArtifactPreview'
import type { ArtifactPreviewProps, ArtifactPreviewEmit } from '@/lib/run/useArtifactPreview'

const props = withDefaults(defineProps<ArtifactPreviewProps>(), {
  scope: 'run',
  emptyHint: '',
  artifacts: () => [],
  run: null,
  hideDelete: false,
  hideCopy: false,
  hideZoom: false,
  hideExport: false,
  annotatable: false,
})
const emit = defineEmits<ArtifactPreviewEmit>()

const {
  t,
  toast,
  annotateChannel,
  stageEl,
  zoom,
  loading,
  loadErr,
  copying,
  structuredMode,
  contentLoadGen,
  contentCache,
  imageSrc,
  imageDownloadLoading,
  imageDownloadError,
  imageLoadGen,
  showDeleteConfirm,
  deleting,
  deleteError,
  structuredExportRootInline,
  structuredExportRootZoom,
  exporting,
  versionMenuOpen,
  selectedVersionIndex,
  versionChoices,
  showVersionChip,
  selectedChoice,
  displayArtifact,
  viewingHistorical,
  versionChipLabel,
  currentChipLabel,
  selectVersion,
  toggleVersionMenu,
  onVersionMenuDocClick,
  activeContent,
  activeIsHtml,
  activeIsJson,
  jsonState,
  previewBranch,
  isStructuredPreview,
  showStructuredUi,
  showRawJson,
  structuredDoc,
  isImageBranch,
  canAnnotate,
  quoteAnnotate,
  blockZoomGesture,
  showDelete,
  stageAnnotation,
  onHtmlPick,
  onQuoteAdd,
  exportDisabled,
  revokeImageBlob,
  resetImageDownloadState,
  loadImageDownload,
  retryImageDownload,
  handleImageLoadError,
  loadContent,
  copyContent,
  download,
  resolveExportRoot,
  runStructuredExport,
  downloadPng,
  downloadPdf,
  openDeleteConfirm,
  closeDeleteConfirm,
  mapDeleteError,
  confirmDelete,
  renderMarkdown,
  fmtTime,
} = useArtifactPreview(props, emit)
</script>

<template>
  <div class="flex h-full min-h-0 min-w-0 flex-1 flex-col">
    <div v-if="displayArtifact" class="flex items-center gap-2 border-b border-line px-4 py-2.5">
      <span class="chip border-n-artifact/30 text-n-artifact">{{ displayArtifact.kind }}</span>
      <span class="flex-1 truncate text-xs font-medium text-txt">{{ displayArtifact.name }}</span>
      <div
        v-if="showVersionChip"
        class="relative shrink-0"
        data-testid="artifact-preview-version-chip"
      >
        <button
          type="button"
          class="inline-flex items-center gap-0.5 border border-line bg-elevated px-1.5 py-px text-[10px] text-txt2 hover:border-line-strong hover:text-txt"
          :class="{ 'border-accent/60 text-txt': versionMenuOpen }"
          :aria-expanded="versionMenuOpen ? 'true' : 'false'"
          aria-haspopup="listbox"
          :aria-label="t('pages.reactArtifactStage.versionMenu')"
          data-testid="artifact-preview-version-chip-btn"
          @click.stop="toggleVersionMenu"
        >
          <span>{{ currentChipLabel }}</span>
        </button>
        <div
          v-if="versionMenuOpen"
          role="listbox"
          class="absolute right-0 top-full z-20 mt-1 min-w-[7.5rem] border border-line bg-surface py-0.5"
          data-testid="artifact-preview-version-menu"
        >
          <button
            v-for="choice in versionChoices"
            :key="choice.index"
            type="button"
            role="option"
            class="flex w-full items-center px-2.5 py-1.5 text-left text-[11px] transition"
            :class="
              !choice.available
                ? 'cursor-not-allowed text-txt3 opacity-45'
                : selectedChoice?.index === choice.index
                  ? 'bg-accent-dim text-txt'
                  : 'text-txt2 hover:bg-elevated'
            "
            :aria-selected="selectedChoice?.index === choice.index ? 'true' : 'false'"
            :disabled="!choice.available"
            :data-testid="'artifact-preview-version-option-v' + choice.index"
            @click.stop="selectVersion(choice)"
          >
            {{ versionChipLabel(choice) }}
          </button>
        </div>
      </div>
      <span
        v-if="viewingHistorical"
        class="shrink-0 border border-line px-1 py-px text-[10px] text-txt3"
        data-testid="artifact-preview-historical-readonly"
      >{{ t('pages.reactArtifactStage.readonlyBadge') }}</span>
      <div
        v-if="isStructuredPreview"
        class="inline-flex shrink-0 border border-line"
        data-testid="artifact-preview-mode-toggle"
        role="group"
        :aria-label="t('pages.artifactPreview.modeToggleAria')"
      >
        <button
          type="button"
          class="px-2 py-0.5 text-[11px]"
          :class="structuredMode === 'structured' ? 'bg-overlay text-txt' : 'text-txt2 hover:text-txt'"
          data-testid="artifact-preview-mode-structured"
          @click="structuredMode = 'structured'"
        >
          {{ t('pages.artifactPreview.modeStructured') }}
        </button>
        <button
          type="button"
          class="px-2 py-0.5 text-[11px]"
          :class="structuredMode === 'raw' ? 'bg-overlay text-txt' : 'text-txt2 hover:text-txt'"
          data-testid="artifact-preview-mode-raw"
          @click="structuredMode = 'raw'"
        >
          {{ t('pages.artifactPreview.modeRaw') }}
        </button>
      </div>
      <button
        v-if="!hideZoom && !activeIsHtml"
        class="text-txt3 hover:text-txt"
        :title="t('pages.artifactPreview.enlarge')"
        data-testid="artifact-preview-zoom"
        @click="zoom = true"
      >
        <Icon name="expand" :size="15" />
      </button>
      <button
        v-if="!hideCopy"
        class="text-txt3 hover:text-txt disabled:opacity-50"
        :title="copying ? t('common.buttons.copying') : t('pages.artifactPreview.copy')"
        :aria-label="copying ? t('common.buttons.copying') : t('pages.artifactPreview.copy')"
        :disabled="copying"
        :aria-busy="copying ? 'true' : undefined"
        data-testid="artifact-preview-copy"
        @click="copyContent"
      >
        <Icon :name="copying ? 'spinner' : 'copy'" :size="15" :class="copying ? 'animate-spin' : ''" aria-hidden="true" />
      </button>
      <button
        class="text-txt3 hover:text-txt"
        :title="t('pages.artifactPreview.downloadRawTitle')"
        data-testid="artifact-preview-download-raw"
        @click="download"
      >
        <Icon name="download" :size="15" />
      </button>
      <button
        v-if="!hideExport && isStructuredPreview"
        type="button"
        class="chip text-[11px] hover:text-txt disabled:cursor-not-allowed disabled:opacity-45"
        data-testid="artifact-preview-download-png"
        :disabled="exportDisabled"
        @click="downloadPng"
      >
        {{ exporting ? t('pages.artifactPreview.exporting') : t('pages.artifactPreview.downloadPng') }}
      </button>
      <button
        v-if="!hideExport && isStructuredPreview"
        type="button"
        class="chip text-[11px] hover:text-txt disabled:cursor-not-allowed disabled:opacity-45"
        data-testid="artifact-preview-download-pdf"
        :disabled="exportDisabled"
        @click="downloadPdf"
      >
        {{ exporting ? t('pages.artifactPreview.exporting') : t('pages.artifactPreview.downloadPdf') }}
      </button>
      <button
        v-if="showDelete"
        class="text-txt3 hover:text-err"
        :title="t('pages.artifactPreview.delete')"
        data-testid="artifact-preview-delete"
        @click="openDeleteConfirm"
      >
        <Icon name="trash" :size="15" />
      </button>
    </div>
    <div
      ref="stageEl"
      class="flex-1 overflow-hidden"
      data-review-annotate-stage
      :class="activeIsHtml && activeContent && !loading && !loadErr ? 'flex min-h-0 flex-col' : 'scroll-area overflow-y-auto p-4'"
    >
      <template v-if="displayArtifact && isImageBranch">
        <div v-if="loadErr" class="flex h-full items-center justify-center text-center text-[12px] text-err" role="alert">
          {{ t('pages.artifactPreview.loadFailed') }}
        </div>
        <div
          v-else-if="imageDownloadError"
          class="flex h-full flex-col items-center justify-center gap-3 text-center"
          data-testid="artifact-preview-image-error"
        >
          <div class="text-[13px] font-medium text-err">{{ t('pages.artifactPreview.imageLoadFailed') }}</div>
          <AppButton variant="outline" data-testid="artifact-preview-image-retry" @click="retryImageDownload">
            {{ t('pages.artifactPreview.retry') }}
          </AppButton>
        </div>
        <div
          v-else-if="loading || imageDownloadLoading || !imageSrc"
          class="flex h-full items-center justify-center text-[12px] text-txt3"
          data-testid="artifact-preview-image-loading"
        >
          {{ t('pages.artifactPreview.loading') }}
        </div>
        <div
          v-else
          class="flex h-full min-h-[320px] w-full items-center justify-center border border-line bg-base p-3"
          data-testid="artifact-preview-image-wrap"
        >
          <img
            :src="imageSrc!"
            :alt="displayArtifact.name"
            class="max-h-full max-w-full object-contain"
            @error="handleImageLoadError"
          />
        </div>
      </template>
      <template v-else-if="displayArtifact">
        <RefreshStrip v-if="loading && activeContent" />
        <HardLoadLayer
          v-else-if="loading && !activeContent && !loadErr"
          :overlay="false"
          :stuck-after-ms="10_000"
          :stage="t('pages.artifactPreview.loading')"
          @retry="displayArtifact && loadContent(displayArtifact, { force: true })"
        />
        <div
          v-if="loadErr"
          class="mb-2 flex items-center justify-center gap-2 text-center text-[12px] text-err"
          role="alert"
          data-testid="artifact-preview-load-error"
        >
          {{ t('pages.artifactPreview.loadFailed') }}
          <button
            type="button"
            class="inline-flex min-h-11 items-center border border-line px-3 text-[12px] text-txt"
            @click="displayArtifact && loadContent(displayArtifact, { force: true })"
          >
            {{ t('pages.artifactPreview.retry') }}
          </button>
        </div>
        <HtmlPreview
          v-if="previewBranch.kind === 'html' && activeContent"
          :html="activeContent"
          fill-parent
          :inspectable="canAnnotate"
          class="h-full min-h-0"
          @pick="onHtmlPick"
        />
        <div
          v-else-if="showStructuredUi"
          ref="structuredExportRootInline"
          data-testid="structured-artifact-export-root"
          class="structured-artifact-export-root"
        >
          <StructuredArtifactView
            :name="displayArtifact.name"
            :doc="structuredDoc"
            :artifacts="artifacts"
            :run-id="runId || displayArtifact.runId"
          />
        </div>
        <div
          v-else-if="showRawJson && jsonState"
          class="json-code-view scroll-area"
          :class="blockZoomGesture ? '' : 'cursor-zoom-in'"
          data-testid="artifact-preview-raw-json"
          @dblclick="!blockZoomGesture && (zoom = true)"
        >
          <div v-if="!jsonState.ok" class="fallback-tag">{{ t('pages.artifactPreview.fallbackPlainText') }}</div>
          <pre v-html="jsonState.html" />
        </div>
        <div
          v-else-if="previewBranch.kind === 'markdown' && activeContent"
          class="md"
          :class="blockZoomGesture ? '' : 'cursor-zoom-in'"
          v-html="renderMarkdown(activeContent)"
          @dblclick="!blockZoomGesture && (zoom = true)"
        />
        <div
          v-else-if="!loading && !loadErr"
          class="flex h-full items-center justify-center text-center text-[12px] text-txt3"
        >
          {{ t('pages.artifactPreview.contentEmpty') }}
        </div>
      </template>
      <div v-else class="flex h-full items-center justify-center text-center text-[12px] text-txt3">
        {{ emptyHint || t('pages.artifactPreview.emptyHint') }}
      </div>
    </div>
  </div>

  <SelectionAddToChat
    v-if="quoteAnnotate"
    :enabled="quoteAnnotate"
    :root="stageEl"
    @add="onQuoteAdd"
  />

  <AppModal :open="zoom" :title="displayArtifact?.name" :width="960" @close="zoom = false">
    <div
      v-if="displayArtifact && showStructuredUi"
      ref="structuredExportRootZoom"
      data-testid="structured-artifact-export-root-zoom"
      class="structured-artifact-export-root"
    >
      <StructuredArtifactView
        :name="displayArtifact.name"
        :doc="structuredDoc"
        :artifacts="artifacts"
        :run-id="runId || displayArtifact.runId"
      />
    </div>
    <div
      v-else-if="displayArtifact && showRawJson"
      class="json-code-view json-code-view--modal scroll-area -m-5 min-h-[280px] p-5"
      data-testid="artifact-preview-zoom-raw-json"
    >
      <template v-if="jsonState">
        <div v-if="!jsonState.ok" class="fallback-tag">{{ t('pages.artifactPreview.fallbackPlainText') }}</div>
        <pre v-html="jsonState.html" />
      </template>
      <div v-else class="py-8 text-center text-[12px] text-txt3">
        {{ t('pages.artifactPreview.contentEmpty') }}
      </div>
    </div>
    <div
      v-else-if="displayArtifact && previewBranch.kind === 'image'"
      class="flex min-h-[280px] items-center justify-center"
      data-testid="artifact-preview-zoom-image"
    >
      <div v-if="loadErr" class="py-8 text-center text-[12px] text-err" role="alert">
        {{ t('pages.artifactPreview.loadFailed') }}
      </div>
      <div
        v-else-if="imageDownloadError"
        class="flex flex-col items-center justify-center gap-3 py-8 text-center"
      >
        <div class="text-[13px] font-medium text-err">{{ t('pages.artifactPreview.imageLoadFailed') }}</div>
        <AppButton variant="outline" @click="retryImageDownload">
          {{ t('pages.artifactPreview.retry') }}
        </AppButton>
      </div>
      <div
        v-else-if="loading || imageDownloadLoading || !imageSrc"
        class="py-8 text-center text-[12px] text-txt3"
      >
        {{ t('pages.artifactPreview.loading') }}
      </div>
      <div v-else class="flex h-full min-h-[360px] w-full items-center justify-center p-3">
        <img
          :src="imageSrc!"
          :alt="displayArtifact.name"
          class="max-h-[70vh] max-w-full object-contain"
          @error="handleImageLoadError"
        />
      </div>
    </div>
    <div v-else-if="displayArtifact" class="md mx-auto max-w-3xl" v-html="renderMarkdown(activeContent)" />
    <template #footer>
      <span class="mr-auto text-[11px] text-txt3">
        <template v-if="scope === 'platform' && displayArtifact?.workflowName">{{ displayArtifact.workflowName }} · </template>
        {{ displayArtifact?.nodeId }} · {{ displayArtifact ? fmtTime(displayArtifact.createdAt) : '' }} · {{ t('pages.artifactPreview.platformStorage') }}
      </span>
      <button
        v-if="!hideCopy"
        class="chip hover:text-txt"
        data-testid="artifact-preview-zoom-copy"
        @click="copyContent"
      >
        <Icon name="copy" :size="13" />{{ t('pages.artifactPreview.copy') }}
      </button>
      <button
        class="chip hover:text-txt"
        :title="t('pages.artifactPreview.downloadRawTitle')"
        data-testid="artifact-preview-zoom-download-raw"
        @click="download"
      >
        <Icon name="download" :size="13" />{{ t('pages.artifactPreview.download') }}
      </button>
      <button
        v-if="!hideExport && isStructuredPreview"
        type="button"
        class="chip hover:text-txt disabled:cursor-not-allowed disabled:opacity-45"
        data-testid="artifact-preview-zoom-download-png"
        :disabled="exportDisabled"
        @click="downloadPng"
      >
        {{ exporting ? t('pages.artifactPreview.exporting') : t('pages.artifactPreview.downloadPng') }}
      </button>
      <button
        v-if="!hideExport && isStructuredPreview"
        type="button"
        class="chip hover:text-txt disabled:cursor-not-allowed disabled:opacity-45"
        data-testid="artifact-preview-zoom-download-pdf"
        :disabled="exportDisabled"
        @click="downloadPdf"
      >
        {{ exporting ? t('pages.artifactPreview.exporting') : t('pages.artifactPreview.downloadPdf') }}
      </button>
    </template>
  </AppModal>

  <AppModal
    :open="showDeleteConfirm"
    :title="t('pages.artifactPreview.deleteTitle', { name: artifact?.name || '' })"
    :width="440"
    @close="closeDeleteConfirm"
  >
    <div class="space-y-3 text-sm text-txt2">
      <div class="flex items-start gap-2 rounded-md border border-err/30 bg-err/10 px-3 py-2 text-[12px] text-err">
        <Icon name="alert" :size="14" class="mt-0.5 shrink-0" />
        {{ t('pages.artifactPreview.deleteWarning') }}
      </div>
      <p>{{ t('pages.artifactPreview.deleteConfirm', { name: artifact?.name || '' }) }}</p>
      <div v-if="deleteError" class="flex items-start gap-2 rounded-md border border-err/30 bg-err/10 px-3 py-2 text-[12px] text-err">
        <Icon name="alert" :size="14" class="mt-0.5" />{{ deleteError }}
      </div>
    </div>
    <template #footer>
      <AppButton variant="ghost" :disabled="deleting" @click="closeDeleteConfirm">{{ t('common.buttons.cancel') }}</AppButton>
      <AppButton variant="danger" icon="trash" :disabled="deleting" @click="confirmDelete">
        {{ deleting ? t('common.buttons.deleting') : t('common.buttons.confirmDelete') }}
      </AppButton>
    </template>
  </AppModal>
</template>

<style scoped>
.json-code-view {
  background: #1e1e1e;
  border: 1px solid rgb(var(--c-line, 38 38 43));
  padding: 12px 14px;
  overflow: auto;
  max-height: 100%;
}
.json-code-view--modal {
  border: none;
  max-height: none;
}
.json-code-view pre {
  margin: 0;
  font-family: ui-monospace, 'SF Mono', Menlo, Consolas, monospace;
  font-size: 12.5px;
  line-height: 1.55;
  white-space: pre;
  tab-size: 2;
  color: #d4d4d4;
}
.fallback-tag {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  margin-bottom: 10px;
  padding: 3px 8px;
  font-size: 11px;
  color: rgb(var(--c-warn, 251 191 36));
  border: 1px solid rgba(251, 191, 36, 0.35);
  background: rgba(251, 191, 36, 0.08);
}
.json-code-view :deep(.tok-key) {
  color: #9cdcfe;
}
.json-code-view :deep(.tok-str) {
  color: #ce9178;
}
.json-code-view :deep(.tok-num) {
  color: #b5cea8;
}
.json-code-view :deep(.tok-bool),
.json-code-view :deep(.tok-null) {
  color: #569cd6;
}
.json-code-view :deep(.tok-punc),
.json-code-view :deep(.tok-plain) {
  color: #d4d4d4;
}
</style>
