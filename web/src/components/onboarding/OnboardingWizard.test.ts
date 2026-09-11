// @vitest-environment happy-dom
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { nextTick } from 'vue'
import OnboardingWizard from './OnboardingWizard.vue'
import {
  DEFAULT_PROJECT_ID,
  suppressOnboarding,
  isOnboardingSuppressed,
  ONBOARDING_WORKFLOW_NAME,
  shouldAutoOpenOnboarding,
} from '@/lib/pm/onboardingWizard'

vi.mock('@/lib/api/api', () => ({
  api: {
    createProject: vi.fn(),
    bootstrapProjectOnboarding: vi.fn(async () => ({
      agentIds: [
        '综合AI技术产品',
        '综合研发工程师',
        '综合测试工程师',
        '综合代码审查工程师',
        '综合运维工程师',
        '综合项目组组长',
      ],
      workflowId: 'wf-1',
      published: true,
      groupName: '综合项目组',
    })),
    openCodeProviders: vi.fn(async () => ({
      providers: [{ id: 'deepseek', name: 'DeepSeek', models: 1 }],
    })),
    openCodeModels: vi.fn(async () => ({ models: [{ id: 'deepseek-v4-pro' }] })),
  },
}))

vi.mock('@/lib/composables/useToast', () => ({
  useToast: () => ({ success: vi.fn(), error: vi.fn(), warn: vi.fn(), show: vi.fn() }),
}))

vi.mock('@/lib/composables/useProjectContext', () => ({
  writeStoredProjectId: vi.fn(),
}))

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({
      t: (k: string) => k,
    }),
  }
})

import { createMemoryHistory, createRouter } from 'vue-router'

async function mountWizard(props: Record<string, unknown> = {}) {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/', component: { template: '<div />' } },
      { path: '/projects/:id', component: { template: '<div />' } },
    ],
  })
  await router.push('/')
  return mount(OnboardingWizard, {
    props: { open: true, projectId: DEFAULT_PROJECT_ID, mode: 'firstInstall', ...props },
    global: {
      plugins: [router],
      stubs: { Teleport: true, Icon: true, AppButton: true },
    },
  })
}
/** Picks the vendor and model the catalog stub serves, then fills the key. */
async function fillOpenCodeAuth(wrapper: ReturnType<typeof mount>, key = 'sk-oc-demo') {
  await wrapper
    .get('[data-test="opencode-provider"] [data-test="app-select-trigger"]')
    .trigger('click')
  await wrapper.get('[data-test="app-select-option-deepseek"]').trigger('click')
  await flushPromises()
  await wrapper.get('[data-test="opencode-model"] [data-test="app-select-trigger"]').trigger('click')
  await wrapper.get('[data-test="app-select-option-deepseek/deepseek-v4-pro"]').trigger('click')
  await wrapper.find('[data-testid="onboarding-api-key"]').setValue(key)
}

