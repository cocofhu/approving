<script setup lang="ts">
import { ref, computed, watch, onBeforeUnmount } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '../ui/Icon.vue'
import AppModal from '../ui/AppModal.vue'
import AppButton from '../ui/AppButton.vue'
import RefreshStrip from './RefreshStrip.vue'
import HardLoadLayer from './HardLoadLayer.vue'
import { isAbortError } from '@/lib/liveLogRehydrate'
import HtmlPreview from '../ui/HtmlPreview.vue'
import StructuredArtifactView from './StructuredArtifactView.vue'
import { isImagePreviewArtifact, resolveArtifactPreviewBranch } from './artifactPreviewBranch'
import { renderMarkdown } from '@/lib/markdown'
import { isJsonArtifact, parseJsonState } from '@/lib/highlightJson'
import { fmtTime } from '@/lib/format'
import { api } from '@/lib/api'
import { copyToClipboard } from '@/lib/copyToClipboard'
import { useToast } from '@/lib/useToast'
import {
  exportStructuredArtifact,
  type StructuredExportFormat,
} from '@/lib/exportStructuredArtifact'
import type { Artifact } from '@/lib/types'

const props = withDefaults(
  defineProps<{
    artifact: Artifact | null
    scope?: 'run' | 'platform'
    emptyHint?: string
    artifacts?: Artifact[]
    runId?: string
  }>(),
  {
    scope: 'run',
    emptyHint: '',
    artifacts: () => [],
  },
)

const emit = defineEmits<{
  deleted: [id: string]
}>()

const { t } = useI18n()
const toast = useToast()

const zoom = ref(false)
const loading = ref(false)
const loadErr = ref('')
const copying = ref(false)
let contentLoadGen = 0
let contentLoadAbort: AbortController | null = null
/** Raw API content only — never store highlighted HTML here. */
const contentCache = ref<Record<string, string>>({})

const imageSrc = ref<string | null>(null)
const imageDownloadLoading = ref(false)
const imageDownloadError = ref(false)
let imageBlobUrl: string | null = null
let imageLoadGen = 0

const showDeleteConfirm = ref(false)
const deleting = ref(false)
const deleteError = ref('')

/** Export roots for structured preview (inline + zoom); prefer zoom when open. */
const structuredExportRootInline = ref<HTMLElement | null>(null)
const structuredExportRootZoom = ref<HTMLElement | null>(null)
const exporting = ref(false)

const activeContent = computed(() => (props.artifact ? contentCache.value[props.artifact.id] ?? '' : ''))
const activeIsHtml = computed(() => props.artifact?.kind === 'html')
const activeIsJson = computed(() => isJsonArtifact(props.artifact))

const jsonState = computed(() => {
  if (!activeIsJson.value || !activeContent.value) return null
  return parseJsonState(activeContent.value)
})

/** Reserved structured JSON (any scope). Shared by inline preview and zoom modal. */
const previewBranch = computed(() => {
  const a = props.artifact
  if (!a) return { kind: 'empty' as const }
  return resolveArtifactPreviewBranch({
    name: a.name,
    kind: a.kind,
    content: activeContent.value,
  })
})

const isStructuredPreview = computed(() => previewBranch.value.kind === 'structured')

const structuredDoc = computed(() =>
  previewBranch.value.kind === 'structured' ? previewBranch.value.doc : null,
)

const isImageBranch = computed(() => previewBranch.value.kind === 'image')

/** Disable style export while content is loading, errored, or already exporting. */
const exportDisabled = computed(
  () =>
    exporting.value ||
    loading.value ||
    !!loadErr.value ||
    !isStructuredPreview.value ||
    !props.artifact,
)

function revokeImageBlob() {
  if (imageBlobUrl) {
    URL.revokeObjectURL(imageBlobUrl)
    imageBlobUrl = null
  }
}

function resetImageDownloadState() {
  imageLoadGen++
  revokeImageBlob()
  imageSrc.value = null
  imageDownloadError.value = false
  imageDownloadLoading.value = false
}

async function loadImageDownload(a: Artifact) {
  imageLoadGen++
  const gen = imageLoadGen
  revokeImageBlob()
  imageSrc.value = null
  imageDownloadError.value = false
  imageDownloadLoading.value = true
  try {
    const res = await fetch(api.artifactDownloadUrl(a.id), { credentials: 'include' })
    if (!res.ok) throw new Error(String(res.status))
    const blob = await res.blob()
    if (gen !== imageLoadGen) return
    const url = URL.createObjectURL(blob)
    imageBlobUrl = url
    imageSrc.value = url
  } catch {
    if (gen === imageLoadGen) imageDownloadError.value = true
  } finally {
    if (gen === imageLoadGen) imageDownloadLoading.value = false
  }
}

function retryImageDownload() {
  if (props.artifact) void loadImageDownload(props.artifact)
}

function handleImageLoadError() {
  revokeImageBlob()
  imageSrc.value = null
  imageDownloadError.value = true
}

