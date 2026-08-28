export type GitCredentialType = 'github_https' | 'gitlab_https' | 'ssh'
export type GitCredentialStatus =
  | 'disabled'
  | 'complete'
  | 'incomplete'
  | 'needs_confirmation'
  | 'unsupported'
export type GitCredentialSource = 'auto' | 'user' | 'none'

export type GitEnv = Record<string, string> | { k: string; v: string }[]

export interface ParsedGitRepo {
  name: string
  url: string
  branch: string
  host: string
  protocol: 'https' | 'ssh' | 'unknown'
  inferredType?: GitCredentialType
  reason?: string
}

export interface GitCredentialIssue {
  repo?: string
  field?: string
  reason: string
}

export interface GitCredentialAnalysis {
  status: GitCredentialStatus
  effectiveType?: GitCredentialType
  source: GitCredentialSource
  repos: ParsedGitRepo[]
  missing: GitCredentialIssue[]
  conflicts: GitCredentialIssue[]
  unresolvedReference: boolean
  selectionValid: boolean
}

const VAR_REFERENCE_RE = /^\$\{vars\.[A-Za-z_][A-Za-z0-9_.-]*\}$/
const KNOWN_CREDENTIAL_TYPES = new Set<GitCredentialType>(['github_https', 'gitlab_https', 'ssh'])

function asCredentialType(value: string | undefined): GitCredentialType | undefined {
  if (!value) return undefined
  return KNOWN_CREDENTIAL_TYPES.has(value as GitCredentialType)
    ? (value as GitCredentialType)
    : undefined
}

function envRecord(env: GitEnv): Record<string, string> {
  if (!Array.isArray(env)) return env
  return Object.fromEntries(env.map(({ k, v }) => [k.trim(), v]))
}

function configured(value: string | undefined): boolean {
  const trimmed = value?.trim() ?? ''
  return trimmed !== '' && (!trimmed.startsWith('${') || VAR_REFERENCE_RE.test(trimmed))
}

function hostFromServiceURL(value: string | undefined): string {
  if (!configured(value)) return ''
  try {
    return new URL(value!.trim()).hostname.toLowerCase()
  } catch {
    return ''
  }
}

function parseRepoURL(raw: string): Pick<ParsedGitRepo, 'host' | 'protocol'> {
  const value = raw.trim()
  try {
    const parsed = new URL(value)
    if ((parsed.protocol === 'http:' || parsed.protocol === 'https:') && parsed.hostname) {
      return { host: parsed.hostname.toLowerCase(), protocol: 'https' }
    }
    if (parsed.protocol === 'ssh:' && parsed.hostname) {
      return { host: parsed.hostname.toLowerCase(), protocol: 'ssh' }
    }
  } catch {
    // Try the common SCP-like Git syntax below.
  }
  const scp = /^(?:[^@\s]+@)?([^:/\s]+):[^/].+$/.exec(value)
  if (scp) return { host: scp[1].toLowerCase(), protocol: 'ssh' }
  return { host: '', protocol: 'unknown' }
}

function parseGitRepos(value: string): ParsedGitRepo[] {
  return value
    .split(',')
    .map((item, index) => {
      const parts = item.split('|')
      const name = (parts[0] ?? '').trim() || `repo-${index + 1}`
      const url = (parts.length >= 2 ? parts[1] : parts[0] ?? '').trim()
      const branch = (parts.length >= 3 ? parts.slice(2).join('|') : '').trim()
      return { name, url, branch, ...parseRepoURL(url) }
    })
    .filter((repo) => repo.url !== '')
}

function inferRepoType(
  repo: ParsedGitRepo,
  githubHost: string,
  gitlabHost: string,
): ParsedGitRepo {
  if (repo.protocol === 'ssh') return { ...repo, inferredType: 'ssh' }
  if (repo.protocol !== 'https') {
    return { ...repo, reason: '仓库 URL 不是受支持的 HTTP(S) 或 SSH 格式' }
  }
  const github = repo.host === 'github.com' || (!!githubHost && repo.host === githubHost)
  const gitlab = repo.host === 'gitlab.com' || (!!gitlabHost && repo.host === gitlabHost)
  if (github && gitlab) return { ...repo, reason: '仓库主机同时匹配 GitHub 与 GitLab 服务地址' }
  if (github) return { ...repo, inferredType: 'github_https' }
  if (gitlab) return { ...repo, inferredType: 'gitlab_https' }
  return { ...repo, reason: '未知自建仓库主机，需要确认凭据类型' }
}

