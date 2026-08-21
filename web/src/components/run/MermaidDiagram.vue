<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { api } from '@/lib/api/api'
import type { Artifact } from '@/lib/shared/types'
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

const format = computed(() => (props.diagram.format || 'mermaid').trim().toLowerCase() || 'mermaid')
const source = computed(() => (props.diagram.source || '').trim())
const caption = computed(() => (props.diagram.caption || '').trim())

const fallbackUrl = computed(() => {
  const name = (props.diagram.fallback_artifact || '').trim()
  if (!name || !props.artifacts?.length) return ''
  const hit = props.artifacts.find((a) => a.name === name)
  return hit?.id ? api.artifactDownloadUrl(hit.id) : ''
})

async function render() {
  const gen = ++renderGen
  failed.value = false
  if (!source.value) {
    failed.value = true
    return
  }
  if (format.value !== 'mermaid') {
    failed.value = true
    return
  }
  rendering.value = true
  try {
    const mod = await import('mermaid')
    if (gen !== renderGen) return
    const mermaid = mod.default
    mermaid.initialize({
      startOnLoad: false,
      securityLevel: 'strict',
      theme: mermaidThemeName(),
      themeVariables: themeVars(),
    })
    const id = `plan-mmd-${Date.now()}-${gen}`
    const { svg } = await mermaid.render(id, source.value)
    if (gen !== renderGen) return
    await nextTick()
    if (host.value) host.value.innerHTML = svg
  } catch {
    if (gen === renderGen) {
      failed.value = true
      if (host.value) host.value.innerHTML = ''
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
  themeObserver = new MutationObserver(() => {
    void render()
  })
  themeObserver.observe(document.documentElement, { attributes: true, attributeFilter: ['class'] })
})

onBeforeUnmount(() => {
  renderGen++
  themeObserver?.disconnect()
  themeObserver = null
})
</script>

<template>
  <div class="mt-2 space-y-1.5" data-testid="plan-diagram">
    <div v-show="!failed" ref="host" class="overflow-x-auto text-txt [&_svg]:max-w-full" />
    <div v-if="failed" class="space-y-2" data-testid="plan-diagram-fallback">
      <div class="text-[11px] text-txt3">{{ t('pages.plan.diagramFallback') }}</div>
      <pre class="overflow-x-auto border border-line bg-base p-2 font-mono text-[11px] leading-relaxed text-txt2 whitespace-pre-wrap">{{ source }}</pre>
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
