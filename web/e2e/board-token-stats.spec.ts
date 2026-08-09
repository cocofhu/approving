import { test, expect } from '@playwright/test'
import path from 'node:path'
import { dismissOnboardingIfOpen, seedOnboardingDismissed } from './helpers/onboarding'

function stubRun(partial: {
  id: string
  status: string
  title?: string
}) {
  return {
    id: partial.id,
    workflowId: 'wf-1',
    workflowName: 'Demo Pipeline',
    title: partial.title,
    status: partial.status,
    trigger: 'manual',
    startedAt: '2026-07-18T12:00:00Z',
    durationSec: 120,
    progress: 40,
    currentNodeLabel: '实现',
    nodeRuns: {},
    artifacts: [],
  }
}

const STATS_30D = {
  window: '30d',
  bucketWidth: 'day',
  timezone: 'Asia/Shanghai',
  empty: false,
  trend: [
    {
      bucket: '2026-07-24',
      total: 1200,
      workflowTotal: 900,
      pmTotal: 300,
      inputTokens: 500,
      outputTokens: 400,
      cacheReadTokens: 200,
      cacheWriteTokens: 100,
    },
    {
      bucket: '2026-07-25',
      total: 800,
      workflowTotal: 600,
      pmTotal: 200,
      inputTokens: 300,
      outputTokens: 250,
      cacheReadTokens: 150,
      cacheWriteTokens: 100,
    },
  ],
  composition: {
    inputTokens: 800,
    outputTokens: 650,
    cacheReadTokens: 350,
    cacheWriteTokens: 200,
    total: 2000,
  },
  workflows: [
    // Fixed order: Top workflows (desc) → PM → other (no mid-insert PM / 12PM34)
    { workflowId: 'wf-a', name: 'approve-main', total: 1200, kind: 'workflow' },
    { workflowId: 'wf-b', name: 'doc-review', total: 300, kind: 'workflow' },
    { name: 'PM', total: 500, kind: 'pm' },
    { name: '其他', total: 200, other: true, kind: 'other' },
  ],
}

const STATS_7D = {
  ...STATS_30D,
  window: '7d',
  composition: { ...STATS_30D.composition, total: 900 },
  workflows: [
    { workflowId: 'wf-a', name: 'approve-main', total: 600 },
    { workflowId: 'wf-b', name: 'doc-review', total: 300 },
  ],
}

const STATS_ALL = {
  window: 'all',
  bucketWidth: 'week',
  timezone: 'Asia/Shanghai',
  empty: false,
  trend: [
    {
      bucket: '2026-W28',
      total: 5000,
      inputTokens: 2000,
      outputTokens: 1800,
      cacheReadTokens: 800,
      cacheWriteTokens: 400,
    },
  ],
  composition: {
    inputTokens: 2000,
    outputTokens: 1800,
    cacheReadTokens: 800,
    cacheWriteTokens: 400,
    total: 5000,
  },
  workflows: [{ workflowId: 'wf-a', name: 'approve-main', total: 5000 }],
}

const EMPTY_STATS = {
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
}

async function mockBoardShell(page: import('@playwright/test').Page) {
  await page.route('**/api/stats/dashboard', async (route) => {
    if (route.request().method() === 'GET') {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ running: 1, waitingHuman: 0, failed: 0, completed: 1 }),
      })
      return
    }
    await route.continue()
  })

  await page.route('**/api/runs**', async (route) => {
    if (route.request().method() !== 'GET') {
      await route.continue()
      return
    }
    const url = new URL(route.request().url())
    if (!url.searchParams.get('projectId')) {
      await route.fulfill({
        status: 400,
        contentType: 'application/json',
        body: JSON.stringify({ error: 'projectId required' }),
      })
      return
    }
    const status = url.searchParams.get('status') || ''
    const items =
      status === 'running'
        ? [stubRun({ id: 'run-1', status: 'running', title: '看板运行中' })]
        : status === 'completed'
          ? [stubRun({ id: 'run-2', status: 'completed', title: '看板已完成' })]
          : []
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ items, total: items.length, page: 1, pageSize: 20, hasMore: false }),
    })
  })
}

