<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/ui/Icon.vue'
import AppButton from '@/components/ui/AppButton.vue'
import { api } from '@/lib/api/api'
import { authGuideFor } from '@/lib/agent/backendAuthGuide'
import { ACP_BACKENDS, getRegionPolicy, type BackendId } from '@/lib/shared/regionPolicy'
import { setLocale } from '@/lib/shared/locale'
import type { AppLocale } from '@/lib/shared/loadLocaleMessages'
import { useToast } from '@/lib/composables/useToast'
import type { GitCredentialType } from '@/lib/agent/gitCredentialAnalysis'
import {
  ONBOARDING_AGENT_NAMES,
  ONBOARDING_GIT_TYPES,
  ONBOARDING_STEPS,
  assembleBootstrapBody,
  dismissOnboarding,
  freshOnboardingDraft,
  gitConfigured,
  gitIdentityConfigured,
  repoConfigured,
  repoNameFromUrl,
  type OnboardingBootstrapResult,
  type OnboardingDraft,
} from '@/lib/pm/onboardingWizard'

const props = defineProps<{
  open: boolean
  projectId: string
}>()

const emit = defineEmits<{
  close: []
  completed: [result: OnboardingBootstrapResult]
}>()

const { t } = useI18n()
const toast = useToast()

const draft = ref<OnboardingDraft>(freshOnboardingDraft())
const creating = ref(false)
const createError = ref('')
const phase = ref<'wizard' | 'success'>('wizard')
const result = ref<OnboardingBootstrapResult | null>(null)
const stepAnimKey = ref(0)
const keyError = ref(false)

const currentStep = computed(() => ONBOARDING_STEPS[draft.value.step] || ONBOARDING_STEPS[0])
const languageOptions: { id: AppLocale; label: string; hint: string }[] = [
  { id: 'zh-CN', label: '简体中文', hint: 'Chinese (Simplified)' },
  { id: 'en', label: 'English', hint: '英语' },
]
const progressPct = computed(() =>
  phase.value === 'success' ? 100 : ((draft.value.step + 1) / ONBOARDING_STEPS.length) * 100,
)
const regionPolicy = computed(() => getRegionPolicy(draft.value.acpBackend))
const authGuide = computed(() => authGuideFor(draft.value.acpBackend, draft.value.region))
const primaryAuthKey = computed(() => authGuide.value.keys[0]?.key || '')
const primaryAuthAlt = computed(() => authGuide.value.keys[0]?.alt || '')
const headSub = computed(() => {
  if (phase.value === 'success') return t('pages.onboarding.head.success')
  return t(`pages.onboarding.head.${currentStep.value.id}`)
})
const gitOk = computed(() => gitConfigured(draft.value))
const identityOk = computed(() => gitIdentityConfigured(draft.value))
const repoOk = computed(() => repoConfigured(draft.value))
const repoDirName = computed(() => repoNameFromUrl(draft.value.repoUrl))

watch(
  () => props.open,
  (open) => {
    if (open) {
      draft.value = freshOnboardingDraft()
      void setLocale(draft.value.language)
      creating.value = false
      createError.value = ''
      phase.value = 'wizard'
      result.value = null
      keyError.value = false
      stepAnimKey.value++
    }
  },
  { immediate: true },
)

function suppressAndClose() {
  if (creating.value) return
  dismissOnboarding(props.projectId)
  emit('close')
}

function selectBackend(id: BackendId) {
  if (draft.value.acpBackend === id) return
  draft.value.acpBackend = id
  const policy = getRegionPolicy(id)
  draft.value.region = policy?.defaultRegion || ''
}

function selectLanguage(language: AppLocale) {
  if (draft.value.language === language) return
  draft.value.language = language
  void setLocale(language)
}

function selectRegion(id: string) {
  draft.value.region = id
}

function selectGitType(id: GitCredentialType) {
  draft.value.gitCredentialType = draft.value.gitCredentialType === id ? '' : id
}

function toggleVncPreview() {
  draft.value.vncPreview = !draft.value.vncPreview
  if (!draft.value.vncPreview) draft.value.browserMcp = false
}

