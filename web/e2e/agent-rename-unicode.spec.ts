import { test, expect } from '@playwright/test'
import path from 'node:path'
import fs from 'node:fs'

const OUT = path.join('/tmp', 'agent-rename-unicode-e2e')

test.beforeAll(() => {
  fs.mkdirSync(OUT, { recursive: true })
})

test('重命名 Agent 支持中文名（Demo 样例 + 非法样例）', async ({ page }) => {
  await page.goto('/agent-rename-unicode.html', { waitUntil: 'networkidle' })
  await expect(page.getByTestId('agent-rename-unicode-root')).toBeVisible()

  const input = page.getByTestId('rename-input')
  const confirm = page.getByTestId('rename-confirm')

  // Prefill is screenshot sample; should be valid (green) and confirm enabled.
  await expect(input).toHaveValue('Approve需求澄清视觉研发')
  await expect(page.getByTestId('rename-ok')).toContainText('校验通过')
  await expect(confirm).toBeEnabled()
  const okText = await page.getByTestId('rename-ok').innerText()
  expect(okText).not.toContain('仅允许字母、数字、- 和 _')
  await page.screenshot({ path: path.join(OUT, '01-chinese-valid.png'), fullPage: true })

  // Illegal: space
  await input.fill('Approve 需求')
  await expect(page.getByTestId('rename-error')).toBeVisible()
  await expect(page.getByTestId('rename-error')).toContainText('Unicode')
  await expect(confirm).toBeDisabled()
  const errText = await page.getByTestId('rename-error').innerText()
  expect(errText).not.toBe('仅允许字母、数字、- 和 _')
  await page.screenshot({ path: path.join(OUT, '02-space-invalid.png'), fullPage: true })

  // Illegal: dotted legacy write target
  await input.fill('clarify.v1')
  await expect(page.getByTestId('rename-error')).toContainText('Unicode')
  await expect(confirm).toBeDisabled()
  await page.screenshot({ path: path.join(OUT, '03-dot-invalid.png'), fullPage: true })

  // Illegal: fullwidth hyphen
  await input.fill('需求－澄清')
  await expect(page.getByTestId('rename-error')).toBeVisible()
  await expect(confirm).toBeDisabled()

  // Legal mixed hyphen Chinese → submit
  await input.fill('Approve-需求澄清')
  await expect(page.getByTestId('rename-ok')).toContainText('校验通过')
  await expect(confirm).toBeEnabled()
  await page.screenshot({ path: path.join(OUT, '04-mixed-valid.png'), fullPage: true })
  await confirm.click()
  await expect(page.getByTestId('renamed-to')).toHaveText('Approve-需求澄清')
})
