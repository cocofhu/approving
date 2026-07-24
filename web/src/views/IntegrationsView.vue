<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/ui/Icon.vue'
import AppModal from '@/components/ui/AppModal.vue'
import { BUILTIN_MCPS, type McpServer } from '@/data/mcp'

const { t } = useI18n()

const selected = ref<McpServer | null>(null)

const isProjectScope = computed(() => selected.value?.scope === 'project')
const isAgentScope = computed(() => selected.value?.scope === 'agent')
const isRunScope = computed(() => selected.value?.scope === 'run')

function openDetail(m: McpServer) {
  selected.value = m
}

function closeDetail() {
  selected.value = null
}

function cardIcon(m: McpServer): string {
  if (m.scope === 'project') return 'user'
  if (m.scope === 'agent') return 'clock'
  return 'connector'
}

function scopeLabelKey(scope: McpServer['scope']): string {
  if (scope === 'project') return 'mcp.integrations.scopeProject'
  if (scope === 'agent') return 'mcp.integrations.scopeAgent'
  return 'mcp.integrations.scopeRun'
}

function availabilityBadgeKey(scope: McpServer['scope']): string {
  if (scope === 'project') return 'mcp.integrations.seenInPmLeader'
  if (scope === 'agent') return 'mcp.integrations.seenInAgentStudio'
  return 'mcp.integrations.alwaysAvailable'
}

function availabilityBadgeClass(scope: McpServer['scope']): string {
  if (scope === 'project') return 'border-info/30 bg-info/10 text-info'
  if (scope === 'agent') return 'border-warn/30 bg-warn/10 text-warn'
  return 'border-ok/30 bg-ok/10 text-ok'
}

function scopeChipClass(scope: McpServer['scope']): string {
  if (scope === 'project') return 'border-warn/30 text-warn'
  if (scope === 'agent') return 'border-ok/30 text-ok'
  return 'border-info/30 text-info'
}

function cardIconClass(scope: McpServer['scope']): string {
  if (scope === 'project') return 'bg-info/10 text-info'
  if (scope === 'agent') return 'bg-ok/10 text-ok'
  return 'bg-accent-dim text-accent-2'
}
</script>

