<script setup lang="ts">
import { onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { readStoredProjectId } from '@/lib/useProjectContext'

/**
 * Legacy /board bookmark compatibility:
 * - remembered project → /projects/{id}?tab=board
 * - otherwise → /projects
 * Never renders an unfiltered global board.
 */
const router = useRouter()

onMounted(() => {
  const id = readStoredProjectId()
  if (id) {
    void router.replace({ path: `/projects/${id}`, query: { tab: 'board' } })
    return
  }
  void router.replace({ path: '/projects' })
})
</script>

<template>
  <div class="p-6 text-sm text-txt3" data-testid="board-redirect">
    …
  </div>
</template>
