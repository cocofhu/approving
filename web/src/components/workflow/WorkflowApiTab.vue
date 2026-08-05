<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/ui/Icon.vue'
import AppButton from '@/components/ui/AppButton.vue'
import AppModal from '@/components/ui/AppModal.vue'
import { api } from '@/lib/api'
import { useWorkflowAskInputs } from '@/lib/useWorkflowAskInputs'
import { fmtTime } from '@/lib/format'
import { useToast } from '@/lib/useToast'
import type { Workflow } from '@/lib/types'

const props = defineProps<{ workflow: Workflow }>()
const toast = useToast()
const { t } = useI18n()

const baseUrl = computed(() => `${window.location.origin}/v1`)
const { fields: askFields } = useWorkflowAskInputs(props.workflow)

// --- API Keys ---
interface APIKeyItem {
  id: string
  name: string
  key_prefix: string
  created_at: string
}

const keys = ref<APIKeyItem[]>([])
const loadingKeys = ref(false)
const showCreateKey = ref(false)
const keyName = ref('')
const newKeyPlain = ref('')
const creatingKey = ref(false)

async function loadKeys() {
  if (!props.workflow.id || props.workflow.status !== 'published') return
  loadingKeys.value = true
  try {
    keys.value = await api.listAPIKeys(props.workflow.id)
  } catch {
    keys.value = []
  } finally {
    loadingKeys.value = false
  }
}

function openCreateKey() {
  keyName.value = ''
  newKeyPlain.value = ''
  showCreateKey.value = true
}

async function confirmCreateKey() {
  if (!props.workflow.id) return
  creatingKey.value = true
  try {
    const res = await api.createAPIKey(props.workflow.id, keyName.value)
    newKeyPlain.value = res.key
    await loadKeys()
  } catch (e: any) {
    toast.error(String(e?.message || e))
  } finally {
    creatingKey.value = false
  }
}

async function revokeKey(id: string) {
  if (!props.workflow.id) return
  try {
    await api.revokeAPIKey(props.workflow.id, id)
    await loadKeys()
    toast.success(t('pages.workflowApi.keyRevoked'))
  } catch (e: any) {
    toast.error(String(e?.message || e))
  }
}

// --- Code examples ---
const codeLang = ref<Record<string, 'curl' | 'python'>>({
  create: 'curl',
  get: 'curl',
  artifacts: 'curl',
  download: 'curl',
  cancel: 'curl',
})

const copied = ref('')
function copyText(text: string, label = '') {
  navigator.clipboard.writeText(text).then(() => {
    copied.value = label
    toast.success(t('pages.workflowApi.copySuccess'))
    setTimeout(() => { copied.value = '' }, 2000)
  }).catch(() => toast.error(t('pages.workflowApi.copyFailed')))
}

const inputsExample = computed(() => {
  const obj: Record<string, string> = {}
  for (const f of askFields.value) {
    obj[f.key] = f.type === 'number' ? '0' : f.type === 'boolean' ? 'true' : `"${t('pages.workflowApi.exampleValue', { key: f.key })}"`
  }
  return JSON.stringify(obj, null, 2).replace(/^/gm, '      ').trim()
})

