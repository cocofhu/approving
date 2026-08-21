/**
 * Browser acceptance: ArtifactPreview page.html version chip (Artifacts page path).
 * Temporary harness for test-node gate; mirrors Approve-stage version UX.
 */
import { test, expect } from '@playwright/test'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import fs from 'node:fs'

const shotDir = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  '../../../.tmp-artifacts-version-shots',
)

test.describe('Artifacts page.html version switch (browser)', () => {
  test.beforeAll(() => {
    fs.mkdirSync(shotDir, { recursive: true })
  })

  test('multi-version chip switch + historical readonly + single/json hide', async ({ page }) => {
    await page.setViewportSize({ width: 1100, height: 800 })
    const shot = (name: string) =>
      page.screenshot({ path: path.join(shotDir, name), animations: 'disabled' })

    // --- multi version ---
    await page.goto('/artifacts-page-version.html?scenario=multi')
    await expect(page.getByTestId('artifacts-version-harness-root')).toBeVisible({ timeout: 15_000 })
    const chipBtn = page.getByTestId('artifact-preview-version-chip-btn')
    await expect(chipBtn).toBeVisible()
    await expect(chipBtn).toContainText('v2')
    await expect(chipBtn).toContainText('最新')
    await expect(page.getByTestId('artifact-preview-historical-readonly')).toHaveCount(0)
    await page.waitForTimeout(400)
    await shot('01-multi-latest.png')

    await chipBtn.click()
    await expect(page.getByTestId('artifact-preview-version-menu')).toBeVisible()
    await expect(page.getByTestId('artifact-preview-version-option-v1')).toHaveText('v1')
    await expect(page.getByTestId('artifact-preview-version-option-v2')).toContainText('最新')
    await page.waitForTimeout(200)
    await shot('02-version-menu.png')

    await page.getByTestId('artifact-preview-version-option-v1').click()
    await expect(page.getByTestId('artifact-preview-historical-readonly')).toBeVisible()
    await expect(page.getByTestId('artifact-preview-delete')).toHaveCount(0)
    await page.waitForTimeout(400)
    await shot('03-historical-readonly.png')

    await page.getByTestId('artifact-preview-version-chip-btn').click()
    await page.getByTestId('artifact-preview-version-option-v2').click()
    await expect(page.getByTestId('artifact-preview-historical-readonly')).toHaveCount(0)
    await page.waitForTimeout(300)
    await shot('04-back-to-latest.png')

    // --- single version: no chip ---
    await page.goto('/artifacts-page-version.html?scenario=single')
    await expect(page.getByTestId('artifacts-version-harness-root')).toBeVisible({ timeout: 15_000 })
    await expect(page.getByTestId('artifact-preview-version-chip')).toHaveCount(0)
    await page.waitForTimeout(300)
    await shot('05-single-no-chip.png')

    // --- non page.html ---
    await page.goto('/artifacts-page-version.html?scenario=json')
    await expect(page.getByTestId('artifacts-version-harness-root')).toBeVisible({ timeout: 15_000 })
    await expect(page.getByTestId('artifact-preview-version-chip')).toHaveCount(0)
    await page.waitForTimeout(300)
    await shot('06-json-no-chip.png')

    // --- run load failure degrade ---
    await page.goto('/artifacts-page-version.html?scenario=no-run')
    await expect(page.getByTestId('artifacts-version-harness-root')).toBeVisible({ timeout: 15_000 })
    await expect(page.getByTestId('artifact-preview-version-chip')).toHaveCount(0)
    await page.waitForTimeout(300)
    await shot('07-no-run-degrade.png')
  })
})
