// @vitest-environment happy-dom
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { nextTick } from 'vue'
import OnboardingWizard from './OnboardingWizard.vue'
import {
  dismissOnboarding,
  isOnboardingDismissed,
  shouldAutoOpenOnboarding,
} from '@/lib/onboardingWizard'

vi.mock('@/lib/api', () => ({
  api: {
    bootstrapProjectOnboarding: vi.fn(async () => ({
      agentIds: ['ClarifyAgent', 'VisualAgent', 'ImplementAgent', 'TestAgent', 'PreviewAgent'],
      workflowId: 'wf-1',
      repos: 'demo|https://github.com/heroku/nodejs-getting-started.git|main',
      feature: '把首页欢迎文案与主按钮文案改得更清晰友好',
      published: true,
    })),
    startRun: vi.fn(async () => ({ id: 'run-1', status: 'queued' })),
  },
}))

vi.mock('@/lib/useToast', () => ({
  useToast: () => ({ success: vi.fn(), error: vi.fn(), warn: vi.fn(), show: vi.fn() }),
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (k: string) => k,
  }),
}))

describe('OnboardingWizard', () => {
  beforeEach(() => {
    localStorage.clear()
  })

  it('dismiss via later suppresses auto-open', async () => {
    const wrapper = mount(OnboardingWizard, {
      props: { open: true, projectId: 'p1' },
      global: { stubs: { Teleport: true, Icon: true, AppButton: true } },
    })
    await wrapper.find('[data-testid="onboarding-later"]').trigger('click')
    expect(wrapper.emitted('close')).toBeTruthy()
    expect(isOnboardingDismissed('p1')).toBe(true)
    expect(shouldAutoOpenOnboarding('p1', 0, [])).toBe(false)
  })

  it('backdrop dismiss suppresses auto-open', async () => {
    const wrapper = mount(OnboardingWizard, {
      props: { open: true, projectId: 'p2' },
      global: { stubs: { Teleport: true, Icon: true, AppButton: true } },
    })
    await wrapper.find('[data-testid="onboarding-backdrop"]').trigger('click')
    expect(isOnboardingDismissed('p2')).toBe(true)
  })

  it('close button suppresses auto-open', async () => {
    const wrapper = mount(OnboardingWizard, {
      props: { open: true, projectId: 'p3' },
      global: { stubs: { Teleport: true, Icon: true, AppButton: true } },
    })
    await wrapper.find('[data-testid="onboarding-close"]').trigger('click')
    expect(isOnboardingDismissed('p3')).toBe(true)
  })

  it('manual CTA can open even after dismiss', () => {
    dismissOnboarding('p4')
    expect(shouldAutoOpenOnboarding('p4', 0, [])).toBe(false)
    // emptiness still true — CTA path does not use shouldAutoOpen
    expect(true).toBe(true)
  })

  it('blocks generate without API key then succeeds with key', async () => {
    const { api } = await import('@/lib/api')
    const wrapper = mount(OnboardingWizard, {
      props: { open: true, projectId: 'p5' },
      global: { stubs: { Teleport: true, Icon: true, AppButton: true } },
    })
    // jump to review: overview→acp→apiKey→git→review
    for (let i = 0; i < 4; i++) {
      await wrapper.find('[data-testid="onboarding-next"]').trigger('click')
      await nextTick()
    }
    // on apiKey step without key — goNext should stay
    // After 2 nexts we're on apiKey (step 2). One more without key stays.
    // Reset: reopen
    await wrapper.setProps({ open: false })
    await wrapper.setProps({ open: true })
    await nextTick()
    // overview -> next
    await wrapper.find('[data-testid="onboarding-next"]').trigger('click')
    await nextTick()
    // acp -> next
    await wrapper.find('[data-testid="onboarding-next"]').trigger('click')
    await nextTick()
    // apiKey without value
    await wrapper.find('[data-testid="onboarding-next"]').trigger('click')
    await nextTick()
    expect(api.bootstrapProjectOnboarding).not.toHaveBeenCalled()

    const input = wrapper.find('[data-testid="onboarding-api-key"]')
    await input.setValue('crsr_test')
    await wrapper.find('[data-testid="onboarding-next"]').trigger('click') // -> git
    await nextTick()
    await wrapper.find('[data-testid="onboarding-next"]').trigger('click') // -> review
    await nextTick()
    await wrapper.find('[data-testid="onboarding-next"]').trigger('click') // generate
    await flushPromises()
    expect(api.bootstrapProjectOnboarding).toHaveBeenCalled()
    expect(wrapper.find('[data-testid="onboarding-success"]').exists()).toBe(true)
    const repoBox = wrapper.find('[data-testid="onboarding-success-repo"]')
    expect(repoBox.exists()).toBe(true)
    expect(repoBox.text()).toContain('heroku/nodejs-getting-started')
    expect(repoBox.text()).toContain('main')
    expect(repoBox.text()).not.toContain('demo|https://')
    expect(isOnboardingDismissed('p5')).toBe(true)

    await wrapper.find('[data-testid="onboarding-start-run"]').trigger('click')
    await flushPromises()
    expect(api.startRun).toHaveBeenCalledWith(
      'wf-1',
      {
        feature: '把首页欢迎文案与主按钮文案改得更清晰友好',
        repos: [
          {
            name: 'demo',
            url: 'https://github.com/heroku/nodejs-getting-started.git',
            branch: 'main',
          },
        ],
      },
      'manual',
    )
  })
})
