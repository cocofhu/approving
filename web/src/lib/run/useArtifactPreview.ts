/**
 * Artifact preview: branch routing, version chip, load/export/delete orchestration.
 */
import { ref, computed, watch, onMounted, onBeforeUnmount } from 'vue'
import { useI18n } from 'vue-i18n'
import { isAbortError } from '@/lib/run/liveLogRehydrate'
import { useReviewAnnotate } from '@/lib/inbox/reviewAnnotate'
import type { ReactAnnotation } from '@/lib/shared/types'
import { isImagePreviewArtifact, resolveArtifactPreviewBranch } from '@/components/run/artifactPreviewBranch'
import {
  artifactFingerprint,
  isHistoricalStageArtifact,
  listVisualPageVersionChoices,
  resolveVisualPagePreviewArtifact,
  type VisualPageVersionChoice,
} from '@/lib/run/reactArtifactPreview'
import { renderMarkdown } from '@/lib/shared/markdown'
import { isJsonArtifact, parseJsonState } from '@/lib/shared/highlightJson'
import { fmtTime } from '@/lib/shared/format'
import { api } from '@/lib/api/api'
import { copyToClipboard } from '@/lib/shared/copyToClipboard'
import { useToast } from '@/lib/composables/useToast'
import { exportStructuredArtifact, type StructuredExportFormat } from '@/lib/run/exportStructuredArtifact'
import type { Artifact, Run } from '@/lib/shared/types'

export interface ArtifactPreviewProps {
  artifact: Artifact | null
  scope?: 'run' | 'platform'
  emptyHint?: string
  artifacts?: Artifact[]
  runId?: string
  run?: Run | null
  hideDelete?: boolean
  hideCopy?: boolean
  hideZoom?: boolean
  hideExport?: boolean
  annotatable?: boolean
}

export type ArtifactPreviewEmit = { (e: 'deleted', id: string): void }


