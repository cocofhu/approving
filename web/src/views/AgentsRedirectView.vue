<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { api } from '@/lib/api/api'
import { readStoredProjectId } from '@/lib/composables/useProjectContext'

const STUDIO_TABS = ['files', 'mcp', 'env', 'prompts', 'platform-rules', 'meta', 'data'] as const

function isStudioTab(q: unknown): q is (typeof STUDIO_TABS)[number] {
  return typeof q === 'string' && (STUDIO_TABS as readonly string[]).includes(q)
}

/**
 * Legacy /agents bookmark compatibility:
 * - ?agent=… → project of that agent, agents tab (+ studioTab/sub)
 * - remembered project → /projects/{id}?tab=agents
 * - else first project, or /projects if none
 */
const route = useRoute()
const router = useRouter()
const busy = ref(true)

onMounted(async () => {
  const agentQ = typeof route.query.agent === 'string' ? route.query.agent.trim() : ''
  const legacyTab = typeof route.query.tab === 'string' ? route.query.tab : ''
  const sub = typeof route.query.sub === 'string' ? route.query.sub : ''
  const studioTabFromLegacy = isStudioTab(legacyTab) ? legacyTab : undefined
  const studioTabDirect =
    typeof route.query.studioTab === 'string' && isStudioTab(route.query.studioTab)
      ? route.query.studioTab
      : undefined
  const studioTab = studioTabDirect || studioTabFromLegacy

  if (agentQ) {
    try {
      const list = await api.listAgents()
      const hit = (list || []).find((a) => a.name === agentQ)
      if (hit?.projectId) {
        const query: Record<string, string> = { tab: 'agents', agent: agentQ }
        if (studioTab) query.studioTab = studioTab
        if (sub) query.sub = sub
        await router.replace({ path: `/projects/${hit.projectId}`, query })
        return
      }
    } catch {
      /* fall through */
    }
  }

  const stored = readStoredProjectId()
  if (stored) {
    await router.replace({ path: `/projects/${stored}`, query: { tab: 'agents' } })
    return
  }

  try {
    const projects = await api.listProjects()
    const first = projects?.[0]?.id
    if (first) {
      await router.replace({ path: `/projects/${first}`, query: { tab: 'agents' } })
      return
    }
  } catch {
    /* fall through */
  }

  await router.replace({ path: '/projects' })
  busy.value = false
})
</script>

<template>
  <div class="p-6 text-sm text-txt3" data-testid="agents-redirect">
    <span v-if="busy">…</span>
  </div>
</template>
