<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/ui/Icon.vue'
import HtmlPreview from '@/components/ui/HtmlPreview.vue'
import ArtifactPreview from '@/components/run/ArtifactPreview.vue'
import NovncPreviewPanel from '@/components/run/NovncPreviewPanel.vue'
import AppPreviewPanel from '@/components/run/AppPreviewPanel.vue'
import PublicAppPreviewPanel from '@/components/run/PublicAppPreviewPanel.vue'
import { api } from '@/lib/api/api'
import type { PublicPreviewPort } from '@/lib/inbox/gateShareLink'
import { relTime } from '@/lib/shared/format'
import { isAbortError } from '@/lib/run/liveLogRehydrate'
import { useToast } from '@/lib/composables/useToast'
import { provideReviewAnnotate, useReviewAnnotate } from '@/lib/inbox/reviewAnnotate'
import { addClarifyAnnotation } from '@/lib/inbox/useClarifyDraft'
import type { AppPreviewPickPayload } from '@/lib/shared/previewPickUrl'
import type { Artifact, ReactAnnotation } from '@/lib/shared/types'
import {
  REACT_STAGE_TAB_GRID,
  REACT_STAGE_TAB_NOVNC,
  artifactKindLabelKey,
  artifactRevision,
  closeStagePreviewTab,
  findArtifactByName,
  nextTabAfterClose,
  openStagePreviewTab,
  previewTabId,
  previewTabName,
  artifactFingerprint,
  canAnnotateStageArtifact,
  isOwnNodeArtifact,
  resolveStageRemoteKind,
  shouldActivatePinnedPreview,
  type ReactStageRemoteKind,
} from '@/lib/run/reactArtifactPreview'

const props = withDefaults(
  defineProps<{
    artifacts: Artifact[]
    previewArtifact?: string | null
    runId?: string
    nodeId?: string
    /** When true, ArtifactPreview will not fetch (content already inlined). */
    inlineContent?: boolean
    /** Enable 取点 / 划选 / ⤴ 标注 on the current node's artifacts. */
    annotatable?: boolean
    remoteKind?: ReactStageRemoteKind
    shareEnabled?: boolean
    token?: string
    ports?: PublicPreviewPort[]
    publicActive?: boolean
    publicMobile?: boolean
  }>(),
  {
    previewArtifact: '',
    runId: '',
    nodeId: '',
    inlineContent: false,
    annotatable: false,
    shareEnabled: false,
    token: '',
    ports: () => [],
    publicActive: true,
    publicMobile: false,
  },
)

const emit = defineEmits<{
  pick: [payload: AppPreviewPickPayload]
  stagedPick: [payload: AppPreviewPickPayload | null]
  openShare: []
}>()

const { t } = useI18n()
const toast = useToast()
const parentAnnotate = useReviewAnnotate()

function stageAnnotation(ann: ReactAnnotation) {
  if (!props.annotatable) return
  if (parentAnnotate) {
    parentAnnotate.annotate(ann)
    return
  }
  if (!props.runId || !props.nodeId) return
  if (addClarifyAnnotation(props.runId, props.nodeId, ann) === 'duplicate') {
    toast.warn(t('pages.reviewComposer.alreadyAdded'))
  }
}

provideReviewAnnotate({
  get enabled() {
    if (!props.annotatable) return false
    if (parentAnnotate) return parentAnnotate.enabled
    return !!props.runId && !!props.nodeId
  },
  annotate: (ann) => stageAnnotation(ann),
})

const activeTab = ref(REACT_STAGE_TAB_GRID)
const openNames = ref<string[]>([])
const htmlThumbs = ref<Record<string, string>>({})
const novncOpen = ref(false)
const sandboxId = ref<number | null>(null)
const sandboxLoading = ref(false)
let htmlThumbGen = 0
let htmlThumbAbort: AbortController | null = null
const htmlThumbFp: Record<string, string> = {}

const resolvedRemoteKind = computed(() =>
  resolveStageRemoteKind({
    remoteKind: props.remoteKind,
    runId: props.runId,
    nodeId: props.nodeId,
    inlineContent: props.inlineContent,
  }),
)
const canOpenNovnc = computed(() => resolvedRemoteKind.value !== 'off')
const showingNovnc = computed(() => activeTab.value === REACT_STAGE_TAB_NOVNC)
const showGridCards = computed(() => props.artifacts.length > 0 || canOpenNovnc.value)
const remoteCardMeta = computed(() =>
  resolvedRemoteKind.value === 'sandbox'
    ? t('pages.reactArtifactStage.novncCardMeta')
    : t('pages.reactArtifactStage.appCardMeta'),
)

