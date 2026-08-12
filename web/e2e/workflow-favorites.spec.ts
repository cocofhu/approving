import { test, expect } from '@playwright/test'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const __dirname = path.dirname(fileURLToPath(import.meta.url))
const shotDir = path.join(__dirname, '..', '..', '.tmp-test-shots')

test.describe('workflow favorites quick-launch', () => {
  test('empty state keeps section title and guide copy', async ({ page }) => {
    await page.goto('/workflow-favorites.html?scene=empty')
    await expect(page.getByTestId('nav-quick-pipelines')).toBeVisible({ timeout: 15_000 })
    await expect(page.getByTestId('nav-quick-pipelines')).toContainText('快捷流水线')
    await expect(page.getByTestId('nav-quick-pipelines-empty')).toContainText('星标')
    // Primary nav still present (notifications before quick zone)
    await expect(page.getByRole('link', { name: '通知' })).toBeVisible()
    await expect(page.getByRole('link', { name: '设置' })).toBeVisible()
    await page.screenshot({ path: path.join(shotDir, 'fav-empty.png'), fullPage: true })
  })

  test('sidebar lists favorites with project subtitle and draft badge; launch opens modal', async ({
    page,
  }) => {
    await page.goto('/workflow-favorites.html?scene=with-items')
    await expect(page.getByTestId('nav-quick-pipeline-item').first()).toBeVisible({ timeout: 15_000 })
    const items = page.getByTestId('nav-quick-pipeline-item')
    await expect(items).toHaveCount(3)
    // Newest favoritedAt first: wf-night
    await expect(items.nth(0)).toContainText('夜间回归')
    await expect(items.nth(0)).toContainText('checkout-service')
    await expect(items.nth(0)).toContainText('草稿')
    await expect(items.nth(1)).toContainText('热修回滚')
    await expect(items.nth(2)).toContainText('发布预检')
    await expect(items.nth(2)).toContainText('billing-api')

    const before = page.url()
    await items.nth(0).click()
    await expect(page.getByText('启动运行 · 夜间回归')).toBeVisible({ timeout: 10_000 })
    await expect(page.getByText('分支')).toBeVisible()
    // Route must not be forced to project/editor
    expect(page.url()).toBe(before)
    await page.screenshot({ path: path.join(shotDir, 'fav-launch-modal.png'), fullPage: true })
  })

  test('sidebar star unfavorites without opening launch modal', async ({ page }) => {
    await page.goto('/workflow-favorites.html?scene=with-items')
    await expect(page.getByTestId('nav-quick-pipeline-item').first()).toBeVisible({ timeout: 15_000 })
    await page.getByTestId('nav-quick-pipeline-unfavorite').first().click()
    await expect(page.getByText('启动运行 ·')).toHaveCount(0)
    await expect(page.getByTestId('nav-quick-pipeline-item')).toHaveCount(2)
    await expect(page.getByTestId('nav-quick-pipeline-item').first()).not.toContainText('夜间回归')
    await page.screenshot({ path: path.join(shotDir, 'fav-unfavorite.png'), fullPage: true })
  })

  test('no-ask fields still open confirm modal from quick item', async ({ page }) => {
    await page.goto('/workflow-favorites.html?scene=with-items')
    await expect(page.getByTestId('nav-quick-pipeline-item').nth(1)).toBeVisible({ timeout: 15_000 })
    await page.getByTestId('nav-quick-pipeline-item').nth(1).click() // 热修回滚 — no ask fields
    await expect(page.getByText('启动运行 · 热修回滚')).toBeVisible({ timeout: 10_000 })
    await expect(page.getByText(/未声明任何输入字段|空参数启动/)).toBeVisible()
    await page.screenshot({ path: path.join(shotDir, 'fav-launch-no-params.png'), fullPage: true })
  })

  test('full limit rejects 9th favorite with toast', async ({ page }) => {
    await page.goto('/workflow-favorites.html?scene=full')
    await expect(page.getByTestId('nav-quick-pipeline-item').first()).toBeVisible({ timeout: 15_000 })
    await expect(page.getByTestId('nav-quick-pipeline-item')).toHaveCount(8)
    await page.getByTestId('demo-favorite-extra').click()
    await expect(page.getByTestId('toast-host')).toContainText('已达 8 条上限', { timeout: 5_000 })
    await expect(page.getByTestId('nav-quick-pipeline-item')).toHaveCount(8)
    await page.screenshot({ path: path.join(shotDir, 'fav-full-toast.png'), fullPage: true })
  })

  test('404 favorite is silently stripped on hydrate', async ({ page }) => {
    await page.goto('/workflow-favorites.html?scene=gone')
    await expect(page.getByTestId('nav-quick-pipeline-item')).toHaveCount(1, { timeout: 15_000 })
    await expect(page.getByTestId('nav-quick-pipeline-item')).toContainText('夜间回归')
    await expect(page.getByTestId('nav-quick-pipelines')).not.toContainText('wf-missing')
    // No dedicated "unavailable" toast required — toast host should not warn about strip
    await page.screenshot({ path: path.join(shotDir, 'fav-gone-strip.png'), fullPage: true })
  })
})
