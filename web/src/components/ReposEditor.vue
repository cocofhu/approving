<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import Icon from '@/components/ui/Icon.vue'
import AppButton from '@/components/ui/AppButton.vue'

export type RepoRow = { name: string; url: string; branch: string }

const props = withDefaults(
  defineProps<{
    repos: RepoRow[]
    minRows?: number
    editable?: boolean
    i18nPrefix?: string
  }>(),
  {
    minRows: 0,
    editable: true,
    i18nPrefix: 'pages.runLaunch.repos',
  },
)

const emit = defineEmits<{
  (e: 'update:repos', value: RepoRow[]): void
}>()

const { t } = useI18n()

function tKey(key: string, params?: Record<string, unknown>) {
  return t(`${props.i18nPrefix}.${key}`, params ?? {})
}

function updateRepos(next: RepoRow[]) {
  emit('update:repos', next)
}

function addRow() {
  updateRepos([...props.repos, { name: '', url: '', branch: '' }])
}

function removeRow(i: number) {
  if (props.repos.length <= props.minRows) return
  const next = [...props.repos]
  next.splice(i, 1)
  updateRepos(next)
}

function patchRow(i: number, patch: Partial<RepoRow>) {
  updateRepos(props.repos.map((r, idx) => (idx === i ? { ...r, ...patch } : r)))
}
</script>

<template>
  <div class="space-y-2">
    <div v-for="(r, ri) in repos" :key="ri" class="rounded-md border border-line bg-base/40 p-2 space-y-1.5">
      <div class="flex items-center gap-1.5">
        <span class="flex items-center gap-1 text-[11px] font-medium text-txt2">
          <Icon name="git" :size="12" />{{ tKey('itemLabel', { n: ri + 1 }) }}
        </span>
        <button
          v-if="editable && repos.length > minRows"
          class="ml-auto shrink-0 text-txt3 hover:text-err"
          :title="tKey('remove')"
          type="button"
          @click="removeRow(ri)"
        >
          <Icon name="close" :size="14" />
        </button>
      </div>
      <input
        :value="r.url"
        :disabled="!editable"
        class="input w-full font-mono text-[12px] disabled:opacity-60"
        :placeholder="tKey('urlPlaceholder')"
        @input="patchRow(ri, { url: ($event.target as HTMLInputElement).value })"
      />
      <div class="flex items-center gap-1.5">
        <input
          :value="r.name"
          :disabled="!editable"
          class="input flex-1 font-mono text-[12px] disabled:opacity-60"
          :placeholder="tKey('namePlaceholder')"
          @input="patchRow(ri, { name: ($event.target as HTMLInputElement).value })"
        />
        <input
          :value="r.branch"
          :disabled="!editable"
          class="input flex-1 font-mono text-[12px] disabled:opacity-60"
          :placeholder="tKey('branchPlaceholder')"
          @input="patchRow(ri, { branch: ($event.target as HTMLInputElement).value })"
        />
      </div>
    </div>
    <AppButton v-if="editable" size="sm" variant="subtle" icon="plus" @click="addRow">{{ tKey('add') }}</AppButton>
  </div>
</template>
