<script setup lang="ts">
/**
 * Run 详情「执行日志」面板壳：复用 LiveLogPanel。
 * 与 RunSandboxPanel 由入口同挂载（v-show），以保留 boot dwell。
 */
import LiveLogPanel from '@/components/run/LiveLogPanel.vue'
import type { AcpEvent, McpCall, NodeRunStatus } from '@/lib/shared/types'
import type { LiveLogBootSession } from '@/lib/run/liveLogBootPhase'
import type { RehydrateStatus } from '@/lib/run/liveLogRehydrate'

defineProps<{
  events: AcpEvent[]
  live: boolean
  busy?: boolean
  status?: NodeRunStatus
  mcpCalls?: McpCall[]
  hasMore?: boolean
  showConsole?: boolean
  sandboxStatus?: string | null
  sandboxContainerStatus?: string | null
  bootSession?: LiveLogBootSession | null
  rehydrateStatus?: RehydrateStatus
}>()

const emit = defineEmits<{
  loadEarlier: []
  consoleClick: []
  goSandboxLog: []
  bootSession: [session: LiveLogBootSession]
  retryRehydrate: []
}>()
</script>

<template>
  <LiveLogPanel
    class="min-h-0 flex-1"
    :events="events"
    :live="live"
    :busy="busy"
    :status="status"
    :mcp-calls="mcpCalls"
    :has-more="hasMore"
    :show-console="showConsole"
    :sandbox-status="sandboxStatus"
    :sandbox-container-status="sandboxContainerStatus"
    :boot-session="bootSession"
    :rehydrate-status="rehydrateStatus"
    @load-earlier="emit('loadEarlier')"
    @console-click="emit('consoleClick')"
    @go-sandbox-log="emit('goSandboxLog')"
    @boot-session="emit('bootSession', $event)"
    @retry-rehydrate="emit('retryRehydrate')"
  />
</template>