const examples = computed(() => {
  const b = baseUrl.value
  const wf = props.workflow.id
  const inputs = askFields.value.length
    ? `{\n    "inputs": ${inputsExample.value.replace(/^      /gm, '')}\n  }`
    : '{ "inputs": {} }'
  return {
    createCurl: `curl -X POST '${b}/workflows/${wf}/runs' \\\n  -H 'Authorization: Bearer YOUR_API_KEY' \\\n  -H 'Content-Type: application/json' \\\n  -d '${inputs}'`,
    createPy: `import requests\n\nresp = requests.post(\n    "${b}/workflows/${wf}/runs",\n    headers={"Authorization": "Bearer YOUR_API_KEY"},\n    json=${inputs.replace(/'/g, '"')},\n)\nprint(resp.json())`,
    getCurl: `curl '${b}/runs/run_xyz789' \\\n  -H 'Authorization: Bearer YOUR_API_KEY'`,
    getPy: `import requests\n\nresp = requests.get(\n    "${b}/runs/run_xyz789",\n    headers={"Authorization": "Bearer YOUR_API_KEY"},\n)\nprint(resp.json())`,
    artifactsCurl: `curl '${b}/runs/run_xyz789/artifacts' \\\n  -H 'Authorization: Bearer YOUR_API_KEY'`,
    artifactsPy: `import requests\n\nresp = requests.get(\n    "${b}/runs/run_xyz789/artifacts",\n    headers={"Authorization": "Bearer YOUR_API_KEY"},\n)\nprint(resp.json())`,
    downloadCurl: `curl -O '${b}/artifacts/art_001/download' \\\n  -H 'Authorization: Bearer YOUR_API_KEY'`,
    downloadPy: `import requests\n\nresp = requests.get(\n    "${b}/artifacts/art_001/download",\n    headers={"Authorization": "Bearer YOUR_API_KEY"},\n)\nopen("artifact.bin", "wb").write(resp.content)`,
    cancelCurl: `curl -X POST '${b}/runs/run_xyz789/cancel' \\\n  -H 'Authorization: Bearer YOUR_API_KEY'`,
    cancelPy: `import requests\n\nresp = requests.post(\n    "${b}/runs/run_xyz789/cancel",\n    headers={"Authorization": "Bearer YOUR_API_KEY"},\n)\nprint(resp.json())`,
  }
})

const tocSections = computed(() => [
  { id: 'sec-overview', label: t('pages.workflowApi.tocOverview') },
  { id: 'sec-base-url', label: t('pages.workflowApi.tocBaseUrl') },
  { id: 'sec-auth', label: t('pages.workflowApi.tocAuth') },
  { id: 'sec-keys', label: t('pages.workflowApi.tocKeys') },
  { id: 'sec-inputs', label: t('pages.workflowApi.tocInputs') },
  { id: 'ep-create-run', label: t('pages.workflowApi.tocCreateRun') },
  { id: 'ep-get-run', label: t('pages.workflowApi.tocGetRun') },
  { id: 'ep-artifacts', label: t('pages.workflowApi.tocArtifacts') },
  { id: 'ep-download', label: t('pages.workflowApi.tocDownload') },
  { id: 'ep-cancel', label: t('pages.workflowApi.tocCancel') },
  { id: 'sec-scope', label: t('pages.workflowApi.tocOutOfScope') },
])

onMounted(loadKeys)
</script>

