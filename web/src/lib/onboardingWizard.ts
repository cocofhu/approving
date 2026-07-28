import type { BackendId } from '@/lib/regionPolicy'
import { getRegionPolicy } from '@/lib/regionPolicy'

export const ONBOARDING_WORKFLOW_NAME = '快速上手·轻量'

export const DEFAULT_ONBOARDING_REPO = {
  name: 'demo',
  url: 'https://github.com/heroku/nodejs-getting-started.git',
  branch: 'main',
} as const

export const DEFAULT_ONBOARDING_REPOS_LITERAL =
  `${DEFAULT_ONBOARDING_REPO.name}|${DEFAULT_ONBOARDING_REPO.url}|${DEFAULT_ONBOARDING_REPO.branch}`

export const DEFAULT_ONBOARDING_FEATURE = '把首页欢迎文案与主按钮文案改得更清晰友好'

export const ONBOARDING_AGENT_NAMES = [
  'ClarifyAgent',
  'VisualAgent',
  'ImplementAgent',
  'TestAgent',
  'PreviewAgent',
] as const

export type OnboardingStepId = 'overview' | 'acp' | 'apiKey' | 'git' | 'review'

export type OnboardingStep = {
  id: OnboardingStepId
  labelKey: string
  skip?: boolean
}

export const ONBOARDING_STEPS: OnboardingStep[] = [
  { id: 'overview', labelKey: 'pages.onboarding.steps.overview' },
  { id: 'acp', labelKey: 'pages.onboarding.steps.acp' },
  { id: 'apiKey', labelKey: 'pages.onboarding.steps.apiKey' },
  { id: 'git', labelKey: 'pages.onboarding.steps.git', skip: true },
  { id: 'review', labelKey: 'pages.onboarding.steps.review' },
]

export type OnboardingRepoFields = {
  name: string
  url: string
  branch: string
}

export type OnboardingDraft = {
  step: number
  acpBackend: BackendId
  region: string
  apiKey: string
  repo: OnboardingRepoFields
  advOpen: boolean
}

export type OnboardingBootstrapBody = {
  acpBackend: BackendId
  apiKey: string
  region?: string
  repos?: string
}

export type OnboardingBootstrapResult = {
  agentIds: string[]
  workflowId: string
  repos: string
  feature: string
  published: boolean
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
    return false
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

export function clearOnboardingDismiss(projectId: string): void {
  if (!projectId) return
  try {
    localStorage.removeItem(onboardingDismissKey(projectId))
  } catch {
    /* ignore */
  }
}

/** Empty project for onboarding: 0 workflows, 0 agents bound to this project,
 * and none of the fixed onboarding agent names are owned by another project. */
export function isEmptyProjectForOnboarding(
  workflowCount: number,
  agents: { name?: string; projectId?: string }[],
  projectId: string,
): boolean {
  if (!projectId) return false
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
    acpBackend: 'cursor',
    region: policy?.defaultRegion || '',
    apiKey: '',
    repo: { ...DEFAULT_ONBOARDING_REPO },
    advOpen: false,
  }
}

export function encodeReposLiteral(repo: OnboardingRepoFields): string {
  const name = (repo.name || '').trim() || DEFAULT_ONBOARDING_REPO.name
  const url = (repo.url || '').trim() || DEFAULT_ONBOARDING_REPO.url
  const branch = (repo.branch || '').trim() || DEFAULT_ONBOARDING_REPO.branch
  return `${name}|${url}|${branch}`
}

export function parseReposLiteral(literal: string): OnboardingRepoFields {
  const parts = (literal || '').split('|')
  if (parts.length < 2) {
    return { ...DEFAULT_ONBOARDING_REPO }
  }
  return {
    name: parts[0]?.trim() || DEFAULT_ONBOARDING_REPO.name,
    url: parts[1]?.trim() || DEFAULT_ONBOARDING_REPO.url,
    branch: (parts[2] || 'main').trim() || 'main',
  }
}

/** Structured repos value accepted by StartRun / workflow Type "repos". */
export function reposInputFromFields(repo: OnboardingRepoFields): Array<{ name: string; url: string; branch: string }> {
  const fields = {
    name: (repo.name || '').trim() || DEFAULT_ONBOARDING_REPO.name,
    url: (repo.url || '').trim() || DEFAULT_ONBOARDING_REPO.url,
    branch: (repo.branch || '').trim() || DEFAULT_ONBOARDING_REPO.branch,
  }
  return [fields]
}

export function assembleBootstrapBody(draft: OnboardingDraft): OnboardingBootstrapBody {
  const body: OnboardingBootstrapBody = {
    acpBackend: draft.acpBackend,
    apiKey: draft.apiKey.trim(),
    repos: encodeReposLiteral(draft.repo),
  }
  const policy = getRegionPolicy(draft.acpBackend)
  if (policy && draft.region.trim()) {
    body.region = draft.region.trim()
  }
  return body
}

export function hostLabelFromUrl(url: string): string {
  try {
    const u = new URL(url)
    if (u.hostname.includes('github')) return 'GitHub'
    if (u.hostname.includes('gitlab')) return 'GitLab'
    return u.hostname || 'Git'
  } catch {
    return 'Git'
  }
}
