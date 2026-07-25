<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppButton from '@/components/ui/AppButton.vue'
import AppModal from '@/components/ui/AppModal.vue'
import {
  analyzeGitCredentials,
  type GitCredentialStatus,
  type GitCredentialType,
} from '@/lib/gitCredentialAnalysis'

type KV = { k: string; v: string }

const GIT_ENV_KEYS = new Set([
  'GIT_REPOS',
  'GITHUB_TOKEN', 'GITHUB_URL', 'GITLAB_TOKEN', 'GITLAB_URL',
  'GIT_SSH_PRIVATE_KEY', 'GIT_SSH_KNOWN_HOSTS',
])

const props = defineProps<{
  env: KV[]
  upsertEnv: (key: string, value: string) => void
  credentialType?: GitCredentialType
}>()
const emit = defineEmits<{
  'update:credentialType': [value: GitCredentialType]
}>()
const { t } = useI18n()
const showChooser = ref(false)
const pendingType = ref<GitCredentialType>('github_https')

const analysis = computed(() =>
  analyzeGitCredentials({ env: props.env, selectedType: props.credentialType }),
)
const statusClass: Record<GitCredentialStatus, string> = {
  disabled: 'border-line bg-base/50',
  complete: 'border-ok/40 bg-ok/5',
  incomplete: 'border-err/40 bg-err/5',
  needs_confirmation: 'border-warn/40 bg-warn/5',
  unsupported: 'border-err/40 bg-err/5',
}
const dotClass: Record<GitCredentialStatus, string> = {
  disabled: 'bg-txt3',
  complete: 'bg-ok',
  incomplete: 'bg-err',
  needs_confirmation: 'bg-warn',
  unsupported: 'bg-err',
}
const typeLabel = computed(() =>
  analysis.value.effectiveType
    ? t(`pages.agentStudio.git.types.${analysis.value.effectiveType}`)
    : t('pages.agentStudio.git.types.unknown'),
)
const issues = computed(() => [...analysis.value.conflicts, ...analysis.value.missing])
/** Runtime refs are informational, not warn badges once a credential type is chosen. */
const showRuntimeResolveBadge = computed(
  () => analysis.value.unresolvedReference && analysis.value.status !== 'disabled',
)
const showActionableTypeHint = computed(
  () =>
    analysis.value.unresolvedReference &&
    !analysis.value.effectiveType &&
    analysis.value.status === 'needs_confirmation',
)
const sourceLabelKey = computed(() => {
  if (analysis.value.source === 'user' && !analysis.value.selectionValid) {
    return 'pages.agentStudio.git.source.invalid'
  }
  return `pages.agentStudio.git.source.${analysis.value.source}`
})

const recommendations: Record<GitCredentialType, { key: string; value: string }[]> = {
  github_https: [
    { key: 'GIT_REPOS', value: '${vars.repos}' },
    { key: 'GITHUB_TOKEN', value: '${vars.github_pat}' },
  ],
  gitlab_https: [
    { key: 'GIT_REPOS', value: '${vars.repos}' },
    { key: 'GITLAB_TOKEN', value: '${vars.gitlab_pat}' },
  ],
  ssh: [
    { key: 'GIT_REPOS', value: '${vars.repos}' },
    { key: 'GIT_SSH_PRIVATE_KEY', value: '${vars.git_ssh_private_key}' },
    { key: 'GIT_SSH_KNOWN_HOSTS', value: '${vars.git_ssh_known_hosts}' },
  ],
}

function openChooser() {
  pendingType.value = props.credentialType ?? analysis.value.effectiveType ?? 'github_https'
  showChooser.value = true
}

function confirmType() {
  emit('update:credentialType', pendingType.value)
  showChooser.value = false
}

function applyRecommended() {
  for (const item of recommendations[pendingType.value]) {
    if (!props.env.some((entry) => entry.k === item.key)) {
      props.upsertEnv(item.key, item.value)
    }
  }
}

function isGitEnvKey(key: string): boolean {
  return GIT_ENV_KEYS.has(key)
}

defineExpose({ isGitEnvKey })
</script>