<template>
  <div class="relative flex h-full overflow-hidden">
    <!-- Draft overlay -->
    <div
      v-if="workflow.status !== 'published'"
      class="absolute inset-0 z-10 flex flex-col items-center justify-center bg-base/80 backdrop-blur-sm"
    >
      <div class="mb-3 flex h-14 w-14 items-center justify-center rounded-xl border border-dashed border-line-strong text-txt3">
        <Icon name="gate" :size="26" />
      </div>
      <div class="text-sm font-medium text-txt2">{{ t('pages.workflowApi.publishRequiredTitle') }}</div>
      <div class="mt-1 max-w-sm text-center text-xs text-txt3">{{ t('pages.workflowApi.publishRequiredDesc') }}</div>
    </div>

    <div class="flex min-h-0 flex-1 overflow-hidden" :class="{ 'opacity-40 pointer-events-none': workflow.status !== 'published' }">
      <!-- Doc content -->
      <div class="scroll-area flex-1 overflow-y-auto px-8 py-6">
        <!-- Overview -->
        <section id="sec-overview" class="mb-8 scroll-mt-4">
          <h3 class="mb-3 text-base font-semibold text-txt">{{ t('pages.workflowApi.overviewTitle') }}</h3>
          <p class="mb-4 text-[13px] leading-6 text-txt2">
            {{ t('pages.workflowApi.overviewBody') }}
          </p>
          <div class="mb-4 flex items-center gap-2 rounded-md border border-warn/30 bg-warn/10 px-3 py-2 text-[12px] text-warn">
            <Icon name="alert" :size="14" class="shrink-0" />
            <span>{{ t('pages.workflowApi.overviewWarn') }}</span>
          </div>
        </section>

        <!-- Base URL -->
        <section id="sec-base-url" class="mb-8 scroll-mt-4">
          <h3 class="mb-3 text-base font-semibold text-txt">Base URL</h3>
          <div class="flex items-center gap-2 rounded-md border border-line bg-base px-3 py-2 font-mono text-[13px]">
            <code class="flex-1 text-accent-2">{{ baseUrl }}</code>
            <button class="chip hover:border-line-strong" @click="copyText(baseUrl, 'base')">
              {{ copied === 'base' ? t('pages.workflowApi.copied') : t('pages.workflowApi.copy') }}
            </button>
          </div>
        </section>

        <!-- Auth -->
        <section id="sec-auth" class="mb-8 scroll-mt-4">
          <h3 class="mb-3 text-base font-semibold text-txt">{{ t('pages.workflowApi.authTitle') }}</h3>
          <div class="rounded-md border border-line bg-surface p-4">
            <p class="mb-3 text-[13px] text-txt2">{{ t('pages.workflowApi.authBody') }}</p>
            <div class="rounded-md border border-line bg-base px-3 py-2 font-mono text-[13px]">
              <code>Authorization: Bearer YOUR_API_KEY</code>
            </div>
          </div>
        </section>

        <!-- API Keys -->
        <section id="sec-keys" class="mb-8 scroll-mt-4">
          <div class="mb-3 flex items-center justify-between">
            <h3 class="text-base font-semibold text-txt">{{ t('pages.workflowApi.keysTitle') }}</h3>
            <AppButton variant="primary" size="sm" icon="plus" @click="openCreateKey">{{ t('pages.workflowApi.createKey') }}</AppButton>
          </div>
          <p class="mb-4 text-[13px] text-txt2">{{ t('pages.workflowApi.keysDesc') }}</p>
          <div v-if="loadingKeys" class="py-6 text-center text-sm text-txt3">{{ t('pages.workflowApi.loading') }}</div>
          <div v-else-if="!keys.length" class="rounded-md border border-line bg-surface px-4 py-6 text-center text-[13px] text-txt3">{{ t('pages.workflowApi.noKeys') }}</div>
          <div v-else class="space-y-2">
            <div
              v-for="k in keys"
              :key="k.id"
              class="flex items-center gap-3 rounded-md border border-line bg-surface px-4 py-3"
            >
              <span class="min-w-0 flex-1 truncate text-[13px] font-medium text-txt">{{ k.name }}</span>
              <span class="font-mono text-[12px] text-txt3">{{ k.key_prefix }}</span>
              <span class="text-[11px] text-txt3">{{ fmtTime(k.created_at) }}</span>
              <AppButton variant="ghost" size="sm" class="!text-err" @click="revokeKey(k.id)">{{ t('pages.workflowApi.revoke') }}</AppButton>
            </div>
          </div>
        </section>

        <!-- Inputs table -->
        <section id="sec-inputs" class="mb-8 scroll-mt-4">
          <h3 class="mb-3 text-base font-semibold text-txt">{{ t('pages.workflowApi.inputsTitle') }}</h3>
          <p class="mb-4 text-[13px] text-txt2">{{ t('pages.workflowApi.inputsDesc') }}</p>
          <div v-if="!askFields.length" class="rounded-md border border-line bg-surface px-4 py-6 text-center text-[13px] text-txt3">{{ t('pages.workflowApi.noAskVars') }}</div>
          <table v-else class="w-full border border-line text-[13px]">
            <thead class="bg-elevated text-[11px] uppercase tracking-wider text-txt3">
              <tr>
                <th class="px-4 py-2 text-left font-medium">name</th>
                <th class="px-4 py-2 text-left font-medium">type</th>
                <th class="px-4 py-2 text-left font-medium">required</th>
                <th class="px-4 py-2 text-left font-medium">desc</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="f in askFields" :key="f.key" class="border-t border-line">
                <td class="px-4 py-2 font-mono text-accent-2">{{ f.key }}</td>
                <td class="px-4 py-2 text-txt2">{{ f.type }}</td>
                <td class="px-4 py-2" :class="f.required ? 'text-ok' : 'text-txt3'">{{ f.required ? t('pages.workflowApi.yes') : t('pages.workflowApi.no') }}</td>
                <td class="px-4 py-2 text-txt2">{{ f.desc || '—' }}</td>
              </tr>
            </tbody>
          </table>
        </section>

        <!-- Endpoints -->
        <section id="sec-endpoints" class="mb-8 scroll-mt-4">
          <h3 class="mb-4 text-base font-semibold text-txt">{{ t('pages.workflowApi.endpointsTitle') }}</h3>

          <!-- POST runs -->
          <div id="ep-create-run" class="mb-5 scroll-mt-4 overflow-hidden rounded-md border border-line">
            <div class="flex items-center gap-3 border-b border-line bg-elevated px-4 py-3">
              <span class="bg-ok/15 px-2 py-0.5 font-mono text-[11px] font-bold text-ok">POST</span>
              <span class="font-mono text-[13px] text-txt">/v1/workflows/{workflow_id}/runs</span>
            </div>
            <div class="p-4">
              <p class="mb-4 text-[13px] text-txt2">{{ t('pages.workflowApi.createRunDesc') }}</p>
              <div class="mb-2 flex gap-1">
                <button class="chip" :class="{ '!border-accent !text-accent': codeLang.create === 'curl' }" @click="codeLang.create = 'curl'">cURL</button>
                <button class="chip" :class="{ '!border-accent !text-accent': codeLang.create === 'python' }" @click="codeLang.create = 'python'">Python</button>
              </div>
              <div class="relative rounded-md border border-line bg-base p-3">
                <button class="absolute right-2 top-2 chip text-[11px]" @click="copyText(codeLang.create === 'curl' ? examples.createCurl : examples.createPy, 'create')">
                  {{ copied === 'create' ? t('pages.workflowApi.copied') : t('pages.workflowApi.copy') }}
                </button>
                <pre class="scroll-area overflow-x-auto pr-16 font-mono text-[12px] leading-5 text-txt2">{{ codeLang.create === 'curl' ? examples.createCurl : examples.createPy }}</pre>
              </div>
            </div>
          </div>

          <!-- GET run -->
          <div id="ep-get-run" class="mb-5 scroll-mt-4 overflow-hidden rounded-md border border-line">
            <div class="flex items-center gap-3 border-b border-line bg-elevated px-4 py-3">
              <span class="bg-info/15 px-2 py-0.5 font-mono text-[11px] font-bold text-info">GET</span>
              <span class="font-mono text-[13px] text-txt">/v1/runs/{run_id}</span>
            </div>
            <div class="p-4">
              <p class="mb-4 text-[13px] text-txt2">{{ t('pages.workflowApi.getRunDesc') }}</p>
              <div class="mb-2 flex gap-1">
                <button class="chip" :class="{ '!border-accent !text-accent': codeLang.get === 'curl' }" @click="codeLang.get = 'curl'">cURL</button>
                <button class="chip" :class="{ '!border-accent !text-accent': codeLang.get === 'python' }" @click="codeLang.get = 'python'">Python</button>
              </div>
              <div class="relative rounded-md border border-line bg-base p-3">
                <button class="absolute right-2 top-2 chip text-[11px]" @click="copyText(codeLang.get === 'curl' ? examples.getCurl : examples.getPy, 'get')">
                  {{ copied === 'get' ? t('pages.workflowApi.copied') : t('pages.workflowApi.copy') }}
                </button>
                <pre class="scroll-area overflow-x-auto pr-16 font-mono text-[12px] leading-5 text-txt2">{{ codeLang.get === 'curl' ? examples.getCurl : examples.getPy }}</pre>
              </div>
            </div>
          </div>

          <!-- GET artifacts -->
          <div id="ep-artifacts" class="mb-5 scroll-mt-4 overflow-hidden rounded-md border border-line">
            <div class="flex items-center gap-3 border-b border-line bg-elevated px-4 py-3">
              <span class="bg-info/15 px-2 py-0.5 font-mono text-[11px] font-bold text-info">GET</span>
              <span class="font-mono text-[13px] text-txt">/v1/runs/{run_id}/artifacts</span>
            </div>
            <div class="p-4">
              <div class="mb-2 flex gap-1">
                <button class="chip" :class="{ '!border-accent !text-accent': codeLang.artifacts === 'curl' }" @click="codeLang.artifacts = 'curl'">cURL</button>
                <button class="chip" :class="{ '!border-accent !text-accent': codeLang.artifacts === 'python' }" @click="codeLang.artifacts = 'python'">Python</button>
              </div>
              <div class="relative rounded-md border border-line bg-base p-3">
                <button class="absolute right-2 top-2 chip text-[11px]" @click="copyText(codeLang.artifacts === 'curl' ? examples.artifactsCurl : examples.artifactsPy, 'artifacts')">
                  {{ copied === 'artifacts' ? t('pages.workflowApi.copied') : t('pages.workflowApi.copy') }}
                </button>
                <pre class="scroll-area overflow-x-auto pr-16 font-mono text-[12px] leading-5 text-txt2">{{ codeLang.artifacts === 'curl' ? examples.artifactsCurl : examples.artifactsPy }}</pre>
              </div>
            </div>
          </div>

          <!-- GET download -->
          <div id="ep-download" class="mb-5 scroll-mt-4 overflow-hidden rounded-md border border-line">
            <div class="flex items-center gap-3 border-b border-line bg-elevated px-4 py-3">
              <span class="bg-info/15 px-2 py-0.5 font-mono text-[11px] font-bold text-info">GET</span>
              <span class="font-mono text-[13px] text-txt">/v1/artifacts/{artifact_id}/download</span>
            </div>
            <div class="p-4">
              <div class="mb-2 flex gap-1">
                <button class="chip" :class="{ '!border-accent !text-accent': codeLang.download === 'curl' }" @click="codeLang.download = 'curl'">cURL</button>
                <button class="chip" :class="{ '!border-accent !text-accent': codeLang.download === 'python' }" @click="codeLang.download = 'python'">Python</button>
              </div>
              <div class="relative rounded-md border border-line bg-base p-3">
                <button class="absolute right-2 top-2 chip text-[11px]" @click="copyText(codeLang.download === 'curl' ? examples.downloadCurl : examples.downloadPy, 'download')">
                  {{ copied === 'download' ? t('pages.workflowApi.copied') : t('pages.workflowApi.copy') }}
                </button>
                <pre class="scroll-area overflow-x-auto pr-16 font-mono text-[12px] leading-5 text-txt2">{{ codeLang.download === 'curl' ? examples.downloadCurl : examples.downloadPy }}</pre>
              </div>
            </div>
          </div>

          <!-- POST cancel -->
          <div id="ep-cancel" class="mb-5 scroll-mt-4 overflow-hidden rounded-md border border-line">
            <div class="flex items-center gap-3 border-b border-line bg-elevated px-4 py-3">
              <span class="bg-ok/15 px-2 py-0.5 font-mono text-[11px] font-bold text-ok">POST</span>
              <span class="font-mono text-[13px] text-txt">/v1/runs/{run_id}/cancel</span>
            </div>
            <div class="p-4">
              <div class="mb-2 flex gap-1">
                <button class="chip" :class="{ '!border-accent !text-accent': codeLang.cancel === 'curl' }" @click="codeLang.cancel = 'curl'">cURL</button>
                <button class="chip" :class="{ '!border-accent !text-accent': codeLang.cancel === 'python' }" @click="codeLang.cancel = 'python'">Python</button>
              </div>
              <div class="relative rounded-md border border-line bg-base p-3">
                <button class="absolute right-2 top-2 chip text-[11px]" @click="copyText(codeLang.cancel === 'curl' ? examples.cancelCurl : examples.cancelPy, 'cancel')">
                  {{ copied === 'cancel' ? t('pages.workflowApi.copied') : t('pages.workflowApi.copy') }}
                </button>
                <pre class="scroll-area overflow-x-auto pr-16 font-mono text-[12px] leading-5 text-txt2">{{ codeLang.cancel === 'curl' ? examples.cancelCurl : examples.cancelPy }}</pre>
              </div>
            </div>
          </div>
        </section>

        <!-- Out of scope -->
        <section id="sec-scope" class="mb-8 scroll-mt-4">
          <h3 class="mb-3 text-base font-semibold text-txt">{{ t('pages.workflowApi.outOfScopeTitle') }}</h3>
          <div class="flex gap-2 rounded-md border border-info/30 bg-info/10 px-3 py-2 text-[12px] text-info">
            <Icon name="doc" :size="14" class="shrink-0" />
            <span>{{ t('pages.workflowApi.outOfScopeBody') }}</span>
          </div>
        </section>
      </div>

      <!-- TOC -->
      <nav class="scroll-area w-48 shrink-0 overflow-y-auto border-l border-line bg-surface px-4 py-6">
        <div class="mb-3 text-[11px] font-semibold uppercase tracking-wider text-txt3">{{ t('pages.workflowApi.toc') }}</div>
        <a
          v-for="s in tocSections"
          :key="s.id"
          :href="'#' + s.id"
          class="block border-l-2 border-transparent py-1.5 pl-3 text-[12px] text-txt2 hover:text-txt"
        >{{ s.label }}</a>
      </nav>
    </div>

    <!-- Create Key Modal -->
    <AppModal :open="showCreateKey" :title="t('pages.workflowApi.createKeyTitle')" :width="440" @close="!creatingKey && (showCreateKey = false)">
      <div v-if="newKeyPlain" class="space-y-3">
        <div class="flex items-center gap-2 rounded-md border border-line bg-base px-3 py-2 font-mono text-[13px]">
          <code class="flex-1 break-all text-accent-2">{{ newKeyPlain }}</code>
          <button class="chip" @click="copyText(newKeyPlain, 'newkey')">{{ copied === 'newkey' ? t('pages.workflowApi.copied') : t('pages.workflowApi.copy') }}</button>
        </div>
        <p class="text-[12px] text-warn">{{ t('pages.workflowApi.copyKeyHint') }}</p>
      </div>
      <div v-else class="space-y-3">
        <label class="text-[12px] text-txt2">{{ t('pages.workflowApi.keyName') }}</label>
        <input v-model="keyName" class="w-full rounded-md border border-line bg-base px-3 py-2 text-[13px] text-txt outline-none focus:border-accent" :placeholder="t('pages.workflowApi.keyNamePh')" />
      </div>
      <template #footer>
        <AppButton variant="ghost" :disabled="creatingKey" @click="showCreateKey = false">{{ newKeyPlain ? t('pages.workflowApi.close') : t('pages.workflowApi.cancel') }}</AppButton>
        <AppButton v-if="!newKeyPlain" variant="primary" :disabled="creatingKey || !keyName.trim()" @click="confirmCreateKey">
          {{ creatingKey ? t('pages.workflowApi.creating') : t('pages.workflowApi.create') }}
        </AppButton>
      </template>
    </AppModal>
  </div>
</template>
