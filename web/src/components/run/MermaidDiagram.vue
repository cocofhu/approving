<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { api } from '@/lib/api/api'
import type { Artifact } from '@/lib/shared/types'

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

const format = computed(() => (props.diagram.format || 'mermaid').trim().toLowerCase() || 'mermaid')
const source = computed(() => (props.diagram.source || '').trim())
const caption = computed(() => (props.diagram.caption || '').trim())

const fallbackUrl = computed(() => {
  const name = (props.diagram.fallback_artifact || '').trim()
  if (!name || !props.artifacts?.length) return ''
  const hit = props.artifacts.find((a) => a.name === name)
  return hit?.id ? api.artifactDownloadUrl(hit.id) : ''
})

function themeVars(): Record<string, string> {
  const cs = getComputedStyle(document.documentElement)
  const pick = (name: string, fb: string) => (cs.getPropertyValue(name).trim() || fb)
  return {
    background: pick('--color-base', '#0d1117'),
    primaryColor: pick('--color-elevated', '#161b22'),
    primaryTextColor: pick('--color-txt', '#e6edf3'),
    primaryBorderColor: pick('--color-line', '#30363d'),
    lineColor: pick('--color-line-strong', '#8b949e'),
    secondaryColor: pick('--color-elevated', '#161b22'),
    tertiaryColor: pick('--color-base', '#0d1117'),
  }
}

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
      theme: 'dark',
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

onBeforeUnmount(() => {
  renderGen++
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
