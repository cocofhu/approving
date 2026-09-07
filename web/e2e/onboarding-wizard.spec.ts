import { test, expect, type Page } from '@playwright/test'
import path from 'node:path'
import fs from 'node:fs'

const OUT = path.join('/tmp', 'onboarding-e2e-shots')

test.beforeAll(() => {
  fs.mkdirSync(OUT, { recursive: true })
})

async function mockOnboardingApi(page: Page) {
  await page.route('**/api/**', async (route) => {
    if (!new URL(route.request().url()).pathname.startsWith('/api/')) {
      await route.continue()
      return
    }
    const url = new URL(route.request().url())
    if (url.pathname.includes('/bootstrap-onboarding') && route.request().method() === 'POST') {
      const body = route.request().postDataJSON() as { apiKey?: string }
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
          agentIds: [
            '综合AI技术产品',
            '综合研发工程师',
            '综合测试工程师',
            '综合代码审查工程师',
            '综合运维工程师',
            '综合项目组组长',
          ],
          workflowId: 'wf-onboard-1',
          published: true,
          groupName: '综合项目组',
        }),
      })
      return
    }
    await route.fulfill({ status: 200, contentType: 'application/json', body: '[]' })
  })
}

async function walkWizardToSuccess(page: Page, opts: { generateLabel: string; apiKey: string }) {
  await page.getByTestId('onboarding-next').click()
  await page.getByTestId('onboarding-next').click()
  await page.getByTestId('onboarding-next').click()
  await page.getByTestId('onboarding-api-key').fill(opts.apiKey)
  await page.getByTestId('onboarding-next').click()
  await page.getByTestId('onboarding-git-user-name').fill('Ada Lovelace')
  await page.getByTestId('onboarding-git-user-email').fill('ada@example.com')
  await page.getByTestId('onboarding-skip').click()
  await expect(page.getByText(opts.generateLabel)).toBeVisible()
  await page.getByTestId('onboarding-next').click()
  await expect(page.getByTestId('onboarding-success')).toBeVisible()
}

test('空项目安装引导六步向导浏览器验收（zh）', async ({ page }) => {
  await mockOnboardingApi(page)

  await page.goto('/onboarding-wizard.html', { waitUntil: 'networkidle' })
  await expect(page.getByTestId('onboarding-wizard-root')).toBeVisible()
  await expect(page.getByTestId('onboarding-empty-desc')).toContainText('默认工作流')
  await expect(page.getByTestId('onboarding-language-zh-CN')).toHaveClass(/border-accent/)
  await expect(page.getByText('概览').first()).toBeVisible()
  await expect(page.getByText(/综合项目组/)).toBeVisible()
  await page.screenshot({ path: path.join(OUT, '01-language.png'), fullPage: true })

  await page.getByTestId('onboarding-next').click()
  await expect(page.getByText('综合项目组：产品 / 研发 / 测试 / 审查 / 运维 / 组长')).toBeVisible()
  await page.screenshot({ path: path.join(OUT, '02-overview.png'), fullPage: true })

  await page.getByTestId('onboarding-next').click()
  await expect(page.getByText('Cursor').first()).toBeVisible()
  await page.screenshot({ path: path.join(OUT, '03-backend.png'), fullPage: true })

  await page.getByTestId('onboarding-next').click()
  await expect(page.getByTestId('onboarding-api-key')).toBeVisible()
  await page.screenshot({ path: path.join(OUT, '04-api-key.png'), fullPage: true })

  await page.getByTestId('onboarding-next').click()
  await expect(page.getByTestId('onboarding-api-key')).toBeVisible()

  await page.getByTestId('onboarding-api-key').fill('crsr_e2e_test_key')
  await page.getByTestId('onboarding-next').click()
  await expect(page.getByTestId('onboarding-git-type-github_https')).toBeVisible()
  await expect(page.getByTestId('onboarding-repo-url')).toBeVisible()
  await page.getByTestId('onboarding-repo-url').fill('https://github.com/org/web.git')
  await expect(page.getByTestId('onboarding-repo-hint')).toContainText('/root/workspace/web/')
  await expect(page.getByTestId('onboarding-vnc-preview')).toHaveClass(/border-accent/)
  await expect(page.getByTestId('onboarding-browser-mcp')).toHaveClass(/border-accent/)
  await page.getByTestId('onboarding-git-user-name').fill('Ada Lovelace')
  await page.getByTestId('onboarding-git-user-email').fill('ada@example.com')
  const gitPane = await page.locator('.flex.min-w-0.flex-1').innerText()
  expect(gitPane).not.toContain('heroku/nodejs-getting-started')
  expect(gitPane).not.toContain('快速上手·轻量')
  await page.screenshot({ path: path.join(OUT, '05-git.png'), fullPage: true })

  await page.getByTestId('onboarding-next').click()
  await expect(page.getByText('生成配置')).toBeVisible()
  await expect(page.getByText('工作流 · 默认工作流')).toBeVisible()
  await expect(page.getByTestId('onboarding-review-repo')).toContainText('web')
  await page.screenshot({ path: path.join(OUT, '06-review.png'), fullPage: true })

  await page.getByTestId('onboarding-next').click()
  await expect(page.getByTestId('onboarding-success')).toBeVisible()
  await expect(page.getByText('综合研发工程师')).toBeVisible()
  await expect(page.getByText('默认工作流（已发布）')).toBeVisible()
  await expect(page.getByTestId('onboarding-success-repo')).toContainText('/root/workspace/web/')
  await expect(page.getByTestId('onboarding-start-run')).toHaveCount(0)
  await page.screenshot({ path: path.join(OUT, '07-success.png'), fullPage: true })
})