function toggleBrowserMcp() {
  draft.value.browserMcp = !draft.value.browserMcp
  if (draft.value.browserMcp) draft.value.vncPreview = true
}

function goPrev() {
  if (draft.value.step === 0 || creating.value || phase.value === 'success') return
  draft.value.step--
  stepAnimKey.value++
  keyError.value = false
}

function goSkip() {
  if (!currentStep.value.skip || creating.value) return
  if (currentStep.value.id === 'git') {
    if (!gitIdentityConfigured(draft.value)) {
      toast.error(t('pages.onboarding.toastNeedGitUser'))
      return
    }
    draft.value.gitCredentialType = ''
    draft.value.githubToken = ''
    draft.value.gitlabToken = ''
    draft.value.gitlabUrl = ''
    draft.value.gitSshPrivateKey = ''
    draft.value.gitSshKnownHosts = ''
    draft.value.repoUrl = ''
    draft.value.repoBranch = ''
  }
  draft.value.step++
  stepAnimKey.value++
}

function goNext() {
  if (creating.value) return
  const step = currentStep.value
  if (step.id === 'apiKey' && !draft.value.apiKey.trim()) {
    keyError.value = true
    toast.error(t('pages.onboarding.toastNeedKey'))
    return
  }
  if (step.id === 'git' && !gitIdentityConfigured(draft.value)) {
    toast.error(t('pages.onboarding.toastNeedGitUser'))
    return
  }
  if (step.id === 'review') {
    void submitBootstrap()
    return
  }
  keyError.value = false
  draft.value.step++
  stepAnimKey.value++
  if (ONBOARDING_STEPS[draft.value.step]?.id === 'apiKey') {
    nextTick(() => document.getElementById('onb-api-key')?.focus())
  }
}

async function submitBootstrap() {
  if (!draft.value.apiKey.trim()) {
    keyError.value = true
    toast.error(t('pages.onboarding.toastNeedKey'))
    return
  }
  creating.value = true
  createError.value = ''
  try {
    const body = assembleBootstrapBody(draft.value)
    const res = await api.bootstrapProjectOnboarding(props.projectId, body)
    result.value = res
    phase.value = 'success'
    dismissOnboarding(props.projectId)
    toast.success(t('pages.onboarding.toastOk'))
    emit('completed', res)
  } catch (e: any) {
    createError.value = e?.message || String(e)
    toast.error(createError.value || t('pages.onboarding.toastErr'))
  } finally {
    creating.value = false
  }
}
</script>