describe('OnboardingWizard', () => {
  beforeEach(() => {
    localStorage.clear()
    vi.clearAllMocks()
  })

  it('shows language first and persists a language switch', async () => {
    const wrapper = await mountWizard()
    expect(wrapper.find('[data-testid="onboarding-language-zh-CN"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="onboarding-language-en"]').exists()).toBe(true)
    await wrapper.find('[data-testid="onboarding-language-zh-CN"]').trigger('click')
    await vi.waitFor(() => {
      expect(localStorage.getItem('approving-locale')).toBe('zh-CN')
    })
  })

  it('sets the theme from the first step', async () => {
    const wrapper = await mountWizard()
    expect(wrapper.find('[data-testid="onboarding-theme-dark"]').exists()).toBe(true)
    await wrapper.find('[data-testid="onboarding-theme-light"]').trigger('click')
    expect(localStorage.getItem('approving-theme')).toBe('light')
    expect(document.documentElement.classList.contains('light')).toBe(true)
    await wrapper.find('[data-testid="onboarding-theme-dark"]').trigger('click')
    expect(localStorage.getItem('approving-theme')).toBe('dark')
    expect(document.documentElement.classList.contains('light')).toBe(false)
  })

  it('backend step offers two start paths and swaps the detail block', async () => {
    const wrapper = await mountWizard()
    for (let i = 0; i < 2; i++) {
      await wrapper.find('[data-testid="onboarding-next"]').trigger('click')
      await nextTick()
    }
    // API-key path is the default: vendor chips, no CLI tiles.
    expect(wrapper.find('[data-testid="onboarding-path-apikey-detail"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="onboarding-backend-cursor"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="onboarding-backend-opencode"]').exists()).toBe(false)

    await wrapper.find('[data-testid="onboarding-path-cli"]').trigger('click')
    await nextTick()
    expect(wrapper.find('[data-testid="onboarding-backend-cursor"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="onboarding-backend-trae"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="onboarding-path-apikey-detail"]').exists()).toBe(false)

    // Back to the API-key path restores the vendor chips.
    await wrapper.find('[data-testid="onboarding-path-apiKey"]').trigger('click')
    await nextTick()
    expect(wrapper.find('[data-testid="onboarding-path-apikey-detail"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="onboarding-backend-cursor"]').exists()).toBe(false)
  })

  it('bootstraps with opencode when the API-key path is chosen', async () => {
    const { api } = await import('@/lib/api/api')
    const wrapper = await mountWizard()
    for (let i = 0; i < 2; i++) {
      await wrapper.find('[data-testid="onboarding-next"]').trigger('click')
      await nextTick()
    }
    await wrapper.find('[data-testid="onboarding-next"]').trigger('click')
    await nextTick()
    await wrapper.find('[data-testid="onboarding-api-key"]').setValue('sk-oc-demo')
    await wrapper.find('[data-testid="onboarding-next"]').trigger('click')
    await nextTick()
    expect(wrapper.find('[data-test="opencode-model-required"]').exists()).toBe(true)

    await fillOpenCodeAuth(wrapper)
    await wrapper.find('[data-testid="onboarding-next"]').trigger('click')
    await nextTick()
    await wrapper.find('[data-testid="onboarding-git-user-name"]').setValue('Ada')
    await wrapper.find('[data-testid="onboarding-git-user-email"]').setValue('ada@example.com')
    await wrapper.find('[data-testid="onboarding-next"]').trigger('click')
    await nextTick()
    await wrapper.find('[data-testid="onboarding-next"]').trigger('click')
    await flushPromises()

    expect(api.bootstrapProjectOnboarding).toHaveBeenCalled()
    const body = (api.bootstrapProjectOnboarding as any).mock.calls[0][1]
    expect(body.acpBackend).toBe('opencode')
    expect(body.apiKey).toBe('sk-oc-demo')
    expect(body.openCodeProvider).toBe('deepseek')
    expect(body.openCodeModel).toBe('deepseek/deepseek-v4-pro')
  })

  it('sends the repo typed in the git step', async () => {
    const { api } = await import('@/lib/api/api')
    const wrapper = await mountWizard()
    for (let i = 0; i < 3; i++) {
      await wrapper.find('[data-testid="onboarding-next"]').trigger('click')
      await nextTick()
    }
    await fillOpenCodeAuth(wrapper, 'crsr_test')
    await wrapper.find('[data-testid="onboarding-next"]').trigger('click')
    await nextTick()

    // useI18n is stubbed to echo keys, so assert the hint switches to the resolved-dir copy.
    const hint = () => wrapper.find('[data-testid="onboarding-repo-hint"]').text()
    expect(hint()).toBe('pages.onboarding.repo.hint')
    await wrapper.find('[data-testid="onboarding-repo-url"]').setValue('https://github.com/org/web.git')
    await wrapper.find('[data-testid="onboarding-repo-branch"]').setValue('develop')
    await nextTick()
    expect(hint()).toBe('pages.onboarding.repo.cloneTo')

    await wrapper.find('[data-testid="onboarding-git-user-name"]').setValue('Ada Lovelace')
    await wrapper.find('[data-testid="onboarding-git-user-email"]').setValue('ada@example.com')
    await wrapper.find('[data-testid="onboarding-next"]').trigger('click')
    await nextTick()
    await wrapper.find('[data-testid="onboarding-next"]').trigger('click')
    await flushPromises()

    expect(api.bootstrapProjectOnboarding).toHaveBeenCalledWith(
      DEFAULT_PROJECT_ID,
      expect.objectContaining({
        repoUrl: 'https://github.com/org/web.git',
        repoBranch: 'develop',
        gitUserName: 'Ada Lovelace',
        gitUserEmail: 'ada@example.com',
        vncPreview: true,
        browserMcp: true,
      }),
    )
  })

  it('skipping the git step drops the repo too', async () => {
    const { api } = await import('@/lib/api/api')
    const wrapper = await mountWizard()
    for (let i = 0; i < 3; i++) {
      await wrapper.find('[data-testid="onboarding-next"]').trigger('click')
      await nextTick()
    }
    await fillOpenCodeAuth(wrapper, 'crsr_test')
    await wrapper.find('[data-testid="onboarding-next"]').trigger('click')
    await nextTick()

    await wrapper.find('[data-testid="onboarding-repo-url"]').setValue('https://github.com/org/web.git')
    await wrapper.find('[data-testid="onboarding-skip"]').trigger('click')
    await nextTick()
    expect(api.bootstrapProjectOnboarding).not.toHaveBeenCalled()

    await wrapper.find('[data-testid="onboarding-git-user-name"]').setValue('Ada Lovelace')
    await wrapper.find('[data-testid="onboarding-git-user-email"]').setValue('ada@example.com')
    await wrapper.find('[data-testid="onboarding-skip"]').trigger('click')
    await nextTick()
    await wrapper.find('[data-testid="onboarding-next"]').trigger('click')
    await flushPromises()

    const calls = vi.mocked(api.bootstrapProjectOnboarding).mock.calls
    expect(calls[calls.length - 1]?.[1]).not.toHaveProperty('repoUrl')
    expect(calls[calls.length - 1]?.[1]).toEqual(
      expect.objectContaining({
        gitUserName: 'Ada Lovelace',
        gitUserEmail: 'ada@example.com',
        vncPreview: true,
        browserMcp: true,
      }),
    )
  })

  it('later closes without persisting, so a reload re-opens the wizard', async () => {
    const wrapper = await mountWizard()
    await wrapper.find('[data-testid="onboarding-later"]').trigger('click')
    expect(wrapper.emitted('close')).toBeTruthy()
    expect(isOnboardingSuppressed(DEFAULT_PROJECT_ID)).toBe(false)
    expect(shouldAutoOpenOnboarding(DEFAULT_PROJECT_ID, [], [])).toBe(true)
  })

  it('backdrop close does not persist suppression', async () => {
    const wrapper = await mountWizard()
    await wrapper.find('[data-testid="onboarding-backdrop"]').trigger('click')
    expect(wrapper.emitted('close')).toBeTruthy()
    expect(isOnboardingSuppressed(DEFAULT_PROJECT_ID)).toBe(false)
  })

  it('close button does not persist suppression', async () => {
    const wrapper = await mountWizard()
    await wrapper.find('[data-testid="onboarding-close"]').trigger('click')
    expect(wrapper.emitted('close')).toBeTruthy()
    expect(isOnboardingSuppressed(DEFAULT_PROJECT_ID)).toBe(false)
  })

  it('the storage escape hatch still suppresses auto-open', () => {
    suppressOnboarding(DEFAULT_PROJECT_ID)
    expect(shouldAutoOpenOnboarding(DEFAULT_PROJECT_ID, [], [])).toBe(false)
  })

  it('blocks generate without API key then succeeds with key', async () => {
    const { api } = await import('@/lib/api/api')
    const wrapper = await mountWizard()
    await wrapper.setProps({ open: false })
    await wrapper.setProps({ open: true })
    await nextTick()
    await wrapper.find('[data-testid="onboarding-next"]').trigger('click')
    await nextTick()
    await wrapper.find('[data-testid="onboarding-next"]').trigger('click')
    await nextTick()
    await wrapper.find('[data-testid="onboarding-next"]').trigger('click')
    await nextTick()
    expect(api.bootstrapProjectOnboarding).not.toHaveBeenCalled()

    await fillOpenCodeAuth(wrapper, 'crsr_test')
    await wrapper.find('[data-testid="onboarding-next"]').trigger('click')
    await nextTick()
    await wrapper.find('[data-testid="onboarding-next"]').trigger('click')
    await nextTick()
    await wrapper.find('[data-testid="onboarding-git-user-name"]').setValue('Ada Lovelace')
    await wrapper.find('[data-testid="onboarding-git-user-email"]').setValue('ada@example.com')
    await wrapper.find('[data-testid="onboarding-next"]').trigger('click')
    await nextTick()
    await wrapper.find('[data-testid="onboarding-next"]').trigger('click')
    await flushPromises()
    expect(api.bootstrapProjectOnboarding).toHaveBeenCalled()
    expect(wrapper.find('[data-testid="onboarding-success"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="onboarding-success-git"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="onboarding-start-run"]').exists()).toBe(false)
    // The created default workflow is what stops the wizard from re-opening.
    expect(isOnboardingSuppressed(DEFAULT_PROJECT_ID)).toBe(false)
    expect(
      shouldAutoOpenOnboarding(DEFAULT_PROJECT_ID, [{ name: ONBOARDING_WORKFLOW_NAME }], []),
    ).toBe(false)
  })

  /** Advance createProject wizard from projectName through review (generate). */
  async function advanceCreateToGenerate(wrapper: Awaited<ReturnType<typeof mountWizard>>) {
    await wrapper.find('[data-testid="onboarding-project-name"]').setValue('支付中台')
    await wrapper.find('[data-testid="onboarding-next"]').trigger('click')
    await nextTick()
    // overview → acp → apiKey (no language step in createProject)
    for (let i = 0; i < 2; i++) {
      await wrapper.find('[data-testid="onboarding-next"]').trigger('click')
      await nextTick()
    }
    await fillOpenCodeAuth(wrapper, 'sk-create')
    await wrapper.find('[data-testid="onboarding-next"]').trigger('click')
    await nextTick()
    // git (skip identity via filling) → review
    await wrapper.find('[data-testid="onboarding-git-user-name"]').setValue('Ada')
    await wrapper.find('[data-testid="onboarding-git-user-email"]').setValue('ada@example.com')
    await wrapper.find('[data-testid="onboarding-next"]').trigger('click')
    await nextTick()
    await wrapper.find('[data-testid="onboarding-next"]').trigger('click')
    await flushPromises()
  }

  it('create mode: bootstrap failure then retry does not call createProject again (s3/f6)', async () => {
    const { api } = await import('@/lib/api/api')
    vi.mocked(api.createProject).mockResolvedValue({ id: 'proj-created-1', name: '支付中台' } as never)
    vi.mocked(api.bootstrapProjectOnboarding)
      .mockRejectedValueOnce(new Error('bootstrap blew up'))
      .mockResolvedValueOnce({
        agentIds: ['支付中台研发工程师'],
        workflowId: 'wf-x',
        published: true,
        groupName: '支付中台项目组',
      } as never)

    const wrapper = await mountWizard({ mode: 'createProject', projectId: '' })
    expect(wrapper.find('[data-testid="onboarding-title"]').text()).toBe('pages.onboarding.titleCreate')

    await advanceCreateToGenerate(wrapper)

    expect(api.createProject).toHaveBeenCalledTimes(1)
    expect(api.bootstrapProjectOnboarding).toHaveBeenCalledTimes(1)
    expect(api.bootstrapProjectOnboarding).toHaveBeenCalledWith('proj-created-1', expect.any(Object))
    expect(wrapper.find('[data-testid="onboarding-success"]').exists()).toBe(false)

    // Stay on review and generate again — must not create another project.
    await wrapper.find('[data-testid="onboarding-next"]').trigger('click')
    await flushPromises()

    expect(api.createProject).toHaveBeenCalledTimes(1)
    expect(api.bootstrapProjectOnboarding).toHaveBeenCalledTimes(2)
    expect(api.bootstrapProjectOnboarding).toHaveBeenNthCalledWith(2, 'proj-created-1', expect.any(Object))
    expect(wrapper.find('[data-testid="onboarding-success"]').exists()).toBe(true)
  })

  it('create mode shows derived-team copy instead of 综合*', async () => {
    const wrapper = await mountWizard({ mode: 'createProject', projectId: '' })
    expect(wrapper.find('[data-testid="onboarding-title"]').text()).toBe('pages.onboarding.titleCreate')
    await wrapper.find('[data-testid="onboarding-project-name"]').setValue('支付中台')
    await wrapper.find('[data-testid="onboarding-next"]').trigger('click')
    await nextTick()
    // projectName → overview directly (no language step)
    expect(wrapper.find('[data-testid="onboarding-overview-agents"]').text()).toBe(
      'pages.onboarding.overview.agentsListDerived',
    )
  })

  it('createProject skips language step and does not overwrite approving-locale (g2.1)', async () => {
    localStorage.setItem('approving-locale', 'zh-CN')
    const { locale } = await import('@/lib/shared/locale')
    locale.value = 'zh-CN'
    vi.stubGlobal('navigator', { language: 'en-US' })

    const wrapper = await mountWizard({ mode: 'createProject', projectId: '' })
    expect(wrapper.find('[data-testid="onboarding-language-zh-CN"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="onboarding-project-name"]').exists()).toBe(true)
    expect(localStorage.getItem('approving-locale')).toBe('zh-CN')
    expect(locale.value).toBe('zh-CN')

    await wrapper.find('[data-testid="onboarding-later"]').trigger('click')
    await nextTick()
    expect(localStorage.getItem('approving-locale')).toBe('zh-CN')
    expect(locale.value).toBe('zh-CN')
    vi.unstubAllGlobals()
  })
})
