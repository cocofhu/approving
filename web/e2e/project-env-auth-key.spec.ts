import { test, expect } from '@playwright/test'
import { dismissOnboardingIfOpen, seedOnboardingDismissed } from './helpers/onboarding'

const MOCK_PROJECT = {
  id: 'proj-1',
  name: 'Demo Project',
  description: 'Project for e2e',
  sandboxEnv: [
    { key: 'CURSOR_API_KEY', value: '****', secret: true },
    { key: 'API_URL', value: 'https://example.com', secret: false },
  ],
  variables: [],
  createdAt: '2026-01-01T00:00:00Z',
  updatedAt: '2026-01-01T00:00:00Z',
}

const MOCK_WORKFLOWS: unknown[] = []

async function gotoProjectDetail(page: import('@playwright/test').Page) {
  await page.setViewportSize({ width: 1280, height: 800 })
  await seedOnboardingDismissed(page, 'proj-1')
  await page.route('**/api/projects/proj-1', async (route) => {
    if (route.request().method() === 'GET' || route.request().method() === 'PUT') {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify(MOCK_PROJECT),
      })
      return
    }
    await route.continue()
  })
  await page.route('**/api/workflows**', async (route) => {
    if (route.request().method() === 'GET') {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify(MOCK_WORKFLOWS),
      })
      return
    }
    await route.continue()
  })
  await page.route('**/api/runs**', async (route) => {
    if (route.request().method() === 'GET') {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ items: [], total: 0, page: 1, pageSize: 20, hasMore: false }),
      })
      return
    }
    await route.continue()
  })
  await page.route('**/api/projects/proj-1/pm-leader', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ enabled: false }),
    })
  })
  await page.goto('/project-detail.html')
  await dismissOnboardingIfOpen(page)
  await expect(page.getByRole('heading', { name: 'Demo Project' })).toBeVisible({ timeout: 10_000 })
}

test.describe('项目 env 官方 ACP 鉴权键 UI', () => {
  test('CURSOR_API_KEY 显示强制 Secret 且不可切明文；行内强制徽标可见', async ({ page }) => {
    await gotoProjectDetail(page)
    await page.getByRole('button', { name: '沙箱环境变量' }).click()

    const panel = page.locator('.border.border-line.bg-surface').filter({ hasText: 'KEY' }).first()
    await expect(panel).toBeVisible()
    await expect(panel.getByRole('textbox', { name: 'KEY' }).first()).toHaveValue('CURSOR_API_KEY')
    await expect(panel.getByText('强制', { exact: true })).toBeVisible()

    const cursorRow = panel
      .locator('.grid.grid-cols-1')
      .filter({ has: page.getByRole('button', { name: '密钥' }) })
      .first()
    await expect(cursorRow.getByRole('button', { name: '密钥' })).toBeDisabled()
    await expect(panel.getByRole('button', { name: '明文' })).toBeEnabled()

    // g3.2: 不再经「合并规则」弹窗断言帮助文案；行内强制 Secret UI 已覆盖鉴权键约束
    await expect(page.getByRole('button', { name: '合并规则' })).toHaveCount(0)
    await expect(page.getByTestId('project-detail-merge-rules')).toHaveCount(0)
    await expect(page.getByText('注入流水线节点沙箱的 OS 环境变量。')).toHaveCount(0)
    await expect(cursorRow.getByText('强制', { exact: true })).toBeVisible()
  })

  test('输入 CURSOR_API_KEY 时自动锁定 Secret', async ({ page }) => {
    await gotoProjectDetail(page)
    await page.getByRole('button', { name: '沙箱环境变量' }).click()

    const panel = page.locator('.border.border-line.bg-surface').filter({ hasText: 'KEY' }).first()
    await panel.getByRole('button', { name: '添加一行' }).click()

    const newRow = panel.locator('.grid.grid-cols-1').last()
    await newRow.locator('input').first().fill('CURSOR_API_KEY')
    await expect(newRow.getByRole('button', { name: '密钥' })).toBeDisabled()
    await expect(newRow.getByText('强制', { exact: true })).toBeVisible()
  })
})