test.describe('看板 Token 统计图', () => {
  test('有数据：默认近 30 天三图布局、切窗清空旧数据、失败重试', async ({ page }, testInfo) => {
    await page.setViewportSize({ width: 1280, height: 900 })
    await mockBoardShell(page)

    let failNext = false
    const seenWindows: string[] = []
    await page.route('**/api/projects/*/token-stats**', async (route) => {
      if (route.request().method() !== 'GET') {
        await route.continue()
        return
      }
      const url = new URL(route.request().url())
      const w = url.searchParams.get('window') || '30d'
      seenWindows.push(w)
      if (failNext) {
        failNext = false
        await route.fulfill({ status: 503, contentType: 'application/json', body: JSON.stringify({ error: 'timeout' }) })
        return
      }
      if (w === '7d') {
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify(STATS_7D),
        })
        return
      }
      if (w === 'all') {
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify(STATS_ALL),
        })
        return
      }
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify(STATS_30D),
      })
    })

    await page.goto('/board.html?start=project-board&memory=1&projectId=proj-1')
    await expect(page.getByTestId('board-view')).toBeVisible({ timeout: 15_000 })
    const panel = page.getByTestId('token-stats-panel')
    await expect(panel).toBeVisible()
    await expect(page.getByTestId('token-stats-window-30d')).toHaveAttribute('aria-selected', 'true')
    await expect(page.getByTestId('token-stats-window-badge')).toContainText('近 30 天')
    await expect(page.getByTestId('token-stats-charts')).toBeVisible({ timeout: 10_000 })
    await expect(page.getByTestId('token-stats-trend-card')).toBeVisible()
    await expect(page.getByTestId('token-stats-comp-card')).toBeVisible()
    await expect(page.getByTestId('token-stats-rank-card')).toBeVisible()
    await expect(page.getByTestId('token-trend-wrap')).toBeVisible()
    await expect(page.getByTestId('token-trend-chart')).toBeVisible()
    await expect(page.getByTestId('token-trend-chart').locator('canvas')).toBeVisible()
    // Regression: no non-uniform SVG stretch path for the trend chart
    await expect(page.getByTestId('token-trend-svg')).toHaveCount(0)
    await expect(page.locator('[data-testid="token-trend-wrap"] svg[preserveAspectRatio="none"]')).toHaveCount(0)
    await expect(page.getByTestId('token-donut-svg')).toBeVisible()
    await expect(page.getByTestId('token-donut-legend')).toContainText('input')
    await expect(panel).toContainText('approve-main')
    await expect(panel).toContainText('项目管理')
    await expect(panel).toContainText('其他')
    await expect(page.getByTestId('token-trend-legend')).toContainText('workflow')
    await expect(page.getByTestId('token-trend-legend')).toContainText('pm')
    await expect(panel).toContainText('消耗排行')
    await expect(panel).toContainText('按来源堆叠')
    await expect(page.getByTestId('token-rank-list').locator('[data-kind="pm"]')).toBeVisible()

    // Continuous numeric badges: Top→PM→其他 → 1,2,3,· (no 12PM34 / no "PM" text badge)
    const rankList = page.getByTestId('token-rank-list')
    const kinds = await rankList.locator('li').evaluateAll((lis) =>
      lis.map((li) => li.getAttribute('data-kind')),
    )
    expect(kinds).toEqual(['workflow', 'workflow', 'pm', 'other'])
    const badges = await rankList.locator('li').evaluateAll((lis) =>
      lis.map((li) => {
        const badge = li.querySelector('span')
        return (badge?.textContent || '').trim()
      }),
    )
    expect(badges).toEqual(['1', '2', '3', '·'])

    // Panel sits above Run columns
    const panelBox = await panel.boundingBox()
    const colBox = await page.getByTestId('run-board-column').first().boundingBox()
    expect(panelBox && colBox).toBeTruthy()
    expect(panelBox!.y).toBeLessThan(colBox!.y)

    // No collapse control
    await expect(panel.getByRole('button', { name: /折叠|收起|展开/ })).toHaveCount(0)
    await expect(page.getByTestId('token-stats-retry')).toHaveCount(0)

    const readyShot = path.join(testInfo.outputDir, 'token-stats-ready.png')
    await page.screenshot({ path: readyShot, fullPage: true })
    await testInfo.attach('token-stats-ready', { path: readyShot, contentType: 'image/png' })

    // Switch window: loading clears charts
    await page.route('**/api/projects/*/token-stats**', async (route) => {
      const url = new URL(route.request().url())
      const w = url.searchParams.get('window') || '30d'
      seenWindows.push(`delayed-${w}`)
      await new Promise((r) => setTimeout(r, 400))
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify(w === '7d' ? STATS_7D : STATS_30D),
      })
    })
    await page.getByTestId('token-stats-window-7d').click()
    await expect(page.getByTestId('token-stats-loading')).toBeVisible()
    await expect(page.getByTestId('token-stats-charts')).toHaveCount(0)
    await expect(page.getByTestId('token-stats-charts')).toBeVisible({ timeout: 10_000 })
    await expect(page.getByTestId('token-stats-window-badge')).toContainText('近 7 天')
    await expect(page.getByTestId('token-stats-window-7d')).toHaveAttribute('aria-selected', 'true')
    // After window switch: Chart.js canvas still hosts trend; no SVG none stretch
    await expect(page.getByTestId('token-trend-chart').locator('canvas')).toBeVisible()
    await expect(page.locator('[data-testid="token-trend-wrap"] svg[preserveAspectRatio="none"]')).toHaveCount(0)
    const wrapBox = await page.getByTestId('token-trend-wrap').boundingBox()
    expect(wrapBox).toBeTruthy()
    expect(wrapBox!.height).toBeGreaterThanOrEqual(180)
    expect(wrapBox!.height).toBeLessThanOrEqual(240)

    // Failure + retry
    failNext = true
    await page.route('**/api/projects/*/token-stats**', async (route) => {
      const url = new URL(route.request().url())
      const w = url.searchParams.get('window') || '30d'
      if (failNext) {
        failNext = false
        await route.fulfill({ status: 503, contentType: 'application/json', body: JSON.stringify({ error: 'timeout' }) })
        return
      }
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify(w === 'all' ? STATS_ALL : STATS_30D),
      })
    })
    await page.getByTestId('token-stats-window-all').click()
    await expect(page.getByTestId('token-stats-error')).toBeVisible({ timeout: 10_000 })
    await expect(page.getByTestId('token-stats-retry')).toBeVisible()
    await expect(page.getByTestId('token-stats-charts')).toHaveCount(0)

    const errShot = path.join(testInfo.outputDir, 'token-stats-error.png')
    await page.screenshot({ path: errShot, fullPage: true })
    await testInfo.attach('token-stats-error', { path: errShot, contentType: 'image/png' })

    await page.getByTestId('token-stats-retry').click()
    await expect(page.getByTestId('token-stats-charts')).toBeVisible({ timeout: 10_000 })
    await expect(page.getByTestId('token-stats-trend-card')).toContainText('按周')
  })

  test('空数据：整区空状态且不画伪造全 0 图', async ({ page }, testInfo) => {
    await page.setViewportSize({ width: 1280, height: 900 })
    await mockBoardShell(page)
    await page.route('**/api/projects/*/token-stats**', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify(EMPTY_STATS),
      })
    })

    await page.goto('/board.html?start=project-board&memory=1&projectId=proj-1')
    await expect(page.getByTestId('token-stats-panel')).toBeVisible({ timeout: 15_000 })
    await expect(page.getByTestId('token-stats-empty')).toBeVisible()
    await expect(page.getByTestId('token-stats-charts')).toHaveCount(0)
    await expect(page.getByTestId('token-trend-chart')).toHaveCount(0)
    await expect(page.getByTestId('token-trend-svg')).toHaveCount(0)
    await expect(page.getByTestId('token-donut-svg')).toHaveCount(0)
    await expect(page.getByTestId('token-stats-empty')).toContainText('未上报不会显示为 0')

    const emptyShot = path.join(testInfo.outputDir, 'token-stats-empty.png')
    await page.screenshot({ path: emptyShot, fullPage: true })
    await testInfo.attach('token-stats-empty', { path: emptyShot, contentType: 'image/png' })
  })

  test('窄屏下方两图可堆叠且统计区仍在 Run 列上方', async ({ page }, testInfo) => {
    await page.setViewportSize({ width: 390, height: 844 })
    await mockBoardShell(page)
    await page.route('**/api/projects/*/token-stats**', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify(STATS_30D),
      })
    })

    await page.goto('/board.html?start=project-board&memory=1&projectId=proj-1')
    await expect(page.getByTestId('token-stats-charts')).toBeVisible({ timeout: 15_000 })
    const comp = await page.getByTestId('token-stats-comp-card').boundingBox()
    const rank = await page.getByTestId('token-stats-rank-card').boundingBox()
    expect(comp && rank).toBeTruthy()
    expect(rank!.y).toBeGreaterThan(comp!.y + 40)

    const narrowShot = path.join(testInfo.outputDir, 'token-stats-narrow.png')
    await page.screenshot({ path: narrowShot, fullPage: true })
    await testInfo.attach('token-stats-narrow', { path: narrowShot, contentType: 'image/png' })
  })

  test('项目详情看板 Tab 展示 Token 统计区且页头 totalTokens 不变', async ({ page }, testInfo) => {
    await page.setViewportSize({ width: 1280, height: 900 })
    // Empty workflows stub would otherwise auto-open onboarding and intercept clicks.
    await seedOnboardingDismissed(page, 'proj-1')
    await page.route('**/api/projects/proj-1', async (route) => {
      if (route.request().method() === 'GET') {
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({
            id: 'proj-1',
            name: 'Approving',
            description: 'Approving Project',
            workflowCount: 3,
            totalTokens: 128400,
            workflowTokens: 100000,
            pmTokens: 28400,
            createdAt: '2026-01-01T00:00:00Z',
            updatedAt: '2026-07-25T03:42:00Z',
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
    // Non-empty agents also keep shouldAutoOpenOnboarding false if dismiss seed fails.
    await page.route('**/api/agents**', async (route) => {
      if (route.request().method() === 'GET') {
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify([{ id: 'agent-stub', name: 'StubAgent', projectId: 'proj-1' }]),
        })
        return
      }
      await route.continue()
    })
    await page.route('**/api/runs**', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ items: [], total: 0, page: 1, pageSize: 20, hasMore: false }),
      })
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
      const url = new URL(route.request().url())
      const w = url.searchParams.get('window') || '30d'
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify(w === 'all' ? STATS_ALL : STATS_30D),
      })
    })

    await page.goto('/project-detail.html?theme=light&tab=board')
    await dismissOnboardingIfOpen(page)
    const headerStat = page.getByTestId('project-token-stat')
    await expect(headerStat).toBeVisible({ timeout: 15_000 })
    await expect(headerStat).toContainText('128.4K')
    await expect(headerStat).not.toContainText('全部历史 · 含工作流与项目管理（上线后）')

    await expect(page.getByTestId('token-stats-panel')).toBeVisible()
    await expect(page.getByTestId('token-stats-charts')).toBeVisible()

    await page.getByTestId('token-stats-window-all').click()
    await expect(page.getByTestId('token-stats-window-badge')).toContainText('全部历史')
    await expect(headerStat).toContainText('128.4K')
    await expect(headerStat).not.toContainText('全部历史 · 含工作流与项目管理（上线后）')

    await headerStat.hover()
    const tip = page.getByTestId('token-detail-tip')
    await expect(tip).toBeVisible()
    await expect(tip.getByTestId('token-detail-tip-breakdown')).toContainText('工作流')
    await expect(tip.getByTestId('token-detail-tip-breakdown')).toContainText('项目管理')
    await expect(tip.getByTestId('token-detail-tip-breakdown')).toContainText('合计')

    const detailShot = path.join(testInfo.outputDir, 'project-detail-token-stats.png')
    await page.screenshot({ path: detailShot, fullPage: true })
    await testInfo.attach('project-detail-token-stats', { path: detailShot, contentType: 'image/png' })
  })
})
