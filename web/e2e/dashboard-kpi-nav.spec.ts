/**
 * E2E: Dashboard KPI → /runs single status navigation.
 * Harnesses:
 * - board.html (query exposure stub)
 * - dashboard-kpi-nav.html (DashboardView + RunListView / StatusFilter)
 */
import { expect, test, type Page } from '@playwright/test'

const KPI_CASES = [
  { status: 'running', testid: 'dashboard-kpi-running', count: '4', filterLabel: '运行中' },
  { status: 'waiting_human', testid: 'dashboard-kpi-waiting_human', count: '0', filterLabel: '等待人工' },
  { status: 'failed', testid: 'dashboard-kpi-failed', count: '1', filterLabel: '失败' },
  { status: 'completed', testid: 'dashboard-kpi-completed', count: '157', filterLabel: '已完成' },
] as const

function stubRun(id: string, status: string, title: string) {
  return {
    id,
    workflowId: 'wf-1',
    workflowName: 'KPI Demo',
    title,
    status,
    trigger: 'manual',
    startedAt: '2026-08-10T12:00:00Z',
    durationSec: 60,
    progress: status === 'completed' || status === 'failed' ? 1 : 0.4,
    currentNodeLabel: '节点',
    nodeRuns: {},
    artifacts: [],
  }
}

const ALL_RUNS = [
  stubRun('run-r1', 'running', '运行中-A'),
  stubRun('run-r2', 'running', '运行中-B'),
  stubRun('run-r3', 'running', '运行中-C'),
  stubRun('run-r4', 'running', '运行中-D'),
  stubRun('run-f1', 'failed', '失败-A'),
  stubRun('run-c1', 'completed', '已完成-A'),
]

async function mockApis(page: Page) {
  await page.route('**/api/stats/dashboard', async (route) => {
    if (route.request().method() === 'GET') {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          running: 4,
          waitingHuman: 0,
          failed: 1,
          completed: 157,
        }),
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
        body: JSON.stringify([{ id: 'wf-1', name: 'KPI Demo', status: 'published' }]),
      })
      return
    }
    await route.continue()
  })

  await page.route('**/api/projects**', async (route) => {
    if (route.request().method() === 'GET') {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ items: [], total: 0 }),
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
    const status = url.searchParams.get('status') || ''
    const items = status
      ? ALL_RUNS.filter((r) => status.split(',').includes(r.status))
      : ALL_RUNS
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        items,
        total: items.length,
        page: 1,
        pageSize: 20,
        hasMore: false,
      }),
    })
  })
}

test.describe('概览 KPI → /runs（board harness 路由契约）', () => {
  test('四卡映射、零值可点、不写 projectId', async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 800 })
    await mockApis(page)

    for (const kpi of KPI_CASES) {
      await page.goto('/board.html?start=dashboard&memory=0')
      await expect(page.getByTestId('dashboard-view')).toBeVisible({ timeout: 10_000 })

      const card = page.getByTestId(kpi.testid)
      await expect(card).toBeVisible()
      await expect(card).toBeEnabled()
      await expect(card).toContainText(kpi.count)
      await expect(card).toHaveAttribute('aria-label', new RegExp(kpi.count))
      await expect(card).not.toContainText(/查看|跳转|\/runs/)

      await card.click()
      await expect(page.getByTestId('runs-page')).toBeVisible({ timeout: 5_000 })
      await expect(page.getByTestId('runs-status-query')).toHaveText(kpi.status)
      await expect(page.getByTestId('runs-project-id-query')).toHaveText('(none)')
      await expect(page.getByTestId('runs-page')).toHaveAttribute('data-project-id', '')
    }
  })

  test('零值等待人工可用 Enter 激活', async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 800 })
    await mockApis(page)
    await page.goto('/board.html?start=dashboard&memory=0')
    await expect(page.getByTestId('dashboard-view')).toBeVisible({ timeout: 10_000 })

    const zero = page.getByTestId('dashboard-kpi-waiting_human')
    await expect(zero).toContainText('0')
    await zero.focus()
    await page.keyboard.press('Enter')
    await expect(page.getByTestId('runs-page')).toBeVisible({ timeout: 5_000 })
    await expect(page.getByTestId('runs-status-query')).toHaveText('waiting_human')
  })

  test('KPI 卡含 cursor/hover/focus-visible class', async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 800 })
    await mockApis(page)
    await page.goto('/board.html?start=dashboard&memory=0')
    await expect(page.getByTestId('dashboard-view')).toBeVisible({ timeout: 10_000 })
    const className = (await page.getByTestId('dashboard-kpi-failed').getAttribute('class')) || ''
    expect(className).toContain('cursor-pointer')
    expect(className).toMatch(/hover:border-line-strong/)
    expect(className).toMatch(/focus-visible:ring/)
  })
})

test.describe('概览 KPI → RunListView StatusFilter', () => {
  test('点击失败 KPI 后 StatusFilter 为单一 failed，列表过滤正确', async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 900 })
    await mockApis(page)
    await page.goto('/dashboard-kpi-nav.html')
    await expect(page.getByTestId('dashboard-view')).toBeVisible({ timeout: 10_000 })

    // 概览 KPI 数字与标签
    await expect(page.getByTestId('dashboard-kpi-running')).toContainText('4')
    await expect(page.getByTestId('dashboard-kpi-waiting_human')).toContainText('0')
    await expect(page.getByTestId('dashboard-kpi-failed')).toContainText('1')
    await expect(page.getByTestId('dashboard-kpi-completed')).toContainText('157')

    await page.getByTestId('dashboard-kpi-failed').click()
    await expect(page.getByRole('heading', { name: '运行' })).toBeVisible({ timeout: 10_000 })

    // StatusFilter 按钮展示单一状态「失败」（后随 count chip）
    await expect(page.getByRole('button', { name: /失败/ })).toBeVisible({ timeout: 5_000 })

    await expect(page.getByText('失败-A')).toBeVisible()
    await expect(page.getByText('运行中-A')).toHaveCount(0)
  })

  test('零值等待人工进入空列表且 StatusFilter=等待人工', async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 900 })
    await mockApis(page)
    await page.goto('/dashboard-kpi-nav.html')
    await expect(page.getByTestId('dashboard-view')).toBeVisible({ timeout: 10_000 })

    await page.getByTestId('dashboard-kpi-waiting_human').click()
    await expect(page.getByRole('heading', { name: '运行' })).toBeVisible({ timeout: 10_000 })
    await expect(page.getByRole('button', { name: /等待人工/ })).toBeVisible()
    // 空结果：无任一 stub run 标题
    await expect(page.getByText('运行中-A')).toHaveCount(0)
    await expect(page.getByText('失败-A')).toHaveCount(0)
  })

  test('Space 键盘激活已完成 KPI', async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 900 })
    await mockApis(page)
    await page.goto('/dashboard-kpi-nav.html')
    await expect(page.getByTestId('dashboard-view')).toBeVisible({ timeout: 10_000 })

    const completed = page.getByTestId('dashboard-kpi-completed')
    await completed.focus()
    await page.keyboard.press('Space')
    await expect(page.getByRole('heading', { name: '运行' })).toBeVisible({ timeout: 10_000 })
    await expect(page.getByRole('button', { name: /已完成/ })).toBeVisible()
  })
})
