// @vitest-environment happy-dom
/**
 * Smoke-mount Demo「入口只装配」抽离的 Run* 面板壳，计入 coverage 分母。
 */
import { createI18n } from 'vue-i18n'
import { flushPromises, mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'
import type { Gate, NodeRun, Run, WFNode } from '@/lib/shared/types'
import RunGatePanel from './RunGatePanel.vue'
import RunLogPanel from './RunLogPanel.vue'
import RunSandboxPanel from './RunSandboxPanel.vue'
import RunClarifyPanel from './RunClarifyPanel.vue'
import RunOutputPanel from './RunOutputPanel.vue'
import RunPreviewPanel from './RunPreviewPanel.vue'
import RunProductPanel from './RunProductPanel.vue'
import RunReviewPanel from './RunReviewPanel.vue'

function i18n() {
  return createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...common, ...pages } },
  })
}

const stubNode = {
  id: 'n1',
  type: 'agent',
  label: 'Agent',
  position: { x: 0, y: 0 },
  config: {},
} as WFNode

const stubNodeRun = {
  nodeId: 'n1',
  status: 'completed',
} as NodeRun

const stubRun = {
  id: 'run-1',
  workflowId: 'wf-1',
  workflowName: 'wf',
  status: 'running',
  trigger: 'manual',
  startedAt: '2026-08-12T00:00:00Z',
  durationSec: 10,
  progress: 50,
  nodeRuns: { n1: stubNodeRun },
  artifacts: [],
} as unknown as Run

const stubGate = {
  runId: 'run-1',
  nodeId: 'n1',
  workflowName: 'wf',
  title: 'Gate',
  bodyMd: 'ok?',
  actions: [{ id: 'approve', label: '通过' }],
  requestedAt: '2026-08-12T00:00:00Z',
} as Gate

const heavyStubs = {
  GateApproval: true,
  LiveLogPanel: true,
  ClarifyChat: true,
  ClarifyBootLoader: true,
  ReviewShell: true,
  ReviewComposer: true,
  AppPreviewPanel: true,
  StructuredProductPanel: true,
  NodeOutputPanel: true,
  Icon: true,
  RefreshStrip: true,
  HardLoadLayer: true,
  StatusPill: true,
}

describe('Run detail panel shells (Demo entry assembly)', () => {
  it('mounts gate/log/sandbox/clarify/output/preview/product/review shells', async () => {
    const plugins = [i18n()]
    const global = { plugins, stubs: heavyStubs }

    const gate = mount(RunGatePanel, {
      props: { gate: stubGate, run: stubRun, submitError: null },
      global,
    })
    expect(gate.exists()).toBe(true)
    ;(gate.vm as any).applyReviewFrame?.({ type: 'x' })
    gate.unmount()

    const log = mount(RunLogPanel, {
      props: { events: [], live: false, status: 'running' },
      global,
    })
    expect(log.exists()).toBe(true)
    log.unmount()

    const sandbox = mount(RunSandboxPanel, {
      props: {
        loading: false,
        sbxLog: { content: 'hello', live: true, found: true },
        selStatus: 'running',
      },
      global,
    })
    expect(sandbox.text()).toMatch(/沙箱|log|Log/i)
    sandbox.unmount()

    const clarify = mount(RunClarifyPanel, {
      props: {
        sandboxFailed: false,
        nodeLabel: '澄清',
        nodeId: 'c1',
        clarify: { nodeId: 'c1', turns: [], done: false },
        runId: 'run-1',
        draft: '',
        attachments: [],
        inputActive: true,
        selStatus: 'waiting_human',
      },
      global,
    })
    expect(clarify.exists()).toBe(true)
    clarify.unmount()

    const output = mount(RunOutputPanel, {
      props: { node: stubNode, nodeRun: stubNodeRun, run: stubRun },
      global,
    })
    expect(output.exists()).toBe(true)
    output.unmount()

    const preview = mount(RunPreviewPanel, {
      props: { runId: 'run-1', nodeId: 'n1' },
      global,
    })
    expect(preview.exists()).toBe(true)
    preview.unmount()

    const product = mount(RunProductPanel, {
      props: { node: stubNode, nodeRun: stubNodeRun, run: stubRun },
      global,
    })
    expect(product.exists()).toBe(true)
    product.unmount()

    const review = mount(RunReviewPanel, {
      props: {
        mobile: false,
        node: stubNode,
        nodeRun: stubNodeRun,
        run: stubRun,
        clarify: { nodeId: 'n1', turns: [], done: false },
        draft: '',
        attachments: [],
        annotations: [],
        inputActive: true,
        selStatus: 'waiting_human',
      },
      global,
    })
    expect(review.exists()).toBe(true)
    await flushPromises()
    review.unmount()
  })
})