test('onboarding wizard English shell copy', async ({ page }) => {
  let bootstrapBody: { apiKey?: string; featureHint?: string; repos?: string } | null = null
  await page.route('**/api/**', async (route) => {
    if (!new URL(route.request().url()).pathname.startsWith('/api/')) {
      await route.continue()
      return
    }
    const url = new URL(route.request().url())
    if (url.pathname.includes('/bootstrap-onboarding') && route.request().method() === 'POST') {
      bootstrapBody = route.request().postDataJSON() as typeof bootstrapBody
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          agentIds: ['综合AI技术产品'],
          workflowId: 'wf-onboard-en',
          published: true,
          groupName: '综合项目组',
        }),
      })
      return
    }
    await route.fulfill({ status: 200, contentType: 'application/json', body: '[]' })
  })

  await page.goto('/onboarding-wizard.html', { waitUntil: 'networkidle' })
  await expect(page.getByTestId('onboarding-wizard-root')).toBeVisible()
  await page.getByTestId('onboarding-language-en').click()
  await expect(page.getByText('Language').first()).toBeVisible()

  await walkWizardToSuccess(page, { generateLabel: 'Generate setup', apiKey: 'crsr_e2e_en' })

  await expect(page.getByText('Default Workflow (published)')).toBeVisible()
  await expect(page.getByText('快速上手·轻量')).toHaveCount(0)
  expect(bootstrapBody?.featureHint).toBeUndefined()
  expect(bootstrapBody?.repos).toBeUndefined()
  await page.screenshot({ path: path.join(OUT, 'en-success.png'), fullPage: true })
})

test('onboarding English empty CTA / review chip', async ({ page }) => {
  await mockOnboardingApi(page)
  await page.goto('/onboarding-wizard.html', { waitUntil: 'networkidle' })
  await expect(page.getByTestId('onboarding-wizard-root')).toBeVisible()
  await page.getByTestId('onboarding-language-en').click()

  const emptyDesc = page.getByTestId('onboarding-empty-desc')
  await expect(emptyDesc).toContainText('Default Workflow')
  await expect(emptyDesc).not.toContainText('快速上手·轻量')

  await page.getByTestId('onboarding-next').click()
  await page.getByTestId('onboarding-next').click()
  await page.getByTestId('onboarding-next').click()
  await page.getByTestId('onboarding-api-key').fill('crsr_chip')
  await page.getByTestId('onboarding-next').click()
  await page.getByTestId('onboarding-git-user-name').fill('Ada Lovelace')
  await page.getByTestId('onboarding-git-user-email').fill('ada@example.com')
  await page.getByTestId('onboarding-skip').click()

  await expect(page.getByText('Workflow · Default Workflow')).toBeVisible()
  await expect(page.getByText('快速上手·轻量')).toHaveCount(0)
  const pane = await page.locator('[data-testid="onboarding-wizard-root"]').innerText()
  expect(pane).not.toContain('快速上手·轻量')
  expect(pane).toContain('Default Workflow')
  await page.screenshot({ path: path.join(OUT, 'en-review-chip.png'), fullPage: true })
})
