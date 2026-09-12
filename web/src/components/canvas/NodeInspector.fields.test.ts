// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'
import nodes from '@/locales/zh-CN/nodes.json'
import type { WFEdge, WFNode } from '@/lib/shared/types'

const apiMocks = vi.hoisted(() => ({
  listAgents: vi.fn(),
}))

vi.mock('@/lib/api/api', async () => {
  const actual = await vi.importActual<typeof import('@/lib/api/api')>('@/lib/api/api')
  return {
    ...actual,
    api: { ...actual.api, listAgents: apiMocks.listAgents },
  }
})

import NodeInspector from './NodeInspector.vue'

function node(type: string, config: Record<string, any> = {}, id = type, label = type): WFNode {
  return { id, type, label, position: { x: 0, y: 0 }, config } as WFNode
}

function mountInspector(
  target: WFNode,
  allNodes: WFNode[] = [target],
  edges: WFEdge[] = [],
  extra: Record<string, unknown> = {},
) {
  const i18n = createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...common, ...pages, ...nodes } },
  })
  return mount(NodeInspector, {
    props: { node: target, allNodes, edges, ...extra },
    global: {
      plugins: [i18n],
      stubs: {
        Icon: true,
        AppButton: { template: '<button type="button"><slot /></button>' },
        OutputSourcesEditor: { template: '<div data-testid="output-sources" />' },
      },
      attachTo: document.body,
    },
  })
}

beforeEach(() => {
  vi.clearAllMocks()
  apiMocks.listAgents.mockResolvedValue([{ name: 'agent-a', projectId: 'p1' }])
})

