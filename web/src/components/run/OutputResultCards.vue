<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { api } from '@/lib/api'
import OutputResultCardBody from './OutputResultCardBody.vue'
import AppModal from '../ui/AppModal.vue'
import Icon from '../ui/Icon.vue'
import type { OutputCard, Run } from '@/lib/types'

const props = defineProps<{ cards: OutputCard[]; run: Run }>()

const { t } = useI18n()

/** Master-detail: always exactly one selected card. Default 0; never -1. */
const selectedIndex = ref(0)
const enlargeOpen = ref(false)

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
    selectedIndex.value = 0
    enlargeOpen.value = false
  },
)

watch(
  [selectedIndex, () => props.cards],
  () => {
    const c = props.cards[selectedIndex.value]
    if (c?.typeTag === '自定义产物' && c.artifactName && !c.structuredArtifactName) {
      void loadArtifactContent(c.artifactName)
    }
  },
  { immediate: true },
)

function selectCard(i: number) {
  if (enlargeOpen.value) return
  if (i < 0 || i >= props.cards.length) return
  if (i === selectedIndex.value) return
  selectedIndex.value = i
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

const showList = computed(() => props.cards.length > 1)
const currentCard = computed(() => props.cards[selectedIndex.value])
const currentDoc = computed(() => (currentCard.value ? parseDoc(currentCard.value) : null))

/** F3: enlarge only for success cards that actually have renderable body (review v2). */
function hasRenderableBody(card: OutputCard | undefined): boolean {
  if (!card || card.status === 'failed') return false
  if (card.typeTag === '结构化产物') {
    if (card.structuredArtifactName && parseDoc(card) != null) return true
    return !!card.markdown?.trim()
  }
  if (card.typeTag === '自定义产物') return !!card.artifactName
  if (card.typeTag === 'Markdown') return !!card.markdown?.trim()
  return false
}

const canEnlarge = computed(() => hasRenderableBody(currentCard.value))

function artifactContent(name: string): string {
  return contentCache.value[name] ?? ''
}

function isHtmlArtifact(name?: string): boolean {
  return !!name && name.endsWith('.html')
}

/** Short list label: failed → 失败; else map typeTag (+ .html → HTML). */
function shortKindLabel(card: OutputCard): string {
  if (card.status === 'failed') return t('pages.nodeOutput.outputCards.kindFailed')
  if (card.typeTag === '结构化产物') return t('pages.nodeOutput.outputCards.kindStructured')
  if (card.typeTag === 'Markdown') return t('pages.nodeOutput.outputCards.kindMarkdown')
  if (card.typeTag === '自定义产物' && isHtmlArtifact(card.artifactName)) {
    return t('pages.nodeOutput.outputCards.kindHtml')
  }
  if (card.typeTag === '自定义产物') return t('pages.nodeOutput.outputCards.kindCustom')
  return card.typeTag
}

function openEnlarge() {
  if (!canEnlarge.value) return
  enlargeOpen.value = true
}

function closeEnlarge() {
  enlargeOpen.value = false
}
</script>

<template>
  <div class="min-w-0" data-testid="output-result-cards">
    <!-- Multi-card name+status list (g1.2 / g3.2): no max-height / own overflow. -->
    <div
      v-if="showList"
      class="mb-3 border border-line bg-base"
      role="listbox"
      :aria-label="t('pages.nodeOutput.outputCards.listTitle')"
      data-testid="output-result-list"
    >
      <div
        class="flex items-center justify-between border-b border-line px-2.5 py-1.5 text-[10px] font-bold uppercase tracking-wider text-txt3"
        data-testid="output-result-list-header"
      >
        <span>{{ t('pages.nodeOutput.outputCards.listTitle') }}</span>
        <b class="text-[11px] font-semibold normal-case tracking-normal text-txt2">{{ cards.length }}</b>
      </div>
      <button
        v-for="(card, i) in cards"
        :key="card.index + ':' + card.template"
        type="button"
        role="option"
        class="flex h-8 w-full items-center gap-2 border-l-2 border-solid px-2.5 text-left"
        :class="[
          i > 0 ? 'border-t border-t-line' : '',
          card.status === 'failed' ? 'text-err' : 'text-txt2',
          i === selectedIndex
            ? card.status === 'failed'
              ? 'border-l-err bg-err/10 text-err'
              : 'border-l-accent bg-accent-dim text-txt'
            : 'border-l-transparent hover:bg-elevated hover:text-txt',
        ]"
        :aria-selected="i === selectedIndex ? 'true' : 'false'"
        :data-testid="`output-result-card-toggle-${i}`"
        @click="selectCard(i)"
      >
        <Icon
          :name="card.status === 'failed' ? 'alert' : 'check'"
          :size="12"
          class="shrink-0"
          :class="card.status === 'failed' ? 'text-err' : 'text-ok'"
        />
        <span class="min-w-0 flex-1 truncate text-[12px]" :title="card.title">{{ card.title }}</span>
        <span
          class="shrink-0 text-[10px]"
          :class="card.status === 'failed' ? 'text-err' : 'text-txt3'"
          :data-testid="`output-result-list-kind-${i}`"
        >{{ shortKindLabel(card) }}</span>
      </button>
    </div>

    <template v-if="currentCard">
      <!-- Sticky detail bar: NodeOutputPanel padding moved inward so top-0 is flush (g3.1). -->
      <div
        class="sticky top-0 z-[2] -mx-4 mb-2.5 flex items-center gap-2 border-b border-line bg-surface px-4 py-2"
        data-testid="output-result-detail-bar"
      >
        <span
          class="min-w-0 flex-1 truncate text-[12px] font-semibold"
          :class="currentCard.status === 'failed' ? 'text-err' : 'text-txt'"
        >{{ currentCard.title }}</span>
        <span class="shrink-0 border border-line px-1.5 py-0.5 text-[10px] text-txt3">{{ currentCard.typeTag }}</span>
        <button
          v-if="canEnlarge"
          type="button"
          class="inline-flex shrink-0 items-center bg-accent px-2.5 py-1.5 text-[12px] font-medium text-white hover:bg-accent-2"
          data-testid="output-result-enlarge"
          @click="openEnlarge"
        >
          {{ t('pages.nodeOutput.outputCards.enlarge') }}
        </button>
      </div>

      <div :data-testid="`output-result-card-body-${selectedIndex}`">
        <OutputResultCardBody
          :card="currentCard"
          :run="run"
          :doc="currentDoc"
          :loading="loading"
          :artifact-html="currentCard.artifactName ? artifactContent(currentCard.artifactName) : ''"
          variant="detail"
        />
      </div>
    </template>

    <AppModal
      :open="enlargeOpen"
      :title="currentCard?.title || ''"
      :width="960"
      :close-on-esc="true"
      @close="closeEnlarge"
    >
      <div v-if="currentCard && canEnlarge" data-testid="output-result-enlarge-body">
        <OutputResultCardBody
          :card="currentCard"
          :run="run"
          :doc="currentDoc"
          :loading="loading"
          :artifact-html="currentCard.artifactName ? artifactContent(currentCard.artifactName) : ''"
          variant="enlarge"
        />
      </div>
    </AppModal>
  </div>
</template>
