// @vitest-environment happy-dom
import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import AppSwitch from './AppSwitch.vue'

describe('AppSwitch', () => {
  it('renders square accent track when on', () => {
    const w = mount(AppSwitch, { props: { modelValue: true } })
    const btn = w.get('button[role="switch"]')
    expect(btn.attributes('aria-checked')).toBe('true')
    expect(btn.classes().join(' ')).toContain('bg-accent')
    expect(btn.classes().join(' ')).not.toContain('rounded-full')
    w.unmount()
  })

  it('renders off-state track when false', () => {
    const w = mount(AppSwitch, { props: { modelValue: false } })
    const btn = w.get('button[role="switch"]')
    expect(btn.attributes('aria-checked')).toBe('false')
    expect(btn.classes().join(' ')).toContain('bg-base')
    w.unmount()
  })

  it('emits update:modelValue on click and Space/Enter', async () => {
    const w = mount(AppSwitch, { props: { modelValue: false } })
    await w.get('button[role="switch"]').trigger('click')
    expect(w.emitted('update:modelValue')?.[0]).toEqual([true])

    await w.get('button[role="switch"]').trigger('keydown', { key: ' ' })
    expect(w.emitted('update:modelValue')?.[1]).toEqual([true])

    await w.get('button[role="switch"]').trigger('keydown', { key: 'Enter' })
    expect(w.emitted('update:modelValue')?.[2]).toEqual([true])
    w.unmount()
  })

  it('does not toggle when disabled', async () => {
    const w = mount(AppSwitch, { props: { modelValue: false, disabled: true } })
    const btn = w.get('button[role="switch"]')
    expect(btn.attributes('disabled')).toBeDefined()
    await btn.trigger('click')
    expect(w.emitted('update:modelValue')).toBeUndefined()
    w.unmount()
  })
})
