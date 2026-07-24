<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/ui/Icon.vue'
import AppButton from '@/components/ui/AppButton.vue'
import AgentGitGuide from '@/components/agent/AgentGitGuide.vue'
import { api, type Agent } from '@/lib/api'
import {
  ACP_BACKENDS,
  GIT_ENV_KEYS,
  WIZARD_STEPS,
  applyAcpBackend,
  assembleCreatePayload,
  buildDefaultRule,
  buildReviewSummary,
  defaultCommandTemplate,
  defaultSkillTemplate,
  freshDraft,
  hasPathDeps,
  validateBasics,
  type WizardBackendId,
  type WizardDraft,
  type WizardMCP,
  type WizardStepId,
} from '@/lib/agentCreateWizard'
import {
  getRegionPolicy,
  isManagedRegionKey,
  setRegion,
} from '@/lib/regionPolicy'

const props = defineProps<{
  open: boolean
  existingNames: string[]
}>()

const emit = defineEmits<{
  close: []
  created: [agent: Agent]
}>()

const { t } = useI18n()

const draft = ref<WizardDraft>(freshDraft())
const nameError = ref('')
const creating = ref(false)
const createError = ref('')
const pendingAcp = ref<WizardBackendId | null>(null)
const showAcpConfirm = ref(false)
const stepAnimKey = ref(0)
const newSkillName = ref('')
const newCmdName = ref('')
const regionManagedNotice = ref(false)

const currentStep = computed(() => WIZARD_STEPS[draft.value.step])
const progressPct = computed(() => ((draft.value.step + 1) / WIZARD_STEPS.length) * 100)
const reviewItems = computed(() => buildReviewSummary(draft.value))
const currentRegionPolicy = computed(() => getRegionPolicy(draft.value.acpBackend))
const currentRegion = computed(() => {
  const policy = currentRegionPolicy.value
  if (!policy) return ''
  return draft.value.env.find((item) => item.k.trim() === policy.regionEnvKey)?.v || ''
})

const promptFragments = computed(() =>
  [
    {
      key: 'upstreamArtifactsHeader' as const,
      label: t('pages.agentStudio.promptFields.upstreamHeader.label'),
      hint: t('pages.agentStudio.promptFields.upstreamHeader.hint'),
      placeholder: t('pages.agentStudio.promptFields.upstreamHeader.placeholder'),
    },
    {
      key: 'producesContract' as const,
      label: t('pages.agentStudio.promptFields.producesContract.label'),
      hint: t('pages.agentStudio.promptFields.producesContract.hint'),
      placeholder: t('pages.agentStudio.promptFields.producesContract.placeholder'),
    },
    {
      key: 'reactOpenSuffix' as const,
      label: t('pages.agentStudio.promptFields.reactOpening.label'),
      hint: t('pages.agentStudio.promptFields.reactOpening.hint'),
      placeholder: t('pages.agentStudio.promptFields.reactOpening.placeholder'),
    },
    {
      key: 'producesRetry' as const,
      label: t('pages.agentStudio.promptFields.reactMissingArtifact.label'),
      hint: t('pages.agentStudio.promptFields.reactMissingArtifact.hint'),
      placeholder: t('pages.agentStudio.promptFields.reactMissingArtifact.placeholder'),
    },
  ],
)

const headSub = computed(() => {
  const id = currentStep.value.id
  if (id === 'review') return t('pages.agentStudio.wizard.head.review')
  if (id === 'basics') return t('pages.agentStudio.wizard.head.basics')
  return t('pages.agentStudio.wizard.head.optional')
})

watch(
  () => props.open,
  (open) => {
    if (open) {
      draft.value = freshDraft()
      nameError.value = ''
      createError.value = ''
      creating.value = false
      pendingAcp.value = null
      showAcpConfirm.value = false
      newSkillName.value = ''
      newCmdName.value = ''
      regionManagedNotice.value = false
      stepAnimKey.value++
      nextTick(() => {
        const el = document.getElementById('wiz-name-input')
        el?.focus()
      })
    }
  },
)

function close() {
  if (creating.value) return
  emit('close')
}

function upsertEnv(key: string, value: string) {
  const row = draft.value.env.find((e) => e.k === key)
  if (row) row.v = value
  else draft.value.env.push({ k: key, v: value })
}

function selectRegion(region: string) {
  const env = Object.fromEntries(draft.value.env.filter((item) => item.k.trim()).map((item) => [item.k.trim(), item.v]))
  draft.value.env = Object.entries(setRegion(env, draft.value.acpBackend, region)).map(([k, v]) => ({ k, v }))
  markConfigured('acp')
}

function markConfigured(step: WizardStepId) {
  delete draft.value.skipped[step]
}

function selectAcp(id: WizardBackendId) {
  if (id === draft.value.acpBackend) return
  if (hasPathDeps(draft.value)) {
    pendingAcp.value = id
    showAcpConfirm.value = true
    return
  }
  applyAcpBackend(draft.value, id)
  markConfigured('acp')
}

