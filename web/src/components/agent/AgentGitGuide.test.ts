// @vitest-environment happy-dom
import { defineComponent } from 'vue'
import { createI18n } from 'vue-i18n'
import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'
import AgentGitGuide from './AgentGitGuide.vue'

const AppModalStub = defineComponent({
  props: { open: Boolean },
  template: `
    <div v-if="open" data-test="modal">
      <slot name="header" />
      <slot />
      <slot name="footer" />
    </div>
  `,
})

function mountGuide(env: { k: string; v: string }[], credentialType?: 'github_https' | 'gitlab_https' | 'ssh') {
  const i18n = createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...common, ...pages } },
  })
  return mount(AgentGitGuide, {
    props: {
      env,
      credentialType,
      upsertEnv: () => {},
    },
    global: {
      plugins: [i18n],
      stubs: {
        AppModal: AppModalStub,
        AppButton: { template: '<button><slot /></button>' },
      },
    },
  })
}

describe('AgentGitGuide', () => {
  it('未配置仓库时显示中性未启用状态', () => {
    const wrapper = mountGuide([])
    expect(wrapper.text()).toContain('Git 未启用')
    expect(wrapper.text()).not.toContain('凭据不完整')
  })

  it('完整状态不再内嵌静态检查长文，改为帮助入口', async () => {
    const wrapper = mountGuide([
      { k: 'GIT_REPOS', v: 'app|https://gitlab.com/acme/app.git|main' },
      { k: 'GITLAB_TOKEN', v: '${vars.gitlab_token}' },
    ])
    expect(wrapper.text()).toContain('配置形态完整')
    expect(wrapper.text()).not.toContain('不会验证变量引用的实际值')
    expect(wrapper.text()).not.toContain('远端 clone / push 权限')
    expect(wrapper.text()).not.toContain('可成功 clone')
    const help = wrapper.get('[data-test="git-help-link"]')
    expect(help.text()).toBe('帮助')
    expect(help.attributes('type')).toBe('button')
    await help.trigger('click')
    expect(wrapper.emitted('help')).toEqual([['git']])
  })

  it('运行时引用未选类型时给出可操作指引，无未逐仓解析告警', () => {
    const wrapper = mountGuide([
      { k: 'GIT_REPOS', v: '${vars.repos}' },
      { k: 'GITLAB_TOKEN', v: '${vars.gitlab_token}' },
    ])
    expect(wrapper.text()).toContain('请选择凭据类型')
    expect(wrapper.text()).toContain('GitHub / GitLab / SSH')
    expect(wrapper.text()).toContain('运行时解析')
    expect(wrapper.text()).not.toContain('本页不逐仓展开')
    expect(wrapper.text()).not.toContain('未逐仓解析')
    expect(wrapper.text()).not.toContain('无法在此页面逐仓解析')
  })

  it('确认类型后显示完整态与中性运行时解析，类型文案为 GitLab', async () => {
    const env = [
      { k: 'GIT_REPOS', v: '${vars.repos}' },
      { k: 'GITLAB_TOKEN', v: '${vars.gitlab_token}' },
    ]
    const wrapper = mountGuide(env)
    expect(wrapper.text()).toContain('请选择凭据类型')

    await wrapper.findAll('header button').find((b) => b.text() === '调整类型')!.trigger('click')
    const radios = wrapper.findAll('input[type="radio"]')
    await radios[1].setValue()
    const buttons = wrapper.findAll('[data-test="modal"] button')
    await buttons[buttons.length - 1].trigger('click')

    expect(wrapper.emitted('update:credentialType')).toEqual([['gitlab_https']])
    await wrapper.setProps({ credentialType: 'gitlab_https' })
    expect(wrapper.text()).toContain('配置形态完整')
    expect(wrapper.text()).toContain('已应用/待保存')
    expect(wrapper.text()).toContain('GitLab')
    expect(wrapper.text()).not.toContain('GitLab · HTTPS')
    expect(wrapper.text()).not.toContain('未逐仓解析')
    expect(wrapper.text()).toContain('运行时解析')
  })

  it('选择与仓库证据冲突时显示选择已失效', () => {
    const wrapper = mountGuide(
      [
        { k: 'GIT_REPOS', v: 'app|https://gitlab.com/acme/app.git|main' },
        { k: 'GITLAB_TOKEN', v: '${vars.gitlab_token}' },
      ],
      'github_https',
    )
    expect(wrapper.text()).toContain('需要确认凭据类型')
    expect(wrapper.text()).toContain('选择已失效')
    expect(wrapper.text()).not.toContain('已应用/待保存')
  })
})
