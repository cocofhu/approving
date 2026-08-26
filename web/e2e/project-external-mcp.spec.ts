import { test, expect } from '@playwright/test'
import { dismissOnboardingIfOpen, seedOnboardingDismissed } from './helpers/onboarding'

const MOCK_PROJECT = {
  id: 'proj-1',
  name: 'Demo Project',
  description: 'Project for e2e',
  variables: [],
  createdAt: '2026-01-01T00:00:00Z',
  updatedAt: '2026-01-01T00:00:00Z',
}

/**
 * plan g3.1 / review v2: open ?tab=externalMcp with onboarding dismissed
 * so the panel (switch / save / keys) is visible without the wizard overlay.
 */
async function gotoExternalMcpPanel(page: import('@playwright/test').Page) {
  await page.setViewportSize({ width: 1280, height: 800 })
  await seedOnboardingDismissed(page, 'proj-1')

  await page.route('**/api/projects/proj-1', async (route) => {
    if (route.request().method() === 'GET' || route.request().method() === 'PUT') {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify(MOCK_PROJECT),
      })
      return
    }
    await route.continue()
  })
  await page.route('**/api/workflows**', async (route) => {
    if (route.request().method() === 'GET') {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify([{ id: 'wf-1', name: 'Alpha', status: 'draft', nodes: [], edges: [] }]),
      })
      return
    }
    await route.continue()
  })
  await page.route('**/api/runs**', async (route) => {
    if (route.request().method() === 'GET') {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ items: [], total: 0, page: 1, pageSize: 20, hasMore: false }),
      })
      return
    }
    await route.continue()
  })
  await page.route('**/api/agents', async (route) => {
    if (route.request().method() === 'GET') {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify([{ name: 'demo-agent', projectId: 'proj-1' }]),
      })
      return
    }
    await route.continue()
  })
  await page.route('**/api/projects/proj-1/pm-leader', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ enabled: false }),
    })
  })
  await page.route('**/api/projects/proj-1/external-mcp', async (route) => {
    if (route.request().method() === 'GET') {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          enabled: true,
          enabledPacks: ['pm-progress', 'pm-agent-fs'],
          mcpBaseUrl: 'http://api.example.com/mcp/external/proj-1',
        }),
      })
      return
    }
    if (route.request().method() === 'PUT') {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          enabled: true,
          enabledPacks: ['pm-progress'],
          mcpBaseUrl: 'http://api.example.com/mcp/external/proj-1',
        }),
      })
      return
    }
    await route.continue()
  })
  await page.route('**/api/projects/proj-1/external-mcp/keys', async (route) => {
    if (route.request().method() === 'GET') {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify([
          {
            id: 'k1',
            name: 'cursor',
            key_prefix: 'cf_proj_••••abcd',
            created_at: '2026-01-01T00:00:00Z',
          },
        ]),
      })
      return
    }
    if (route.request().method() === 'POST') {
      await route.fulfill({
        status: 201,
        contentType: 'application/json',
        body: JSON.stringify({
          id: 'k2',
          name: 'new-key',
          key: 'cf_proj_plaintext_once',
          key_prefix: 'cf_proj_••••once',
          created_at: '2026-01-02T00:00:00Z',
        }),
      })
      return
    }
    await route.continue()
  })

  await page.goto('/project-detail.html?tab=externalMcp')
  await dismissOnboardingIfOpen(page)
  await expect(page.getByRole('heading', { name: 'Demo Project' })).toBeVisible({ timeout: 10_000 })
  await expect(page.getByTestId('onboarding-backdrop')).toHaveCount(0)
}

test.describe('项目外部 MCP 面板（无引导遮罩）', () => {
  test('总开关、保存、创钥与密钥行可见 (g3.1/g3.2)', async ({ page }) => {
    await gotoExternalMcpPanel(page)

    await expect(page.getByTestId('project-external-mcp-panel')).toBeVisible()
    await expect(page.getByTestId('external-mcp-enabled')).toBeVisible()
    await expect(page.getByTestId('external-mcp-save')).toBeVisible()
    await expect(page.getByTestId('external-mcp-create-key')).toBeVisible()
    await expect(page.getByTestId('external-mcp-key-row')).toBeVisible()
    await expect(page.getByText('http://api.example.com/mcp/external/proj-1')).toBeVisible()
    // g3.2 review v3: {base} must be interpolated, not shown literally
    await expect(page.getByText(/完整 URL 形如：http:\/\/api\.example\.com\/mcp\/external\/proj-1\/pm-progress/)).toBeVisible()
    await expect(page.getByText('{base}')).toHaveCount(0)
  })
})
