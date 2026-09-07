import type { BackendId } from '@/lib/shared/regionPolicy'
import { getRegionPolicy } from '@/lib/shared/regionPolicy'
import type { GitCredentialType } from '@/lib/agent/gitCredentialAnalysis'
import type { AppLocale } from '@/lib/shared/loadLocaleMessages'

/** Matches models.DefaultProjectID — first-install wizard only opens here. */
export const DEFAULT_PROJECT_ID = 'proj-default'

export const ONBOARDING_WORKFLOW_NAME = '默认工作流'
export const FIRST_INSTALL_GROUP_NAME = '综合项目组'

export const ONBOARDING_AGENT_NAMES = [
  '综合AI技术产品',
  '综合研发工程师',
  '综合测试工程师',
  '综合代码审查工程师',
  '综合运维工程师',
  '综合项目组组长',
] as const

export type OnboardingStepId = 'language' | 'overview' | 'acp' | 'apiKey' | 'git' | 'review'

export type OnboardingStep = {
  id: OnboardingStepId
  labelKey: string
  skip?: boolean
}

export const ONBOARDING_STEPS: OnboardingStep[] = [
  { id: 'language', labelKey: 'pages.onboarding.steps.language' },
  { id: 'overview', labelKey: 'pages.onboarding.steps.overview' },
  { id: 'acp', labelKey: 'pages.onboarding.steps.acp' },
  { id: 'apiKey', labelKey: 'pages.onboarding.steps.apiKey' },
  { id: 'git', labelKey: 'pages.onboarding.steps.git', skip: true },
  { id: 'review', labelKey: 'pages.onboarding.steps.review' },
]

export const ONBOARDING_GIT_TYPES: { id: GitCredentialType; labelKey: string }[] = [
  { id: 'github_https', labelKey: 'pages.agentStudio.git.types.github_https' },
  { id: 'gitlab_https', labelKey: 'pages.agentStudio.git.types.gitlab_https' },
  { id: 'ssh', labelKey: 'pages.agentStudio.git.types.ssh' },
]

export type OnboardingDraft = {
  step: number
  language: AppLocale
  acpBackend: BackendId
  region: string
  apiKey: string
  gitCredentialType: GitCredentialType | ''
  githubToken: string
  gitlabToken: string
  gitlabUrl: string
  gitSshPrivateKey: string
  gitSshKnownHosts: string
  repoUrl: string
  repoBranch: string
  gitUserName: string
  gitUserEmail: string
  vncPreview: boolean
  browserMcp: boolean
}

export type OnboardingBootstrapBody = {
  acpBackend: BackendId
  apiKey: string
  region?: string
  gitCredentialType?: GitCredentialType
  githubToken?: string
  gitlabToken?: string
  gitlabUrl?: string
  gitSshPrivateKey?: string
  gitSshKnownHosts?: string
  repoUrl?: string
  repoBranch?: string
  gitUserName?: string
  gitUserEmail?: string
  vncPreview?: boolean
  browserMcp?: boolean
}

export type OnboardingBootstrapResult = {
  agentIds: string[]
  workflowId: string
  published: boolean
  groupName?: string
}

const DISMISS_PREFIX = 'approving-onboarding-dismiss:'

export function onboardingDismissKey(projectId: string): string {
  return `${DISMISS_PREFIX}${projectId}`
}

export function isOnboardingDismissed(projectId: string): boolean {
  if (!projectId) return true
  try {
    return localStorage.getItem(onboardingDismissKey(projectId)) === '1'
  } catch {
    return true
  }
}

export function dismissOnboarding(projectId: string): void {
  if (!projectId) return
  try {
    localStorage.setItem(onboardingDismissKey(projectId), '1')
  } catch {
    /* ignore */
  }
}

/** First-install empty default project: 0 workflows, 0 bound agents, no name conflicts. */
export function isEmptyProjectForOnboarding(
  workflowCount: number,
  agents: { name?: string; projectId?: string }[],
  projectId: string,
): boolean {
  if (projectId !== DEFAULT_PROJECT_ID) return false
  if (workflowCount > 0) return false
  const bound = agents.filter((a) => (a.projectId || '') === projectId)
  if (bound.length > 0) return false
  const conflict = agents.some((a) => {
    const name = (a.name || '').trim()
    if (!name || !(ONBOARDING_AGENT_NAMES as readonly string[]).includes(name)) return false
    const owner = (a.projectId || '').trim()
    return owner !== '' && owner !== projectId
  })
  return !conflict
}

