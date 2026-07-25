import { test, expect } from '@playwright/test'

function stubRun(partial: {
  id: string
  status: string
  title?: string
  workflowName?: string
  startedAt?: string
  progress?: number
  currentNodeLabel?: string
  durationSec?: number
}) {
  return {
    id: partial.id,
    workflowId: 'wf-1',
    workflowName: partial.workflowName || 'Demo Pipeline',
    title: partial.title,
    status: partial.status,
    trigger: 'manual',
    startedAt: partial.startedAt || '2026-07-18T12:00:00Z',
    durationSec: partial.durationSec ?? 120,
    progress: partial.progress ?? 40,
    currentNodeLabel: partial.currentNodeLabel || '实现',
    nodeRuns: {},
    artifacts: [],
  }
}

const MOCK_RUNS = {
  running: [stubRun({ id: 'run-running-1', status: 'running', title: '看板需求-运行中' })],
  waiting_human: [stubRun({ id: 'run-waiting-1', status: 'waiting_human', title: '看板需求-等待人工', progress: 70 })],
  completed: [stubRun({ id: 'run-done-1', status: 'completed', title: '看板需求-已完成', progress: 100, durationSec: 600 })],
  failed: [stubRun({ id: 'run-fail-1', status: 'failed', title: '看板需求-失败', progress: 55 })],
  queued: [],
  cancelled: [],
  'running,waiting_human': [
    stubRun({ id: 'run-waiting-1', status: 'waiting_human', title: '看板需求-等待人工', startedAt: '2026-07-18T14:00:00Z' }),
    stubRun({ id: 'run-running-1', status: 'running', title: '看板需求-运行中', startedAt: '2026-07-18T10:00:00Z' }),
  ],
}

async function mockBoardApis(page: import('@playwright/test').Page) {
  await page.route('**/api/stats/dashboard', async (route) => {
    if (route.request().method() === 'GET') {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          running: 1,
          waitingHuman: 1,
          failed: 1,
          completed: 1,
        }),
      })
      return
    }
    await route.continue()
  })

  await page.route('**/api/projects/*/token-stats**', async (route) => {
    if (route.request().method() === 'GET') {
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
    // Fail-safe: unfiltered global list must not be used by project board.
    if (!url.searchParams.get('projectId')) {
      await route.fulfill({
        status: 400,
        contentType: 'application/json',
        body: JSON.stringify({ error: 'projectId required in e2e mock' }),
      })
      return
    }
    const status = url.searchParams.get('status') || ''
    const items = (MOCK_RUNS as Record<string, unknown[]>)[status] || []
    const pageSize = Number(url.searchParams.get('pageSize') || items.length || 20)
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        items,
        total: items.length,
        page: 1,
        pageSize,
        hasMore: false,
      }),
    })
  })
}

async function gotoBoardHarness(
  page: import('@playwright/test').Page,
  opts: {
    width: number
    height?: number
    start?: 'dashboard' | 'board' | 'project-board'
    memory?: '0' | '1'
    projectId?: string
  } = { width: 1280 },
) {
  await page.setViewportSize({ width: opts.width, height: opts.height ?? 800 })
  await mockBoardApis(page)
  const qs = new URLSearchParams()
  if (opts.start) qs.set('start', opts.start)
  if (opts.memory) qs.set('memory', opts.memory)
  if (opts.projectId) qs.set('projectId', opts.projectId)
  const q = qs.toString()
  await page.goto(`/board.html${q ? `?${q}` : ''}`)
}