function validateSelection(
  type: GitCredentialType,
  repos: ParsedGitRepo[],
  githubHost: string,
  gitlabHost: string,
): GitCredentialIssue[] {
  const conflicts: GitCredentialIssue[] = []
  for (const repo of repos) {
    if (repo.inferredType && repo.inferredType !== type) {
      conflicts.push({
        repo: repo.name,
        reason: `仓库识别为 ${repo.inferredType}，与已选择的 ${type} 不一致`,
      })
      continue
    }
    if (type === 'ssh' && repo.protocol !== 'ssh') {
      conflicts.push({ repo: repo.name, reason: 'HTTPS 仓库不能使用 SSH 凭据类型' })
    } else if (type !== 'ssh' && repo.protocol !== 'https') {
      conflicts.push({ repo: repo.name, reason: 'SSH 仓库不能使用 HTTPS 凭据类型' })
    } else if (type === 'github_https' && githubHost && repo.host !== githubHost) {
      conflicts.push({
        repo: repo.name,
        field: 'GITHUB_URL',
        reason: '仓库主机与 GITHUB_URL 不匹配',
      })
    } else if (type === 'github_https' && repo.host !== 'github.com' && !githubHost) {
      conflicts.push({
        repo: repo.name,
        field: 'GITHUB_URL',
        reason: '自建 GitHub 仓库主机必须与 GITHUB_URL 匹配',
      })
    } else if (type === 'gitlab_https' && gitlabHost && repo.host !== gitlabHost) {
      conflicts.push({
        repo: repo.name,
        field: 'GITLAB_URL',
        reason: '仓库主机与 GITLAB_URL 不匹配',
      })
    }
  }
  return conflicts
}

function missingCredentials(
  type: GitCredentialType,
  repos: ParsedGitRepo[],
  env: Record<string, string>,
  unresolvedReference: boolean,
): GitCredentialIssue[] {
  const checks =
    type === 'github_https'
      ? ['GITHUB_TOKEN']
      : type === 'gitlab_https'
        ? ['GITLAB_TOKEN']
        : ['GIT_SSH_PRIVATE_KEY', 'GIT_SSH_KNOWN_HOSTS']
  const targets = unresolvedReference || repos.length === 0 ? [undefined] : repos
  return targets.flatMap((repo) =>
    checks
      .filter((field) => !configured(env[field]))
      .map((field) => ({
        repo: repo?.name,
        field,
        reason: `缺少 ${field}`,
      })),
  )
}

