<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/ui/Icon.vue'
import AppButton from '@/components/ui/AppButton.vue'
import AppModal from '@/components/ui/AppModal.vue'
import type { AgentOrg } from '@/lib/api/api'
import {
  groupIdsOf,
  groupPath,
  setAgentMembership,
} from '@/lib/agent/agentOrg'
import {
  ACP_BACKENDS,
  getRegionPolicy,
  normalizeRegions,
  setRegion,
  switchBackendRegions,
  type BackendId,
} from '@/lib/shared/regionPolicy'
import {
  DEFAULT_CONFIG_ROOT,
  DEFAULT_WORKSPACE_DIR,
  defaultConfigRootFor,
  kvToRec,
  recToKV,
  type AgentStudioDraft,
} from '@/lib/agent/agentStudioDraft'

const props = defineProps<{
  draft: AgentStudioDraft
  org: AgentOrg
  agentName: string
  agentNames: string[]
  projects: { id: string; name: string }[]
  isProjectBound: boolean
}>()

const emit = defineEmits<{
  'update:org': [value: AgentOrg]
  error: [message: string]
}>()

const { t } = useI18n()

let configRootTouched = false

const pendingProjectId = ref<string | null>(null)
const showProjectSwitch = ref(false)

watch(
  () => props.agentName,
  () => {
    configRootTouched = false
  },
)

function projectNameById(id: string): string {
  return props.projects.find((p) => p.id === id)?.name || id
}

const projectSelectValue = computed<string>({
  get: () => props.draft.projectId || '',
  set: (val) => {
    const old = props.draft.projectId || ''
    if (val === old) return
    if (old) {
      pendingProjectId.value = val
      showProjectSwitch.value = true
      return
    }
    props.draft.projectId = val
  },
})

const pendingProjectLabel = computed(() =>
  pendingProjectId.value ? projectNameById(pendingProjectId.value) : '',
)

function confirmProjectChange() {
  if (pendingProjectId.value !== null) {
    props.draft.projectId = pendingProjectId.value
  }
  pendingProjectId.value = null
  showProjectSwitch.value = false
}

function cancelProjectChange() {
  pendingProjectId.value = null
  showProjectSwitch.value = false
}

const metaGroupTiles = computed(() => {
  const list = [...(props.org.groups || [])]
  list.sort((a, b) => groupPath(props.org, a.id).localeCompare(groupPath(props.org, b.id)))
  return list.map((g) => ({
    id: g.id,
    name: g.name,
    path: groupPath(props.org, g.id),
    selected: props.agentName ? groupIdsOf(props.org, props.agentName).includes(g.id) : false,
  }))
})

function toggleMetaGroup(groupId: string) {
  if (!props.agentName) return
  const cur = groupIdsOf(props.org, props.agentName)
  const next = cur.includes(groupId) ? cur.filter((id) => id !== groupId) : [...cur, groupId]
  emit('update:org', setAgentMembership(props.org, props.agentName, next))
}

function selectAcpBackend(id: BackendId) {
  const prev = props.draft.acpBackend
  props.draft.acpBackend = id
  if (!configRootTouched) {
    props.draft.layout.configRoot = defaultConfigRootFor(id)
  }
  if (prev !== id) props.draft.env = recToKV(switchBackendRegions(kvToRec(props.draft.env), id))
}

const currentRegionPolicy = computed(() => getRegionPolicy(props.draft.acpBackend))
const showMetaRegionBlock = computed(() => !!currentRegionPolicy.value)
const metaRegionOptions = computed(() => currentRegionPolicy.value?.options || [])

const displayRegion = computed(() => {
  const normalized = normalizeRegions(kvToRec(props.draft.env), props.draft.acpBackend, 'preserve-special')
  return normalized.special ? '' : normalized.region
})

const specialRegion = computed(() => {
  const normalized = normalizeRegions(kvToRec(props.draft.env), props.draft.acpBackend, 'preserve-special')
  return normalized.special ? normalized.region : ''
})

function selectRegion(id: string) {
  props.draft.env = recToKV(setRegion(kvToRec(props.draft.env), props.draft.acpBackend, id))
}

function joinConfigPath(root: string, sub: string): string {
  return (root || DEFAULT_CONFIG_ROOT).replace(/\/+$/, '') + '/' + sub
}

