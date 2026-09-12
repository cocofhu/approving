// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'
import AgentGitGuide from './AgentGitGuide.vue'
import type { GitCredentialType } from '@/lib/agent/gitCredentialAnalysis'

const DIAGNOSTIC_TERMS = [
  '调整类型',
  '尚未选择',
  '待确认连接方式',
  '运行时解析',
  '配置形态完整',
  '静态识别凭据类型',
]

function mountGuide(
  env: { k: string; v: string }[],
  opts: {
    credentialType?: GitCredentialType
    allowTokenRecommend?: boolean
    inheritedEnv?: { k: string; v: string }[] | Record<string, string>
    upsertEnv?: (key: string, value: string) => void
  } = {},
) {
  const i18n = createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...common, ...pages } },
  })
  return mount(AgentGitGuide, {
    props: {
      env,
      credentialType: opts.credentialType,
      upsertEnv: opts.upsertEnv ?? (() => {}),
      allowTokenRecommend: opts.allowTokenRecommend,
      inheritedEnv: opts.inheritedEnv,
    },
    global: { plugins: [i18n] },
  })
}

describe('AgentGitGuide', () => {
  it('无 Token 时平铺 GitHub / GitLab / SSH，点击立即写入类型（g1.1 / plan g3.1）', async () => {
    const wrapper = mountGuide([])
    expect(wrapper.find('[data-test="git-guide"]').exists()).toBe(true)
    expect(wrapper.get('[data-test="git-choice-github_https"]').text()).toContain('GitHub')
    expect(wrapper.get('[data-test="git-choice-gitlab_https"]').text()).toContain('GitLab')
    expect(wrapper.get('[data-test="git-choice-ssh"]').text()).toContain('SSH')
    expect(wrapper.text()).not.toContain('调整类型')
    expect(wrapper.find('input[type="radio"]').exists()).toBe(false)

    await wrapper.get('[data-test="git-choice-gitlab_https"]').trigger('click')
    expect(wrapper.emitted('update:credentialType')).toEqual([['gitlab_https']])

    await wrapper.setProps({ credentialType: 'gitlab_https' })
    expect(wrapper.get('[data-test="git-choice-gitlab_https"]').attributes('aria-pressed')).toBe(
      'true',
    )
  })

  it('首屏不含诊断词，帮助入口仍可打开（g1.4）', async () => {
    const wrapper = mountGuide([])
    for (const term of DIAGNOSTIC_TERMS) {
      expect(wrapper.text(), term).not.toContain(term)
    }
    const help = wrapper.get('[data-test="git-help-link"]')
    expect(help.text()).toBe('帮助')
    await help.trigger('click')
    expect(wrapper.emitted('help')).toEqual([['git']])
  })

  it('本地任一 Git Token 仍渲染三选并可改选（plan g1.1 / g1.2 / g3.2）', async () => {
    const wrapper = mountGuide([{ k: 'GITLAB_TOKEN', v: '${vars.gitlab_pat}' }])
    expect(wrapper.find('[data-test="git-guide"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="git-choice-github_https"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="git-choice-gitlab_https"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="git-choice-ssh"]').exists()).toBe(true)
    expect(wrapper.emitted('update:credentialType')).toEqual([['gitlab_https']])

    await wrapper.get('[data-test="git-choice-ssh"]').trigger('click')
    expect(wrapper.emitted('update:credentialType')).toEqual([['gitlab_https'], ['ssh']])
  })

  it('仅继承 Token 时仍渲染引导并预选（plan g1.2 / g3.2）', () => {
    const wrapper = mountGuide([], {
      inheritedEnv: { GITLAB_TOKEN: '${vars.gitlab_pat}' },
    })
    expect(wrapper.find('[data-test="git-guide"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="git-choice-gitlab_https"]').exists()).toBe(true)
    expect(wrapper.emitted('update:credentialType')).toEqual([['gitlab_https']])
  })

  it('仅 ACP API Key、没有 Git Token 时仍显示三选（plan g3.1）', () => {
    const wrapper = mountGuide([
      { k: 'GRASP_CURSOR_API_KEY', v: 'sk-test' },
      { k: 'CURSOR_API_KEY', v: 'sk-alt' },
    ])
    expect(wrapper.find('[data-test="git-guide"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="git-choice-github_https"]').exists()).toBe(true)
    expect(wrapper.emitted('update:credentialType')).toBeUndefined()
  })

  it('仅一种 Token 且无类型则预选对应卡片（plan g1.2）', () => {
    const wrapper = mountGuide([], {
      inheritedEnv: { GITLAB_TOKEN: '${vars.gitlab_pat}' },
    })
    expect(wrapper.find('[data-test="git-guide"]').exists()).toBe(true)
    expect(wrapper.emitted('update:credentialType')).toEqual([['gitlab_https']])
  })

  it('用户已选类型不被 Token 推断覆盖（plan g1.2）', () => {
    const wrapper = mountGuide([{ k: 'GITHUB_TOKEN', v: 'ghp-x' }], {
      credentialType: 'ssh',
    })
    expect(wrapper.find('[data-test="git-guide"]').exists()).toBe(true)
    expect(wrapper.get('[data-test="git-choice-ssh"]').attributes('aria-pressed')).toBe('true')
    expect(wrapper.emitted('update:credentialType')).toBeUndefined()
  })

  it('多类 Git Token 且无已选类型时仍可见且不擅自预选（plan g1.2）', () => {
    const wrapper = mountGuide([
      { k: 'GITHUB_TOKEN', v: 'ghp-x' },
      { k: 'GITLAB_TOKEN', v: 'glpat-x' },
    ])
    expect(wrapper.find('[data-test="git-guide"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="git-choice-github_https"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="git-choice-gitlab_https"]').exists()).toBe(true)
    expect(wrapper.emitted('update:credentialType')).toBeUndefined()
  })

  it('Studio 点选类型不写入 Git Token，也不出现补推荐变量（plan g2.1 / g2.2）', async () => {
    const added: string[] = []
    const wrapper = mountGuide([], {
      allowTokenRecommend: false,
      upsertEnv: (key) => {
        added.push(key)
      },
    })
    await wrapper.get('[data-test="git-choice-gitlab_https"]').trigger('click')
    expect(wrapper.emitted('update:credentialType')).toEqual([['gitlab_https']])
    expect(wrapper.find('[data-test="git-apply-recommended"]').exists()).toBe(false)
    expect(added).toEqual([])
  })

  it('共享面无 Token 时可选补推荐变量，写入对应 Token 键', async () => {
    const added: string[] = []
    const wrapper = mountGuide([], {
      credentialType: 'gitlab_https',
      allowTokenRecommend: true,
      upsertEnv: (key) => {
        added.push(key)
      },
    })
    await wrapper.get('[data-test="git-apply-recommended"]').trigger('click')
    expect(added).toContain('GIT_REPOS')
    expect(added).toContain('GITLAB_TOKEN')
  })

  it('向导文案强调并存不互斥，不含「点一个即可」（g1.3 / review v3）', () => {
    const wrapper = mountGuide([])
    const text = wrapper.text()
    expect(text).toContain('并存')
    expect(text).toContain('不互斥')
    expect(text).not.toContain('点一个即可')
  })
})
