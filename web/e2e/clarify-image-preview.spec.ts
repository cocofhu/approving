import { test, expect } from '@playwright/test'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import fs from 'node:fs'

const shotDir = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  '../../../.tmp-clarify-image-preview-shots',
)

const PNG_A =
  'iVBORw0KGgoAAAANSUhEUgAAAPAAAACgCAIAAAC9uXYyAAABRElEQVR42u3SQQ0AAAgDsQmbfxWIQQUf0qQKLpdp4Y1IgKHB0GBoMDSGBkODocHQYGgMDYYGQ4OhwdAYGgwNhgZDg6ExNBgaDA2GBkNjaDA0GBoMDYbG0GBoMDQYGgyNocHQYGgwNIYGQ4OhwdBgaAwNhgZDg6HB0BgaDA2GBkODoTE0GBoMDYYGQ2NoMDQYGgwNhsbQYGgwNBgaDI2hwdBgaDA0GBpDg6HB0GBoDA2GBkODocHQGBoMDYYGQ4OhMTQYGgwNhgZDY2gwNBgaDA2GxtBgaDA0GBoMjaHB0GBoMDQYGkODocHQYGgMrQKGBkODocHQGBoMDYYGQ4OhMTQYGgwNhgZDY2gwNBgaDA2GxtBgaDA0GBoMjaHB0GBoMDQYGkODocHQYGgwNIYGQ4OhwdAYGgwNhgZDg6ExNBgaDA03FrNxQ6p/RCs5AAAAAElFTkSuQmCC'
const PNG_B =
  'iVBORw0KGgoAAAANSUhEUgAAAPAAAACgCAIAAAC9uXYyAAABRElEQVR42u3SQQ0AAAjEsJODJtQhFRV8SJMqWJaahjciAYYGQ4OhwdAYGgwNhgZDg6ExNBgaDA2GBkNjaDA0GBoMDYbG0GBoMDQYGgyNocHQYGgwNBgaQ4OhwdBgaDA0hgZDg6HB0BgaDA2GBkODoTE0GBoMDYYGQ2NoMDQYGgwNhsbQYGgwNBgaDI2hwdBgaDA0GBpDg6HB0GBoMDSGBkODocHQYGgMDYYGQ4OhMTQYGgwNhgZDY2gwNBgaDA2GxtBgaDA0GBoMjaHB0GBoMDQYGkODocHQYGgwNIYGQ4OhwdBgaAwNhgZDg6ExtAoYGgwNhgZDY2gwNBgaDA2GxtBgaDA0GBoMjaHB0GBoMDQYGkODocHQYGgwNIYGQ4OhwdBgaAwNhgZDg6HB0BgaDA2GBkNjaDA0GBoMDYbG0GBoMDTcWANS9egxNJk8AAAAAElFTkSuQmCC'

test.describe('ClarifyChat human history image AppModal preview', () => {
  test.beforeAll(() => {
    fs.mkdirSync(shotDir, { recursive: true })
  })

  test('open / title / × / backdrop / Esc stay / multi-image / scope guards', async ({ page }) => {
    await page.setViewportSize({ width: 1100, height: 900 })
    await page.goto('/clarify-image-preview.html')
    await expect(page.getByTestId('clarify-image-preview-root')).toBeVisible({ timeout: 15_000 })

    const thumbs = page.getByTestId('clarify-history-image-thumb')
    await expect(thumbs).toHaveCount(2)
    await expect(thumbs.first()).toHaveClass(/cursor-pointer/)
    await expect(thumbs.first()).toHaveClass(/hover:border-accent/)

    await thumbs.first().hover()
    await expect(thumbs.first()).toContainText('点击放大')
    const shot = (name: string) =>
      page.screenshot({ path: path.join(shotDir, name), animations: 'disabled' })

    await page.waitForTimeout(200)
    await shot('01-thumbs-hover.png')

    await thumbs.nth(0).click()
    const previewImg = page.getByTestId('clarify-image-preview-img')
    await expect(previewImg).toBeVisible()
    await expect(page.getByText('图片预览 · 图片 1')).toBeVisible()
    await expect(previewImg).toHaveAttribute('src', `data:image/png;base64,${PNG_A}`)
    await expect(previewImg).toHaveClass(/object-contain/)
    await expect(page.getByTestId('clarify-image-preview-prev')).toHaveCount(0)
    await expect(page.getByTestId('clarify-image-preview-next')).toHaveCount(0)
    await page.waitForTimeout(300)
    await shot('02-modal-image-1.png')

    await page.keyboard.press('Escape')
    await expect(previewImg).toBeVisible()
    await expect(page.getByText('图片预览 · 图片 1')).toBeVisible()

    const headerClose = page.locator('.fixed.inset-0.z-50 .h-14 button')
    await headerClose.click()
    await expect(previewImg).toHaveCount(0)

    await thumbs.nth(0).click()
    await expect(previewImg).toBeVisible()
    await page.locator('.fixed.inset-0.z-50 > .absolute.inset-0').click({ position: { x: 8, y: 8 } })
    await expect(previewImg).toHaveCount(0)
    await page.waitForTimeout(200)
    await shot('03-closed.png')

    await thumbs.nth(1).click()
    await expect(page.getByText('图片预览 · 图片 2')).toBeVisible()
    await expect(previewImg).toHaveAttribute('src', `data:image/png;base64,${PNG_B}`)
    await page.waitForTimeout(300)
    await shot('04-modal-image-2.png')
    await headerClose.click()
    await expect(previewImg).toHaveCount(0)

    const agentThumb = page.getByTestId('clarify-agent-image-thumb')
    await expect(agentThumb).toBeVisible()
    await agentThumb.click()
    await expect(page.getByTestId('clarify-image-preview-img')).toHaveCount(0)

    const draftSrc = `data:image/png;base64,${PNG_A}`
    const draftImg = page.locator(`img[src="${draftSrc}"]`).last()
    await draftImg.click()
    await expect(page.getByTestId('clarify-image-preview-img')).toHaveCount(0)
    await shot('05-scope-guards.png')
  })
})
