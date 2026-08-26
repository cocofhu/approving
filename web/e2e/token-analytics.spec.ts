import { test, expect } from '@playwright/test'
import { routeApi } from './helpers/apiRoute'

const MOCK_STATS = {
  window: '30d',
  bucketWidth: 'day',
  timezone: 'UTC',
  empty: false,
  kpi: {
    total: 9000,
    deltaPct: 10,
    inputTokens: 5000,
    outputTokens: 3000,
    cacheReadTokens: 800,
    cacheWriteTokens: 200,
    workflowTotal: 7000,
    pmTotal: 2000,
    projectCount: 1,
    runCount: 3,
    modelCount: 1,
  },
  trend: [
    {
      bucket: '2026-07-01',
      total: 9000,
      workflowTotal: 7000,
      pmTotal: 2000,
      inputTokens: 5000,
      outputTokens: 3000,
      cacheReadTokens: 800,
      cacheWriteTokens: 200,
    },
  ],
  prevTrend: [{ bucket: '2026-06-01', total: 8000, workflowTotal: 6000, pmTotal: 2000, inputTokens: 4000, outputTokens: 2500, cacheReadTokens: 700, cacheWriteTokens: 180 }],
  composition: {
    total: 9000,
    inputTokens: 5000,
    outputTokens: 3000,
    cacheReadTokens: 800,
    cacheWriteTokens: 200,
  },
  projects: [{ projectId: 'p1', name: 'Demo', total: 9000, inputTokens: 5000, outputTokens: 3000, cacheReadTokens: 800, cacheWriteTokens: 200 }],
  modelRanking: [{ modelKey: 'm1', name: 'Model', total: 9000 }],
  nodeTypes: [{ name: 'agent', total: 9000 }],
  workflows: [{ name: 'wf', total: 9000, kind: 'workflow' }],
  heatmap: { rows: ['Model'], cols: ['Demo'], grid: [[9000]] },
  topRuns: [
    {
      runId: 'r1',
      title: 'Run',
      projectId: 'p1',
      projectName: 'Demo',
      workflowName: 'wf',
      modelKey: 'm1',
      modelName: 'Model',
      total: 9000,
    },
  ],
  projectTrends: [],
  modelTrends: [],
  filterOptions: {
    projects: [{ key: 'p1', name: 'Demo' }],
    models: [{ key: 'm1', name: 'Model' }],
  },
}

async function openStatsPage(page: import('@playwright/test').Page) {
  await routeApi(page, '**/api/stats/token**', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify(MOCK_STATS),
    })
  })
  await page.goto('/token-analytics.html')
  await expect(page.getByTestId('shell-main-dashboard')).toBeVisible({ timeout: 15_000 })
  await page.getByRole('link', { name: '统计', exact: true }).click()
  await expect(page.getByTestId('token-analytics-page')).toBeVisible({ timeout: 15_000 })
}

test.describe('Global token analytics', () => {
  test('sidebar stats entry navigates to /stats with chart sections', async ({ page }) => {
    await openStatsPage(page)
    await expect(page.getByTestId('token-analytics-lines')).toBeVisible({ timeout: 15_000 })
    await expect(page.getByTestId('token-analytics-pies')).toBeVisible()
    await expect(page.getByTestId('token-analytics-kpis')).toBeVisible()
    await expect(page.getByTestId('token-analytics-section-nav')).toHaveCount(0)
    await expect(page.getByText('用量统计')).toBeVisible()
  })

  test('tablet viewport has no section nav and shows main chart sections', async ({ page }) => {
    await page.setViewportSize({ width: 1024, height: 768 })
    await openStatsPage(page)
    await expect(page.getByTestId('token-analytics-section-nav-mobile')).toHaveCount(0)
    await expect(page.getByTestId('token-analytics-section-nav')).toHaveCount(0)
    await expect(page.getByTestId('token-analytics-lines')).toBeVisible()
  })

  test('line mode tabs switch visible labels', async ({ page }) => {
    await openStatsPage(page)
    await page.getByRole('button', { name: '按项目' }).click()
    await expect(page.getByRole('button', { name: '按项目' })).toHaveClass(/font-semibold/)
    await page.getByRole('button', { name: '按模型' }).click()
    await expect(page.getByRole('button', { name: '按模型' })).toHaveClass(/font-semibold/)
    await page.getByRole('button', { name: '总量（对比上一周期）' }).click()
    await expect(page.getByRole('button', { name: '总量（对比上一周期）' })).toHaveClass(/font-semibold/)
  })

  test('project table link navigates to project board tab', async ({ page }) => {
    await openStatsPage(page)
    await page.getByRole('button', { name: 'Demo' }).click()
    await expect(page.getByTestId('project-board-page')).toBeVisible({ timeout: 10_000 })
    await expect(page.getByText('board:p1')).toBeVisible()
  })

  test('top run link navigates to run detail', async ({ page }) => {
    await openStatsPage(page)
    await page.getByRole('button', { name: 'Run' }).click()
    await expect(page.getByTestId('run-detail-page')).toBeVisible({ timeout: 10_000 })
    await expect(page.getByText('run:r1')).toBeVisible()
  })
})
