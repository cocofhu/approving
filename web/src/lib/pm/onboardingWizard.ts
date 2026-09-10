import type { BackendId } from '@/lib/shared/regionPolicy'
import { ACP_BACKENDS, getRegionPolicy } from '@/lib/shared/regionPolicy'
import type { GitCredentialType } from '@/lib/agent/gitCredentialAnalysis'
import type { AppLocale } from '@/lib/shared/loadLocaleMessages'
import type { ThemeName } from '@/lib/shared/theme'
import { theme } from '@/lib/shared/theme'
import { DEFAULT_OPENCODE_PROVIDER } from '@/lib/agent/openCodeProvider'

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

/**
 * Two ways to start: bring a model-vendor API key (BYOK, served by the OpenCode
 * backend), or sign in with a coding-CLI vendor account (Cursor / Claude Code /
 * CodeBuddy / Trae). The backend step picks a path first, then its detail.
 */
export type OnboardingStartPath = 'apiKey' | 'cli'

/** BYOK path is served by a single backend; keep the mapping in one place. */
export const ONBOARDING_APIKEY_BACKEND: BackendId = 'opencode'

export const ONBOARDING_CLI_BACKEND_DEFAULT: BackendId = 'cursor'

/** CLI-account backends, in ACP_BACKENDS order (BYOK backend excluded). */
export const ONBOARDING_CLI_BACKENDS = ACP_BACKENDS.filter(
  (b) => b.id !== ONBOARDING_APIKEY_BACKEND,
)

export function startPathForBackend(backend: BackendId): OnboardingStartPath {
  return backend === ONBOARDING_APIKEY_BACKEND ? 'apiKey' : 'cli'
}

export const ONBOARDING_GIT_TYPES: { id: GitCredentialType; labelKey: string }[] = [
  { id: 'github_https', labelKey: 'pages.agentStudio.git.types.github_https' },
  { id: 'gitlab_https', labelKey: 'pages.agentStudio.git.types.gitlab_https' },
  { id: 'ssh', labelKey: 'pages.agentStudio.git.types.ssh' },
]

export type OnboardingDraft = {
  step: number
  language: AppLocale
  theme: ThemeName
  startPath: OnboardingStartPath
  acpBackend: BackendId
  /** Last CLI-path backend, so switching paths back restores the pick. */
  cliBackend: BackendId
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
  openCodeProvider: string
  openCodeBaseURL: string
  openCodeModel: string
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
  openCodeProvider?: string
  openCodeBaseURL?: string
  openCodeModel?: string
}

export type OnboardingBootstrapResult = {
  agentIds: string[]
  workflowId: string
  published: boolean
  groupName?: string
}

/**
 * Hard suppression for tests and local debugging only. The wizard's "later"
 * button deliberately does NOT write this: closing it is per-view, and a reload
 * re-opens the wizard until the default workflow exists (see needsOnboarding).
 * The key differs from the old `approving-onboarding-dismiss:` one so browsers
 * that dismissed the wizard before this rule change are not stuck forever.
 */
const SUPPRESS_PREFIX = 'approving-onboarding-suppress:'

export function onboardingSuppressKey(projectId: string): string {
  return `${SUPPRESS_PREFIX}${projectId}`
}

export function isOnboardingSuppressed(projectId: string): boolean {
  if (!projectId) return true
  try {
    return localStorage.getItem(onboardingSuppressKey(projectId)) === '1'
  } catch {
    return true
  }
}

export function suppressOnboarding(projectId: string): void {
  if (!projectId) return
  try {
    localStorage.setItem(onboardingSuppressKey(projectId), '1')
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

/** The default workflow is the completion marker for first install. */
export function hasDefaultWorkflow(workflows: { name?: string }[]): boolean {
  return workflows.some((w) => (w.name || '').trim() === ONBOARDING_WORKFLOW_NAME)
}

/**
 * First install is pending for as long as the default project has no default
 * workflow — that, not a "seen it" flag, is what gates the wizard. Already-bound
 * agents do not count as done (bootstrap is idempotent and re-upserts them), but
 * a fixed-name agent owned by another project would make bootstrap fail, so that
 * case stays blocked.
 */
export function needsOnboarding(
  workflows: { name?: string }[],
  agents: { name?: string; projectId?: string }[],
  projectId: string,
): boolean {
  if (projectId !== DEFAULT_PROJECT_ID) return false
  if (hasDefaultWorkflow(workflows)) return false
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
  workflows: { name?: string }[],
  agents: { name?: string; projectId?: string }[],
): boolean {
  if (!projectId) return false
  if (isOnboardingSuppressed(projectId)) return false
  return needsOnboarding(workflows, agents, projectId)
}

export function freshOnboardingDraft(): OnboardingDraft {
  return {
    step: 0,
    language: detectSystemLocale(),
    theme: theme.value,
    startPath: 'apiKey',
    acpBackend: ONBOARDING_APIKEY_BACKEND,
    cliBackend: ONBOARDING_CLI_BACKEND_DEFAULT,
    region: getRegionPolicy(ONBOARDING_APIKEY_BACKEND)?.defaultRegion || '',
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
    openCodeProvider: DEFAULT_OPENCODE_PROVIDER,
    openCodeBaseURL: '',
    openCodeModel: '',
  }
}

/**
 * Select a backend. Keeps startPath in sync and resets values that belong to the
 * previous backend (region default, and the key, which is vendor-specific).
 */
export function applyOnboardingBackend(draft: OnboardingDraft, id: BackendId): void {
  const path = startPathForBackend(id)
  draft.startPath = path
  if (path === 'cli') draft.cliBackend = id
  if (draft.acpBackend === id) return
  draft.acpBackend = id
  draft.region = getRegionPolicy(id)?.defaultRegion || ''
  draft.apiKey = ''
  if (id === ONBOARDING_APIKEY_BACKEND && !draft.openCodeProvider) {
    draft.openCodeProvider = DEFAULT_OPENCODE_PROVIDER
  }
}

/** Switch start path; the CLI path restores the last CLI backend that was picked. */
export function applyStartPath(draft: OnboardingDraft, path: OnboardingStartPath): void {
  const id =
    path === 'apiKey'
      ? ONBOARDING_APIKEY_BACKEND
      : draft.cliBackend || ONBOARDING_CLI_BACKEND_DEFAULT
  applyOnboardingBackend(draft, id)
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
  if (draft.acpBackend === 'opencode') {
    body.openCodeProvider = draft.openCodeProvider || 'openai'
    if (draft.openCodeBaseURL.trim()) body.openCodeBaseURL = draft.openCodeBaseURL.trim()
    if (draft.openCodeModel.trim()) body.openCodeModel = draft.openCodeModel.trim()
  }
  return body
}
