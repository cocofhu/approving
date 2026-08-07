<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import {
  api,
  type DashboardStats,
  type LiveProbeReport,
  type LiveStatus,
  type SandboxView,
  type SettingItem,
} from '@/lib/api'
import { useAuth } from '@/lib/useAuth'
import AppButton from '@/components/ui/AppButton.vue'
import EmptyState from '@/components/ui/EmptyState.vue'
import Icon from '@/components/ui/Icon.vue'

const { t } = useI18n()
const router = useRouter()
const { user } = useAuth()

const isAdmin = computed(() => !!user.value?.isAdmin)

const groups = computed(() => [
  {
    id: 'concurrency',
    title: t('pages.settings.groups.concurrency.title'),
    desc: t('pages.settings.groups.concurrency.desc'),
    icon: 'runs',
    keys: ['max_concurrent_runs'],
  },
  {
    id: 'recycle',
    title: t('pages.settings.groups.recycle.title'),
    desc: t('pages.settings.groups.recycle.desc'),
    icon: 'trash',
    keys: ['run_sandbox_ttl_minutes', 'test_sandbox_ttl_minutes'],
  },
  {
    id: 'capacity',
    title: t('pages.settings.groups.capacity.title'),
    desc: t('pages.settings.groups.capacity.desc'),
    icon: 'flask',
    keys: ['max_test_sandboxes'],
  },
  {
    id: 'live',
    title: t('pages.settings.groups.live.title'),
    desc: t('pages.settings.groups.live.desc'),
    icon: 'chat',
    keys: ['live_base_url', 'live_model', 'live_api_key', 'live_timeout_seconds'],
  },
])

const SETTING_UNIT_KEYS: Record<string, string> = {
  run_sandbox_ttl_minutes: 'common.format.minutes',
  test_sandbox_ttl_minutes: 'common.format.minutes',
  live_timeout_seconds: 'common.format.seconds',
}

function settingDescription(key: string): string {
  return t(`pages.settings.settings.${key}`)
}

function settingLabel(key: string): string {
  return t(`pages.settings.labels.${key}`)
}

function settingUnit(key: string): string {
  const unitKey = SETTING_UNIT_KEYS[key]
  return unitKey ? t(unitKey) : ''
}

function sourceLabelOf(source: string): string {
  return t(`pages.settings.sourceLabels.${source}`) || source
}

const items = ref<SettingItem[]>([])
const form = reactive<Record<string, number | string>>({})
const sandboxes = ref<SandboxView[]>([])
const dashboard = ref<DashboardStats | null>(null)
const loading = ref(true)
const saving = ref(false)
const error = ref('')
const usageError = ref('')
const savedAt = ref(0)

let poll: number | undefined

function itemOf(key: string): SettingItem | undefined {
  return items.value.find((it) => it.key === key)
}

function hydrate(list: SettingItem[]) {
  items.value = list
  for (const it of list) {
    // A secret reads back as the mask, which must never become the field's
    // value: submitting it would be indistinguishable from setting the key to
    // literal asterisks. The field starts empty and the placeholder says
    // whether one is stored.
    form[it.key] = it.kind === 'secret' ? '' : it.value
  }
}

// liveConfigured mirrors the server rule: the layer is on once the endpoint
// and model are set. The key is excluded because endpoints on the local
// network take no auth.
const liveConfigured = computed(() =>
  ['live_base_url', 'live_model'].every((key) => {
    const it = itemOf(key)
    return !!it && String(it.value ?? '').trim() !== ''
  }),
)

const probe = ref<LiveProbeReport | null>(null)
const probing = ref(false)
const probeError = ref('')
const liveStatus = ref<LiveStatus | null>(null)

function probeCheckLabel(name: string): string {
  const known: Record<string, string> = {
    reachable: 'pages.settings.liveTest.checks.reachable',
    tool_calls: 'pages.settings.liveTest.checks.toolCalls',
  }
  return known[name] ? t(known[name]) : name
}

// The probe is sent the same patch save() would send, so what is tested is what
// is on screen. Editing the form clears a stale result rather than leaving a
// pass sitting next to a changed address.
function livePatch(): Record<string, number | string> {
  const patch: Record<string, number | string> = {}
  for (const key of ['live_base_url', 'live_model', 'live_api_key', 'live_timeout_seconds']) {
    const it = itemOf(key)
    if (!it || it.locked) continue
    if (it.kind === 'int') {
      patch[key] = Number(form[key])
      continue
    }
    patch[key] = String(form[key] ?? '').trim()
  }
  return patch
}

