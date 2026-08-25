<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { api } from '@/lib/api/api'
import type { Artifact } from '@/lib/shared/types'
import { withMermaidSerial } from './mermaidRenderQueue'
import { mermaidThemeName, themeVars } from './mermaidTheme'

export type PlanDiagram = {
  format?: string
  source?: string
  fallback_artifact?: string
  caption?: string
}

const props = defineProps<{
  diagram: PlanDiagram
  jsonPath: string
  artifacts?: Artifact[]
}>()

const { t } = useI18n()
const host = ref<HTMLElement | null>(null)
const failed = ref(false)
const rendering = ref(false)
let renderGen = 0
let themeObserver: MutationObserver | null = null
let uidSeq = 0

const format = computed(() => (props.diagram.format || 'mermaid').trim().toLowerCase() || 'mermaid')
const source = computed(() => (props.diagram.source || '').trim())
const caption = computed(() => (props.diagram.caption || '').trim())

const fallbackUrl = computed(() => {
  const name = (props.diagram.fallback_artifact || '').trim()
  if (!name || !props.artifacts?.length) return ''
  const hit = props.artifacts.find((a) => a.name === name)
  return hit?.id ? api.artifactDownloadUrl(hit.id) : ''
})

function clearHost() {
  if (host.value) host.value.innerHTML = ''
}

async function render() {
  const gen = ++renderGen
  failed.value = false
  if (!source.value) {
    failed.value = true
    clearHost()
    return
  }
  if (format.value !== 'mermaid') {
    failed.value = true
    clearHost()
    return
  }
  rendering.value = true
  // Drop residual SVG before the next paint so a late sibling cannot leave mixed content.
  clearHost()
  try {
    const svg = await withMermaidSerial(async (mermaid) => {
      // Stale after enqueue: skip initialize/render so we do not burn the lock uselessly.
      if (gen !== renderGen) return null
      mermaid.initialize({
        startOnLoad: false,
        securityLevel: 'strict',
        theme: mermaidThemeName(),
        themeVariables: themeVars(),
      })
      const id = `plan-mmd-${Date.now()}-${gen}-${++uidSeq}`
      const out = await mermaid.render(id, source.value)
      return out.svg
    })
    if (gen !== renderGen) return
    if (svg == null) return
    await nextTick()
    if (gen !== renderGen) return
    if (host.value) host.value.innerHTML = svg
  } catch {
    if (gen === renderGen) {
      failed.value = true
      clearHost()
    }
  } finally {
    if (gen === renderGen) rendering.value = false
  }
}

watch(
  () => [source.value, format.value, props.diagram.fallback_artifact],
  () => {
    void render()
  },
  { immediate: true },
)

onMounted(() => {
  // Theme class toggles re-render every mounted instance concurrently — serial queue isolates them.
  themeObserver = new MutationObserver(() => {
    void render()
  })
  themeObserver.observe(document.documentElement, { attributes: true, attributeFilter: ['class'] })
})

onBeforeUnmount(() => {
  // Invalidate in-flight work so a finished render never writes into a destroyed host.
  renderGen++
  themeObserver?.disconnect()
  themeObserver = null
  clearHost()
})
</script>

<template>
  <div class="mt-2 space-y-1.5" data-testid="plan-diagram">
    <div v-show="!failed" ref="host" class="overflow-x-auto text-txt [&_svg]:max-w-full" />
    <div v-if="failed" class="space-y-2" data-testid="plan-diagram-fallback">
      <div class="text-[11px] text-txt2" data-testid="plan-diagram-fallback-hint">{{ t('pages.plan.diagramFallback') }}</div>
      <pre
        class="overflow-x-auto border border-line bg-base p-2 font-mono text-[11px] leading-relaxed text-txt2 whitespace-pre-wrap"
        data-testid="plan-diagram-fallback-source"
      >{{ source }}</pre>
      <img
        v-if="fallbackUrl"
        :src="fallbackUrl"
        :alt="caption || 'diagram fallback'"
        class="max-h-64 max-w-full border border-line object-contain"
        data-testid="plan-diagram-fallback-img"
      />
    </div>
    <div v-if="caption" class="text-center text-[11px] text-txt3">{{ caption }}</div>
    <div v-if="rendering && !failed" class="text-[11px] text-txt3">…</div>
  </div>
</template>