function artifactAnnotatable(a: Artifact | null): boolean {
  return canAnnotateStageArtifact(!!props.annotatable, a, props.nodeId)
}

function artifactReadonly(a: Artifact): boolean {
  return !isOwnNodeArtifact(a, props.nodeId)
}

const kindIcon: Record<string, string> = {
  html: 'dashboard',
  json: 'doc',
  markdown: 'doc',
  yaml: 'doc',
  image: 'artifact',
  text: 'doc',
}

function metaLine(a: Artifact): string {
  return t('pages.reactArtifactStage.meta', {
    kind: t(artifactKindLabelKey(a.kind)),
    n: artifactRevision(a),
    time: relTime(a.updatedAt || a.createdAt),
  })
}

function artifactByName(name: string): Artifact | null {
  return findArtifactByName(props.artifacts, name)
}

function activatePreview(name: string) {
  const art = findArtifactByName(props.artifacts, name)
  if (!art) return
  openNames.value = openStagePreviewTab(openNames.value, art.name)
  activeTab.value = previewTabId(art.name)
}

function openArtifact(a: Artifact) {
  activatePreview(a.name)
}

function closePreview(name: string) {
  const next = nextTabAfterClose(openNames.value, name, activeTab.value, novncOpen.value)
  openNames.value = closeStagePreviewTab(openNames.value, name)
  activeTab.value = next
}

function openNovnc() {
  if (!canOpenNovnc.value) return
  novncOpen.value = true
  activeTab.value = REACT_STAGE_TAB_NOVNC
}

function closeNovnc() {
  novncOpen.value = false
  if (openNames.value.length) {
    activeTab.value = previewTabId(openNames.value[openNames.value.length - 1])
    return
  }
  activeTab.value = REACT_STAGE_TAB_GRID
}

function onRemotePick(payload: AppPreviewPickPayload) {
  emit('pick', payload)
  stageAnnotation({
    selector: payload.selector,
    url: payload.url,
    label: payload.selector || payload.tagName,
  })
}

const showingGrid = computed(() => activeTab.value === REACT_STAGE_TAB_GRID)
const activePreviewName = computed(() => previewTabName(activeTab.value))

watch(
  () => [String(props.previewArtifact || '').trim(), props.artifacts.map((a) => a.name)] as const,
  ([pin, names], prev) => {
    const nameSet = new Set(names)
    const kept = openNames.value.filter((n) => nameSet.has(n))
    if (kept.length !== openNames.value.length) {
      const gone = openNames.value.find((n) => !nameSet.has(n))
      if (gone && previewTabName(activeTab.value) === gone) {
        activeTab.value = nextTabAfterClose(openNames.value, gone, activeTab.value, novncOpen.value)
      }
      openNames.value = kept
    }
    if (shouldActivatePinnedPreview(pin, names, prev?.[0], prev?.[1] !== undefined ? [...prev[1]] : undefined)) {
      activatePreview(pin)
    }
  },
  { immediate: true },
)

watch(
  () =>
    props.artifacts
      .filter((a) => a.kind === 'html')
      .map((a) => artifactFingerprint(a))
      .join('|'),
  async () => {
    const gen = ++htmlThumbGen
    htmlThumbAbort?.abort()
    const ac = new AbortController()
    htmlThumbAbort = ac
    const htmlArts = props.artifacts.filter((a) => a.kind === 'html')
    const next: Record<string, string> = {}
    for (const a of htmlArts) {
      const fp = artifactFingerprint(a)
      if (typeof a.content === 'string') {
        next[a.id] = a.content
        htmlThumbFp[a.id] = fp
        continue
      }
      if (props.inlineContent) continue
      if (htmlThumbFp[a.id] === fp && htmlThumbs.value[a.id] !== undefined) {
        next[a.id] = htmlThumbs.value[a.id]
        continue
      }
      try {
        const full = await api.artifactContent(a.id, { signal: ac.signal })
        if (gen !== htmlThumbGen) return
        next[a.id] = full.content ?? ''
        htmlThumbFp[a.id] = fp
      } catch (e) {
        if (isAbortError(e) || gen !== htmlThumbGen) return
        if (htmlThumbs.value[a.id] !== undefined) next[a.id] = htmlThumbs.value[a.id]
      }
    }
    if (gen !== htmlThumbGen) return
    for (const id of Object.keys(htmlThumbFp)) {
      if (!htmlArts.some((a) => a.id === id)) delete htmlThumbFp[id]
    }
    htmlThumbs.value = next
  },
  { immediate: true },
)

watch(
  () => resolvedRemoteKind.value,
  (kind) => {
    if (kind !== 'app' && kind !== 'public') return
    novncOpen.value = true
    if (activeTab.value === REACT_STAGE_TAB_GRID) activeTab.value = REACT_STAGE_TAB_NOVNC
  },
  { immediate: true },
)

