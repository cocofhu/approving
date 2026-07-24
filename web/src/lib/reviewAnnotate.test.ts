// @vitest-environment happy-dom
import { describe, expect, it } from 'vitest'
import { defineComponent, h, nextTick } from 'vue'
import { mount } from '@vue/test-utils'
import { provideReviewAnnotate, useReviewAnnotate } from './reviewAnnotate'

describe('reviewAnnotate', () => {
  it('injects null outside a provider', () => {
    const Child = defineComponent({
      setup() {
        return () => h('span', useReviewAnnotate() == null ? 'none' : 'has')
      },
    })
    const w = mount(Child)
    expect(w.text()).toBe('none')
  })

  it('provides annotate channel to descendants', async () => {
    const picked: string[] = []
    const quotes: string[] = []
    const Child = defineComponent({
      setup() {
        const api = useReviewAnnotate()
        return () =>
          h(
            'button',
            {
              onClick: () => {
                api?.annotate({ jsonPath: 'a.b', label: 'A' })
                api?.annotate({ jsonPath: 'summary', quote: 'excerpt', label: '概述' })
              },
            },
            api?.enabled ? 'on' : 'off',
          )
      },
    })
    const Parent = defineComponent({
      setup() {
        provideReviewAnnotate({
          enabled: true,
          annotate: (ann) => {
            if (ann.quote) quotes.push(ann.quote)
            else if (ann.jsonPath) picked.push(ann.jsonPath)
          },
        })
        return () => h(Child)
      },
    })
    const w = mount(Parent)
    expect(w.text()).toBe('on')
    await w.get('button').trigger('click')
    await nextTick()
    expect(picked).toEqual(['a.b'])
    expect(quotes).toEqual(['excerpt'])
  })
})
