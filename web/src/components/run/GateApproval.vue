<script lang="ts">
export type ActionIconName = 'check' | 'arrow-left' | 'refresh'
export type ActionVariant = 'ok' | 'neutral'

const POSITIVE_ACTION_IDS = new Set(['pass', 'approve'])
const REVERT_ACTION_IDS = new Set(['fail', 'revise', 'limit'])

export function actionIcon(id: string): ActionIconName {
  if (POSITIVE_ACTION_IDS.has(id)) return 'check'
  if (REVERT_ACTION_IDS.has(id)) return 'arrow-left'
  return 'refresh'
}

export function actionVariant(id: string): ActionVariant {
  return POSITIVE_ACTION_IDS.has(id) ? 'ok' : 'neutral'
}

export function actionVariantClasses(variant: ActionVariant): string {
  if (variant === 'ok') return 'bg-ok/15 text-ok hover:bg-ok/25'
  return 'border border-line text-txt2 hover:bg-elevated hover:text-txt'
}
</script>

<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '../ui/Icon.vue'
import ParagraphInput from '../ui/ParagraphInput.vue'
import { renderMarkdown } from '@/lib/markdown'
import { api, type PreviewIssue } from '@/lib/api'
import { useToast } from '@/lib/useToast'
import { isCompositeFilled, normalizeCompositeSubmit } from '@/lib/compositeText'
import { useBreakpoint } from '@/lib/useBreakpoint'
import { provideReviewAnnotate } from '@/lib/reviewAnnotate'
import { pushAnnotationUnique } from '@/lib/reviewQuote'
import PlanView, { type PlanDoc } from './PlanView.vue'
import ProposalSelectView, { type ProposalsDoc } from './ProposalSelectView.vue'
import StructuredArtifactView, { isStructuredArtifactName } from './StructuredArtifactView.vue'
import SelectionAddToChat from './SelectionAddToChat.vue'
import HtmlPreview from '../ui/HtmlPreview.vue'
import AppPreviewPanel from './AppPreviewPanel.vue'
import PreviewFeedbackChat from './PreviewFeedbackChat.vue'
import ArtifactLoadingPane from './ArtifactLoadingPane.vue'
import UpstreamRequirementContext from './UpstreamRequirementContext.vue'
import ReviewShell from './ReviewShell.vue'
import ReviewComposer from './ReviewComposer.vue'
import GateReactStreamPanel from './GateReactStreamPanel.vue'
import { REVIEW_SHELL_WIDTH_KEY_APPROVAL } from '@/lib/reviewLayoutBudget'
import { OUTPUT_KEY_TO_ARTIFACT } from '@/lib/structuredArtifacts'
import {
  CONTENT_FIT_PREVIEW_MAX_VH,
  CONTENT_FIT_REVIEWING_STRIP_PX,
  dataUrlToImageParts,
} from '@/lib/htmlPreviewSandbox'
import {
  listPrimaryProducts,
  listExcludedProduces,
  pickProductRef,
  resolveUpstreamOutputs,
  reviewingUpstreamN,
  inferArtifactKind,
  isReadonlyArtifactKind,
  type GatePrimaryProductRef,
} from '@/lib/gateUpstream'
import GateProductEditor from './GateProductEditor.vue'
import type { ClarifyImage, Gate, Run, ReactAnnotation } from '@/lib/types'

const props = defineProps<{
  gate: Gate
  run?: Run
  compact?: boolean
  submitError?: string | null
  /**
   * When true (GatesInboxView / mobile Run detail), the approval panel fills
   * the card. Visual HTML and structured artifacts use a content-fit shell:
   * preview capped at 60vh with overflow scroll, form/actions shrink-0 pinned
   * outside the preview scroll region (not a single scroll column).
   * app_preview still fills remaining height.
   */
  fillPreview?: boolean
  /**
   * Run-detail only: on mobile visual gates, use ReviewShell drawer (stage
   * fills remaining height above the composable sidebar). Inbox must omit.
   */
  mobileFillRemaining?: boolean
}>()
const emit = defineEmits<{
  (e: 'resolve', action: string, form: Record<string, any>): void
  (e: 'react-revised'): void
}>()

const { t } = useI18n()
const { isMobile } = useBreakpoint()
const toast = useToast()

const formText = ref<Record<string, string>>({})
const formImages = ref<Record<string, ClarifyImage[]>>({})
const resolved = ref<string | null>(null)
const formError = ref<string | null>(null)
const productEditorRef = ref<{ isDirty: { value: boolean }; discard: () => void } | null>(null)
const productDirty = ref(false)
const savedProductContent = ref<Record<string, string>>({})
const savedProductMeta = ref<Record<string, { etag?: string; updatedAt?: string; sizeBytes?: number }>>({})

function formSchemaKey(fields: Gate['form']): string {
  return (fields || []).map((f) => `${f.key}:${f.label}:${f.required}`).join('|')
}

watch(
  () => formSchemaKey(props.gate.form),
  (key, prev) => {
    if (prev !== undefined && key === prev) return
    const text: Record<string, string> = {}
    const imgs: Record<string, ClarifyImage[]> = {}
    for (const f of props.gate.form || []) {
      text[f.key] = ''
      imgs[f.key] = []
    }
    formText.value = text
    formImages.value = imgs
  },
  { immediate: true },
)

// A submission rejected by the backend (parent passes the error down): clear the
// optimistic "已提交" lock and show the reason so the reviewer can retry.
watch(
  () => props.submitError,
  (e) => {
    if (e) {
      resolved.value = null
      formError.value = e
    }
  },
)

// A proposal_select gate presents candidate proposals as a card picker (parsed
// from the upstream proposals artifact) instead of a markdown wall + generic
// buttons. Detected via the gate's originating node type; defensively also when
// the gate's actions look like proposal ids (p1, p2, …) so it still engages if
// the run's pinned graph / node config differs.
const gateNode = computed(() => props.run?.nodes?.find((n) => n.id === props.gate.nodeId))
const looksLikeProposalActions = computed(
  () => !!props.gate.actions?.length && props.gate.actions.every((a) => /^p\d+$/.test(a.id)),
)
const isProposalSelect = computed(
  () => gateNode.value?.type === 'proposal_select' || looksLikeProposalActions.value,
)
const isAppPreview = computed(() => gateNode.value?.type === 'app_preview')

const PASS_ACTION_IDS = new Set(['pass', 'approve'])
const FAIL_ACTION_IDS = new Set(['fail', 'revise'])

const previewIssues = ref<PreviewIssue[]>([])
const previewIssuesLoading = ref(false)
const previewIssuesError = ref<string | null>(null)
const pickedSelector = ref('')
const pickedElementImage = ref<ClarifyImage | null>(null)
/** Staged ReAct annotations (jsonPath / selector / quote chips) for hot revise. */
const reactAnnotations = ref<ReactAnnotation[]>([])
const gateStageEl = ref<HTMLElement | null>(null)

function pushReactAnnotation(ann: ReactAnnotation) {
  const result = pushAnnotationUnique(reactAnnotations.value, ann)
  if (result === 'duplicate') toast.warn(t('pages.reviewComposer.alreadyAdded'))
}

const annotateEnabled = computed(
  () => !!props.gate.reactSessionAlive && resolved.value == null,
)

// StructuredArtifactView AnnotateBtn → side-panel chips (only when hot session alive).
provideReviewAnnotate({
  get enabled() {
    return annotateEnabled.value
  },
  annotate: (ann) => {
    if (!annotateEnabled.value) return
    pushReactAnnotation(ann)
  },
})

function onQuoteAdd(ann: ReactAnnotation) {
  if (!annotateEnabled.value) return
  pushReactAnnotation(ann)
}

function onHtmlPreviewPick(payload: { selector: string; tagName: string; imageDataUrl: string }) {
  pickedSelector.value = payload.selector
  const parts = dataUrlToImageParts(payload.imageDataUrl)
  pickedElementImage.value = parts
  if (!parts) {
    toast.error(t('pages.appPreview.feedback.requireScreenshot'))
  }
  // Dual-channel: PreviewIssue path keeps selector/screenshot; ReAct annotations
  // get the same selector chip for hot gateReactRevise.
  if (canReactRevise.value) {
    pushReactAnnotation({
      selector: payload.selector,
      label: payload.selector || payload.tagName,
    })
  }
}

/** VNC/app_preview pick payload uses outerHTML (no imageDataUrl). */
function onAppPreviewPick(payload: { selector: string; tagName: string; outerHTML: string }) {
  pickedSelector.value = payload.selector
  pickedElementImage.value = null
  if (canReactRevise.value) {
    pushReactAnnotation({
      selector: payload.selector,
      label: payload.selector || payload.tagName,
    })
  }
}

function clearHtmlPreviewPick() {
  pickedSelector.value = ''
  pickedElementImage.value = null
}
const proposalsDoc = ref<ProposalsDoc | null>(null)
const proposalsLoading = ref(false)
async function loadProposals() {
  if (!isProposalSelect.value) {
    proposalsDoc.value = null
    return
  }
  const from = (gateNode.value?.config?.from || 'proposals.json').toString()
  // Prefer the configured source; fall back to the conventional proposals.json.
  const a =
    props.run?.artifacts.find((x) => x.name === from) ||
    props.run?.artifacts.find((x) => x.name === 'proposals.json')
  if (!a) {
    proposalsDoc.value = null
    return
  }
  proposalsLoading.value = true
  try {
    const full = await api.artifactContent(a.id)
    const doc = JSON.parse(full.content || '{}') as ProposalsDoc
    proposalsDoc.value = Array.isArray(doc.proposals) && doc.proposals.length ? doc : null
  } catch {
    proposalsDoc.value = null
  } finally {
    proposalsLoading.value = false
  }
}
watch(
  () => [props.gate.nodeId, isProposalSelect.value, props.run?.artifacts.map((x) => x.name).join(',')],
  loadProposals,
  { immediate: true },
)

