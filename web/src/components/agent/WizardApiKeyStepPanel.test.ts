// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import commonZh from '@/locales/zh-CN/common.json'
import pagesZh from '@/locales/zh-CN/pages.json'
import { authGuideFor } from '@/lib/agent/backendAuthGuide'
import WizardApiKeyStepPanel from './WizardApiKeyStepPanel.vue'

const CodeEditorStub = {
  name: 'CodeEditor',
  props: ['modelValue', 'language'],
  emits: ['update:modelValue'],
  template:
    '<textarea data-test="custom-config-editor" :value="modelValue" @input="$emit(\'update:modelValue\', $event.target.value)" />',
}

function mountPanel(overrides: Record<string, unknown> = {}) {
  const guide = authGuideFor('claude_code')
  const i18n = createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...commonZh, ...pagesZh } },
  })
  return mount(WizardApiKeyStepPanel, {
    props: {
      acpBackend: 'claude_code',
      configRoot: '/root/.claude',
      authMode: 'customConfig',
      apiKeyInput: '',
      customConfigContent: '{\n  "env": {}\n}',
      customConfigError: false,
      authGuide: guide,
      primaryAuthKey: guide.keys[0].key,
      primaryAuthAlt: guide.keys[0].alt ?? '',
      ...overrides,
    },
    global: {
      plugins: [i18n],
      stubs: { CodeEditor: CodeEditorStub },
    },
  })
}

describe('WizardApiKeyStepPanel custom config editor', () => {
  it('gives CodeEditor host an explicit height so Monaco can layout', () => {
    const wrapper = mountPanel()
    const host = wrapper.get('[data-test="custom-config-editor-host"]')
    expect(host.classes()).toContain('h-[220px]')
    expect(host.classes()).not.toContain('min-h-[220px]')
    wrapper.unmount()
  })

  it('shows custom config content and emits edits', async () => {
    const wrapper = mountPanel()
    const editor = wrapper.get('[data-test="custom-config-editor"]')
    expect((editor.element as HTMLTextAreaElement).value).toContain('"env"')

    await editor.setValue('{\n  "ok": true\n}')
    expect(wrapper.emitted('update:customConfigContent')?.at(-1)).toEqual(['{\n  "ok": true\n}'])
    wrapper.unmount()
  })

  it('shows invalid JSON error when customConfigError is set', () => {
    const wrapper = mountPanel({ customConfigError: true })
    expect(wrapper.get('[data-test="custom-config-error"]').text()).toContain('JSON')
    wrapper.unmount()
  })

  it('switches between auth modes without losing custom config section', async () => {
    const wrapper = mountPanel()
    expect(wrapper.find('[data-test="custom-config-editor-host"]').exists()).toBe(true)

    await wrapper.setProps({ authMode: 'apiKey' })
    expect(wrapper.find('[data-test="custom-config-editor-host"]').exists()).toBe(false)
    expect(wrapper.find('[data-test="api-key-input"]').exists()).toBe(true)

    await wrapper.setProps({ authMode: 'customConfig' })
    expect(wrapper.find('[data-test="custom-config-editor-host"]').exists()).toBe(true)
    wrapper.unmount()
  })
})
