/**
 * Temporary / verification E2E for keep_footer_hide_bottom (dual「刚刚」fix).
 * Asserts completed agent turns keep a single relTime channel in the footer.
 */
import { test, expect } from '@playwright/test'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const shotDir = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  '../../../.tmp-clarify-reltime-shots',
)

test.describe('ClarifyChat 完成态相对时间单通道', () => {
  test('完成态：已完成脚注一处相对时间，无底部重复', async ({ page }) => {
    await page.setViewportSize({ width: 1100, height: 900 })
    await page.goto('/clarify-session-ux.html')
    await expect(page.getByTestId('clarify-ux-root')).toBeVisible({ timeout: 15_000 })

    await page.getByTestId('sim-four-phase-complete').click()
    await expect(page.getByText('四阶段正文输出。')).toBeVisible()

    const completed = page.getByTestId('clarify-turn-completed')
    await expect(completed).toBeVisible()
    await expect(completed).toContainText('已完成')
    await expect(completed).toContainText('刚刚')
    const footerText = await completed.innerText()
    expect(footerText.match(/刚刚/g)?.length ?? 0).toBe(1)

    // Agent completed turn must not render bottom time; human turn still may.
    const bottomTimes = page.getByTestId('clarify-turn-bottom-time')
    await expect(bottomTimes).toHaveCount(1)

    // Within the completed agent column, only the footer channel shows 刚刚
    const agentMsg = page.getByTestId('clarify-agent-message')
    await expect(agentMsg).toBeVisible()
    const agentColumn = agentMsg.locator(
      'xpath=ancestor::*[.//*[@data-testid="clarify-turn-completed"]][1]',
    )
    await expect(agentColumn.getByTestId('clarify-turn-bottom-time')).toHaveCount(0)

    await page.screenshot({
      path: path.join(shotDir, '01-completed-single-reltime.png'),
      fullPage: true,
    })
  })

  test('流式中：无完成脚注，底部时间仍保留', async ({ page }) => {
    await page.setViewportSize({ width: 1100, height: 900 })
    await page.goto('/clarify-session-ux.html')
    await expect(page.getByTestId('clarify-ux-root')).toBeVisible({ timeout: 15_000 })

    await page.getByTestId('clarify-input').fill('流式探测')
    await page.getByTestId('clarify-send-label').click()
    await page.getByTestId('sim-turn-stream').click()
    await expect(page.getByText('流式产出正文（非整轮一次性）。')).toBeVisible()

    await expect(page.getByTestId('clarify-turn-completed')).toHaveCount(0)
    await expect(page.getByTestId('clarify-stream-caret')).toBeVisible()
    // human + streaming agent
    await expect(page.getByTestId('clarify-turn-bottom-time')).toHaveCount(2)

    await page.screenshot({
      path: path.join(shotDir, '02-streaming-keeps-bottom-time.png'),
      fullPage: true,
    })
  })
})
