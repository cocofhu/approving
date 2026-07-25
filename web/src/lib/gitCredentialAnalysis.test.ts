import { describe, expect, it } from 'vitest'
import {
  analyzeGitCredentials,
  isGitVariableReference,
  type GitCredentialStatus,
  type GitCredentialType,
} from './gitCredentialAnalysis'

type Case = {
  name: string
  env: Record<string, string>
  selectedType?: GitCredentialType
  status: GitCredentialStatus
  type?: GitCredentialType
  unresolved?: boolean
}

describe('analyzeGitCredentials', () => {
  const cases: Case[] = [
    { name: 'Git 未启用', env: {}, status: 'disabled' },
    {
      name: 'GitHub HTTPS',
      env: { GIT_REPOS: 'app|https://github.com/acme/app.git|main', GITHUB_TOKEN: 'token' },
      status: 'complete',
      type: 'github_https',
    },
    {
      name: 'GitLab HTTPS 从官方仓库推导地址',
      env: { GIT_REPOS: 'app|https://gitlab.com/acme/app.git|main', GITLAB_TOKEN: 'token' },
      status: 'complete',
      type: 'gitlab_https',
    },
    {
      name: '自建 GitLab 由显式服务地址识别',
      env: {
        GIT_REPOS: 'app|https://git.example.com/acme/app.git|main',
        GITLAB_URL: 'https://git.example.com',
        GITLAB_TOKEN: '${vars.gitlab_token}',
      },
      status: 'complete',
      type: 'gitlab_https',
    },
    {
      name: '自建 GitHub 由显式服务地址识别',
      env: {
        GIT_REPOS: 'app|https://github.example.com/acme/app.git|main',
        GITHUB_URL: 'https://github.example.com',
        GITHUB_TOKEN: '${vars.github_token}',
      },
      status: 'complete',
      type: 'github_https',
    },
    {
      name: 'SSH',
      env: {
        GIT_REPOS: 'app|git@github.com:acme/app.git|main',
        GIT_SSH_PRIVATE_KEY: '${vars.deploy_key}',
        GIT_SSH_KNOWN_HOSTS: '${vars.known_hosts}',
      },
      status: 'complete',
      type: 'ssh',
    },
    {
      name: '未知自建域名需要确认',
      env: {
        GIT_REPOS: 'app|https://code.example.com/acme/app.git|main',
        GITLAB_TOKEN: 'token',
      },
      status: 'needs_confirmation',
    },
    {
      name: '混合 provider 当前不支持',
      env: {
        GIT_REPOS: [
          'one|https://github.com/acme/one.git|main',
          'two|https://gitlab.com/acme/two.git|main',
        ].join(','),
        GITHUB_TOKEN: 'token',
        GITLAB_TOKEN: 'token',
      },
      status: 'unsupported',
    },
    {
      name: '混合协议当前不支持',
      env: {
        GIT_REPOS: [
          'one|https://github.com/acme/one.git|main',
          'two|git@github.com:acme/two.git|main',
        ].join(','),
        GITHUB_TOKEN: 'token',
      },
      status: 'unsupported',
    },
    {
      name: '运行时引用无选择时待确认类型（非死胡同冲突）',
      env: { GIT_REPOS: '${vars.repos}', GITLAB_TOKEN: '${vars.gitlab_token}' },
      status: 'needs_confirmation',
      unresolved: true,
    },
    {
      name: '运行时引用有选择时按字段形态校验',
      env: { GIT_REPOS: '${vars.repos}', GITLAB_TOKEN: '${vars.gitlab_token}' },
      selectedType: 'gitlab_https',
      status: 'complete',
      type: 'gitlab_https',
      unresolved: true,
    },
    {
      name: '无效变量引用不受支持',
      env: { GIT_REPOS: '${vars.}', GITHUB_TOKEN: 'token' },
      status: 'unsupported',
      unresolved: true,
    },
    {
      name: '不支持的 URL',
      env: { GIT_REPOS: 'app|file:///tmp/app|main' },
      status: 'unsupported',
    },
  ]

  it.each(cases)('$name', ({ env, selectedType, status, type, unresolved }) => {
    const result = analyzeGitCredentials({ env, selectedType })
    expect(result.status).toBe(status)
    expect(result.effectiveType).toBe(type)
    expect(result.unresolvedReference).toBe(unresolved ?? false)
  })

  it('运行时引用未选类型时不写入逐仓解析死胡同 conflicts', () => {
    const result = analyzeGitCredentials({
      env: { GIT_REPOS: '${vars.repos}', GITLAB_TOKEN: '${vars.gitlab_token}' },
    })
    expect(result.status).toBe('needs_confirmation')
    expect(result.unresolvedReference).toBe(true)
    expect(result.conflicts).toEqual([])
    expect(JSON.stringify(result)).not.toContain('逐仓解析')
  })

  it('同类型多仓全部校验且不只看首仓', () => {
    const result = analyzeGitCredentials({
      env: {
        GIT_REPOS: [
          'one|https://github.com/acme/one.git|main',
          'broken|not-a-repository-url|main',
        ].join(','),
        GITHUB_TOKEN: 'token',
      },
    })
    expect(result.status).toBe('unsupported')
    expect(result.conflicts[0].repo).toBe('broken')
  })

  it('缺凭据时列出每个受影响仓库和字段', () => {
    const result = analyzeGitCredentials({
      env: {
        GIT_REPOS: [
          'one|https://gitlab.com/acme/one.git|main',
          'two|https://gitlab.com/acme/two.git|main',
        ].join(','),
      },
    })
    expect(result.status).toBe('incomplete')
    expect(result.missing).toEqual([
      { repo: 'one', field: 'GITLAB_TOKEN', reason: '缺少 GITLAB_TOKEN' },
      { repo: 'two', field: 'GITLAB_TOKEN', reason: '缺少 GITLAB_TOKEN' },
    ])
  })

  it('已保存选择与仓库证据冲突时保留类型但标记失效', () => {
    const result = analyzeGitCredentials({
      env: {
        GIT_REPOS: 'app|https://gitlab.com/acme/app.git|main',
        GITLAB_TOKEN: 'token',
      },
      selectedType: 'github_https',
    })
    expect(result.status).toBe('needs_confirmation')
    expect(result.effectiveType).toBe('github_https')
    expect(result.selectionValid).toBe(false)
    expect(result.conflicts[0].repo).toBe('app')
  })

  it('未知 selectedType 降级为需要确认', () => {
    const result = analyzeGitCredentials({
      env: {
        GIT_REPOS: 'app|https://gitlab.com/acme/app.git|main',
        GITLAB_TOKEN: 'token',
      },
      selectedType: 'gitea_https',
    })
    expect(result.status).toBe('needs_confirmation')
    expect(result.selectionValid).toBe(false)
    expect(result.source).toBe('user')
    expect(result.effectiveType).toBeUndefined()
    expect(result.conflicts[0].reason).toContain('未知凭据类型')
  })

  it('人工确认未知自建 GitLab 后可由仓库 URL 推导 host', () => {
    const result = analyzeGitCredentials({
      env: {
        GIT_REPOS: 'app|https://code.example.com/acme/app.git|main',
        GITLAB_TOKEN: 'token',
      },
      selectedType: 'gitlab_https',
    })
    expect(result.status).toBe('complete')
    expect(result.source).toBe('user')
  })

  it('自建 GitHub 人工选择仍要求 GITHUB_URL 与仓库 host 匹配', () => {
    const result = analyzeGitCredentials({
      env: {
        GIT_REPOS: 'app|https://code.example.com/acme/app.git|main',
        GITHUB_TOKEN: 'token',
      },
      selectedType: 'github_https',
    })
    expect(result.status).toBe('needs_confirmation')
    expect(result.conflicts[0].field).toBe('GITHUB_URL')
  })

  it('显式 GitLab 服务地址与仓库 host 冲突时要求重新确认', () => {
    const result = analyzeGitCredentials({
      env: {
        GIT_REPOS: 'app|https://gitlab.com/acme/app.git|main',
        GITLAB_URL: 'https://gitlab.example.com',
        GITLAB_TOKEN: 'token',
      },
    })
    expect(result.status).toBe('needs_confirmation')
    expect(result.conflicts[0].field).toBe('GITLAB_URL')
  })

  it('多个自建 GitLab host 无显式地址时标记当前不支持', () => {
    const result = analyzeGitCredentials({
      env: {
        GIT_REPOS: [
          'one|https://git.one.example/acme/one.git|main',
          'two|https://git.two.example/acme/two.git|main',
        ].join(','),
        GITLAB_TOKEN: 'token',
      },
      selectedType: 'gitlab_https',
    })
    expect(result.status).toBe('unsupported')
    expect(result.conflicts[0].field).toBe('GITLAB_URL')
  })
})

describe('isGitVariableReference', () => {
  it('只接受完整且非空的 vars 引用', () => {
    expect(isGitVariableReference('${vars.repos}')).toBe(true)
    expect(isGitVariableReference('${vars.gitlab.token}')).toBe(true)
    expect(isGitVariableReference('${vars.}')).toBe(false)
    expect(isGitVariableReference('${secrets.token}')).toBe(false)
  })
})