<template>
  <div>
    <div class="mb-5">
      <h2 class="text-lg font-semibold text-txt">{{ t('mcp.integrations.title') }}</h2>
      <p class="text-sm text-txt3">{{ t('mcp.integrations.subtitle') }}</p>
    </div>

    <div class="mb-2 flex items-center gap-2 text-xs font-semibold uppercase tracking-wider text-txt3">
      {{ t('mcp.integrations.sectionTitle') }}
      <span class="chip border-accent/30 text-accent-2">{{ t('mcp.integrations.noConfigNeeded') }}</span>
    </div>

    <button
      v-for="m in BUILTIN_MCPS"
      :key="m.id"
      type="button"
      class="card mb-3 w-full p-4 text-left transition-colors hover:border-line-strong hover:bg-elevated"
      @click="openDetail(m)"
    >
      <div class="flex items-start gap-3">
        <div
          class="flex h-10 w-10 shrink-0 items-center justify-center rounded-md"
          :class="cardIconClass(m.scope)"
        >
          <Icon :name="cardIcon(m)" :size="20" />
        </div>
        <div class="min-w-0 flex-1">
          <div class="flex flex-wrap items-center gap-2">
            <span class="font-medium text-txt">{{ m.name }}</span>
            <span class="chip">MCP</span>
            <span class="chip" :class="scopeChipClass(m.scope)">{{ t(scopeLabelKey(m.scope)) }}</span>
          </div>
          <div class="mt-0.5 text-[12px] text-txt3">{{ t(m.descKey) }}</div>
        </div>
        <span
          class="inline-flex shrink-0 items-center gap-1 rounded-full border px-2 py-0.5 text-[11px]"
          :class="availabilityBadgeClass(m.scope)"
        >
          <Icon :name="m.scope === 'run' ? 'check' : 'clock'" :size="11" />
          {{ t(availabilityBadgeKey(m.scope)) }}
        </span>
      </div>
      <div class="mt-3 flex items-center justify-between border-t border-line pt-3 text-[12px] text-txt3">
        <span>{{ t('mcp.integrations.toolCount', { n: m.tools.length }) }}</span>
        <span class="inline-flex items-center gap-1 text-accent-2">
          {{ t('mcp.integrations.openDetail') }}
          <Icon name="chevron-right" :size="14" />
        </span>
      </div>
    </button>

    <AppModal
      :open="!!selected"
      :title="selected?.name"
      :width="800"
      @close="closeDetail"
    >
      <div v-if="selected" class="space-y-5">
        <div>
          <div class="flex flex-wrap items-center gap-2">
            <span class="font-medium text-txt">{{ selected.name }}</span>
            <span class="chip">MCP</span>
            <span v-if="!isRunScope" class="chip border-accent/30 text-accent-2">{{ selected.id }}</span>
          </div>
          <p class="mt-2 text-[13px] leading-6 text-txt2">
            {{ t(selected.overviewKey || selected.descKey) }}
          </p>
          <div class="mt-2.5 flex flex-wrap gap-1.5">
            <span class="chip" :class="scopeChipClass(selected.scope)">{{ t(scopeLabelKey(selected.scope)) }}</span>
            <template v-if="isProjectScope">
              <span class="chip border-info/30 text-info">{{ t('mcp.integrations.notRunScoped') }}</span>
              <span class="chip border-accent/30 text-accent-2">{{ t('mcp.integrations.noConfigHere') }}</span>
            </template>
            <template v-else-if="isAgentScope">
              <span class="chip border-info/30 text-info">{{ t('mcp.integrations.notRunScoped') }}</span>
              <span class="chip border-accent/30 text-accent-2">{{ t('mcp.integrations.addViaAgentStudio') }}</span>
            </template>
            <template v-else>
              <span class="chip border-ok/30 bg-ok/10 text-ok">{{ t('mcp.integrations.alwaysAvailable') }}</span>
              <span class="chip border-accent/30 text-accent-2">{{ t('mcp.integrations.noConfigNeeded') }}</span>
            </template>
          </div>
          <p
            v-if="isProjectScope"
            class="mt-3 text-[12px] leading-5 text-txt3"
          >
            {{ t('mcp.integrations.entryPathLabel') }}
            <span class="pointer-events-none cursor-default border-b border-dotted border-line-strong font-medium text-txt">
              {{ t('mcp.integrations.entryPath') }}
            </span>
          </p>
          <p
            v-else-if="isAgentScope"
            class="mt-3 text-[12px] leading-5 text-txt3"
          >
            {{ t('mcp.integrations.entryPathLabel') }}
            <span class="pointer-events-none cursor-default border-b border-dotted border-line-strong font-medium text-txt">
              {{ t('mcp.integrations.entryPathAgent') }}
            </span>
          </p>
        </div>

        <p
          v-if="selected.conventionKey"
          class="flex gap-1.5 rounded-md border border-warn/30 bg-warn/10 p-2.5 text-[11px] leading-5 text-warn"
        >
          <Icon name="alert" :size="13" class="mt-0.5 shrink-0" />
          {{ t(selected.conventionKey) }}
        </p>

        <div>
          <div class="mb-2.5 flex items-baseline justify-between">
            <h3 class="text-xs font-semibold uppercase tracking-wider text-txt3">
              {{ t('mcp.integrations.toolsTitle') }}
            </h3>
            <span class="text-[11px] text-txt3">{{ selected.tools.length }}</span>
          </div>
          <div class="space-y-2">
            <div
              v-for="tool in selected.tools"
              :key="tool.name"
              class="rounded-md border border-line bg-base px-3 py-2.5"
            >
              <div class="flex flex-wrap items-center gap-2">
                <code
                  class="rounded px-1.5 py-0.5 font-mono text-[10px]"
                  :class="tool.io === 'write' ? 'bg-ok/10 text-ok' : 'bg-info/10 text-info'"
                >{{ t(`mcp.integrations.io.${tool.io}`) }}</code>
                <code class="font-mono text-[12px] text-txt">{{ tool.name }}</code>
              </div>
              <div class="mt-1.5 text-[11px] leading-5 text-txt3">{{ t(tool.descKey) }}</div>
              <div class="mt-2 break-all rounded border border-line bg-elevated px-2 py-1.5 font-mono text-[11px] leading-4 text-accent-2">
                {{ t(tool.signatureKey) }}
              </div>
            </div>
          </div>
        </div>
      </div>
    </AppModal>
  </div>
</template>