export function useArtifactPreview(props: ArtifactPreviewProps, emit: ArtifactPreviewEmit) {

const { t } = useI18n()
const toast = useToast()
const annotateChannel = useReviewAnnotate()
const stageEl = ref<HTMLElement | null>(null)

const zoom = ref(false)
const loading = ref(false)
const loadErr = ref('')
const copying = ref(false)
/** Structured reserved JSON: partitioned UI vs readable raw source. Resets on artifact change. */
const structuredMode = ref<'structured' | 'raw'>('structured')
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

/** page.html version chip (g2.1): aligned with ReactArtifactStage. */
const versionMenuOpen = ref(false)
const selectedVersionIndex = ref<number | null>(null)

const versionChoices = computed(() =>
  listVisualPageVersionChoices(props.run, props.artifact),
)
const showVersionChip = computed(() => versionChoices.value.length >= 2)

const selectedChoice = computed((): VisualPageVersionChoice | null => {
  const choices = versionChoices.value
  if (!choices.length) return null
  const picked = selectedVersionIndex.value
  return choices.find((c) => c.index === picked) || choices[choices.length - 1]
})

/** Live list row vs historical snapshot resolved for preview (g2.1 / g2.2). */
const displayArtifact = computed(() => {
  if (!props.artifact) return null
  return resolveVisualPagePreviewArtifact(props.artifact, selectedChoice.value)
})

const viewingHistorical = computed(() => isHistoricalStageArtifact(displayArtifact.value))

function versionChipLabel(choice: VisualPageVersionChoice): string {
  if (choice.latest) return t('pages.reactArtifactStage.versionChipLatest', { n: choice.index })
  return t('pages.reactArtifactStage.versionChip', { n: choice.index })
}

const currentChipLabel = computed(() => {
  const sel = selectedChoice.value
  return sel ? versionChipLabel(sel) : ''
})

function selectVersion(choice: VisualPageVersionChoice) {
  if (!choice.available) return
  selectedVersionIndex.value = choice.index
  versionMenuOpen.value = false
}

function toggleVersionMenu() {
  versionMenuOpen.value = !versionMenuOpen.value
}

function onVersionMenuDocClick(e: MouseEvent) {
  if (!versionMenuOpen.value) return
  const el = e.target as HTMLElement | null
  if (el?.closest?.('[data-testid="artifact-preview-version-chip"]')) return
  versionMenuOpen.value = false
}

watch(
  () =>
    `${props.artifact?.id || ''}|${versionChoices.value.map((c) => `${c.index}:${c.available ? '1' : '0'}`).join(',')}`,
  () => {
    selectedVersionIndex.value = null
    versionMenuOpen.value = false
    const choices = versionChoices.value
    if (choices.length < 2) return
    const latest = choices[choices.length - 1]
    if (latest?.available) selectedVersionIndex.value = latest.index
  },
)

const activeContent = computed(() =>
  displayArtifact.value ? contentCache.value[displayArtifact.value.id] ?? '' : '',
)
const activeIsHtml = computed(() => displayArtifact.value?.kind === 'html')
const activeIsJson = computed(() => isJsonArtifact(displayArtifact.value))

const jsonState = computed(() => {
  if (!activeIsJson.value || !activeContent.value) return null
  return parseJsonState(activeContent.value)
})

/** Reserved structured JSON (any scope). Shared by inline preview and zoom modal. */
const previewBranch = computed(() => {
  const a = displayArtifact.value
  if (!a) return { kind: 'empty' as const }
  return resolveArtifactPreviewBranch({
    name: a.name,
    kind: a.kind,
    content: activeContent.value,
  })
})

const isStructuredPreview = computed(() => previewBranch.value.kind === 'structured')

/** Show partitioned StructuredArtifactView (not raw JSON). */
const showStructuredUi = computed(
  () => isStructuredPreview.value && structuredMode.value === 'structured',
)

/** Raw JSON pane for structured artifacts, or ordinary json branch. */
const showRawJson = computed(() => {
  if (isStructuredPreview.value && structuredMode.value === 'raw') return true
  return previewBranch.value.kind === 'json'
})

const structuredDoc = computed(() =>
  previewBranch.value.kind === 'structured' ? previewBranch.value.doc : null,
)

const isImageBranch = computed(() => previewBranch.value.kind === 'image')

/** Historical snapshots are always view-only (g2.2). */
const canAnnotate = computed(
  () => !!props.annotatable && !!annotateChannel?.enabled && !viewingHistorical.value,
)
const quoteAnnotate = computed(
  () => canAnnotate.value && !isImageBranch.value && previewBranch.value.kind !== 'html',
)
const blockZoomGesture = computed(() => props.hideZoom || canAnnotate.value)
const showDelete = computed(() => !props.hideDelete && !viewingHistorical.value)

function stageAnnotation(ann: ReactAnnotation) {
  if (!canAnnotate.value) return
  annotateChannel?.annotate(ann)
}

function onHtmlPick(payload: { selector: string; tagName: string }) {
  stageAnnotation({
    selector: payload.selector,
    label: payload.selector || payload.tagName,
  })
}

function onQuoteAdd(ann: ReactAnnotation) {
  stageAnnotation(ann)
}

/** Disable style export while content is loading, errored, or already exporting. */
const exportDisabled = computed(
  () =>
    exporting.value ||
    loading.value ||
    !!loadErr.value ||
    !isStructuredPreview.value ||
    !displayArtifact.value,
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
  if (displayArtifact.value) void loadImageDownload(displayArtifact.value)
}

function handleImageLoadError() {
  revokeImageBlob()
  imageSrc.value = null
  imageDownloadError.value = true
}

async function loadContent(a: Artifact, opts?: { force?: boolean }) {
  if (!opts?.force && contentCache.value[a.id] !== undefined) return
  if (typeof a.content === 'string') {
    contentCache.value[a.id] = a.content
    loading.value = false
    loadErr.value = ''
    return
  }
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
  const a = displayArtifact.value
  if (!a) return
  // Historical snapshots only exist in-memory; download from cached content (g2.2 read-only).
  if (isHistoricalStageArtifact(a) && typeof contentCache.value[a.id] === 'string') {
    const blob = new Blob([contentCache.value[a.id]], { type: 'text/html;charset=utf-8' })
    const url = URL.createObjectURL(blob)
    const link = document.createElement('a')
    link.href = url
    link.download = a.name.replace(/#iter-\d+$/, '') || 'page.html'
    link.click()
    URL.revokeObjectURL(url)
    return
  }
  window.open(api.artifactDownloadUrl(a.id), '_blank')
}

function resolveExportRoot(): HTMLElement | null {
  if (zoom.value && structuredExportRootZoom.value) return structuredExportRootZoom.value
  return structuredExportRootInline.value
}

async function runStructuredExport(format: StructuredExportFormat) {
  if (exportDisabled.value || !displayArtifact.value) return
  const root = resolveExportRoot()
  if (!root) {
    toast.error(t('pages.artifactPreview.exportFailed'))
    return
  }
  exporting.value = true
  try {
    const result = await exportStructuredArtifact(root, displayArtifact.value.name, format)
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
  if (viewingHistorical.value) return
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
  // Always delete the live list artifact, never a synthetic historical id (g2.2).
  const a = props.artifact
  if (!a || deleting.value || viewingHistorical.value) return
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
  () => {
    const a = displayArtifact.value
    if (!a) return ''
    return artifactFingerprint(a)
  },
  (fp, prev) => {
    const id = displayArtifact.value?.id
    const sameId = !!prev && !!fp && prev.split(':')[0] === fp.split(':')[0]
    if (!sameId) {
      // Do not remember raw across products — always default structured UI.
      structuredMode.value = 'structured'
      resetImageDownloadState()
    }
    if (displayArtifact.value) {
      if (sameId) delete contentCache.value[displayArtifact.value.id]
      void loadContent(displayArtifact.value, { force: sameId })
      if (isImagePreviewArtifact(displayArtifact.value.name, displayArtifact.value.kind)) {
        void loadImageDownload(displayArtifact.value)
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

onMounted(() => document.addEventListener('click', onVersionMenuDocClick))
onBeforeUnmount(() => {
  contentLoadAbort?.abort()
  contentLoadAbort = null
  contentLoadGen++
  imageLoadGen++
  revokeImageBlob()
  document.removeEventListener('click', onVersionMenuDocClick)
})

  return {
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
  }
}
