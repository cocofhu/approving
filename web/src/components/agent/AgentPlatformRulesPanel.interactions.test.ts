// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'
import AgentPlatformRulesPanel from './AgentPlatformRulesPanel.vue'

const mocks = vi.hoisted(() => ({
  list: vi.fn(),
  get: vi.fn(),
  save: vi.fn(),
  del: vi.fn(),
}))

vi.mock('@/lib/api/api', async () => {
  const actual = await vi.importActual<typeof import('@/lib/api/api')>('@/lib/api/api')
  return {
    ...actual,
    api: {
      ...actual.api,
      listAgentPlatformRules: mocks.list,
      getAgentPlatformRule: mocks.get,
      saveAgentPlatformRule: mocks.save,
      deleteAgentPlatformRule: mocks.del,
    },
  }
})

function mountPanel(props: Record<string, unknown> = {}) {
  const i18n = createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...common, ...pages } },
  })
  return mount(AgentPlatformRulesPanel, {
    props: { agentName: 'demo', active: true, ...props },
    global: {
      plugins: [i18n],
      stubs: {
        Icon: true,
        AppButton: { template: '<button type="button" v-bind="$attrs"><slot /></button>' },
        MarkdownSplitEditor: {
          props: ['modelValue', 'filePath', 'readonly'],
          emits: ['update:modelValue'],
          template: '<textarea data-testid="editor" :value="modelValue" @input="$emit(\'update:modelValue\', $event.target.value)" />',
        },
      },
    },
  })
}

describe('AgentPlatformRulesPanel interactions', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.list.mockResolvedValue({
      items: [
        { file: 'base.md', source: 'platform' },
        { file: 'custom.md', source: 'override' },
      ],
    })
    mocks.get.mockImplementation((_agent: string, file: string) =>
      Promise.resolve({ file, source: file === 'custom.md' ? 'override' : 'platform', content: `content:${file}` }),
    )
    mocks.save.mockImplementation((_agent: string, file: string, content: string) =>
      Promise.resolve({ file, source: 'override', content }),
    )
    mocks.del.mockResolvedValue({ status: 'deleted' })
  })

  it('loads, selects, creates, saves, and deletes overrides', async () => {
    const w = mountPanel()
    await flushPromises()
    expect(mocks.list).toHaveBeenCalledWith('demo')
    expect(mocks.get).toHaveBeenCalledWith('demo', 'base.md')
    expect(w.text()).toContain('base.md')
    expect(w.text()).toContain('1')

    const custom = w.findAll('aside button').find((b) => b.text().includes('custom.md'))!
    await custom.trigger('click')
    await flushPromises()
    expect(mocks.get).toHaveBeenLastCalledWith('demo', 'custom.md')

    await w.get('[data-testid="editor"]').setValue('changed')
    const save = w.findAll('button').find((b) => b.text().trim() === '保存')!
    await save.trigger('click')
    await flushPromises()
    expect(mocks.save).toHaveBeenCalledWith('demo', 'custom.md', 'changed')
    expect(w.emitted('toast')).toBeTruthy()

    const del = w.findAll('button').find((b) => b.text().includes('删除覆盖'))!
    await del.trigger('click')
    await flushPromises()
    expect(mocks.del).toHaveBeenCalledWith('demo', 'custom.md')

    await (w.vm as any).selectPlatformRuleFile('base.md')
    await flushPromises()
    await w.get('[data-testid="editor"]').setValue('new override')
    await (w.vm as any).createPlatformRuleOverride()
    await flushPromises()
    expect(mocks.save).toHaveBeenCalledWith('demo', 'base.md', 'new override')
    w.unmount()
  })

  it('surfaces load/select/write failures and honors guards and prop watches', async () => {
    mocks.list.mockRejectedValueOnce(new Error('list down'))
    const w = mountPanel()
    await flushPromises()
    expect(w.text()).toContain('list down')
    expect((w.vm as any).platformRuleLoading).toBe(false)

    mocks.list.mockResolvedValue({ items: [{ file: 'base.md', source: 'platform' }] })
    await (w.vm as any).loadPlatformRules()
    await flushPromises()
    mocks.get.mockRejectedValueOnce(new Error('read down'))
    await (w.vm as any).selectPlatformRuleFile('base.md')
    expect((w.vm as any).platformRuleError).toBe('read down')

    mocks.save.mockRejectedValueOnce(new Error('save down'))
    await (w.vm as any).createPlatformRuleOverride()
    expect((w.vm as any).platformRuleError).toBe('save down')

    ;(w.vm as any).platformRuleItems = [{ file: 'base.md', source: 'override' }]
    mocks.del.mockRejectedValueOnce(new Error('delete down'))
    await (w.vm as any).deletePlatformRuleOverride()
    expect((w.vm as any).platformRuleError).toBe('delete down')

    await w.setProps({ agentName: '', active: true })
    await flushPromises()
    expect((w.vm as any).platformRuleItems).toEqual([])
    await (w.vm as any).loadPlatformRules()
    await (w.vm as any).selectPlatformRuleFile('x')
    await (w.vm as any).createPlatformRuleOverride()
    expect(mocks.list).toHaveBeenCalledTimes(2)

    await w.setProps({ active: false, agentName: 'other' })
    await w.setProps({ active: true })
    await flushPromises()
    expect(mocks.list).toHaveBeenCalledWith('other')
    w.unmount()
  })
})
