<script setup lang="ts">
/**
 * Legacy topbar shell kept only for StatusMetrics E2E harness
 * (`e2e/status-metrics-topbar-main.ts`). AppShell no longer mounts this.
 */
import ShellChromeControls from './ShellChromeControls.vue'
</script>

<template>
  <header
    class="safe-area-top relative z-20 flex h-14 shrink-0 items-center gap-3 border-b border-line bg-surface/80 px-4 backdrop-blur md:px-6"
    data-testid="shell-topbar-harness"
  >
    <div class="flex-1" />
    <ShellChromeControls layout="bar" />
  </header>
</template>
