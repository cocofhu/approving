import { test, expect } from '@playwright/test'

test.describe('shell loading infra', () => {
  test('route switch succeeds with non-empty main', async ({ page }) => {
    await page.goto('/shell-loading.html')
    await expect(page.getByTestId('shell-main-dashboard')).toBeVisible({ timeout: 10_000 })
    await expect(page.getByTestId('shell-main-dashboard')).toContainText('工作台内容')
    await page.getByTestId('go-runs').click()
    await expect(page.getByTestId('shell-main-runs')).toBeVisible({ timeout: 10_000 })
    await expect(page.getByTestId('shell-main-runs')).not.toHaveText(/^$/)
    await expect(page.getByTestId('shell-main-runs')).toContainText('运行列表')
  })

  test('load failure can retry and recover', async ({ page }) => {
    await page.goto('/shell-loading.html?scene=fail-runs&start=runs')
    await expect(page.getByTestId('app-inline-error')).toBeVisible({ timeout: 10_000 })
    await page.getByTestId('app-inline-error-retry').click()
    await expect(page.getByTestId('shell-main-runs')).toContainText('已恢复')
  })
})
