<script setup lang="ts">
import { RouterLink, useRoute } from 'vue-router'
import { watch, onMounted, onBeforeUnmount } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '../ui/Icon.vue'
import { sidebarNavGroups } from '@/data/sidebarNav'
import { usePendingGates } from '@/lib/usePendingGates'
import { useRunTerminalNotifications } from '@/lib/useRunTerminalNotifications'

defineProps<{ drawer?: boolean }>()
const emit = defineEmits<{ (e: 'navigate'): void }>()

const { t } = useI18n()
const route = useRoute()

// Shared singleton source so approving a gate elsewhere updates the badge immediately.
const { count: gateCount, peek, refresh } = usePendingGates()
// Same unreadCount singleton as topbar bell — keep sidebar /notifications badge in sync.
const { unreadCount } = useRunTerminalNotifications()
let timer: number | undefined
function badgeFor(to: string): number {
  if (to === '/gates') return gateCount.value
  if (to === '/notifications') return unreadCount.value
  return 0
}
function pollRefresh() {
  return peek({ source: 'sidebar-poll' })
}
onMounted(() => {
  refresh({ source: 'mount' })
  timer = window.setInterval(pollRefresh, 15000)
})
onBeforeUnmount(() => {
  if (timer) clearInterval(timer)
})
// Refresh promptly when navigating (e.g. right after approving a gate).
watch(() => route.path, () => refresh({ source: 'navigate' }))

function isActive(to: string) {
  return route.path === to || route.path.startsWith(to + '/')
}

function onNavigate() {
  emit('navigate')
}
</script>

<template>
  <nav class="scroll-area flex-1 overflow-y-auto px-3 py-2">
    <div v-for="(g, gi) in sidebarNavGroups" :key="gi" class="mb-3">
      <div v-if="g.titleKey" class="px-3 pb-1 pt-2 text-[10px] font-semibold uppercase tracking-wider text-txt3">
        {{ t(g.titleKey) }}
      </div>
      <RouterLink
        v-for="item in g.items"
        :key="item.to"
        :to="item.to"
        class="nav-item mb-0.5"
        :class="{ active: isActive(item.to) }"
        @click="onNavigate"
      >
        <Icon :name="item.icon" :size="17" />
        <span class="flex-1">{{ t(item.labelKey) }}</span>
        <span
          v-if="badgeFor(item.to)"
          class="flex h-5 min-w-5 items-center justify-center rounded-full bg-accent px-1.5 text-[11px] font-semibold text-white"
          :data-testid="item.to === '/notifications' ? 'nav-notifications-badge' : item.to === '/gates' ? 'nav-gates-badge' : undefined"
        >{{ badgeFor(item.to) }}</span>
      </RouterLink>
    </div>
  </nav>
</template>
