import { test, expect } from '@playwright/test'

async function gotoRunDetail(
  page: import('@playwright/test').Page,
  opts: { width: number; height?: number; view?: 'stats' | 'timeline' } = { width: 390 },
) {
  await page.setViewportSize({ width: opts.width, height: opts.height ?? 844 })
  const qs = new URLSearchParams()
  if (opts.view === 'timeline') qs.set('view', 'timeline')
  const suffix = qs.toString() ? `?${qs}` : ''
  await page.goto(`/run-detail-mobile.html${suffix}`)
  await expect(page.getByTestId('run-detail-root')).toBeVisible({ timeout: 10_000 })
}

async function noHorizontalOverflow(page: import('@playwright/test').Page) {
  const overflow = await page.evaluate(() => {
    const doc = document.documentElement
    const body = document.body
    return Math.max(doc.scrollWidth, body.scrollWidth) > Math.max(doc.clientWidth, body.clientWidth) + 1
  })
  expect(overflow).toBe(false)
}

/** KPI grid: grid-cols-1 → md:grid-cols-3 → xl:grid-cols-5 (wall/node/gap/tokens/rate). */
const KPI_CARD_COUNT = 5

test.describe('Run 详情移动端适配', () => {
  test('390px：页头两行、状态全文、操作外露（f1/f2）', async ({ page }) => {
    await gotoRunDetail(page, { width: 390 })

    const status = page.getByTestId('status-pill')
    await expect(status).toContainText('等待人工')
    const statusBox = await status.boundingBox()
    expect(statusBox).toBeTruthy()
    expect(statusBox!.x + statusBox!.width).toBeLessThanOrEqual(390 + 1)

    // 工作流名窄屏隐藏，版本保留
    await expect(page.getByTestId('workflow-chip')).toBeHidden()
    await expect(page.getByTestId('version-chip')).toBeVisible()

    const actions = page.getByTestId('run-header-actions')
    for (const name of ['编辑', '详情', '刷新', '取消运行']) {
      const btn = actions.getByRole('button', { name })
      await expect(btn).toBeVisible()
      const box = await btn.boundingBox()
      expect(box).toBeTruthy()
      expect(box!.x + box!.width).toBeLessThanOrEqual(390 + 1)
    }

    // 两行：操作行 top 低于状态行
    const row1 = await page.getByTestId('run-header-row1').boundingBox()
    const row2 = await actions.boundingBox()
    expect(row1 && row2).toBeTruthy()
    expect(row2!.y).toBeGreaterThan(row1!.y + 8)

    await noHorizontalOverflow(page)
  })

  test('390px：摘要单列、safe-area、变量 inline 展开（f3/f4/f5）', async ({ page }) => {
    await gotoRunDetail(page, { width: 390, view: 'stats' })

    const panel = page.getByTestId('stats-panel')
    await expect(panel).toBeVisible()

    const scroll = panel.locator('.scroll-area.safe-area-bottom')
    await expect(scroll).toHaveCount(1)

    const cards = panel.locator('.mb-3\\.5.grid > div')
    await expect(cards).toHaveCount(KPI_CARD_COUNT)
    const tops: number[] = []
    for (let i = 0; i < KPI_CARD_COUNT; i++) {
      const box = await cards.nth(i).boundingBox()
      expect(box).toBeTruthy()
      expect(box!.x + box!.width).toBeLessThanOrEqual(390 + 1)
      tops.push(box!.y)
    }
    // 单列堆叠
    for (let i = 1; i < tops.length; i++) {
      expect(tops[i]).toBeGreaterThan(tops[i - 1])
    }

    await page.getByRole('button', { name: '时间线' }).click()
    await expect(page.getByTestId('timeline-panel')).toBeVisible()

    // 外层时间线条目也是 role=button 且 accessible name 含 repos，须锚定 chip 自身。
    const chip = page.locator('button').filter({ hasText: /^repos\s*=/ })
    await expect(chip).toBeVisible()
    await chip.click()
    const expanded = page.locator('pre').filter({ hasText: /^repos\s*=/ })
    await expect(expanded).toBeVisible()
    await expect(expanded).toContainText('approving')

    await chip.click()
    await expect(expanded).toHaveCount(0)

    await noHorizontalOverflow(page)
  })

  test('1024px：页头单行与摘要三列（桌面回归）', async ({ page }) => {
    await gotoRunDetail(page, { width: 1024, height: 768, view: 'stats' })

    await expect(page.getByTestId('workflow-chip')).toBeVisible()
    await expect(page.getByTestId('status-pill')).toContainText('等待人工')

    const row1 = await page.getByTestId('run-header-row1').boundingBox()
    const actions = await page.getByTestId('run-header-actions').boundingBox()
    expect(row1 && actions).toBeTruthy()
    // 单行：状态与操作近似同行
    expect(Math.abs(row1!.y - actions!.y)).toBeLessThan(12)

    // md:grid-cols-3 → first three KPIs share a row; 4th wraps
    const cards = page.getByTestId('stats-panel').locator('.mb-3\\.5.grid > div')
    await expect(cards).toHaveCount(KPI_CARD_COUNT)
    const y0 = (await cards.nth(0).boundingBox())!.y
    const y1 = (await cards.nth(1).boundingBox())!.y
    const y2 = (await cards.nth(2).boundingBox())!.y
    const y3 = (await cards.nth(3).boundingBox())!.y
    expect(Math.abs(y0 - y1)).toBeLessThan(8)
    expect(Math.abs(y1 - y2)).toBeLessThan(8)
    expect(y3).toBeGreaterThan(y0 + 8)
  })
})
