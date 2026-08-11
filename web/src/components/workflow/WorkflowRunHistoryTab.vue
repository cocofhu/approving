<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import StatusPill from '@/components/ui/StatusPill.vue'
import { api, isPaginated } from '@/lib/api/api'
import { fmtTime, fmtDuration, formatTrigger } from '@/lib/shared/format'
import type { Run } from '@/lib/shared/types'

const SKELETON_ROWS = 5

const props = defineProps<{ workflowId: string }>()
const router = useRouter()
const { t } = useI18n()
const runs = ref<Run[]>([])
const initialLoading = ref(false)
let timer: number | undefined

function runIdShort(id: string) {
  return id.replace('run-', '')
}

async function load() {
  try {
    const data = await api.listRuns({ wf: props.workflowId })
    runs.value = isPaginated(data) ? data.items : data
  } catch {
    /* keep previous list */
  }
}

onMounted(async () => {
  initialLoading.value = true
  await load()
  initialLoading.value = false
  timer = window.setInterval(load, 3000)
})

onUnmounted(() => {
  if (timer) window.clearInterval(timer)
})
</script>

<template>
  <div class="flex h-full flex-col overflow-hidden bg-base">
    <div class="card m-4 flex-1 overflow-hidden">
      <table class="w-full text-sm">
        <thead>
          <tr class="text-left text-[11px] uppercase tracking-wider text-txt3">
            <th class="px-5 py-2.5 font-medium">{{ t('common.table.run') }}</th>
            <th class="px-5 py-2.5 font-medium">{{ t('common.table.trigger') }}</th>
            <th class="px-5 py-2.5 font-medium">{{ t('common.table.startTime') }}</th>
            <th class="px-5 py-2.5 font-medium">{{ t('common.table.duration') }}</th>
            <th class="px-5 py-2.5 font-medium">{{ t('common.table.status') }}</th>
          </tr>
        </thead>
        <tbody>
          <template v-if="initialLoading">
            <tr v-for="n in SKELETON_ROWS" :key="'skel-' + n" class="border-t border-line">
              <td class="px-5 py-3">
                <div class="h-3.5 w-[80px] rounded bg-elevated animate-pulse" />
              </td>
              <td class="px-5 py-3">
                <div class="h-3 w-[50%] rounded bg-elevated animate-pulse" />
              </td>
              <td class="px-5 py-3">
                <div class="h-3 w-[72px] rounded bg-elevated animate-pulse" />
              </td>
              <td class="px-5 py-3">
                <div class="h-3 w-[40%] rounded bg-elevated animate-pulse" />
              </td>
              <td class="px-5 py-3">
                <div class="h-3.5 w-14 rounded bg-elevated animate-pulse" />
              </td>
            </tr>
          </template>
          <tr v-else-if="!runs.length">
            <td colspan="5" class="px-5 py-10 text-center text-[13px] text-txt3">
              {{ t('common.empty.noRuns') }}
            </td>
          </tr>
          <template v-else>
            <tr
              v-for="r in runs"
              :key="r.id"
              class="cursor-pointer border-t border-line transition hover:bg-elevated"
              @click="router.push('/runs/' + r.id)"
            >
              <td class="px-5 py-3 font-mono text-[13px] text-txt2">#{{ runIdShort(r.id) }}</td>
              <td class="px-5 py-3 text-txt2">{{ formatTrigger(r.trigger) }}</td>
              <td class="px-5 py-3 text-txt2">{{ fmtTime(r.startedAt || r.createdAt || '') }}</td>
              <td class="px-5 py-3 text-txt2">{{ fmtDuration(r.durationSec) }}</td>
              <td class="px-5 py-3"><StatusPill :status="r.status" size="sm" /></td>
            </tr>
          </template>
        </tbody>
      </table>
    </div>
  </div>
</template>

<style scoped>
.table-loading {
  opacity: 0.55;
  pointer-events: none;
}
</style>
