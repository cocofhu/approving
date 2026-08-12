import { test, expect } from '@playwright/test'
import fs from 'node:fs'
import path from 'node:path'

const OUT = '/tmp/status-metrics-shots'
fs.mkdirSync(OUT, { recursive: true })

test.describe('StatusMetrics topbar E2E', () => {
  test('desktop: five metrics left of lang, /5m rate, tip, no TOK labels', async ({ page }, testInfo) => {
    await page.setViewportSize({ width: 1280, height: 720 })
    await page.goto('/status-metrics-topbar.html')
    const metrics = page.getByTestId('status-metrics')
    await expect(metrics).toBeVisible({ timeout: 15_000 })

    await expect(page.getByTestId('status-metrics-tokens')).toContainText(/1\.24M/i)
    await expect(page.getByTestId('status-metrics-rate')).toContainText(/\/5m/)
    await expect(page.getByTestId('status-metrics-peak')).toBeVisible()
    await expect(page.getByTestId('status-metrics-running')).toContainText('3')
    await expect(page.getByTestId('status-metrics-queued')).toContainText('5')
    // Visible values are icon+compact numbers (no TOK/5M/PEAK label chips).
    const visibleVals = await page.locator('[data-testid^="status-metrics-"] .sm-val').allTextContents()
    expect(visibleVals.join(' ')).not.toMatch(/\bTOK\b|\b5M\b|\bPEAK\b/)
    await expect(page.getByTestId('status-metrics-compact')).toHaveCount(0)

    // StatusMetrics sits before LangSelect in the topbar DOM order.
    const order = await page.evaluate(() => {
      const header = document.querySelector('header')
      if (!header) return []
      return Array.from(header.children).map((el) => {
        if ((el as HTMLElement).dataset?.testid === 'status-metrics') return 'status-metrics'
        if (el.querySelector?.('[data-testid="status-metrics"]')) return 'status-metrics'
        if (el.tagName === 'SELECT' || el.querySelector?.('select') || el.textContent?.includes('中文'))
          return 'lang'
        return el.tagName.toLowerCase()
      })
    })
    const smIdx = order.indexOf('status-metrics')
    const langIdx = order.findIndex((x) => x === 'lang')
    expect(smIdx).toBeGreaterThanOrEqual(0)
    expect(langIdx).toBeGreaterThan(smIdx)

    const tokensTip = page.getByTestId('status-metrics-tokens').locator('.sm-tip')
    await page.getByTestId('status-metrics-tokens').hover()
    await expect(tokensTip).toBeVisible()
    await expect(tokensTip).toContainText('累计')
    await expect(tokensTip).toContainText('1,240,582')
    await expect(tokensTip).not.toContainText('完整值')
    await expect(tokensTip).not.toContainText('/5m')

    const rateTip = page.getByTestId('status-metrics-rate').locator('.sm-tip')
    await page.getByTestId('status-metrics-rate').hover()
    await expect(rateTip).toBeVisible()
    await expect(rateTip).toContainText('速率')
    await expect(rateTip).toContainText('4,812')
    await expect(rateTip).not.toContainText('/5m')

    // Click pin (tip-open) still works.
    await page.getByTestId('status-metrics-running').click()
    await expect(page.getByTestId('status-metrics-running')).toHaveClass(/tip-open/)
    await expect(page.getByTestId('status-metrics-running').locator('.sm-tip')).toBeVisible()
    await expect(page.getByTestId('status-metrics-running').locator('.sm-tip')).toContainText(/执行中:\s*3/)

    const shot = path.join(testInfo.outputDir, 'desktop-five-metrics.png')
    await page.locator('header').screenshot({ path: shot })
    await page.screenshot({
      path: path.join(OUT, '01-desktop-status-metrics.png'),
      fullPage: false,
    })
  })

  test('narrow: Token·RUN/Q summary; compact tip five label:value rows', async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 })
    await page.goto('/status-metrics-topbar.html')
    await expect(page.getByTestId('status-metrics-compact')).toBeVisible({ timeout: 15_000 })
    await expect(page.getByTestId('status-metrics-tokens')).toHaveCount(0)
    const compact = page.getByTestId('status-metrics-compact')
    await expect(compact).toContainText(/1\.24M/i)
    await expect(compact).toContainText('3')
    await expect(compact).toContainText('5')

    await compact.click()
    const tip = compact.locator('.sm-tip')
    await expect(tip).toBeVisible()
    await expect(tip).toContainText(/累计 Token:\s*1,240,582/)
    await expect(tip).toContainText(/速率:\s*4,812/)
    await expect(tip).toContainText(/峰值:\s*12,104/)
    await expect(tip).toContainText(/执行中:\s*3/)
    await expect(tip).toContainText(/排队:\s*5/)
    await expect(tip).not.toContainText('/5m')
    await expect(tip).not.toContainText('完整值')

    await page.screenshot({
      path: path.join(OUT, '02-narrow-status-metrics.png'),
      fullPage: false,
    })
  })

  test('null tokens show em-dash; running/queued true zero', async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 720 })
    await page.goto('/status-metrics-topbar.html?scene=null')
    await expect(page.getByTestId('status-metrics-tokens')).toContainText('—', { timeout: 15_000 })
    await expect(page.getByTestId('status-metrics-rate')).toContainText('—')
    await expect(page.getByTestId('status-metrics-peak')).toContainText('—')
    await expect(page.getByTestId('status-metrics-running')).toContainText('0')
    await expect(page.getByTestId('status-metrics-queued')).toContainText('0')

    await page.screenshot({
      path: path.join(OUT, '03-null-emdash.png'),
      fullPage: false,
    })
  })

  test('metrics are buttons that do not navigate', async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 720 })
    await page.goto('/status-metrics-topbar.html')
    await expect(page.getByTestId('status-metrics')).toBeVisible({ timeout: 15_000 })
    const urlBefore = page.url()
    await page.getByTestId('status-metrics-rate').click()
    await expect(page).toHaveURL(urlBefore)
    await expect(page.getByTestId('page-body')).toBeVisible()
  })
})