async function runLiveTest() {
  probing.value = true
  probeError.value = ''
  probe.value = null
  try {
    probe.value = await api.testLiveEndpoint(livePatch())
  } catch (e: any) {
    probeError.value = e?.message || t('pages.settings.liveTest.failed')
  } finally {
    probing.value = false
    loadLiveStatus()
  }
}

async function loadLiveStatus() {
  try {
    liveStatus.value = await api.liveStatus()
  } catch {
    // Runtime stats are supplementary; a failure here must not disturb the page.
    liveStatus.value = null
  }
}

function secretPlaceholder(key: string): string {
  const it = itemOf(key)
  const stored = !!it && String(it.value ?? '').trim() !== ''
  return t(stored ? 'pages.settings.liveSecretStored' : 'pages.settings.liveSecretEmpty')
}

function aggregateUsage(list: SandboxView[]) {
  const active = (s: SandboxView) => s.status === 'creating' || s.status === 'running'
  let testCount = 0
  let total = 0
  for (const s of list) {
    if (!active(s)) continue
    total++
    if (s.purpose === 'test') testCount++
  }
  return { testCount, total }
}

const usage = computed(() => aggregateUsage(sandboxes.value))
const runCount = computed(() => dashboard.value?.running ?? 0)
const capacityTotal = computed(() => usage.value.testCount + runCount.value)

const testLimit = computed(() => Math.max(1, Number(form.max_test_sandboxes) || 1))
const runLimit = computed(() => Math.max(1, Number(form.max_concurrent_runs) || 1))

const testRatio = computed(() => (testLimit.value > 0 ? usage.value.testCount / testLimit.value : 0))
const runRatio = computed(() => (runLimit.value > 0 ? runCount.value / runLimit.value : 0))

function usageBarClass(ratio: number, base: 'test' | 'run'): string {
  if (ratio >= 1) return 'bg-err'
  if (ratio >= 0.8) return 'bg-warn'
  return base === 'test' ? 'bg-info' : 'bg-accent-2'
}

function barWidth(ratio: number): string {
  return `${Math.min(100, ratio * 100)}%`
}

async function loadSettings() {
  loading.value = true
  error.value = ''
  try {
    const res = await api.getSettings()
    hydrate(res.items)
  } catch (e: any) {
    error.value = e?.message || t('pages.settings.loadFailed')
  } finally {
    loading.value = false
  }
}

async function pollSandboxes() {
  try {
    const [sb, dash] = await Promise.all([api.listSandboxes(), api.dashboard()])
    sandboxes.value = sb
    dashboard.value = dash
    usageError.value = ''
  } catch (e: any) {
    usageError.value = e?.message || t('pages.settings.usageRefreshFailed')
  }
}

async function save() {
  const maxTest = itemOf('max_test_sandboxes')
  if (maxTest && !maxTest.locked) {
    const n = Number(form.max_test_sandboxes)
    if (!Number.isFinite(n) || n < 1) {
      error.value = t('pages.settings.minTestSandboxes')
      return
    }
  }

  saving.value = true
  error.value = ''
  try {
    const patch: Record<string, number | string> = {}
    for (const it of items.value) {
      if (it.locked) continue
      if (it.kind === 'int') {
        patch[it.key] = Number(form[it.key])
        continue
      }
      const v = String(form[it.key] ?? '').trim()
      // A blank secret means "keep the stored one", which the server honours,
      // so there is nothing to send.
      if (it.kind === 'secret' && v === '') continue
      patch[it.key] = v
    }
    const res = await api.updateSettings(patch)
    hydrate(res.items)
    savedAt.value = Date.now()
    loadLiveStatus()
  } catch (e: any) {
    error.value = e?.message || t('pages.settings.saveFailed')
  } finally {
    saving.value = false
  }
}

const dirty = () =>
  items.value.some((it) => {
    if (it.locked) return false
    if (it.kind === 'int') return Number(form[it.key]) !== it.value
    const v = String(form[it.key] ?? '').trim()
    // A secret field is only dirty once something is typed into it; empty is
    // the resting state, not a cleared value.
    if (it.kind === 'secret') return v !== ''
    return v !== String(it.value ?? '')
  })

// A result that outlives the values it was measured against is worse than no
// result: it reads as a pass for whatever is on screen now.
watch(
  () => ['live_base_url', 'live_model', 'live_api_key', 'live_timeout_seconds'].map((k) => form[k]).join('\u0000'),
  () => {
    probe.value = null
    probeError.value = ''
  },
)

