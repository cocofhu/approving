import type { Agent, AgentFile, AgentPrompts, MCPServer } from '@/lib/api'
import type { GitCredentialType } from '@/lib/gitCredentialAnalysis'
import { validateAgentName } from '@/lib/agentIO'
import {
  ACP_BACKENDS,
  isManagedRegionKey,
  normalizeRegions,
  regionSummary,
  switchBackendRegions,
  type BackendId,
} from '@/lib/regionPolicy'

export type WizardBackendId = BackendId
export { ACP_BACKENDS }
export type WizardStepId =
  | 'basics'
  | 'acp'
  | 'git'
  | 'env'
  | 'mcp'
  | 'rules'
  | 'skills'
  | 'commands'
  | 'prompts'
  | 'review'

export type WizardKV = { k: string; v: string }

export type WizardMCP = {
  name: string
  transport: 'url' | 'command'
  url: string
  headers: WizardKV[]
  command: string
  args: string
  env: WizardKV[]
}

export type WizardSkill = { name: string; content: string }
export type WizardCommand = { name: string; content: string }

export type WizardPromptKey = keyof AgentPrompts
export type WizardPrompts = Record<WizardPromptKey, string>

export type WizardSkipped = Partial<Record<WizardStepId, boolean>>

export type WizardDraft = {
  step: number
  name: string
  description: string
  acpBackend: WizardBackendId
  gitCredentialType?: GitCredentialType
  configRoot: string
  env: WizardKV[]
  mcp: WizardMCP[]
  rulesEdited: boolean
  rulesContent: string
  skills: WizardSkill[]
  commands: WizardCommand[]
  prompts: WizardPrompts
  skipped: WizardSkipped
}

export type WizardStepDef = {
  id: WizardStepId
  labelKey: string
  skip: boolean
}

export const WIZARD_STEPS: WizardStepDef[] = [
  { id: 'basics', labelKey: 'pages.agentStudio.wizard.steps.basics', skip: false },
  { id: 'acp', labelKey: 'pages.agentStudio.wizard.steps.acp', skip: true },
  { id: 'git', labelKey: 'pages.agentStudio.wizard.steps.git', skip: true },
  { id: 'env', labelKey: 'pages.agentStudio.wizard.steps.env', skip: true },
  { id: 'mcp', labelKey: 'pages.agentStudio.wizard.steps.mcp', skip: true },
  { id: 'rules', labelKey: 'pages.agentStudio.wizard.steps.rules', skip: true },
  { id: 'skills', labelKey: 'pages.agentStudio.wizard.steps.skills', skip: true },
  { id: 'commands', labelKey: 'pages.agentStudio.wizard.steps.commands', skip: true },
  { id: 'prompts', labelKey: 'pages.agentStudio.wizard.steps.prompts', skip: true },
  { id: 'review', labelKey: 'pages.agentStudio.wizard.steps.review', skip: false },
]

export const DEFAULT_CONFIG_ROOT = '/root/.cursor'
export const DEFAULT_WORKSPACE_DIR = '/root/workspace'

export const WIZARD_PROMPT_KEYS: WizardPromptKey[] = [
  'upstreamArtifactsHeader',
  'producesContract',
  'reactOpenSuffix',
  'producesRetry',
]

export const GIT_ENV_KEYS = new Set([
  'GIT_REPOS',
  'GITHUB_TOKEN',
  'GITHUB_URL',
  'GITLAB_TOKEN',
  'GITLAB_URL',
  'GIT_SSH_PRIVATE_KEY',
  'GIT_SSH_KNOWN_HOSTS',
])

export function emptyPrompts(): WizardPrompts {
  return {
    upstreamArtifactsHeader: '',
    producesContract: '',
    reactOpenSuffix: '',
    producesRetry: '',
  }
}

export function freshDraft(): WizardDraft {
  return {
    step: 0,
    name: '',
    description: '',
    acpBackend: 'cursor',
    gitCredentialType: undefined,
    configRoot: DEFAULT_CONFIG_ROOT,
    env: [],
    mcp: [],
    rulesEdited: false,
    rulesContent: '',
    skills: [],
    commands: [],
    prompts: emptyPrompts(),
    skipped: {},
  }
}

export function configRootFor(backend: WizardBackendId): string {
  return ACP_BACKENDS.find((b) => b.id === backend)?.configRoot || DEFAULT_CONFIG_ROOT
}

