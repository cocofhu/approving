// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'
import type { LiveProbeReport, LiveStatus, SettingItem } from '@/lib/api'

const mocks = vi.hoisted(() => ({
  getSettings: vi.fn(),
  updateSettings: vi.fn(),
  testLiveEndpoint: vi.fn(),
  liveStatus: vi.fn(),
  listSandboxes: vi.fn(),
  dashboard: vi.fn(),
}))

vi.mock('@/lib/api', async () => {
  const actual = await vi.importActual<typeof import('@/lib/api')>('@/lib/api')
  return {
    ...actual,
    api: {
      ...actual.api,
      getSettings: mocks.getSettings,
      updateSettings: mocks.updateSettings,
      testLiveEndpoint: mocks.testLiveEndpoint,
      liveStatus: mocks.liveStatus,
      listSandboxes: mocks.listSandboxes,
      dashboard: mocks.dashboard,
    },
  }
})

vi.mock('vue-router', () => ({
  useRouter: () => ({ push: vi.fn() }),
  useRoute: () => ({ query: {} }),
}))

vi.mock('@/lib/useAuth', () => ({
  useAuth: () => ({ user: { value: { isAdmin: true } } }),
}))

function settingsFixture(): SettingItem[] {
  const str = (key: string, value: string): SettingItem => ({
    key,
    label: key,
    kind: 'string',
    value,
    min: 0,
    source: 'db',
    locked: false,
  })
  return [
    { key: 'max_concurrent_runs', label: '', kind: 'int', value: 5, min: 1, source: 'config', locked: false },
    { key: 'run_sandbox_ttl_minutes', label: '', kind: 'int', value: 30, min: 1, source: 'config', locked: false },
    { key: 'test_sandbox_ttl_minutes', label: '', kind: 'int', value: 10, min: 1, source: 'config', locked: false },
    { key: 'max_test_sandboxes', label: '', kind: 'int', value: 2, min: 1, source: 'config', locked: false },
    str('live_base_url', 'http://192.168.2.20:8080/v1'),
    str('live_model', 'holo-3.1-35b-a3b'),
    { key: 'live_api_key', label: '', kind: 'secret', value: '****', min: 0, source: 'db', locked: false },
    { key: 'live_timeout_seconds', label: '', kind: 'int', value: 30, min: 1, source: 'db', locked: false },
  ]
}

function liveStatusFixture(over: Partial<LiveStatus['stats']> = {}, configured = true): LiveStatus {
  return {
    configured,
    stats: { calls: 0, failed: 0, avgLatencyMs: 0, lastLatencyMs: 0, ...over },
  }
}

async function mountSettings() {
  const i18n = createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...common, ...pages } },
  })
  const SettingsView = (await import('@/views/SettingsView.vue')).default
  const wrapper = mount(SettingsView, {
    global: { plugins: [i18n], stubs: { RouterLink: true } },
  })
  await flushPromises()
  return wrapper
}

beforeEach(() => {
  vi.clearAllMocks()
  mocks.getSettings.mockResolvedValue({ items: settingsFixture() })
  mocks.updateSettings.mockResolvedValue({ items: settingsFixture() })
  mocks.liveStatus.mockResolvedValue(liveStatusFixture())
  mocks.listSandboxes.mockResolvedValue([])
  mocks.dashboard.mockResolvedValue({ running: 0 })
})

describe('conversation model endpoint test', () => {
  it('tests the values in the form so a fix can be checked before saving', async () => {
    mocks.testLiveEndpoint.mockResolvedValue({
      configured: true,
      ok: true,
      latencyMs: 420,
      checks: [
        { name: 'reachable', ok: true },
        { name: 'tool_calls', ok: true },
      ],
      sample: 'ok',
    } satisfies LiveProbeReport)

    const wrapper = await mountSettings()
    const input = wrapper.findAll('input[type="text"]')[0]
    await input.setValue('http://10.0.0.9:8080/v1')

    const button = wrapper.findAll('button').find((b) => b.text().includes('测试'))!
    await button.trigger('click')
    await flushPromises()

    // The edited value is what got tested, and a blank secret is omitted so the
    // server falls back to the stored key.
    const patch = mocks.testLiveEndpoint.mock.calls[0][0]
    expect(patch.live_base_url).toBe('http://10.0.0.9:8080/v1')
    expect(patch.live_api_key).toBe('')

    expect(wrapper.text()).toContain('可用')
    expect(wrapper.text()).toContain('420')
  })

  it('names the failing check and shows the reason the server gave', async () => {
    mocks.testLiveEndpoint.mockResolvedValue({
      configured: true,
      ok: false,
      latencyMs: 310,
      checks: [
        { name: 'reachable', ok: true },
        { name: 'tool_calls', ok: false, reason: '这个模型没有发出工具调用，只回了文字。' },
      ],
    } satisfies LiveProbeReport)

    const wrapper = await mountSettings()
    await wrapper
      .findAll('button')
      .find((b) => b.text().includes('测试'))!
      .trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('不可用')
    expect(wrapper.text()).toContain('工具调用')
    expect(wrapper.text()).toContain('没有发出工具调用')
  })

  // A pass that outlives the values it was measured against reads as a pass for
  // whatever is on screen now, which is worse than showing nothing.
  it('drops a stale result when the endpoint is edited again', async () => {
    mocks.testLiveEndpoint.mockResolvedValue({
      configured: true,
      ok: true,
      latencyMs: 100,
      checks: [{ name: 'reachable', ok: true }],
    } satisfies LiveProbeReport)

    const wrapper = await mountSettings()
    await wrapper
      .findAll('button')
      .find((b) => b.text().includes('测试'))!
      .trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('可用')

    await wrapper.findAll('input[type="text"]')[0].setValue('http://changed/v1')
    await flushPromises()
    expect(wrapper.text()).not.toContain('往返')
  })
})

describe('conversation model runtime status', () => {
  // This is the question the test button cannot answer, and the one that went
  // unanswered while every message quietly went to a sandbox instead.
  it('warns when a working configuration has never actually been called', async () => {
    mocks.liveStatus.mockResolvedValue(liveStatusFixture({ calls: 0 }))
    const wrapper = await mountSettings()
    expect(wrapper.text()).toContain('一次都没被调用过')
  })

  it('reports call counts and the most recent failure once there is traffic', async () => {
    mocks.liveStatus.mockResolvedValue(
      liveStatusFixture({
        calls: 42,
        failed: 3,
        avgLatencyMs: 380,
        lastFailure: '端点拒绝了密钥（HTTP 401）。',
      }),
    )
    const wrapper = await mountSettings()
    const text = wrapper.text()
    expect(text).not.toContain('一次都没被调用过')
    expect(text).toContain('42')
    expect(text).toContain('380')
    expect(text).toContain('HTTP 401')
  })

  it('stays silent rather than erroring when status is unavailable', async () => {
    mocks.liveStatus.mockRejectedValue(new Error('boom'))
    const wrapper = await mountSettings()
    expect(wrapper.text()).not.toContain('boom')
    expect(wrapper.text()).toContain('运行状况')
  })
})
