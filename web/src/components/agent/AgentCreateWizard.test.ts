// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { mount } from '@vue/test-utils'
import { afterEach, describe, expect, it } from 'vitest'
import commonZh from '@/locales/zh-CN/common.json'
import pagesZh from '@/locales/zh-CN/pages.json'
import commonEn from '@/locales/en/common.json'
import pagesEn from '@/locales/en/pages.json'
import AgentCreateWizard from './AgentCreateWizard.vue'

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

afterEach(() => {
  document.body.innerHTML = ''
})

describe('AgentCreateWizard region UI', () => {
  it('defaults CodeBuddy to international and renders its managed ENV row read-only', async () => {
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

    buttonByText('下一步').click()
    await wrapper.vm.$nextTick()
    buttonByText('跳过').click()
    await wrapper.vm.$nextTick()

    const key = document.body.querySelector(
      'input[aria-label="ACP 管理的区域变量"]',
    ) as HTMLInputElement
    const value = document.body.querySelector(
      'input[aria-label="ACP 管理的区域值"]',
    ) as HTMLInputElement
    expect(key.readOnly).toBe(true)
    expect(key.value).toBe('APPROVING_CODEBUDDY_REGION')
    expect(value.value).toBe('public')
    wrapper.unmount()
  })

  it('renders equivalent English site semantics and accessible radio names', async () => {
    const wrapper = mountWizard('en')
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