watch(
  () => `${props.runId}|${props.nodeId}|${resolvedRemoteKind.value}`,
  async () => {
    sandboxId.value = null
    if (resolvedRemoteKind.value !== 'sandbox') return
    sandboxLoading.value = true
    try {
      const sbx = await api.getRunNodeSandbox(props.runId, props.nodeId)
      sandboxId.value = typeof sbx?.id === 'number' ? sbx.id : Number(sbx?.id) || null
    } catch (e) {
      if (isAbortError(e)) return
      sandboxId.value = null
    } finally {
      sandboxLoading.value = false
    }
  },
  { immediate: true },
)

onBeforeUnmount(() => {
  htmlThumbGen++
  htmlThumbAbort?.abort()
})
</script>

<template>
  <div class="flex h-full min-h-0 flex-col bg-base" data-testid="react-artifact-stage">
    <div
      class="flex shrink-0 items-center gap-1 overflow-x-auto border-b border-line px-2 py-1.5"
      data-testid="react-artifact-tabs"
      role="tablist"
    >
      <button
        type="button"
        role="tab"
        class="inline-flex max-w-[200px] items-center gap-1.5 rounded-md px-2.5 py-1 text-[12px] transition"
        :class="
          showingGrid
            ? 'bg-elevated text-txt'
            : 'text-txt3 hover:bg-elevated/60 hover:text-txt2'
        "
        :aria-selected="showingGrid ? 'true' : 'false'"
        data-testid="react-artifact-tab-grid"
        @click="activeTab = REACT_STAGE_TAB_GRID"
      >
        <Icon name="dashboard" :size="13" />
        <span class="truncate">{{ t('pages.reactArtifactStage.pipelineTab') }}</span>
      </button>
      <div
        v-for="name in openNames"
        :key="name"
        class="group inline-flex max-w-[220px] items-center gap-1 rounded-md text-[12px] transition"
        :class="
          activePreviewName === name
            ? 'bg-elevated text-txt'
            : 'text-txt3 hover:bg-elevated/60 hover:text-txt2'
        "
      >
        <button
          type="button"
          role="tab"
          class="inline-flex min-w-0 items-center gap-1.5 py-1 pl-2.5 pr-1"
          :aria-selected="activePreviewName === name ? 'true' : 'false'"
          :data-testid="'react-artifact-tab-' + name"
          @click="activeTab = previewTabId(name)"
        >
          <Icon :name="kindIcon[artifactByName(name)?.kind || ''] || 'doc'" :size="13" />
          <span class="truncate">{{ name }}</span>
        </button>
        <button
          type="button"
          class="mr-1 inline-flex h-5 w-5 shrink-0 items-center justify-center rounded text-txt3 hover:bg-overlay hover:text-txt"
          :aria-label="t('pages.reactArtifactStage.closeTab')"
          :title="t('pages.reactArtifactStage.closeTab')"
          :data-testid="'react-artifact-tab-close-' + name"
          @click.stop="closePreview(name)"
        >
          <Icon name="close" :size="12" />
        </button>
      </div>
      <div
        v-if="novncOpen"
        class="group inline-flex max-w-[220px] items-center gap-1 rounded-md text-[12px] transition"
        :class="
          showingNovnc
            ? 'bg-elevated text-txt'
            : 'text-txt3 hover:bg-elevated/60 hover:text-txt2'
        "
      >
        <button
          type="button"
          role="tab"
          class="inline-flex min-w-0 items-center gap-1.5 py-1 pl-2.5 pr-1"
          :aria-selected="showingNovnc ? 'true' : 'false'"
          data-testid="react-artifact-tab-novnc"
          @click="activeTab = REACT_STAGE_TAB_NOVNC"
        >
          <Icon name="globe" :size="13" />
          <span class="truncate">{{ t('pages.reactArtifactStage.novncTab') }}</span>
        </button>
        <button
          type="button"
          class="mr-1 inline-flex h-5 w-5 shrink-0 items-center justify-center rounded text-txt3 hover:bg-overlay hover:text-txt"
          :aria-label="t('pages.reactArtifactStage.closeTab')"
          :title="t('pages.reactArtifactStage.closeTab')"
          data-testid="react-artifact-tab-close-novnc"
          @click.stop="closeNovnc"
        >
          <Icon name="close" :size="12" />
        </button>
      </div>
    </div>

    <div v-show="showingGrid" class="min-h-0 flex-1 overflow-y-auto p-4" data-testid="react-artifact-grid">
      <div
        v-if="!showGridCards"
        class="flex h-full min-h-[160px] flex-col items-center justify-center text-center text-[12px] text-txt3"
        data-testid="react-artifact-grid-empty"
      >
        <Icon name="artifact" :size="26" class="mb-2 opacity-40" />
        {{ t('pages.reactArtifactStage.gridEmpty') }}
      </div>
      <div
        v-else
        class="grid grid-cols-[repeat(auto-fill,minmax(176px,1fr))] gap-3"
      >
        <button
          v-if="canOpenNovnc"
          type="button"
          class="overflow-hidden rounded-lg border border-line bg-surface text-left transition hover:border-line-strong"
          data-testid="react-artifact-card-novnc"
          @click="openNovnc"
        >
          <div class="relative flex h-[110px] items-center justify-center overflow-hidden bg-elevated text-txt3">
            <Icon name="globe" :size="28" class="opacity-50" />
          </div>
          <div class="px-2.5 py-2">
            <div class="truncate text-[12px] font-medium text-txt">{{ t('pages.reactArtifactStage.novncCardTitle') }}</div>
            <div class="mt-0.5 truncate text-[11px] text-txt3">{{ remoteCardMeta }}</div>
          </div>
        </button>
        <button
          v-for="a in artifacts"
          :key="a.id"
          type="button"
          class="overflow-hidden rounded-lg border border-line bg-surface text-left transition hover:border-line-strong"
          :data-testid="'react-artifact-card-' + a.name"
          @click="openArtifact(a)"
        >
          <div class="relative h-[110px] overflow-hidden bg-elevated">
            <HtmlPreview
              v-if="a.kind === 'html' && htmlThumbs[a.id]"
              :html="htmlThumbs[a.id]"
              mode="demo"
              :enlargeable="false"
              class="pointer-events-none h-full w-full"
            />
            <div v-else class="flex h-full items-center justify-center text-txt3">
              <Icon :name="kindIcon[a.kind] || 'artifact'" :size="28" class="opacity-50" />
            </div>
          </div>
          <div class="px-2.5 py-2">
            <div class="flex items-center gap-1.5">
              <div class="min-w-0 truncate text-[12px] font-medium text-txt" :title="a.name">{{ a.name }}</div>
              <span
                v-if="artifactReadonly(a)"
                class="shrink-0 rounded border border-line px-1 py-px text-[10px] text-txt3"
                data-testid="react-artifact-card-readonly"
              >{{ t('pages.reactArtifactStage.readonlyBadge') }}</span>
            </div>
            <div class="mt-0.5 truncate text-[11px] text-txt3">{{ metaLine(a) }}</div>
          </div>
        </button>
      </div>
    </div>

    <div
      v-for="name in openNames"
      v-show="activePreviewName === name"
      :key="name"
      class="flex min-h-0 flex-1 flex-col"
      :data-testid="'react-artifact-preview-' + name"
    >
      <ArtifactPreview
        v-if="artifactByName(name)"
        :artifact="artifactByName(name)"
        :artifacts="artifacts"
        :run-id="runId"
        hide-delete
        :annotatable="artifactAnnotatable(artifactByName(name))"
        class="min-h-0 flex-1"
      />
    </div>

    <div
      v-if="novncOpen"
      v-show="showingNovnc"
      class="flex min-h-0 flex-1 flex-col"
      data-testid="react-artifact-preview-novnc"
    >
      <AppPreviewPanel
        v-if="resolvedRemoteKind === 'app'"
        :run-id="runId"
        :node-id="nodeId"
        fill
        :show-feedback="false"
        :share-enabled="shareEnabled"
        @pick="onRemotePick"
        @staged-pick="emit('stagedPick', $event)"
        @open-share="emit('openShare')"
      />
      <PublicAppPreviewPanel
        v-else-if="resolvedRemoteKind === 'public'"
        :token="token"
        :ports="ports"
        :active="publicActive"
        :mobile="publicMobile"
        fill
        @pick="onRemotePick"
        @staged-pick="emit('stagedPick', $event)"
      />
      <NovncPreviewPanel
        v-else-if="sandboxId"
        :sandbox-id="sandboxId"
        fill
        :inspectable="annotatable"
        @pick="onRemotePick"
      />
      <div
        v-else-if="resolvedRemoteKind === 'sandbox'"
        class="flex h-full flex-col items-center justify-center p-6 text-center text-[12px] text-txt3"
        data-testid="react-artifact-novnc-missing"
      >
        <Icon name="globe" :size="26" class="mb-2 opacity-40" />
        {{ sandboxLoading ? t('pages.appPreview.loading') : t('pages.reactArtifactStage.novncMissing') }}
      </div>
    </div>
  </div>
</template>
