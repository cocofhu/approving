// @vitest-environment happy-dom
import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import ComposerShell from './ComposerShell.vue'

describe('ComposerShell', () => {
  it('stacks multiline input above a fixed-height toolbar (g1.1)', () => {
    const wrapper = mount(ComposerShell, {
      slots: {
        input: '<textarea data-testid="shell-ta">a\nb</textarea>',
        'toolbar-start': '<button data-testid="shell-attach" class="h-10 w-10">clip</button>',
        'toolbar-end': '<button data-testid="shell-send">send</button>',
        footer: '<button data-testid="shell-confirm" class="h-9">ok</button>',
      },
    })
    const box = wrapper.get('[data-testid="composer-shell-box"]')
    const toolbar = wrapper.get('[data-testid="composer-shell-toolbar"]')
    expect(box.find('[data-testid="shell-ta"]').exists()).toBe(true)
    expect(box.find('[data-testid="shell-send"]').exists()).toBe(true)
    expect(toolbar.classes()).toEqual(expect.arrayContaining(['h-11', 'shrink-0']))
    expect(wrapper.get('[data-testid="composer-shell-footer"]').find('[data-testid="shell-confirm"]').exists()).toBe(
      true,
    )
    expect(box.find('[data-testid="shell-confirm"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('compact trims the toolbar row for height-constrained hosts (g2.2)', () => {
    const wrapper = mount(ComposerShell, {
      props: { compact: true },
      slots: { input: '<textarea data-testid="shell-ta"></textarea>', footer: '<button class="h-9">ok</button>' },
    })
    const toolbar = wrapper.get('[data-testid="composer-shell-toolbar"]')
    expect(toolbar.classes()).toEqual(expect.arrayContaining(['h-9', 'shrink-0']))
    expect(toolbar.classes()).not.toContain('h-11')
    expect(wrapper.get('[data-testid="composer-shell-footer"]').classes()).toContain('mt-1.5')
    wrapper.unmount()
  })

  it('drops the hint column when no hint is supplied so the footer stays one row (g2.2)', () => {
    const withoutHint = mount(ComposerShell, {
      slots: { footer: '<button data-testid="shell-confirm" class="h-9">ok</button>' },
    })
    const withHint = mount(ComposerShell, {
      slots: { hint: '<p data-testid="shell-hint">hint</p>', footer: '<button class="h-9">ok</button>' },
    })
    expect(withoutHint.get('[data-testid="composer-shell-footer"]').element.children).toHaveLength(1)
    expect(withHint.find('[data-testid="shell-hint"]').exists()).toBe(true)
    expect(withHint.get('[data-testid="composer-shell-footer"]').element.children).toHaveLength(2)
    withoutHint.unmount()
    withHint.unmount()
  })

  it('can hide chrome for confirm-only cold path (g2.3)', () => {
    const wrapper = mount(ComposerShell, {
      props: { showChrome: false, showFooter: true },
      slots: { footer: '<button data-testid="shell-confirm" class="h-9">ok</button>' },
    })
    expect(wrapper.find('[data-testid="composer-shell-box"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="shell-confirm"]').exists()).toBe(true)
    wrapper.unmount()
  })
})
