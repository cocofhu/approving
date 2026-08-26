import { test, expect } from '@playwright/test'
import { loginAsAdmin } from './helpers/auth'

test.describe('Global token analytics', () => {
  test.beforeEach(async ({ page }) => {
    await loginAsAdmin(page)
  })

  test('sidebar stats entry navigates to /stats with chart sections', async ({ page }) => {
    await page.route('**/api/stats/token**', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
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
          trend: [{ bucket: '2026-07-01', total: 9000, workflowTotal: 7000, pmTotal: 2000, inputTokens: 5000, outputTokens: 3000, cacheReadTokens: 800, cacheWriteTokens: 200 }],
          prevTrend: [],
          composition: { total: 9000, inputTokens: 5000, outputTokens: 3000, cacheReadTokens: 800, cacheWriteTokens: 200 },
          projects: [{ projectId: 'p1', name: 'Demo', total: 9000, inputTokens: 5000, outputTokens: 3000 }],
          modelRanking: [{ modelKey: 'm1', name: 'Model', total: 9000 }],
          nodeTypes: [{ name: 'agent', total: 9000 }],
          workflows: [{ name: 'wf', total: 9000, kind: 'workflow' }],
          heatmap: { rows: ['Model'], cols: ['Demo'], grid: [[9000]] },
          topRuns: [{ runId: 'r1', title: 'Run', projectId: 'p1', projectName: 'Demo', workflowName: 'wf', modelKey: 'm1', modelName: 'Model', total: 9000 }],
          projectTrends: [],
          modelTrends: [],
          filterOptions: { projects: [{ key: 'p1', name: 'Demo' }], models: [{ key: 'm1', name: 'Model' }] },
        }),
      })
    })

    await page.goto('/dashboard')
    await page.getByRole('link', { name: /统计|Statistics/i }).click()
    await expect(page).toHaveURL(/\/stats/)
    await expect(page.getByTestId('token-analytics-lines')).toBeVisible()
    await expect(page.getByTestId('token-analytics-pies')).toBeVisible()
    await expect(page.getByTestId('token-analytics-section-nav')).toBeVisible()
  })
})
