<script setup lang="ts">
import { computed } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import AppSidebarNav from './AppSidebarNav.vue'
import BrandLogo from './BrandLogo.vue'
import Icon from '../ui/Icon.vue'
import { authApi } from '@/lib/api/api'
import { useAuth } from '@/lib/composables/useAuth'
import {
  focusDesktopNavControl,
  hideDesktopSidebar,
  sidebarHidden,
} from '@/lib/shared/sidebarHidden'

const { t } = useI18n()
const router = useRouter()
const { user, clearUser } = useAuth()

const initials = computed(() => {
  const name = user.value?.username || '?'
  return name.slice(0, 2).toUpperCase()
})

async function logout() {
  try {
    await authApi.logout()
  } catch {
    // ignore — cookie cleared server-side when possible
  }
  clearUser()
  await router.push('/login')
}

async function onHideNav() {
  hideDesktopSidebar()
  await focusDesktopNavControl('show')
}
</script>

<template>
  <aside
    id="app-desktop-sidebar"
    class="app-desktop-sidebar hidden h-full min-w-0 shrink-0 flex-col overflow-hidden bg-surface md:flex"
    :class="sidebarHidden ? 'w-0 border-r-0' : 'w-[232px] border-r border-line'"
    data-testid="app-desktop-sidebar"
    :aria-hidden="sidebarHidden ? 'true' : undefined"
    :inert="sidebarHidden"
  >
    <div class="flex h-full w-[232px] min-w-[232px] flex-col">
      <div class="flex h-14 items-center justify-between gap-2 px-3">
        <BrandLogo size="md" align="start" show-mark />
        <button
          type="button"
          class="flex h-11 w-11 shrink-0 items-center justify-center rounded-md text-txt2 hover:bg-elevated hover:text-txt"
          data-testid="desktop-nav-hide"
          :aria-label="t('shell.aria.hideNav')"
          :title="t('shell.aria.hideNav')"
          aria-expanded="true"
          aria-controls="app-desktop-sidebar"
          @click="onHideNav"
        >
          <Icon name="panel-left" :size="18" />
        </button>
      </div>

      <AppSidebarNav />

      <div class="border-t border-line p-3">
        <div class="flex items-center gap-2.5 rounded-md px-2 py-1.5">
          <div class="flex h-8 w-8 items-center justify-center rounded-full bg-elevated text-xs font-semibold text-accent-2">{{ initials }}</div>
          <div class="flex-1 truncate leading-tight">
            <div class="truncate text-[13px] font-medium text-txt">{{ user?.username || '—' }}</div>
          </div>
          <button
            type="button"
            class="shrink-0 border border-line px-2 py-1 text-[11px] text-txt3 transition hover:border-err/40 hover:bg-err/10 hover:text-err"
            @click="logout"
          >
            {{ t('shell.logout') }}
          </button>
        </div>
      </div>
    </div>
  </aside>
</template>