export function applyAcpBackend(draft: WizardDraft, id: WizardBackendId): void {
  draft.acpBackend = id
  draft.configRoot = configRootFor(id)
  draft.env = recToKV(switchBackendRegions(kvToRec(draft.env), id))
}

/** True when switching Backend may remapping path-dependent configs. */
export function hasPathDeps(draft: WizardDraft): boolean {
  return (
    draft.mcp.length > 0 ||
    draft.skills.length > 0 ||
    draft.commands.length > 0 ||
    draft.rulesEdited
  )
}

/** Default alwaysApply identity rule; description (if any) becomes the preface. */
export function buildDefaultRule(name: string, description = ''): string {
  const n = name.trim() || 'agent'
  const intro = description.trim() ? `${description.trim()}\n\n` : ''
  return `---\ndescription: ${n} 身份\nalwaysApply: true\n---\n\n# ${n}\n\n${intro}描述该 Agent 的职责与行为。`
}

export function defaultSkillTemplate(name: string): string {
  const n = name.trim() || 'skill'
  return `---\nname: ${n}\ndescription: \n---\n\n# ${n}\n\n描述该 Skill 的用途与用法。\n`
}

export function defaultCommandTemplate(name: string): string {
  const n = name.trim() || 'command'
  return `# ${n}\n\n描述该 Command 的用途与触发方式。\n`
}

export function kvToRec(kvs: WizardKV[]): Record<string, string> {
  const out: Record<string, string> = {}
  for (const { k, v } of kvs) {
    if (k.trim()) out[k.trim()] = v
  }
  return out
}

export function recToKV(rec: Record<string, string>): WizardKV[] {
  return Object.entries(rec).map(([k, v]) => ({ k, v }))
}

export function normalizeWizardRegions(draft: WizardDraft): Record<string, string> {
  const normalized = normalizeRegions(kvToRec(draft.env), draft.acpBackend, 'strict').env
  draft.env = recToKV(normalized)
  return normalized
}

function draftMcpToApi(m: WizardMCP): MCPServer {
  if (m.transport === 'command') {
    return {
      name: m.name.trim(),
      command: m.command.trim(),
      args: m.args
        .split('\n')
        .map((s) => s.trim())
        .filter(Boolean),
      env: kvToRec(m.env),
    }
  }
  return {
    name: m.name.trim(),
    url: m.url.trim(),
    headers: kvToRec(m.headers),
  }
}

function draftPromptsToApi(p: WizardPrompts, skipped: boolean): AgentPrompts | undefined {
  if (skipped) return undefined
  const out: AgentPrompts = {}
  let any = false
  for (const k of WIZARD_PROMPT_KEYS) {
    if (p[k].trim()) {
      out[k] = p[k]
      any = true
    }
  }
  return any ? out : undefined
}

function collectFiles(draft: WizardDraft): AgentFile[] {
  const files: AgentFile[] = []
  const name = draft.name.trim() || 'agent'
  const ruleContent =
    draft.rulesEdited && draft.rulesContent.trim()
      ? draft.rulesContent
      : buildDefaultRule(name, draft.description)
  files.push({ path: `rules/${name}.md`, content: ruleContent })

  for (const s of draft.skills) {
    const slug = s.name.trim()
    if (!slug) continue
    files.push({
      path: `skills/${slug}/SKILL.md`,
      content: s.content || defaultSkillTemplate(slug),
    })
  }
  for (const c of draft.commands) {
    const slug = c.name.trim()
    if (!slug) continue
    files.push({
      path: `commands/${slug}.md`,
      content: c.content || defaultCommandTemplate(slug),
    })
  }
  return files
}

/** Assemble POST /agents payload. Skip Rules still writes default rule; Skip Prompts omits prompts. */
export function assembleCreatePayload(draft: WizardDraft): Agent {
  const name = draft.name.trim()
  const prompts = draftPromptsToApi(draft.prompts, !!draft.skipped.prompts)
  const env = normalizeWizardRegions(draft)
  return {
    name,
    acpBackend: draft.acpBackend || 'cursor',
    ...(draft.gitCredentialType ? { gitCredentialType: draft.gitCredentialType } : {}),
    files: collectFiles(draft),
    mcp: draft.mcp.filter((m) => m.name.trim()).map(draftMcpToApi),
    env,
    layout: {
      configRoot: draft.configRoot.trim() || configRootFor(draft.acpBackend),
      workspaceDir: DEFAULT_WORKSPACE_DIR,
    },
    ...(prompts ? { prompts } : {}),
  }
}

