<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { api, authApi, type PlatformRuleMeta } from '@/lib/api'
import { useAuth } from '@/lib/useAuth'
import { createListRequestSeq, httpStatusOf } from '@/lib/listRequestSeq'
import AppButton from '@/components/ui/AppButton.vue'
import Icon from '@/components/ui/Icon.vue'
import MarkdownSplitEditor from '@/components/agent/MarkdownSplitEditor.vue'

const { t } = useI18n()
const router = useRouter()
const { user } = useAuth()

const items = ref<PlatformRuleMeta[]>([])
const activeFile = ref('')
const content = ref('')
const source = ref<'global' | 'embed'>('global')
const loading = ref(true)
const hasInitialLoaded = ref(false)
const loadFailed = ref(false)
const loadDenied = ref(false)
const saving = ref(false)
const resetting = ref(false)
const error = ref('')
const savedAt = ref(0)
/** Shared seq for list load + file select so refresh and click-file cannot cross-write. */
const rulesSeq = createListRequestSeq()
const showRefreshProgress = computed(
  () => (loading.value || resetting.value) && hasInitialLoaded.value && items.value.length > 0,
)
const showSkeleton = computed(
  () => loading.value && items.value.length === 0 && !loadFailed.value && !loadDenied.value,
)

const isAdmin = computed(() => !!user.value?.isAdmin)
const canWrite = computed(() => isAdmin.value)

async function ensureAuth() {
  if (user.value) return
  try {
    const me = await authApi.me()
    const { setUser } = useAuth()
    setUser({ username: me.username, expiresAt: me.expires_at, isAdmin: !!me.is_admin })
  } catch {
    /* router guard handles redirect */
  }
}

function ruleSource(src: string | undefined): 'global' | 'embed' {
  return src === 'embed' ? 'embed' : 'global'
}

/** Fetch only — callers must check isCurrentSeq before writing refs. */
async function fetchRuleList() {
  const res = await api.listPlatformRules()
  return res.items
}

async function fetchRuleFile(file: string) {
  const item = await api.getPlatformRule(file)
  return { content: item.content, source: ruleSource(item.source) }
}

async function loadAll() {
  const localSeq = rulesSeq.beginListRequest()
  loading.value = true
  error.value = ''
  loadFailed.value = false
  loadDenied.value = false
  try {
    await ensureAuth()
    const list = await fetchRuleList()
    if (!rulesSeq.isCurrentSeq(localSeq)) return
    items.value = list
    let file = activeFile.value
    if (!file && list.length) {
      file = list[0].file
      activeFile.value = file
    }
    if (file) {
      const data = await fetchRuleFile(file)
      if (!rulesSeq.isCurrentSeq(localSeq)) return
      content.value = data.content
      source.value = data.source
    }
  } catch (e: any) {
    if (!rulesSeq.isCurrentSeq(localSeq)) return
    if (items.value.length > 0) {
      error.value = e?.message || t('pages.platformRules.loadFailed')
      return
    }
    const status = httpStatusOf(e)
    loadDenied.value = status === 403
    loadFailed.value = status !== 403
    error.value = e?.message || t('pages.platformRules.loadFailed')
  } finally {
    if (!rulesSeq.isCurrentSeq(localSeq)) return
    loading.value = false
    hasInitialLoaded.value = true
  }
}

async function selectFile(file: string) {
  const localSeq = rulesSeq.beginListRequest()
  activeFile.value = file
  error.value = ''
  try {
    const data = await fetchRuleFile(file)
    if (!rulesSeq.isCurrentSeq(localSeq)) return
    content.value = data.content
    source.value = data.source
  } catch (e: any) {
    if (!rulesSeq.isCurrentSeq(localSeq)) return
    error.value = e?.message || t('pages.platformRules.loadFailed')
  }
}

async function save() {
  if (!canWrite.value || !activeFile.value) return
  saving.value = true
  error.value = ''
  try {
    const item = await api.savePlatformRule(activeFile.value, content.value)
    content.value = item.content
    source.value = 'global'
    const list = await fetchRuleList()
    items.value = list
    savedAt.value = Date.now()
  } catch (e: any) {
    error.value = e?.message || t('pages.platformRules.saveFailed')
  } finally {
    saving.value = false
  }
}

async function resetToEmbed() {
  if (!canWrite.value || !activeFile.value) return
  resetting.value = true
  error.value = ''
  try {
    const item = await api.resetPlatformRule(activeFile.value)
    content.value = item.content
    source.value = 'global'
    const list = await fetchRuleList()
    items.value = list
    savedAt.value = Date.now()
  } catch (e: any) {
    error.value = e?.message || t('pages.platformRules.resetFailed')
  } finally {
    resetting.value = false
  }
}

