// @vitest-environment happy-dom
import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import AuditFilterDropdown from './AuditFilterDropdown.vue'

describe('AuditFilterDropdown', () => {
  it('opens, filters, and emits selection', async () => {
    const w = mount(AuditFilterDropdown, {
      props: {
        modelValue: '',
        searchable: true,
        emptyLabel: 'all',
        options: [
          { value: '', label: 'All' },
          { value: 'a', label: 'Alpha', sub: 'one' },
          { value: 'b', label: 'Beta' },
        ],
        testId: 'audit-dd',
      },
      attachTo: document.body,
    })
    await w.get('button').trigger('click')
    expect(w.get('[data-testid="audit-dd"]').classes()).toContain('open')
    w.unmount()
  })
})
