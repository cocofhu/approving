<script setup lang="ts">
import { computed } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import AppDrawer from '@/components/ui/AppDrawer.vue'
import AppButton from '@/components/ui/AppButton.vue'
import StatusPill from '@/components/ui/StatusPill.vue'
import PriorityBadge from '@/components/ui/PriorityBadge.vue'
import { fmtDuration, fmtTime } from '@/lib/shared/format'
import { runBoardTitle, runIdShort } from '@/lib/run/runBoard'
import type { Run } from '@/lib/shared/types'

const props = defineProps<{ open: boolean; run: Run | null }>()
const emit = defineEmits<{ (e: 'close'): void }>()
const router = useRouter()
const { t } = useI18n()

const title = computed(() => (props.run ? runBoardTitle(props.run) : ''))
const shortId = computed(() => (props.run ? runIdShort(props.run.id) : ''))

function goDetail() {
  if (!props.run) return
  const id = props.run.id
  emit('close')
  router.push('/runs/' + id)
}
</script>

<template>
  <AppDrawer :open="open" :title="t('pages.board.preview.title')" :width="420" @close="emit('close')">
    <div v-if="run" class="p-5">
      <div class="mb-4 flex flex-wrap items-center gap-2">
        <StatusPill :status="run.status" size="sm" />
        <PriorityBadge :priority="run.priority" />
      </div>
      <h3 class="mb-4 text-base font-semibold leading-snug text-txt">{{ title }}</h3>
      <dl class="grid grid-cols-[96px_minmax(0,1fr)] gap-x-3 gap-y-2.5 text-[13px]">
        <dt class="text-txt3">{{ t('pages.board.preview.runId') }}</dt>
        <dd class="break-words text-txt">#{{ shortId }}</dd>
        <dt class="text-txt3">{{ t('common.table.status') }}</dt>
        <dd class="text-txt">{{ t(`common.status.${run.status}`) }}</dd>
        <dt class="text-txt3">{{ t('common.table.priority') }}</dt>
        <dd class="text-txt">{{ t(`common.priority.${run.priority || 'normal'}`) }}</dd>
        <dt class="text-txt3">{{ t('common.table.workflow') }}</dt>
        <dd class="break-words text-txt">{{ run.workflowName || '—' }}</dd>
        <dt class="text-txt3">{{ t('pages.board.preview.currentNode') }}</dt>
        <dd class="break-words text-txt">{{ run.currentNodeLabel || '—' }}</dd>
        <dt class="text-txt3">{{ t('pages.board.preview.progress') }}</dt>
        <dd class="text-txt">{{ run.progress != null ? `${run.progress}%` : '—' }}</dd>
        <dt class="text-txt3">{{ t('common.table.start') }}</dt>
        <dd class="text-txt">{{ fmtTime(run.startedAt) }}</dd>
        <dt class="text-txt3">{{ t('pages.board.preview.duration') }}</dt>
        <dd class="text-txt">{{ fmtDuration(run.durationSec) }}</dd>
      </dl>
      <p class="mt-5 text-xs leading-relaxed text-txt3">{{ t('pages.board.preview.hint') }}</p>
    </div>
    <template #footer>
      <div class="flex justify-end gap-2">
        <AppButton variant="outline" size="sm" @click="emit('close')">
          {{ t('pages.board.preview.dismiss') }}
        </AppButton>
        <AppButton variant="primary" size="sm" :disabled="!run" @click="goDetail">
          {{ t('pages.board.preview.openDetail') }}
        </AppButton>
      </div>
    </template>
  </AppDrawer>
</template>
