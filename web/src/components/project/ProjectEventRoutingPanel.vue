<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { api } from '@/lib/api'
import type { EventRouteStatus } from '@/lib/types'

const props = defineProps<{
  projectId: string
  /** Bumped by the parent after a notify policy save so the table re-resolves. */
  revision?: number
}>()

const { t, te } = useI18n()

const routes = ref<EventRouteStatus[]>([])
const loading = ref(true)
const failed = ref(false)

async function load() {
  loading.value = true
  failed.value = false
  try {
    const res = await api.getProjectEventRouting(props.projectId)
    routes.value = res.routes || []
  } catch {
    failed.value = true
    routes.value = []
  } finally {
    loading.value = false
  }
}

onMounted(() => void load())
watch(
  () => [props.projectId, props.revision],
  () => void load(),
)

/** Falls back to the raw key so a newly added backend route is still readable. */
function eventLabel(key: string): string {
  const k = `pages.projectDetail.eventRouting.events.${key}.label`
  return te(k) ? t(k) : key
}

function eventTrigger(key: string): string {
  const k = `pages.projectDetail.eventRouting.events.${key}.trigger`
  return te(k) ? t(k) : ''
}

function liveLabel(route: EventRouteStatus): string {
  if (route.noEgress) return t('pages.projectDetail.eventRouting.noEgress')
  return route.toLive
    ? t('pages.projectDetail.eventRouting.yes')
    : t('pages.projectDetail.eventRouting.no')
}

function templateLabel(route: EventRouteStatus): string {
  if (!route.toTemplate) return t('pages.projectDetail.eventRouting.no')
  if (!route.templateActive) return t('pages.projectDetail.eventRouting.templateOff')
  const kinds = (route.activeKinds || [])
    .map((kind) => {
      const k = `pages.projectDetail.eventRouting.kinds.${kind}`
      return te(k) ? t(k) : kind
    })
    .join('、')
  return kinds
    ? t('pages.projectDetail.eventRouting.templateOnWith', { kinds })
    : t('pages.projectDetail.eventRouting.yes')
}

function templateTone(route: EventRouteStatus): string {
  if (!route.toTemplate) return 'text-txt3'
  return route.templateActive ? 'text-ok' : 'text-warn'
}

function liveTone(route: EventRouteStatus): string {
  if (route.noEgress) return 'text-txt3'
  return route.toLive ? 'text-ok' : 'text-txt3'
}
</script>

<template>
  <div class="mx-auto mt-6 max-w-2xl" data-testid="project-event-routing-panel">
    <div class="mb-3">
      <h3 class="text-sm font-semibold text-txt">
        {{ t('pages.projectDetail.eventRouting.title') }}
      </h3>
      <p class="mt-1 text-[12px] text-txt3">
        {{ t('pages.projectDetail.eventRouting.lead') }}
      </p>
    </div>

    <div
      v-if="failed"
      class="rounded-lg border border-warn/35 bg-warn/10 px-3 py-2.5 text-[12px] text-txt2"
      data-testid="event-routing-error"
    >
      {{ t('pages.projectDetail.eventRouting.loadFailed') }}
    </div>

    <div
      v-else-if="loading"
      class="rounded-lg border border-line bg-surface px-3 py-4 text-[12px] text-txt3"
    >
      {{ t('common.states.loading') }}
    </div>

    <div v-else class="overflow-hidden rounded-lg border border-line bg-surface">
      <!-- Desktop -->
      <table class="hidden w-full text-left text-sm md:table">
        <thead>
          <tr class="border-b border-line text-[11px] uppercase tracking-wide text-txt3">
            <th class="px-4 py-2.5 font-medium">{{ t('pages.projectDetail.eventRouting.colEvent') }}</th>
            <th class="px-4 py-2.5 font-medium">{{ t('pages.projectDetail.eventRouting.colLive') }}</th>
            <th class="px-4 py-2.5 font-medium">{{ t('pages.projectDetail.eventRouting.colTemplate') }}</th>
            <th class="px-4 py-2.5 font-medium">{{ t('pages.projectDetail.eventRouting.colUnbindable') }}</th>
          </tr>
        </thead>
        <tbody>
          <tr
            v-for="route in routes"
            :key="route.key"
            class="border-b border-line/60 last:border-0"
            :data-testid="`event-routing-row-${route.key}`"
          >
            <td class="px-4 py-3 align-top">
              <div class="text-[13px] font-medium text-txt">{{ eventLabel(route.key) }}</div>
              <div v-if="eventTrigger(route.key)" class="mt-0.5 text-[12px] text-txt3">
                {{ eventTrigger(route.key) }}
              </div>
            </td>
            <td class="px-4 py-3 align-top text-[12px]" :class="liveTone(route)">
              {{ liveLabel(route) }}
            </td>
            <td class="px-4 py-3 align-top text-[12px]" :class="templateTone(route)">
              {{ templateLabel(route) }}
            </td>
            <td class="px-4 py-3 align-top text-[12px] text-txt2">
              {{
                route.unbindable
                  ? t('pages.projectDetail.eventRouting.yes')
                  : t('pages.projectDetail.eventRouting.no')
              }}
            </td>
          </tr>
        </tbody>
      </table>

      <!-- Mobile -->
      <div class="divide-y divide-line/60 md:hidden">
        <div v-for="route in routes" :key="route.key" class="px-4 py-3">
          <div class="text-[13px] font-medium text-txt">{{ eventLabel(route.key) }}</div>
          <div v-if="eventTrigger(route.key)" class="mt-0.5 text-[12px] text-txt3">
            {{ eventTrigger(route.key) }}
          </div>
          <dl class="mt-2 space-y-1 text-[12px]">
            <div class="flex gap-2">
              <dt class="w-20 shrink-0 text-txt3">{{ t('pages.projectDetail.eventRouting.colLive') }}</dt>
              <dd :class="liveTone(route)">{{ liveLabel(route) }}</dd>
            </div>
            <div class="flex gap-2">
              <dt class="w-20 shrink-0 text-txt3">{{ t('pages.projectDetail.eventRouting.colTemplate') }}</dt>
              <dd :class="templateTone(route)">{{ templateLabel(route) }}</dd>
            </div>
            <div class="flex gap-2">
              <dt class="w-20 shrink-0 text-txt3">{{ t('pages.projectDetail.eventRouting.colUnbindable') }}</dt>
              <dd class="text-txt2">
                {{
                  route.unbindable
                    ? t('pages.projectDetail.eventRouting.yes')
                    : t('pages.projectDetail.eventRouting.no')
                }}
              </dd>
            </div>
          </dl>
        </div>
      </div>
    </div>

    <p class="mt-2 text-[12px] text-txt3">
      {{ t('pages.projectDetail.eventRouting.footnote') }}
    </p>
  </div>
</template>