test.describe('需求进度看板（项目级）', () => {
  test('Dashboard 有项目记忆：迷你看板 + 完整看板进入项目级', async ({ page }) => {
    await gotoBoardHarness(page, { width: 1280, start: 'dashboard', memory: '1', projectId: 'proj-1' })
    await expect(page.getByTestId('dashboard-view')).toBeVisible({ timeout: 10_000 })
    await expect(page.getByRole('heading', { name: '概览' })).toBeVisible()
    await expect(page.getByText('需求进度看板')).toBeVisible()
    await expect(page.getByTestId('dashboard-board-empty')).toHaveCount(0)
    await expect(page.getByTestId('run-board-column')).toHaveCount(2)
    await expect(page.getByText('看板需求-运行中')).toBeVisible()
    await expect(page.getByText('看板需求-已完成')).toBeVisible()

    await page.getByTestId('dashboard-view-full-board').click()
    await expect(page.getByTestId('board-view')).toBeVisible({ timeout: 10_000 })
    await expect(page.getByTestId('run-board-column')).toHaveCount(3)
  })

  test('Dashboard 无项目记忆：空态引导至项目列表', async ({ page }) => {
    await gotoBoardHarness(page, { width: 1280, start: 'dashboard', memory: '0' })
    await expect(page.getByTestId('dashboard-view')).toBeVisible({ timeout: 10_000 })
    await expect(page.getByTestId('dashboard-board-empty')).toBeVisible()
    await expect(page.getByTestId('dashboard-select-project')).toBeVisible()
    await expect(page.getByTestId('run-board-column')).toHaveCount(0)
    await page.getByTestId('dashboard-select-project').click()
    await expect(page.getByTestId('projects-page')).toBeVisible({ timeout: 5_000 })
  })

  test('/board 有记忆重定向到项目看板', async ({ page }) => {
    await gotoBoardHarness(page, { width: 1280, start: 'board', memory: '1', projectId: 'proj-1' })
    await expect(page.getByTestId('board-view')).toBeVisible({ timeout: 10_000 })
    await expect(page.getByTestId('projects-page')).toHaveCount(0)
    await expect(page.getByTestId('run-board-column')).toHaveCount(3)
  })

  test('/board 无记忆重定向到项目列表', async ({ page }) => {
    await gotoBoardHarness(page, { width: 1280, start: 'board', memory: '0' })
    await expect(page.getByTestId('projects-page')).toBeVisible({ timeout: 10_000 })
    await expect(page.getByTestId('board-view')).toHaveCount(0)
  })

  test('项目看板三主列、侧滑预览关闭后不离页', async ({ page }) => {
    await gotoBoardHarness(page, { width: 1280, start: 'project-board', memory: '1', projectId: 'proj-1' })
    await expect(page.getByTestId('board-view')).toBeVisible({ timeout: 10_000 })
    const cols = page.getByTestId('run-board-column')
    await expect(cols).toHaveCount(3)
    await expect(cols.nth(0).locator('span').filter({ hasText: /^运行中$/ })).toBeVisible()
    await expect(cols.nth(1).locator('span').filter({ hasText: /^等待人工$/ })).toBeVisible()
    await expect(cols.nth(2).locator('span').filter({ hasText: /^已完成$/ })).toBeVisible()
    await expect(page.getByTestId('board-extra-columns')).toHaveCount(0)

    await page.getByText('看板需求-运行中').click()
    await expect(page.getByText('运行摘要')).toBeVisible({ timeout: 5_000 })
    await expect(page.getByRole('button', { name: '进入运行详情' })).toBeVisible()
    await page.getByRole('button', { name: '继续浏览' }).click()
    await expect(page.getByText('运行摘要')).toHaveCount(0)
    await expect(page.getByTestId('board-view')).toBeVisible()
    await expect(page.getByTestId('run-detail-page')).toHaveCount(0)
  })

  test('额外列筛选失败状态，查看更多跳转运行列表', async ({ page }) => {
    await gotoBoardHarness(page, { width: 1280, start: 'project-board', memory: '1', projectId: 'proj-1' })
    await expect(page.getByTestId('board-view')).toBeVisible({ timeout: 10_000 })

    await page.getByTestId('board-filter-failed').click()
    await expect(page.getByTestId('board-extra-columns')).toBeVisible()
    await expect(page.getByText('看板需求-失败')).toBeVisible()
    await expect(page.getByText('看板需求-已完成')).toBeVisible()

    await page.getByTestId('board-view-more-completed').click()
    await expect(page.getByTestId('runs-page')).toBeVisible({ timeout: 5_000 })
  })

  test('窄屏主列纵向堆叠仍可读', async ({ page }) => {
    await gotoBoardHarness(page, {
      width: 390,
      height: 844,
      start: 'project-board',
      memory: '1',
      projectId: 'proj-1',
    })
    await expect(page.getByTestId('board-view')).toBeVisible({ timeout: 10_000 })
    await expect(page.getByText('看板需求-运行中')).toBeVisible()
    const cols = page.getByTestId('run-board-column')
    await expect(cols).toHaveCount(3)
    const first = await cols.nth(0).boundingBox()
    const second = await cols.nth(1).boundingBox()
    expect(first && second).toBeTruthy()
    expect(second!.y).toBeGreaterThan(first!.y)
  })

  test('已完成列可独立滚动且分页策略不变', async ({ page }) => {
    const pageSizeByStatus: Record<string, number> = {}
    await page.setViewportSize({ width: 1280, height: 800 })

    await page.route('**/api/stats/dashboard', async (route) => {
      if (route.request().method() === 'GET') {
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({ running: 20, waitingHuman: 1, failed: 0, completed: 24 }),
        })
        return
      }
      await route.continue()
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
          body: JSON.stringify({ error: 'projectId required in e2e mock' }),
        })
        return
      }
      const status = url.searchParams.get('status') || ''
      const pageSize = Number(url.searchParams.get('pageSize') || 20)
      pageSizeByStatus[status] = pageSize

      let items: ReturnType<typeof stubRun>[] = []
      if (status === 'completed') {
        items = Array.from({ length: Math.min(20, pageSize) }, (_, i) =>
          stubRun({
            id: `run-done-${i}`,
            status: 'completed',
            title: `看板已完成-${i}`,
            progress: 100,
            durationSec: 600,
          }),
        )
      } else if (status === 'running') {
        items = Array.from({ length: Math.min(20, pageSize) }, (_, i) =>
          stubRun({
            id: `run-running-${i}`,
            status: 'running',
            title: `看板运行中-${i}`,
            progress: 40,
          }),
        )
      } else if (status === 'waiting_human') {
        items = [
          stubRun({
            id: 'run-waiting-1',
            status: 'waiting_human',
            title: '看板需求-等待人工',
            progress: 70,
          }),
        ]
      }

      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          items,
          total: status === 'completed' ? 24 : items.length,
          page: 1,
          pageSize,
          hasMore: status === 'completed',
        }),
      })
    })

    await page.goto('/board.html?start=project-board&memory=1&projectId=proj-1')
    await expect(page.getByTestId('board-view')).toBeVisible({ timeout: 10_000 })
    await expect(page.getByTestId('run-board-column-body')).toHaveCount(3)

    expect(pageSizeByStatus.completed).toBe(20)
    expect(pageSizeByStatus.running).toBe(100)

    const completedCol = page.getByTestId('run-board-column').nth(2)
    const completedBody = completedCol.getByTestId('run-board-column-body')
    await expect(completedBody).toBeVisible()

    const overflow = await completedBody.evaluate((el) => ({
      scrollHeight: el.scrollHeight,
      clientHeight: el.clientHeight,
      overflowY: getComputedStyle(el).overflowY,
      maxHeight: getComputedStyle(el).maxHeight,
    }))
    expect(overflow.overflowY).toBe('auto')
    expect(overflow.scrollHeight).toBeGreaterThan(overflow.clientHeight)
    expect(overflow.maxHeight).toMatch(/px|vh/)

    const runningBody = page.getByTestId('run-board-column').nth(0).getByTestId('run-board-column-body')
    await completedBody.evaluate((el) => {
      el.scrollTop = 120
    })
    await runningBody.evaluate((el) => {
      el.scrollTop = 40
    })
    const tops = await page.evaluate(() => {
      const bodies = Array.from(document.querySelectorAll('[data-testid="run-board-column-body"]'))
      return bodies.map((el) => (el as HTMLElement).scrollTop)
    })
    expect(tops[0]).toBe(40)
    expect(tops[2]).toBe(120)

    const viewMore = page.getByTestId('board-view-more-completed')
    await expect(viewMore).toBeVisible()
    await viewMore.click()
    await expect(page.getByTestId('runs-page')).toBeVisible({ timeout: 5_000 })
  })
})
