import type { BackendId } from '@/lib/shared/regionPolicy'
import { getRegionPolicy } from '@/lib/shared/regionPolicy'
import type { GitCredentialType } from '@/lib/agent/gitCredentialAnalysis'
import type { AppLocale } from '@/lib/shared/loadLocaleMessages'
import type { ThemeName } from '@/lib/shared/theme'
import { locale } from '@/lib/shared/locale'
import { theme } from '@/lib/shared/theme'
import { DEFAULT_OPENCODE_PROVIDER } from '@/lib/agent/openCodeProvider'
import { normalizeAgentName, validateAgentName } from '@/lib/agent/agentIO'
import {
  APIKEY_BACKEND,
  CLI_BACKEND_DEFAULT,
  CLI_BACKENDS,
  type StartPath,
  startPathForBackend,
  syncStartPathFields,
} from '@/lib/shared/startPath'

export {
  APIKEY_BACKEND as ONBOARDING_APIKEY_BACKEND,
  CLI_BACKEND_DEFAULT as ONBOARDING_CLI_BACKEND_DEFAULT,
  CLI_BACKENDS as ONBOARDING_CLI_BACKENDS,
  startPathForBackend,
  type StartPath as OnboardingStartPath,
}

/** Matches models.DefaultProjectID — first-install wizard only opens here. */
export const DEFAULT_PROJECT_ID = 'proj-default'

export const ONBOARDING_WORKFLOW_NAME = '默认工作流'
export const FIRST_INSTALL_GROUP_NAME = '综合项目组'
export const ONBOARDING_NAME_MARKER = '综合'

/** Longest role suffix after replacing 综合 (代码审查工程师). */
const LONGEST_ONBOARDING_ROLE_SUFFIX = 7
const MAX_AGENT_NAME_RUNES = 64

export const ONBOARDING_AGENT_NAMES = [
  '综合AI技术产品',
  '综合研发工程师',
  '综合测试工程师',
  '综合代码审查工程师',
  '综合运维工程师',
  '综合项目组组长',
] as const

export type OnboardingMode = 'firstInstall' | 'createProject' | 'retry'

export type OnboardingStepId =
  | 'projectName'
  | 'language'
  | 'overview'
  | 'acp'
  | 'apiKey'
  | 'git'
  | 'review'

export type OnboardingStep = {
  id: OnboardingStepId
  labelKey: string
  skip?: boolean
}

const BASE_ONBOARDING_STEPS: OnboardingStep[] = [
  { id: 'language', labelKey: 'pages.onboarding.steps.language' },
  { id: 'overview', labelKey: 'pages.onboarding.steps.overview' },
  { id: 'acp', labelKey: 'pages.onboarding.steps.acp' },
  { id: 'apiKey', labelKey: 'pages.onboarding.steps.apiKey' },
  { id: 'git', labelKey: 'pages.onboarding.steps.git', skip: true },
  { id: 'review', labelKey: 'pages.onboarding.steps.review' },
]

/** Default-project / retry steps (no project name). */
export const ONBOARDING_STEPS: OnboardingStep[] = BASE_ONBOARDING_STEPS

export function onboardingStepsForMode(mode: OnboardingMode): OnboardingStep[] {
  if (mode === 'createProject') {
    // New project is not first-install: skip language/theme; inherit app prefs.
    return [
      { id: 'projectName', labelKey: 'pages.onboarding.steps.projectName' },
      ...BASE_ONBOARDING_STEPS.filter((s) => s.id !== 'language'),
    ]
  }
  return BASE_ONBOARDING_STEPS
}

export const ONBOARDING_GIT_TYPES: { id: GitCredentialType; labelKey: string }[] = [
  { id: 'github_https', labelKey: 'pages.agentStudio.git.types.github_https' },
  { id: 'gitlab_https', labelKey: 'pages.agentStudio.git.types.gitlab_https' },
  { id: 'ssh', labelKey: 'pages.agentStudio.git.types.ssh' },
]

