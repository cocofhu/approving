/**
 * Browser acceptance for PM citation false-positive governance (f4/f5/f6 + Demo scenes).
 */
import { expect, test, type Page } from '@playwright/test'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const __dirname = path.dirname(fileURLToPath(import.meta.url))
const shotDir = path.join(__dirname, '..', 'test-results', 'pm-citation-fp')

async function openRunDetail(page: Page, id: string, mode: 'not_found' | 'network') {
  await page.setViewportSize({ width: 1280, height: 900 })
  await page.route('**/api/**', async (route) => {
    // Skip Vite module URLs like /@fs/.../src/lib/api/api.ts (pathname is not /api/...)
    if (!new URL(route.request().url()).pathname.startsWith('/api/')) {
      await route.continue()
      return
    }
    if (mode === 'network') {
      await route.abort('failed')
      return
    }
    const url = new URL(route.request().url())
    if (url.pathname.startsWith('/api/runs/')) {
      await route.fulfill({ status: 404, json: { error: 'not found' } })
      return
    }
    await route.fulfill({ status: 404, json: { error: 'not mocked' } })
  })
  await page.goto(`/pm-run-load-error.html?id=${encodeURIComponent(id)}`)
  await expect(page.getByTestId('run-detail-root')).toBeVisible({ timeout: 15_000 })
  await expect(page.getByTestId('run-load-error')).toBeVisible({ timeout: 15_000 })
}

test.describe('PM citation false-positive (browser)', () => {
  test('history fake citation greyed + blocked; valid card readable', async ({ page }) => {
    await page.setViewportSize({ width: 900, height: 700 })
    await page.goto('/pm-citation-fp.html')
    await expect(page.getByTestId('pm-citation-fp-root')).toBeVisible({ timeout: 15_000 })

    const invalid = page.getByTestId('citation-card-invalid')
    await expect(invalid).toBeVisible()
    await expect(invalid).toContainText('run:trigger')
    await expect(page.getByTestId('citation-invalid-note')).toHaveText('引用无效或目标不存在')
    const invalidJump = invalid.getByTestId('citation-open-source')
    await expect(invalidJump).toBeDisabled()
    await invalidJump.click({ force: true })
    await expect(page.getByTestId('run-nav')).toHaveCount(0)

    const valid = page.getByTestId('citation-card-valid')
    await expect(valid).toContainText('Run #a1b2c3d4')
    await expect(valid).toContainText('需求澄清 · 进行中')
    await expect(valid).not.toContainText('run:run-a1b2c3d4')
    await expect(valid.getByTestId('citation-open-source')).toBeEnabled()

    await page.screenshot({ path: path.join(shotDir, '01-citation-cards.png'), fullPage: true })
  })

  test('Run #trigger not-found: copy + grey retry unavailable', async ({ page }) => {
    await openRunDetail(page, 'trigger', 'not_found')

    await expect(page.getByTestId('run-load-error-chip')).toHaveText('不存在')
    await expect(page.getByTestId('run-load-error')).toContainText('找不到该 Run')
    await expect(page.getByTestId('run-load-error')).toContainText('这通常不是网络问题')
    await expect(page.getByTestId('run-load-error')).not.toContainText('请检查网络或确认 Run 是否存在')
    const unavailable = page.getByTestId('run-retry-unavailable')
    await expect(unavailable).toBeVisible()
    await expect(unavailable).toBeDisabled()
    await expect(unavailable).toHaveText('重试不可用')
    await expect(page.getByTestId('run-retry')).toHaveCount(0)

    await page.screenshot({ path: path.join(shotDir, '02-run-trigger-not-found.png'), fullPage: false })
  })

  test('network/server error keeps usable retry', async ({ page }) => {
    await openRunDetail(page, 'run-a1b2c3d4', 'network')

    await expect(page.getByTestId('run-load-error-chip')).toHaveText('加载失败')
    await expect(page.getByTestId('run-load-error')).toContainText('暂时无法加载运行详情')
    await expect(page.getByTestId('run-load-error')).toContainText('网络或服务异常')
    const retry = page.getByTestId('run-retry')
    await expect(retry).toBeVisible()
    await expect(retry).toBeEnabled()
    await expect(page.getByTestId('run-retry-unavailable')).toHaveCount(0)

    await page.screenshot({ path: path.join(shotDir, '03-run-network-error.png'), fullPage: false })
  })
})
