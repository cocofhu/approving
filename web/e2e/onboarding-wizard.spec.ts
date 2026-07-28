import { test, expect } from '@playwright/test'
import path from 'node:path'
import fs from 'node:fs'

const OUT = path.join('/tmp', 'onboarding-e2e-shots')

test.beforeAll(() => {
  fs.mkdirSync(OUT, { recursive: true })
})

test('空项目上手引导五步向导浏览器验收', async ({ page }) => {
  await page.route('**/api/**', async (route) => {
    const url = new URL(route.request().url())
    if (url.pathname.includes('/bootstrap-onboarding') && route.request().method() === 'POST') {
      const body = route.request().postDataJSON() as { apiKey?: string; repos?: string }
      if (!body?.apiKey?.trim()) {
        await route.fulfill({ status: 400, contentType: 'application/json', body: JSON.stringify({ error: 'apiKey required' }) })
        return
      }
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          agentIds: ['ClarifyAgent', 'VisualAgent', 'ImplementAgent', 'TestAgent', 'PreviewAgent'],
          workflowId: 'wf-onboard-1',
          repos: body.repos || 'demo|https://github.com/heroku/nodejs-getting-started.git|main',
          feature: '把首页欢迎文案与主按钮文案改得更清晰友好',
          published: true,
        }),
      })
      return
    }
    if (url.pathname.includes('/runs') && route.request().method() === 'POST') {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ id: 'run-e2e-1', status: 'queued' }),
      })
      return
    }
    await route.fulfill({ status: 200, contentType: 'application/json', body: '[]' })
  })

  await page.goto('/onboarding-wizard.html', { waitUntil: 'networkidle' })
  await expect(page.getByTestId('onboarding-wizard-root')).toBeVisible()
  await expect(page.getByText('概览').first()).toBeVisible()
  await expect(page.getByText(/Heroku 官方 Node Getting Started/)).toBeVisible()
  await expect(page.getByText('GitHub').first()).toBeVisible()
  await page.screenshot({ path: path.join(OUT, '01-overview.png'), fullPage: true })

  await page.getByTestId('onboarding-next').click()
  await expect(page.getByText('Cursor').first()).toBeVisible()
  await page.screenshot({ path: path.join(OUT, '02-backend.png'), fullPage: true })

  await page.getByTestId('onboarding-next').click()
  await expect(page.getByTestId('onboarding-api-key')).toBeVisible()
  await page.screenshot({ path: path.join(OUT, '03-api-key.png'), fullPage: true })

  // without key should stay
  await page.getByTestId('onboarding-next').click()
  await expect(page.getByTestId('onboarding-api-key')).toBeVisible()

  await page.getByTestId('onboarding-api-key').fill('crsr_e2e_test_key')
  await page.getByTestId('onboarding-next').click()
  await expect(page.getByText('nodejs-getting-started')).toBeVisible()
  const gitPane = await page.locator('.flex.min-w-0.flex-1').innerText()
  expect(gitPane).not.toContain('approving-demo')
  expect(gitPane).toContain('heroku/nodejs-getting-started')
  await page.screenshot({ path: path.join(OUT, '04-git.png'), fullPage: true })

  await page.getByTestId('onboarding-next').click()
  await expect(page.getByText('生成配置')).toBeVisible()
  await page.screenshot({ path: path.join(OUT, '05-review.png'), fullPage: true })

  await page.getByTestId('onboarding-next').click()
  await expect(page.getByTestId('onboarding-success')).toBeVisible()
  await expect(page.getByText('VisualAgent')).toBeVisible()
  await expect(page.getByText('PreviewAgent')).toBeVisible()
  await expect(page.getByText('快速上手·轻量（published）')).toBeVisible()
  await expect(page.getByTestId('onboarding-start-run')).toBeVisible()
  await page.screenshot({ path: path.join(OUT, '06-success.png'), fullPage: true })
})
