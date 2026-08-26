<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppButton from '@/components/ui/AppButton.vue'
import AppModal from '@/components/ui/AppModal.vue'
import AppSwitch from '@/components/ui/AppSwitch.vue'
import { api } from '@/lib/api/api'
import { useToast } from '@/lib/composables/useToast'
import { copyToClipboard } from '@/lib/shared/copyToClipboard'
import { fmtTime } from '@/lib/shared/format'

const props = defineProps<{ projectId: string }>()

const { t } = useI18n()
const toast = useToast()

const PM_MCP_OPTIONS = [
  { id: 'pm-progress', labelKey: 'pages.projectDetail.pm.mcpProgress' },
  { id: 'pm-workflow-read', labelKey: 'pages.projectDetail.pm.mcpWorkflowRead' },
  { id: 'pm-workflow-write', labelKey: 'pages.projectDetail.pm.mcpWorkflowWrite' },
  { id: 'pm-agent-fs', labelKey: 'pages.projectDetail.pm.mcpAgentFs' },
  { id: 'pm-prd-manager', labelKey: 'pages.projectDetail.pm.mcpPrdManager' },
] as const

interface ExternalMcpSettings {
  enabled: boolean
  enabledPacks: string[]
  mcpBaseUrl: string
}

interface ProjectMcpKeyItem {
  id: string
  name: string
  key_prefix: string
  created_at: string
}

const loading = ref(true)
const saving = ref(false)
const enabled = ref(false)
const enabledPacks = ref<string[]>([])
const mcpBaseUrl = ref('')
const keys = ref<ProjectMcpKeyItem[]>([])
const loadingKeys = ref(false)
const showCreateKey = ref(false)
const keyName = ref('')
const newKeyPlain = ref('')
const creatingKey = ref(false)
const copied = ref('')

const packExampleUrl = computed(() => {
  const base = mcpBaseUrl.value || `${window.location.origin}/mcp/external/${props.projectId}`
  return `${base}/pm-progress`
})

const mcpBaseForHint = computed(() =>
  mcpBaseUrl.value || `${window.location.origin}/mcp/external/${props.projectId}`,
)

const mcpJsonExample = computed(() => JSON.stringify({
  mcpServers: {
    'project-pm': {
      url: packExampleUrl.value,
      headers: { Authorization: 'Bearer YOUR_PROJECT_MCP_KEY' },
    },
  },
}, null, 2))

async function loadSettings() {
  loading.value = true
  try {
    const s = await api.getProjectExternalMcp(props.projectId)
    enabled.value = s.enabled
    enabledPacks.value = [...(s.enabledPacks || [])]
    mcpBaseUrl.value = s.mcpBaseUrl || ''
  } catch (e: unknown) {
    toast.error(String((e as Error)?.message || e))
  } finally {
    loading.value = false
  }
}

async function loadKeys() {
  loadingKeys.value = true
  try {
    keys.value = await api.listProjectMcpKeys(props.projectId)
  } catch {
    keys.value = []
  } finally {
    loadingKeys.value = false
  }
}

function togglePack(id: string) {
  if (enabledPacks.value.includes(id)) {
    enabledPacks.value = enabledPacks.value.filter((x) => x !== id)
  } else {
    enabledPacks.value = [...enabledPacks.value, id]
  }
}

async function saveSettings() {
  saving.value = true
  try {
    const s = await api.updateProjectExternalMcp(props.projectId, {
      enabled: enabled.value,
      enabledPacks: enabledPacks.value,
    })
    enabled.value = s.enabled
    enabledPacks.value = [...(s.enabledPacks || [])]
    mcpBaseUrl.value = s.mcpBaseUrl || mcpBaseUrl.value
    toast.success(t('pages.projectDetail.externalMcp.saved'))
  } catch (e: unknown) {
    toast.error(String((e as Error)?.message || e))
  } finally {
    saving.value = false
  }
}

function openCreateKey() {
  keyName.value = ''
  newKeyPlain.value = ''
  showCreateKey.value = true
}

async function confirmCreateKey() {
  creatingKey.value = true
  try {
    const res = await api.createProjectMcpKey(props.projectId, keyName.value)
    newKeyPlain.value = res.key
    await loadKeys()
  } catch (e: unknown) {
    toast.error(String((e as Error)?.message || e))
  } finally {
    creatingKey.value = false
  }
}

async function revokeKey(id: string) {
  try {
    await api.revokeProjectMcpKey(props.projectId, id)
    await loadKeys()
    toast.success(t('pages.projectDetail.externalMcp.keyRevoked'))
  } catch (e: unknown) {
    toast.error(String((e as Error)?.message || e))
  }
}

