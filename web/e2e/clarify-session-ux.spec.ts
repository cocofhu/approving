import { test, expect } from '@playwright/test'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const shotDir = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../../../.tmp-clarify-ux-shots')

async function sendClarify(page: import('@playwright/test').Page, text: string) {
  const box = page.locator('[data-testid="clarify-input"]')
  await box.fill(text)
  await page.getByTestId('clarify-send-label').click()
}

test.describe('ClarifyChat 非 reviewMode 澄清会话', () => {
  test('排队 → 流式 → Cancel 保留队列 → 刷新续传', async ({ page }) => {
    await page.setViewportSize({ width: 1100, height: 900 })
    await page.goto('/clarify-session-ux.html')
    await expect(page.getByTestId('clarify-ux-root')).toBeVisible({ timeout: 15_000 })

    await sendClarify(page, '澄清意见甲（队列首条）')
    await sendClarify(page, '澄清意见乙（仍在队列）')
    const queue = page.getByTestId('clarify-review-queue')
    await expect(queue).toBeVisible()
    await expect(queue).toContainText('待发送队列')
    await expect(queue).toContainText('澄清意见甲')
    await expect(queue).toContainText('澄清意见乙')
    await page.screenshot({ path: path.join(shotDir, '01-clarify-queue.png'), fullPage: true })

    await page.getByTestId('sim-turn-stream').click()
    await expect(page.getByText('流式产出正文（非整轮一次性）。')).toBeVisible()
    await expect(queue).toContainText('澄清意见乙')
    await page.screenshot({ path: path.join(shotDir, '02-clarify-stream.png'), fullPage: true })

    // Demo Cancel: keep queue (乙 remains), mark interrupted.
    await page.getByTestId('clarify-review-cancel').click()
    await expect(page.getByTestId('clarify-interrupted')).toBeVisible()
    await expect(page.getByTestId('clarify-review-queue')).toContainText('澄清意见乙')
    await page.screenshot({ path: path.join(shotDir, '03-clarify-cancel-keep.png'), fullPage: true })

    // Refresh resume: busy + activeItem + continued stream.
    await page.getByTestId('sim-refresh-resume').click()
    await expect(page.getByText('刷新后续上的流式正文。')).toBeVisible()
    await expect(page.getByTestId('clarify-review-queue')).toContainText('澄清意见乙')
    await page.screenshot({ path: path.join(shotDir, '04-clarify-refresh-resume.png'), fullPage: true })
  })
})