export function analyzeGitCredentials(
  input: { env: GitEnv; selectedType?: string },
): GitCredentialAnalysis {
  const env = envRecord(input.env)
  const selectedType = asCredentialType(input.selectedType)
  const unknownSelection = !!input.selectedType?.trim() && !selectedType
  const reposValue = env.GIT_REPOS?.trim() ?? ''
  const base = {
    repos: [] as ParsedGitRepo[],
    missing: [] as GitCredentialIssue[],
    conflicts: [] as GitCredentialIssue[],
    unresolvedReference: false,
    selectionValid: true,
  }
  if (!reposValue) return { ...base, status: 'disabled', source: 'none' }

  const unresolvedReference = VAR_REFERENCE_RE.test(reposValue)
  if (reposValue.startsWith('${') && !unresolvedReference) {
    return {
      ...base,
      status: 'unsupported',
      source: 'none',
      unresolvedReference: true,
      selectionValid: false,
      conflicts: [{ field: 'GIT_REPOS', reason: 'GIT_REPOS 变量引用语法无效' }],
    }
  }
  if (unresolvedReference) {
    // Runtime repo list refs (e.g. ${vars.repos}) are expected deferred resolution —
    // do not treat them as per-repo parse conflicts / dead-end warnings.
    if (!selectedType) {
      return {
        ...base,
        status: 'needs_confirmation',
        source: unknownSelection ? 'user' : 'none',
        unresolvedReference: true,
        selectionValid: false,
        conflicts: unknownSelection
          ? [{ reason: `未知凭据类型 ${input.selectedType}` }]
          : [],
      }
    }
    const missing = missingCredentials(selectedType, [], env, true)
    return {
      ...base,
      status: missing.length ? 'incomplete' : 'complete',
      effectiveType: selectedType,
      source: 'user',
      missing,
      unresolvedReference: true,
    }
  }

  const githubHost = hostFromServiceURL(env.GITHUB_URL)
  const gitlabHost = hostFromServiceURL(env.GITLAB_URL)
  const repos = parseGitRepos(reposValue).map((repo) =>
    inferRepoType(repo, githubHost, gitlabHost),
  )
  if (!repos.length) {
    return {
      ...base,
      status: 'unsupported',
      source: 'none',
      conflicts: [{ field: 'GIT_REPOS', reason: 'GIT_REPOS 未包含有效仓库' }],
    }
  }
  const invalid = repos.filter((repo) => repo.protocol === 'unknown')
  if (invalid.length) {
    return {
      ...base,
      repos,
      status: 'unsupported',
      source: 'none',
      conflicts: invalid.map((repo) => ({ repo: repo.name, reason: repo.reason! })),
    }
  }

  const inferredTypes = new Set(repos.flatMap((repo) => repo.inferredType ?? []))
  if (inferredTypes.size > 1) {
    return {
      ...base,
      repos,
      status: 'unsupported',
      source: 'none',
      selectionValid: false,
      conflicts: repos.map((repo) => ({
        repo: repo.name,
        reason: `仓库需要 ${repo.inferredType ?? '待确认'}，当前 Agent 仅支持一种全局凭据类型`,
      })),
    }
  }

  const unknown = repos.filter((repo) => !repo.inferredType)
  let effectiveType = selectedType
  let source: GitCredentialSource = selectedType ? 'user' : 'none'
  if (unknownSelection) {
    return {
      ...base,
      repos,
      status: 'needs_confirmation',
      source: 'user',
      selectionValid: false,
      conflicts: [{ reason: `未知凭据类型 ${input.selectedType}` }],
    }
  }
  if (!effectiveType && inferredTypes.size === 1 && unknown.length === 0) {
    effectiveType = [...inferredTypes][0]
    source = 'auto'
  }
  if (!effectiveType) {
    return {
      ...base,
      repos,
      status: 'needs_confirmation',
      source,
      selectionValid: false,
      conflicts: unknown.map((repo) => ({ repo: repo.name, reason: repo.reason! })),
    }
  }

  const conflicts = validateSelection(effectiveType, repos, githubHost, gitlabHost)
  if (conflicts.length) {
    return {
      ...base,
      repos,
      status: 'needs_confirmation',
      effectiveType,
      source,
      conflicts,
      selectionValid: false,
    }
  }

  if (effectiveType === 'gitlab_https' && !gitlabHost) {
    const hosts = new Set(repos.map((repo) => repo.host))
    if (hosts.size > 1) {
      return {
        ...base,
        repos,
        status: 'unsupported',
        effectiveType,
        source,
        conflicts: [{
          field: 'GITLAB_URL',
          reason: '多个 GitLab 主机且未显式配置 GITLAB_URL，运行时只能从首仓推导',
        }],
      }
    }
  }

  const missing = missingCredentials(effectiveType, repos, env, false)
  return {
    ...base,
    repos,
    status: missing.length ? 'incomplete' : 'complete',
    effectiveType,
    source,
    missing,
  }
}

export function isGitVariableReference(value: string): boolean {
  return VAR_REFERENCE_RE.test(value.trim())
}

/** Git Token keys that hide the connection-type guide (ACP keys do not count). */
export const GIT_VISIBILITY_TOKEN_KEYS = [
  'GITHUB_TOKEN',
  'GITLAB_TOKEN',
  'GIT_SSH_PRIVATE_KEY',
] as const

const GIT_TOKEN_TO_TYPE: Record<(typeof GIT_VISIBILITY_TOKEN_KEYS)[number], GitCredentialType> = {
  GITHUB_TOKEN: 'github_https',
  GITLAB_TOKEN: 'gitlab_https',
  GIT_SSH_PRIVATE_KEY: 'ssh',
}

/** True if any Git Token is configured in local and/or inherited env (union, not overwrite). */
export function hasConfiguredGitToken(...envs: Array<GitEnv | undefined>): boolean {
  for (const env of envs) {
    if (!env) continue
    const rec = envRecord(env)
    for (const key of GIT_VISIBILITY_TOKEN_KEYS) {
      if (configured(rec[key])) return true
    }
  }
  return false
}

/**
 * Infer a single connection type from configured Git Tokens across envs.
 * Returns undefined when none or more than one type is present.
 */
export function inferGitCredentialTypeFromTokens(
  ...envs: Array<GitEnv | undefined>
): GitCredentialType | undefined {
  const types = new Set<GitCredentialType>()
  for (const env of envs) {
    if (!env) continue
    const rec = envRecord(env)
    for (const key of GIT_VISIBILITY_TOKEN_KEYS) {
      if (configured(rec[key])) types.add(GIT_TOKEN_TO_TYPE[key])
    }
  }
  if (types.size === 1) return [...types][0]
  return undefined
}
