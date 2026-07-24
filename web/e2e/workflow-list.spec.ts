import { test, expect } from '@playwright/test'

const MOCK_WORKFLOWS = [
  {
    id: 'wf-1',
    name: 'Demo Workflow',
    description: 'A demo description that may be long',
    status: 'published',
    version: 3,
    updatedAt: '2026-01-01T00:00:00Z',
    lastRunAt: '2026-01-02T00:00:00Z',
    needsRepo: false,
    nodes: [],
    edges: [],
  },
  {
    id: 'wf-2',
    name: 'Draft Workflow',
    description: '',
    status: 'draft',
    version: 1,
    updatedAt: '2026-01-01T00:00:00Z',
    needsRepo: true,
    nodes: [],
    edges: [],
  },
]

async function gotoWorkflowList(
  page: import('@playwright/test').Page,
  opts: { width: number; height?: number; theme?: 'light' } = { width: 390 },
) {
  await page.setViewportSize({ width: opts.width, height: opts.height ?? 844 })
  await page.route('**/api/workflows', async (route) => {
    if (route.request().method() === 'GET') {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify(MOCK_WORKFLOWS),
      })
      return
    }
    await route.continue()
  })
  const qs = opts.theme === 'light' ? '?theme=light' : ''
  await page.goto(`/workflow-list.html${qs}`)
  await expect(page.getByRole('heading', { name: '工作流' })).toBeVisible({ timeout: 10_000 })
}

test.describe('WorkflowListView 移动端双模板', () => {
  test('窄屏：卡片列表与页头堆叠', async ({ page }) => {
    await gotoWorkflowList(page, { width: 390 })

    const cards = page.locator('article')
    await expect(cards).toHaveCount(2)
    await expect(page.locator('table')).toHaveCount(0)

    await expect(page.getByRole('button', { name: '导入' })).toBeVisible()
    await expect(page.getByRole('button', { name: /新建工作流/ })).toBeVisible()

    const runBtn = cards.first().getByRole('button', { name: '运行' })
    await expect(runBtn).toBeVisible()
    const box = await runBtn.boundingBox()
    expect(box?.height).toBeGreaterThanOrEqual(44)
  })

  test('更多菜单项与 Esc 关闭', async ({ page }) => {
    await gotoWorkflowList(page, { width: 390 })

    const more = page.getByRole('button', { name: '更多操作' }).first()
    await expect(more).toHaveAttribute('aria-haspopup', 'menu')
    await more.click()
    await expect(more).toHaveAttribute('aria-expanded', 'true')

    const menu = page.getByRole('menu')
    await expect(menu).toBeVisible()
    await expect(menu.getByRole('menuitem', { name: '编辑' })).toBeVisible()
    await expect(menu.getByRole('menuitem', { name: '复制' })).toBeVisible()
    await expect(menu.getByRole('menuitem', { name: '导出' })).toBeVisible()
    await expect(menu.getByRole('menuitem', { name: '删除' })).toBeVisible()

    await page.keyboard.press('Escape')
    await expect(menu).toHaveCount(0)
    await expect(more).toHaveAttribute('aria-expanded', 'false')
  })

  test('滚动关闭更多菜单', async ({ page }) => {
    await gotoWorkflowList(page, { width: 390, height: 500 })

    // Extra tall content so scroll fires even with two cards.
    await page.evaluate(() => {
      const spacer = document.createElement('div')
      spacer.style.height = '1200px'
      document.body.appendChild(spacer)
    })

    const more = page.getByRole('button', { name: '更多操作' }).first()
    await more.click()
    await expect(page.getByRole('menu')).toBeVisible()

    await page.evaluate(() => window.scrollBy(0, 120))
    await expect(page.getByRole('menu')).toHaveCount(0)
  })

  test('运行隔离与主体进编辑', async ({ page }) => {
    await gotoWorkflowList(page, { width: 390 })

    const card = page.locator('article').first()
    await card.getByRole('button', { name: '运行' }).click()
    await expect(page.getByText(/启动运行/)).toBeVisible({ timeout: 5_000 })
    await expect(page.getByTestId('edit-page')).toHaveCount(0)

    // Close launch modal via backdrop click.
    await page.locator('.fixed.inset-0 > .absolute.inset-0').click({ position: { x: 8, y: 8 } })
    await expect(page.getByText(/启动运行/)).toHaveCount(0)

    await card.locator('button').filter({ hasText: 'Demo Workflow' }).click()
    await expect(page.getByTestId('edit-page')).toBeVisible({ timeout: 5_000 })
  })

  test('桌面六列表格零回归', async ({ page }) => {
    await gotoWorkflowList(page, { width: 1280, height: 800 })

    await expect(page.locator('article')).toHaveCount(0)
    const table = page.locator('table')
    await expect(table).toBeVisible()
    await expect(table.getByRole('columnheader', { name: '名称' })).toBeVisible()
    await expect(table.getByRole('columnheader', { name: '状态' })).toBeVisible()
    await expect(table.getByRole('columnheader', { name: '版本' })).toBeVisible()
    await expect(table.getByRole('columnheader', { name: '仓库' })).toBeVisible()
    await expect(table.getByRole('columnheader', { name: '最近运行' })).toBeVisible()
    await expect(table.getByRole('columnheader', { name: '操作' })).toBeVisible()

    const actionCell = table.locator('tbody tr').first().locator('td').last()
    await expect(actionCell.getByRole('button', { name: '编辑' })).toBeVisible()
    await expect(actionCell.getByRole('button', { name: '运行' })).toBeVisible()
    await expect(actionCell.getByRole('button', { name: '复制' })).toBeVisible()
    await expect(actionCell.getByRole('button', { name: '导出' })).toBeVisible()
    await expect(actionCell.getByRole('button', { name: '删除' })).toBeVisible()
  })

  test('窄屏浅色主题可读性', async ({ page }) => {
    await gotoWorkflowList(page, { width: 390, theme: 'light' })
    await expect(page.locator('article')).toHaveCount(2)
    const overflow = await page.evaluate(() => {
      const el = document.documentElement
      return el.scrollWidth > el.clientWidth + 1
    })
    expect(overflow).toBe(false)
  })
})
