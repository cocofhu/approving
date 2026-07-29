// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'
import TokenUsageByModelTip from './TokenUsageByModelTip.vue'

const usage = {
  inputTokens: 10,
  outputTokens: 5,
  cacheReadTokens: 2,
  cacheWriteTokens: 1,
}

const usageByModel = {
  'claude-sonnet-4': {
    inputTokens: 8,
    outputTokens: 4,
    cacheReadTokens: 1,
    cacheWriteTokens: 0,
    source: 'upstream',
    filled: false,
  },
  default: {
    inputTokens: 2,
    outputTokens: 1,
    cacheReadTokens: 1,
    cacheWriteTokens: 1,
    source: 'bridge',
    filled: true,
  },
}

function mountTip(props: Record<string, unknown> = {}) {
  const i18n = createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...common, ...pages } },
  })
  return mount(TokenUsageByModelTip, {
    props: { usage, usageByModel, ...props },
    global: { plugins: [i18n] },
  })
}

describe('TokenUsageByModelTip', () => {
  it('omitted open stays undefined (uncontrolled), click expands tip', async () => {
    const wrapper = mountTip()
    // Regression: Vue Boolean casting must not force open===false when omitted.
    expect(wrapper.props('open')).toBeUndefined()
    expect(wrapper.find('[data-testid="token-by-model-tip"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="token-by-model-trigger"]').attributes('aria-expanded')).toBe(
      'false',
    )

    await wrapper.get('[data-testid="token-by-model-trigger"]').trigger('click')

    expect(wrapper.find('[data-testid="token-by-model-tip"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="token-by-model-trigger"]').attributes('aria-expanded')).toBe(
      'true',
    )
    expect(wrapper.find('[data-model="claude-sonnet-4"]').exists()).toBe(true)
    expect(wrapper.find('[data-filled="1"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('click again closes tip in uncontrolled mode', async () => {
    const wrapper = mountTip()
    await wrapper.get('[data-testid="token-by-model-trigger"]').trigger('click')
    expect(wrapper.find('[data-testid="token-by-model-tip"]').exists()).toBe(true)
    await wrapper.get('[data-testid="token-by-model-close"]').trigger('click')
    expect(wrapper.find('[data-testid="token-by-model-tip"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('controlled open:true shows tip; open:false keeps closed on click (parent owns state)', async () => {
    const wrapper = mountTip({ open: true })
    expect(wrapper.find('[data-testid="token-by-model-tip"]').exists()).toBe(true)

    await wrapper.setProps({ open: false })
    expect(wrapper.find('[data-testid="token-by-model-tip"]').exists()).toBe(false)

    await wrapper.get('[data-testid="token-by-model-trigger"]').trigger('click')
    // Still closed until parent updates open; emit notifies parent.
    expect(wrapper.find('[data-testid="token-by-model-tip"]').exists()).toBe(false)
    const openEmits = wrapper.emitted('update:open')
    expect(openEmits?.[openEmits.length - 1]).toEqual([true])
    wrapper.unmount()
  })
})