async function loadContent(a: Artifact, opts?: { force?: boolean }) {
  if (!opts?.force && contentCache.value[a.id] !== undefined) return
  contentLoadAbort?.abort()
  const gen = ++contentLoadGen
  contentLoadAbort = new AbortController()
  loading.value = true
  loadErr.value = ''
  try {
    const full = await api.artifactContent(a.id, { signal: contentLoadAbort.signal })
    if (gen !== contentLoadGen) return
    contentCache.value[a.id] = full.content ?? ''
  } catch (e: any) {
    if (gen !== contentLoadGen || isAbortError(e) || contentLoadAbort.signal.aborted) return
    loadErr.value = t('pages.artifactPreview.loadFailed')
    if (contentCache.value[a.id] === undefined) contentCache.value[a.id] = ''
  } finally {
    if (gen === contentLoadGen) loading.value = false
  }
}

async function copyContent() {
  if (copying.value) return
  copying.value = true
  try {
    const ok = await copyToClipboard(activeContent.value)
    if (!ok) {
      toast.error(t('common.toast.copyFailed'))
    } else {
      toast.success(t('pages.artifactPreview.copied'))
    }
  } finally {
    copying.value = false
  }
}

function download() {
  if (props.artifact) window.open(api.artifactDownloadUrl(props.artifact.id), '_blank')
}

function resolveExportRoot(): HTMLElement | null {
  if (zoom.value && structuredExportRootZoom.value) return structuredExportRootZoom.value
  return structuredExportRootInline.value
}

async function runStructuredExport(format: StructuredExportFormat) {
  if (exportDisabled.value || !props.artifact) return
  const root = resolveExportRoot()
  if (!root) {
    toast.error(t('pages.artifactPreview.exportFailed'))
    return
  }
  exporting.value = true
  try {
    const result = await exportStructuredArtifact(root, props.artifact.name, format)
    if (result.incomplete) {
      toast.warn(t('pages.artifactPreview.exportIncomplete', { filename: result.filename }))
    } else {
      toast.success(t('pages.artifactPreview.exportSuccess', { filename: result.filename }))
    }
  } catch {
    toast.error(t('pages.artifactPreview.exportFailed'))
  } finally {
    exporting.value = false
  }
}

function downloadPng() {
  void runStructuredExport('png')
}

function downloadPdf() {
  void runStructuredExport('pdf')
}

function openDeleteConfirm() {
  deleteError.value = ''
  showDeleteConfirm.value = true
}

function closeDeleteConfirm() {
  if (deleting.value) return
  showDeleteConfirm.value = false
  deleteError.value = ''
}

function mapDeleteError(e: unknown): string {
  const status = (e as { status?: number })?.status
  if (status === 409) return t('pages.artifactPreview.deleteErrorRunNotEnded')
  if (status === 404) return t('pages.artifactPreview.deleteErrorNotFound')
  const msg = e instanceof Error ? e.message : String(e || '')
  return msg || t('pages.artifactPreview.deleteErrorGeneric')
}

async function confirmDelete() {
  const a = props.artifact
  if (!a || deleting.value) return
  deleting.value = true
  deleteError.value = ''
  try {
    await api.deleteArtifact(a.id)
    delete contentCache.value[a.id]
    resetImageDownloadState()
    showDeleteConfirm.value = false
    toast.success(t('pages.artifactPreview.deleteSuccess'))
    emit('deleted', a.id)
  } catch (e) {
    deleteError.value = mapDeleteError(e)
  } finally {
    deleting.value = false
  }
}

watch(
  () => props.artifact?.id,
  (id) => {
    resetImageDownloadState()
    if (props.artifact) {
      void loadContent(props.artifact)
      if (isImagePreviewArtifact(props.artifact.name, props.artifact.kind)) {
        void loadImageDownload(props.artifact)
      }
    } else {
      loading.value = false
      loadErr.value = ''
    }
    if (!id) zoom.value = false
    showDeleteConfirm.value = false
    deleteError.value = ''
    exporting.value = false
  },
  { immediate: true },
)

onBeforeUnmount(() => {
  contentLoadAbort?.abort()
  contentLoadAbort = null
  contentLoadGen++
  imageLoadGen++
  revokeImageBlob()
})
</script>

