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
  it('无 Token 时平铺 GitHub / GitLab / SSH，点击立即写入类型（g1.1）', async () => {
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

  it('本地任一 Git Token 隐藏整块引导（g1.2）', () => {
    const wrapper = mountGuide([{ k: 'GITLAB_TOKEN', v: '${vars.gitlab_pat}' }])
    expect(wrapper.find('[data-test="git-guide"]').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('GitHub')
    expect(wrapper.text()).not.toContain('添加推荐变量')
  })

  it('仅继承 Token 时隐藏引导；空本地不覆盖继承（g1.2 / g2.1）', () => {
    const wrapper = mountGuide([], {
      inheritedEnv: { GITLAB_TOKEN: '${vars.gitlab_pat}' },
    })
    expect(wrapper.find('[data-test="git-guide"]').exists()).toBe(false)
  })

  it('仅 ACP API Key、没有 Git Token 时仍显示三选（g1.2）', () => {
    const wrapper = mountGuide([
      { k: 'APPROVING_CURSOR_API_KEY', v: 'sk-test' },
      { k: 'CURSOR_API_KEY', v: 'sk-alt' },
    ])
    expect(wrapper.find('[data-test="git-guide"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="git-choice-github_https"]').exists()).toBe(true)
  })

  it('隐藏时仅 GITLAB_TOKEN 且无类型则推断为 GitLab（g1.3）', () => {
    const wrapper = mountGuide([], {
      inheritedEnv: { GITLAB_TOKEN: '${vars.gitlab_pat}' },
    })
    expect(wrapper.find('[data-test="git-guide"]').exists()).toBe(false)
    expect(wrapper.emitted('update:credentialType')).toEqual([['gitlab_https']])
  })

  it('隐藏时用户已选类型不被推断覆盖（g1.3）', () => {
    const wrapper = mountGuide([{ k: 'GITHUB_TOKEN', v: 'ghp-x' }], {
      credentialType: 'ssh',
    })
    expect(wrapper.find('[data-test="git-guide"]').exists()).toBe(false)
    expect(wrapper.emitted('update:credentialType')).toBeUndefined()
  })

  it('多类 Git Token 且无已选类型时仍隐藏且不改写（g1.3）', () => {
    const wrapper = mountGuide([
      { k: 'GITHUB_TOKEN', v: 'ghp-x' },
      { k: 'GITLAB_TOKEN', v: 'glpat-x' },
    ])
    expect(wrapper.find('[data-test="git-guide"]').exists()).toBe(false)
    expect(wrapper.emitted('update:credentialType')).toBeUndefined()
  })

  it('Studio 点选类型不写入 Git Token，也不出现补推荐变量（g1.2 / f5）', async () => {
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
})