<template>
  <Teleport to="body">
    <div v-if="open" class="fixed inset-0 z-50 flex items-center justify-center p-4" data-testid="onboarding-wizard">
      <div class="absolute inset-0 bg-black/70" data-testid="onboarding-backdrop" @click="suppressAndClose" />
      <div
        class="rounded-xl relative z-10 flex w-full flex-col overflow-hidden border border-line bg-surface shadow-card"
        style="width: min(980px, 100%); height: min(640px, 92vh); border-radius: 16px"
        role="dialog"
        aria-modal="true"
      >
        <div class="relative flex h-16 shrink-0 items-center gap-3.5 border-b border-line px-5">
          <div class="rounded-md grid h-9 w-9 shrink-0 place-items-center border border-accent/55 text-accent-2">
            <Icon name="sparkles" :size="20" />
          </div>
          <div class="min-w-0 flex-1">
            <h2 class="m-0 text-[16px] font-semibold text-txt">{{ t('pages.onboarding.title') }}</h2>
            <span class="mt-0.5 block text-[12px] text-txt3">{{ headSub }}</span>
          </div>
          <button
            type="button"
            class="grid h-8 w-8 shrink-0 place-items-center text-txt3 hover:bg-elevated hover:text-txt"
            :disabled="creating"
            data-testid="onboarding-close"
            @click="suppressAndClose"
          >
            <Icon name="close" :size="18" />
          </button>
          <div class="absolute inset-x-0 bottom-0 h-[3px] overflow-hidden bg-elevated">
            <span
              class="block h-full bg-accent transition-[width] duration-500"
              :style="{ width: progressPct + '%' }"
            />
          </div>
        </div>

        <div v-if="phase === 'success'" class="flex min-h-0 flex-1 flex-col px-8 py-7" data-testid="onboarding-success">
          <h3 class="m-0 text-[18px] font-semibold text-txt">{{ t('pages.onboarding.success.title') }}</h3>
          <p class="mt-2 text-[13px] text-txt2">{{ t('pages.onboarding.success.desc') }}</p>
          <ul class="mt-4 space-y-1.5 text-[13px] text-txt2">
            <li v-for="n in ONBOARDING_AGENT_NAMES" :key="n">· {{ n }}</li>
            <li>· {{ t('pages.onboarding.success.publishedLine') }}</li>
          </ul>
          <p
            class="rounded-lg mt-4 border px-3 py-2 text-[12px]"
            :class="identityOk ? 'border-ok/35 bg-ok/10 text-ok' : 'border-warn/35 bg-warn/10 text-warn'"
            data-testid="onboarding-success-git-user"
          >
            {{
              identityOk
                ? t('pages.onboarding.success.gitUserOk', { name: draft.gitUserName, email: draft.gitUserEmail })
                : t('pages.onboarding.success.gitUserSkip')
            }}
          </p>
          <p class="rounded-lg mt-2 border border-ok/35 bg-ok/10 px-3 py-2 text-[12px] text-ok" data-testid="onboarding-success-preview">
            {{
              t('pages.onboarding.success.preview', {
                vnc: draft.vncPreview
                  ? t('pages.onboarding.preview.vncOn')
                  : t('pages.onboarding.preview.vncOff'),
                mcp: draft.browserMcp
                  ? t('pages.onboarding.preview.browserOn')
                  : t('pages.onboarding.preview.browserOff'),
              })
            }}
          </p>
          <p
            class="rounded-lg mt-2 border px-3 py-2 text-[12px]"
            :class="gitOk ? 'border-ok/35 bg-ok/10 text-ok' : 'border-warn/35 bg-warn/10 text-warn'"
            data-testid="onboarding-success-git"
          >
            {{ gitOk ? t('pages.onboarding.success.gitOk') : t('pages.onboarding.success.gitSkip') }}
          </p>
          <p
            class="rounded-lg mt-4 border px-3 py-2 text-[12px]"
            :class="repoOk ? 'border-ok/35 bg-ok/10 text-ok' : 'border-warn/35 bg-warn/10 text-warn'"
            data-testid="onboarding-success-repo"
          >
            {{
              repoOk
                ? t('pages.onboarding.success.repoOk', { dir: repoDirName })
                : t('pages.onboarding.success.limit')
            }}
          </p>
          <div class="mt-auto flex flex-wrap justify-end gap-2 border-t border-line pt-4">
            <AppButton variant="primary" data-testid="onboarding-success-close" @click="emit('close')">
              {{ t('pages.onboarding.close') }}
            </AppButton>
          </div>
        </div>

        <div v-else class="flex min-h-0 flex-1">
          <aside class="w-[208px] shrink-0 overflow-y-auto border-r border-line bg-elevated px-3 pb-4 pt-5">
            <div class="mb-4 flex items-center gap-2 px-1.5 text-[10px] font-semibold uppercase tracking-[0.08em] text-txt3">
              <span class="h-1.5 w-1.5 shrink-0 bg-accent" />
              {{ t('pages.onboarding.railCap') }}
            </div>
            <div
              v-for="(s, i) in ONBOARDING_STEPS"
              :key="s.id"
              class="mb-1 flex items-stretch gap-2.5 text-txt3"
              :class="{ 'text-txt2': i < draft.step, 'text-txt': i === draft.step }"
            >
              <div class="flex w-[18px] shrink-0 flex-col items-center">
                <div
                  class="mt-0.5 grid h-3.5 w-3.5 place-items-center rounded-full border"
                  :class="i < draft.step ? 'border-ok/55' : i === draft.step ? 'border-accent' : 'border-line-strong'"
                >
                  <i
                    class="block h-1.5 w-1.5"
                    :class="i < draft.step ? 'bg-ok' : i === draft.step ? 'bg-accent' : 'bg-transparent'"
                  />
                </div>
                <div v-if="i < ONBOARDING_STEPS.length - 1" class="mt-1 w-px flex-1 bg-line" />
              </div>
              <strong class="pb-3 text-[13px] font-medium">{{ t(s.labelKey) }}</strong>
            </div>
          </aside>

          <div class="flex min-w-0 flex-1 flex-col">
            <div class="min-h-0 flex-1 overflow-y-auto px-8 py-7">
              <div :key="stepAnimKey">
                <h3 class="m-0 text-[15px] font-semibold text-txt">{{ t(currentStep.labelKey) }}</h3>

                <template v-if="currentStep.id === 'language'">
                  <p class="mt-2 text-[13px] text-txt2">{{ t('pages.onboarding.language.meta') }}</p>
                  <div class="mt-5 grid max-w-lg grid-cols-2 gap-3">
                    <button
                      v-for="option in languageOptions"
                      :key="option.id"
                      type="button"
                      class="rounded-lg border px-4 py-4 text-left transition"
                      :class="
                        draft.language === option.id
                          ? 'border-accent bg-accent-dim'
                          : 'border-line bg-base hover:border-line-strong'
                      "
                      :data-testid="`onboarding-language-${option.id}`"
                      @click="selectLanguage(option.id)"
                    >
                      <strong class="block text-[14px] text-txt">{{ option.label }}</strong>
                      <span class="mt-1 block text-[11px] text-txt3">{{ option.hint }}</span>
                    </button>
                  </div>
                  <p class="mt-4 text-[12px] text-txt3">{{ t('pages.onboarding.language.detected') }}</p>
                </template>

                <template v-else-if="currentStep.id === 'overview'">
                  <p class="mt-2 text-[13px] text-txt2">{{ t('pages.onboarding.overview.meta') }}</p>
                  <ol class="mt-4 list-decimal space-y-2 pl-5 text-[13px] text-txt2">
                    <li>{{ t('pages.onboarding.overview.itemBackend') }}</li>
                    <li>{{ t('pages.onboarding.overview.itemToken') }}</li>
                    <li>{{ t('pages.onboarding.overview.itemGit') }}</li>
                    <li>{{ t('pages.onboarding.overview.itemPreview') }}</li>
                  </ol>
                  <div class="mt-4 grid gap-3 sm:grid-cols-2">
                    <div class="rounded-lg border border-line bg-base px-3 py-3">
                      <div class="text-[11px] uppercase text-txt3">{{ t('pages.onboarding.overview.agents') }}</div>
                      <div class="mt-1 text-[18px] font-semibold text-txt">6</div>
                      <p class="mt-1 text-[12px] text-txt2">{{ t('pages.onboarding.overview.agentsList') }}</p>
                    </div>
                    <div class="rounded-lg border border-line bg-base px-3 py-3">
                      <div class="text-[11px] uppercase text-txt3">{{ t('pages.onboarding.overview.workflow') }}</div>
                      <div class="mt-1 text-[18px] font-semibold text-txt">{{ t('pages.onboarding.workflowName') }}</div>
                      <p class="mt-1 text-[12px] text-txt2">{{ t('pages.onboarding.overview.workflowHint') }}</p>
                    </div>
                  </div>
                  <p class="rounded-lg mt-4 border border-accent/35 bg-accent-dim px-3 py-2 text-[12px] text-accent-2">
                    {{ t('pages.onboarding.overview.banner') }}
                  </p>
                </template>

                <template v-else-if="currentStep.id === 'acp'">
                  <p class="mt-2 text-[13px] text-txt2">{{ t('pages.onboarding.acp.meta') }}</p>
                  <div class="mt-4 grid grid-cols-2 gap-2.5 md:grid-cols-4">
                    <button
                      v-for="b in ACP_BACKENDS"
                      :key="b.id"
                      type="button"
                      class="rounded-lg border px-3 py-3.5 text-center transition"
                      :class="
                        draft.acpBackend === b.id
                          ? 'border-accent bg-accent-dim'
                          : 'border-line bg-base hover:border-line-strong'
                      "
                      @click="selectBackend(b.id)"
                    >
                      <strong class="block text-[13px] text-txt">{{ b.label }}</strong>
                      <span class="mt-1 block font-mono text-[10px] text-txt3">{{ b.configRoot }}</span>
                    </button>
                  </div>
                  <div v-if="regionPolicy" class="mt-5 border-t border-dashed border-line pt-4">
                    <div class="mb-2 text-[12px] font-medium text-txt2">{{ t('pages.onboarding.acp.region') }}</div>
                    <div class="grid max-w-lg grid-cols-2 gap-2.5">
                      <button
                        v-for="option in regionPolicy.options"
                        :key="option.id"
                        type="button"
                        class="rounded-lg border px-3 py-3 text-left"
                        :class="
                          draft.region === option.id
                            ? 'border-accent bg-accent-dim'
                            : 'border-line bg-base hover:border-line-strong'
                        "
                        @click="selectRegion(option.id)"
                      >
                        <strong class="block text-[13px] text-txt">{{ t(option.labelKey) }}</strong>
                        <span class="mt-1 block font-mono text-[10px] text-txt3">{{ option.id }}</span>
                      </button>
                    </div>
                  </div>
                </template>

                <template v-else-if="currentStep.id === 'apiKey'">
                  <p class="mt-2 text-[13px] text-txt2">{{ t('pages.onboarding.apiKey.meta') }}</p>
                  <div class="rounded-lg mt-3 border border-accent/35 bg-accent-dim px-3 py-2 text-[12px] text-accent-2">
                    <code>{{ primaryAuthKey }}</code>
                    <span v-if="primaryAuthAlt" class="ml-2 text-txt3">/ {{ primaryAuthAlt }}</span>
                  </div>
                  <ol class="mt-3 list-decimal space-y-1 pl-5 text-[12px] text-txt2">
                    <li v-for="(k, i) in authGuide.pathStepKeys" :key="i">{{ t(k) }}</li>
                  </ol>
                  <div class="mt-3 flex flex-wrap gap-2">
                    <a
                      v-for="link in authGuide.links"
                      :key="link.url"
                      :href="link.url"
                      target="_blank"
                      rel="noopener noreferrer"
                      class="rounded-md border border-line px-2 py-1 text-[11px] text-txt2 hover:border-line-strong hover:text-txt"
                    >{{ t(link.labelKey) }}</a>
                  </div>
                  <label class="mt-4 block">
                    <span class="mb-1.5 block text-[12px] font-medium text-txt2">
                      API Key <span class="text-err">*</span>
                    </span>
                    <input
                      id="onb-api-key"
                      v-model="draft.apiKey"
                      type="password"
                      autocomplete="off"
                      class="rounded-md w-full border border-line bg-base px-3 py-2 font-mono text-[13px] text-txt outline-none focus:border-accent"
                      data-testid="onboarding-api-key"
                      @input="keyError = false"
                    />
                    <p class="mt-1.5 text-[11px] text-txt3">{{ t('pages.onboarding.apiKey.hint') }}</p>
                    <p v-if="keyError" class="mt-1 text-[12px] text-err">{{ t('pages.onboarding.apiKey.required') }}</p>
                  </label>
                </template>

                <template v-else-if="currentStep.id === 'git'">
                  <p class="mt-2 text-[13px] text-txt2">{{ t('pages.onboarding.git.meta') }}</p>

                  <div class="rounded-lg mt-4 border border-line bg-base px-3 py-3">
                    <div class="text-[11px] uppercase tracking-[0.06em] text-txt3">
                      {{ t('pages.onboarding.repo.section') }}
                    </div>
                    <label class="mt-2.5 block">
                      <span class="mb-1.5 block text-[12px] font-medium text-txt2">
                        {{ t('pages.onboarding.repo.urlLabel') }}
                      </span>
                      <input
                        v-model="draft.repoUrl"
                        type="text"
                        autocomplete="off"
                        placeholder="https://github.com/org/repo.git"
                        class="rounded-md w-full border border-line bg-surface px-3 py-2 font-mono text-[13px] text-txt outline-none focus:border-accent"
                        data-testid="onboarding-repo-url"
                      />
                    </label>
                    <label class="mt-2.5 block">
                      <span class="mb-1.5 block text-[12px] font-medium text-txt2">
                        {{ t('pages.onboarding.repo.branchLabel') }}
                      </span>
                      <input
                        v-model="draft.repoBranch"
                        type="text"
                        autocomplete="off"
                        :placeholder="t('pages.onboarding.repo.branchPlaceholder')"
                        class="rounded-md w-full border border-line bg-surface px-3 py-2 font-mono text-[13px] text-txt outline-none focus:border-accent"
                        data-testid="onboarding-repo-branch"
                      />
                    </label>
                    <p class="mt-2 text-[11px] text-txt3" data-testid="onboarding-repo-hint">
                      {{
                        repoDirName
                          ? t('pages.onboarding.repo.cloneTo', { dir: repoDirName })
                          : t('pages.onboarding.repo.hint')
                      }}
                    </p>
                  </div>

                  <div class="rounded-lg mt-4 border border-line bg-base px-3 py-3">
                    <div class="text-[11px] uppercase tracking-[0.06em] text-txt3">
                      {{ t('pages.onboarding.gitUser.section') }}
                    </div>
                    <div class="mt-2.5 grid gap-3 sm:grid-cols-2">
                      <label class="block">
                        <span class="mb-1.5 block text-[12px] font-medium text-txt2">
                          {{ t('pages.onboarding.gitUser.nameLabel') }} <span class="text-err">*</span>
                        </span>
                        <input
                          v-model="draft.gitUserName"
                          type="text"
                          autocomplete="off"
                          :placeholder="t('pages.onboarding.gitUser.namePlaceholder')"
                          class="rounded-md w-full border border-line bg-surface px-3 py-2 font-mono text-[13px] text-txt outline-none focus:border-accent"
                          data-testid="onboarding-git-user-name"
                        />
                      </label>
                      <label class="block">
                        <span class="mb-1.5 block text-[12px] font-medium text-txt2">
                          {{ t('pages.onboarding.gitUser.emailLabel') }} <span class="text-err">*</span>
                        </span>
                        <input
                          v-model="draft.gitUserEmail"
                          type="email"
                          autocomplete="off"
                          :placeholder="t('pages.onboarding.gitUser.emailPlaceholder')"
                          class="rounded-md w-full border border-line bg-surface px-3 py-2 font-mono text-[13px] text-txt outline-none focus:border-accent"
                          data-testid="onboarding-git-user-email"
                        />
                      </label>
                    </div>
                    <p class="mt-2 text-[11px] text-txt3">{{ t('pages.onboarding.gitUser.hint') }}</p>
                  </div>

                  <div class="rounded-lg mt-4 border border-line bg-base px-3 py-3">
                    <div class="text-[11px] uppercase tracking-[0.06em] text-txt3">
                      {{ t('pages.onboarding.preview.section') }}
                    </div>
                    <div class="mt-2.5 grid gap-2 sm:grid-cols-2">
                      <button
                        type="button"
                        class="rounded-lg border px-3 py-3 text-left"
                        :class="
                          draft.vncPreview
                            ? 'border-accent bg-accent-dim'
                            : 'border-line bg-surface hover:border-line-strong'
                        "
                        data-testid="onboarding-vnc-preview"
                        @click="toggleVncPreview"
                      >
                        <strong class="block text-[13px] text-txt">{{ t('pages.onboarding.preview.vncLabel') }}</strong>
                        <span class="mt-1 block text-[11px] text-txt3">{{ t('pages.onboarding.preview.vncHint') }}</span>
                      </button>
                      <button
                        type="button"
                        class="rounded-lg border px-3 py-3 text-left"
                        :class="
                          draft.browserMcp
                            ? 'border-accent bg-accent-dim'
                            : 'border-line bg-surface hover:border-line-strong'
                        "
                        data-testid="onboarding-browser-mcp"
                        @click="toggleBrowserMcp"
                      >
                        <strong class="block text-[13px] text-txt">{{ t('pages.onboarding.preview.browserLabel') }}</strong>
                        <span class="mt-1 block text-[11px] text-txt3">{{ t('pages.onboarding.preview.browserHint') }}</span>
                      </button>
                    </div>
                  </div>

                  <div class="mt-4 text-[11px] uppercase tracking-[0.06em] text-txt3">
                    {{ t('pages.onboarding.repo.credSection') }}
                  </div>
                  <div class="mt-2 grid grid-cols-3 gap-2.5">
                    <button
                      v-for="g in ONBOARDING_GIT_TYPES"
                      :key="g.id"
                      type="button"
                      class="rounded-lg border px-3 py-3 text-center"
                      :class="
                        draft.gitCredentialType === g.id
                          ? 'border-accent bg-accent-dim'
                          : 'border-line bg-base hover:border-line-strong'
                      "
                      :data-testid="`onboarding-git-type-${g.id}`"
                      @click="selectGitType(g.id)"
                    >
                      <strong class="block text-[13px] text-txt">{{ t(g.labelKey) }}</strong>
                    </button>
                  </div>
                  <label v-if="draft.gitCredentialType === 'github_https'" class="mt-4 block">
                    <span class="mb-1.5 block text-[12px] font-medium text-txt2">GITHUB_TOKEN</span>
                    <input
                      v-model="draft.githubToken"
                      type="password"
                      autocomplete="off"
                      class="rounded-md w-full border border-line bg-base px-3 py-2 font-mono text-[13px] text-txt"
                      data-testid="onboarding-github-token"
                    />
                  </label>
                  <div v-else-if="draft.gitCredentialType === 'gitlab_https'" class="mt-4 grid gap-3">
                    <label class="block">
                      <span class="mb-1.5 block text-[12px] font-medium text-txt2">GITLAB_TOKEN</span>
                      <input
                        v-model="draft.gitlabToken"
                        type="password"
                        autocomplete="off"
                        class="rounded-md w-full border border-line bg-base px-3 py-2 font-mono text-[13px] text-txt"
                        data-testid="onboarding-gitlab-token"
                      />
                    </label>
                    <label class="block">
                      <span class="mb-1.5 block text-[12px] font-medium text-txt2">GITLAB_URL</span>
                      <input
                        v-model="draft.gitlabUrl"
                        type="text"
                        placeholder="https://gitlab.example.com"
                        class="rounded-md w-full border border-line bg-base px-3 py-2 font-mono text-[13px] text-txt"
                        data-testid="onboarding-gitlab-url"
                      />
                    </label>
                  </div>
                  <div v-else-if="draft.gitCredentialType === 'ssh'" class="mt-4 grid gap-3">
                    <label class="block">
                      <span class="mb-1.5 block text-[12px] font-medium text-txt2">SSH private key</span>
                      <textarea
                        v-model="draft.gitSshPrivateKey"
                        rows="4"
                        class="rounded-md w-full border border-line bg-base px-3 py-2 font-mono text-[12px] text-txt"
                        data-testid="onboarding-ssh-key"
                      />
                    </label>
                    <label class="block">
                      <span class="mb-1.5 block text-[12px] font-medium text-txt2">known_hosts</span>
                      <textarea
                        v-model="draft.gitSshKnownHosts"
                        rows="2"
                        class="rounded-md w-full border border-line bg-base px-3 py-2 font-mono text-[12px] text-txt"
                      />
                    </label>
                  </div>
                  <p class="mt-3 text-[12px] text-txt3">{{ t('pages.onboarding.git.foot') }}</p>
                </template>

                <template v-else-if="currentStep.id === 'review'">
                  <p class="mt-2 text-[13px] text-txt2">{{ t('pages.onboarding.review.meta') }}</p>
                  <div class="mt-3 flex flex-wrap gap-2 text-[12px]">
                    <span class="rounded-lg border border-ok/35 bg-ok/10 px-2 py-1 text-ok">
                      Backend · {{ ACP_BACKENDS.find((b) => b.id === draft.acpBackend)?.label }}
                    </span>
                    <span
                      v-if="regionPolicy && draft.region"
                      class="rounded-lg border border-ok/35 bg-ok/10 px-2 py-1 text-ok"
                    >Region · {{ draft.region }}</span>
                    <span
                      class="rounded-lg border px-2 py-1"
                      :class="draft.apiKey.trim() ? 'border-ok/35 bg-ok/10 text-ok' : 'border-warn/35 bg-warn/10 text-warn'"
                    >
                      API Key · {{ draft.apiKey.trim() ? t('pages.onboarding.review.keyOn') : t('pages.onboarding.review.keyOff') }}
                    </span>
                    <span
                      class="rounded-lg border px-2 py-1"
                      :class="repoOk ? 'border-ok/35 bg-ok/10 text-ok' : 'border-line text-txt2'"
                      data-testid="onboarding-review-repo"
                    >
                      {{ t('pages.onboarding.repo.chip') }} ·
                      {{ repoOk ? repoDirName : t('pages.onboarding.review.repoSkip') }}
                    </span>
                    <span
                      class="rounded-lg border px-2 py-1"
                      :class="identityOk ? 'border-ok/35 bg-ok/10 text-ok' : 'border-warn/35 bg-warn/10 text-warn'"
                      data-testid="onboarding-review-git-user"
                    >
                      {{ t('pages.onboarding.gitUser.chip') }} ·
                      {{ identityOk ? draft.gitUserName : t('pages.onboarding.review.gitUserOff') }}
                    </span>
                    <span
                      class="rounded-lg border px-2 py-1"
                      :class="draft.vncPreview ? 'border-ok/35 bg-ok/10 text-ok' : 'border-line text-txt2'"
                    >
                      {{ t('pages.onboarding.preview.vncLabel') }} · {{ draft.vncPreview ? t('pages.onboarding.review.flagOn') : t('pages.onboarding.review.flagOff') }}
                    </span>
                    <span
                      class="rounded-lg border px-2 py-1"
                      :class="draft.browserMcp ? 'border-ok/35 bg-ok/10 text-ok' : 'border-line text-txt2'"
                    >
                      {{ t('pages.onboarding.preview.browserLabel') }} · {{ draft.browserMcp ? t('pages.onboarding.review.flagOn') : t('pages.onboarding.review.flagOff') }}
                    </span>
                    <span
                      class="rounded-lg border px-2 py-1"
                      :class="gitOk ? 'border-ok/35 bg-ok/10 text-ok' : 'border-line text-txt2'"
                    >
                      Git · {{ gitOk ? draft.gitCredentialType : t('pages.onboarding.review.gitSkip') }}
                    </span>
                    <span class="rounded-lg border border-ok/35 bg-ok/10 px-2 py-1 text-ok">{{ t('pages.onboarding.review.agentsChip') }}</span>
                    <span class="rounded-lg border border-ok/35 bg-ok/10 px-2 py-1 text-ok">{{ t('pages.onboarding.review.wfChip') }}</span>
                  </div>
                  <p v-if="!draft.apiKey.trim()" class="mt-3 text-[12px] text-warn">{{ t('pages.onboarding.review.needKey') }}</p>
                  <p class="mt-3 text-[12px] text-txt3">{{ t('pages.onboarding.review.featureHint') }}</p>
                  <p v-if="createError" class="mt-2 text-[12px] text-err">{{ createError }}</p>
                </template>
              </div>
            </div>

            <div class="flex shrink-0 items-center gap-2 border-t border-line px-6 py-3">
              <AppButton variant="ghost" data-testid="onboarding-later" :disabled="creating" @click="suppressAndClose">
                {{ t('pages.onboarding.later') }}
              </AppButton>
              <div class="flex-1" />
              <AppButton variant="ghost" :disabled="draft.step === 0 || creating" @click="goPrev">
                {{ t('pages.onboarding.prev') }}
              </AppButton>
              <AppButton
                v-if="currentStep.skip"
                variant="outline"
                :disabled="creating"
                data-testid="onboarding-skip"
                @click="goSkip"
              >{{ t('pages.onboarding.skip') }}</AppButton>
              <AppButton
                variant="primary"
                :disabled="creating"
                data-testid="onboarding-next"
                @click="goNext"
              >
                {{
                  creating
                    ? t('pages.onboarding.generating')
                    : currentStep.id === 'review'
                      ? t('pages.onboarding.generate')
                      : t('pages.onboarding.next')
                }}
              </AppButton>
            </div>
          </div>
        </div>
      </div>
    </div>
  </Teleport>
</template>