function confirmAcpSwitch() {
  if (pendingAcp.value) {
    applyAcpBackend(draft.value, pendingAcp.value)
    markConfigured('acp')
  }
  pendingAcp.value = null
  showAcpConfirm.value = false
}

function cancelAcpSwitch() {
  pendingAcp.value = null
  showAcpConfirm.value = false
}

function ensureRulesContent() {
  if (!draft.value.rulesContent) {
    draft.value.rulesContent = buildDefaultRule(draft.value.name, draft.value.description)
  }
}

function goPrev() {
  if (draft.value.step === 0 || creating.value) return
  draft.value.step--
  stepAnimKey.value++
  if (currentStep.value.id === 'rules') ensureRulesContent()
}

function goSkip() {
  const step = currentStep.value
  if (!step.skip || creating.value) return
  draft.value.skipped[step.id] = true
  if (step.id === 'rules' && !draft.value.rulesEdited) {
    draft.value.rulesContent = ''
  }
  draft.value.step++
  stepAnimKey.value++
  if (currentStep.value.id === 'rules') ensureRulesContent()
}

function goNext() {
  if (creating.value) return
  const step = currentStep.value
  if (step.id === 'basics') {
    const err = validateBasics(draft.value, props.existingNames)
    if (err) {
      nameError.value =
        err === 'required'
          ? t('pages.agentStudio.dialogs.nameRequired')
          : err === 'invalid'
            ? t('pages.agentStudio.dialogs.nameInvalid')
            : t('pages.agentStudio.dialogs.nameExists')
      return
    }
    nameError.value = ''
  }
  if (step.id === 'review') {
    void submitCreate()
    return
  }
  markConfigured(step.id)
  draft.value.step++
  stepAnimKey.value++
  if (currentStep.value.id === 'rules') ensureRulesContent()
}

async function submitCreate() {
  const err = validateBasics(draft.value, props.existingNames)
  if (err) {
    draft.value.step = 0
    nameError.value =
      err === 'required'
        ? t('pages.agentStudio.dialogs.nameRequired')
        : err === 'invalid'
          ? t('pages.agentStudio.dialogs.nameInvalid')
          : t('pages.agentStudio.dialogs.nameExists')
    stepAnimKey.value++
    return
  }
  creating.value = true
  createError.value = ''
  try {
    const payload = assembleCreatePayload(draft.value)
    const created = await api.createAgent(payload)
    emit('created', created)
    emit('close')
  } catch (e: any) {
    createError.value = e?.message || String(e)
  } finally {
    creating.value = false
  }
}

function onRulesInput() {
  draft.value.rulesEdited = true
  markConfigured('rules')
}

function addEnvRow() {
  draft.value.env.push({ k: '', v: '' })
  markConfigured('env')
}

function updateEnvKey(i: number, value: string) {
  if (isManagedRegionKey(value)) {
    draft.value.env[i].k = ''
    regionManagedNotice.value = true
    return
  }
  draft.value.env[i].k = value
  regionManagedNotice.value = false
  markConfigured('env')
}

function removeEnv(i: number) {
  draft.value.env.splice(i, 1)
  markConfigured('env')
}

function emptyMcp(): WizardMCP {
  return {
    name: '',
    transport: 'url',
    url: '',
    headers: [],
    command: '',
    args: '',
    env: [],
  }
}

function addMcp() {
  draft.value.mcp.push(emptyMcp())
  markConfigured('mcp')
}

function removeMcp(i: number) {
  draft.value.mcp.splice(i, 1)
  markConfigured('mcp')
}

function addArtifactStore() {
  if (draft.value.mcp.some((m) => m.name.trim() === 'artifact-store')) return
  draft.value.mcp.push({
    name: 'artifact-store',
    transport: 'url',
    url: '${APPROVING_ARTIFACT_URL}',
    headers: [{ k: 'Authorization', v: 'Bearer ${APPROVING_ARTIFACT_TOKEN}' }],
    command: '',
    args: '',
    env: [],
  })
  markConfigured('mcp')
}

function addSkill() {
  const n = newSkillName.value.trim().replace(/[^A-Za-z0-9_-]/g, '')
  if (!n) return
  if (draft.value.skills.some((s) => s.name === n)) return
  draft.value.skills.push({ name: n, content: defaultSkillTemplate(n) })
  newSkillName.value = ''
  markConfigured('skills')
}

function removeSkill(i: number) {
  draft.value.skills.splice(i, 1)
  markConfigured('skills')
}

function addCommand() {
  const n = newCmdName.value.trim().replace(/[^A-Za-z0-9_-]/g, '')
  if (!n) return
  if (draft.value.commands.some((c) => c.name === n)) return
  draft.value.commands.push({ name: n, content: defaultCommandTemplate(n) })
  newCmdName.value = ''
  markConfigured('commands')
}