export type OnboardingDraft = {
  step: number
  projectName: string
  language: AppLocale
  theme: ThemeName
  startPath: StartPath
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
  openCodeModelVision: boolean
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
  openCodeModelVision?: boolean
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

/** Wash project name into a valid Agent-name prefix (mirrors server SanitizeOnboardingPrefix). */
export function sanitizeOnboardingPrefix(projectName: string): string {
  const raw = normalizeAgentName(projectName)
  if (!raw) return ''
  let out = ''
  for (const ch of raw) {
    if (/\s/u.test(ch)) continue
    if (/[./\\]/u.test(ch)) continue
    if (/[－＿．／＼、，。！？：；（）【】]/u.test(ch)) continue
    if (/^[\p{L}\p{N}_-]$/u.test(ch)) out += ch
  }
  const maxPrefix = MAX_AGENT_NAME_RUNES - LONGEST_ONBOARDING_ROLE_SUFFIX
  if (Array.from(out).length > maxPrefix) {
    out = Array.from(out).slice(0, maxPrefix).join('')
  }
  return validateAgentName(out) === '' ? out : ''
}

/** Agent names that bootstrap will create for this project. */
export function deriveOnboardingAgentNames(projectId: string, projectName: string): string[] {
  if (projectId === DEFAULT_PROJECT_ID) return [...ONBOARDING_AGENT_NAMES]
  const prefix = sanitizeOnboardingPrefix(projectName)
  if (!prefix) return []
  return ONBOARDING_AGENT_NAMES.map((n) => n.replace(ONBOARDING_NAME_MARKER, prefix))
}

function hasOnboardingNameConflict(
  agents: { name?: string; projectId?: string }[],
  projectId: string,
  names: readonly string[],
): boolean {
  return agents.some((a) => {
    const name = (a.name || '').trim()
    if (!name || !(names as readonly string[]).includes(name)) return false
    const owner = (a.projectId || '').trim()
    return owner !== '' && owner !== projectId
  })
}

/**
 * Empty project eligible for install CTA: 0 workflows, 0 bound agents, no name conflicts.
 * Any project may retry; App-level auto-open stays default-only via needsOnboarding.
 */
export function isEmptyProjectForOnboarding(
  workflowCount: number,
  agents: { name?: string; projectId?: string }[],
  projectId: string,
  projectName = '',
): boolean {
  if (!projectId) return false
  if (workflowCount > 0) return false
  const bound = agents.filter((a) => (a.projectId || '') === projectId)
  if (bound.length > 0) return false
  const names = deriveOnboardingAgentNames(projectId, projectName)
  if (!names.length) return projectId === DEFAULT_PROJECT_ID
  return !hasOnboardingNameConflict(agents, projectId, names)
}

/** The default workflow is the completion marker for first install. */
export function hasDefaultWorkflow(workflows: { name?: string }[]): boolean {
  return workflows.some((w) => (w.name || '').trim() === ONBOARDING_WORKFLOW_NAME)
}

/**
 * App-level auto-open only for the default project until 默认工作流 exists.
 */
export function needsOnboarding(
  workflows: { name?: string }[],
  agents: { name?: string; projectId?: string }[],
  projectId: string,
): boolean {
  if (projectId !== DEFAULT_PROJECT_ID) return false
  if (hasDefaultWorkflow(workflows)) return false
  return !hasOnboardingNameConflict(agents, projectId, ONBOARDING_AGENT_NAMES)
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

/**
 * @param opts.inheritAppLocale — createProject: seed from current app locale
 *   (not browser/OS). firstInstall/retry keep detectSystemLocale().
 */
export function freshOnboardingDraft(opts?: { inheritAppLocale?: boolean }): OnboardingDraft {
  return {
    step: 0,
    projectName: '',
    language: opts?.inheritAppLocale ? locale.value : detectSystemLocale(),
    theme: theme.value,
    startPath: 'apiKey',
    acpBackend: APIKEY_BACKEND,
    cliBackend: CLI_BACKEND_DEFAULT,
    region: getRegionPolicy(APIKEY_BACKEND)?.defaultRegion || '',
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
    openCodeModelVision: false,
  }
}

/**
 * Select a backend. Keeps startPath in sync and resets values that belong to the
 * previous backend (region default, and the key, which is vendor-specific).
 */
export function applyOnboardingBackend(draft: OnboardingDraft, id: BackendId): void {
  syncStartPathFields(draft, id)
  if (draft.acpBackend === id) return
  draft.acpBackend = id
  draft.region = getRegionPolicy(id)?.defaultRegion || ''
  draft.apiKey = ''
  if (id === APIKEY_BACKEND && !draft.openCodeProvider) {
    draft.openCodeProvider = DEFAULT_OPENCODE_PROVIDER
  }
}

/** Switch start path; the CLI path restores the last CLI backend that was picked. */
export function applyStartPath(draft: OnboardingDraft, path: StartPath): void {
  const id =
    path === 'apiKey' ? APIKEY_BACKEND : draft.cliBackend || CLI_BACKEND_DEFAULT
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
    body.openCodeModelVision = draft.openCodeModelVision
  }
  return body
}
