// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import commonZh from '@/locales/zh-CN/common.json'
import pagesZh from '@/locales/zh-CN/pages.json'
import commonEn from '@/locales/en/common.json'
import pagesEn from '@/locales/en/pages.json'
import AgentCreateWizard from './AgentCreateWizard.vue'
import { WIZARD_STEPS } from '@/lib/agentCreateWizard'

const createAgent = vi.fn(async (payload: unknown) => payload)
vi.mock('@/lib/api', () => ({
  api: {
    createAgent: (payload: unknown) => createAgent(payload),
  },
}))

function mountWizard(locale: 'zh-CN' | 'en' = 'zh-CN') {
  const i18n = createI18n({
    legacy: false,
    locale,
    messages: {
      'zh-CN': { ...commonZh, ...pagesZh },
      en: { ...commonEn, ...pagesEn },
    },
  })
  return mount(AgentCreateWizard, {
    attachTo: document.body,
    props: { open: true, existingNames: [] },
    global: { plugins: [i18n] },
  })
}

function buttonByText(text: string) {
  const button = Array.from(document.body.querySelectorAll('button')).find(
    (item) => item.textContent?.trim().startsWith(text),
  )
  if (!button) throw new Error(`button not found: ${text}`)
  return button
}

function fillName(value: string) {
  const input = document.body.querySelector('#wiz-name-input') as HTMLInputElement
  input.value = value
  input.dispatchEvent(new Event('input', { bubbles: true }))
}

function railLabels(): string[] {
  return Array.from(document.body.querySelectorAll('.rail-item .lbl strong')).map(
    (el) => el.textContent?.trim() || '',
  )
}

afterEach(() => {
  document.body.innerHTML = ''
})

describe('AgentCreateWizard 5-step IA', () => {
  it('exposes exactly 5 wizard steps in model order', () => {
    expect(WIZARD_STEPS.map((s) => s.id)).toEqual(['basics', 'acp', 'apiKey', 'git', 'review'])
  })

  it('renders sidebar without ENV/ACP/capability steps and labels Agent', async () => {
    const wrapper = mountWizard()
    const labels = railLabels()
    expect(labels).toEqual(['基础信息', 'Agent', 'API Key', 'Git', '确认创建'])
    expect(labels.join(' ')).not.toContain('ACP')
    expect(labels.join(' ')).not.toContain('ENV')
    expect(labels.join(' ')).not.toMatch(/MCP|Rules|Skills|Commands|Prompts/)
    wrapper.unmount()
  })

  it('defaults CodeBuddy to international on the Agent step', async () => {
    const wrapper = mountWizard()
    fillName('region-agent')
    await wrapper.vm.$nextTick()
    buttonByText('下一步').click()
    await wrapper.vm.$nextTick()

    buttonByText('CodeBuddy').click()
    await wrapper.vm.$nextTick()
    const international = document.body.querySelector(
      'button[role="radio"][aria-label="国际站 (public)"]',
    )
    expect(international?.getAttribute('aria-checked')).toBe('true')
    expect(document.body.textContent).toContain('Agent')
    expect(document.body.textContent).not.toMatch(/配置步骤[\s\S]*\bACP\b/)
    wrapper.unmount()
  })

  it('shows API Key apply guide for current backend and allows skip', async () => {
    const wrapper = mountWizard()
    fillName('key-agent')
    await wrapper.vm.$nextTick()
    buttonByText('下一步').click()
    await wrapper.vm.$nextTick()
    buttonByText('Cursor').click()
    await wrapper.vm.$nextTick()
    buttonByText('下一步').click()
    await wrapper.vm.$nextTick()

    expect(document.body.textContent).toContain('APPROVING_CURSOR_API_KEY')
    expect(document.body.textContent).toContain('CURSOR_API_KEY')
    expect(document.body.textContent).toContain('Cursor Dashboard')
    const dash = Array.from(document.body.querySelectorAll('a')).find((a) =>
      a.href.includes('cursor.com/dashboard'),
    )
    expect(dash).toBeTruthy()

    buttonByText('跳过').click()
    await wrapper.vm.$nextTick()
    expect(document.body.textContent).toContain('Git')
    wrapper.unmount()
  })

  it('review page shows non-blocking auth reminder when API Key skipped', async () => {
    const wrapper = mountWizard()
    fillName('skip-agent')
    await wrapper.vm.$nextTick()
    buttonByText('下一步').click()
    await wrapper.vm.$nextTick()
    buttonByText('下一步').click()
    await wrapper.vm.$nextTick()
    buttonByText('跳过').click()
    await wrapper.vm.$nextTick()
    buttonByText('跳过').click()
    await wrapper.vm.$nextTick()

    expect(document.body.textContent).toContain('确认创建')
    expect(document.body.textContent).toContain('Studio Env')
    expect(document.body.textContent).toContain('鉴权提醒')
    expect(railLabels()).not.toContain('ENV')
    expect(railLabels()).not.toContain('MCP')
    expect(railLabels()).toContain('Agent')
    expect(railLabels()).not.toContain('ACP')

    buttonByText('创建并进入 Studio').click()
    await wrapper.vm.$nextTick()
    await vi.waitFor(() => {
      expect(createAgent).toHaveBeenCalled()
    })
    wrapper.unmount()
  })

  it('renders equivalent English site semantics and Agent step label', async () => {
    const wrapper = mountWizard('en')
    expect(railLabels()).toEqual(['Basics', 'Agent', 'API Key', 'Git', 'Confirm'])
    fillName('region-agent')
    await wrapper.vm.$nextTick()
    buttonByText('Next').click()
    await wrapper.vm.$nextTick()
    buttonByText('Trae').click()
    await wrapper.vm.$nextTick()

    const international = document.body.querySelector(
      'button[role="radio"][aria-label="International (intl)"]',
    )
    expect(international?.getAttribute('aria-checked')).toBe('true')
    expect(document.body.textContent).toContain('www.trae.ai · intl')
    wrapper.unmount()
  })
})
