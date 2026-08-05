import { test, expect } from '@playwright/test'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import fs from 'node:fs'
import os from 'node:os'

const shotDir = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  '../../../.tmp-file-attach-shots',
)

test.describe('ClarifyChat arbitrary file attachments', () => {
  test.beforeAll(() => {
    fs.mkdirSync(shotDir, { recursive: true })
  })

  test('ask_question + PDF attach + oversize reject + no accept filter', async ({ page }) => {
    await page.setViewportSize({ width: 1100, height: 900 })
    await page.goto('/file-attach-harness.html')
    await expect(page.getByTestId('file-attach-root')).toBeVisible({ timeout: 15_000 })

    const shot = (name: string) =>
      page.screenshot({ path: path.join(shotDir, name), animations: 'disabled' })

    // History shows non-image file chip with original name.
    const histChip = page.getByTestId('clarify-history-file-chip')
    await expect(histChip).toBeVisible()
    await expect(histChip).toContainText('需求说明-v3.pdf')

    // ask_question card is options-only (no file input inside the card).
    await expect(page.getByText('本次需求需要我优先阅读哪类材料？')).toBeVisible()
    const askCard = page.locator('.border-n-clarify\\/25').first()
    await expect(askCard.locator('input[type="file"]')).toHaveCount(0)
    await expect(askCard.getByRole('button', { name: '现有需求文档（PDF/Word）' })).toBeVisible()

    // Composer attach input accepts any type (no accept=image/*).
    const fileInput = page.locator('input[type="file"]')
    await expect(fileInput).toHaveCount(1)
    await expect(fileInput).not.toHaveAttribute('accept', /image/)

    await shot('01-ask-question-and-pdf-history.png')

    // Select a small PDF via composer; pending chip shows original name.
    const tmpPdf = path.join(os.tmpdir(), '验收材料.pdf')
    fs.writeFileSync(tmpPdf, '%PDF-1.4 tiny fixture\n')
    await page.getByTestId('clarify-attach-btn').click()
    await fileInput.setInputFiles({
      name: '验收材料.pdf',
      mimeType: 'application/pdf',
      buffer: Buffer.from('%PDF-1.4 tiny fixture\n'),
    })
    await expect(page.getByTestId('clarify-pending-file-chip')).toBeVisible({ timeout: 5_000 })
    await expect(page.getByTestId('clarify-pending-file-chip')).toContainText('验收材料.pdf')
    await shot('02-pending-pdf-chip.png')

    // Send carries name on the attachment payload.
    await page.getByTestId('clarify-input').fill('附上材料')
    await page.getByTestId('clarify-send-icon').click()
    const lastSent = page.getByTestId('file-attach-last-sent')
    await expect
      .poll(async () => lastSent.textContent(), { timeout: 8_000 })
      .toMatch(/验收材料\.pdf/)

    await shot('03-sent-with-name.png')

    // Selection-stage oversize (>50 MiB): write a sparse file (Playwright forbids >50Mb buffers).
    const hugePath = path.join(os.tmpdir(), 'dataset-backup.tar.gz')
    const fd = fs.openSync(hugePath, 'w')
    fs.ftruncateSync(fd, 50 * 1024 * 1024 + 64)
    fs.closeSync(fd)
    await fileInput.setInputFiles(hugePath)
    const notice = page.getByTestId('clarify-attach-notice')
    await expect(notice).toBeVisible({ timeout: 15_000 })
    await expect(notice).toContainText('50 MiB')
    await expect(notice).toContainText('dataset-backup.tar.gz')
    await expect(page.getByTestId('clarify-pending-file-chip')).toHaveCount(0)
    await shot('04-oversize-select-reject.png')
    fs.unlinkSync(hugePath)
  })
})
