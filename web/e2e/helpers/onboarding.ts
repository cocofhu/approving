import type { Page } from '@playwright/test'

/** localStorage key used by shouldAutoOpenOnboarding / dismissOnboarding. */
export function onboardingDismissStorageKey(projectId: string): string {
  return `approving-onboarding-dismiss:${projectId}`
}

/**
 * Seed dismiss flag before navigation so empty ProjectDetailView stubs
 * (0 workflows + 0 agents) do not auto-open the onboarding overlay and
 * intercept pointer events in unrelated e2e suites.
 */
export async function seedOnboardingDismissed(
  page: Page,
  projectId = 'proj-1',
): Promise<void> {
  await page.addInitScript((pid: string) => {
    try {
      localStorage.setItem(`approving-onboarding-dismiss:${pid}`, '1')
    } catch {
      /* ignore */
    }
  }, projectId)
}

/**
 * If the onboarding wizard is already open, dismiss via backdrop click.
 * Safe no-op when the overlay is absent.
 */
export async function dismissOnboardingIfOpen(page: Page): Promise<void> {
  const backdrop = page.getByTestId('onboarding-backdrop')
  if ((await backdrop.count()) === 0) return
  if (!(await backdrop.first().isVisible().catch(() => false))) return
  await backdrop.first().click({ force: true })
  await page.getByTestId('onboarding-backdrop').waitFor({ state: 'hidden', timeout: 5_000 }).catch(() => {})
}
