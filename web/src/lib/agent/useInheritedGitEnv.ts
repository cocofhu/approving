import { ref, toValue, watch, type MaybeRefOrGetter } from 'vue'
import { api } from '@/lib/api/api'
import { recToKV, type KV } from '@/lib/agent/agentStudioDraft'

/**
 * Load project shared Agent env for Git-guide inheritance.
 * Failure is non-blocking: inheritedEnv stays empty and the caller falls back to local env.
 */
export function useInheritedGitEnv(projectId: MaybeRefOrGetter<string | undefined>) {
  const inheritedEnv = ref<KV[]>([])
  let seq = 0

  async function load(raw: string | undefined) {
    const my = ++seq
    const id = raw?.trim() ?? ''
    if (!id) {
      inheritedEnv.value = []
      return
    }
    try {
      const cfg = await api.getProjectSharedAgentConfig(id)
      if (my !== seq) return
      inheritedEnv.value = recToKV(cfg.env || {})
    } catch {
      if (my !== seq) return
      inheritedEnv.value = []
    }
  }

  watch(
    () => toValue(projectId),
    (id) => {
      void load(id)
    },
    { immediate: true },
  )

  return { inheritedEnv }
}
