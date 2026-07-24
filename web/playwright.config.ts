import { defineConfig } from '@playwright/test'

export default defineConfig({
  testDir: './e2e',
  testMatch: '*.spec.ts',
  timeout: 30_000,
  retries: 0,
  // Shared vite.e2e mock state (vncConnectCount / delay/fail flags) is process-global.
  workers: 1,
  fullyParallel: false,
  use: {
    headless: true,
    locale: 'zh-CN',
  },
  webServer: {
    command: 'npx vite --config vite.e2e.config.ts',
    port: 5174,
    reuseExistingServer: !process.env.CI,
  },
})