// A plan gate resolves its body to the plan checklist markdown (see
// RenderPlanMarkdown); rendering that raw is ugly, so when we detect it we swap
// in the structured PlanView fed from the run's plan.json artifact.
const isPlanBody = computed(() => /- \[[ x]\] `g\d/.test(props.gate.bodyMd || ''))
const planDoc = ref<PlanDoc | null>(null)
const planLoading = ref(false)
async function loadPlan() {
  const a = props.run?.artifacts.find((x) => x.name === 'plan.json')
  if (!isPlanBody.value || !a) {
    planDoc.value = null
    return
  }
  planLoading.value = true
  try {
    const full = await api.artifactContent(a.id)
    const doc = JSON.parse(full.content || '{}') as PlanDoc
    planDoc.value = Array.isArray(doc.goals) ? doc : null
  } catch {
    planDoc.value = null
  } finally {
    planLoading.value = false
  }
}
watch(() => [props.gate.bodyMd, props.run?.artifacts.find((x) => x.name === 'plan.json')?.id], loadPlan, { immediate: true })

// Generalized structured body: when the gate's configured 展示内容 references an
// upstream node's structured product ({{nodes.<id>.outputs.<key>}}), render it
// with the SAME structured views as the node's 产物 tab — not raw markdown.
const bodyTemplate = computed(() => (gateNode.value?.config?.body_template || '').toString())
/** Client-side parse of body_template; used offline / when API is unavailable. */
const clientPrimaryProducts = computed<GatePrimaryProductRef[]>(() =>
  listPrimaryProducts(bodyTemplate.value, {
    isProposalSelect: isProposalSelect.value,
    proposalSelectFrom: (gateNode.value?.config?.from || 'proposals.json').toString(),
  }),
)
/**
 * Server ListGatePrimaryProducts is the SSOT for kind/readonly (store Kind may
 * mark image even when the filename has no image suffix). null = use client fallback.
 */
const apiPrimaryProducts = ref<GatePrimaryProductRef[] | null>(null)
/** True after listGatePrimaryArtifacts settles (success/offline); gates first loadProduct. */
const primaryProductsHydrated = ref(false)

function normalizeApiPrimaryItem(item: {
  name: string
  kind: string
  readonly?: boolean
  nodeId?: string
  outputKey?: string
}): GatePrimaryProductRef {
  const kind = (item.kind || inferArtifactKind(item.name)) as GatePrimaryProductRef['kind']
  return {
    name: item.name,
    kind,
    readonly: item.readonly ?? isReadonlyArtifactKind(kind),
    nodeId: item.nodeId,
    outputKey: item.outputKey,
  }
}

async function loadPrimaryProductsFromApi() {
  // Server ListGatePrimaryArtifacts requires waiting_human; skip silently otherwise
  // so resume→running (DTO may still carry gate) does not spam console 400s.
  if (
    !props.run?.id ||
    !props.gate.nodeId ||
    resolved.value != null ||
    props.run.status !== 'waiting_human'
  ) {
    apiPrimaryProducts.value = null
    primaryProductsHydrated.value = true
    return
  }
  primaryProductsHydrated.value = false
  try {
    const r = await api.listGatePrimaryArtifacts(props.run.id, props.gate.nodeId)
    const items = r.items || []
    apiPrimaryProducts.value = items.length
      ? items.map(normalizeApiPrimaryItem)
      : null
  } catch {
    // Offline / older server: keep client listPrimaryProducts as fallback.
    apiPrimaryProducts.value = null
  } finally {
    primaryProductsHydrated.value = true
  }
}

watch(
  () =>
    [
      props.run?.id,
      props.run?.status,
      props.gate.nodeId,
      resolved.value,
      bodyTemplate.value,
    ] as const,
  () => {
    void loadPrimaryProductsFromApi()
  },
  { immediate: true },
)

const primaryProducts = computed<GatePrimaryProductRef[]>(
  () => apiPrimaryProducts.value ?? clientPrimaryProducts.value,
)
const excludedProduces = computed(() =>
  listExcludedProduces(bodyTemplate.value, props.run?.nodes, {
    isProposalSelect: isProposalSelect.value,
    proposalSelectFrom: (gateNode.value?.config?.from || 'proposals.json').toString(),
  }),
)
// page/page.html preferred over the first template ref (aligned with backend pointer).
const productRef = computed<{ nodeId: string; key: string } | null>(() =>
  pickProductRef(bodyTemplate.value),
)
const productName = computed<string | null>(() => {
  const key = productRef.value?.key
  if (key && OUTPUT_KEY_TO_ARTIFACT[key]) return OUTPUT_KEY_TO_ARTIFACT[key]
  return primaryProducts.value[0]?.name || null
})
/** Editable only while gate pending (not yet submitted) and products exist. */
const canEditProducts = computed(
  () => resolved.value == null && primaryProducts.value.length > 0 && !!props.run?.id,
)
// The visual node's product is a raw HTML page: preview it in an iframe instead
// of parsing it as a structured JSON doc.
const isVisualBody = computed(() => productName.value === 'page.html')
/** Paragraph quote float: structured products only (HTML keeps pick-to-annotate). */
const selectionQuoteEnabled = computed(
  () =>
    annotateEnabled.value &&
    !!productName.value &&
    isStructuredArtifactName(productName.value) &&
    !isVisualBody.value,
)
/** page.html HtmlPreview path shares PreviewIssue + Pass/Fail-by-count with app_preview. */
const usesPreviewIssues = computed(() => isAppPreview.value || isVisualBody.value)

async function loadPreviewIssues() {
  if (!usesPreviewIssues.value || !props.run?.id) {
    previewIssues.value = []
    previewIssuesLoading.value = false
    previewIssuesError.value = null
    return
  }
  previewIssuesLoading.value = true
  previewIssuesError.value = null
  try {
    const r = await api.listPreviewIssues(props.run.id, props.gate.nodeId)
    previewIssues.value = r.issues || []
  } catch {
    previewIssues.value = []
    previewIssuesError.value = 'load failed'
    toast.error(t('pages.gateApproval.loadPreviewIssuesFailed'))
  } finally {
    previewIssuesLoading.value = false
  }
}

watch(
  () => [props.run?.id, props.gate.nodeId, usesPreviewIssues.value] as const,
  loadPreviewIssues,
  { immediate: true },
)

/** Open PreviewIssue count for the current gate node (drafts / resolved excluded). */
const openPreviewIssueCount = computed(
  () => previewIssues.value.filter((i) => i.status === 'open').length,
)

/**
 * Standard review UI only exposes the positive exit (approve/pass → 确认并流转).
 * Config-layer fail/revise actions remain on the gate DTO for silent edge compat
 * but are never rendered as dual buttons.
 */
const visibleActions = computed(() => {
  const positive = props.gate.actions.filter((a) => PASS_ACTION_IDS.has(a.id))
  return positive.map((a) => ({
    ...a,
    label: t('pages.clarify.confirmFlow'),
  }))
})

const footerActions = computed(() => visibleActions.value)

/** True when the gate exposes at least one form field (comment channel). */
const hasFormFields = computed(() => (props.gate.form?.length ?? 0) > 0)

/**
 * Preview path: hide gate.form when PreviewFeedbackChat is the sole input.
 * Also hide when hot ReAct composer is active — form text overlaps reject draft (v3).
 */
const shouldHideGateForm = computed(
  () =>
    (usesPreviewIssues.value && hasFormFields.value) ||
    (!!props.gate.reactSessionAlive && resolved.value == null && !!props.run?.id),
)

/** Action buttons are not disabled by hidden form/issue state; validate() handles form rules. */
function isActionDisabled(_actionId: string): boolean {
  return false
}

function actionButtonTitle(_actionId: string): string {
  return ''
}

/** Help line when n_open===0 (PreviewIssues path) or non-preview without form. */
const helpReviseDetailNoIssuesText = computed(() =>
  usesPreviewIssues.value || !hasFormFields.value
    ? t('pages.gateApproval.helpReviseDetailNoIssuesNoForm')
    : t('pages.gateApproval.helpReviseDetailNoIssues'),
)

/** Hot send label (review semantics — no「打回」verb). */
const composerRejectLabel = computed(() => t('pages.reviewComposer.send'))

/**
 * Shared hot-path visibility (cold footer uses positive-only visibleActions).
 * Review semantics: always offer send + confirm when hot; open issues disable confirm.
 * send visibility uses reactSessionAlive directly (canReactRevise is defined later).
 */
const showHotPass = computed(() => props.gate.actions.some((a) => PASS_ACTION_IDS.has(a.id)))
const showHotReject = computed(
  () => !!props.gate.reactSessionAlive && resolved.value == null && !!props.run?.id,
)
/** PreviewIssues with open issues: send without extra composer draft. */
const hotRejectAllowEmpty = computed(
  () => usesPreviewIssues.value && openPreviewIssueCount.value >= 1,
)
/** Cold session: send unavailable — show note, confirm only. */
const isColdSession = computed(
  () => props.gate.reactSessionAlive === false && resolved.value == null,
)

const useFillLayout = computed(() => !!props.fillPreview)
/** Visual + fillPreview: content-fit shell (preview capped, form pinned). */
const shouldFillPreview = computed(
  () => useFillLayout.value && isVisualBody.value && !!productHtml.value,
)
/**
 * Run-detail mobile visual: remaining-height shell (not Inbox / desktop /
 * structured). Active for visual body even while loading/editing.
 */
const useMobileFillRemaining = computed(
  () =>
    !!props.mobileFillRemaining &&
    isMobile.value &&
    useFillLayout.value &&
    isVisualBody.value &&
    !isAppPreview.value &&
    !isProposalSelect.value,
)
const shouldFillAppPreview = computed(
  () => useFillLayout.value && isAppPreview.value && !isMobile.value,
)
const productDoc = ref<any>(null)
const productHtml = ref('')
const productLoading = ref(false)
/** Distinguishes load failure from empty body; null when last load succeeded. */
const productLoadError = ref<string | null>(null)
/**
 * Structured artifact + fillPreview: same content-fit column as visual.
 * Requires a parsed productDoc; loading / parse failure stay on the v-else path.
 * Exclude proposal_select — interactive ProposalSelectView must stay on the
 * default path (p1/p2 select); ReviewShell would swap in readonly StructuredArtifactView.
 */
const shouldFitStructured = computed(
  () =>
    useFillLayout.value &&
    !isProposalSelect.value &&
    !!productName.value &&
    isStructuredArtifactName(productName.value) &&
    !!productDoc.value &&
    !productLoading.value,
)
/** Shared enter condition for the content-fit shell (visual or structured). */
const shouldContentFit = computed(() => shouldFillPreview.value || shouldFitStructured.value)
const selectedUpstreamIteration = ref<number | null>(null)
/** True when pointer was set but snapshot missed and preview came from run artifact store. */
const previewFromArtifactFallback = ref(false)

const reviewingIteration = computed(() =>
  reviewingUpstreamN({
    upstreamIteration: props.gate.upstreamIteration,
    selectedIteration: selectedUpstreamIteration.value,
  }),
)
/** Reserve reviewing-upstream strip so HtmlPreview clamp stays within 60vh shell. */
const contentFitChromeOffsetPx = computed(() =>
  reviewingIteration.value != null ? CONTENT_FIT_REVIEWING_STRIP_PX : 0,
)

/** Narrow execution fingerprint to upstream product nodes + gate (poll anti-noise). */
function upstreamExecutionsFingerprint(
  execs: Record<string, { iteration?: number; status?: string }[] | undefined> | undefined,
): string {
  if (!execs) return ''
  const ids = new Set<string>()
  for (const p of primaryProducts.value) {
    if (p.nodeId) ids.add(p.nodeId)
  }
  if (props.gate.upstreamNodeId) ids.add(props.gate.upstreamNodeId)
  if (props.gate.nodeId) ids.add(props.gate.nodeId)
  return [...ids]
    .sort()
    .map((id) => {
      const rows = execs[id] || []
      return `${id}:${rows.map((e) => `${e.iteration ?? 0}:${e.status || ''}`).join(',')}`
    })
    .join('|')
}

function hashContentFingerprint(text: string): string {
  let hash = 0
  for (let i = 0; i < text.length; i++) {
    hash = (Math.imul(31, hash) + text.charCodeAt(i)) | 0
  }
  return `${text.length}-${hash}`
}

function buildProductLoadFingerprint(): string {
  const products = primaryProducts.value
  const pending = resolved.value == null
  const parts: string[] = [
    products.map((p) => p.name).join('|'),
    String(props.gate.iteration ?? ''),
    props.gate.upstreamNodeId ?? '',
    String(props.gate.upstreamIteration ?? ''),
    props.gate.nodeId ?? '',
    String(resolved.value ?? ''),
    pending ? 'live' : 'snap',
  ]
  for (const p of products) {
    const a = props.run?.artifacts.find((x) => x.name === p.name)
    // Resolved/history: ignore sizeBytes poll noise — content changes are
    // detected via snap hash. Pending/live: include sizeBytes (not updatedAt)
    // so a store-only write still forces a reload; etag is usually empty on the
    // list DTO poll path. Pure updatedAt bumps from Artifact.Save must not reload.
    parts.push(`${p.name}:${a?.etag ?? ''}`)
    // Pending/live: sizeBytes alone (not updatedAt) — Artifact.Save bumps UpdatedAt on
    // every upsert, so timestamp noise would force page.html reload → srcdoc flicker.
    // Content/snap hash below still catch same-size rewrites after fetch.
    if (pending && a) {
      parts.push(`meta:${a.sizeBytes ?? 0}`)
    }
    if (p.name === 'page.html' || p.outputKey === 'page') {
      const ref =
        p.nodeId && p.outputKey ? { nodeId: p.nodeId, key: p.outputKey } : productRef.value
      if (ref && props.run) {
        const result = resolveUpstreamOutputs({
          productNodeId: ref.nodeId,
          execsByNode: props.run.nodeExecutions || {},
          upstreamNodeId: props.gate.upstreamNodeId,
          upstreamIteration: props.gate.upstreamIteration,
          gateIteration: props.gate.iteration,
          pending,
        })
        const snap = result.outputs?.page
        if (typeof snap === 'string') parts.push(`snap:${hashContentFingerprint(snap)}`)
      }
    } else if (p.outputKey) {
      const ref =
        p.nodeId && p.outputKey ? { nodeId: p.nodeId, key: p.outputKey } : productRef.value
      if (ref && props.run) {
        const result = resolveUpstreamOutputs({
          productNodeId: ref.nodeId,
          execsByNode: props.run.nodeExecutions || {},
          upstreamNodeId: props.gate.upstreamNodeId,
          upstreamIteration: props.gate.upstreamIteration,
          gateIteration: props.gate.iteration,
          pending,
        })
        const snap = result.outputs?.[`${p.outputKey}_json`]
        if (typeof snap === 'string') parts.push(`snap:${hashContentFingerprint(snap)}`)
      }
    }
  }
  parts.push(upstreamExecutionsFingerprint(props.run?.nodeExecutions))
  return parts.join('::')
}

const lastProductLoadFingerprint = ref<string | null>(null)

// Upstream outputs for this gate: pointer → exact exec; miss → artifact store;
// legacy (no pointer) → max(completed) while pending, else ≤ gate.iteration.
function upstreamOutputs(): { outputs: Record<string, any> | null; pointerMiss: boolean } {
  const ref = productRef.value
  if (!ref || !props.run) {
    selectedUpstreamIteration.value = null
    return { outputs: null, pointerMiss: false }
  }
  const result = resolveUpstreamOutputs({
    productNodeId: ref.nodeId,
    execsByNode: props.run.nodeExecutions || {},
    upstreamNodeId: props.gate.upstreamNodeId,
    upstreamIteration: props.gate.upstreamIteration,
    gateIteration: props.gate.iteration,
    pending: resolved.value == null,
  })
  selectedUpstreamIteration.value = result.selectedIteration
  if (result.pointerMiss) return { outputs: null, pointerMiss: true }
  return { outputs: result.outputs, pointerMiss: false }
}

async function loadOneProductContent(
  p: GatePrimaryProductRef,
  opts?: { preferSnapshot?: boolean },
): Promise<string> {
  let snapContent = ''
  const pending = resolved.value == null
  const ref = p.nodeId && p.outputKey ? { nodeId: p.nodeId, key: p.outputKey } : productRef.value
  if (ref && props.run) {
    const result = resolveUpstreamOutputs({
      productNodeId: ref.nodeId,
      execsByNode: props.run.nodeExecutions || {},
      upstreamNodeId: props.gate.upstreamNodeId,
      upstreamIteration: props.gate.upstreamIteration,
      gateIteration: props.gate.iteration,
      pending,
    })
    selectedUpstreamIteration.value = result.selectedIteration
    if (!result.pointerMiss && result.outputs) {
      if (p.name === 'page.html' || p.outputKey === 'page') {
        const snap = result.outputs.page
        if (typeof snap === 'string' && snap.trim()) snapContent = snap
      } else if (p.outputKey) {
        const snap = result.outputs[`${p.outputKey}_json`]
        if (typeof snap === 'string' && snap.trim()) snapContent = snap
      }
    }
    if (result.pointerMiss) previewFromArtifactFallback.value = true
  }
  // Pending/live gates follow the artifact store; history/resolved keep snapshots.
  if (opts?.preferSnapshot && snapContent) return snapContent
  const a = props.run?.artifacts.find((x) => x.name === p.name)
  if (!a) return snapContent
  const full = await api.artifactContent(a.id)
  const storeContent = full.content || ''
  // Prefer store ETag on first load so subsequent saves send If-Match by default.
  savedProductMeta.value = {
    ...savedProductMeta.value,
    [p.name]: {
      etag: full.etag,
      updatedAt: full.updatedAt || a.createdAt,
      sizeBytes: full.sizeBytes ?? a.sizeBytes,
    },
  }
  if (pending) return storeContent || snapContent
  // Snapshot is the gate-bound preview source when present; otherwise store content.
  return snapContent || storeContent
}

async function loadProduct(opts?: { force?: boolean }) {
  previewFromArtifactFallback.value = false
  productLoadError.value = null
  const products = primaryProducts.value
  if (!products.length) {
    productDoc.value = null
    productHtml.value = ''
    selectedUpstreamIteration.value = null
    savedProductContent.value = {}
    lastProductLoadFingerprint.value = null
    return
  }

  const fingerprint = buildProductLoadFingerprint()
  const hasContent = products.some((p) => (savedProductContent.value[p.name] ?? '').trim())

  if (!opts?.force && fingerprint === lastProductLoadFingerprint.value && hasContent) {
    return
  }

  const isInitialLoad = !hasContent
  const fingerprintChanged = fingerprint !== lastProductLoadFingerprint.value
  const pending = resolved.value == null
  const showLoading = opts?.force || isInitialLoad

  if (showLoading) productLoading.value = true
  try {
    const next: Record<string, string> = { ...savedProductContent.value }
    const errors: string[] = []
    for (const p of products) {
      try {
        next[p.name] = await loadOneProductContent(p, {
          // Pending follows live store; only resolved/history prefer frozen snap.
          preferSnapshot: !pending && fingerprintChanged && !isInitialLoad,
        })
      } catch (e: any) {
        // Keep prior content if any; do not collapse failure into empty string.
        const msg = e?.message || String(e)
        errors.push(`${p.name}: ${msg}`)
      }
    }
    if (errors.length) {
      productLoadError.value = errors.join('; ')
    } else {
      lastProductLoadFingerprint.value = fingerprint
    }
    savedProductContent.value = next
    // Keep legacy single-product fields for read-only / content-fit paths.
    const primary = products[0]
    const text = next[primary.name] || ''
    if (primary.name === 'page.html') {
      // Avoid HtmlPreview srcdoc reassignment when body is unchanged (no iframe flicker).
      if (productHtml.value !== text) productHtml.value = text
      productDoc.value = null
    } else if (isStructuredArtifactName(primary.name)) {
      try {
        productDoc.value = JSON.parse(text || '{}')
      } catch {
        productDoc.value = null
      }
      productHtml.value = ''
    } else {
      productDoc.value = null
      productHtml.value = ''
    }
  } finally {
    productLoading.value = false
  }
}

function retryLoadProduct() {
  productLoadError.value = null
  void loadProduct({ force: true })
}
watch(
  () =>
    [
      buildProductLoadFingerprint(),
      resolved.value,
      primaryProductsHydrated.value,
    ] as const,
  ([, , hydrated]) => {
    if (!hydrated) return
    void loadProduct()
  },
  { immediate: true },
)

function onProductSaved(payload: {
  name: string
  content: string
  etag: string
  updatedAt: string
  sizeBytes: number
}) {
  savedProductContent.value = { ...savedProductContent.value, [payload.name]: payload.content }
  savedProductMeta.value = {
    ...savedProductMeta.value,
    [payload.name]: {
      etag: payload.etag,
      updatedAt: payload.updatedAt,
      sizeBytes: payload.sizeBytes,
    },
  }
  if (payload.name === 'page.html') {
    productHtml.value = payload.content
  } else if (isStructuredArtifactName(payload.name)) {
    try {
      productDoc.value = JSON.parse(payload.content || '{}')
    } catch {
      /* keep */
    }
  }
  // Refresh proposals picker if that artifact was edited.
  if (payload.name === 'proposals.json' || payload.name.endsWith('proposals.json')) {
    loadProposals()
  }
}

async function onProductRefresh(name: string) {
  const p = primaryProducts.value.find((x) => x.name === name)
  if (!p) return
  productLoadError.value = null
  // Keep GateProductEditor mounted: toggling productLoading would tear down
  // draft / mode / external-change UI in the content-fit shell.
  try {
    const content = await loadOneProductContent(p, { preferSnapshot: true })
    savedProductContent.value = { ...savedProductContent.value, [name]: content }
    if (name === 'page.html') {
      productHtml.value = content
    } else if (isStructuredArtifactName(name)) {
      try {
        productDoc.value = JSON.parse(content || '{}')
      } catch {
        /* keep prior parse */
      }
    }
    lastProductLoadFingerprint.value = buildProductLoadFingerprint()
  } catch (e: any) {
    const msg = e?.message || String(e)
    productLoadError.value = `${name}: ${msg}`
    toast.error(t('pages.gateApproval.loadPreviewIssuesFailed'))
  }
}

function buildFormPayload(): Record<string, any> {
  const out: Record<string, any> = {}
  for (const f of props.gate.form || []) {
    out[f.key] = normalizeCompositeSubmit(formText.value[f.key] ?? '', formImages.value[f.key] ?? [])
  }
  return out
}

// A form field must be filled when it is itself required, or when the chosen
// action mandates the form (e.g. a "reject" action requiring a review comment).
// F5: with ≥1 PreviewIssue, skip requireForm on fail/revise so empty comment is OK.
function validate(id: string): string | null {
  const action = props.gate.actions.find((a) => a.id === id)
  let requireAll = !!action?.requireForm
  if (
    requireAll &&
    usesPreviewIssues.value &&
    (FAIL_ACTION_IDS.has(id) || PASS_ACTION_IDS.has(id))
  ) {
    requireAll = false
  }
  for (const f of props.gate.form || []) {
    if (!f.required && !requireAll) continue
    if (!isCompositeFilled({ text: formText.value[f.key] ?? '', images: formImages.value[f.key] ?? [] })) {
      return t('pages.gateApproval.submitMissing', { label: f.label || f.key })
    }
  }
  return null
}

const hasUnsentReactDraft = computed(
  () =>
    reactText.value.trim().length > 0 ||
    reactImages.value.length > 0 ||
    reactAnnotations.value.length > 0 ||
    !!pickedElementImage.value?.data ||
    !!pickedSelector.value.trim(),
)

function clearUnifiedDraft() {
  reactText.value = ''
  reactImages.value = []
  reactAnnotations.value = []
  clearHtmlPreviewPick()
  hotRejectHistorySynced.value = false
}

function choose(id: string) {
  const err = validate(id)
  if (err) {
    formError.value = err
    return
  }
  if (productDirty.value) {
    const label = props.gate.actions.find((a) => a.id === id)?.label || id
    const isRevise = REVERT_ACTION_IDS.has(id)
    const ok = window.confirm(
      isRevise
        ? t('pages.gateApproval.discardConfirmRevise', { label })
        : t('pages.gateApproval.discardConfirmApprove', { label }),
    )
    if (!ok) return
    productEditorRef.value?.discard()
    productDirty.value = false
  }
  // Pass/approve: silently discard unsent draft (no confirm).
  if (POSITIVE_ACTION_IDS.has(id) && hasUnsentReactDraft.value) {
    clearUnifiedDraft()
  }
  formError.value = null
  resolved.value = id
  emit('resolve', id, buildFormPayload())
}

const isEditing = computed(() => {
  if (productDirty.value) return true
  if (hasUnsentReactDraft.value) return true
  for (const f of props.gate.form || []) {
    if ((formText.value[f.key] ?? '').trim().length > 0) return true
    if ((formImages.value[f.key] ?? []).length > 0) return true
  }
  return false
})

// --- ReAct 打回修改 (in-place edit of the upstream producer's live session) ---
// Only offered when the DTO reports the upstream producer's review session is
// still alive; otherwise the reviewer uses the normal approve/reject buttons
// (whose reject follows the failure edge with a cold restart).
const canReactRevise = computed(() => !!props.gate.reactSessionAlive && resolved.value == null && !!props.run?.id)
const reactText = ref('')
const reactImages = ref<ClarifyImage[]>([])
const reactSending = ref(false)
/** Sandbox-aligned: pending-send queue + in-flight turn (HTTP returns on enqueue). */
const reactQueued = ref<{ text: string }[]>([])
const reactThinking = ref(false)
/** True between turn_begin and turn_done/error (mirrors ClarifyChat liveAgentIdx). */
const reactInFlight = ref(false)
const reactStreamText = ref('')
/** ACP thought rail (separate from message — Demo: thought must not be dropped). */
const reactStreamThought = ref('')
const reactInterrupted = ref(false)
/** Normal completion timestamp for restrained「已完成」footnote (Demo phase 4). */
const reactStreamCompletedAt = ref<string | null>(null)
const reactError = ref<string | null>(null)
const feedbackChatRef = ref<{
  send: (opts?: { body?: string }) => Promise<boolean>
  flush: () => Promise<boolean>
  reload: () => Promise<void>
  clearDraft: () => void
} | null>(null)

/**
 * After history was written but gateReactRevise failed, skip re-writing the same
 * issue on retry (avoids duplicate history entries).
 */
const hotRejectHistorySynced = ref(false)

/** Hot 打回 threshold: text ∪ images ∪ annotations ∪ pick screenshot. */
const canSubmitReact = computed(
  () =>
    !reactSending.value &&
    (reactText.value.trim().length > 0 ||
      reactImages.value.length > 0 ||
      reactAnnotations.value.length > 0 ||
      !!pickedElementImage.value?.data),
)

function applyReviewFrame(frame: {
  event?: string
  nodeId?: string
  item?: { text?: string }
  interrupted?: boolean
  message?: string
  waiting?: number
  items?: { text?: string }[]
  busy?: boolean
  activeItem?: { text?: string } | null
}) {
  const producer = props.gate.reactUpstreamNodeId
  if (producer && frame.nodeId && frame.nodeId !== producer) return
  switch (frame.event) {
    case 'turn_begin':
      reactQueued.value.shift()
      reactThinking.value = true
      reactInFlight.value = true
      reactStreamText.value = ''
      reactStreamThought.value = ''
      reactInterrupted.value = false
      reactStreamCompletedAt.value = null
      break
    case 'turn_done':
      reactInFlight.value = false
      reactThinking.value = reactQueued.value.length > 0
      if (frame.interrupted) {
        reactInterrupted.value = true
        reactStreamCompletedAt.value = null
      } else if (reactStreamText.value || reactStreamThought.value) {
        reactStreamCompletedAt.value = new Date().toISOString()
      }
      break
    case 'error':
      reactInFlight.value = false
      reactThinking.value = reactQueued.value.length > 0
      reactError.value = frame.message || reactError.value
      reactStreamCompletedAt.value = null
      if (frame.interrupted) reactInterrupted.value = true
      break
    case 'queue_state': {
      // Platform-authoritative: remote Cancel / cross-entry must clear ghost rows.
      // Refresh resume: busy+activeItem must keep thinking/inFlight even when waiting=0.
      const waiting = typeof frame.waiting === 'number' ? frame.waiting : 0
      const items = Array.isArray(frame.items) ? frame.items : null
      const busy = !!frame.busy
      const activeItem = frame.activeItem
      if (busy && activeItem && !reactInFlight.value) {
        reactInFlight.value = true
        reactThinking.value = true
        reactStreamCompletedAt.value = null
        // Empty rails — host seeds ACP (pending buffer / nodeEvents) next.
        if (!reactStreamText.value && !reactStreamThought.value) {
          reactStreamText.value = ''
          reactStreamThought.value = ''
        }
      }
      if (waiting === 0) {
        reactQueued.value = []
        if (!reactInFlight.value && !busy) reactThinking.value = false
        else reactThinking.value = reactInFlight.value || busy
        break
      }
      if (items) {
        const rebuilt = items.map((it) => ({ text: it.text ?? '' }))
        const maxLocal = reactInFlight.value || busy ? rebuilt.length : rebuilt.length + 1
        if (reactQueued.value.length > maxLocal) {
          const optimistic = reactQueued.value
            .slice(rebuilt.length)
            .slice(0, Math.max(0, maxLocal - rebuilt.length))
          reactQueued.value = [...rebuilt, ...optimistic]
        } else if (reactQueued.value.length < rebuilt.length) {
          reactQueued.value = rebuilt
        } else {
          const optimistic = reactQueued.value.slice(rebuilt.length)
          reactQueued.value = [...rebuilt, ...optimistic]
        }
      }
      reactThinking.value = reactInFlight.value || reactQueued.value.length > 0 || busy
      break
    }
  }
}

function applyAcpEvents(events: { kind?: string; text?: string }[] | undefined) {
  // Accept ACP while busy/inFlight even if waiting=0 cleared the queue panel.
  if ((!reactThinking.value && !reactInFlight.value) || !events?.length) return
  if (!reactThinking.value) reactThinking.value = true
  for (const ev of events) {
    // Ignore tool_call / plan UI; keep rails separate so thought is never overwritten.
    if (ev.kind === 'message' && ev.text) reactStreamText.value = ev.text
    if (ev.kind === 'thought' && ev.text) reactStreamThought.value = ev.text
  }
}

async function cancelReactRevise() {
  if (!props.run?.id || !canReactRevise.value) return
  reactQueued.value = []
  reactThinking.value = false
  reactInFlight.value = false
  reactInterrupted.value = true
  reactStreamCompletedAt.value = null
  try {
    await api.gateReactCancel(props.run.id, props.gate.nodeId)
  } catch (e: any) {
    reactError.value = e?.message || t('pages.gateApproval.reactRevise.failed')
  }
}

defineExpose({ isEditing, applyReviewFrame, applyAcpEvents, cancelReactRevise })

/** 记入意见 requires PreviewIssue-capable content (API: body or images). */
const canRecordIssue = computed(
  () =>
    !reactSending.value &&
    (reactText.value.trim().length > 0 ||
      reactImages.value.length > 0 ||
      !!pickedElementImage.value?.data),
)

function collectUnifiedIssueImages(): { data: string; mimeType: string }[] {
  const images: { data: string; mimeType: string }[] = []
  if (pickedElementImage.value?.data) {
    images.push({
      data: pickedElementImage.value.data,
      mimeType: pickedElementImage.value.mimeType || 'image/png',
    })
  }
  for (const im of reactImages.value) {
    images.push({ data: im.data, mimeType: im.mimeType })
  }
  return images
}

function annotationHistoryBody(anns: ReactAnnotation[]): string {
  const parts = anns
    .map((a) => a.label || a.jsonPath || a.selector || '')
    .map((s) => s.trim())
    .filter(Boolean)
  if (!parts.length) return t('pages.gateApproval.reactRevise.annotationOnly')
  return t('pages.gateApproval.reactRevise.annotationSummary', { summary: parts.join('、') })
}

/**
 * Write unsent unified draft into PreviewIssue history via PreviewFeedbackChat
 * so the sidebar list updates immediately (push + issues-changed).
 */
async function flushFeedbackDraft(): Promise<boolean> {
  if (!props.run?.id || !usesPreviewIssues.value) return false
  const chat = feedbackChatRef.value
  if (chat) {
    try {
      return await chat.flush()
    } catch (e: any) {
      reactError.value = e?.message || t('pages.gateApproval.reactRevise.failed')
      return false
    }
  }
  // Fallback when chat ref is unavailable (should be rare on ReviewShell paths).
  const body = reactText.value.trim()
  const images = collectUnifiedIssueImages()
  if (!body && images.length === 0) return false
  try {
    await api.createPreviewIssue(
      props.run.id,
      props.gate.nodeId,
      body,
      pickedSelector.value || '',
      0,
      images,
    )
    reactText.value = ''
    reactImages.value = []
    clearHtmlPreviewPick()
    await loadPreviewIssues()
    return true
  } catch (e: any) {
    reactError.value = e?.message || t('pages.gateApproval.reactRevise.failed')
    return false
  }
}

/** Hot: 记入意见 — PreviewIssue only, no gateReactRevise. */
async function recordFeedbackIssue() {
  if (!canRecordIssue.value) return
  reactSending.value = true
  reactError.value = null
  try {
    const ok = await flushFeedbackDraft()
    if (!ok && !reactError.value) {
      reactError.value = t('pages.gateApproval.reactRevise.failed')
    }
  } finally {
    reactSending.value = false
  }
}

/** Non-preview ReviewComposer path: gateReactRevise only. */
async function sendReactRevise() {
  if (!props.run?.id || !canSubmitReact.value) return
  const body = reactText.value.trim()
  reactQueued.value.push({ text: body })
  reactThinking.value = true
  reactSending.value = true
  reactError.value = null
  try {
    await api.gateReactRevise(
      props.run.id,
      props.gate.nodeId,
      body,
      reactImages.value.map((im) => ({ data: im.data, mimeType: im.mimeType })),
      reactAnnotations.value.slice(),
    )
    clearUnifiedDraft()
    emit('react-revised')
  } catch (e: any) {
    reactQueued.value.pop()
    reactThinking.value = reactQueued.value.length > 0 || reactStreamText.value.length > 0
    reactError.value = e?.message || t('pages.gateApproval.reactRevise.failed')
  } finally {
    reactSending.value = false
  }
}

/**
 * Sync PreviewIssue history for hot reject (text/images via flush; annotation-only
 * via placeholder body). Returns false if a required history write failed.
 */
async function syncHotRejectHistory(anns: ReactAnnotation[]): Promise<boolean> {
  if (!usesPreviewIssues.value || !props.run?.id) return true
  if (hotRejectHistorySynced.value) return true

  const body = reactText.value.trim()
  const issueImages = collectUnifiedIssueImages()
  const chat = feedbackChatRef.value

  if (body || issueImages.length > 0) {
    const ok = await flushFeedbackDraft()
    if (ok) hotRejectHistorySynced.value = true
    return ok
  }

  if (anns.length === 0) return true

  // Annotation-only: write a placeholder so f4 history stays consistent.
  const summary = annotationHistoryBody(anns)
  try {
    if (chat) {
      const ok = await chat.send({ body: summary })
      if (ok) hotRejectHistorySynced.value = true
      return ok
    }
    await api.createPreviewIssue(props.run.id, props.gate.nodeId, summary, '', 0, [])
    await loadPreviewIssues()
    await feedbackChatRef.value?.reload?.()
    hotRejectHistorySynced.value = true
    return true
  } catch (e: any) {
    reactError.value = e?.message || t('pages.gateApproval.reactRevise.failed')
    return false
  }
}

/**
 * Hot preview path: sync PreviewIssue history then gateReactRevise (send).
 * When n_open≥1, allow send without extra draft. Empty send with n_open===0
 * still needs payload. On revise failure after history write, keep the draft.
 */
async function sendHotReject() {
  if (!props.run?.id || reactSending.value) return
  const hasPayload =
    reactText.value.trim().length > 0 ||
    reactImages.value.length > 0 ||
    reactAnnotations.value.length > 0 ||
    !!pickedElementImage.value?.data
  // n_open≥1 may send without extra draft; otherwise need payload.
  if (!hasPayload && openPreviewIssueCount.value === 0) return
  reactSending.value = true
  reactError.value = null
  const body = reactText.value.trim()
  const issueImages = collectUnifiedIssueImages()
  const anns = reactAnnotations.value.slice()
  const reviseImages = issueImages.map((im) => ({ data: im.data, mimeType: im.mimeType }))
  const draftSnap = {
    text: reactText.value,
    images: reactImages.value.slice(),
    anns: anns.slice(),
    selector: pickedSelector.value,
    elementImage: pickedElementImage.value,
  }
  reactQueued.value.push({ text: body || '(annotate)' })
  reactThinking.value = true
  try {
    const synced = await syncHotRejectHistory(anns)
    if (!synced) {
      reactQueued.value.pop()
      return
    }

    await api.gateReactRevise(props.run.id, props.gate.nodeId, body, reviseImages, anns)
    clearUnifiedDraft()
    emit('react-revised')
  } catch (e: any) {
    reactQueued.value.pop()
    reactThinking.value = reactQueued.value.length > 0
    if (hotRejectHistorySynced.value) {
      reactError.value = t('pages.gateApproval.reactRevise.issueSavedReviseFailed')
      reactText.value = draftSnap.text
      reactImages.value = draftSnap.images
      reactAnnotations.value = draftSnap.anns
      pickedSelector.value = draftSnap.selector
      pickedElementImage.value = draftSnap.elementImage
    } else {
      reactError.value = e?.message || t('pages.gateApproval.reactRevise.failed')
    }
  } finally {
    reactSending.value = false
  }
}

/** Cold sidebar action: flush unsent feedback text before revise; approve via choose. */
async function onSidebarAction(id: string) {
  if (REVERT_ACTION_IDS.has(id) && usesPreviewIssues.value) {
    reactSending.value = true
    reactError.value = null
    try {
      await flushFeedbackDraft()
    } finally {
      reactSending.value = false
    }
  }
  choose(id)
}

/** Desktop/narrow ReviewShell for content-fit gates (stage | sidebar). */
const useReviewShellLayout = computed(
  () =>
    !isProposalSelect.value &&
    (shouldContentFit.value ||
      (canEditProducts.value && !!props.fillPreview && !isAppPreview.value)) &&
    !useMobileFillRemaining.value,
)

const passAction = computed(() => {
  // Prefer configured positive action; fall back to first approve/pass id for confirm.
  return (
    props.gate.actions.find((a) => POSITIVE_ACTION_IDS.has(a.id)) ||
    footerActions.value.find((a) => POSITIVE_ACTION_IDS.has(a.id))
  )
})

/** Align with review semantics: confirm disabled when open PreviewIssues or busy.
 * Also FR4: busy (thinking / queued) disables confirm — Cancel first or wait ready. */
const composerPassDisabled = computed(
  () =>
    !passAction.value ||
    isProposalSelect.value ||
    isActionDisabled(passAction.value.id) ||
    reactThinking.value ||
    reactQueued.value.length > 0 ||
    (usesPreviewIssues.value && openPreviewIssueCount.value >= 1),
)

function onComposerPass() {
  if (!showHotPass.value || composerPassDisabled.value) return
  const a = passAction.value
  if (a) choose(a.id)
}

/** PreviewIssues hot reject syncs history; non-preview uses draft-only revise. */
function onComposerReject() {
  if (!showHotReject.value) return
  if (usesPreviewIssues.value) {
    void sendHotReject()
    return
  }
  void sendReactRevise()
}
</script>

<template>
  <div class="flex h-full min-h-0 flex-col">
    <div v-if="!compact" class="shrink-0 border-b border-line px-4 py-3">
      <div class="flex items-center gap-2">
        <Icon name="gate" :size="16" class="text-warn" />
        <span class="text-sm font-semibold text-txt">{{ gate.title }}</span>
      </div>
      <div class="mt-1 text-[11px] text-txt3">{{ t('pages.gateApproval.subtitle') }}</div>
    </div>

    <!-- Run-detail mobile visual: ReviewShell stage (fillParent) + drawer composer (f9). -->
    <div
      v-if="useMobileFillRemaining"
      class="flex min-h-0 flex-1 flex-col overflow-hidden"
      data-testid="mobile-fill-remaining"
    >
      <ReviewShell
        class="min-h-0 flex-1"
        mobile
        :sidebar-width="400"
        :drawer-height="340"
        :storage-key="REVIEW_SHELL_WIDTH_KEY_APPROVAL"
      >
        <template #stage>
          <div
            class="flex h-full min-h-0 flex-col overflow-hidden"
            data-testid="mobile-fill-scroll"
          >
            <div
              class="flex min-h-0 flex-1 flex-col overflow-hidden border border-line"
              data-testid="mobile-fill-preview"
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
              <GateProductEditor
                v-if="canEditProducts && run"
                ref="productEditorRef"
                class="min-h-0 flex-1"
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
                fill-parent
                :inspectable="isVisualBody"
                @saved="onProductSaved"
                @dirty-change="productDirty = $event"
                @refresh-request="onProductRefresh"
                @retry-load="retryLoadProduct"
                @pick="onHtmlPreviewPick"
              />
              <div
                v-else-if="productLoadError"
                class="flex min-h-0 flex-1 flex-col items-center justify-center gap-2.5 px-6 py-8 text-center"
                data-testid="mobile-fill-product-error"
                role="alert"
              >
                <p class="text-[13px] font-medium text-txt">{{ t('pages.gateApproval.previewLoadFailedTitle') }}</p>
                <p class="max-w-[36ch] text-[12px] text-txt3">
                  {{ t('pages.gateApproval.previewLoadFailedBody') }}
                </p>
                <p class="max-w-[42ch] text-[11px] text-err">{{ productLoadError }}</p>
                <button
                  type="button"
                  class="mt-1 bg-accent px-3.5 py-1.5 text-xs font-medium text-white hover:bg-accent-2"
                  data-testid="mobile-fill-product-retry"
                  @click="retryLoadProduct"
                >
                  {{ t('pages.gateApproval.previewRetry') }}
                </button>
              </div>
              <HtmlPreview
                v-else-if="shouldFillPreview"
                class="min-h-0 flex-1"
                :html="productHtml"
                mode="inline"
                :enlargeable="false"
                fill-parent
                inspectable
                @pick="onHtmlPreviewPick"
              />
              <div
                v-else
                class="flex min-h-0 flex-1 items-center justify-center text-[12px] text-txt3"
                data-testid="mobile-fill-product-loading"
              >
                <Icon name="spinner" :size="20" class="mr-2 animate-spin text-accent" />
                {{ t('pages.gateApproval.loadingArtifact') }}
              </div>
            </div>
            <UpstreamRequirementContext
              :artifacts="run?.artifacts"
              :run-id="run?.id"
              :run-status="run?.status"
              :product-name="productName"
              :body-template="bodyTemplate"
            />
          </div>
        </template>
        <template #sidebar>
          <div
            class="flex h-full min-h-0 flex-col overflow-hidden p-3 safe-area-bottom"
            data-testid="mobile-fill-sticky-actions"
          >
            <!-- Cold gate.form only (hidden when preview feedback / hot unified input owns the draft). -->
            <div v-if="gate.form?.length && !shouldHideGateForm" class="mb-3 shrink-0 space-y-3">
              <div v-for="f in gate.form" :key="f.key">
                <label class="label">{{ f.label }} <span v-if="f.required" class="text-err">*</span></label>
                <ParagraphInput
                  v-model:text="formText[f.key]"
                  v-model:images="formImages[f.key]"
                  text-only
                  :placeholder="t('pages.gateApproval.formPlaceholder', { label: f.label })"
                />
              </div>
            </div>
            <div v-if="formError" class="mb-2 shrink-0 rounded-md border border-err/30 bg-err/10 px-3 py-2 text-xs text-err">
              {{ formError }}
            </div>
            <div v-if="resolved" class="rounded-md border border-ok/30 bg-ok/10 px-3 py-2 text-xs text-ok">
              {{ t('pages.gateApproval.submitted', { label: gate.actions.find((a) => a.id === resolved)?.label }) }}
            </div>
            <div
              v-else-if="usesPreviewIssues && previewIssuesLoading"
              class="flex items-center justify-center gap-2 py-2 text-xs text-txt3"
              data-testid="mobile-fill-preview-issues-loading"
            >
              <Icon name="spinner" :size="14" class="animate-spin text-accent" />
              {{ t('pages.gateApproval.loadingPreviewIssues') }}
            </div>
            <div v-else class="flex min-h-0 flex-1 flex-col">
              <p
                v-if="usesPreviewIssues && openPreviewIssueCount === 0"
                class="mb-2 shrink-0 text-[11px] leading-relaxed text-txt3"
              >
                <b class="font-medium text-txt2">{{ t('pages.clarify.confirmFlow') }}</b>
                {{ t('pages.gateApproval.helpApproveDetail') }}
                <span class="mx-1">·</span>
                <b class="font-medium text-txt2">{{ t('pages.reviewComposer.send') }}</b>
                {{ helpReviseDetailNoIssuesText }}
              </p>
              <p
                v-else-if="usesPreviewIssues && openPreviewIssueCount >= 1"
                class="mb-2 shrink-0 text-[11px] leading-relaxed text-txt3"
              >
                <b class="font-medium text-txt2">{{ t('pages.reviewComposer.send') }}</b>
                {{ t('pages.gateApproval.helpReviseWithIssuesDetail') }}
                <span class="mx-1">·</span>
                {{ t('pages.reviewComposer.openIssuesConfirmHint') }}
              </p>
              <div
                v-if="previewIssuesError"
                class="mb-2 shrink-0 rounded-md border border-warn/30 bg-warn/10 px-2.5 py-2 text-[11px] text-warn"
                data-testid="mobile-fill-preview-issues-error"
              >
                {{ t('pages.gateApproval.loadPreviewIssuesFailed') }}
                <button
                  type="button"
                  class="ml-2 underline"
                  @click="loadPreviewIssues()"
                >
                  {{ t('pages.gateApproval.previewRetry') }}
                </button>
              </div>
              <div
                v-if="isColdSession"
                class="mb-2 shrink-0 border border-warn/30 bg-warn/10 px-2.5 py-2 text-[11.5px] leading-relaxed text-warn"
                data-testid="gate-cold-session-note"
              >
                {{ t('pages.reviewComposer.coldNote') }}
              </div>
              <!-- Help → scrollable feedback → sticky decisions -->
              <div
                v-if="usesPreviewIssues && run"
                class="flex min-h-0 flex-1 flex-col"
                data-testid="mobile-fill-feedback"
              >
                <PreviewFeedbackChat
                  ref="feedbackChatRef"
                  class="min-h-0 flex-1"
                  :run-id="run.id"
                  :node-id="gate.nodeId"
                  :selector="pickedSelector"
                  :element-image="pickedElementImage"
                  v-model:text="reactText"
                  v-model:images="reactImages"
                  copy-variant="review"
                  fill-sidebar
                  :hide-submit="canReactRevise"
                  @clear-selector="clearHtmlPreviewPick"
                  @issues-changed="loadPreviewIssues()"
                />
              </div>
              <div
                v-if="canReactRevise && usesPreviewIssues"
                class="mt-2 shrink-0"
                data-testid="review-composer-gate"
              >
                <div v-if="reactError" class="mb-1.5 text-[11px] text-err">{{ reactError }}</div>
                <!-- review semantics: 记入 + 发送 + 确认并流转 -->
                <div class="mb-2 flex justify-end">
                  <button
                    type="button"
                    class="inline-flex min-h-[36px] items-center justify-center rounded-md border border-line px-3 text-xs font-medium text-txt2 transition hover:bg-elevated disabled:cursor-not-allowed disabled:opacity-50"
                    data-testid="review-record-issue"
                    :disabled="!canRecordIssue"
                    @click="recordFeedbackIssue"
                  >
                    {{ t('pages.gateApproval.reviewFeedback.record') }}
                  </button>
                </div>
                <div class="flex flex-wrap gap-2" data-testid="review-composer-actions">
                  <button
                    v-if="showHotReject"
                    type="button"
                    class="inline-flex min-h-[44px] flex-1 items-center justify-center gap-1.5 bg-accent/15 px-3 text-sm font-medium text-accent-2 transition hover:bg-accent/25 disabled:cursor-not-allowed disabled:opacity-50"
                    data-testid="review-composer-send"
                    :disabled="reactSending || (!hotRejectAllowEmpty && !canSubmitReact)"
                    @click="sendHotReject"
                  >
                    <Icon name="arrow-left" :size="14" />
                    {{ reactSending ? t('pages.gateApproval.reactRevise.sending') : composerRejectLabel }}
                  </button>
                  <button
                    v-if="showHotPass"
                    type="button"
                    class="inline-flex min-h-[44px] flex-1 items-center justify-center gap-1.5 bg-ok/15 px-3 text-sm font-medium text-ok transition hover:bg-ok/25 disabled:cursor-not-allowed disabled:opacity-50"
                    data-testid="review-composer-pass"
                    :disabled="composerPassDisabled"
                    :title="passAction ? actionButtonTitle(passAction.id) : ''"
                    @click="onComposerPass"
                  >
                    <Icon name="check" :size="14" />
                    {{ t('pages.clarify.confirmFlow') }}
                  </button>
                  <button
                    v-if="canReactRevise && (reactThinking || reactQueued.length)"
                    type="button"
                    class="inline-flex min-h-[44px] items-center justify-center gap-1.5 border border-line bg-elevated px-3 text-sm font-medium text-txt2"
                    data-testid="gate-react-cancel"
                    title="Cancel"
                    @click="cancelReactRevise"
                  >
                    Cancel
                  </button>
                </div>
                <div
                  v-if="reactQueued.length"
                  class="mt-2 rounded border border-line bg-base/40 px-2 py-1.5"
                  data-testid="gate-react-queue"
                >
                  <div class="mb-1 text-[11px] text-txt3">
                    {{ t('pages.agentChatTester.queue', { n: reactQueued.length }) }}
                  </div>
                  <div
                    v-for="(q, qi) in reactQueued"
                    :key="qi"
                    class="truncate text-[12px] text-txt2"
                  >
                    {{ qi + 1 }}. {{ q.text }}
                  </div>
                </div>
                <GateReactStreamPanel
                  :thinking="reactThinking"
                  :stream-text="reactStreamText"
                  :stream-thought="reactStreamThought"
                  :interrupted="reactInterrupted"
                  :completed-at="reactStreamCompletedAt"
                />
              </div>
              <div v-else class="mt-2 flex shrink-0 flex-col gap-2">
                <p
                  v-if="isColdSession"
                  class="text-[11px] leading-relaxed text-warn"
                  data-testid="review-composer-cold-note"
                >
                  {{ t('pages.reviewComposer.coldNote') }}
                </p>
                <div class="flex flex-wrap gap-3">
                  <button
                    v-for="a in footerActions"
                    :key="a.id"
                    class="inline-flex min-h-[44px] flex-1 items-center justify-center gap-1.5 rounded-md px-3.5 text-sm font-medium transition disabled:cursor-not-allowed disabled:opacity-50"
                    :class="actionVariantClasses(actionVariant(a.id))"
                    :disabled="isActionDisabled(a.id) || reactSending || (usesPreviewIssues && openPreviewIssueCount >= 1)"
                    :title="actionButtonTitle(a.id)"
                    data-testid="review-composer-pass"
                    @click="onSidebarAction(a.id)"
                  >
                    <Icon :name="actionIcon(a.id)" :size="14" />
                    {{ a.label }}
                  </button>
                </div>
              </div>
            </div>
          </div>
        </template>
      </ReviewShell>
    </div>

    <!-- Content-fit + fillPreview: ReviewShell (stage | sidebar); keep content-fit-* testids. -->
    <div
      v-else-if="useReviewShellLayout"
      class="flex min-h-0 flex-1 flex-col overflow-hidden"
      data-testid="content-fit-scroll"
    >
      <ReviewShell
        class="min-h-0 flex-1"
        :mobile="isMobile"
        :sidebar-width="400"
        :storage-key="REVIEW_SHELL_WIDTH_KEY_APPROVAL"
      >
        <template #stage>
          <div class="flex h-full min-h-0 flex-col overflow-hidden">
            <div
              ref="gateStageEl"
              class="scroll-area min-h-0 overflow-x-hidden overflow-y-auto border border-line"
              data-testid="content-fit-preview"
              data-review-annotate-stage
              :style="{ maxHeight: `${CONTENT_FIT_PREVIEW_MAX_VH}vh` }"
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
              <GateProductEditor
                v-if="canEditProducts && run"
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
                :fit-content="!isMobile"
                :max-content-height-vh="CONTENT_FIT_PREVIEW_MAX_VH"
                :content-height-offset-px="contentFitChromeOffsetPx"
                :inspectable="isVisualBody"
                @saved="onProductSaved"
                @dirty-change="productDirty = $event"
                @refresh-request="onProductRefresh"
                @retry-load="retryLoadProduct"
                @pick="onHtmlPreviewPick"
              />
              <div
                v-else-if="productLoadError"
                class="flex min-h-[200px] flex-col items-center justify-center gap-2.5 px-6 py-8 text-center"
                data-testid="content-fit-product-error"
                role="alert"
              >
                <p class="text-[13px] font-medium text-txt">{{ t('pages.gateApproval.previewLoadFailedTitle') }}</p>
                <p class="max-w-[36ch] text-[12px] text-txt3">
                  {{ t('pages.gateApproval.previewLoadFailedBody') }}
                </p>
                <p class="max-w-[42ch] text-[11px] text-err">{{ productLoadError }}</p>
                <button
                  type="button"
                  class="mt-1 bg-accent px-3.5 py-1.5 text-xs font-medium text-white hover:bg-accent-2"
                  data-testid="content-fit-product-retry"
                  @click="retryLoadProduct"
                >
                  {{ t('pages.gateApproval.previewRetry') }}
                </button>
              </div>
              <HtmlPreview
                v-else-if="shouldFillPreview"
                :html="productHtml"
                :mode="isMobile ? 'inline' : 'default'"
                :enlargeable="!isMobile"
                :fit-content="!isMobile"
                :max-content-height-vh="CONTENT_FIT_PREVIEW_MAX_VH"
                :content-height-offset-px="contentFitChromeOffsetPx"
                inspectable
                @pick="onHtmlPreviewPick"
              />
              <div v-else-if="shouldFitStructured && productName" class="p-4">
                <StructuredArtifactView
                  :name="productName"
                  :doc="productDoc"
                  :artifacts="run?.artifacts"
                  :run-id="run?.id"
                  :run-status="run?.status"
                />
              </div>
            </div>
            <UpstreamRequirementContext
              :artifacts="run?.artifacts"
              :run-id="run?.id"
              :run-status="run?.status"
              :product-name="productName"
              :body-template="bodyTemplate"
            />
          </div>
        </template>
        <template #sidebar>
          <div
            class="flex h-full min-h-0 shrink-0 flex-col overflow-hidden border-t border-line p-3 safe-area-bottom"
            data-testid="content-fit-form"
          >
            <div v-if="gate.form?.length && !shouldHideGateForm" class="mb-3 shrink-0 space-y-3">
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
            <div v-if="formError" class="mb-2 shrink-0 rounded-md border border-err/30 bg-err/10 px-3 py-2 text-xs text-err">
              {{ formError }}
            </div>
            <div v-if="resolved" class="rounded-md border border-ok/30 bg-ok/10 px-3 py-2 text-xs text-ok">
              {{ t('pages.gateApproval.submitted', { label: gate.actions.find((a) => a.id === resolved)?.label }) }}
            </div>
            <div
              v-else-if="usesPreviewIssues && previewIssuesLoading"
              class="flex items-center justify-center gap-2 py-2 text-xs text-txt3"
              data-testid="content-fit-preview-issues-loading"
            >
              <Icon name="spinner" :size="14" class="animate-spin text-accent" />
              {{ t('pages.gateApproval.loadingPreviewIssues') }}
            </div>
            <div v-else class="flex min-h-0 flex-1 flex-col">
              <p
                v-if="usesPreviewIssues && openPreviewIssueCount === 0"
                class="mb-2 shrink-0 text-[11px] leading-relaxed text-txt3"
              >
                <b class="font-medium text-txt2">{{ t('pages.clarify.confirmFlow') }}</b>
                {{ t('pages.gateApproval.helpApproveDetail') }}
                <span class="mx-1">·</span>
                <b class="font-medium text-txt2">{{ t('pages.reviewComposer.send') }}</b>
                {{ helpReviseDetailNoIssuesText }}
              </p>
              <p
                v-else-if="usesPreviewIssues && openPreviewIssueCount >= 1"
                class="mb-2 shrink-0 text-[11px] leading-relaxed text-txt3"
              >
                <b class="font-medium text-txt2">{{ t('pages.reviewComposer.send') }}</b>
                {{ t('pages.gateApproval.helpReviseWithIssuesDetail') }}
                <span class="mx-1">·</span>
                {{ t('pages.reviewComposer.openIssuesConfirmHint') }}
              </p>
              <p v-else-if="canEditProducts" class="mb-2 shrink-0 text-[11px] leading-relaxed text-txt3">
                <b class="font-medium text-txt2">{{ t('pages.clarify.confirmFlow') }}</b>
                {{ t('pages.gateApproval.helpApproveDetail') }}
                <span class="mx-1">·</span>
                <b class="font-medium text-txt2">{{ t('pages.reviewComposer.send') }}</b>
                {{ t('pages.gateApproval.helpReviseDetail') }}
              </p>
              <div
                v-if="previewIssuesError"
                class="mb-2 shrink-0 rounded-md border border-warn/30 bg-warn/10 px-2.5 py-2 text-[11px] text-warn"
                data-testid="content-fit-preview-issues-error"
              >
                {{ t('pages.gateApproval.loadPreviewIssuesFailed') }}
                <button
                  type="button"
                  class="ml-2 underline"
                  @click="loadPreviewIssues()"
                >
                  {{ t('pages.gateApproval.previewRetry') }}
                </button>
              </div>
              <div
                v-if="isColdSession"
                class="mb-2 shrink-0 border border-warn/30 bg-warn/10 px-2.5 py-2 text-[11.5px] leading-relaxed text-warn"
                data-testid="gate-cold-session-note"
              >
                {{ t('pages.reviewComposer.coldNote') }}
              </div>
              <!-- Preview path: unified feedback in sidebar (hot hides built-in submit). -->
              <div
                v-if="usesPreviewIssues && run"
                class="flex min-h-0 flex-1 flex-col"
                data-testid="content-fit-feedback"
              >
                <PreviewFeedbackChat
                  ref="feedbackChatRef"
                  class="min-h-0 flex-1"
                  :run-id="run.id"
                  :node-id="gate.nodeId"
                  :selector="pickedSelector"
                  :element-image="pickedElementImage"
                  v-model:text="reactText"
                  v-model:images="reactImages"
                  copy-variant="review"
                  fill-sidebar
                  :hide-submit="canReactRevise"
                  @clear-selector="clearHtmlPreviewPick"
                  @issues-changed="loadPreviewIssues()"
                />
              </div>
              <!-- Hot unified actions (no ReviewComposer input): 记入 + 发送 + 确认并流转. -->
              <div
                v-if="canReactRevise && usesPreviewIssues"
                class="mt-2 shrink-0"
                data-testid="review-composer-gate"
              >
                <div v-if="reactError" class="mb-1.5 text-[11px] text-err">{{ reactError }}</div>
                <div class="mb-2 flex justify-end">
                  <button
                    type="button"
                    class="inline-flex items-center justify-center rounded-md border border-line px-3 py-1.5 text-xs font-medium text-txt2 transition hover:bg-elevated disabled:cursor-not-allowed disabled:opacity-50"
                    :class="isMobile ? 'min-h-[36px]' : ''"
                    data-testid="review-record-issue"
                    :disabled="!canRecordIssue"
                    @click="recordFeedbackIssue"
                  >
                    {{ t('pages.gateApproval.reviewFeedback.record') }}
                  </button>
                </div>
                <div class="flex flex-wrap gap-2" data-testid="review-composer-actions">
                  <button
                    v-if="showHotReject"
                    type="button"
                    class="inline-flex flex-1 items-center justify-center gap-1.5 bg-accent/15 px-3 py-2 text-sm font-medium text-accent-2 transition hover:bg-accent/25 disabled:cursor-not-allowed disabled:opacity-50"
                    :class="isMobile ? 'min-h-[44px]' : ''"
                    data-testid="review-composer-send"
                    :disabled="reactSending || (!hotRejectAllowEmpty && !canSubmitReact)"
                    @click="sendHotReject"
                  >
                    <Icon name="arrow-left" :size="14" />
                    {{ reactSending ? t('pages.gateApproval.reactRevise.sending') : composerRejectLabel }}
                  </button>
                  <button
                    v-if="showHotPass"
                    type="button"
                    class="inline-flex flex-1 items-center justify-center gap-1.5 bg-ok/15 px-3 py-2 text-sm font-medium text-ok transition hover:bg-ok/25 disabled:cursor-not-allowed disabled:opacity-50"
                    :class="isMobile ? 'min-h-[44px]' : ''"
                    data-testid="review-composer-pass"
                    :disabled="composerPassDisabled"
                    :title="passAction ? actionButtonTitle(passAction.id) : ''"
                    @click="onComposerPass"
                  >
                    <Icon name="check" :size="14" />
                    {{ t('pages.clarify.confirmFlow') }}
                  </button>
                  <button
                    v-if="canReactRevise && (reactThinking || reactQueued.length)"
                    type="button"
                    class="inline-flex items-center justify-center gap-1.5 border border-line bg-elevated px-3 py-2 text-sm font-medium text-txt2"
                    :class="isMobile ? 'min-h-[44px]' : ''"
                    data-testid="gate-react-cancel"
                    title="Cancel"
                    @click="cancelReactRevise"
                  >
                    Cancel
                  </button>
                </div>
                <div
                  v-if="reactQueued.length"
                  class="mt-2 rounded border border-line bg-base/40 px-2 py-1.5"
                  data-testid="gate-react-queue"
                >
                  <div class="mb-1 text-[11px] text-txt3">
                    {{ t('pages.agentChatTester.queue', { n: reactQueued.length }) }}
                  </div>
                  <div
                    v-for="(q, qi) in reactQueued"
                    :key="qi"
                    class="truncate text-[12px] text-txt2"
                  >
                    {{ qi + 1 }}. {{ q.text }}
                  </div>
                </div>
                <GateReactStreamPanel
                  :thinking="reactThinking"
                  :stream-text="reactStreamText"
                  :stream-thought="reactStreamThought"
                  :interrupted="reactInterrupted"
                  :completed-at="reactStreamCompletedAt"
                />
              </div>
              <!-- Non-preview hot (structured etc.): ReviewComposer review semantics. -->
              <ReviewComposer
                v-else-if="canReactRevise"
                class="min-h-0 flex-1"
                mode="gate"
                v-model:draft="reactText"
                v-model:attachments="reactImages"
                v-model:annotations="reactAnnotations"
                :can-reject="showHotReject"
                :can-pass="showHotPass"
                :reject-allow-empty="hotRejectAllowEmpty"
                :rejecting="reactSending"
                :reject-error="reactError"
                :pass-disabled="composerPassDisabled"
                :pass-title="passAction ? actionButtonTitle(passAction.id) : ''"
                :pass-label="t('pages.clarify.confirmFlow')"
                :reject-label="composerRejectLabel"
                :queued="reactQueued"
                :thinking="reactThinking"
                :stream-text="reactStreamText"
                :stream-thought="reactStreamThought"
                :interrupted="reactInterrupted"
                :stream-completed-at="reactStreamCompletedAt"
                @reject="onComposerReject"
                @pass="onComposerPass"
                @cancel="cancelReactRevise"
              />
              <div
                v-else
                class="mt-2 flex shrink-0 flex-col gap-2"
              >
                <p
                  v-if="isColdSession"
                  class="text-[11px] leading-relaxed text-warn"
                  data-testid="review-composer-cold-note"
                >
                  {{ t('pages.reviewComposer.coldNote') }}
                </p>
                <div class="flex flex-wrap gap-2" :class="isMobile ? 'gap-3' : ''">
                  <button
                    v-for="a in footerActions"
                    :key="a.id"
                    class="inline-flex items-center justify-center gap-1.5 rounded-md px-3.5 text-sm font-medium transition disabled:cursor-not-allowed disabled:opacity-50"
                    :class="[
                      actionVariantClasses(actionVariant(a.id)),
                      isMobile ? 'min-h-[44px] flex-1' : 'py-2',
                    ]"
                    :disabled="isActionDisabled(a.id) || reactSending || (usesPreviewIssues && openPreviewIssueCount >= 1)"
                    :title="actionButtonTitle(a.id)"
                    data-testid="review-composer-pass"
                    @click="onSidebarAction(a.id)"
                  >
                    <Icon :name="actionIcon(a.id)" :size="14" />
                    {{ a.label }}
                  </button>
                </div>
              </div>
            </div>
          </div>
        </template>
      </ReviewShell>
    </div>

    <template v-else>
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
            <ReviewComposer
              mode="gate"
              v-model:draft="reactText"
              v-model:attachments="reactImages"
              v-model:annotations="reactAnnotations"
              :can-reject="showHotReject"
              :can-pass="showHotPass"
              :reject-allow-empty="hotRejectAllowEmpty"
              :rejecting="reactSending"
              :reject-error="reactError"
              :pass-disabled="composerPassDisabled"
              :pass-title="passAction ? actionButtonTitle(passAction.id) : ''"
              :pass-label="t('pages.clarify.confirmFlow')"
              :reject-label="composerRejectLabel"
              :queued="reactQueued"
              :thinking="reactThinking"
              :stream-text="reactStreamText"
              :stream-thought="reactStreamThought"
              :interrupted="reactInterrupted"
              :stream-completed-at="reactStreamCompletedAt"
              @reject="onComposerReject"
              @pass="onComposerPass"
              @cancel="cancelReactRevise"
            />
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
        <div v-if="formError" class="mb-2 rounded-md border border-err/30 bg-err/10 px-3 py-2 text-xs text-err">
          {{ formError }}
        </div>
        <div v-if="resolved" class="rounded-md border border-ok/30 bg-ok/10 px-3 py-2 text-xs text-ok">
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
            v-if="usesPreviewIssues && openPreviewIssueCount === 0"
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
            <b class="font-medium text-txt2">{{ t('pages.reviewComposer.send') }}</b>
            {{ t('pages.gateApproval.helpReviseWithIssuesDetail') }}
            <span class="mx-1">·</span>
            {{ t('pages.reviewComposer.openIssuesConfirmHint') }}
          </p>
          <p v-else-if="canEditProducts" class="mb-2 text-[11px] leading-relaxed text-txt3">
            <b class="font-medium text-txt2">{{ t('pages.clarify.confirmFlow') }}</b>
            {{ t('pages.gateApproval.helpApproveDetail') }}
            <span class="mx-1">·</span>
            <b class="font-medium text-txt2">{{ t('pages.reviewComposer.send') }}</b>
            {{ t('pages.gateApproval.helpReviseDetail') }}
          </p>
          <p
            v-if="isColdSession && !canReactRevise"
            class="mb-2 text-[11px] leading-relaxed text-warn"
            data-testid="review-composer-cold-note"
          >
            {{ t('pages.reviewComposer.coldNote') }}
          </p>
          <ReviewComposer
            v-if="canReactRevise"
            class="mb-3"
            mode="gate"
            v-model:draft="reactText"
            v-model:attachments="reactImages"
            v-model:annotations="reactAnnotations"
            :can-reject="showHotReject"
            :can-pass="showHotPass"
            :reject-allow-empty="hotRejectAllowEmpty"
            :rejecting="reactSending"
            :reject-error="reactError"
            :pass-disabled="composerPassDisabled"
            :pass-title="passAction ? actionButtonTitle(passAction.id) : ''"
            :pass-label="t('pages.clarify.confirmFlow')"
            :reject-label="composerRejectLabel"
            :queued="reactQueued"
            :thinking="reactThinking"
            :stream-text="reactStreamText"
            :stream-thought="reactStreamThought"
            :interrupted="reactInterrupted"
            :stream-completed-at="reactStreamCompletedAt"
            @reject="onComposerReject"
            @pass="onComposerPass"
            @cancel="cancelReactRevise"
          />
          <div v-else class="flex flex-wrap gap-2" :class="isMobile ? 'gap-3' : ''">
            <button
              v-for="a in footerActions"
              :key="a.id"
              class="inline-flex items-center justify-center gap-1.5 rounded-md px-3.5 text-sm font-medium transition disabled:cursor-not-allowed disabled:opacity-50"
              :class="[
                actionVariantClasses(actionVariant(a.id)),
                isMobile ? 'min-h-[44px] flex-1' : 'py-2',
              ]"
              :disabled="isActionDisabled(a.id) || (usesPreviewIssues && openPreviewIssueCount >= 1)"
              :title="actionButtonTitle(a.id)"
              data-testid="review-composer-pass"
              @click="choose(a.id)"
            >
              <Icon :name="actionIcon(a.id)" :size="14" />
              {{ a.label }}
            </button>
          </div>
        </div>
      </div>
    </template>

    <SelectionAddToChat
      v-if="selectionQuoteEnabled"
      :enabled="selectionQuoteEnabled"
      :root="gateStageEl"
      @add="onQuoteAdd"
    />
  </div>
</template>

<style scoped>
/* Stream four-phase UI styles live in GateReactStreamPanel. */
</style>