onMounted(() => {
  loadSettings()
  loadLiveStatus()
  pollSandboxes()
  poll = window.setInterval(pollSandboxes, 5000)
})

onBeforeUnmount(() => {
  if (poll) clearInterval(poll)
})
</script>

<template>
  <div>
    <div class="mb-5 flex items-end justify-between gap-3">
      <div>
        <h2 class="text-lg font-semibold text-txt">{{ t('pages.settings.title') }}</h2>
        <p class="text-sm text-txt3">{{ t('pages.settings.subtitle') }}</p>
      </div>
      <div class="flex items-center gap-2">
        <span v-if="savedAt" class="text-xs text-ok">{{ t('pages.settings.saved') }}</span>
        <AppButton variant="ghost" size="sm" icon="refresh" :disabled="loading || saving" @click="loadSettings">
          {{ t('common.buttons.reset') }}
        </AppButton>
        <AppButton variant="primary" size="sm" icon="check" :disabled="loading || saving || !dirty()" @click="save">
          {{ saving ? t('common.buttons.saving') : t('common.buttons.save') }}
        </AppButton>
      </div>
    </div>

    <div v-if="error" class="mb-4 rounded-lg border border-err/30 bg-err/10 px-3 py-2 text-sm text-err">
      {{ error }}
    </div>

    <div v-if="loading" class="card">
      <EmptyState icon="settings" :title="t('pages.settings.loadingTitle')" :desc="t('pages.settings.loadingDesc')" />
    </div>

    <template v-else>
      <div class="card mb-4 border-accent/20 bg-accent-dim/20">
        <div class="flex flex-wrap items-center justify-between gap-3 px-4 py-4">
          <div class="flex items-start gap-3">
            <span class="flex h-8 w-8 shrink-0 items-center justify-center bg-accent-dim text-accent-2">
              <Icon name="file" :size="16" />
            </span>
            <div>
              <h3 class="text-[13px] font-semibold text-txt">{{ t('pages.settings.platformRulesCard.title') }}</h3>
              <p class="mt-1 max-w-2xl text-xs leading-relaxed text-txt3">{{ t('pages.settings.platformRulesCard.desc') }}</p>
              <p v-if="!isAdmin" class="mt-1 text-[11px] text-warn">{{ t('pages.settings.platformRulesCard.readOnly') }}</p>
            </div>
          </div>
          <AppButton variant="primary" size="sm" icon="chevron-right" @click="router.push('/settings/platform-rules')">
            {{ isAdmin ? t('pages.settings.platformRulesCard.manage') : t('pages.settings.platformRulesCard.view') }}
          </AppButton>
        </div>
      </div>

      <div v-for="group in groups" :key="group.id" class="card mb-4">
        <div class="border-b border-line px-4 py-3.5">
          <h3 class="flex items-center gap-2 text-[13px] font-semibold text-txt">
            <span class="flex h-7 w-7 shrink-0 items-center justify-center bg-accent-dim text-accent-2">
              <Icon :name="group.icon" :size="16" />
            </span>
            {{ group.title }}
          </h3>
          <p class="mt-1 text-xs leading-relaxed text-txt3">{{ group.desc }}</p>
        </div>

        <div v-if="group.id === 'capacity'" class="border-b border-line px-4 py-3.5">
          <div class="border border-line bg-base px-3.5 py-3">
            <div class="mb-2.5 flex items-baseline justify-between gap-3">
              <span class="text-xs text-txt2">{{ t('pages.settings.capacity.activeSandboxes') }}</span>
              <span class="text-[22px] font-bold tabular-nums leading-none text-txt">
                {{ capacityTotal }}<span class="ml-1 text-xs font-normal text-txt3">{{ t('common.format.units') }}</span>
              </span>
            </div>

            <div class="mb-2.5 flex flex-wrap gap-2">
              <span class="inline-flex items-center gap-1.5 border border-line bg-elevated px-2.5 py-1 text-[11px] text-txt2">
                <span class="h-1.5 w-1.5 shrink-0 bg-info" />
                {{ t('pages.settings.capacity.test') }} <strong class="tabular-nums text-txt">{{ usage.testCount }}</strong>
                <span class="text-[10px] text-txt3">/ {{ testLimit }} {{ t('pages.settings.capacity.limit') }}</span>
              </span>
              <span class="inline-flex items-center gap-1.5 border border-line bg-elevated px-2.5 py-1 text-[11px] text-txt2">
                <span class="h-1.5 w-1.5 shrink-0 bg-accent-2" />
                {{ t('pages.settings.capacity.run') }} <strong class="tabular-nums text-txt">{{ runCount }}</strong>
                <span class="text-[10px] text-txt3">/ {{ runLimit }} {{ t('pages.settings.capacity.concurrency') }}</span>
              </span>
            </div>

            <div class="mb-1.5 flex items-center gap-2.5">
              <span class="w-14 shrink-0 text-[11px] text-txt3">{{ t('pages.settings.capacity.test') }}</span>
              <div class="flex flex-1 items-center gap-2">
                <div class="h-[3px] flex-1 overflow-hidden bg-elevated">
                  <div
                    class="h-full transition-[width] duration-300 ease-out"
                    :class="usageBarClass(testRatio, 'test')"
                    :style="{ width: barWidth(testRatio) }"
                  />
                </div>
                <span class="w-9 shrink-0 text-right text-[11px] tabular-nums text-txt2">
                  {{ usage.testCount }}/{{ testLimit }}
                </span>
              </div>
            </div>

            <div class="flex items-center gap-2.5">
              <span class="w-14 shrink-0 text-[11px] text-txt3">{{ t('pages.settings.capacity.run') }}</span>
              <div class="flex flex-1 items-center gap-2">
                <div class="h-[3px] flex-1 overflow-hidden bg-elevated">
                  <div
                    class="h-full transition-[width] duration-300 ease-out"
                    :class="usageBarClass(runRatio, 'run')"
                    :style="{ width: barWidth(runRatio) }"
                  />
                </div>
                <span
                  class="w-9 shrink-0 text-right text-[11px] tabular-nums"
                  :class="runRatio >= 1 ? 'font-semibold text-err' : 'text-txt2'"
                >
                  {{ runCount }}/{{ runLimit }}
                </span>
              </div>
            </div>

            <div class="mt-2.5 flex items-center justify-between border-t border-line pt-2 text-[10px] text-txt3">
              <span class="inline-flex items-center gap-1">
                <span class="h-[5px] w-[5px] animate-pulseglow bg-ok" />
                {{ t('pages.settings.capacity.autoRefresh') }}
              </span>
              <span>{{ t('pages.settings.capacity.note') }}</span>
            </div>

            <p v-if="usageError" class="mt-2 text-[11px] text-err">{{ usageError }}</p>
          </div>
        </div>

        <div
          v-for="key in group.keys"
          :key="key"
          class="border-b border-line px-4 py-3.5 last:border-b-0"
        >
          <template v-if="itemOf(key)">
            <div class="flex items-start justify-between gap-4">
              <div class="min-w-0 flex-1">
                <div class="flex flex-wrap items-center gap-1.5">
                  <span class="text-sm font-medium text-txt">{{ settingLabel(key) }}</span>
                  <span
                    class="rounded-full border border-line bg-elevated px-2 py-0.5 text-[10px] text-txt3"
                    :title="t('pages.settings.sourceTitle', { source: itemOf(key)!.source })"
                  >
                    {{ sourceLabelOf(itemOf(key)!.source) }}
                  </span>
                  <span
                    v-if="itemOf(key)!.locked"
                    class="inline-flex items-center gap-1 text-[10px] text-warn"
                    :title="t('pages.settings.envLockedTitle')"
                  >
                    <Icon name="gate" :size="11" /> {{ t('pages.settings.envLocked') }}
                  </span>
                </div>
                <div class="mt-0.5 font-mono text-[11px] text-txt3">{{ key }}</div>
                <p v-if="settingDescription(key)" class="mt-1.5 text-xs leading-relaxed text-txt3">
                  {{ settingDescription(key) }}
                </p>
              </div>
              <div class="flex shrink-0 items-center gap-2">
                <input
                  v-if="itemOf(key)!.kind === 'int'"
                  v-model.number="form[key]"
                  type="number"
                  :min="itemOf(key)!.min"
                  :disabled="itemOf(key)!.locked || saving"
                  class="input w-[88px] text-right"
                />
                <input
                  v-else-if="itemOf(key)!.kind === 'secret'"
                  v-model="form[key]"
                  type="password"
                  autocomplete="new-password"
                  :placeholder="secretPlaceholder(key)"
                  :disabled="itemOf(key)!.locked || saving"
                  class="input w-[260px]"
                />
                <input
                  v-else
                  v-model="form[key]"
                  type="text"
                  :disabled="itemOf(key)!.locked || saving"
                  class="input w-[260px]"
                />
                <span v-if="settingUnit(key)" class="w-9 text-xs text-txt3">{{ settingUnit(key) }}</span>
                <span v-else class="w-9" />
              </div>
            </div>
          </template>
        </div>

        <template v-if="group.id === 'live'">
          <div v-if="!liveConfigured" class="px-4 py-3">
            <p class="border border-warn/30 bg-warn/10 px-3 py-2 text-xs leading-relaxed text-warn">
              {{ t('pages.settings.liveUnconfigured') }}
            </p>
          </div>

          <div class="border-t border-line px-4 py-3.5">
            <div class="flex flex-wrap items-center justify-between gap-2">
              <div class="min-w-0">
                <span class="text-sm font-medium text-txt">{{ t('pages.settings.liveTest.title') }}</span>
                <p class="mt-1 text-xs leading-relaxed text-txt3">{{ t('pages.settings.liveTest.desc') }}</p>
              </div>
              <AppButton
                variant="ghost"
                size="sm"
                icon="flask"
                :disabled="probing || saving || !liveConfigured"
                @click="runLiveTest"
              >
                {{ probing ? t('pages.settings.liveTest.running') : t('pages.settings.liveTest.action') }}
              </AppButton>
            </div>

            <p v-if="probeError" class="mt-2.5 text-[11px] text-err">{{ probeError }}</p>

            <div v-if="probe" class="mt-2.5 border border-line bg-elevated px-3 py-2.5">
              <div class="flex flex-wrap items-center gap-2 text-xs">
                <span
                  class="inline-flex items-center gap-1 font-medium"
                  :class="probe.ok ? 'text-ok' : 'text-err'"
                >
                  <Icon :name="probe.ok ? 'check' : 'close'" :size="12" />
                  {{ probe.ok ? t('pages.settings.liveTest.pass') : t('pages.settings.liveTest.fail') }}
                </span>
                <span v-if="probe.latencyMs > 0" class="text-txt3 tabular-nums">
                  {{ t('pages.settings.liveTest.latency', { ms: probe.latencyMs }) }}
                </span>
              </div>

              <ul class="mt-2 space-y-1.5">
                <li v-for="check in probe.checks" :key="check.name" class="flex items-start gap-1.5 text-[11px]">
                  <Icon
                    :name="check.ok ? 'check' : 'close'"
                    :size="11"
                    :class="check.ok ? 'mt-0.5 shrink-0 text-ok' : 'mt-0.5 shrink-0 text-err'"
                  />
                  <span class="min-w-0">
                    <span class="text-txt2">{{ probeCheckLabel(check.name) }}</span>
                    <span v-if="check.reason" class="text-txt3"> — {{ check.reason }}</span>
                  </span>
                </li>
              </ul>

              <p v-if="probe.sample" class="mt-2 border-t border-line pt-2 text-[11px] text-txt3">
                {{ t('pages.settings.liveTest.sample') }}
                <span class="font-mono text-txt2">{{ probe.sample }}</span>
              </p>
            </div>
          </div>

          <div class="border-t border-line px-4 py-3.5">
            <span class="text-sm font-medium text-txt">{{ t('pages.settings.liveRuntime.title') }}</span>
            <p class="mt-1 text-xs leading-relaxed text-txt3">{{ t('pages.settings.liveRuntime.desc') }}</p>

            <p
              v-if="liveStatus && liveStatus.configured && liveStatus.stats.calls === 0"
              class="mt-2.5 border border-warn/30 bg-warn/10 px-3 py-2 text-[11px] leading-relaxed text-warn"
            >
              {{ t('pages.settings.liveRuntime.neverCalled') }}
            </p>

            <div v-if="liveStatus && liveStatus.stats.calls > 0" class="mt-2.5 space-y-1.5 text-[11px]">
              <div class="text-txt2 tabular-nums">
                {{
                  t('pages.settings.liveRuntime.counts', {
                    calls: liveStatus.stats.calls,
                    failed: liveStatus.stats.failed,
                  })
                }}
              </div>
              <div v-if="liveStatus.stats.avgLatencyMs > 0" class="text-txt3 tabular-nums">
                {{ t('pages.settings.liveRuntime.avgLatency', { ms: liveStatus.stats.avgLatencyMs }) }}
              </div>
              <div v-if="liveStatus.stats.lastFailure" class="text-err">
                {{ t('pages.settings.liveRuntime.lastFailure') }} {{ liveStatus.stats.lastFailure }}
              </div>
            </div>
          </div>
        </template>
      </div>
    </template>

    <p class="mt-1 text-xs text-txt3">
      {{ t('pages.settings.priorityNote') }}
    </p>
  </div>
</template>
