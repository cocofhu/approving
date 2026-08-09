<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { renderMarkdown } from '@/lib/markdown'
import { api } from '@/lib/api'
import StructuredArtifactView from './StructuredArtifactView.vue'
import HtmlPreview from '../ui/HtmlPreview.vue'
import type { OutputCard, Run } from '@/lib/types'

const props = defineProps<{ cards: OutputCard[]; run: Run }>()

const { t } = useI18n()

/** Exclusive accordion: 0 = first card; -1 = all collapsed. */
const openIndex = ref(0)

const contentCache = ref<Record<string, string>>({})
const loading = ref(false)

async function loadArtifactContent(name: string) {
  if (contentCache.value[name] !== undefined) return
  const art = props.run.artifacts.find((a) => a.name === name)
  if (!art) {
    contentCache.value[name] = ''
    return
  }
  loading.value = true
  try {
    const full = await api.artifactContent(art.id)
    contentCache.value[name] = full.content ?? ''
  } catch {
    contentCache.value[name] = ''
  } finally {
    loading.value = false
  }
}

watch(
  () => props.cards.map((c) => `${c.index}:${c.template}`).join(','),
  () => {
    openIndex.value = props.cards.length ? 0 : -1
  },
)

watch(
  [openIndex, () => props.cards],
  () => {
    const i = openIndex.value
    if (i < 0) return
    const c = props.cards[i]
    if (c?.typeTag === '自定义产物' && c.artifactName && !c.structuredArtifactName) {
      void loadArtifactContent(c.artifactName)
    }
  },
  { immediate: true },
)

function toggleCard(i: number) {
  openIndex.value = openIndex.value === i ? -1 : i
}

function isOpen(i: number) {
  return openIndex.value === i
}

function parseDoc(card: OutputCard): unknown {
  const raw = card.jsonSnapshot?.trim()
  if (!raw) return null
  try {
    return JSON.parse(raw)
  } catch {
    return null
  }
}

const cardsWithDoc = computed(() =>
  props.cards.map((c) => ({ card: c, doc: parseDoc(c) })),
)

function artifactContent(name: string): string {
  return contentCache.value[name] ?? ''
}

function isHtmlArtifact(name?: string): boolean {
  return !!name && name.endsWith('.html')
}
</script>

<template>
  <div class="space-y-3" data-testid="output-result-cards">
    <article
      v-for="({ card, doc }, i) in cardsWithDoc"
      :key="card.index + ':' + card.template"
      class="card overflow-hidden"
      :class="card.status === 'failed' ? 'border-err/30' : ''"
      :data-testid="`output-result-card-${i}`"
    >
      <h3 class="m-0">
        <button
          type="button"
          class="flex w-full items-center gap-2 border-b border-line bg-elevated px-3 py-2.5 text-left"
          :class="card.status === 'failed' ? 'border-err/25 bg-err/10' : ''"
          :aria-expanded="isOpen(i) ? 'true' : 'false'"
          :data-testid="`output-result-card-toggle-${i}`"
          @click="toggleCard(i)"
        >
          <span class="bg-accent-dim px-1.5 py-0.5 text-[10px] font-semibold text-accent-2">{{ card.index }}</span>
          <span class="flex-1 truncate text-[12px] font-semibold" :class="card.status === 'failed' ? 'text-err' : 'text-txt'">
            {{ card.title }}
          </span>
          <span class="border border-line px-1.5 py-0.5 text-[10px] text-txt3">{{ card.typeTag }}</span>
        </button>
      </h3>
      <div v-if="isOpen(i)" class="p-3" :data-testid="`output-result-card-body-${i}`">
        <template v-if="card.status === 'failed'">
          <p class="text-[12px] text-txt2">
            <strong class="text-err">{{ t('pages.nodeOutput.outputCards.sourceFailedTitle') }}</strong><br />
            {{ card.errorReason || t('pages.nodeOutput.outputCards.invalidSource') }}
          </p>
        </template>
        <template v-else-if="card.typeTag === '结构化产物' && card.structuredArtifactName && doc">
          <StructuredArtifactView
            :name="card.structuredArtifactName"
            :doc="doc"
            :artifacts="run.artifacts"
            :run-id="run.id"
            :run-status="run.status"
          />
        </template>
        <template v-else-if="card.typeTag === '结构化产物' && card.markdown">
          <div class="md text-[12px] leading-relaxed text-txt2" v-html="renderMarkdown(card.markdown)" />
        </template>
        <template v-else-if="card.typeTag === '自定义产物' && card.artifactName">
          <div v-if="loading && !artifactContent(card.artifactName)" class="text-[12px] text-txt3">…</div>
          <HtmlPreview
            v-else-if="isHtmlArtifact(card.artifactName)"
            :html="artifactContent(card.artifactName) || card.markdown || ''"
          />
          <pre
            v-else
            class="scroll-area max-h-48 overflow-y-auto whitespace-pre-wrap border border-line bg-base p-2.5 font-mono text-[11px] leading-relaxed text-txt2"
          >{{ artifactContent(card.artifactName) }}</pre>
        </template>
        <template v-else-if="card.typeTag === 'Markdown' && card.markdown">
          <div class="md text-[12px] leading-relaxed text-txt2" v-html="renderMarkdown(card.markdown)" />
        </template>
        <template v-else>
          <p class="text-[12px] text-txt3">{{ t('pages.nodeOutput.outputCards.invalidSource') }}</p>
        </template>
      </div>
    </article>
  </div>
</template>
