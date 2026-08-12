<script setup lang="ts">
/**
 * Run 详情「沙箱日志」面板壳：容器 docker logs UI。
 * 与 RunLogPanel 由入口同挂载（v-show），以保留 boot dwell。
 */
import Icon from '@/components/ui/Icon.vue'
import RefreshStrip from '@/components/run/RefreshStrip.vue'
import HardLoadLayer from '@/components/run/HardLoadLayer.vue'
import type { NodeRunStatus } from '@/lib/shared/types'
import type { SandboxView } from '@/lib/api/api'
import { useI18n } from 'vue-i18n'

defineProps<{
  loading: boolean
  sbxLog: { content: string; live: boolean; found: boolean; error?: string } | null
  selStatus?: NodeRunStatus | string | null
  sandboxLookup?: SandboxView | null
}>()

const emit = defineEmits<{
  refresh: []
  consoleClick: []
}>()

const { t } = useI18n()
</script>

<template>
  <div class="relative flex h-full min-h-0 flex-col">
    <RefreshStrip v-if="loading && sbxLog?.content" :message="t('pages.sandboxConsole.logRefreshing')" />
    <HardLoadLayer
      v-else-if="loading && !sbxLog?.content && !sbxLog?.error"
      :overlay="true"
      :stuck-after-ms="10_000"
      :stage="t('common.loading.label')"
      @retry="emit('refresh')"
    />
    <div class="flex items-center gap-2 border-b border-line px-3 py-1.5 text-[11px] text-txt3">
      <Icon name="terminal" :size="12" />
      <span>{{ t('pages.runDetail.sandboxLog.title') }}</span>
      <span
        v-if="sbxLog?.error"
        class="inline-flex items-center rounded-full border border-err/40 bg-err/10 px-2 py-0.5 text-[10px] text-err"
      >{{ t('pages.runDetail.sandboxLog.errorBadge') }}</span>
      <span
        v-else-if="sbxLog?.live"
        class="inline-flex items-center rounded-full border border-accent/40 bg-accent-dim px-2 py-0.5 text-[10px] text-accent"
      >{{ t('pages.runDetail.sandboxLog.live') }}</span>
      <span v-else-if="sbxLog?.found" class="chip">{{ t('pages.runDetail.sandboxLog.archived') }}</span>
      <div class="flex-1" />
      <button
        v-if="sandboxLookup"
        type="button"
        class="rounded border border-line px-2 py-1 text-[11px] text-txt2 hover:border-line-strong"
        @click="emit('consoleClick')"
      >
        <Icon name="terminal" :size="12" class="-mt-0.5 mr-0.5 inline" />{{ t('common.buttons.console') }}
      </button>
      <button class="text-txt3 hover:text-txt" :title="t('common.buttons.refresh')" @click="emit('refresh')"><Icon name="refresh" :size="12" /></button>
    </div>
    <div class="scroll-area min-h-0 flex-1 overflow-auto bg-base p-3">
      <div
        v-if="sbxLog?.error"
        class="mb-2 rounded-lg border border-err/30 bg-err/10 px-3 py-2.5 text-[12px] text-err"
        data-testid="sandbox-log-error"
        role="alert"
      >
        <strong class="mb-1 block">{{ t('pages.runDetail.sandboxLog.errorTitle') }}</strong>
        <span>{{ sbxLog.error }}</span>
        <button
          type="button"
          class="mt-2 inline-flex min-h-11 items-center border border-line px-3 text-[12px] text-txt"
          @click="emit('refresh')"
        >
          {{ t('common.chatImage.retry') }}
        </button>
      </div>
      <pre
        v-if="sbxLog?.found && sbxLog.content"
        class="min-w-max whitespace-pre font-mono text-[11px] leading-relaxed text-txt2"
      >{{ sbxLog.content }}</pre>
      <pre
        v-else-if="sbxLog?.found && sbxLog.live"
        class="whitespace-pre-wrap font-mono text-[11px] leading-relaxed text-txt3"
        data-testid="sandbox-log-live-empty"
      >{{ t('pages.runDetail.sandboxLog.liveEmpty') }}</pre>
      <div v-else class="flex h-full items-center justify-center text-center text-[12px] text-txt3">
        <div>
          <Icon name="terminal" :size="24" class="mx-auto mb-2 opacity-40" />
          <p>{{ selStatus === 'pending' ? t('pages.runDetail.sandboxLog.pending') : t('pages.runDetail.sandboxLog.empty') }}</p>
        </div>
      </div>
    </div>
  </div>
</template>
