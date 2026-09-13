import type { Page } from '@playwright/test'

/** localStorage key read by shouldAutoOpenOnboarding. */
export function onboardingDismissStorageKey(projectId: string): string {
  return `grasp-onboarding-suppress:${projectId}`
}

/**
 * Seed dismiss flag before navigation so stubs without the default workflow do
 * not auto-open the onboarding overlay and intercept pointer events in
 * unrelated e2e suites. This storage key is the only hard suppression left —
 * the wizard's own "later" button closes it for the current view only.
 */
export async function seedOnboardingDismissed(
  page: Page,
  projectId = 'proj-1',
): Promise<void> {
  await page.addInitScript((pids: string[]) => {
    try {
      for (const pid of pids) {
        localStorage.setItem(`grasp-onboarding-suppress:${pid}`, '1')
      }
    } catch {
      /* ignore */
    }
  }, [projectId, 'proj-default'])
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
