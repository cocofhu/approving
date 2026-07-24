<script setup lang="ts">
import { ref, watch, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import AppButton from '@/components/ui/AppButton.vue'
import AppModal from '@/components/ui/AppModal.vue'
import { api } from '@/lib/api'
import { buildEnvelope, downloadJson, sanitizeFilename, type WorkflowGraphPayload } from '@/lib/workflowIO'
import { useToast } from '@/lib/useToast'
import type { Workflow, WorkflowVersion } from '@/lib/types'

const props = defineProps<{
  open: boolean
  workflowId: string
  workflowName: string
  description: string
  needsRepo: boolean
  status: Workflow['status']
  /** When set (editor), draft export uses this graph including unsaved edits. */
  localDraft?: WorkflowGraphPayload | null
}>()

const emit = defineEmits<{ (e: 'close'): void }>()

const { t } = useI18n()
const toast = useToast()

const versions = ref<WorkflowVersion[]>([])
const loading = ref(false)
const exporting = ref(false)
const selected = ref<string>('draft')

watch(
  () => props.open,
  async (open) => {
    if (!open) return
    selected.value = 'draft'
    versions.value = []
    if (props.status === 'published' && props.workflowId) {
      loading.value = true
      try {
        versions.value = await api.listWorkflowVersions(props.workflowId)
      } catch {
        versions.value = []
      } finally {
        loading.value = false
      }
    }
  },
  { immediate: true },
)

const options = computed(() => {
  const opts: { id: string; label: string; desc?: string }[] = [
    {
      id: 'draft',
      label: t('pages.workflowIO.export.currentDraft'),
      desc: props.localDraft
        ? t('pages.workflowIO.export.currentDraftEditorDesc')
        : t('pages.workflowIO.export.currentDraftDesc'),
    },
  ]
  for (const v of versions.value) {
    opts.push({
      id: `v${v.version}`,
      label: t('pages.workflowIO.export.publishedVersion', { n: v.version }),
      desc: t('pages.workflowIO.export.publishedVersionDesc'),
    })
  }
  return opts
})

async function confirmExport() {
  if (exporting.value) return
  exporting.value = true
  try {
    const meta = { name: props.workflowName, description: props.description, needsRepo: props.needsRepo }
    let graph: WorkflowGraphPayload

    if (selected.value === 'draft') {
      if (props.localDraft) {
        graph = props.localDraft
      } else {
        const wf = await api.getWorkflow(props.workflowId)
        graph = { nodes: wf.nodes, edges: wf.edges, variables: (wf as any).variables }
      }
    } else {
      const version = parseInt(selected.value.slice(1), 10)
      graph = await api.getWorkflowVersionGraph(props.workflowId, version)
    }

    const envelope = buildEnvelope(meta, graph)
    downloadJson(sanitizeFilename(meta.name), envelope)
    toast.success(t('pages.workflowIO.export.success', { name: meta.name }))
    emit('close')
  } catch (e: any) {
    toast.error(String(e?.message || e))
  } finally {
    exporting.value = false
  }
}
</script>

<template>
  <AppModal :open="open" :title="t('pages.workflowIO.export.title', { name: workflowName })" :width="440" @close="emit('close')">
    <div class="space-y-3">
      <p class="text-[12px] leading-relaxed text-txt3">{{ t('pages.workflowIO.export.intro') }}</p>
      <div v-if="loading" class="py-6 text-center text-sm text-txt3">{{ t('common.buttons.loading') }}</div>
      <div v-else class="space-y-2">
        <label
          v-for="opt in options"
          :key="opt.id"
          class="flex cursor-pointer items-start gap-3 rounded-md border px-3 py-2.5 transition"
          :class="selected === opt.id ? 'border-accent/50 bg-accent-dim/40' : 'border-line bg-base/40 hover:border-line-strong'"
        >
          <input v-model="selected" type="radio" :value="opt.id" class="mt-1 accent-accent" />
          <div class="min-w-0 flex-1">
            <div class="text-[13px] font-medium text-txt">{{ opt.label }}</div>
            <div v-if="opt.desc" class="mt-0.5 text-[11px] text-txt3">{{ opt.desc }}</div>
          </div>
        </label>
      </div>
    </div>
    <template #footer>
      <AppButton variant="ghost" @click="emit('close')">{{ t('common.buttons.cancel') }}</AppButton>
      <AppButton variant="primary" icon="download" :disabled="exporting || loading" @click="confirmExport">
        {{ exporting ? t('common.buttons.loading') : t('pages.workflowIO.export.confirm') }}
      </AppButton>
    </template>
  </AppModal>
</template>
