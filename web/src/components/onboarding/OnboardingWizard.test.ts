// @vitest-environment happy-dom
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { nextTick } from 'vue'
import OnboardingWizard from './OnboardingWizard.vue'
import {
  DEFAULT_PROJECT_ID,
  dismissOnboarding,
  isOnboardingDismissed,
  shouldAutoOpenOnboarding,
} from '@/lib/pm/onboardingWizard'

vi.mock('@/lib/api/api', () => ({
  api: {
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
  },
}))

vi.mock('@/lib/composables/useToast', () => ({
  useToast: () => ({ success: vi.fn(), error: vi.fn(), warn: vi.fn(), show: vi.fn() }),
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

describe('OnboardingWizard', () => {
  beforeEach(() => {
    localStorage.clear()
    vi.clearAllMocks()
  })

  it('shows language first and persists a language switch', async () => {
    const wrapper = mount(OnboardingWizard, {
      props: { open: true, projectId: DEFAULT_PROJECT_ID },
      global: { stubs: { Teleport: true, Icon: true, AppButton: true } },
    })
    expect(wrapper.find('[data-testid="onboarding-language-zh-CN"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="onboarding-language-en"]').exists()).toBe(true)
    await wrapper.find('[data-testid="onboarding-language-zh-CN"]').trigger('click')
    await vi.waitFor(() => {
      expect(localStorage.getItem('approving-locale')).toBe('zh-CN')
    })
  })

  it('sends the repo typed in the git step', async () => {
    const { api } = await import('@/lib/api/api')
    const wrapper = mount(OnboardingWizard, {
      props: { open: true, projectId: DEFAULT_PROJECT_ID },
      global: { stubs: { Teleport: true, Icon: true, AppButton: true } },
    })
    for (let i = 0; i < 3; i++) {
      await wrapper.find('[data-testid="onboarding-next"]').trigger('click')
      await nextTick()
    }
    await wrapper.find('[data-testid="onboarding-api-key"]').setValue('crsr_test')
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
    const wrapper = mount(OnboardingWizard, {
      props: { open: true, projectId: DEFAULT_PROJECT_ID },
      global: { stubs: { Teleport: true, Icon: true, AppButton: true } },
    })
    for (let i = 0; i < 3; i++) {
      await wrapper.find('[data-testid="onboarding-next"]').trigger('click')
      await nextTick()
    }
    await wrapper.find('[data-testid="onboarding-api-key"]').setValue('crsr_test')
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

  it('dismiss via later suppresses auto-open', async () => {
    const wrapper = mount(OnboardingWizard, {
      props: { open: true, projectId: DEFAULT_PROJECT_ID },
      global: { stubs: { Teleport: true, Icon: true, AppButton: true } },
    })
    await wrapper.find('[data-testid="onboarding-later"]').trigger('click')
    expect(wrapper.emitted('close')).toBeTruthy()
    expect(isOnboardingDismissed(DEFAULT_PROJECT_ID)).toBe(true)
    expect(shouldAutoOpenOnboarding(DEFAULT_PROJECT_ID, 0, [])).toBe(false)
  })

  it('backdrop dismiss suppresses auto-open', async () => {
    const wrapper = mount(OnboardingWizard, {
      props: { open: true, projectId: DEFAULT_PROJECT_ID },
      global: { stubs: { Teleport: true, Icon: true, AppButton: true } },
    })
    await wrapper.find('[data-testid="onboarding-backdrop"]').trigger('click')
    expect(isOnboardingDismissed(DEFAULT_PROJECT_ID)).toBe(true)
  })

  it('close button suppresses auto-open', async () => {
    const wrapper = mount(OnboardingWizard, {
      props: { open: true, projectId: DEFAULT_PROJECT_ID },
      global: { stubs: { Teleport: true, Icon: true, AppButton: true } },
    })
    await wrapper.find('[data-testid="onboarding-close"]').trigger('click')
    expect(isOnboardingDismissed(DEFAULT_PROJECT_ID)).toBe(true)
  })

  it('manual CTA can open even after dismiss', () => {
    dismissOnboarding(DEFAULT_PROJECT_ID)
    expect(shouldAutoOpenOnboarding(DEFAULT_PROJECT_ID, 0, [])).toBe(false)
  })

  it('blocks generate without API key then succeeds with key', async () => {
    const { api } = await import('@/lib/api/api')
    const wrapper = mount(OnboardingWizard, {
      props: { open: true, projectId: DEFAULT_PROJECT_ID },
      global: { stubs: { Teleport: true, Icon: true, AppButton: true } },
    })
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

    const input = wrapper.find('[data-testid="onboarding-api-key"]')
    await input.setValue('crsr_test')
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
    expect(isOnboardingDismissed(DEFAULT_PROJECT_ID)).toBe(true)
  })
})
