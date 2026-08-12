<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/ui/Icon.vue'
import AppButton from '@/components/ui/AppButton.vue'
import MarkdownSplitEditor from '@/components/agent/MarkdownSplitEditor.vue'
import { api, type PlatformRuleMeta } from '@/lib/api/api'

const props = defineProps<{ agentName: string; active?: boolean }>()
const emit = defineEmits<{ toast: [msg: string] }>()

const { t } = useI18n()

const platformRuleItems = ref<PlatformRuleMeta[]>([])
const platformRuleFile = ref('')
const platformRuleContent = ref('')
const platformRuleLoading = ref(false)
const platformRuleSaving = ref(false)
const platformRuleError = ref('')

const activePlatformRule = computed(() => platformRuleItems.value.find((x) => x.file === platformRuleFile.value))
const platformRuleOverridden = computed(() => activePlatformRule.value?.source === 'override')
const platformRuleOverrideCount = computed(() => platformRuleItems.value.filter((x) => x.source === 'override').length)

async function loadPlatformRules() {
  if (!props.agentName) return
  platformRuleLoading.value = true
  platformRuleError.value = ''
  try {
    const res = await api.listAgentPlatformRules(props.agentName)
    platformRuleItems.value = res.items
    if (!platformRuleFile.value && res.items.length) {
      platformRuleFile.value = res.items[0].file
    }
    if (platformRuleFile.value) {
      const item = await api.getAgentPlatformRule(props.agentName, platformRuleFile.value)
      platformRuleContent.value = item.content
    }
  } catch (e: any) {
    platformRuleError.value = e?.message || t('pages.agentStudio.platformRules.loadFailed')
  } finally {
    platformRuleLoading.value = false
  }
}

async function selectPlatformRuleFile(file: string) {
  if (!props.agentName) return
  platformRuleFile.value = file
  platformRuleError.value = ''
  try {
    const item = await api.getAgentPlatformRule(props.agentName, file)
    platformRuleContent.value = item.content
  } catch (e: any) {
    platformRuleError.value = e?.message || t('pages.agentStudio.platformRules.loadFailed')
  }
}

async function createPlatformRuleOverride() {
  if (!props.agentName || !platformRuleFile.value) return
  platformRuleSaving.value = true
  platformRuleError.value = ''
  try {
    const item = await api.saveAgentPlatformRule(props.agentName, platformRuleFile.value, platformRuleContent.value)
    platformRuleContent.value = item.content
    await loadPlatformRules()
    emit('toast', t('pages.agentStudio.platformRules.overrideCreated'))
  } catch (e: any) {
    platformRuleError.value = e?.message || t('pages.agentStudio.platformRules.saveFailed')
  } finally {
    platformRuleSaving.value = false
  }
}

async function savePlatformRuleOverride() {
  if (!props.agentName || !platformRuleFile.value || !platformRuleOverridden.value) return
  platformRuleSaving.value = true
  platformRuleError.value = ''
  try {
    const item = await api.saveAgentPlatformRule(props.agentName, platformRuleFile.value, platformRuleContent.value)
    platformRuleContent.value = item.content
    await loadPlatformRules()
    emit('toast', t('pages.agentStudio.platformRules.saved'))
  } catch (e: any) {
    platformRuleError.value = e?.message || t('pages.agentStudio.platformRules.saveFailed')
  } finally {
    platformRuleSaving.value = false
  }
}

async function deletePlatformRuleOverride() {
  if (!props.agentName || !platformRuleFile.value || !platformRuleOverridden.value) return
  platformRuleSaving.value = true
  platformRuleError.value = ''
  try {
    await api.deleteAgentPlatformRule(props.agentName, platformRuleFile.value)
    await loadPlatformRules()
    if (platformRuleFile.value) {
      const item = await api.getAgentPlatformRule(props.agentName, platformRuleFile.value)
      platformRuleContent.value = item.content
    }
    emit('toast', t('pages.agentStudio.platformRules.overrideDeleted'))
  } catch (e: any) {
    platformRuleError.value = e?.message || t('pages.agentStudio.platformRules.deleteFailed')
  } finally {
    platformRuleSaving.value = false
  }
}

function resetState() {
  platformRuleFile.value = ''
  platformRuleItems.value = []
  platformRuleContent.value = ''
  platformRuleError.value = ''
}

watch(
  () => props.agentName,
  () => {
    resetState()
    if (props.active) void loadPlatformRules()
  },
)

watch(
  () => props.active,
  (v) => {
    if (v) void loadPlatformRules()
  },
  { immediate: true },
)

defineExpose({ load: loadPlatformRules, resetState })
</script>