<template>
  <section class="mb-4 overflow-hidden rounded-lg border border-line bg-surface shadow-sm">
    <header class="flex items-center gap-3 border-b border-line px-4 py-3">
      <div class="min-w-0 flex-1">
        <div class="text-[13px] font-semibold text-txt">{{ t('pages.agentStudio.git.title') }}</div>
        <div class="mt-0.5 text-[11px] text-txt3">{{ t('pages.agentStudio.git.subtitle') }}</div>
      </div>
      <AppButton size="sm" variant="outline" @click="openChooser">
        {{ t('pages.agentStudio.git.adjustType') }}
      </AppButton>
    </header>

    <div class="p-4">
      <div class="rounded-lg border px-3.5 py-3" :class="statusClass[analysis.status]">
        <div class="flex items-start gap-2.5">
          <span class="mt-1 h-2.5 w-2.5 shrink-0 rounded-full" :class="dotClass[analysis.status]" />
          <div class="min-w-0 flex-1">
            <div class="text-[12px] font-semibold text-txt">
              {{
                showActionableTypeHint
                  ? t('pages.agentStudio.git.status.needs_type.title')
                  : t(`pages.agentStudio.git.status.${analysis.status}.title`)
              }}
            </div>
            <div class="mt-1 text-[11px] leading-5 text-txt2">
              {{
                showActionableTypeHint
                  ? t('pages.agentStudio.git.status.needs_type.description')
                  : t(`pages.agentStudio.git.status.${analysis.status}.description`)
              }}
            </div>
          </div>
          <span class="rounded bg-surface/70 px-2 py-1 text-[10px] text-txt3">
            {{ t(sourceLabelKey) }}
          </span>
        </div>
      </div>

      <div
        v-if="showRuntimeResolveBadge"
        class="mt-3 rounded-lg border border-line bg-base/40 px-3.5 py-2.5 text-[11px] leading-5 text-txt2"
      >
        <span class="mr-2 inline-block rounded bg-elevated px-1.5 py-0.5 text-[10px] text-txt3">
          {{ t('pages.agentStudio.git.runtimeResolve') }}
        </span>
        {{ t('pages.agentStudio.git.runtimeResolveHint') }}
      </div>

      <div v-if="analysis.status !== 'disabled'" class="mt-3 rounded-lg border border-line">
        <div class="flex items-center gap-2 border-b border-line bg-base/40 px-3 py-2.5">
          <span class="font-medium text-txt">{{ typeLabel }}</span>
          <span
            v-if="analysis.effectiveType && analysis.unresolvedReference"
            class="rounded bg-elevated px-1.5 py-0.5 text-[10px] text-txt3"
          >
            {{ t('pages.agentStudio.git.runtimeResolve') }}
          </span>
        </div>
        <div v-if="analysis.repos.length" class="divide-y divide-line px-3">
          <div v-for="repo in analysis.repos" :key="`${repo.name}:${repo.url}`" class="flex gap-3 py-2 text-[11px]">
            <code class="w-28 shrink-0 truncate text-accent-2">{{ repo.name }}</code>
            <span class="min-w-0 flex-1 truncate text-txt2">{{ repo.url }}</span>
            <span class="text-txt3">{{ repo.protocol.toUpperCase() }}</span>
          </div>
        </div>
        <div v-if="issues.length" class="space-y-1 border-t border-line px-3 py-2">
          <div v-for="(issue, i) in issues" :key="i" class="text-[11px] text-err">
            <code v-if="issue.repo" class="mr-1">{{ issue.repo }}</code>{{ issue.reason }}
          </div>
        </div>
      </div>

      <p class="mt-3 text-[11px] leading-5 text-txt3">{{ t('pages.agentStudio.git.envHint') }}</p>
      <p class="mt-1.5 text-[11px] leading-5 text-txt3">{{ t('pages.agentStudio.git.boundary') }}</p>
    </div>
  </section>

  <AppModal :open="showChooser" :width="520" @close="showChooser = false">
    <template #header>
      <div>
        <div class="text-[15px] font-semibold text-txt">{{ t('pages.agentStudio.git.chooseTitle') }}</div>
        <div class="mt-1 text-[11px] text-txt3">{{ t('pages.agentStudio.git.chooseHint') }}</div>
      </div>
    </template>
    <div class="space-y-2">
      <label
        v-for="type in (['github_https', 'gitlab_https', 'ssh'] as GitCredentialType[])"
        :key="type"
        class="flex cursor-pointer items-center gap-3 rounded-lg border p-3"
        :class="pendingType === type ? 'border-accent bg-accent-dim' : 'border-line'"
      >
        <input v-model="pendingType" type="radio" :value="type" />
        <span>
          <strong class="block text-[12px] text-txt">{{ t(`pages.agentStudio.git.types.${type}`) }}</strong>
          <small class="text-[11px] text-txt3">{{ t(`pages.agentStudio.git.typeHints.${type}`) }}</small>
        </span>
      </label>
      <AppButton size="sm" variant="outline" @click="applyRecommended">
        {{ t('pages.agentStudio.git.applyRecommended') }}
      </AppButton>
    </div>
    <template #footer>
      <AppButton variant="ghost" @click="showChooser = false">{{ t('common.buttons.cancel') }}</AppButton>
      <AppButton variant="primary" @click="confirmType">{{ t('pages.agentStudio.git.confirmApply') }}</AppButton>
    </template>
  </AppModal>
</template>
