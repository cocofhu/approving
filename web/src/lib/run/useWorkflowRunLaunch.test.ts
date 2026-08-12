// @vitest-environment happy-dom
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createI18n } from 'vue-i18n'
import { createApp, defineComponent } from 'vue'
import common from '@/locales/zh-CN/common.json'
import { useWorkflowRunLaunch } from './useWorkflowRunLaunch'
import type { Workflow } from '@/lib/shared/types'

vi.mock('@/lib/run/runDraft', () => ({
  mergeRunDraft: (_id: string, seed: Record<string, string>, images: Record<string, unknown>) => ({
    inputs: seed,
    images,
    restored: false,
  }),
  saveRunDraft: vi.fn(() => 'ok'),
  clearRunDraft: vi.fn(),
}))

function withSetup<T>(fn: () => T): T {
  let result!: T
  const i18n = createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': common },
  })
  const Comp = defineComponent({
    setup() {
      result = fn()
      return () => null
    },
  })
  const app = createApp(Comp)
  app.use(i18n)
  app.mount(document.createElement('div'))
  return result
}

const baseWf = (over: Partial<Workflow> = {}): Workflow => ({
  id: 'wf-1',
  projectId: 'p1',
  name: '夜间回归',
  description: '',
  status: 'draft',
  version: 1,
  updatedAt: '',
  needsRepo: false,
  nodes: [
    {
      id: 'n1',
      type: 'input',
      label: '输入',
      config: {
        variables: [
          { name: 'branch', ask: true, type: 'string', value: 'main', required: true },
        ],
      },
    } as any,
  ],
  edges: [],
  ...over,
})

beforeEach(() => {
  const launch = withSetup(() => useWorkflowRunLaunch())
  launch.closeLaunch()
})

describe('useWorkflowRunLaunch', () => {
  it('openLaunch pre-fills ask fields and opens the modal without changing route', () => {
    const launch = withSetup(() => useWorkflowRunLaunch())
    launch.openLaunch(baseWf())
    expect(launch.open.value).toBe(true)
    expect(launch.target.value?.id).toBe('wf-1')
    expect(launch.runFields.value.map((f) => f.key)).toEqual(['branch'])
    expect(launch.runInputs.value.branch).toBe('main')
  })

  it('openLaunch still opens when there are no ask fields', () => {
    const launch = withSetup(() => useWorkflowRunLaunch())
    launch.openLaunch(
      baseWf({
        nodes: [{ id: 'n1', type: 'input', label: '输入', config: { variables: [] } } as any],
      }),
    )
    expect(launch.open.value).toBe(true)
    expect(launch.runFields.value).toEqual([])
  })
})