function removeCommand(i: number) {
  draft.value.commands.splice(i, 1)
  markConfigured('commands')
}

function onPromptInput() {
  markConfigured('prompts')
}

function chipClass(kind: string) {
  if (kind === 'ok') return 'border-ok/35 bg-ok/10 text-ok'
  if (kind === 'def') return 'border-accent/35 bg-accent-dim text-accent-2'
  return 'border-line bg-elevated text-txt3'
}

/** ENV 步不展示 Git 步管理的凭据键（空 key 的新行仍显示）。 */
function showInEnvStep(key: string) {
  const k = key.trim()
  return !k || (!GIT_ENV_KEYS.has(k) && !isManagedRegionKey(k))
}
</script>

<template>
  <Teleport to="body">
    <div v-if="open" class="wiz-root fixed inset-0 z-50 flex items-center justify-center p-4">
      <div class="absolute inset-0 bg-black/70" @click="close" />
      <div
        class="wiz-modal relative z-10 flex w-full flex-col overflow-hidden border border-line bg-surface shadow-card"
        style="width: min(980px, 100%); height: min(700px, 94vh); border-radius: 0"
        role="dialog"
        aria-modal="true"
      >
        <!-- head -->
        <div class="wiz-head relative flex h-16 shrink-0 items-center gap-3.5 border-b border-line px-5">
          <div class="hero-mark grid h-9 w-9 shrink-0 place-items-center border border-accent/55 text-accent-2">
            <Icon name="robot" :size="20" />
          </div>
          <div class="min-w-0 flex-1">
            <h2 class="m-0 text-[16px] font-semibold tracking-tight text-txt">{{ t('pages.agentStudio.wizard.title') }}</h2>
            <span class="mt-0.5 block text-[12px] font-normal text-txt3">{{ headSub }}</span>
          </div>
          <button
            type="button"
            class="grid h-8 w-8 shrink-0 place-items-center text-txt3 hover:bg-elevated hover:text-txt"
            :aria-label="t('pages.agentStudio.dialogs.cancel')"
            :disabled="creating"
            @click="close"
          >
            <Icon name="close" :size="18" />
          </button>
          <div class="wiz-progress absolute inset-x-0 bottom-0 h-[3px] overflow-hidden bg-elevated">
            <span :style="{ width: progressPct + '%' }" />
          </div>
        </div>

        <div class="flex min-h-0 flex-1">
          <!-- rail -->
          <aside class="wiz-rail scroll-area w-[208px] shrink-0 overflow-y-auto border-r border-line bg-elevated px-3 pb-4 pt-5">
            <div class="mb-4 flex items-center gap-2 px-1.5 text-[10px] font-semibold uppercase tracking-[0.08em] text-txt3">
              <span class="pulse-dot h-1.5 w-1.5 shrink-0 bg-accent" />
              {{ t('pages.agentStudio.wizard.railCap') }}
            </div>
            <div
              v-for="(s, i) in WIZARD_STEPS"
              :key="s.id"
              class="rail-item"
              :class="{ done: i < draft.step, cur: i === draft.step }"
            >
              <div class="track">
                <div class="node" aria-hidden="true"><i /></div>
                <div class="connector" />
              </div>
              <div class="lbl">
                <strong>{{ t(s.labelKey) }}</strong>
              </div>
            </div>
          </aside>

          <!-- main -->
          <div class="flex min-w-0 flex-1 flex-col">
            <div class="scroll-area min-h-0 flex-1 overflow-y-auto px-8 py-7">
              <div :key="stepAnimKey" class="step-pane">
                <div class="sec-head">
                  <div class="sec-bar">
                    <h3>{{ t(currentStep.labelKey) }}</h3>
                  </div>
                </div>

                <!-- basics -->
                <template v-if="currentStep.id === 'basics'">
                  <p class="sec-meta">{{ t('pages.agentStudio.wizard.basics.meta') }}</p>
                  <label class="mb-4 block">
                    <span class="mb-1.5 block text-[12px] font-medium text-txt2">
                      {{ t('pages.agentStudio.dialogs.createLabel') }}
                      <span class="text-err">*</span>
                    </span>
                    <input
                      id="wiz-name-input"
                      v-model="draft.name"
                      class="w-full border border-line bg-base px-3 py-2 text-[13px] text-txt outline-none focus:border-accent"
                      :placeholder="t('pages.agentStudio.dialogs.createPlaceholder')"
                      @input="nameError = ''"
                    />
                    <p v-if="nameError" class="mt-1.5 text-[12px] text-err">{{ nameError }}</p>
                  </label>
                  <label class="block">
                    <span class="mb-1.5 block text-[12px] font-medium text-txt2">{{ t('pages.agentStudio.wizard.basics.descLabel') }}</span>
                    <textarea
                      v-model="draft.description"
                      rows="3"
                      class="w-full resize-y border border-line bg-base px-3 py-2 font-mono text-[12px] leading-6 text-txt outline-none focus:border-accent"
                      :placeholder="t('pages.agentStudio.wizard.basics.descPlaceholder')"
                    />
                    <p class="mt-1.5 text-[11px] text-txt3">{{ t('pages.agentStudio.wizard.basics.descHint') }}</p>
                  </label>
                </template>

                <!-- acp -->
                <template v-else-if="currentStep.id === 'acp'">
                  <p class="sec-meta">{{ t('pages.agentStudio.wizard.acp.meta') }}</p>
                  <div class="grid grid-cols-2 gap-2.5 md:grid-cols-4">
                    <button
                      v-for="b in ACP_BACKENDS"
                      :key="b.id"
                      type="button"
                      class="border px-3 py-3.5 text-center transition"
                      :class="draft.acpBackend === b.id ? 'border-accent bg-accent-dim' : 'border-line bg-base hover:border-line-strong'"
                      @click="selectAcp(b.id)"
                    >
                      <strong class="block text-[13px] font-semibold text-txt">{{ b.label }}</strong>
                      <span class="mt-1 block font-mono text-[10px] text-txt3">{{ b.configRoot }}</span>
                    </button>
                  </div>
                  <div v-if="currentRegionPolicy" class="mt-5 border-t border-dashed border-line pt-4">
                    <div class="mb-2 text-[12px] font-medium text-txt2">
                      {{ t('pages.agentStudio.region.title') }}
                    </div>
                    <div class="grid max-w-lg grid-cols-2 gap-2.5" role="radiogroup" :aria-label="t('pages.agentStudio.region.title')">
                      <button
                        v-for="option in currentRegionPolicy.options"
                        :key="option.id"
                        type="button"
                        role="radio"
                        :aria-checked="currentRegion === option.id"
                        :aria-label="`${t(option.labelKey)} (${option.id})`"
                        class="border px-3 py-3 text-left transition"
                        :class="currentRegion === option.id ? 'border-accent bg-accent-dim' : 'border-line bg-base hover:border-line-strong'"
                        @click="selectRegion(option.id)"
                      >
                        <strong class="block text-[13px] font-semibold text-txt">{{ t(option.labelKey) }}</strong>
                        <span class="mt-1 block font-mono text-[10px] text-accent-2">{{ option.id }}</span>
                        <span class="mt-1 block text-[10px] text-txt3">{{ t(option.hintKey) }}</span>
                      </button>
                    </div>
                  </div>
                  <p class="mt-3 font-mono text-[11px] text-txt3">
                    configRoot → <span class="text-accent-2">{{ draft.configRoot }}</span>
                  </p>
                </template>

                <!-- git -->
                <template v-else-if="currentStep.id === 'git'">
                  <p class="sec-meta">{{ t('pages.agentStudio.wizard.git.meta') }}</p>
                  <AgentGitGuide
                    :env="draft.env"
                    :upsert-env="(k, v) => { upsertEnv(k, v); markConfigured('git') }"
                    :credential-type="draft.gitCredentialType"
                    @update:credential-type="draft.gitCredentialType = $event; markConfigured('git')"
                  />
                </template>

                <!-- env -->
                <template v-else-if="currentStep.id === 'env'">
                  <p class="sec-meta">{{ t('pages.agentStudio.wizard.env.meta') }}</p>
                  <div v-if="currentRegionPolicy" class="mb-3 border border-accent/30 bg-accent-dim/40 p-3">
                    <div class="text-[11px] text-txt3">{{ t('pages.agentStudio.region.managedByAcp') }}</div>
                    <div class="mt-2 flex items-center gap-1.5">
                      <input
                        :value="currentRegionPolicy.regionEnvKey"
                        readonly
                        :aria-label="t('pages.agentStudio.region.managedKey')"
                        class="w-1/3 border border-line bg-elevated px-2 py-1.5 font-mono text-[12px] text-txt3"
                      />
                      <input
                        :value="currentRegion"
                        readonly
                        :aria-label="t('pages.agentStudio.region.managedValue')"
                        class="flex-1 border border-line bg-elevated px-2 py-1.5 font-mono text-[12px] text-txt3"
                      />
                    </div>
                  </div>
                  <p v-if="regionManagedNotice" class="mb-3 text-[11px] text-warn">
                    {{ t('pages.agentStudio.region.managedConflict') }}
                  </p>
                  <div class="space-y-2">
                    <template v-for="(e, i) in draft.env" :key="i">
                      <div v-if="showInEnvStep(e.k)" class="flex items-center gap-1.5">
                        <input
                          :value="e.k"
                          placeholder="KEY"
                          class="w-1/3 border border-line bg-base px-2 py-1.5 font-mono text-[12px] text-txt outline-none focus:border-accent"
                          @input="updateEnvKey(i, ($event.target as HTMLInputElement).value)"
                        />
                        <input
                          v-model="e.v"
                          placeholder="value"
                          class="flex-1 border border-line bg-base px-2 py-1.5 font-mono text-[12px] text-txt2 outline-none focus:border-accent"
                          @input="markConfigured('env')"
                        />
                        <button type="button" class="text-txt3 hover:text-err" @click="removeEnv(i)">
                          <Icon name="close" :size="14" />
                        </button>
                      </div>
                    </template>
                    <AppButton size="sm" variant="outline" icon="plus" @click="addEnvRow">{{ t('pages.agentStudio.env.add') }}</AppButton>
                  </div>
                </template>

                <!-- mcp -->
                <template v-else-if="currentStep.id === 'mcp'">
                  <p class="sec-meta">{{ t('pages.agentStudio.wizard.mcp.meta') }}</p>
                  <div class="mb-3 border border-accent/30 bg-accent-dim/40 p-2.5 text-[11px] leading-5 text-txt2">
                    <button
                      v-if="!draft.mcp.some((m) => m.name.trim() === 'artifact-store')"
                      type="button"
                      class="border border-accent/40 px-2 py-1 text-accent-2 hover:bg-accent-dim"
                      @click="addArtifactStore"
                    >
                      {{ t('pages.agentStudio.mcp.addArtifactStore') }}
                    </button>
                    <span v-else class="text-txt3">{{ t('pages.agentStudio.wizard.mcp.hasArtifact') }}</span>
                  </div>
                  <div class="space-y-3">
                    <div v-for="(m, i) in draft.mcp" :key="i" class="border border-line bg-base p-3">
                      <div class="mb-2 flex items-center gap-2">
                        <input
                          v-model="m.name"
                          :placeholder="t('pages.agentStudio.mcp.serviceName')"
                          class="flex-1 border border-line bg-surface px-2 py-1 text-[12px] text-txt outline-none focus:border-accent"
                          @input="markConfigured('mcp')"
                        />
                        <select v-model="m.transport" class="border border-line bg-surface px-2 py-1 text-[12px] text-txt2" @change="markConfigured('mcp')">
                          <option value="url">HTTP (url)</option>
                          <option value="command">{{ t('pages.agentStudio.mcp.transportCommand') }}</option>
                        </select>
                        <button type="button" class="text-txt3 hover:text-err" @click="removeMcp(i)"><Icon name="close" :size="14" /></button>
                      </div>
                      <template v-if="m.transport === 'url'">
                        <input
                          v-model="m.url"
                          placeholder="https://mcp.example.com/sse"
                          class="mb-2 w-full border border-line bg-surface px-2 py-1 font-mono text-[11px] outline-none focus:border-accent"
                          @input="markConfigured('mcp')"
                        />
                        <div class="text-[11px] text-txt3">{{ t('pages.agentStudio.mcp.headers') }}</div>
                        <div v-for="(h, hi) in m.headers" :key="hi" class="mt-1 flex items-center gap-1.5">
                          <input v-model="h.k" placeholder="Authorization" class="w-1/3 border border-line bg-surface px-2 py-1 font-mono text-[11px] outline-none" @input="markConfigured('mcp')" />
                          <input v-model="h.v" placeholder="Bearer …" class="flex-1 border border-line bg-surface px-2 py-1 font-mono text-[11px] outline-none" @input="markConfigured('mcp')" />
                          <button type="button" class="text-txt3 hover:text-err" @click="m.headers.splice(hi, 1)"><Icon name="close" :size="12" /></button>
                        </div>
                        <button type="button" class="mt-1.5 text-[11px] text-accent-2 hover:underline" @click="m.headers.push({ k: '', v: '' }); markConfigured('mcp')">
                          {{ t('pages.agentStudio.mcp.addHeader') }}
                        </button>
                      </template>
                      <template v-else>
                        <input
                          v-model="m.command"
                          placeholder="npx"
                          class="mb-2 w-full border border-line bg-surface px-2 py-1 font-mono text-[11px] outline-none focus:border-accent"
                          @input="markConfigured('mcp')"
                        />
                        <div class="text-[11px] text-txt3">{{ t('pages.agentStudio.mcp.args') }}</div>
                        <textarea
                          v-model="m.args"
                          rows="2"
                          placeholder="-y&#10;@upstash/context7-mcp"
                          class="mt-1 w-full resize-y border border-line bg-surface px-2 py-1 font-mono text-[11px] outline-none"
                          @input="markConfigured('mcp')"
                        />
                        <div class="mt-2 text-[11px] text-txt3">{{ t('pages.agentStudio.mcp.env') }}</div>
                        <div v-for="(e, ei) in m.env" :key="ei" class="mt-1 flex items-center gap-1.5">
                          <input
                            v-model="e.k"
                            placeholder="KEY"
                            class="w-1/3 border border-line bg-surface px-2 py-1 font-mono text-[11px] outline-none"
                            @input="markConfigured('mcp')"
                          />
                          <input
                            v-model="e.v"
                            placeholder="value"
                            class="flex-1 border border-line bg-surface px-2 py-1 font-mono text-[11px] outline-none"
                            @input="markConfigured('mcp')"
                          />
                          <button type="button" class="text-txt3 hover:text-err" @click="m.env.splice(ei, 1); markConfigured('mcp')">
                            <Icon name="close" :size="12" />
                          </button>
                        </div>
                        <button
                          type="button"
                          class="mt-1.5 text-[11px] text-accent-2 hover:underline"
                          @click="m.env.push({ k: '', v: '' }); markConfigured('mcp')"
                        >
                          {{ t('pages.agentStudio.mcp.addEnv') }}
                        </button>
                      </template>
                    </div>
                    <AppButton size="sm" variant="outline" icon="plus" @click="addMcp">{{ t('pages.agentStudio.mcp.addService') }}</AppButton>
                  </div>
                </template>

                <!-- rules -->
                <template v-else-if="currentStep.id === 'rules'">
                  <p class="sec-meta">{{ t('pages.agentStudio.wizard.rules.meta') }}</p>
                  <textarea
                    v-model="draft.rulesContent"
                    rows="14"
                    class="w-full resize-y border border-line bg-base px-3 py-2 font-mono text-[12px] leading-6 text-txt outline-none focus:border-accent"
                    @input="onRulesInput"
                  />
                </template>

                <!-- skills -->
                <template v-else-if="currentStep.id === 'skills'">
                  <p class="sec-meta">{{ t('pages.agentStudio.wizard.skills.meta') }}</p>
                  <div class="mb-3 flex gap-2">
                    <input
                      v-model="newSkillName"
                      :placeholder="t('pages.agentStudio.wizard.skills.namePlaceholder')"
                      class="flex-1 border border-line bg-base px-3 py-2 font-mono text-[12px] outline-none focus:border-accent"
                      @keydown.enter.prevent="addSkill"
                    />
                    <AppButton size="sm" variant="outline" icon="plus" @click="addSkill">{{ t('pages.agentStudio.wizard.skills.add') }}</AppButton>
                  </div>
                  <div class="space-y-3">
                    <div v-for="(s, i) in draft.skills" :key="s.name" class="border border-line bg-base p-3">
                      <div class="mb-2 flex items-center justify-between">
                        <code class="font-mono text-[12px] text-accent-2">skills/{{ s.name }}/SKILL.md</code>
                        <button type="button" class="text-txt3 hover:text-err" @click="removeSkill(i)"><Icon name="close" :size="14" /></button>
                      </div>
                      <textarea
                        v-model="s.content"
                        rows="6"
                        class="w-full resize-y border border-line bg-surface px-2 py-1.5 font-mono text-[11px] outline-none focus:border-accent"
                        @input="markConfigured('skills')"
                      />
                    </div>
                  </div>
                </template>

                <!-- commands -->
                <template v-else-if="currentStep.id === 'commands'">
                  <p class="sec-meta">{{ t('pages.agentStudio.wizard.commands.meta') }}</p>
                  <div class="mb-3 flex gap-2">
                    <input
                      v-model="newCmdName"
                      :placeholder="t('pages.agentStudio.wizard.commands.namePlaceholder')"
                      class="flex-1 border border-line bg-base px-3 py-2 font-mono text-[12px] outline-none focus:border-accent"
                      @keydown.enter.prevent="addCommand"
                    />
                    <AppButton size="sm" variant="outline" icon="plus" @click="addCommand">{{ t('pages.agentStudio.wizard.commands.add') }}</AppButton>
                  </div>
                  <div class="space-y-3">
                    <div v-for="(c, i) in draft.commands" :key="c.name" class="border border-line bg-base p-3">
                      <div class="mb-2 flex items-center justify-between">
                        <code class="font-mono text-[12px] text-accent-2">commands/{{ c.name }}.md</code>
                        <button type="button" class="text-txt3 hover:text-err" @click="removeCommand(i)"><Icon name="close" :size="14" /></button>
                      </div>
                      <textarea
                        v-model="c.content"
                        rows="6"
                        class="w-full resize-y border border-line bg-surface px-2 py-1.5 font-mono text-[11px] outline-none focus:border-accent"
                        @input="markConfigured('commands')"
                      />
                    </div>
                  </div>
                </template>

                <!-- prompts -->
                <template v-else-if="currentStep.id === 'prompts'">
                  <p class="sec-meta">{{ t('pages.agentStudio.wizard.prompts.meta') }}</p>
                  <div class="space-y-4">
                    <label v-for="f in promptFragments" :key="f.key" class="block">
                      <span class="text-[12px] font-medium text-txt2">{{ f.label }}</span>
                      <p class="mb-1.5 text-[11px] text-txt3">{{ f.hint }}</p>
                      <textarea
                        v-model="draft.prompts[f.key]"
                        rows="3"
                        spellcheck="false"
                        :placeholder="t('pages.agentStudio.prompts.defaultPrefix') + f.placeholder"
                        class="w-full resize-y border border-line bg-base px-3 py-2 font-mono text-[12px] leading-6 outline-none focus:border-accent"
                        @input="onPromptInput"
                      />
                    </label>
                  </div>
                </template>

                <!-- review -->
                <template v-else-if="currentStep.id === 'review'">
                  <p class="sec-meta">{{ t('pages.agentStudio.wizard.review.meta') }}</p>
                  <div class="flex flex-wrap gap-2">
                    <span
                      v-for="item in reviewItems"
                      :key="item.key"
                      class="inline-flex items-center gap-1 border px-2 py-1 text-[11px]"
                      :class="chipClass(item.kind)"
                    >
                      {{ t(item.labelKey) }}
                      <template v-if="item.detail"> · {{ item.detail }}</template>
                    </span>
                  </div>
                  <p v-if="createError" class="mt-4 text-[12px] text-err">{{ createError }}</p>
                </template>
              </div>
            </div>

            <!-- foot -->
            <div class="flex shrink-0 items-center justify-between gap-2 border-t border-line bg-surface px-5 py-3.5">
              <AppButton variant="ghost" :disabled="creating" @click="close">
                {{ t('pages.agentStudio.dialogs.cancel') }}
              </AppButton>
              <div class="ml-auto flex items-center gap-2">
                <AppButton variant="outline" :disabled="draft.step === 0 || creating" @click="goPrev">
                  {{ t('pages.agentStudio.wizard.prev') }}
                </AppButton>
                <AppButton v-if="currentStep.skip" variant="outline" :disabled="creating" @click="goSkip">
                  {{ t('pages.agentStudio.wizard.skip') }}
                </AppButton>
                <AppButton variant="primary" :disabled="creating" @click="goNext">
                  {{
                    creating
                      ? t('pages.agentStudio.wizard.creating')
                      : currentStep.id === 'review'
                        ? t('pages.agentStudio.wizard.create')
                        : t('pages.agentStudio.wizard.next')
                  }}
                </AppButton>
              </div>
            </div>
          </div>
        </div>

        <div v-if="creating" class="absolute inset-0 z-20 grid place-items-center bg-black/50">
          <div class="border border-line bg-surface px-6 py-4 text-[13px] text-txt2">
            {{ t('pages.agentStudio.wizard.creating') }}…
          </div>
        </div>
      </div>

      <!-- ACP remapping confirm -->
      <div v-if="showAcpConfirm" class="fixed inset-0 z-[60] flex items-center justify-center p-4">
        <div class="absolute inset-0 bg-black/60" @click="cancelAcpSwitch" />
        <div class="relative z-10 w-full max-w-[420px] border border-line bg-surface p-5 shadow-card" style="border-radius: 0">
          <h3 class="m-0 text-[15px] font-semibold text-txt">{{ t('pages.agentStudio.wizard.acp.remapTitle') }}</h3>
          <p class="mt-2 text-[13px] leading-6 text-txt2">{{ t('pages.agentStudio.wizard.acp.remapBody') }}</p>
          <div class="mt-4 flex justify-end gap-2">
            <AppButton variant="outline" @click="cancelAcpSwitch">{{ t('pages.agentStudio.dialogs.cancel') }}</AppButton>
            <AppButton variant="primary" @click="confirmAcpSwitch">{{ t('pages.agentStudio.dialogs.confirm') }}</AppButton>
          </div>
        </div>
      </div>
    </div>
  </Teleport>
