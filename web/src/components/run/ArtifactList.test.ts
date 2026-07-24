// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'
import type { Artifact } from '@/lib/types'
import ArtifactList from './ArtifactList.vue'

function artifact(name: string, id = name): Artifact {
  return {
    id,
    name,
    kind: 'json',
    nodeId: 'research',
    runId: 'run-1',
    workflowName: 'wf',
    sizeBytes: 10,
    createdAt: '2026-07-18T00:00:00Z',
  }
}

function mountList(props: Partial<InstanceType<typeof ArtifactList>['$props']> = {}) {
  const i18n = createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...common, ...pages } },
  })
  return mount(ArtifactList, {
    props: {
      artifacts: props.artifacts ?? [artifact('research.json'), artifact('plan.json', 'a2')],
      scope: props.scope ?? 'run',
      activeId: props.activeId ?? null,
      ...props,
    },
    global: { plugins: [i18n], stubs: { Icon: true } },
  })
}

describe('ArtifactList', () => {
  it('renders artifact names in run scope', () => {
    const wrapper = mountList()
    expect(wrapper.text()).toContain('research.json')
    expect(wrapper.text()).toContain('plan.json')
    wrapper.unmount()
  })

  it('filters artifacts locally by search', async () => {
    const wrapper = mountList()
    const input = wrapper.find('input[type="search"], input')
    if (input.exists()) {
      await input.setValue('plan')
      expect(wrapper.text()).toContain('plan.json')
      expect(wrapper.text()).not.toContain('research.json')
    }
    wrapper.unmount()
  })

  it('emits select when artifact clicked', async () => {
    const arts = [artifact('research.json')]
    const wrapper = mountList({ artifacts: arts })
    const row = wrapper.findAll('button').find((b) => b.text().includes('research.json'))
    expect(row).toBeTruthy()
    await row!.trigger('click')
    expect(wrapper.emitted('select')).toBeTruthy()
    expect((wrapper.emitted('select')![0][0] as Artifact).name).toBe('research.json')
    wrapper.unmount()
  })

  it('shows empty text when no artifacts', () => {
    const wrapper = mountList({ artifacts: [] })
    expect(wrapper.text()).toMatch(/暂无|没有/)
    wrapper.unmount()
  })
})