async function copyText(text: string, label = '') {
  const ok = await copyToClipboard(text)
  if (ok) {
    copied.value = label
    toast.success(t('pages.projectDetail.externalMcp.copySuccess'))
    setTimeout(() => { copied.value = '' }, 2000)
  } else {
    toast.error(t('pages.projectDetail.externalMcp.copyFailed'))
  }
}

onMounted(async () => {
  await Promise.all([loadSettings(), loadKeys()])
})
</script>

<template>
  <div
    class="flex min-h-0 flex-1 flex-col overflow-hidden"
    data-testid="project-external-mcp-panel"
  >
    <div class="scroll-area min-h-0 flex-1 overflow-y-auto px-4 py-4 sm:px-6">
      <div class="mx-auto max-w-3xl space-y-6">
        <div>
          <h2 class="text-lg font-semibold text-txt">{{ t('pages.projectDetail.externalMcp.title') }}</h2>
          <p class="mt-1 text-sm text-txt3">{{ t('pages.projectDetail.externalMcp.subtitle') }}</p>
          <p class="mt-2 rounded-md border border-info/30 bg-info/10 px-3 py-2 text-xs leading-snug text-info">
            {{ t('pages.projectDetail.externalMcp.distinctHint') }}
          </p>
        </div>

        <div class="card space-y-4 p-4">
          <div class="flex items-start justify-between gap-3">
            <div>
              <strong class="block text-sm font-medium text-txt">{{ t('pages.projectDetail.externalMcp.enable') }}</strong>
              <p class="m-0 mt-1 text-xs leading-snug text-txt3">{{ t('pages.projectDetail.externalMcp.enableHint') }}</p>
            </div>
            <AppSwitch
              v-model="enabled"
              data-testid="external-mcp-enabled"
              :disabled="loading || saving"
              :aria-label="t('pages.projectDetail.externalMcp.enable')"
            />
          </div>

          <div>
            <strong class="block text-sm font-medium text-txt">{{ t('pages.projectDetail.externalMcp.packsTitle') }}</strong>
            <p class="m-0 mt-1 text-xs leading-snug text-txt3">{{ t('pages.projectDetail.externalMcp.packsHint') }}</p>
            <div class="mt-2.5 flex flex-col gap-2">
              <label
                v-for="opt in PM_MCP_OPTIONS"
                :key="opt.id"
                class="flex cursor-pointer items-start gap-2.5 text-[13px] text-txt"
                :class="loading || saving ? 'cursor-not-allowed opacity-55' : ''"
              >
                <AppSwitch
                  class="mt-0.5"
                  :model-value="enabledPacks.includes(opt.id)"
                  :disabled="loading || saving"
                  :aria-label="opt.id"
                  @update:model-value="togglePack(opt.id)"
                />
                <span>
                  <code class="font-mono text-[12px] text-accent-2">{{ opt.id }}</code>
                  <span class="mt-0.5 block text-xs text-txt3">{{ t(opt.labelKey) }}</span>
                </span>
              </label>
            </div>
          </div>
        </div>

        <div class="card space-y-3 p-4">
          <h3 class="text-sm font-semibold text-txt">{{ t('pages.projectDetail.externalMcp.urlTitle') }}</h3>
          <p class="text-xs leading-snug text-txt3">{{ t('pages.projectDetail.externalMcp.urlHint') }}</p>
          <div class="flex items-center gap-2 rounded-md border border-line bg-base px-3 py-2 font-mono text-[12px]">
            <code class="min-w-0 flex-1 break-all text-accent-2">{{ mcpBaseUrl || t('pages.projectDetail.externalMcp.urlPending') }}</code>
            <button
              type="button"
              class="chip shrink-0 hover:border-line-strong"
              data-testid="external-mcp-copy-url"
              @click="copyText(mcpBaseUrl, 'url')"
            >
              {{ copied === 'url' ? t('pages.projectDetail.externalMcp.copied') : t('pages.projectDetail.externalMcp.copy') }}
            </button>
          </div>
          <p class="text-xs text-txt3">{{ t('pages.projectDetail.externalMcp.urlPackSuffix', { base: mcpBaseForHint }) }}</p>
        </div>

        <div class="card space-y-3 p-4">
          <div class="flex items-center justify-between gap-2">
            <h3 class="text-sm font-semibold text-txt">{{ t('pages.projectDetail.externalMcp.keysTitle') }}</h3>
            <AppButton variant="primary" size="sm" icon="plus" data-testid="external-mcp-create-key" @click="openCreateKey">
              {{ t('pages.projectDetail.externalMcp.createKey') }}
            </AppButton>
          </div>
          <p class="text-xs leading-snug text-txt3">{{ t('pages.projectDetail.externalMcp.keysHint') }}</p>
          <div v-if="loadingKeys" class="py-4 text-center text-sm text-txt3">{{ t('common.loading') }}</div>
          <div v-else-if="!keys.length" class="rounded-md border border-line bg-surface px-4 py-5 text-center text-[13px] text-txt3">
            {{ t('pages.projectDetail.externalMcp.noKeys') }}
          </div>
          <div v-else class="space-y-2">
            <div
              v-for="k in keys"
              :key="k.id"
              class="flex flex-wrap items-center gap-3 rounded-md border border-line bg-surface px-4 py-3"
              data-testid="external-mcp-key-row"
            >
              <span class="min-w-0 flex-1 truncate text-[13px] font-medium text-txt">{{ k.name }}</span>
              <span class="font-mono text-[12px] text-txt3">{{ k.key_prefix }}</span>
              <span class="text-[11px] text-txt3">{{ fmtTime(k.created_at) }}</span>
              <AppButton variant="ghost" size="sm" class="!text-err" @click="revokeKey(k.id)">
                {{ t('pages.projectDetail.externalMcp.revoke') }}
              </AppButton>
            </div>
          </div>
        </div>

        <div class="card space-y-3 p-4">
          <h3 class="text-sm font-semibold text-txt">{{ t('pages.projectDetail.externalMcp.exampleTitle') }}</h3>
          <p class="text-xs leading-snug text-txt3">{{ t('pages.projectDetail.externalMcp.exampleHint') }}</p>
          <div class="relative rounded-md border border-line bg-base p-3">
            <button
              type="button"
              class="absolute right-2 top-2 chip text-[11px]"
              data-testid="external-mcp-copy-example"
              @click="copyText(mcpJsonExample, 'example')"
            >
              {{ copied === 'example' ? t('pages.projectDetail.externalMcp.copied') : t('pages.projectDetail.externalMcp.copy') }}
            </button>
            <pre class="scroll-area overflow-x-auto pr-16 font-mono text-[12px] leading-5 text-txt2">{{ mcpJsonExample }}</pre>
          </div>
        </div>
      </div>
    </div>

    <div class="flex shrink-0 justify-end gap-2 border-t border-line bg-surface p-3">
      <AppButton
        variant="primary"
        data-testid="external-mcp-save"
        :disabled="loading || saving"
        @click="saveSettings"
      >
        {{ saving ? t('common.buttons.saving') : t('pages.projectDetail.externalMcp.save') }}
      </AppButton>
    </div>

    <AppModal
      :open="showCreateKey"
      :title="newKeyPlain ? t('pages.projectDetail.externalMcp.keyCreatedTitle') : t('pages.projectDetail.externalMcp.createKeyTitle')"
      :width="440"
      @close="!creatingKey && (showCreateKey = false)"
    >
      <template v-if="!newKeyPlain">
        <label class="label" for="external-mcp-key-name">{{ t('pages.projectDetail.externalMcp.keyName') }}</label>
        <input
          id="external-mcp-key-name"
          v-model="keyName"
          class="input w-full"
          data-testid="external-mcp-key-name"
          :placeholder="t('pages.projectDetail.externalMcp.keyNamePlaceholder')"
        />
        <div class="mt-4 flex justify-end gap-2">
          <AppButton variant="ghost" @click="showCreateKey = false">{{ t('common.buttons.cancel') }}</AppButton>
          <AppButton variant="primary" :disabled="creatingKey || !keyName.trim()" @click="confirmCreateKey">
            {{ creatingKey ? t('common.buttons.saving') : t('pages.projectDetail.externalMcp.createKey') }}
          </AppButton>
        </div>
      </template>
      <template v-else>
        <p class="mb-3 text-[13px] text-warn">{{ t('pages.projectDetail.externalMcp.keyOnceHint') }}</p>
        <div class="flex items-center gap-2 rounded-md border border-line bg-base px-3 py-2 font-mono text-[12px]">
          <code class="min-w-0 flex-1 break-all text-accent-2">{{ newKeyPlain }}</code>
          <button type="button" class="chip shrink-0" @click="copyText(newKeyPlain, 'plain')">
            {{ copied === 'plain' ? t('pages.projectDetail.externalMcp.copied') : t('pages.projectDetail.externalMcp.copy') }}
          </button>
        </div>
        <div class="mt-4 flex justify-end">
          <AppButton variant="primary" @click="showCreateKey = false">{{ t('common.buttons.close') }}</AppButton>
        </div>
      </template>
    </AppModal>
  </div>
</template>
