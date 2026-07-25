import { defineConfig } from '@playwright/test'

export default defineConfig({
  testDir: './e2e',
  testMatch: '*.spec.ts',
  timeout: 30_000,
  retries: 0,
  // Shared vite.e2e mock state (vncConnectCount / delay/fail flags) is process-global.
  workers: 1,
  fullyParallel: false,
  // CI uploads html report + traces on failure (see ci-web web-e2e job).
  reporter: process.env.CI ? [['list'], ['html', { open: 'never' }]] : 'list',
  use: {
    headless: true,
    locale: 'zh-CN',
    trace: 'retain-on-failure',
  },
  webServer: {
    command: 'npx vite --config vite.e2e.config.ts',
    port: 5174,
    reuseExistingServer: !process.env.CI,
  },
})
