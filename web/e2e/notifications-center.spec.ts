import { expect, test, type Page } from '@playwright/test'

/** Wait for /auth/me to paint; prefs must hydrate on auth settle (no focus/poll required). */
async function settleAuth(page: Page) {
  await expect(page.getByText('e2e', { exact: true })).toBeVisible({ timeout: 15_000 })
}

test.describe('shell notification center (IA separation)', () => {
  test('dropdown titled 通知; empty has no clickable runs escape', async ({ page }) => {
    await page.goto('/notifications-center.html?scene=empty')
    await expect(page.getByTestId('shell-main-dashboard')).toBeVisible({ timeout: 15_000 })
    await settleAuth(page)

    const bell = page.getByTestId('run-notifications-bell')
    await expect(bell).toHaveAttribute('aria-label', '通知')
    await expect(page.getByTestId('run-notifications-badge')).toHaveCount(0)

    await bell.click()
    const panel = page.getByTestId('run-notifications-panel')
    await expect(panel).toBeVisible()
    await expect(panel.getByRole('heading', { name: '通知' })).toBeVisible()
    await expect(panel).not.toContainText('运行通知')

    const empty = page.getByTestId('run-notifications-empty')
    await expect(empty).toContainText('暂无通知')
    await expect(empty).toContainText('执行完成或失败后才会出现')
    await expect(empty).toContainText('运行')
    await expect(empty.locator('a')).toHaveCount(0)
    await expect(empty.locator('button')).toHaveCount(0)
  })

  test('history-only enable: badge not inventory; list items are read', async ({ page }) => {
    await page.goto('/notifications-center.html?scene=history-only&start=notifications')
    await expect(page.getByTestId('notifications-page')).toBeVisible({ timeout: 15_000 })
    await settleAuth(page)
    await expect(page.getByTestId('run-notifications-badge')).toHaveCount(0)
    const items = page.getByTestId('notifications-item')
    await expect(items).toHaveCount(2)
    for (const el of await items.all()) {
      await expect(el).toHaveAttribute('data-unread', 'false')
    }
  })

  test('cleans noisy titles; view-all and sidebar land on /notifications; failed goes to detail', async ({
    page,
  }) => {
    await page.goto('/notifications-center.html?scene=with-items')
    await expect(page.getByTestId('shell-main-dashboard')).toBeVisible({ timeout: 15_000 })
    await settleAuth(page)

    const badge = page.getByTestId('run-notifications-badge')
    await expect(badge).toBeVisible()
    await expect(badge).toHaveText('3')

    await page.getByTestId('run-notifications-bell').click()
    const panel = page.getByTestId('run-notifications-panel')
    await expect(panel).toBeVisible()
    await expect(panel).toContainText('自我迭代 · 已完成')
    await expect(panel).not.toContainText('运行中 4')

    const items = panel.getByTestId('run-notifications-item')
    await expect(items).toHaveCount(3)

    await page.getByTestId('run-notifications-view-all').click()
    await expect(page.getByTestId('notifications-page')).toBeVisible()
    await expect(page.getByRole('heading', { name: '通知' })).toBeVisible()
    await expect(page.getByTestId('shell-main-runs')).toHaveCount(0)

    // Sidebar dual entry still present alongside runs/gates
    await expect(page.getByRole('link', { name: '通知' })).toBeVisible()
    await expect(page.getByRole('link', { name: '运行' })).toBeVisible()
    await expect(page.getByRole('link', { name: '待审批' })).toBeVisible()

    await page.getByTestId('notifications-filter-unread').click()
    await expect(page.getByTestId('notifications-item')).toHaveCount(3)

    await page.locator('[data-testid="notifications-item"][data-status="failed"]').click()
    await expect(page.getByTestId('shell-main-run-detail')).toContainText('run-new-fail')
  })

  test('completed click opens output without marking read; mark-as-read then clears badge', async ({
    page,
  }) => {
    await page.goto('/notifications-center.html?scene=with-items')
    await expect(page.getByTestId('shell-main-dashboard')).toBeVisible({ timeout: 15_000 })
    await settleAuth(page)
    await expect(page.getByTestId('run-notifications-badge')).toHaveText('3')
    await page.getByTestId('run-notifications-bell').click()
    await page
      .locator('[data-testid="run-notifications-item"][data-status="completed"]')
      .first()
      .click()
    await expect(page.getByTestId('run-output-empty').or(page.getByTestId('run-output-master-detail'))).toBeVisible({
      timeout: 10_000,
    })
    // Opening alone must NOT drop unread (3 stays 3)
    await expect(page.getByTestId('run-notifications-badge')).toHaveText('3')
    await page.getByTestId('run-output-mark-read').click()
    await expect(page.getByTestId('run-output-mark-read')).toHaveCount(0)
    // badge should drop by 1 (3 → 2) only after mark-as-read
    await expect(page.getByTestId('run-notifications-badge')).toHaveText('2')
  })

  test('auth settle: badge matches unread immediately after /auth/me (no focus needed)', async ({
    page,
  }) => {
    await page.goto('/notifications-center.html?scene=with-items')
    await expect(page.getByTestId('shell-main-dashboard')).toBeVisible({ timeout: 15_000 })
    await expect(page.getByText('e2e', { exact: true })).toBeVisible({ timeout: 15_000 })
    // Must not under-count until focus/15s poll — auth watch rehydrates prefs sync.
    await expect(page.getByTestId('run-notifications-badge')).toHaveText('3', { timeout: 5_000 })
  })
})
