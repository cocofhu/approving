import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'

export const PROJECT_CONTEXT_STORAGE_KEY = 'approving-project-context'

/** Sentinel for「全部项目」— empty string in URL/storage means all. */
const PROJECT_CONTEXT_ALL = ''

export function readStoredProjectId(): string {
  try {
    const v = localStorage.getItem(PROJECT_CONTEXT_STORAGE_KEY)
    return v == null ? PROJECT_CONTEXT_ALL : v
  } catch {
    return PROJECT_CONTEXT_ALL
  }
}

export function writeStoredProjectId(id: string) {
  try {
    if (!id) localStorage.removeItem(PROJECT_CONTEXT_STORAGE_KEY)
    else localStorage.setItem(PROJECT_CONTEXT_STORAGE_KEY, id)
  } catch {
    /* ignore quota / private mode */
  }
}

/**
 * Project context for Runs / Gates / Artifacts.
 * - URL `?projectId=` is the source of truth when present
 * - On first visit without query: restore last choice from localStorage;
 *   if none, default to「全部项目」
 * - Changing selection updates URL + localStorage
 */
export function useProjectContext() {
  const route = useRoute()
  const router = useRouter()

  const selected = computed<string>({
    get: () => {
      if (typeof route.query.projectId === 'string') return route.query.projectId
      // Absent from URL → treat as「全部」for the getter; hydration happens via ensureHydrated.
      return PROJECT_CONTEXT_ALL
    },
    set: (val) => {
      writeStoredProjectId(val)
      const query = { ...route.query }
      if (val) query.projectId = val
      else delete query.projectId
      router.replace({ query })
    },
  })

  /** Call once on mount of platform list pages to restore stored project into URL. */
  function ensureHydrated() {
    if (typeof route.query.projectId === 'string') {
      writeStoredProjectId(route.query.projectId)
      return
    }
    const stored = readStoredProjectId()
    if (stored) {
      const query = { ...route.query, projectId: stored }
      router.replace({ query })
    }
  }

  function setProject(id: string) {
    selected.value = id
  }

  return { selected, ensureHydrated, setProject }
}