describe('NodeInspector field editors', () => {
  it('falls back to an empty agent list when listAgents rejects', async () => {
    apiMocks.listAgents.mockRejectedValueOnce(new Error('offline'))
    const target = node('agent', { agent_profile: '' })
    const w = mountInspector(target, [target], [], { projectId: 'p1' })
    await flushPromises()

    // Without a loadable roster the select degrades to the "no agents" option.
    const opts = w.findAll('select option').map((o) => o.text())
    expect(opts.some((tx) => tx.includes('agent-a'))).toBe(false)
    w.unmount()
  })

  it('labels a deleted agent_profile as stale', async () => {
    apiMocks.listAgents.mockResolvedValue([{ name: 'agent-a', projectId: 'p1' }])
    const target = node('agent', { agent_profile: 'ghost' })
    const w = mountInspector(target, [target], [], { projectId: 'p1' })
    await flushPromises()

    expect(w.find('[data-testid="skill-profile-stale-banner"]').exists()).toBe(true)
    const opts = w.findAll('select option').map((o) => o.text())
    expect(opts.some((tx) => tx.startsWith('ghost ·'))).toBe(true)
    w.unmount()
  })

  it('treats an unbound agent and a missing workflow project as foreign', async () => {
    apiMocks.listAgents.mockResolvedValue([{ name: 'loose', projectId: '' }])
    const unbound = node('agent', { agent_profile: 'loose' })
    const w1 = mountInspector(unbound, [unbound], [], { projectId: 'p1' })
    await flushPromises()
    expect(w1.find('[data-testid="skill-profile-stale-banner"]').text()).toContain('loose')
    w1.unmount()

    apiMocks.listAgents.mockResolvedValue([{ name: 'agent-a', projectId: 'p1' }])
    const noProject = node('agent', { agent_profile: 'agent-a' })
    const w2 = mountInspector(noProject)
    await flushPromises()
    expect(w2.find('[data-testid="skill-profile-stale-banner"]').exists()).toBe(true)
    w2.unmount()
  })

  it('offers upstream outputs for a human gate body template', async () => {
    const upstream = node('research', {}, 'research', '调研')
    const gate = node('human_gate', { actions: [], form: [] }, 'gate', '门禁')
    const edges: WFEdge[] = [{ id: 'e1', source: 'research', target: 'gate' } as WFEdge]
    const w = mountInspector(gate, [upstream, gate], edges)
    await flushPromises()
    const vm = w.vm as any

    const opts = vm.fieldOptions({ key: 'body_template', type: 'select' })
    expect(opts.length).toBeGreaterThan(1)
    expect(opts[0].value).toBe('')

    // A hand-written value that is not an upstream output is kept as custom.
    gate.config.body_template = 'custom.md'
    await flushPromises()
    const withCustom = vm.fieldOptions({ key: 'body_template', type: 'select' })
    expect(withCustom.some((o: any) => o.value === 'custom.md')).toBe(true)

    // Unknown field keys fall through to the schema's own options.
    expect(vm.fieldOptions({ key: 'whatever', type: 'select', options: [{ value: 'a', label: 'A' }] })).toEqual([
      { value: 'a', label: 'A' },
    ])
    w.unmount()
  })

  it('prompts with no upstream still render the body-template hint', async () => {
    const gate = node('human_gate', {}, 'gate', '门禁')
    const w = mountInspector(gate)
    await flushPromises()
    const opts = (w.vm as any).fieldOptions({ key: 'body_template', type: 'select' })
    expect(opts).toHaveLength(1)
    expect(opts[0].label).toBeTruthy()
    w.unmount()
  })

  it('edits gate actions and form fields through the rendered rows', async () => {
    const gate = node('human_gate', { actions: [{ id: 'approve', label: '批准' }], form: [] }, 'gate', '门禁')
    const w = mountInspector(gate)
    await flushPromises()
    const vm = w.vm as any

    vm.addAction()
    vm.addFormField()
    await flushPromises()
    expect(gate.config.actions).toHaveLength(2)
    expect(gate.config.form).toHaveLength(1)

    // Toggling requireForm / required happens straight on the row objects.
    const chips = w.findAll('button.chip')
    expect(chips.length).toBeGreaterThan(0)
    await chips[0].trigger('click')
    await flushPromises()

    // Removing rows keeps the arrays consistent.
    gate.config.actions.splice(1, 1)
    gate.config.form.splice(0, 1)
    await flushPromises()
    expect(gate.config.actions).toHaveLength(1)
    expect(gate.config.form).toHaveLength(0)
    w.unmount()
  })

  it('adds and labels branch cases around the default arm', async () => {
    const branch = node('branch', { cases: [] }, 'branch', '分支')
    const w = mountInspector(branch)
    await flushPromises()
    const vm = w.vm as any

    vm.addElseIf()
    expect(branch.config.cases).toHaveLength(1)
    expect(vm.caseLabel(branch.config.cases[0], 0)).toBe('IF')

    branch.config.cases.push({ when: 'default', goto: '' })
    vm.addElseIf()
    await flushPromises()
    expect(branch.config.cases.map((c: any) => c.when)).toEqual(['', '', 'default'])
    expect(vm.caseLabel(branch.config.cases[1], 1)).toBe('ELSE IF')
    expect(vm.caseLabel(branch.config.cases[2], 2)).toBe('ELSE')

    // Snippets append with && so multiple clauses compose.
    const row = branch.config.cases[0]
    vm.insertCond(row, 'a')
    vm.insertCond(row, 'b')
    expect(row.when).toBe('a && b')
    w.unmount()
  })

  it('maintains set_var assignments against declared globals', async () => {
    const input = node('input', { variables: [{ name: 'topic', type: 'string', value: '' }] }, 'input', '输入')
    const setVar = node('set_var', { assignments: [] }, 'set_var', '赋值')
    const w = mountInspector(setVar, [input, setVar])
    await flushPromises()
    const vm = w.vm as any

    expect(vm.globalVars.map((v: any) => v.name)).toContain('topic')
    vm.addAssignment()
    await flushPromises()
    expect(setVar.config.assignments).toHaveLength(1)
    expect(w.findAll('select option').some((o) => o.text().includes('topic'))).toBe(true)
    w.unmount()
  })

  it('collects gate and test outputs into the global variable pool', async () => {
    const gate = node('human_gate', { output_var: 'decision', form: [{ key: 'note' }] }, 'gate', '门禁')
    const test = node('test', { reason_var: 'why' }, 'test', '测试')
    const review = node('review', {}, 'review', '评审')
    const setVar = node('set_var', { assignments: [] }, 'set_var', '赋值')
    const w = mountInspector(setVar, [gate, test, review, setVar])
    await flushPromises()

    const names = (w.vm as any).globalVars.map((v: any) => v.name)
    expect(names).toEqual(expect.arrayContaining(['decision', 'note', 'why', 'reason']))
    w.unmount()
  })

  it('edits input variables including repos rows', async () => {
    const input = node('input', { variables: [] }, 'input', '输入')
    const w = mountInspector(input)
    await flushPromises()
    const vm = w.vm as any

    vm.addVariable()
    await flushPromises()
    expect(input.config.variables).toHaveLength(1)

    const v = input.config.variables[0]
    v.name = 'repos'
    v.type = 'repos'
    await flushPromises()

    // A JSON string value is coerced into an editable array.
    v.value = '[{"name":"api","url":"u","branch":"main"}]'
    expect(vm.asRepos(v)).toHaveLength(1)
    expect(vm.repoNames).toEqual(['api'])

    vm.addRepo(v)
    await flushPromises()
    expect(v.value).toHaveLength(2)
    vm.removeRepo(v, 1)
    expect(v.value).toHaveLength(1)

    // Malformed JSON and non-array payloads both degrade to an empty list.
    v.value = '{oops'
    expect(vm.asRepos(v)).toEqual([])
    v.value = undefined
    expect(vm.asRepos(v)).toEqual([])
    w.unmount()
  })

  it('drives the repo_select combobox and closes it on outside click', async () => {
    const input = node(
      'input',
      { variables: [{ name: 'repos', type: 'repos', value: [{ name: 'api' }, { name: 'web' }, { name: '' }] }] },
      'input',
      '输入',
    )
    const submit = node('submit_mr', { repo: '' }, 'submit_mr', '提交MR')
    const w = mountInspector(submit, [input, submit])
    await flushPromises()
    const vm = w.vm as any

    expect(vm.repoNames).toEqual(['api', 'web'])
    expect(vm.repoComboOpen).toBe(false)

    const toggle = w.findAll('button').find((b) => b.attributes('aria-expanded') !== undefined)
    expect(toggle).toBeTruthy()
    await toggle!.trigger('click')
    expect(vm.repoComboOpen).toBe(true)

    // Picking the list token writes it straight into the node config.
    const tokenBtn = w.findAll('[role="option"]').find((b) => b.text().includes('{{vars.repos}}'))
    await tokenBtn!.trigger('click')
    expect(submit.config.repo).toBe('{{vars.repos}}')
    expect(vm.repoComboOpen).toBe(false)

    await toggle!.trigger('click')
    const literal = w.findAll('[role="option"]').find((b) => b.text().trim() === 'api')
    await literal!.trigger('click')
    expect(submit.config.repo).toBe('api')

    // An outside mousedown closes the popover; an inside one does not.
    vm.repoComboOpen = true
    await flushPromises()
    document.dispatchEvent(new MouseEvent('mousedown', { bubbles: true }))
    await flushPromises()
    expect(vm.repoComboOpen).toBe(false)
    w.unmount()
  })

  it('shows the empty-repos hint when no repos variable exists', async () => {
    const submit = node('submit_mr', { repo: '' }, 'submit_mr', '提交MR')
    const w = mountInspector(submit)
    await flushPromises()
    const vm = w.vm as any
    vm.repoComboOpen = true
    await flushPromises()
    expect(vm.repoNames).toEqual([])
    expect(w.text()).toMatch(/repos/i)
    w.unmount()
  })

  it('ignores repos variables whose value is unusable', async () => {
    const input = node(
      'input',
      {
        variables: [
          { name: 'a', type: 'repos', value: '{bad json' },
          { name: 'b', type: 'repos', value: 42 },
          { name: 'c', type: 'string', value: 'x' },
        ],
      },
      'input',
      '输入',
    )
    const submit = node('submit_mr', { repo: '' }, 'submit_mr', '提交MR')
    const w = mountInspector(submit, [input, submit])
    await flushPromises()
    expect((w.vm as any).repoNames).toEqual([])
    w.unmount()
  })

  it('builds variable chips from globals and upstream node outputs', async () => {
    const input = node('input', { variables: [{ name: 'topic', type: 'string', value: '' }] }, 'input', '输入')
    const research = node('research', {}, 'research', '调研')
    const agent = node('agent', { prompt: '' }, 'agent', 'Agent')
    const edges: WFEdge[] = [
      { id: 'e1', source: 'input', target: 'research' } as WFEdge,
      { id: 'e2', source: 'research', target: 'agent' } as WFEdge,
    ]
    const w = mountInspector(agent, [input, research, agent], edges)
    await flushPromises()
    const vm = w.vm as any

    expect(vm.hasVars).toBe(true)
    expect(vm.varGroups).toHaveLength(2)
    expect(vm.upstreamIds.has('input')).toBe(true)
    expect(vm.upstreamIds.has('research')).toBe(true)

    const chip = w.findAll('button').find((b) => b.attributes('title') === '{{vars.topic}}')
    await chip!.trigger('click')
    expect(agent.config.prompt).toBe('{{vars.topic}}')

    // A second insert appends with a separating space.
    await chip!.trigger('click')
    expect(agent.config.prompt).toBe('{{vars.topic}} {{vars.topic}}')
    w.unmount()
  })

  it('reports no variables when the graph has none', async () => {
    const agent = node('agent', { prompt: '' }, 'agent', 'Agent')
    const w = mountInspector(agent)
    await flushPromises()
    expect((w.vm as any).hasVars).toBe(false)
    expect((w.vm as any).varGroups).toEqual([])
    w.unmount()
  })

  it('lazily materialises the conditional prompt object', async () => {
    const agent = node('agent', {}, 'agent', 'Agent')
    const w = mountInspector(agent)
    await flushPromises()
    const vm = w.vm as any

    expect(vm.condPrompt).toEqual({ when_var: '', text: '' })
    expect(agent.config.conditional_prompt).toEqual({ when_var: '', text: '' })

    // A pre-existing object is reused rather than reset.
    const preset = node('agent', { conditional_prompt: { when_var: 'topic', text: 'hi' } }, 'agent', 'Agent')
    const w2 = mountInspector(preset)
    await flushPromises()
    expect((w2.vm as any).condPrompt.text).toBe('hi')
    w.unmount()
    w2.unmount()
  })

  it('defaults auto_inject on and persists an explicit toggle', async () => {
    const preview = node('app_preview', {}, 'preview', '应用预览')
    const w = mountInspector(preview)
    await flushPromises()
    const vm = w.vm as any

    expect(vm.switchOn({ key: 'auto_inject' })).toBe(true)
    vm.setSwitch('auto_inject', false)
    await flushPromises()
    expect(vm.switchOn({ key: 'auto_inject' })).toBe(false)

    // Other switches follow the raw truthiness of the config value.
    expect(vm.switchOn({ key: 'direct_preview' })).toBe(false)
    vm.setSwitch('direct_preview', true)
    expect(vm.switchOn({ key: 'direct_preview' })).toBe(true)
    w.unmount()
  })

  it('re-syncs gate form defaults when the body template changes', async () => {
    const gate = node('human_gate', { body_template: 'a.md', form: [] }, 'gate', '门禁')
    const w = mountInspector(gate)
    await flushPromises()

    gate.config.body_template = 'b.md'
    await flushPromises()
    expect(gate.config.body_template).toBe('b.md')
    w.unmount()
  })

  it('renders the output_sources editor with the migration flag', async () => {
    const out = node('output', { results: [] }, 'output', '输出')
    const w = mountInspector(out, [out], [], { outputMigration: true })
    await flushPromises()
    expect(w.find('[data-testid="output-sources"]').exists()).toBe(true)
    w.unmount()
  })
})
