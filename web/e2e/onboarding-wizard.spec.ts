import { test, expect, type Page } from '@playwright/test'
import path from 'node:path'
import fs from 'node:fs'

const OUT = path.join('/tmp', 'onboarding-e2e-shots')

test.beforeAll(() => {
  fs.mkdirSync(OUT, { recursive: true })
})

async function mockOnboardingApi(page: Page) {
  await page.route('**/api/**', async (route) => {
    // Skip Vite module URLs like /@fs/.../src/lib/api/api.ts (pathname is not /api/...)
    if (!new URL(route.request().url()).pathname.startsWith('/api/')) {
      await route.continue()
      return
    }
    const url = new URL(route.request().url())
    if (url.pathname.includes('/bootstrap-onboarding') && route.request().method() === 'POST') {
      const body = route.request().postDataJSON() as {
        apiKey?: string
        repos?: string
        featureHint?: string
      }
      if (!body?.apiKey?.trim()) {
        await route.fulfill({
          status: 400,
          contentType: 'application/json',
          body: JSON.stringify({ error: 'apiKey required' }),
        })
        return
      }
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          agentIds: ['ClarifyAgent', 'VisualAgent', 'ImplementAgent', 'TestAgent', 'PreviewAgent'],
          workflowId: 'wf-onboard-1',
          repos: body.repos || 'demo|https://github.com/heroku/nodejs-getting-started.git|main',
          feature: body.featureHint || '把首页欢迎文案与主按钮文案改得更清晰友好',
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
}

async function walkWizardToSuccess(page: Page, opts: { generateLabel: string; apiKey: string }) {
  await page.getByTestId('onboarding-next').click() // overview -> acp
  await page.getByTestId('onboarding-next').click() // acp -> apiKey
  await page.getByTestId('onboarding-api-key').fill(opts.apiKey)
  await page.getByTestId('onboarding-next').click() // apiKey -> git
  await page.getByTestId('onboarding-next').click() // git -> review
  await expect(page.getByText(opts.generateLabel)).toBeVisible()
  await page.getByTestId('onboarding-next').click() // generate
  await expect(page.getByTestId('onboarding-success')).toBeVisible()
}

test('空项目上手引导五步向导浏览器验收（zh）', async ({ page }) => {
  await mockOnboardingApi(page)

  await page.goto('/onboarding-wizard.html', { waitUntil: 'networkidle' })
  await expect(page.getByTestId('onboarding-wizard-root')).toBeVisible()
  await expect(page.getByTestId('onboarding-empty-desc')).toContainText('快速上手·轻量')
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
  await expect(page.getByText('工作流 · 快速上手·轻量')).toBeVisible()
  await page.screenshot({ path: path.join(OUT, '05-review.png'), fullPage: true })

  await page.getByTestId('onboarding-next').click()
  await expect(page.getByTestId('onboarding-success')).toBeVisible()
  await expect(page.getByText('VisualAgent')).toBeVisible()
  await expect(page.getByText('PreviewAgent')).toBeVisible()
  await expect(page.getByText('快速上手·轻量（已发布）')).toBeVisible()
  const successRepo = page.getByTestId('onboarding-success-repo')
  await expect(successRepo).toBeVisible()
  await expect(successRepo).toContainText('heroku/nodejs-getting-started')
  await expect(successRepo).toContainText('main')
  await expect(successRepo).not.toContainText('demo|https://')
  await expect(page.getByTestId('onboarding-start-run')).toBeVisible()
  await page.screenshot({ path: path.join(OUT, '06-success.png'), fullPage: true })
})

test('onboarding wizard English shell copy (Demo gold)', async ({ page }) => {
  let bootstrapBody: { featureHint?: string } | null = null
  await page.route('**/api/**', async (route) => {
    if (!new URL(route.request().url()).pathname.startsWith('/api/')) {
      await route.continue()
      return
    }
    const url = new URL(route.request().url())
    if (url.pathname.includes('/bootstrap-onboarding') && route.request().method() === 'POST') {
      bootstrapBody = route.request().postDataJSON() as { featureHint?: string; apiKey?: string; repos?: string }
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          agentIds: ['ClarifyAgent', 'VisualAgent', 'ImplementAgent', 'TestAgent', 'PreviewAgent'],
          workflowId: 'wf-onboard-en',
          repos: bootstrapBody?.repos || 'demo|https://github.com/heroku/nodejs-getting-started.git|main',
          feature: bootstrapBody?.featureHint || '',
          published: true,
        }),
      })
      return
    }
    await route.fulfill({ status: 200, contentType: 'application/json', body: '[]' })
  })

  await page.goto('/onboarding-wizard.html?lang=en', { waitUntil: 'networkidle' })
  await expect(page.getByTestId('onboarding-wizard-root')).toBeVisible()
  await expect(page.getByText('Overview').first()).toBeVisible()

  await walkWizardToSuccess(page, { generateLabel: 'Generate setup', apiKey: 'crsr_e2e_en' })

  await expect(page.getByText('Quick Start · Light (published)')).toBeVisible()
  await expect(page.getByText('快速上手·轻量')).toHaveCount(0)
  // Review chip was visible before generate; re-open path is heavy — assert from prior review via regenerate
  // After success we already left review; assert chip was in DOM during walk by checking bootstrap featureHint
  expect(bootstrapBody?.featureHint).toBe(
    'Add a lightweight quick-start sample so the team can walk the clarify → gate → agent path.',
  )
  await page.screenshot({ path: path.join(OUT, 'en-success.png'), fullPage: true })
})

test('onboarding English empty CTA / review chip gold sample', async ({ page }) => {
  await mockOnboardingApi(page)
  await page.goto('/onboarding-wizard.html?lang=en', { waitUntil: 'networkidle' })
  await expect(page.getByTestId('onboarding-wizard-root')).toBeVisible()

  const emptyDesc = page.getByTestId('onboarding-empty-desc')
  await expect(emptyDesc).toHaveText('Publishes Quick Start · Light workflow and starts a sample run.')
  await expect(emptyDesc).not.toContainText('快速上手·轻量')

  await page.getByTestId('onboarding-next').click() // acp
  await page.getByTestId('onboarding-next').click() // apiKey
  await page.getByTestId('onboarding-api-key').fill('crsr_chip')
  await page.getByTestId('onboarding-next').click() // git
  await page.getByTestId('onboarding-next').click() // review

  await expect(page.getByText('Workflow · Quick Start · Light')).toBeVisible()
  await expect(page.getByText('快速上手·轻量')).toHaveCount(0)
  const pane = await page.locator('[data-testid="onboarding-wizard-root"]').innerText()
  expect(pane).not.toContain('快速上手·轻量')
  expect(pane).toContain('Quick Start · Light')
  await page.screenshot({ path: path.join(OUT, 'en-review-chip.png'), fullPage: true })
})
