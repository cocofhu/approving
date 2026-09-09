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
