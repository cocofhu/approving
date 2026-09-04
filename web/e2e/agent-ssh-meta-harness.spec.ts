import { test, expect } from '@playwright/test'
import path from 'node:path'
import fs from 'node:fs'

const OUT = path.join('/tmp', 'ssh-meta-e2e-shots')

test.beforeAll(() => {
  fs.mkdirSync(OUT, { recursive: true })
})

test('Agent 元信息 SSH 双栏：明文 host、密文私钥、禁止 vars 提示', async ({ page }) => {
  await page.goto('/agent-ssh-meta-harness.html', { waitUntil: 'networkidle' })
  await expect(page.getByTestId('agent-ssh-meta-root')).toBeVisible({ timeout: 15_000 })

  const hosts = page.locator('[data-test="agent-ssh-known-hosts"]')
  const key = page.locator('[data-test="agent-ssh-private-key"]')
  await expect(hosts).toBeVisible()
  await expect(key).toBeVisible()

  // Hint that meta SSH does not support ${vars.*}
  await expect(page.getByText(/不支持.*\$\{vars|vars\.\*|引用/).first()).toBeVisible()

  const multilineHosts = 'github.com ssh-ed25519 AAAAtest1\ngitlab.com ssh-ed25519 AAAAtest2'
  await hosts.fill(multilineHosts)
  await expect(hosts).toHaveValue(multilineHosts)

  await key.fill('-----BEGIN OPENSSH PRIVATE KEY-----\nsecret-line\n-----END OPENSSH PRIVATE KEY-----')
  // Private key uses disc masking when non-empty
  const keyStyle = await key.evaluate((el) => getComputedStyle(el).webkitTextSecurity)
  expect(keyStyle).toBe('disc')

  await page.screenshot({ path: path.join(OUT, '01-ssh-meta-filled.png'), fullPage: true })
})