export function shouldAutoOpenOnboarding(
  projectId: string,
  workflowCount: number,
  agents: { name?: string; projectId?: string }[],
): boolean {
  if (!projectId) return false
  if (isOnboardingDismissed(projectId)) return false
  return isEmptyProjectForOnboarding(workflowCount, agents, projectId)
}

export function freshOnboardingDraft(): OnboardingDraft {
  const policy = getRegionPolicy('cursor')
  return {
    step: 0,
    language: detectSystemLocale(),
    acpBackend: 'cursor',
    region: policy?.defaultRegion || '',
    apiKey: '',
    gitCredentialType: '',
    githubToken: '',
    gitlabToken: '',
    gitlabUrl: '',
    gitSshPrivateKey: '',
    gitSshKnownHosts: '',
    repoUrl: '',
    repoBranch: '',
    gitUserName: '',
    gitUserEmail: '',
    vncPreview: true,
    browserMcp: true,
  }
}

/** Mirrors the server's RepoNameFromURL so the wizard can preview the clone dir. */
export function repoNameFromUrl(raw: string): string {
  const trimmed = raw.trim().replace(/\/+$/, '')
  if (!trimmed) return ''
  const seg = trimmed.split(/[/:]/).pop() || ''
  return seg.replace(/\.git$/, '').trim()
}

export function repoConfigured(draft: OnboardingDraft): boolean {
  return Boolean(draft.repoUrl.trim())
}

export function gitIdentityConfigured(draft: OnboardingDraft): boolean {
  return Boolean(draft.gitUserName.trim() && draft.gitUserEmail.trim())
}

/** First-install language defaults to the browser/OS language, not a prior app preference. */
export function detectSystemLocale(): AppLocale {
  if (typeof navigator === 'undefined') return 'zh-CN'
  return (navigator.language || '').toLowerCase().startsWith('zh') ? 'zh-CN' : 'en'
}

export function gitConfigured(draft: OnboardingDraft): boolean {
  if (!draft.gitCredentialType) return false
  if (draft.gitCredentialType === 'github_https') return Boolean(draft.githubToken.trim())
  if (draft.gitCredentialType === 'gitlab_https') return Boolean(draft.gitlabToken.trim())
  if (draft.gitCredentialType === 'ssh') return Boolean(draft.gitSshPrivateKey.trim())
  return false
}

export function assembleBootstrapBody(draft: OnboardingDraft): OnboardingBootstrapBody {
  const body: OnboardingBootstrapBody = {
    acpBackend: draft.acpBackend,
    apiKey: draft.apiKey.trim(),
  }
  const policy = getRegionPolicy(draft.acpBackend)
  if (policy && draft.region.trim()) {
    body.region = draft.region.trim()
  }
  if (draft.gitCredentialType) {
    body.gitCredentialType = draft.gitCredentialType
  }
  if (draft.githubToken.trim()) body.githubToken = draft.githubToken.trim()
  if (draft.gitlabToken.trim()) body.gitlabToken = draft.gitlabToken.trim()
  if (draft.gitlabUrl.trim()) body.gitlabUrl = draft.gitlabUrl.trim()
  if (draft.gitSshPrivateKey.trim()) body.gitSshPrivateKey = draft.gitSshPrivateKey.trim()
  if (draft.gitSshKnownHosts.trim()) body.gitSshKnownHosts = draft.gitSshKnownHosts.trim()
  if (draft.repoUrl.trim()) {
    body.repoUrl = draft.repoUrl.trim()
    if (draft.repoBranch.trim()) body.repoBranch = draft.repoBranch.trim()
  }
  if (draft.gitUserName.trim()) body.gitUserName = draft.gitUserName.trim()
  if (draft.gitUserEmail.trim()) body.gitUserEmail = draft.gitUserEmail.trim()
  body.vncPreview = draft.vncPreview
  body.browserMcp = draft.browserMcp
  return body
}
