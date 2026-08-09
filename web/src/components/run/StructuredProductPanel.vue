<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '../ui/Icon.vue'
import { api } from '@/lib/api'
import { nodeColorHex } from '@/data/nodeRegistry'
import { useNodeDefs } from '@/lib/useNodeDefs'
import StructuredArtifactView from './StructuredArtifactView.vue'
import HtmlPreview from '../ui/HtmlPreview.vue'
import SelectionAddToChat from './SelectionAddToChat.vue'
import UpstreamRequirementContext from './UpstreamRequirementContext.vue'
import { ARTIFACT_TO_OUTPUT_JSON } from '@/lib/structuredArtifacts'
import { productArtifactName } from '@/lib/productNodeArtifacts'
import { provideReviewAnnotate } from '@/lib/reviewAnnotate'
import { addClarifyAnnotation } from '@/lib/useClarifyDraft'
import { useToast } from '@/lib/useToast'
import type { NodeRun, WFNode, Run, ReactAnnotation } from '@/lib/types'

// annotatable enables the per-field "⤴ 标注" affordance in the review phase:
// each staged reference is pushed onto the node's review composer draft.
const props = withDefaults(defineProps<{ node: WFNode; nodeRun: NodeRun; run: Run; annotatable?: boolean }>(), {
  annotatable: false,
})

const { t } = useI18n()
const toast = useToast()
const stageEl = ref<HTMLElement | null>(null)

function stageAnnotation(ann: ReactAnnotation) {
  if (!props.annotatable) return
  const result = addClarifyAnnotation(props.run.id, props.node.id, ann)
  if (result === 'duplicate') toast.warn(t('pages.reviewComposer.alreadyAdded'))
}

// Provide the annotate channel to descendant product cards. It is inert unless
// annotatable is set, so read-only product views never show pick affordances.
// Use a getter so `enabled` tracks the prop if the panel is reused.
provideReviewAnnotate({
  get enabled() {
    return !!props.annotatable
  },
  annotate: (ann) => {
    stageAnnotation(ann)
  },
})

/** HTML toolbar pick → same annotations store as structured ⤴ chips (g3). */
function onHtmlPick(payload: { selector: string; tagName: string; imageDataUrl: string }) {
  if (!props.annotatable) return
  stageAnnotation({
    selector: payload.selector,
    label: payload.selector || payload.tagName,
  })
}

function onQuoteAdd(ann: ReactAnnotation) {
  stageAnnotation(ann)
}

const { NODE_DEFS } = useNodeDefs()

const spec = computed(() => {
  const name = productArtifactName(props.node.type)
  return name ? { name } : undefined
})
const hex = computed(() => nodeColorHex(props.node.type))
const def = computed(() => NODE_DEFS.value[props.node.type])

const artifact = computed(() => {
  if (!spec.value) return null
  return props.run.artifacts.find((a) => a.name === spec.value!.name) || null
})

const doc = ref<any>(null)
const rawHtml = ref('')
const isVisual = computed(() => props.node.type === 'visual')
const loading = ref(false)
function parseDoc(raw: string) {
  try {
    doc.value = JSON.parse(raw || '{}')
  } catch {
    doc.value = null
  }
}

/** In-flight / review pause: follow live page.html. Historical completed keep snap.
 *  annotatable only controls pick affordances — completed + annotatable must not
 *  bypass frozen snap (Clarify/Inbox shells always pass annotatable). */
const followLive = computed(
  () =>
    props.nodeRun.status === 'waiting_human' ||
    props.nodeRun.status === 'running' ||
    props.nodeRun.status === 'pending',
)

