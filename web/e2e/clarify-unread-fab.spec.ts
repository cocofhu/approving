import { expect, test, type Page } from '@playwright/test'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const __dirname = path.dirname(fileURLToPath(import.meta.url))
const shotDir = path.join(__dirname, '..', 'test-results', 'clarify-unread-fab')

async function gotoHarness(page: Page) {
  await page.goto('/clarify-unread-fab.html')
  await expect(page.getByTestId('clarify-unread-fab-root')).toBeVisible({ timeout: 15_000 })
  await expect(page.getByTestId('clarify-scroller')).toBeVisible()
}

test.describe('ClarifyChat unread FAB (browser)', () => {
  test('off-bottom accumulates exact unread; FAB visual matches Demo; clear paths work', async ({
    page,
  }, testInfo) => {
    await gotoHarness(page)

    // Stick path: near bottom + new turn → no FAB
    await page.getByTestId('btn-add-agent').click()
    await expect(page.getByTestId('clarify-unread-fab')).toHaveCount(0)
    await page.screenshot({
      path: path.join(shotDir, '01-stick-no-fab.png'),
      fullPage: false,
    })

    // Leave bottom then +1 → FAB shows "1", no visible copy beyond icon+digit
    await page.getByTestId('btn-leave-bottom').click()
    await page.getByTestId('btn-add-agent').click()
    const fab = page.getByTestId('clarify-unread-fab')
    await expect(fab).toBeVisible()
    await expect(fab).toContainText('1')
    await expect(fab).toHaveAttribute('aria-label', /有 1 条新消息/)
    await expect(fab).toHaveAttribute('title', /有 1 条新消息/)
    const fabText = (await fab.innerText()).replace(/\s+/g, '')
    expect(fabText).toMatch(/1/)
    expect(fabText).not.toMatch(/有新消息|new messages/i)

    // Position: centered over scroller, above composer
    const scrollerBox = await page.getByTestId('clarify-scroller').boundingBox()
    const fabBox = await fab.boundingBox()
    const inputBox = await page.getByTestId('clarify-input').boundingBox()
    expect(scrollerBox).toBeTruthy()
    expect(fabBox).toBeTruthy()
    expect(inputBox).toBeTruthy()
    if (scrollerBox && fabBox && inputBox) {
      const fabCenterX = fabBox.x + fabBox.width / 2
      const scrollerCenterX = scrollerBox.x + scrollerBox.width / 2
      expect(Math.abs(fabCenterX - scrollerCenterX)).toBeLessThan(24)
      expect(fabBox.y + fabBox.height).toBeLessThanOrEqual(inputBox.y + 2)
      expect(fabBox.y).toBeGreaterThan(scrollerBox.y + scrollerBox.height * 0.5)
    }

    await fab.screenshot({ path: path.join(shotDir, '02-fab-unread-1.png') })
    await page.screenshot({
      path: path.join(shotDir, '03-chat-with-fab.png'),
      fullPage: false,
    })

    // Accumulate +3 → exact 4
    await page.getByTestId('btn-add-3').click()
    await expect(fab).toContainText('4')
    await fab.screenshot({ path: path.join(shotDir, '04-fab-unread-4.png') })

    // Click FAB clears
    await fab.click()
    await expect(page.getByTestId('clarify-unread-fab')).toHaveCount(0)

    // Manual scroll-near-bottom clears
    await page.getByTestId('btn-leave-bottom').click()
    await page.getByTestId('btn-add-agent').click()
    await expect(page.getByTestId('clarify-unread-fab')).toBeVisible()
    await page.evaluate(() => {
      const el = document.querySelector('[data-testid="clarify-scroller"]') as HTMLElement
      el.scrollTop = el.scrollHeight
      el.dispatchEvent(new Event('scroll'))
    })
    await expect(page.getByTestId('clarify-unread-fab')).toHaveCount(0)

    // Send force-sticks and clears
    await page.getByTestId('btn-leave-bottom').click()
    await page.getByTestId('btn-add-agent').click()
    await expect(page.getByTestId('clarify-unread-fab')).toBeVisible()
    await page.getByTestId('clarify-input').fill('我看完了，继续。')
    await page.getByTestId('clarify-input').press('Enter')
    await expect(page.getByTestId('clarify-unread-fab')).toHaveCount(0)
    await page.screenshot({
      path: path.join(shotDir, '05-after-send-cleared.png'),
      fullPage: false,
    })

    // Three-digit exact count (no 99+)
    await page.getByTestId('btn-leave-bottom').click()
    await page.getByTestId('btn-add-128').click()
    const fabBig = page.getByTestId('clarify-unread-fab')
    await expect(fabBig).toBeVisible()
    await expect(fabBig).toContainText('128')
    await expect(fabBig).not.toContainText('99+')
    await expect(fabBig).toHaveAttribute('aria-label', /128/)
    await fabBig.screenshot({ path: path.join(shotDir, '06-fab-unread-128.png') })
    await page.screenshot({
      path: path.join(shotDir, '07-chat-fab-128.png'),
      fullPage: false,
    })

    testInfo.annotations.push({
      type: 'shotDir',
      description: shotDir,
    })
  })
})