export function validateBasics(draft: WizardDraft, existingNames: string[]): string {
  const code = validateAgentName(draft.name)
  if (code === 'required') return 'required'
  if (code === 'invalid') return 'invalid'
  if (existingNames.includes(draft.name.trim())) return 'exists'
  return ''
}

export function envConfiguredCount(draft: WizardDraft, gitOnly = false): number {
  return draft.env.filter((e) => {
    const k = e.k.trim()
    if (!k) return false
    return gitOnly ? GIT_ENV_KEYS.has(k) : !GIT_ENV_KEYS.has(k) && !isManagedRegionKey(k)
  }).length
}

export function promptConfiguredCount(draft: WizardDraft): number {
  return WIZARD_PROMPT_KEYS.filter((k) => draft.prompts[k].trim()).length
}

export type ReviewChipKind = 'ok' | 'empty' | 'def'

export type ReviewSummaryItem = {
  key: string
  kind: ReviewChipKind
  labelKey: string
  detail?: string
}

/** Build confirmation-page summary chips (Rules default vs empty, Prompts platform default, etc.). */
export function buildReviewSummary(draft: WizardDraft): ReviewSummaryItem[] {
  const normalizedEnv = normalizeRegions(kvToRec(draft.env), draft.acpBackend, 'strict').env
  const region = regionSummary(normalizedEnv, draft.acpBackend, 'strict')
  const name = draft.name.trim() || '—'
  const gitN = envConfiguredCount(draft, true)
  const envN = envConfiguredCount(draft, false)
  const mcpN = draft.mcp.filter((m) => m.name.trim()).length
  const skillN = draft.skills.filter((s) => s.name.trim()).length
  const cmdN = draft.commands.filter((c) => c.name.trim()).length
  const promptN = promptConfiguredCount(draft)
  const rulesSkipped = !!draft.skipped.rules && !draft.rulesEdited
  const promptsSkipped = !!draft.skipped.prompts || promptN === 0

  const items: ReviewSummaryItem[] = [
    { key: 'name', kind: 'ok', labelKey: 'pages.agentStudio.wizard.review.name', detail: name },
    ...(region
      ? [
          {
            key: 'region',
            kind: 'ok' as const,
            labelKey: region.labelKey!,
            detail: region.region,
          },
        ]
      : []),
    {
      key: 'acp',
      kind: 'ok',
      labelKey: 'pages.agentStudio.wizard.review.acp',
      detail: `${draft.acpBackend} · ${draft.configRoot}`,
    },
    {
      key: 'git',
      kind: gitN > 0 ? 'ok' : 'empty',
      labelKey: 'pages.agentStudio.wizard.review.git',
      detail: gitN > 0 ? String(gitN) : undefined,
    },
    {
      key: 'env',
      kind: envN > 0 ? 'ok' : 'empty',
      labelKey: 'pages.agentStudio.wizard.review.env',
      detail: envN > 0 ? String(envN) : undefined,
    },
    {
      key: 'mcp',
      kind: mcpN > 0 ? 'ok' : 'empty',
      labelKey: 'pages.agentStudio.wizard.review.mcp',
      detail: mcpN > 0 ? String(mcpN) : undefined,
    },
    {
      key: 'rules',
      kind: rulesSkipped ? 'def' : draft.rulesEdited ? 'ok' : 'def',
      labelKey: rulesSkipped || !draft.rulesEdited
        ? 'pages.agentStudio.wizard.review.rulesDefault'
        : 'pages.agentStudio.wizard.review.rulesCustom',
    },
    {
      key: 'skills',
      kind: skillN > 0 ? 'ok' : 'empty',
      labelKey: 'pages.agentStudio.wizard.review.skills',
      detail: skillN > 0 ? String(skillN) : undefined,
    },
    {
      key: 'commands',
      kind: cmdN > 0 ? 'ok' : 'empty',
      labelKey: 'pages.agentStudio.wizard.review.commands',
      detail: cmdN > 0 ? String(cmdN) : undefined,
    },
    {
      key: 'prompts',
      kind: promptsSkipped ? 'def' : 'ok',
      labelKey: promptsSkipped
        ? 'pages.agentStudio.wizard.review.promptsDefault'
        : 'pages.agentStudio.wizard.review.promptsCustom',
      detail: promptsSkipped ? undefined : String(promptN),
    },
  ]
  return items
}
