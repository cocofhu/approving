import { test, expect } from '@playwright/test'

test.describe('human_gate 临时审批链接', () => {
  test('Inbox 复制临时链接打开管理面板', async ({ page }) => {
    await page.addInitScript(() => {
      Object.defineProperty(navigator, 'clipboard', {
        configurable: true,
        value: {
          writeText: async (text: string) => {
            ;(window as unknown as { __copied?: string }).__copied = text
          },
        },
      })
    })
    await page.setViewportSize({ width: 1280, height: 800 })
    await page.goto('/gate-share-link.html?scene=inbox')
    await expect(page.getByTestId('gate-share-e2e-root')).toBeVisible({ timeout: 10_000 })
    await expect(page.getByTestId('gate-share-copy-btn')).toBeVisible()
    await expect(page.getByTestId('gate-share-status')).toContainText('尚未创建')
    await page.getByTestId('gate-share-copy-btn').click()
    await expect(page.getByTestId('gate-share-panel-body')).toBeVisible()
    await expect(page.getByTestId('gate-share-panel-body')).toContainText('信任')
    await page.getByTestId('gate-share-create').click()
    await expect(page.getByTestId('gate-share-url')).toBeVisible()
    await expect(page.getByTestId('gate-share-url')).toHaveValue(/#t=••••/)
    const copied = await page.evaluate(() => (window as unknown as { __copied?: string }).__copied || '')
    expect(copied).toContain('/public/gate-approvals#t=')
    expect(copied).toMatch(/#t=[0-9a-f]{64}$/)
  })

  test('未登录外部页可批准', async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 })
    await page.goto('/gate-share-link.html?scene=public')
    await expect(page.getByTestId('public-gate-root')).toBeVisible({ timeout: 10_000 })
    await expect(page.getByTestId('public-gate-title')).toHaveText('审阅视觉稿')
    await expect(page.getByTestId('public-gate-root')).not.toContainText('run-e2e')
    await page.getByTestId('public-gate-name').fill('Jordan')
    await page.getByTestId('public-gate-approve').click()
    await expect(page.getByTestId('public-gate-done')).toContainText('已批准')
    await expect(page.getByTestId('public-gate-approve')).toHaveCount(0)
  })

  test('未登录外部页驳回需意见', async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 })
    await page.goto('/gate-share-link.html?scene=public')
    await expect(page.getByTestId('public-gate-reject')).toBeVisible({ timeout: 10_000 })
    await page.getByTestId('public-gate-reject').click()
    await expect(page.getByText('驳回必须填写意见')).toBeVisible()
    await page.getByTestId('public-gate-comment').fill('需要修改文案')
    await page.getByTestId('public-gate-reject').click()
    await expect(page.getByTestId('public-gate-done')).toContainText('已驳回')
  })
})
