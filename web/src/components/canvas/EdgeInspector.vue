<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '../ui/Icon.vue'
import type { WFEdge, EdgeKind } from '@/lib/shared/types'

const { t } = useI18n()

const props = defineProps<{ edge: WFEdge }>()
const emit = defineEmits<{ (e: 'delete'): void }>()

const KINDS = computed(() => [
  { value: 'success' as EdgeKind, label: t('pages.workflowEditor.edgeInspector.kinds.success.label'), desc: t('pages.workflowEditor.edgeInspector.kinds.success.desc'), tone: 'text-ok' },
  { value: 'failure' as EdgeKind, label: t('pages.workflowEditor.edgeInspector.kinds.failure.label'), desc: t('pages.workflowEditor.edgeInspector.kinds.failure.desc'), tone: 'text-err' },
  { value: 'rollback' as EdgeKind, label: t('pages.workflowEditor.edgeInspector.kinds.rollback.label'), desc: t('pages.workflowEditor.edgeInspector.kinds.rollback.desc'), tone: 'text-warn' },
])

const kind = computed<EdgeKind>({
  get: () => props.edge.kind || 'success',
  set: (v) => { props.edge.kind = v },
})

const carryText = computed<string>({
  get: () => (props.edge.carry || []).join(', '),
  set: (v) => { props.edge.carry = v.split(',').map((s) => s.trim()).filter(Boolean) },
})
</script>

<template>
  <div class="flex h-full flex-col">
    <div class="flex items-center gap-2.5 border-b border-line px-4 py-3">
      <div class="flex h-9 w-9 items-center justify-center rounded-md bg-accent-dim text-accent-2">
        <Icon name="branch" :size="18" />
      </div>
      <div class="min-w-0 flex-1">
        <div class="text-sm font-semibold text-txt">{{ t('pages.workflowEditor.edgeInspector.title') }}</div>
        <div class="truncate font-mono text-[11px] text-txt3">{{ edge.source }} → {{ edge.target }}</div>
      </div>
      <button class="flex h-8 w-8 items-center justify-center rounded-md text-txt3 hover:bg-err/10 hover:text-err" @click="emit('delete')">
        <Icon name="close" :size="16" />
      </button>
    </div>

    <div class="scroll-area flex-1 space-y-4 overflow-y-auto p-4">
      <div>
        <label class="label">{{ t('pages.workflowEditor.edgeInspector.transitionKind') }}</label>
        <div class="space-y-1.5">
          <label
            v-for="k in KINDS"
            :key="k.value"
            class="flex cursor-pointer items-start gap-2.5 rounded-md border px-3 py-2 transition"
            :class="kind === k.value ? 'border-accent bg-accent-dim' : 'border-line hover:border-line-strong'"
          >
            <input type="radio" class="mt-0.5" :value="k.value" v-model="kind" />
            <span class="min-w-0">
              <span class="text-[13px] font-medium" :class="k.tone">{{ k.label }}</span>
              <span class="block text-[11px] text-txt3">{{ k.desc }}</span>
            </span>
          </label>
        </div>
      </div>

      <div v-if="kind === 'success'">
        <label class="label">{{ t('pages.workflowEditor.edgeInspector.whenGuard') }}</label>
        <textarea v-model="edge.when" class="input min-h-[64px] font-mono text-[12px]" :placeholder="t('pages.workflowEditor.edgeInspector.whenPlaceholder')" />
        <p class="mt-1.5 text-[11px] text-txt3">{{ t('pages.workflowEditor.edgeInspector.whenHelp') }}</p>
      </div>

      <template v-if="kind !== 'success'">
        <div>
          <label class="label">{{ t('pages.workflowEditor.edgeInspector.carry') }}</label>
          <input v-model="carryText" class="input font-mono text-[12px]" :placeholder="t('pages.workflowEditor.edgeInspector.carryPlaceholder')" />
          <p class="mt-1.5 text-[11px] text-txt3">{{ t('pages.workflowEditor.edgeInspector.carryHelp') }}</p>
        </div>
        <div v-if="kind === 'rollback'">
          <label class="label">{{ t('pages.workflowEditor.edgeInspector.maxAttempts') }}</label>
          <input v-model.number="edge.maxAttempts" type="number" min="1" class="input" placeholder="3" />
          <p class="mt-1.5 text-[11px] text-txt3">{{ t('pages.workflowEditor.edgeInspector.maxAttemptsHelp') }}</p>
        </div>
      </template>

      <div>
        <label class="label">{{ t('pages.workflowEditor.edgeInspector.edgeLabel') }}</label>
        <input v-model="edge.label" class="input" :placeholder="t('pages.workflowEditor.edgeInspector.edgeLabelPlaceholder')" />
      </div>
    </div>
  </div>
</template>