<template>
  <div class="grid min-h-0 flex-1 grid-cols-[240px_1fr_260px] overflow-hidden">
    <aside class="flex min-h-0 flex-col border-r border-line bg-base/30">
      <div class="border-b border-line px-3 py-3">
        <h3 class="text-[13px] font-semibold text-txt">{{ t('pages.agentStudio.platformRules.title') }}</h3>
        <p class="mt-1 text-[11px] leading-relaxed text-txt3">{{ t('pages.agentStudio.platformRules.subtitle', { agent: agentName }) }}</p>
      </div>
      <div v-if="platformRuleLoading" class="p-4 text-xs text-txt3">{{ t('common.buttons.loading') }}</div>
      <div v-else class="scroll-area flex-1 overflow-y-auto p-2">
        <button
          v-for="item in platformRuleItems"
          :key="item.file"
          class="mb-0.5 flex w-full items-center gap-2 rounded px-2 py-1.5 text-left text-[12px] transition"
          :class="platformRuleFile === item.file ? 'bg-accent-dim text-txt' : 'text-txt3 hover:bg-elevated hover:text-txt2'"
          @click="selectPlatformRuleFile(item.file)"
        >
          <Icon name="file" :size="14" class="shrink-0 opacity-70" />
          <span class="min-w-0 flex-1 truncate font-mono text-[11px]">{{ item.file }}</span>
          <span
            class="shrink-0 rounded border px-1.5 py-0.5 text-[10px]"
            :class="item.source === 'override' ? 'border-warn/30 bg-warn/10 text-warn' : 'border-info/30 bg-info/10 text-info'"
          >
            {{ item.source === 'override' ? t('pages.agentStudio.platformRules.overridden') : t('pages.agentStudio.platformRules.inherited') }}
          </span>
        </button>
      </div>
    </aside>

    <section class="flex min-h-0 min-w-0 flex-col">
      <div class="flex items-center justify-between gap-2 border-b border-line px-4 py-2">
        <div class="flex min-w-0 items-center gap-2">
          <span class="truncate font-mono text-[12px] text-txt2">
            {{ platformRuleOverridden ? `profiles/${agentName}/platform-rules/${platformRuleFile}` : t('pages.agentStudio.platformRules.inheritPath', { file: platformRuleFile }) }}
          </span>
          <span
            class="shrink-0 rounded border px-2 py-0.5 text-[10px]"
            :class="platformRuleOverridden ? 'border-warn/30 bg-warn/10 text-warn' : 'border-info/30 bg-info/10 text-info'"
          >
            {{ platformRuleOverridden ? t('pages.agentStudio.platformRules.overridden') : t('pages.agentStudio.platformRules.inherited') }}
          </span>
        </div>
        <div class="flex shrink-0 items-center gap-2">
          <AppButton
            v-if="!platformRuleOverridden"
            variant="primary"
            size="sm"
            icon="plus"
            :disabled="platformRuleSaving || !platformRuleFile"
            @click="createPlatformRuleOverride"
          >
            {{ t('pages.agentStudio.platformRules.createOverride') }}
          </AppButton>
          <template v-else>
            <AppButton variant="ghost" size="sm" icon="trash" :disabled="platformRuleSaving" @click="deletePlatformRuleOverride">
              {{ t('pages.agentStudio.platformRules.deleteOverride') }}
            </AppButton>
            <AppButton variant="primary" size="sm" icon="check" :disabled="platformRuleSaving" @click="savePlatformRuleOverride">
              {{ platformRuleSaving ? t('common.buttons.saving') : t('common.buttons.save') }}
            </AppButton>
          </template>
        </div>
      </div>
      <div v-if="platformRuleError" class="border-b border-err/30 bg-err/10 px-4 py-2 text-[12px] text-err">{{ platformRuleError }}</div>
      <div class="min-h-0 flex-1">
        <MarkdownSplitEditor
          v-if="platformRuleFile"
          v-model="platformRuleContent"
          :file-path="`rules/${platformRuleFile}`"
          :readonly="!platformRuleOverridden"
        />
      </div>
    </section>

    <aside class="scroll-area min-h-0 overflow-y-auto border-l border-line bg-base/20 p-3">
      <h4 class="text-[11px] font-semibold uppercase tracking-wider text-txt3">{{ t('pages.agentStudio.platformRules.statusTitle') }}</h4>
      <div class="mt-2 flex flex-wrap gap-2 text-[10px]">
        <span class="rounded border border-warn/30 bg-warn/10 px-2 py-1 text-warn">
          {{ t('pages.agentStudio.platformRules.overriddenCount', { n: platformRuleOverrideCount }) }}
        </span>
        <span class="rounded border border-info/30 bg-info/10 px-2 py-1 text-info">
          {{ t('pages.agentStudio.platformRules.inheritedCount', { n: platformRuleItems.length - platformRuleOverrideCount }) }}
        </span>
      </div>
      <p v-if="platformRuleOverridden" class="mt-3 text-[11px] leading-relaxed text-warn">
        {{ t('pages.agentStudio.platformRules.diffHint') }}
      </p>
      <p class="mt-3 text-[11px] leading-relaxed text-txt3">{{ t('pages.agentStudio.platformRules.promptsNote') }}</p>
    </aside>
  </div>
</template>