function sourceLabel(item: PlatformRuleMeta): string {
  return item.source === 'embed' ? t('pages.platformRules.sourceEmbed') : t('pages.platformRules.sourceGlobal')
}

onMounted(loadAll)
</script>

<template>
  <div class="flex h-full min-h-0 flex-col" data-testid="platform-rules-panel" :aria-busy="loading || saving || resetting ? 'true' : 'false'">
    <div class="mb-4 flex items-end justify-between gap-3">
      <div>
        <div class="mb-1 flex items-center gap-2">
          <button class="text-xs text-txt3 hover:text-txt2" @click="router.push('/settings')">
            ← {{ t('pages.platformRules.backToSettings') }}
          </button>
        </div>
        <h2 class="text-lg font-semibold text-txt">{{ t('pages.platformRules.title') }}</h2>
        <p class="text-sm text-txt3">{{ t('pages.platformRules.subtitle') }}</p>
      </div>
      <div class="flex items-center gap-2">
        <span v-if="!canWrite" class="text-xs text-warn">{{ t('pages.platformRules.readOnlyHint') }}</span>
        <span v-if="savedAt" class="text-xs text-ok">{{ t('pages.settings.saved') }}</span>
        <AppButton
          variant="ghost"
          size="sm"
          icon="refresh"
          :disabled="loading || saving || resetting || !canWrite"
          @click="resetToEmbed"
        >
          {{ resetting ? t('common.buttons.saving') : t('pages.platformRules.resetEmbed') }}
        </AppButton>
        <AppButton
          variant="primary"
          size="sm"
          icon="check"
          :disabled="loading || saving || resetting || !canWrite"
          @click="save"
        >
          {{ saving ? t('common.buttons.saving') : t('common.buttons.save') }}
        </AppButton>
      </div>
    </div>

    <div
      v-if="error && items.length && !loadFailed && !loadDenied"
      class="mb-3 rounded-lg border border-err/30 bg-err/10 px-3 py-2 text-sm text-err"
    >{{ error }}</div>

    <div
      v-if="showRefreshProgress"
      class="mb-2 h-[2px] overflow-hidden bg-line"
      data-testid="platform-rules-thin-progress"
      aria-hidden="true"
    >
      <i class="admin-list-thin-bar bg-accent" />
    </div>

    <div
      v-if="showSkeleton"
      class="card grid min-h-0 flex-1 grid-cols-[240px_1fr_280px] overflow-hidden"
      data-testid="platform-rules-skeleton"
      aria-hidden="true"
    >
      <aside class="border-r border-line p-3">
        <div class="mb-3 h-3 w-24 bg-elevated animate-pulse" />
        <div v-for="n in 6" :key="'pr-file-' + n" class="mb-1.5 h-8 bg-elevated animate-pulse" />
      </aside>
      <section class="space-y-3 p-4">
        <div class="h-4 w-48 bg-elevated animate-pulse" />
        <div class="h-48 w-full bg-elevated animate-pulse" />
      </section>
      <aside class="border-l border-line p-3">
        <div class="h-3 w-20 bg-elevated animate-pulse" />
        <div class="mt-3 h-24 w-full bg-elevated animate-pulse" />
      </aside>
    </div>

    <div
      v-else-if="loadDenied"
      role="status"
      data-testid="platform-rules-denied"
      class="border border-warn/40 bg-warn/10 px-5 py-10 text-center"
    >
      <Icon name="lock" :size="22" class="mx-auto mb-3 text-warn" />
      <h3 class="text-sm font-semibold text-txt">{{ t('common.asyncState.permissionDeniedTitle') }}</h3>
      <p class="mt-1 text-xs text-txt2">{{ t('common.asyncState.permissionDeniedDesc') }}</p>
      <AppButton class="mt-4" variant="outline" data-testid="platform-rules-retry" @click="loadAll">
        {{ t('common.buttons.retry') }}
      </AppButton>
    </div>

    <div
      v-else-if="loadFailed"
      role="status"
      data-testid="platform-rules-failed"
      class="border border-err/40 bg-err/10 px-5 py-10 text-center"
    >
      <h3 class="text-sm font-semibold text-txt">{{ t('common.asyncState.loadFailedTitle') }}</h3>
      <p class="mt-1 text-xs text-txt2">{{ t('common.asyncState.loadFailedDesc') }}</p>
      <AppButton class="mt-4" variant="outline" data-testid="platform-rules-retry" @click="loadAll">
        {{ t('common.buttons.retry') }}
      </AppButton>
    </div>

    <div v-else class="card grid min-h-0 flex-1 grid-cols-[240px_1fr_280px] overflow-hidden" :class="showRefreshProgress ? 'opacity-[0.55]' : ''">
      <aside class="flex min-h-0 flex-col border-r border-line bg-base/30">
        <div class="border-b border-line px-3 py-3">
          <h3 class="text-[13px] font-semibold text-txt">{{ t('pages.platformRules.fileListTitle') }}</h3>
          <p class="mt-1 text-[11px] leading-relaxed text-txt3">{{ t('pages.platformRules.fileListDesc') }}</p>
        </div>
        <div class="scroll-area flex-1 overflow-y-auto p-2">
          <button
            v-for="item in items"
            :key="item.file"
            class="mb-0.5 flex w-full items-center gap-2 rounded px-2 py-1.5 text-left text-[12px] transition"
            :class="activeFile === item.file ? 'bg-accent-dim text-txt' : 'text-txt3 hover:bg-elevated hover:text-txt2'"
            @click="selectFile(item.file)"
          >
            <Icon name="file" :size="14" class="shrink-0 opacity-70" />
            <span class="min-w-0 flex-1 truncate font-mono text-[11px]">{{ item.file }}</span>
            <span
              class="shrink-0 rounded border px-1.5 py-0.5 text-[10px]"
              :class="item.source === 'embed' ? 'border-info/30 bg-info/10 text-info' : 'border-info/30 bg-info/10 text-info'"
            >
              {{ sourceLabel(item) }}
            </span>
          </button>
        </div>
      </aside>

      <section class="flex min-h-0 min-w-0 flex-col">
        <div class="flex items-center gap-2 border-b border-line px-4 py-2">
          <span class="font-mono text-[12px] text-txt2">data/platform-rules/{{ activeFile }}</span>
          <span class="rounded border border-info/30 bg-info/10 px-2 py-0.5 text-[10px] text-info">
            {{ source === 'embed' ? t('pages.platformRules.sourceEmbed') : t('pages.platformRules.sourceGlobal') }}
          </span>
        </div>
        <div class="min-h-0 flex-1">
          <MarkdownSplitEditor
            v-if="activeFile"
            v-model="content"
            :file-path="`rules/${activeFile}`"
            :readonly="!canWrite"
          />
        </div>
      </section>

      <aside class="scroll-area min-h-0 overflow-y-auto border-l border-line bg-base/20">
        <div class="border-b border-line px-3 py-3">
          <h4 class="text-[11px] font-semibold uppercase tracking-wider text-txt3">{{ t('pages.platformRules.priorityTitle') }}</h4>
          <ol class="mt-3 space-y-3 text-[12px]">
            <li class="flex gap-2">
              <span class="flex h-5 w-5 shrink-0 items-center justify-center border border-line text-[10px] text-txt3">1</span>
              <div>
                <div class="font-medium text-txt">{{ t('pages.platformRules.priorityAgent') }}</div>
                <div class="font-mono text-[10px] text-txt3">profiles/&lt;agent&gt;/platform-rules/</div>
              </div>
            </li>
            <li class="flex gap-2">
              <span class="flex h-5 w-5 shrink-0 items-center justify-center border border-accent/40 bg-accent-dim text-[10px] text-accent-2">2</span>
              <div>
                <div class="font-medium text-accent-2">{{ t('pages.platformRules.priorityGlobal') }}</div>
                <div class="font-mono text-[10px] text-txt3">data/platform-rules/</div>
              </div>
            </li>
            <li class="flex gap-2">
              <span class="flex h-5 w-5 shrink-0 items-center justify-center border border-line text-[10px] text-txt3">3</span>
              <div>
                <div class="font-medium text-txt">{{ t('pages.platformRules.priorityEmbed') }}</div>
                <div class="font-mono text-[10px] text-txt3">go:embed skills_embed/</div>
              </div>
            </li>
          </ol>
        </div>
        <div class="border-b border-line px-3 py-3">
          <h4 class="text-[11px] font-semibold uppercase tracking-wider text-txt3">{{ t('pages.platformRules.injectTitle') }}</h4>
          <div class="mt-2 space-y-2 text-[11px] text-txt2">
            <div class="border border-line bg-base px-2 py-1.5">1. {{ t('pages.platformRules.injectAgentDir') }}</div>
            <div class="text-center text-txt3">↓</div>
            <div class="border border-accent/30 bg-accent-dim px-2 py-1.5 text-txt">2. {{ t('pages.platformRules.injectPlatformRules') }}</div>
          </div>
        </div>
        <div class="px-3 py-3">
          <h4 class="text-[11px] font-semibold uppercase tracking-wider text-txt3">{{ t('pages.platformRules.constraintsTitle') }}</h4>
          <ul class="mt-2 space-y-2 text-[11px] leading-relaxed text-txt2">
            <li>{{ t('pages.platformRules.constraintNodereg') }}</li>
            <li>{{ t('pages.platformRules.constraintWholeFile') }}</li>
            <li>{{ t('pages.platformRules.constraintScope') }}</li>
          </ul>
        </div>
      </aside>
    </div>
  </div>
</template>
