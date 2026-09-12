<script setup lang="ts">
import AgentStudioView from '@/views/AgentStudioView.vue'

defineProps<{ projectId: string }>()
</script>

<template>
  <div
    class="flex min-h-0 flex-1 flex-col"
    data-testid="project-agents-panel"
  >
    <AgentStudioView :project-id="projectId" :embedded="true" />
  </div>
</template>
