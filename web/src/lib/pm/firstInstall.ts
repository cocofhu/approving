import { ref } from 'vue'
import { api } from '@/lib/api/api'
import {
  DEFAULT_PROJECT_ID,
  isOnboardingDismissed,
  shouldAutoOpenOnboarding,
} from '@/lib/pm/onboardingWizard'

/**
 * First install is an app-level state, not a project one: the wizard is mounted
 * once in App.vue and opens on entry regardless of the current route.
 */
export const firstInstallOpen = ref(false)

/** Bumped after a successful bootstrap so open views can refetch. */
export const firstInstallCompletedAt = ref(0)

export function openFirstInstall(): void {
  firstInstallOpen.value = true
}

export function closeFirstInstall(): void {
  firstInstallOpen.value = false
}

export function markFirstInstallCompleted(): void {
  firstInstallCompletedAt.value = Date.now()
}

let probe: Promise<void> | null = null

/** Probes once per session; later route changes must not re-open the wizard. */
export function probeFirstInstall(): Promise<void> {
  if (!probe) probe = runProbe()
  return probe
}

export function resetFirstInstallProbe(): void {
  probe = null
  firstInstallOpen.value = false
}

async function runProbe(): Promise<void> {
  if (isOnboardingDismissed(DEFAULT_PROJECT_ID)) return
  try {
    // getProject rejects when the default project is absent — nothing to bootstrap into.
    const [, workflows, agents] = await Promise.all([
      api.getProject(DEFAULT_PROJECT_ID),
      api.listWorkflows({ projectId: DEFAULT_PROJECT_ID }),
      api.listAgents(),
    ])
    const named = agents.map((a) => ({ name: a.name, projectId: a.projectId }))
    if (shouldAutoOpenOnboarding(DEFAULT_PROJECT_ID, workflows.length, named)) {
      firstInstallOpen.value = true
    }
  } catch {
    /* offline or no default project: stay silent, CTA still opens it manually */
  }
}
