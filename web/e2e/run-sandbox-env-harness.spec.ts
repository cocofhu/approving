import { test, expect } from '@playwright/test'
import path from 'node:path'
import fs from 'node:fs'

const OUT = path.join('/tmp', 'run-sandbox-env-e2e-shots')

test.beforeAll(() => {
  fs.mkdirSync(OUT, { recursive: true })
})

test('RunLaunchModal 与 Run 快照面板浏览器验收', async ({ page }) => {
  await page.route('**/api/**', async (route) => {
    if (!new URL(route.request().url()).pathname.startsWith('/api/')) {
      await route.continue()
      return
    }
    const url = new URL(route.request().url())
    if (url.pathname.includes('/runs') && route.request().method() === 'POST') {
      const body = route.request().postDataJSON() as { env?: { key: string; value: string; secret?: boolean }[] }
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          id: 'run-env-e2e',
          status: 'queued',
          sandboxEnv: body.env || [],
        }),
      })
      return
    }
    if (url.pathname.includes('/run-tags')) {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ tags: [] }),
      })
      return
    }
    await route.fulfill({ status: 200, contentType: 'application/json', body: '[]' })
  })

  await page.goto('/run-sandbox-env-harness.html', { waitUntil: 'networkidle' })
  await expect(page.getByTestId('run-sandbox-env-root')).toBeVisible()
  await expect(page.getByText('运行级环境变量').first()).toBeVisible()
  await page.screenshot({ path: path.join(OUT, '01-launch-modal.png'), fullPage: true })

  await page.getByRole('button', { name: '添加行' }).click()
  const row = page.getByTestId('run-launch-env-row').first()
  await row.locator('input').nth(0).fill('CURSOR_API_KEY')
  await row.locator('input').nth(1).fill('sk-demo')
  await page.getByRole('button', { name: /开始运行|Start/i }).click()
  await expect(page.getByText(/CURSOR_API_KEY/)).toBeVisible()
  await page.screenshot({ path: path.join(OUT, '02-reject-auth-key.png'), fullPage: true })

  await row.locator('input').nth(0).fill('LOG_LEVEL')
  await row.locator('input').nth(1).fill('debug')
  await page.getByRole('button', { name: /开始运行|Start/i }).click()
  // plan g2.2: sandbox env start still lands on success phase before navigation
  await expect(page.getByText(/工作流已启动|Workflow started/)).toBeVisible()
  await expect(page.getByRole('button', { name: /查看运行|View run/i })).toBeVisible()
  await expect(page.getByRole('button', { name: /留在当前页|Stay on this page/i })).toBeVisible()
  await expect(page.getByTestId('last-start')).toContainText('started:run-env-e2e')
  await page.screenshot({ path: path.join(OUT, '03-success-phase.png'), fullPage: true })

  await page.getByRole('button', { name: /查看运行|View run/i }).click()
  await expect(page.getByTestId('last-start')).toContainText('view-run:run-env-e2e')
  await expect(page.getByTestId('run-sandbox-env-panel')).toBeVisible()
  await expect(page.getByText('••••••••')).toBeVisible()
  await expect(page.getByText('编辑')).toHaveCount(0)
  await page.screenshot({ path: path.join(OUT, '04-run-detail-snapshot.png'), fullPage: true })
})