<template>
  <div class="flex h-full min-h-0 min-w-0 flex-1 flex-col">
    <div v-if="artifact" class="flex items-center gap-2 border-b border-line px-4 py-2.5">
      <span class="chip border-n-artifact/30 text-n-artifact">{{ artifact.kind }}</span>
      <span class="flex-1 truncate text-xs font-medium text-txt">{{ artifact.name }}</span>
      <button v-if="!activeIsHtml" class="text-txt3 hover:text-txt" :title="t('pages.artifactPreview.enlarge')" @click="zoom = true">
        <Icon name="expand" :size="15" />
      </button>
      <button
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
        v-if="isStructuredPreview"
        type="button"
        class="chip text-[11px] hover:text-txt disabled:cursor-not-allowed disabled:opacity-45"
        data-testid="artifact-preview-download-png"
        :disabled="exportDisabled"
        @click="downloadPng"
      >
        {{ exporting ? t('pages.artifactPreview.exporting') : t('pages.artifactPreview.downloadPng') }}
      </button>
      <button
        v-if="isStructuredPreview"
        type="button"
        class="chip text-[11px] hover:text-txt disabled:cursor-not-allowed disabled:opacity-45"
        data-testid="artifact-preview-download-pdf"
        :disabled="exportDisabled"
        @click="downloadPdf"
      >
        {{ exporting ? t('pages.artifactPreview.exporting') : t('pages.artifactPreview.downloadPdf') }}
      </button>
      <button class="text-txt3 hover:text-err" :title="t('pages.artifactPreview.delete')" @click="openDeleteConfirm">
        <Icon name="trash" :size="15" />
      </button>
    </div>
    <div
      class="flex-1 overflow-hidden"
      :class="activeIsHtml && activeContent && !loading && !loadErr ? '' : 'scroll-area overflow-y-auto p-4'"
    >
      <template v-if="artifact && isImageBranch">
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
            :alt="artifact.name"
            class="max-h-full max-w-full object-contain"
            @error="handleImageLoadError"
          />
        </div>
      </template>
      <template v-else-if="artifact">
        <RefreshStrip v-if="loading && activeContent" />
        <HardLoadLayer
          v-else-if="loading && !activeContent && !loadErr"
          :overlay="false"
          :stuck-after-ms="10_000"
          :stage="t('pages.artifactPreview.loading')"
          @retry="artifact && loadContent(artifact, { force: true })"
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
            @click="artifact && loadContent(artifact, { force: true })"
          >
            {{ t('pages.artifactPreview.retry') }}
          </button>
        </div>
        <HtmlPreview v-if="previewBranch.kind === 'html' && activeContent" :html="activeContent" />
        <div
          v-else-if="previewBranch.kind === 'structured'"
          ref="structuredExportRootInline"
          data-testid="structured-artifact-export-root"
          class="structured-artifact-export-root"
        >
          <StructuredArtifactView
            :name="artifact.name"
            :doc="structuredDoc"
            :artifacts="artifacts"
            :run-id="runId || artifact.runId"
          />
        </div>
        <div
          v-else-if="previewBranch.kind === 'json' && jsonState"
          class="json-code-view scroll-area cursor-zoom-in"
          @dblclick="zoom = true"
        >
          <div v-if="!jsonState.ok" class="fallback-tag">{{ t('pages.artifactPreview.fallbackPlainText') }}</div>
          <pre v-html="jsonState.html" />
        </div>
        <div
          v-else-if="previewBranch.kind === 'markdown' && activeContent"
          class="md cursor-zoom-in"
          v-html="renderMarkdown(activeContent)"
          @dblclick="zoom = true"
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

  <AppModal :open="zoom" :title="artifact?.name" :width="960" @close="zoom = false">
    <div
      v-if="artifact && previewBranch.kind === 'structured'"
      ref="structuredExportRootZoom"
      data-testid="structured-artifact-export-root-zoom"
      class="structured-artifact-export-root"
    >
      <StructuredArtifactView
        :name="artifact.name"
        :doc="structuredDoc"
        :artifacts="artifacts"
        :run-id="runId || artifact.runId"
      />
    </div>
    <div
      v-else-if="artifact && previewBranch.kind === 'json'"
      class="json-code-view json-code-view--modal scroll-area -m-5 min-h-[280px] p-5"
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
      v-else-if="artifact && previewBranch.kind === 'image'"
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
          :alt="artifact.name"
          class="max-h-[70vh] max-w-full object-contain"
          @error="handleImageLoadError"
        />
      </div>
    </div>
    <div v-else-if="artifact" class="md mx-auto max-w-3xl" v-html="renderMarkdown(activeContent)" />
    <template #footer>
      <span class="mr-auto text-[11px] text-txt3">
        <template v-if="scope === 'platform' && artifact?.workflowName">{{ artifact.workflowName }} · </template>
        {{ artifact?.nodeId }} · {{ artifact ? fmtTime(artifact.createdAt) : '' }} · {{ t('pages.artifactPreview.platformStorage') }}
      </span>
      <button class="chip hover:text-txt" data-testid="artifact-preview-zoom-copy" @click="copyContent">
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
        v-if="isStructuredPreview"
        type="button"
        class="chip hover:text-txt disabled:cursor-not-allowed disabled:opacity-45"
        data-testid="artifact-preview-zoom-download-png"
        :disabled="exportDisabled"
        @click="downloadPng"
      >
        {{ exporting ? t('pages.artifactPreview.exporting') : t('pages.artifactPreview.downloadPng') }}
      </button>
      <button
        v-if="isStructuredPreview"
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
