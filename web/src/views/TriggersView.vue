<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/ui/Icon.vue'

const { t } = useI18n()

const triggers = computed(() => [
  { type: t('pages.triggers.items.manual.type'), icon: 'play', desc: t('pages.triggers.items.manual.desc'), available: true },
  { type: t('pages.triggers.items.webhook.type'), icon: 'connector', desc: t('pages.triggers.items.webhook.desc'), available: false },
  { type: t('pages.triggers.items.cron.type'), icon: 'clock', desc: t('pages.triggers.items.cron.desc'), available: false },
  { type: t('pages.triggers.items.gitlab.type'), icon: 'connector', desc: t('pages.triggers.items.gitlab.desc'), available: false },
])
</script>

<template>
  <div>
    <div class="mb-5">
      <h2 class="text-lg font-semibold text-txt">{{ t('pages.triggers.title') }}</h2>
      <p class="text-sm text-txt3">{{ t('pages.triggers.subtitle') }}</p>
    </div>
    <div class="space-y-3">
      <div v-for="item in triggers" :key="item.type" class="card flex min-h-11 items-center gap-3 p-4" :class="item.available ? '' : 'opacity-70'">
        <div class="flex h-10 w-10 shrink-0 items-center justify-center rounded-md" :class="item.available ? 'bg-accent-dim text-accent-2' : 'bg-elevated text-txt3'">
          <Icon :name="item.icon" :size="18" />
        </div>
        <div class="min-w-0 flex-1">
          <div class="flex flex-wrap items-center gap-2">
            <span class="font-medium text-txt">{{ item.type }}</span>
            <span v-if="item.available" class="chip border-ok/30 text-ok">{{ t('pages.triggers.available') }}</span>
            <span v-else class="chip">{{ t('pages.triggers.planned') }}</span>
          </div>
          <div class="break-words text-[12px] text-txt3">{{ item.desc }}</div>
        </div>
      </div>
    </div>
  </div>
</template>
