import { test, expect } from '@playwright/test'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const __dirname = path.dirname(fileURLToPath(import.meta.url))
const shotDir = path.join(__dirname, '..', 'test-results', 'cold-silent-shots')

async function mockApi(page: import('@playwright/test').Page) {
  await page.route('**/api/**', async (route) => {
    const url = new URL(route.request().url())
    if (url.pathname.includes('/preview-issues')) {
      await route.fulfill({ json: { issues: [] } })
      return
    }
    if (url.pathname.includes('/artifacts/')) {
      await route.fulfill({
        json: {
          content: JSON.stringify({
            summary: 'human_gate 冷会话静默验收',
            goals: ['冷态无 ReAct 提示', '热态就地改保留'],
          }),
          etag: 'e1',
          updatedAt: '2026-07-18T00:00:00Z',
          sizeBytes: 64,
        },
      })
      return
    }
    if (url.pathname.includes('/primary-artifacts') || url.pathname.includes('/gate/')) {
      await route.fulfill({ status: 400, json: { error: 'offline' } })
      return
    }
    await route.fulfill({ status: 404, json: { error: 'not mocked' } })
  })
}

test.describe('human_gate cold silent / hot in-place', () => {
  test('cold: no ReAct/hot hints or send; confirm + ordinary help', async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 800 })
    await mockApi(page)
    await page.goto('/gate-cold-silent.html?session=cold')
    await expect(page.getByTestId('gate-cold-silent-root')).toBeVisible({ timeout: 15_000 })
    await expect(page.getByTestId('gate-cold-silent-root')).toHaveAttribute('data-session', 'cold')

    await expect(page.getByTestId('gate-cold-session-note')).toHaveCount(0)
    await expect(page.getByTestId('review-composer-cold-note')).toHaveCount(0)
    await expect(page.getByTestId('review-composer-send')).toHaveCount(0)
    await expect(page.getByTestId('gate-cold-help')).toBeVisible()
    await expect(page.getByTestId('gate-cold-help')).toContainText('确认并流转')
    await expect(page.getByRole('button', { name: '确认并流转' })).toBeVisible()

    const body = await page.locator('body').innerText()
    expect(body).not.toMatch(/ReAct|热会话|就地改|恢复热会话|无法继续就地改码/)

    await page.screenshot({
      path: path.join(shotDir, 'cold-session-silent.png'),
      fullPage: true,
    })
  })

  test('hot: send present; no cold note/help', async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 800 })
    await mockApi(page)
    await page.goto('/gate-cold-silent.html?session=hot')
    await expect(page.getByTestId('gate-cold-silent-root')).toBeVisible({ timeout: 15_000 })
    await expect(page.getByTestId('gate-cold-silent-root')).toHaveAttribute('data-session', 'hot')

    await expect(page.getByTestId('review-composer-send')).toBeVisible()
    await expect(page.getByTestId('gate-cold-session-note')).toHaveCount(0)
    await expect(page.getByTestId('gate-cold-help')).toHaveCount(0)
    await expect(page.getByRole('button', { name: '确认并流转' })).toBeVisible()

    await page.screenshot({
      path: path.join(shotDir, 'hot-session-inplace.png'),
      fullPage: true,
    })
  })
})