</template>

<style scoped>
.wiz-head {
  background: linear-gradient(90deg, rgba(123, 97, 255, 0.12) 0%, transparent 42%), rgb(var(--c-surface));
}
.hero-mark {
  background: linear-gradient(145deg, rgba(123, 97, 255, 0.22), rgb(var(--c-elevated)));
  border-radius: 0;
}
.wiz-progress span {
  display: block;
  height: 100%;
  position: relative;
  background: linear-gradient(90deg, #6d4dff 0%, #7b61ff 40%, #a78bfa 78%, #818cf8 100%);
  box-shadow: 0 0 14px rgba(123, 97, 255, 0.55);
  transition: width 0.55s cubic-bezier(0.4, 0, 0.2, 1);
}
.wiz-progress span::before {
  content: '';
  position: absolute;
  inset: -2px 0;
  background: inherit;
  filter: blur(6px);
  opacity: 0.55;
  animation: progPulse 1.8s ease-in-out infinite;
}
.wiz-progress span::after {
  content: '';
  position: absolute;
  top: 0;
  bottom: 0;
  left: 0;
  width: 42%;
  background: linear-gradient(90deg, transparent, rgba(255, 255, 255, 0.42), transparent);
  animation: progShine 1.7s ease-in-out infinite;
}
@keyframes progPulse {
  0%,
  100% {
    opacity: 0.35;
  }
  50% {
    opacity: 0.75;
  }
}
@keyframes progShine {
  0% {
    transform: translateX(-130%);
    opacity: 0;
  }
  35% {
    opacity: 1;
  }
  100% {
    transform: translateX(260%);
    opacity: 0;
  }
}
.pulse-dot {
  animation: railCapPulse 1.6s ease-in-out infinite;
}
@keyframes railCapPulse {
  0%,
  100% {
    box-shadow: 0 0 0 0 rgba(123, 97, 255, 0.5);
    opacity: 1;
  }
  50% {
    box-shadow: 0 0 0 5px rgba(123, 97, 255, 0);
    opacity: 0.72;
  }
}
.rail-item {
  position: relative;
  display: flex;
  align-items: stretch;
  gap: 10px;
  width: 100%;
  min-height: 40px;
  padding: 0 2px;
  color: rgb(var(--c-txt3));
  font-size: 13px;
}
.rail-item .track {
  position: relative;
  width: 18px;
  flex-shrink: 0;
  display: flex;
  flex-direction: column;
  align-items: center;
}
.rail-item .node {
  position: relative;
  z-index: 1;
  width: 14px;
  height: 14px;
  flex-shrink: 0;
  margin-top: 2px;
  border: 1.5px solid rgb(var(--c-line-strong));
  background: transparent;
  display: grid;
  place-items: center;
  transition:
    border-color 0.3s ease,
    background 0.3s ease,
    box-shadow 0.3s ease;
}
.rail-item .node i {
  display: block;
  width: 6px;
  height: 6px;
  background: transparent;
}
.rail-item .connector {
  width: 1px;
  flex: 1;
  min-height: 14px;
  margin: 4px 0 0;
  background: rgb(var(--c-line));
  transition: background 0.45s ease;
}
.rail-item:last-child .connector {
  display: none;
}
.rail-item .lbl {
  flex: 1;
  min-width: 0;
  padding: 0 0 12px;
  display: flex;
  align-items: center;
}
.rail-item .lbl strong {
  font-size: 13px;
  font-weight: 500;
  line-height: 1.3;
  letter-spacing: -0.01em;
  transition: color 0.25s ease;
}
.rail-item.done {
  color: rgb(var(--c-txt2));
}
.rail-item.done .node {
  border-color: rgba(52, 211, 153, 0.55);
  animation: railDonePop 0.4s cubic-bezier(0.22, 1, 0.36, 1);
}
.rail-item.done .node i {
  background: #34d393;
}
.rail-item.done .connector {
  background: rgba(52, 211, 153, 0.35);
}
.rail-item.done .lbl strong {
  color: rgb(var(--c-txt2));
}
.rail-item.cur {
  color: rgb(var(--c-txt));
}
.rail-item.cur .node {
  border-color: #7b61ff;
  background: rgba(123, 97, 255, 0.18);
  animation: railNodePulse 2s ease-in-out infinite;
}
.rail-item.cur .node i {
  background: rgb(var(--c-accent-2));
}
.rail-item.cur .lbl strong {
  color: rgb(var(--c-txt));
  font-weight: 600;
}
@keyframes railNodePulse {
  0%,
  100% {
    box-shadow: 0 0 0 0 rgba(123, 97, 255, 0.45);
  }
  55% {
    box-shadow: 0 0 0 7px rgba(123, 97, 255, 0);
  }
}
@keyframes railDonePop {
  0% {
    transform: scale(0.72);
    opacity: 0.55;
  }
  70% {
    transform: scale(1.08);
  }
  100% {
    transform: scale(1);
    opacity: 1;
  }
}
.sec-head {
  display: block;
  margin: 0 0 8px;
}
.sec-bar {
  display: block;
  padding: 0;
  border: 0;
  background: none;
  min-height: 0;
  position: static;
}
.sec-bar h3 {
  margin: 0;
  font-size: 15px;
  font-weight: 600;
  line-height: 1.3;
  letter-spacing: -0.01em;
  color: rgb(var(--c-txt));
}
.sec-meta {
  margin: 0 0 20px;
  font-size: 13px;
  color: rgb(var(--c-txt2));
  line-height: 1.65;
  max-width: 38rem;
}
.step-pane {
  animation: stepIn 0.38s cubic-bezier(0.22, 1, 0.36, 1);
}
@keyframes stepIn {
  from {
    opacity: 0;
    transform: translateY(10px);
  }
  to {
    opacity: 1;
    transform: none;
  }
}

/* 浅色：降低装饰性 glow / 渐变不透明度（对齐 page.html mode-fixed） */
html.light .wiz-head {
  background: linear-gradient(90deg, rgba(99, 102, 241, 0.08) 0%, transparent 42%), rgb(var(--c-surface));
}
html.light .hero-mark {
  color: #6366f1;
  background: linear-gradient(145deg, rgba(99, 102, 241, 0.14), rgb(var(--c-elevated)));
  border-color: rgba(99, 102, 241, 0.45);
}
html.light .wiz-progress span {
  box-shadow: 0 0 10px rgba(99, 102, 241, 0.28);
}
html.light .wiz-progress span::before {
  opacity: 0.32;
}
</style>