const derivedPaths = computed(() => {
  const root = props.draft.layout.configRoot || DEFAULT_CONFIG_ROOT
  return [
    { label: t('pages.agentStudio.configPaths.mcp'), path: joinConfigPath(root, 'mcp.json'), note: t('pages.agentStudio.configPaths.mcpNote') },
    { label: t('pages.agentStudio.configPaths.rules'), path: joinConfigPath(root, 'rules/'), note: t('pages.agentStudio.configPaths.rulesNote') },
    { label: t('pages.agentStudio.configPaths.skills'), path: joinConfigPath(root, 'skills/'), note: t('pages.agentStudio.configPaths.skillsNote') },
    { label: t('pages.agentStudio.configPaths.commands'), path: joinConfigPath(root, 'commands/'), note: t('pages.agentStudio.configPaths.commandsNote') },
    { label: t('pages.agentStudio.configPaths.env'), path: 'container-env', note: t('pages.agentStudio.configPaths.envNote') },
  ]
})
</script>

<template>
  <div class="scroll-area min-h-0 flex-1 overflow-auto p-4">
    <div class="mb-4 max-w-3xl">
      <h3 class="text-sm font-semibold text-txt">{{ t('pages.agentStudio.org.metaTitle') }}</h3>
      <p class="mt-1 text-[12px] leading-6 text-txt3">{{ t('pages.agentStudio.org.metaIntro') }}</p>
      <p class="mt-2 border-l-2 border-accent-2 bg-accent-dim px-2.5 py-1.5 text-[11px] leading-5 text-txt2">
        {{ t('pages.agentStudio.org.metaRenameHint') }}
      </p>
    </div>

    <div class="mb-8 max-w-3xl">
      <div class="mb-1 text-[12px] font-medium text-txt2">{{ t('pages.agentStudio.project.label') }}</div>
      <p class="mb-2.5 text-[11px] leading-5 text-txt3">{{ t('pages.agentStudio.project.hint') }}</p>
      <select
        v-model="projectSelectValue"
        data-test="agent-project-select"
        class="max-w-sm w-full rounded border border-line bg-surface px-2 py-1.5 text-[12px] text-txt outline-none focus:border-accent"
      >
        <option value="">{{ t('pages.agentStudio.project.unbound') }}</option>
        <option v-for="p in projects" :key="p.id" :value="p.id">{{ p.name }}</option>
      </select>
      <p v-if="!isProjectBound" class="mt-2 rounded border border-dashed border-warn/40 bg-warn/10 px-2.5 py-1.5 text-[11px] leading-5 text-warn">
        {{ t('pages.agentStudio.project.unboundWarn') }}
      </p>
    </div>

    <div class="mb-8 max-w-3xl space-y-5">
      <div>
        <div class="mb-1 flex items-center justify-between gap-2 text-[12px] font-medium text-txt2">
          <span>{{ t('pages.agentStudio.org.groupsLabel') }}</span>
          <span class="text-[11px] font-normal text-txt3">{{ t('pages.agentStudio.org.groupsMeta') }}</span>
        </div>
        <p class="mb-2.5 text-[11px] leading-5 text-txt3">{{ t('pages.agentStudio.org.groupsHint') }}</p>
        <div v-if="metaGroupTiles.length" class="grid grid-cols-1 gap-2 sm:grid-cols-2">
          <button
            v-for="tile in metaGroupTiles"
            :key="tile.id"
            type="button"
            class="flex min-h-[56px] items-start gap-2.5 border px-3 py-2.5 text-left transition"
            :class="tile.selected
              ? 'border-accent bg-accent-dim text-txt shadow-[inset_0_0_0_1px_rgba(99,102,241,0.25)]'
              : 'border-line bg-base text-txt2 hover:border-line-strong hover:bg-elevated hover:text-txt'"
            @click="toggleMetaGroup(tile.id)"
          >
            <span
              class="mt-0.5 flex h-4 w-4 shrink-0 items-center justify-center border"
              :class="tile.selected ? 'border-accent bg-accent text-white' : 'border-line-strong bg-surface'"
            >
              <Icon v-if="tile.selected" name="check" :size="11" />
            </span>
            <span class="min-w-0 flex-1">
              <span class="block truncate text-[12.5px] font-semibold">{{ tile.name }}</span>
              <span
                class="mt-0.5 block truncate text-[10.5px]"
                :class="tile.selected ? 'text-accent-2/80' : 'text-txt3'"
              >{{ tile.path }}</span>
            </span>
          </button>
        </div>
        <p v-else class="border border-dashed border-line px-3 py-3 text-[12px] text-txt3">
          {{ t('pages.agentStudio.org.noGroups') }}
        </p>
      </div>
    </div>

    <div class="mb-4 max-w-3xl border-t border-line pt-5">
      <h3 class="text-sm font-semibold text-txt">{{ t('pages.agentStudio.meta.layoutTitle') }}</h3>
      <p class="mt-1 text-[12px] leading-6 text-txt3" v-html="t('pages.agentStudio.meta.layoutIntro')" />
    </div>

    <div class="max-w-3xl space-y-4">
      <div>
        <div class="text-[12px] font-medium text-txt2">{{ t('pages.agentStudio.meta.acpBackend') }}</div>
        <p class="mb-2 text-[11px] text-txt3">{{ t('pages.agentStudio.meta.acpBackendDesc') }}</p>
        <div class="grid grid-cols-2 gap-2 sm:grid-cols-4">
          <button
            v-for="b in ACP_BACKENDS"
            :key="b.id"
            type="button"
            class="border px-2 py-3 text-center transition"
            :class="draft.acpBackend === b.id ? 'border-accent bg-accent-dim text-txt' : 'border-line bg-base text-txt2 hover:border-line-strong'"
            @click="selectAcpBackend(b.id)"
          >
            <div class="text-[12px] font-semibold">{{ b.label }}</div>
            <div class="mt-0.5 font-mono text-[10px] text-txt3">{{ b.id }}</div>
          </button>
        </div>
      </div>
      <div v-if="showMetaRegionBlock" class="border-t border-dashed border-line pt-4">
        <div class="text-[12px] font-medium text-txt2">
          {{ t('pages.agentStudio.meta.regionTitle') }}
          <span class="ml-1.5 inline-block border border-accent/30 bg-accent-dim px-1.5 py-0.5 text-[9px] font-bold uppercase tracking-wider text-accent-2">{{ t('pages.agentStudio.meta.regionNewBadge') }}</span>
        </div>
        <p class="mb-2 text-[11px] text-txt3">{{ t('pages.agentStudio.meta.regionDesc') }}</p>
        <p v-if="specialRegion" class="mb-2 border border-warn/35 bg-warn/10 px-2.5 py-2 font-mono text-[11px] text-warn">
          {{ t('pages.agentStudio.region.special', { value: specialRegion }) }}
        </p>
        <div class="grid max-w-md grid-cols-2 gap-2" role="radiogroup" :aria-label="t('pages.agentStudio.region.title')">
          <button
            v-for="r in metaRegionOptions"
            :key="r.id"
            type="button"
            class="border px-2 py-3 text-center transition"
            :class="displayRegion === r.id ? 'border-accent bg-accent-dim text-txt' : 'border-line bg-base text-txt2 hover:border-line-strong'"
            role="radio"
            :aria-checked="displayRegion === r.id"
            :aria-label="`${t(r.labelKey)} (${r.id})`"
            @click="selectRegion(r.id)"
          >
            <div class="text-[12px] font-semibold">{{ t(r.labelKey) }}</div>
            <div class="mt-0.5 font-mono text-[10px] text-txt3">{{ r.id }}</div>
            <div class="mt-1.5 text-[10px] leading-snug" :class="displayRegion === r.id ? 'text-accent-2' : 'text-txt3'">{{ t(r.hintKey) }}</div>
          </button>
        </div>
      </div>
      <label class="block">
        <span class="text-[12px] font-medium text-txt2">{{ t('pages.agentStudio.meta.configRoot') }}</span>
        <p class="mb-1.5 text-[11px] text-txt3">{{ t('pages.agentStudio.meta.configRootDesc') }}</p>
        <input
          v-model="draft.layout.configRoot"
          :placeholder="defaultConfigRootFor(draft.acpBackend)"
          spellcheck="false"
          class="w-full rounded-md border border-line bg-base px-3 py-2 font-mono text-[12px] text-txt outline-none focus:border-accent"
          @input="configRootTouched = true"
        />
      </label>
      <label class="block">
        <span class="text-[12px] font-medium text-txt2">{{ t('pages.agentStudio.meta.workspaceDir') }}</span>
        <p class="mb-1.5 text-[11px] text-txt3">{{ t('pages.agentStudio.meta.workspaceDirDesc') }}</p>
        <input
          v-model="draft.layout.workspaceDir"
          :placeholder="DEFAULT_WORKSPACE_DIR"
          spellcheck="false"
          class="w-full rounded-md border border-line bg-base px-3 py-2 font-mono text-[12px] text-txt outline-none focus:border-accent"
        />
      </label>
    </div>

    <div class="mt-8 max-w-3xl border-t border-line pt-5">
      <h3 class="text-sm font-semibold text-txt">{{ t('pages.agentStudio.meta.sshTitle') }}</h3>
      <p class="mt-1 text-[12px] leading-6 text-txt3">{{ t('pages.agentStudio.meta.sshIntro') }}</p>
      <p class="mt-2 border-l-2 border-accent-2 bg-accent-dim px-2.5 py-1.5 text-[11px] leading-5 text-txt2">
        {{ t('pages.agentStudio.meta.sshNoVars') }}
      </p>
      <div class="mt-4 space-y-4">
        <label class="block">
          <span class="text-[12px] font-medium text-txt2">{{ t('pages.agentStudio.meta.sshKnownHosts') }}</span>
          <p class="mb-1.5 text-[11px] text-txt3">{{ t('pages.agentStudio.meta.sshKnownHostsDesc') }}</p>
          <textarea
            :value="draft.gitSshKnownHosts || ''"
            data-test="agent-ssh-known-hosts"
            rows="4"
            spellcheck="false"
            class="w-full rounded-md border border-line bg-base px-3 py-2 font-mono text-[12px] text-txt outline-none focus:border-accent"
            :placeholder="t('pages.agentStudio.meta.sshKnownHostsPh')"
            @input="draft.gitSshKnownHosts = ($event.target as HTMLTextAreaElement).value"
          />
        </label>
        <label class="block">
          <span class="text-[12px] font-medium text-txt2">{{ t('pages.agentStudio.meta.sshPrivateKey') }}</span>
          <p class="mb-1.5 text-[11px] text-txt3">{{ t('pages.agentStudio.meta.sshPrivateKeyDesc') }}</p>
          <textarea
            :value="draft.gitSshPrivateKey || ''"
            data-test="agent-ssh-private-key"
            rows="5"
            spellcheck="false"
            class="w-full rounded-md border border-line bg-base px-3 py-2 font-mono text-[12px] text-txt outline-none focus:border-accent"
            :placeholder="t('pages.agentStudio.meta.sshPrivateKeyPh')"
            :style="draft.gitSshPrivateKey ? { WebkitTextSecurity: 'disc' } as Record<string, string> : undefined"
            @input="draft.gitSshPrivateKey = ($event.target as HTMLTextAreaElement).value"
          />
        </label>
      </div>
    </div>

    <div class="mt-5 max-w-3xl">
      <div class="mb-1.5 text-[11px] uppercase tracking-wider text-txt3">{{ t('pages.agentStudio.meta.derivedPaths') }}</div>
      <div class="overflow-hidden rounded-lg border border-line">
        <table class="w-full text-left text-[12px]">
          <tbody>
            <tr v-for="(e, i) in derivedPaths" :key="e.label" :class="i % 2 ? 'bg-base/40' : ''">
              <td class="px-3 py-2 text-txt2">{{ e.label }}</td>
              <td class="px-3 py-2"><code class="rounded bg-base px-1.5 py-0.5 font-mono text-accent-2">{{ e.path }}</code></td>
              <td class="px-3 py-2 text-txt3">{{ e.note }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <p class="mt-3 max-w-3xl text-[11px] leading-5 text-txt3">
      {{ t('pages.agentStudio.meta.capabilitiesNote') }}
    </p>

    <AppModal :open="showProjectSwitch" :title="t('pages.agentStudio.project.switchTitle')" :width="460" @close="cancelProjectChange">
      <div class="space-y-2 text-[13px] leading-6 text-txt2">
        <p>{{ t('pages.agentStudio.project.switchWarn') }}</p>
        <ul class="list-disc space-y-1 pl-5 text-[12px]">
          <li>{{ t('pages.agentStudio.project.switchItemMemory') }}</li>
          <li>{{ t('pages.agentStudio.project.switchItemContext') }}</li>
          <li>{{ t('pages.agentStudio.project.switchItemJobs') }}</li>
          <li>{{ t('pages.agentStudio.project.switchItemPm') }}</li>
        </ul>
        <p class="mt-2 text-[11.5px] text-txt3">{{ t('pages.agentStudio.project.switchApplyHint') }}</p>
      </div>
      <template #footer>
        <AppButton size="sm" variant="ghost" @click="cancelProjectChange">{{ t('common.buttons.cancel') }}</AppButton>
        <AppButton size="sm" variant="danger" @click="confirmProjectChange">
          {{ pendingProjectLabel ? t('pages.agentStudio.project.switchConfirm', { name: pendingProjectLabel }) : t('pages.agentStudio.project.unbindConfirm') }}
        </AppButton>
      </template>
    </AppModal>
  </div>
</template>