async function load() {
  const name = spec.value?.name
  if (!name) {
    doc.value = null
    rawHtml.value = ''
    return
  }
  // The visual node's product is raw HTML, not JSON. Prefer THIS execution's own
  // HTML snapshot (nodeRun.outputs.page): the run artifact store replaces the
  // same-named page.html on every iteration, so reading it would show the latest
  // page for every historical "第 N 次" tab. Fall back to the store only when the
  // snapshot is absent (older runs predating the snapshot).
  // Exception: review / in-flight (followLive) reads the live store first so
  // interactive write_artifact updates show without a full-node re-run.
  if (isVisual.value) {
    const priorHtml = rawHtml.value
    doc.value = null
    const snap = props.nodeRun.outputs?.page
    const a = artifact.value
    if (followLive.value && a) {
      loading.value = true
      try {
        const full = await api.artifactContent(a.id)
        rawHtml.value = full.content || (typeof snap === 'string' ? snap : '') || priorHtml || ''
      } catch {
        // Keep prior success / snap on fetch failure — never blank a good preview.
        rawHtml.value =
          (typeof snap === 'string' && snap.trim() ? snap : priorHtml) || ''
      } finally {
        loading.value = false
      }
      return
    }
    rawHtml.value = ''
    if (typeof snap === 'string' && snap.trim()) {
      rawHtml.value = snap
      return
    }
    if (!a) return
    loading.value = true
    try {
      const full = await api.artifactContent(a.id)
      rawHtml.value = full.content || ''
    } catch {
      rawHtml.value = ''
    } finally {
      loading.value = false
    }
    return
  }
  // Each execution persists its own JSON in nodeRun.outputs; the run artifact
  // store replaces same-named files, so historical tabs must not read it.
  const jsonKey = ARTIFACT_TO_OUTPUT_JSON[name]
  const snap = jsonKey ? props.nodeRun.outputs?.[jsonKey] : undefined
  if (typeof snap === 'string' && snap.trim()) {
    parseDoc(snap)
    return
  }
  const a = artifact.value
  if (!a) {
    doc.value = null
    return
  }
  loading.value = true
  try {
    // List DTO omits artifact content; fetch the full record by id. Re-fetched
    // when id/size changes (e.g. a plan node's plan.json growing mid-run).
    const full = await api.artifactContent(a.id)
    parseDoc(full.content || '{}')
  } catch {
    doc.value = null
  } finally {
    loading.value = false
  }
}
watch(
  () => {
    const name = spec.value?.name || ''
    const jsonKey = name ? ARTIFACT_TO_OUTPUT_JSON[name] : ''
    const snap = jsonKey ? props.nodeRun.outputs?.[jsonKey] : ''
    // The visual node keeps its per-iteration HTML under outputs.page; include a
    // fingerprint so switching execution tabs reloads the right page.
    const page = isVisual.value ? props.nodeRun.outputs?.page : ''
    const pageKey = typeof page === 'string' ? `${page.length}` : ''
    const a = artifact.value
    // followLive also keys on updatedAt so interactive writes reload even when
    // sizeBytes is unchanged (rare) or snap has not yet been synced.
    const liveKey = followLive.value
      ? `${a?.sizeBytes ?? ''}:${a?.updatedAt ?? ''}:${props.annotatable ? 1 : 0}:${props.nodeRun.status || ''}`
      : ''
    return `${props.nodeRun.iteration ?? 0}:${jsonKey}:${snap}:${pageKey}:${a ? `${a.id}:${a.sizeBytes}` : ''}:${liveKey}`
  },
  load,
  { immediate: true },
)

const pending = computed(() => props.nodeRun.status === 'pending')
</script>

<template>
  <div ref="stageEl" class="flex h-full min-h-0 flex-col" data-review-annotate-stage>
    <div
      class="flex shrink-0 items-center gap-2.5 px-4 pt-4 pb-3"
      data-testid="structured-product-header"
    >
      <div class="flex h-8 w-8 items-center justify-center rounded-md" :style="{ background: hex + '22', color: hex }">
        <Icon :name="def.icon" :size="16" />
      </div>
      <div class="min-w-0 flex-1">
        <div class="text-sm font-semibold text-txt">{{ def.label }}{{ t('pages.structuredProduct.titleSuffix') }}</div>
        <div class="text-[11px] text-txt3" data-testid="structured-product-name">{{ spec?.name }}</div>
      </div>
    </div>

    <div
      class="min-h-0 flex-1"
      :class="isVisual && rawHtml ? 'flex flex-col overflow-hidden px-4' : 'overflow-y-auto px-4 pb-4'"
      data-testid="structured-product-preview"
    >
      <div v-if="isVisual && rawHtml" class="min-h-0 flex-1 border border-line">
        <HtmlPreview
          :html="rawHtml"
          :inspectable="annotatable"
          fill-parent
          @pick="onHtmlPick"
        />
      </div>

      <StructuredArtifactView
        v-else-if="doc && spec"
        :name="spec.name"
        :doc="doc"
        :accent="hex"
        :run-id="run.id"
        :artifacts="run.artifacts"
        :run-status="run.status"
      />

      <div v-else class="flex h-[60%] items-center justify-center text-center text-[12px] text-txt3">
        <div>
          <Icon name="artifact" :size="26" class="mx-auto mb-2 opacity-40" />
          <p v-if="loading">{{ t('pages.structuredProduct.loading') }}</p>
          <p v-else-if="pending">{{ t('pages.structuredProduct.pending') }}</p>
          <p v-else-if="isVisual">{{ t('pages.structuredProduct.noPage') }}</p>
          <p v-else>{{ t('pages.structuredProduct.noStructured') }}</p>
        </div>
      </div>

      <SelectionAddToChat
        v-if="annotatable && !isVisual"
        :enabled="annotatable"
        :root="stageEl"
        @add="onQuoteAdd"
      />
    </div>

    <UpstreamRequirementContext
      variant="persistent-bar"
      disable-annotate
      :artifacts="run.artifacts"
      :run-id="run.id"
      :run-status="run.status"
      :product-name="spec?.name"
    />
  </div>
</template>
