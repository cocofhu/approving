import { test, expect } from '@playwright/test'
import path from 'node:path'

const MOCK_PROJECTS = [
  {
    id: 'p1',
    name: 'Approving',
    description: 'Approving Project',
    workflowCount: 3,
    totalTokens: 128400,
    createdAt: '2026-01-01T00:00:00Z',
    updatedAt: '2026-07-25T03:42:00Z',
    sandboxEnv: [],
    variables: [],
  },
  {
    id: 'p2',
    name: 'Ops Sync',
    description: '运维同步流水线',
    workflowCount: 5,
    totalTokens: 1_020_000,
    createdAt: '2026-01-01T00:00:00Z',
    updatedAt: '2026-07-24T10:06:00Z',
    sandboxEnv: [],
    variables: [],
  },
  {
    id: 'p3',
    name: 'Demo Sandbox',
    description: '演示沙箱',
    workflowCount: 1,
    totalTokens: 42,
    createdAt: '2026-01-01T00:00:00Z',
    updatedAt: '2026-07-22T00:00:00Z',
    sandboxEnv: [],
    variables: [],
  },
  {
    id: 'p4',
    name: 'Empty Draft',
    description: '尚未产生 Usage',
    workflowCount: 0,
    totalTokens: null,
    createdAt: '2026-01-01T00:00:00Z',
    updatedAt: '2026-07-18T00:00:00Z',
    sandboxEnv: [],
    variables: [],
  },
  {
    id: 'p5',
    name: 'Zero Probe',
    description: '已上报但合计为 0',
    workflowCount: 2,
    totalTokens: 0,
    createdAt: '2026-01-01T00:00:00Z',
    updatedAt: '2026-07-25T01:10:00Z',
    sandboxEnv: [],
    variables: [],
  },
]

const MOCK_DETAIL = {
  ...MOCK_PROJECTS[0],
  sandboxEnv: [],
  variables: [],
}

test.describe('项目 Token 总体消耗 UI', () => {
  test('全部项目列表 meta 展示 Token 合计与 —/0/K/M', async ({ page }, testInfo) => {
    await page.setViewportSize({ width: 1280, height: 900 })
    await page.route('**/api/projects', async (route) => {
      if (route.request().method() === 'GET') {
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify(MOCK_PROJECTS),
        })
        return
      }
      await route.continue()
    })

    await page.goto('/project-list.html?theme=light')
    await expect(page.getByRole('heading', { name: '项目' })).toBeVisible({ timeout: 15_000 })

    const approving = page.getByRole('button').filter({ hasText: 'Approving' }).first()
    await expect(approving).toContainText('Token')
    await expect(approving).toContainText('128.4K')
    await expect(approving).toContainText('个工作流')

    await expect(page.getByRole('button').filter({ hasText: 'Ops Sync' })).toContainText('1.02M')
    await expect(page.getByRole('button').filter({ hasText: 'Demo Sandbox' })).toContainText('Token')
    await expect(page.getByRole('button').filter({ hasText: 'Demo Sandbox' })).toContainText('42')
    await expect(page.getByRole('button').filter({ hasText: 'Empty Draft' })).toContainText('—')
    await expect(page.getByRole('button').filter({ hasText: 'Zero Probe' })).toContainText('Token')
    await expect(page.getByRole('button').filter({ hasText: 'Zero Probe' })).toContainText('0')

    const shot = path.join(testInfo.outputDir, 'project-list-token.png')
    await page.screenshot({ path: shot, fullPage: true })
    await testInfo.attach('project-list-token', { path: shot, contentType: 'image/png' })
  })

  test('项目详情标题区 Token 消耗统计块各 tab 常驻', async ({ page }, testInfo) => {
    await page.setViewportSize({ width: 1280, height: 900 })
    await page.route('**/api/projects/proj-1', async (route) => {
      if (route.request().method() === 'GET') {
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({ ...MOCK_DETAIL, id: 'proj-1' }),
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
          body: JSON.stringify([]),
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
          body: JSON.stringify([]),
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
    await page.route('**/api/projects/proj-1/cron-jobs', async (route) => {
      await route.fulfill({ status: 200, contentType: 'application/json', body: '[]' })
    })
    await page.route('**/api/projects/*/token-stats**', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          window: '30d',
          bucketWidth: 'day',
          timezone: 'UTC',
          empty: true,
          trend: [],
          composition: {
            inputTokens: 0,
            outputTokens: 0,
            cacheReadTokens: 0,
            cacheWriteTokens: 0,
            total: 0,
          },
          workflows: [],
        }),
      })
    })

    await page.goto('/project-detail.html?theme=light&tab=board')
    const stat = page.getByTestId('project-token-stat')
    await expect(stat).toBeVisible({ timeout: 15_000 })
    await expect(stat).toContainText('Token 消耗')
    await expect(stat).toContainText('128.4K')
    await expect(stat).toContainText('全部历史 · 工作流合计')

    const boardShot = path.join(testInfo.outputDir, 'project-detail-token-board.png')
    await page.screenshot({ path: boardShot, fullPage: true })
    await testInfo.attach('project-detail-token-board', { path: boardShot, contentType: 'image/png' })

    await page.getByTestId('project-tab-workflows').click()
    await expect(stat).toBeVisible()
    await expect(stat).toContainText('128.4K')

    await page.getByTestId('project-tab-meta').click()
    await expect(stat).toBeVisible()
    await expect(stat).toContainText('Token 消耗')

    const infoShot = path.join(testInfo.outputDir, 'project-detail-token-info.png')
    await page.screenshot({ path: infoShot, fullPage: true })
    await testInfo.attach('project-detail-token-info', { path: infoShot, contentType: 'image/png' })
  })

  test('无 Usage 时详情显示 —', async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 900 })
    await page.route('**/api/projects/proj-1', async (route) => {
      if (route.request().method() === 'GET') {
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({
            id: 'proj-1',
            name: 'Empty Draft',
            description: '尚未产生 Usage',
            workflowCount: 0,
            totalTokens: null,
            createdAt: '2026-01-01T00:00:00Z',
            updatedAt: '2026-07-18T00:00:00Z',
            sandboxEnv: [],
            variables: [],
          }),
        })
        return
      }
      await route.continue()
    })
    await page.route('**/api/workflows**', async (route) => {
      await route.fulfill({ status: 200, contentType: 'application/json', body: '[]' })
    })
    await page.route('**/api/runs**', async (route) => {
      await route.fulfill({ status: 200, contentType: 'application/json', body: '[]' })
    })
    await page.route('**/api/projects/proj-1/pm-leader', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ enabled: false }),
      })
    })
    await page.route('**/api/projects/proj-1/cron-jobs', async (route) => {
      await route.fulfill({ status: 200, contentType: 'application/json', body: '[]' })
    })
    await page.route('**/api/projects/*/token-stats**', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          window: '30d',
          bucketWidth: 'day',
          timezone: 'UTC',
          empty: true,
          trend: [],
          composition: {
            inputTokens: 0,
            outputTokens: 0,
            cacheReadTokens: 0,
            cacheWriteTokens: 0,
            total: 0,
          },
          workflows: [],
        }),
      })
    })

    await page.goto('/project-detail.html?theme=light&tab=board')
    const stat = page.getByTestId('project-token-stat')
    await expect(stat).toBeVisible({ timeout: 15_000 })
    await expect(stat).toContainText('—')
    await expect(stat).not.toContainText('0')
  })
})
